package fnos

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	webserver "github.com/gsjaylab/liteterm/internal/web"
)

const gatewayPrefix = "/app/liteterm"

// RouteGateway 只改写交给应用内部路由的副本，保留原请求供日志和上游诊断使用。
func RouteGateway(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == gatewayPrefix || strings.HasPrefix(r.URL.Path, gatewayPrefix+"/") {
			clone := r.Clone(r.Context())
			clone.URL.Path = strings.TrimPrefix(r.URL.Path, gatewayPrefix)
			if clone.URL.Path == "" {
				clone.URL.Path = "/"
			}
			next.ServeHTTP(w, clone)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func AssertionFor(listen string) (webserver.Assertion, error) {
	if filepath.IsAbs(listen) {
		// 生产流量只能经应用 Unix Socket 到达；仍需校验网关注入的 UID，
		// 不能仅凭“请求来自本机”就授予终端能力。
		return unixSocketAssertion{}, nil
	}
	if os.Getenv("LITETERM_DEV_AUTH") == "1" {
		return webserver.DevLoopbackAssertion{}, nil
	}
	return nil, errors.New("fnOS assertion adapter is not configured; set LITETERM_DEV_AUTH=1 only for loopback development")
}

type unixSocketAssertion struct{}

func (unixSocketAssertion) Assert(r *http.Request) bool {
	// fnOS 网关保证该 Header 来自已登录会话。此处再约束为正整数 UID，
	// 防止缺失值或异常代理请求进入应用授权链路。
	uid := r.Header.Get("X-Trim-Userid")
	value, err := strconv.ParseUint(uid, 10, 32)
	return err == nil && value > 0
}

// AuthenticateAPI 保护 token 和 WebSocket 两条 API 路由。静态资源仍由
// 外层 fnOS 网关控制，避免重复鉴权影响浏览器缓存与页面加载。
func AuthenticateAPI(assertion webserver.Assertion, next http.Handler) http.Handler {
	authenticated := webserver.Authenticate(assertion, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			authenticated.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
