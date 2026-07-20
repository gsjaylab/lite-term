package fnos

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	webserver "github.com/gsjaylab/liteterm/internal/web"
)

func TestRouteGatewayStripsApplicationPrefix(t *testing.T) {
	var paths []string
	handler := RouteGateway(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, path := range []string{"/app/liteterm", "/app/liteterm/", "/app/liteterm/api/token", "/healthz"} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
	}
	want := []string{"/", "/", "/api/token", "/healthz"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("path[%d]=%q want %q", i, paths[i], want[i])
		}
	}
}

func TestAuthenticateAPIProtectsAllAPIRoutes(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := AuthenticateAPI(denyAssertion{}, next)

	for _, tc := range []struct {
		path string
		code int
	}{{"/api/token", http.StatusUnauthorized}, {"/api/terminal", http.StatusUnauthorized}, {"/", http.StatusNoContent}} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, tc.path, nil))
		if res.Code != tc.code {
			t.Errorf("%s: code=%d", tc.path, res.Code)
		}
	}
}

func TestAssertionForUnixSocketUsesTransportAssertion(t *testing.T) {
	assertion, err := AssertionFor(filepath.Join(t.TempDir(), "app.sock"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/token", nil)
	request.Header.Set("X-Trim-Userid", "1000")
	if !assertion.Assert(request) {
		t.Fatal("Unix socket assertion denied request")
	}
}

func TestUnixSocketAssertionRequiresValidGatewayUID(t *testing.T) {
	assertion := unixSocketAssertion{}
	for _, uid := range []string{"", "0", "-1", "admin"} {
		request := httptest.NewRequest(http.MethodGet, "/api/terminal", nil)
		request.Header.Set("X-Trim-Userid", uid)
		if assertion.Assert(request) {
			t.Fatalf("accepted invalid UID %q", uid)
		}
	}
}

func TestAssertionForLoopbackRequiresDevelopmentOptIn(t *testing.T) {
	t.Setenv("LITETERM_DEV_AUTH", "")
	if _, err := AssertionFor("127.0.0.1:8189"); err == nil {
		t.Fatal("loopback assertion enabled without LITETERM_DEV_AUTH")
	}
	t.Setenv("LITETERM_DEV_AUTH", "1")
	assertion, err := AssertionFor("127.0.0.1:8189")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/token", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	if !assertion.Assert(request) {
		t.Fatal("development assertion denied loopback request")
	}
}

type denyAssertion struct{}

func (denyAssertion) Assert(*http.Request) bool { return false }

var _ webserver.Assertion = denyAssertion{}
