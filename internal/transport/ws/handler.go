package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gsjaylab/liteterm/internal/credentials"
	"github.com/gsjaylab/liteterm/internal/terminal"
	webserver "github.com/gsjaylab/liteterm/internal/web"
)

type Handler struct {
	factory         terminal.Factory
	consume         func(string, string) bool
	maxBytes        int64
	idle            time.Duration
	maximum         time.Duration
	credentialStore credentials.Store
	active          map[string]struct{}
	mu              sync.Mutex
	stopping        bool
	sessions        sync.WaitGroup
	shutdown        chan struct{}
	stopOnce        sync.Once
}

func NewHandler(factory terminal.Factory, store credentials.Store, consume func(string, string) bool, maxBytes int64, idle, maximum time.Duration) *Handler {
	return &Handler{
		factory: factory, credentialStore: store, consume: consume, maxBytes: maxBytes, idle: idle, maximum: maximum,
		active: make(map[string]struct{}), shutdown: make(chan struct{}),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// token 必须与当前网关 UID 同时匹配；校验发生在创建 SSH Session 之前，避免
	// 未授权请求消耗进程、文件描述符和伪终端资源。
	subject, ok := webserver.Subject(r.Context())
	if !ok || !h.consume(r.URL.Query().Get("token"), subject) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !h.reserve(subject) {
		http.Error(w, "terminal capacity reached", http.StatusServiceUnavailable)
		return
	}
	defer h.release(subject)

	h.mu.Lock()
	if h.stopping {
		h.mu.Unlock()
		http.Error(w, "service shutting down", http.StatusServiceUnavailable)
		return
	}
	h.sessions.Add(1)
	h.mu.Unlock()
	defer h.sessions.Done()

	// fnOS 会保留外部 Origin、改写内部 Host；一次性 token 已承担 CSRF 边界，因此允许该可信代理跳转。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	conn.SetReadLimit(h.maxBytes)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	connect, err := h.readConnect(ctx, conn)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid connect message")
		return
	}
	credential, err := h.resolveCredential(subject, connect)
	if err != nil {
		_ = writeStatus(ctx, conn, "authentication_failed", "saved_credential_missing", "已保存的登录信息不存在，请重新输入密码")
		return
	}

	session, err := h.factory.Start(ctx, credential, connect.Cols, connect.Rows)
	if err != nil {
		_ = writeStatus(ctx, conn, "authentication_failed", "invalid_credentials", "用户名、密码或 SSH 端口不正确")
		_ = conn.Close(websocket.StatusPolicyViolation, "terminal authentication failed")
		return
	}
	if err := h.persistCredential(subject, credential, connect.Remember); err != nil {
		_ = session.Close()
		_ = conn.Close(websocket.StatusInternalError, "persist credential failed")
		return
	}
	if err := writeStatus(ctx, conn, "authenticated", "", ""); err != nil {
		_ = session.Close()
		return
	}
	(&bridge{conn: conn, session: session, idle: h.idle, maximum: h.maximum}).run(ctx, h.shutdown)
}

func (h *Handler) readConnect(ctx context.Context, conn *websocket.Conn) (connectMessage, error) {
	readCtx, readCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readCancel()
	typ, data, readErr := conn.Read(readCtx)
	if readErr != nil || typ != websocket.MessageText {
		return connectMessage{}, errors.New("connect message required")
	}
	connect, err := decodeConnect(data)
	if err != nil {
		return connectMessage{}, errors.New("invalid connect message")
	}
	return connect, nil
}

func (h *Handler) resolveCredential(subject string, connect connectMessage) (terminal.Credentials, error) {
	credential := terminal.Credentials{Port: connect.Port, Username: connect.Username, Password: connect.Password}
	if !connect.UseSavedCredential {
		return credential, nil
	}
	saved, err := h.credentialStore.Load(subject)
	if err != nil || saved.Port != connect.Port || saved.Username != connect.Username {
		return terminal.Credentials{}, credentials.ErrNotFound
	}
	credential.Password = saved.Password
	return credential, nil
}

func (h *Handler) persistCredential(subject string, credential terminal.Credentials, remember bool) error {
	if !remember {
		return h.credentialStore.Delete(subject)
	}
	return h.credentialStore.Save(subject, credentials.Credential{
		Port: credential.Port, Username: credential.Username, Password: credential.Password,
	})
}

func writeStatus(ctx context.Context, conn *websocket.Conn, kind, code, message string) error {
	payload, _ := json.Marshal(map[string]string{"type": kind, "code": code, "message": message})
	return conn.Write(ctx, websocket.MessageText, payload)
}

func (h *Handler) reserve(subject string) bool {
	const maxActiveSessions = 8
	// 每个 UID 只能占用一个会话，同时设置设备级上限，防止大量合法账号
	// 一起耗尽 SSH Session 和 WebSocket 文件描述符。
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping || len(h.active) >= maxActiveSessions {
		return false
	}
	if _, exists := h.active[subject]; exists {
		return false
	}
	h.active[subject] = struct{}{}
	return true
}

func (h *Handler) release(subject string) {
	h.mu.Lock()
	delete(h.active, subject)
	h.mu.Unlock()
}

// Shutdown 阻止新会话，并等待活动 SSH Session 清理。
func (h *Handler) Shutdown() {
	h.mu.Lock()
	h.stopping = true
	h.stopOnce.Do(func() { close(h.shutdown) })
	h.mu.Unlock()
	h.sessions.Wait()
}
