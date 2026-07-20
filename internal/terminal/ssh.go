package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHFactory struct {
	timeout time.Duration
}

func NewSSHFactory(timeout time.Duration) *SSHFactory { return &SSHFactory{timeout: timeout} }

func (f *SSHFactory) Start(ctx context.Context, credentials Credentials, cols, rows uint16) (Session, error) {
	if credentials.Port == 0 || cols < 2 || cols > 300 || rows < 1 || rows > 150 {
		return nil, errors.New("invalid SSH session parameters")
	}
	client, err := f.connect(ctx, credentials)
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create SSH session: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	reader, writer := io.Pipe()
	safeWriter := &lockedWriter{writer: writer}
	session.Stdout = safeWriter
	session.Stderr = safeWriter
	if err := requestPTY(session, cols, rows); err != nil {
		session.Close()
		client.Close()
		reader.Close()
		writer.Close()
		return nil, fmt.Errorf("request SSH PTY: %w", err)
	}
	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		reader.Close()
		writer.Close()
		return nil, fmt.Errorf("start SSH shell: %w", err)
	}
	result := &sshSession{client: client, session: session, input: stdin, output: reader, outputWriter: writer, done: make(chan struct{})}
	go func() { result.waitErr = session.Wait(); writer.Close(); close(result.done) }()
	go func() {
		select {
		case <-ctx.Done():
			result.Close()
		case <-result.done:
		}
	}()
	return result, nil
}

func (f *SSHFactory) Test(ctx context.Context, credentials Credentials) error {
	client, err := f.connect(ctx, credentials)
	if err != nil {
		return err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()
	if err := requestPTY(session, 80, 24); err != nil {
		return fmt.Errorf("request SSH PTY: %w", err)
	}
	return nil
}

func (f *SSHFactory) connect(ctx context.Context, credentials Credentials) (*ssh.Client, error) {
	timeout := f.timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(credentials.Port)))
	connection, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect SSH: %w", err)
	}
	_ = connection.SetDeadline(time.Now().Add(timeout))
	// NewClientConn 没有 Context 参数；在调用方取消时主动关闭底层连接，
	// 让正在进行的 SSH 握手立即返回。
	stopCancel := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancel()
	password := credentials.Password
	// 目标地址由服务端固定为 loopback；这里沿用项目原有“不持久化
	// localhost host key”的策略，客户端输入无法把连接重定向到远端。
	config := &ssh.ClientConfig{User: credentials.Username, Auth: []ssh.AuthMethod{
		ssh.Password(password),
		ssh.KeyboardInteractive(func(_ string, _ string, questions []string, echoes []bool) ([]string, error) {
			if len(questions) != 1 || len(echoes) != 1 || echoes[0] {
				return nil, errors.New("unsupported keyboard-interactive challenge")
			}
			return []string{password}, nil
		}),
	}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: timeout}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("authenticate SSH: %w", err)
	}
	_ = connection.SetDeadline(time.Time{})
	return ssh.NewClient(clientConnection, channels, requests), nil
}

func requestPTY(session *ssh.Session, cols, rows uint16) error {
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 38400,
		ssh.TTY_OP_OSPEED: 38400,
	}
	return session.RequestPty("xterm-256color", int(rows), int(cols), modes)
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

type sshSession struct {
	client       *ssh.Client
	session      *ssh.Session
	input        io.WriteCloser
	output       *io.PipeReader
	outputWriter *io.PipeWriter
	done         chan struct{}
	waitErr      error
	closeOnce    sync.Once
	closeErr     error
}

func (s *sshSession) Read(data []byte) (int, error)  { return s.output.Read(data) }
func (s *sshSession) Write(data []byte) (int, error) { return s.input.Write(data) }
func (s *sshSession) Resize(cols, rows uint16) error {
	if cols < 2 || cols > 300 || rows < 1 || rows > 150 {
		return ErrInvalidSize
	}
	return s.session.WindowChange(int(rows), int(cols))
}
func (s *sshSession) Wait() error { <-s.done; return s.waitErr }
func (s *sshSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.input.Close()
		if err := s.session.Close(); err != nil && !errors.Is(err, io.EOF) {
			s.closeErr = err
		}
		_ = s.client.Close()
		_ = s.output.Close()
		_ = s.outputWriter.Close()
	})
	return s.closeErr
}
