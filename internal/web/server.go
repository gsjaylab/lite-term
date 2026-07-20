package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/gsjaylab/liteterm/internal/auth"
	"github.com/gsjaylab/liteterm/internal/credentials"
	"github.com/gsjaylab/liteterm/internal/terminal"
)

type CredentialTester interface {
	Test(context.Context, terminal.Credentials) error
}

type api struct {
	tokens  *auth.Store
	store   credentials.Store
	tester  CredentialTester
	limiter *attemptLimiter
}

func NewServer(assets fs.FS, tokens *auth.Store, terminalHandler http.Handler, store credentials.Store, tester CredentialTester) http.Handler {
	server := &api{
		tokens:  tokens,
		store:   store,
		tester:  tester,
		limiter: newAttemptLimiter(10, time.Minute),
	}
	mux := http.NewServeMux()
	mux.Handle("/healthz", allowMethod(http.MethodGet, http.HandlerFunc(health)))
	mux.Handle("/api/token", allowMethod(http.MethodPost, http.HandlerFunc(server.issueToken)))
	mux.Handle("/api/credentials", allowMethods(map[string]http.Handler{
		http.MethodGet: http.HandlerFunc(server.getCredential), http.MethodDelete: http.HandlerFunc(server.deleteCredential),
	}))
	mux.Handle("/api/connection/test", allowMethod(http.MethodPost, http.HandlerFunc(server.testConnection)))
	mux.Handle("/api/terminal", allowMethod(http.MethodGet, terminalHandler))
	mux.HandleFunc("/", spaHandler(assets))
	return mux
}

func allowMethod(method string, handler http.Handler) http.Handler {
	return allowMethods(map[string]http.Handler{method: handler})
}

func allowMethods(handlers map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler, ok := handlers[r.Method]; ok {
			handler.ServeHTTP(w, r)
			return
		}
		for method := range handlers {
			w.Header().Add("Allow", method)
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (a *api) issueToken(w http.ResponseWriter, r *http.Request) {
	subject, ok := requireSubject(w, r)
	if !ok {
		return
	}
	token, err := a.tokens.Issue(subject)
	if errors.Is(err, auth.ErrRateLimited) {
		http.Error(w, "too many token requests", http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (a *api) getCredential(w http.ResponseWriter, r *http.Request) {
	subject, ok := requireSubject(w, r)
	if !ok {
		return
	}
	saved, err := a.store.Load(subject)
	if errors.Is(err, credentials.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"saved": false, "port": 22})
		return
	}
	if err != nil {
		http.Error(w, "load credential failed", http.StatusInternalServerError)
		return
	}
	// 密码只在服务端使用；浏览器只需要知道可以复用已有凭据。
	writeJSON(w, http.StatusOK, map[string]any{
		"saved": true, "port": saved.Port, "username": saved.Username, "hasPassword": true,
	})
}

func (a *api) deleteCredential(w http.ResponseWriter, r *http.Request) {
	subject, ok := requireSubject(w, r)
	if !ok {
		return
	}
	if err := a.store.Delete(subject); err != nil {
		http.Error(w, "delete credential failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type connectionTestRequest struct {
	Port               uint16 `json:"port"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	UseSavedCredential bool   `json:"useSavedCredential"`
}

func (a *api) testConnection(w http.ResponseWriter, r *http.Request) {
	subject, ok := requireSubject(w, r)
	if !ok {
		return
	}
	if !a.limiter.Allow(subject, time.Now()) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok": false, "code": "rate_limited", "message": "测试次数过多，请稍后重试",
		})
		return
	}

	var request connectionTestRequest
	if err := decodeJSON(w, r, &request); err != nil || !validConnectionTest(request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "code": "invalid_input", "message": "请检查端口、用户名和密码",
		})
		return
	}

	credential := terminal.Credentials{Port: request.Port, Username: request.Username, Password: request.Password}
	if request.UseSavedCredential {
		saved, err := a.store.Load(subject)
		if err != nil || saved.Port != request.Port || saved.Username != request.Username {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok": false, "code": "saved_credential_missing", "message": "已保存的登录信息不存在，请重新输入密码",
			})
			return
		}
		credential.Password = saved.Password
	}
	if credential.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "code": "invalid_input", "message": "请输入密码"})
		return
	}
	if err := a.tester.Test(r.Context(), credential); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"ok": false, "code": "connection_failed", "message": "用户名、密码或 SSH 端口不正确",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": "success", "message": "连接测试成功"})
}

func requireSubject(w http.ResponseWriter, r *http.Request) (string, bool) {
	subject, ok := Subject(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	return subject, ok
}

func validConnectionTest(request connectionTestRequest) bool {
	return request.Port != 0 && terminal.ValidUsername(request.Username) && len(request.Password) <= 1024
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type attemptWindow struct {
	start time.Time
	count int
}

type attemptLimiter struct {
	mu       sync.Mutex
	maximum  int
	duration time.Duration
	subjects map[string]attemptWindow
}

func newAttemptLimiter(maximum int, duration time.Duration) *attemptLimiter {
	return &attemptLimiter{maximum: maximum, duration: duration, subjects: make(map[string]attemptWindow)}
}

func (l *attemptLimiter) Allow(subject string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	window := l.subjects[subject]
	if window.start.IsZero() || now.Sub(window.start) >= l.duration {
		window = attemptWindow{start: now}
	}
	if window.count >= l.maximum {
		return false
	}
	window.count++
	l.subjects[subject] = window
	return true
}
