package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gsjaylab/liteterm/internal/auth"
	"github.com/gsjaylab/liteterm/internal/credentials"
	"github.com/gsjaylab/liteterm/internal/terminal"
)

func TestSecurityHeadersAndServerMethodRules(t *testing.T) {
	handler := SecurityHeaders(newTestServer(fstest.MapFS{"index.html": {Data: []byte("ok")}}, auth.NewStore(time.Minute, time.Now), http.NotFoundHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if got := res.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Fatalf("csp=%q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/token", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d", res.Code)
	}
}

func TestTokenRequiresAssertionAndAPIResponsesAreNotCached(t *testing.T) {
	tokens := auth.NewStore(time.Minute, time.Now)
	handler := SecurityHeaders(newTestServer(fstest.MapFS{"index.html": {Data: []byte("ok")}}, tokens, http.NotFoundHandler()))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/token", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", res.Code)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control=%q", got)
	}

	res = httptest.NewRecorder()
	Authenticate(allowAssertion{}, handler).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/token", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.Token == "" {
		t.Fatalf("body=%q err=%v", res.Body.String(), err)
	}
	if !tokens.Consume(body.Token, "dev-loopback") {
		t.Fatal("issued token is not usable")
	}
}

func TestServerRoutesHealthTerminalAndSPAFallback(t *testing.T) {
	var terminalCalls int
	handler := newTestServer(fstest.MapFS{
		"index.html": {Data: []byte("app")},
		"asset.js":   {Data: []byte("asset")},
	}, auth.NewStore(time.Minute, time.Now), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { terminalCalls++; w.WriteHeader(http.StatusNoContent) }))

	for _, tc := range []struct {
		path string
		code int
		body string
	}{{"/healthz", 200, "ok"}, {"/asset.js", 200, "asset"}, {"/missing.js", 404, "404 page not found"}, {"/some/route", 200, "app"}, {"/api/terminal", 204, ""}} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if res.Code != tc.code || !strings.Contains(res.Body.String(), tc.body) {
			t.Errorf("%s: code=%d body=%q", tc.path, res.Code, res.Body.String())
		}
	}
	if terminalCalls != 1 {
		t.Fatalf("terminal calls=%d", terminalCalls)
	}
}

func TestServerRejectsWrongMethodsForAllAPIEndpoints(t *testing.T) {
	handler := newTestServer(fstest.MapFS{"index.html": {Data: []byte("app")}}, auth.NewStore(time.Minute, time.Now), http.NotFoundHandler())
	for _, tc := range []struct{ method, path string }{{http.MethodPost, "/healthz"}, {http.MethodGet, "/api/token"}, {http.MethodPost, "/api/terminal"}} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(tc.method, tc.path, nil))
		if res.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: code=%d", tc.method, tc.path, res.Code)
		}
	}
}

func TestDevAssertionOnlyAcceptsLoopback(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subject, ok := Subject(r.Context()); !ok || subject != "dev-loopback" {
			t.Errorf("subject=%q ok=%v", subject, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Authenticate(DevLoopbackAssertion{}, inner)
	for _, tc := range []struct {
		remote string
		code   int
	}{{"127.0.0.1:1234", 204}, {"[::1]:1234", 204}, {"192.0.2.1:1234", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/api/token", nil)
		req.RemoteAddr = tc.remote
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.code {
			t.Errorf("%s: code=%d", tc.remote, res.Code)
		}
	}
}

var _ fs.FS = fstest.MapFS{}

func newTestServer(assets fs.FS, tokens *auth.Store, terminalHandler http.Handler) http.Handler {
	return NewServer(assets, tokens, terminalHandler, testCredentialStore{}, testCredentialTester{})
}

type testCredentialStore struct{}

func (testCredentialStore) Load(string) (credentials.Credential, error) {
	return credentials.Credential{}, credentials.ErrNotFound
}
func (testCredentialStore) Save(string, credentials.Credential) error { return nil }
func (testCredentialStore) Delete(string) error                       { return nil }

type testCredentialTester struct{}

func (testCredentialTester) Test(context.Context, terminal.Credentials) error { return nil }

type allowAssertion struct{}

func (allowAssertion) Assert(*http.Request) bool { return true }
