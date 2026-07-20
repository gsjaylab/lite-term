package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/gsjaylab/liteterm/internal/auth"
	"github.com/gsjaylab/liteterm/internal/config"
	"github.com/gsjaylab/liteterm/internal/credentials"
	"github.com/gsjaylab/liteterm/internal/platform/fnos"
	"github.com/gsjaylab/liteterm/internal/terminal"
	transportws "github.com/gsjaylab/liteterm/internal/transport/ws"
	webserver "github.com/gsjaylab/liteterm/internal/web"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	credentialStore, err := credentials.NewFileStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open credential store: %w", err)
	}
	assertion, err := fnos.AssertionFor(cfg.Listen)
	if err != nil {
		return fmt.Errorf("configure assertion: %w", err)
	}

	root, cancel := context.WithCancel(ctx)
	defer cancel()
	tokens := auth.NewStore(time.Minute, time.Now)
	// 空闲超时处理遗忘关闭的页面，绝对时限则防止持续发送心跳的会话
	// 永久占用设备级终端配额。
	sshFactory := terminal.NewSSHFactory(8 * time.Second)
	terminalHandler := transportws.NewHandler(sshFactory, credentialStore, tokens.Consume, cfg.MaxMessageBytes, cfg.IdleTimeout, time.Hour)
	handler := newHTTPHandler(assertion, webserver.Assets(), tokens, terminalHandler, credentialStore, sshFactory)

	listener, cleanupListener, err := fnos.Listen(cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	defer cleanupListener()

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return root },
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()

	select {
	case err := <-errCh:
		cancel()
		terminalHandler.Shutdown()
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		if err := shutdown(cancel, terminalHandler.Shutdown, server.Shutdown, 5*time.Second); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}

func newHTTPHandler(assertion webserver.Assertion, assets fs.FS, tokens *auth.Store, terminalHandler http.Handler, store credentials.Store, tester webserver.CredentialTester) http.Handler {
	handler := webserver.NewServer(assets, tokens, terminalHandler, store, tester)
	// 安全头置于最外层，确保 token 认证失败也带有相同的浏览器安全策略。
	return webserver.SecurityHeaders(fnos.RouteGateway(fnos.AuthenticateAPI(assertion, handler)))
}
