package app

import (
	"context"
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

func TestHTTPHandlerAddsSecurityHeadersToTokenAuthenticationFailure(t *testing.T) {
	handler := newHTTPHandler(denyAssertion{}, fstest.MapFS{
		"index.html": {Data: []byte("app")},
	}, auth.NewStore(time.Minute, time.Now), http.NotFoundHandler(), appTestStore{}, appTestTester{})

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/token", nil))

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%q", res.Code, res.Body.String())
	}
	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store",
	} {
		if got := res.Header().Get(name); !strings.Contains(got, want) {
			t.Errorf("%s=%q, want to contain %q", name, got, want)
		}
	}
}

func TestHTTPHandlerRoutesFnOSGatewayPrefix(t *testing.T) {
	terminalCalls := 0
	handler := newHTTPHandler(allowAssertion{}, fstest.MapFS{
		"index.html": {Data: []byte("gateway app")},
	}, auth.NewStore(time.Minute, time.Now), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		terminalCalls++
		w.WriteHeader(http.StatusNoContent)
	}), appTestStore{}, appTestTester{})

	for _, tc := range []struct {
		method string
		path   string
		code   int
	}{{http.MethodGet, "/app/liteterm/", http.StatusOK}, {http.MethodPost, "/app/liteterm/api/token", http.StatusOK}, {http.MethodGet, "/app/liteterm/api/terminal", http.StatusNoContent}} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(tc.method, tc.path, nil))
		if res.Code != tc.code {
			t.Errorf("%s %s: code=%d body=%q", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
	if terminalCalls != 1 {
		t.Fatalf("terminal calls=%d", terminalCalls)
	}
}

type denyAssertion struct{}

func (denyAssertion) Assert(*http.Request) bool { return false }

type allowAssertion struct{}

func (allowAssertion) Assert(*http.Request) bool { return true }

type appTestStore struct{}

func (appTestStore) Load(string) (credentials.Credential, error) {
	return credentials.Credential{}, credentials.ErrNotFound
}
func (appTestStore) Save(string, credentials.Credential) error { return nil }
func (appTestStore) Delete(string) error                       { return nil }

type appTestTester struct{}

func (appTestTester) Test(context.Context, terminal.Credentials) error { return nil }
