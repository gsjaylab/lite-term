package web

import (
	"context"
	"net"
	"net/http"
	"strings"
)

const contentSecurityPolicy = "default-src 'self'; connect-src 'self' ws: wss:; style-src 'self' 'unsafe-inline'; img-src 'self' data:"

type subjectKey struct{}

type Assertion interface {
	Assert(*http.Request) bool
}

func Authenticate(assertion Assertion, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if assertion == nil || !assertion.Assert(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// 生产环境的 X-Trim-Userid 由 fnOS 统一网关注入，并已由 assertion
		// 验证；回环开发模式没有网关，因此使用固定的隔离主体。
		subject := r.Header.Get("X-Trim-Userid")
		if subject == "" {
			subject = "dev-loopback"
		}
		ctx := context.WithValue(r.Context(), subjectKey{}, subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Subject 只读取 Authenticate 写入的上下文值，业务代码不应直接信任
// 客户端请求头中的用户 ID。
func Subject(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(subjectKey{}).(string)
	return value, ok && value != ""
}

type DevLoopbackAssertion struct{}

func (DevLoopbackAssertion) Assert(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
