package ws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gsjaylab/liteterm/internal/credentials"
	"github.com/gsjaylab/liteterm/internal/terminal"
	webserver "github.com/gsjaylab/liteterm/internal/web"
)

type fakeFactory struct {
	mu      sync.Mutex
	session *fakeSession
	starts  int
}

type testHandler struct {
	*Handler
	authenticated http.Handler
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.authenticated.ServeHTTP(w, r)
}

func newTestHandler(factory terminal.Factory, consume func(string) bool, maxBytes int64, idle time.Duration) *testHandler {
	handler := NewHandler(factory, testCredentialStore{}, func(token, _ string) bool { return consume(token) }, maxBytes, idle, 4*time.Hour)
	return &testHandler{Handler: handler, authenticated: webserver.Authenticate(testAssertion{}, handler)}
}

type testAssertion struct{}

func (testAssertion) Assert(*http.Request) bool { return true }

type testCredentialStore struct{}

func (testCredentialStore) Load(string) (credentials.Credential, error) {
	return credentials.Credential{}, credentials.ErrNotFound
}
func (testCredentialStore) Save(string, credentials.Credential) error { return nil }
func (testCredentialStore) Delete(string) error                       { return nil }

func dialTerminal(ctx context.Context, url string, options *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	conn, response, err := websocket.Dial(ctx, url, options)
	if err != nil {
		return nil, response, err
	}
	connect := `{"type":"connect","port":22,"username":"admin","password":"secret","useSavedCredential":false,"remember":false,"cols":80,"rows":24}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(connect)); err != nil {
		conn.CloseNow()
		return nil, response, err
	}
	typ, data, err := conn.Read(ctx)
	if err != nil || typ != websocket.MessageText || !strings.Contains(string(data), `"type":"authenticated"`) {
		conn.CloseNow()
		if err == nil {
			err = fmt.Errorf("unexpected authentication response: %s", data)
		}
		return nil, response, err
	}
	return conn, response, nil
}

func (f *fakeFactory) Start(context.Context, terminal.Credentials, uint16, uint16) (terminal.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	return f.session, nil
}

func (f *fakeFactory) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.starts }

type fakeSession struct {
	mu          sync.Mutex
	input       bytes.Buffer
	size        [2]uint16
	output      chan []byte
	done        chan struct{}
	closed      chan struct{}
	closeOnce   sync.Once
	closeCount  int
	readErr     error
	resizeErr   error
	writeLimit  int
	writeErr    error
	writeCalls  int
	sizes       [][2]uint16
	beforeClose func()
}

func newFakeSession() *fakeSession {
	return &fakeSession{output: make(chan []byte, 4), done: make(chan struct{}), closed: make(chan struct{})}
}

func (s *fakeSession) Read(p []byte) (int, error) {
	select {
	case data := <-s.output:
		return copy(p, data), nil
	case <-s.done:
		if s.readErr != nil {
			return 0, s.readErr
		}
		return 0, io.EOF
	}
}
func (s *fakeSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeCalls++
	if s.writeErr != nil && s.writeCalls > 1 {
		return 0, s.writeErr
	}
	if s.writeLimit > 0 && len(p) > s.writeLimit {
		p = p[:s.writeLimit]
	}
	n, _ := s.input.Write(p)
	return n, nil
}
func (s *fakeSession) Resize(c, r uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = [2]uint16{c, r}
	s.sizes = append(s.sizes, s.size)
	return s.resizeErr
}
func (s *fakeSession) Wait() error { <-s.done; return nil }
func (s *fakeSession) Close() error {
	s.closeOnce.Do(func() {
		if s.beforeClose != nil {
			s.beforeClose()
		}
		s.mu.Lock()
		s.closeCount++
		s.mu.Unlock()
		select {
		case <-s.done:
		default:
			close(s.done)
		}
		close(s.closed)
	})
	return nil
}

func TestHandlerShutdownWaitsForActiveSessionCleanup(t *testing.T) {
	session := newFakeSession()
	closeStarted := make(chan struct{})
	releaseClose := make(chan struct{})
	session.beforeClose = func() {
		close(closeStarted)
		<-releaseClose
	}
	handler := newTestHandler(&fakeFactory{session: session}, func(string) bool { return true }, 64<<10, time.Minute)
	server := httptest.NewServer(handler)
	defer server.Close()
	conn, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	done := make(chan struct{})
	go func() { handler.Shutdown(); close(done) }()
	select {
	case <-closeStarted:
	case <-time.After(time.Second):
		t.Fatal("session cleanup did not start")
	}
	select {
	case <-done:
		t.Fatal("Shutdown returned while session cleanup was in progress")
	default:
	}
	close(releaseClose)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not return after cleanup completed")
	}
}
func (s *fakeSession) waitClosed(t *testing.T) {
	t.Helper()
	select {
	case <-s.closed:
	case <-time.After(time.Second):
		t.Fatal("session not closed")
	}
}

func wsURL(url string) string { return "ws" + strings.TrimPrefix(url, "http") }

func TestHandlerRejectsMissingTokenWithoutStartingPTY(t *testing.T) {
	factory := &fakeFactory{}
	server := httptest.NewServer(newTestHandler(factory, func(string) bool { return false }, 64<<10, time.Minute))
	defer server.Close()
	_, _, err := websocket.Dial(context.Background(), wsURL(server.URL), nil)
	if err == nil {
		t.Fatal("unauthenticated websocket accepted")
	}
	if factory.count() != 0 {
		t.Fatalf("starts=%d", factory.count())
	}
}

func TestHandlerAcceptsAuthenticatedGatewayOriginAfterHostRewrite(t *testing.T) {
	session := newFakeSession()
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: session}, func(token string) bool { return token == "once" }, 64<<10, time.Minute))
	defer server.Close()
	conn, response, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=once", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://ymbaby.fnos.net"}},
	})
	if err != nil {
		if response != nil {
			t.Fatalf("gateway websocket rejected with status %d: %v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	defer conn.CloseNow()
}

func TestHandlerForwardsInputResizeAndOutputThenCleansUp(t *testing.T) {
	session := newFakeSession()
	factory := &fakeFactory{session: session}
	server := httptest.NewServer(newTestHandler(factory, func(token string) bool { return token == "once" }, 64<<10, time.Minute))
	defer server.Close()
	conn, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=once", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(context.Background(), websocket.MessageBinary, []byte("pwd\n")); err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, []byte(`{"type":"resize","cols":100,"rows":40}`)); err != nil {
		t.Fatal(err)
	}
	session.output <- []byte("ready")
	typ, output, err := conn.Read(context.Background())
	if err != nil || typ != websocket.MessageBinary || string(output) != "ready" {
		t.Fatalf("output type=%v data=%q err=%v", typ, output, err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "done")
	session.waitClosed(t)
	session.mu.Lock()
	defer session.mu.Unlock()
	if got := session.input.String(); got != "pwd\n" {
		t.Fatalf("input=%q", got)
	}
	if session.size != [2]uint16{100, 40} {
		t.Fatalf("size=%v", session.size)
	}
	if session.closeCount != 1 {
		t.Fatalf("closes=%d", session.closeCount)
	}
}

func TestHandlerRejectsSecondSession(t *testing.T) {
	s1 := newFakeSession()
	f := &fakeFactory{session: s1}
	server := httptest.NewServer(newTestHandler(f, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c1, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=one", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.CloseNow()
	_, _, err = websocket.Dial(context.Background(), wsURL(server.URL)+"?token=two", nil)
	if err == nil {
		t.Fatal("second websocket accepted")
	}
	if f.count() != 1 {
		t.Fatalf("starts=%d", f.count())
	}
}

func TestHandlerReservesAtMostOneSessionPerSubject(t *testing.T) {
	handler := NewHandler(&fakeFactory{}, testCredentialStore{}, func(string, string) bool { return true }, 64<<10, time.Minute, time.Hour)
	if !handler.reserve("1000") {
		t.Fatal("first session was rejected")
	}
	if handler.reserve("1000") {
		t.Fatal("second session for the same subject was accepted")
	}
	if !handler.reserve("1001") {
		t.Fatal("different subject was rejected")
	}
	handler.release("1000")
	if !handler.reserve("1000") {
		t.Fatal("released subject could not reconnect")
	}
}

func TestHandlerRejectsMalformedResize(t *testing.T) {
	s := newFakeSession()
	f := &fakeFactory{session: s}
	server := httptest.NewServer(newTestHandler(f, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(context.Background(), websocket.MessageText, []byte(`{"type":"resize","cols":0,"rows":24}`)); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Read(context.Background())
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("status=%v err=%v", websocket.CloseStatus(err), err)
	}
	s.waitClosed(t)
}

func TestHandlerAcceptsResizeBoundaries(t *testing.T) {
	s := newFakeSession()
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	for _, message := range []string{
		`{"type":"resize","cols":2,"rows":1}`,
		`{"type":"resize","cols":300,"rows":150}`,
	} {
		if err := c.Write(context.Background(), websocket.MessageText, []byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		got := append([][2]uint16(nil), s.sizes...)
		s.mu.Unlock()
		if len(got) == 2 {
			if got[0] != [2]uint16{2, 1} || got[1] != [2]uint16{300, 150} {
				t.Fatalf("sizes=%v", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sizes=%v", got)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHandlerRejectsOutOfRangeResizeBeforeSession(t *testing.T) {
	for _, size := range [][2]uint16{{1, 24}, {301, 24}, {80, 151}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			s := newFakeSession()
			server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
			defer server.Close()
			c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
			if err != nil {
				t.Fatal(err)
			}
			message := fmt.Sprintf(`{"type":"resize","cols":%d,"rows":%d}`, size[0], size[1])
			if err := c.Write(context.Background(), websocket.MessageText, []byte(message)); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, _, err = c.Read(ctx)
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("status=%v err=%v", websocket.CloseStatus(err), err)
			}
			s.mu.Lock()
			calls := len(s.sizes)
			s.mu.Unlock()
			if calls != 0 {
				t.Fatalf("Resize calls=%d", calls)
			}
		})
	}
}

func TestHandlerReportsResizeFailureAsInternalError(t *testing.T) {
	s := newFakeSession()
	s.resizeErr = errors.New("resize failed")
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(context.Background(), websocket.MessageText, []byte(`{"type":"resize","cols":80,"rows":24}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusInternalError {
		t.Fatalf("status=%v err=%v", websocket.CloseStatus(err), err)
	}
}

func TestHandlerCompletesPartialSessionWrites(t *testing.T) {
	s := newFakeSession()
	s.writeLimit = 2
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err := c.Write(context.Background(), websocket.MessageBinary, []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		got, calls := s.input.String(), s.writeCalls
		s.mu.Unlock()
		if got == "abcdef" {
			if calls != 3 {
				t.Fatalf("write calls=%d", calls)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("input=%q calls=%d", got, calls)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHandlerReportsPartialSessionWriteFailureAsInternalError(t *testing.T) {
	s := newFakeSession()
	s.writeLimit = 2
	s.writeErr = errors.New("write failed")
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(context.Background(), websocket.MessageBinary, []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusInternalError {
		t.Fatalf("status=%v err=%v", websocket.CloseStatus(err), err)
	}
}

func TestHandlerRejectsResizeWithUnknownFields(t *testing.T) {
	s := newFakeSession()
	f := &fakeFactory{session: s}
	server := httptest.NewServer(newTestHandler(f, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(context.Background(), websocket.MessageText, []byte(`{"type":"resize","cols":80,"rows":24,"extra":true}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("status=%v err=%v", websocket.CloseStatus(err), err)
	}
}

func TestHandlerRejectsOversizedFrame(t *testing.T) {
	s := newFakeSession()
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(context.Background(), websocket.MessageBinary, bytes.Repeat([]byte{'x'}, (64<<10)+1)); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Read(context.Background())
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("status=%v err=%v", websocket.CloseStatus(err), err)
	}
	s.waitClosed(t)
}

func TestHandlerIdleTimeoutCleansUp(t *testing.T) {
	s := newFakeSession()
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, 25*time.Millisecond))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	s.waitClosed(t)
}

func TestHandlerReportsExit(t *testing.T) {
	s := newFakeSession()
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	close(s.done)
	typ, msg, err := c.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageText || string(msg) != `{"type":"exit","code":0}` {
		t.Fatalf("type=%v msg=%q", typ, msg)
	}
}

func TestHandlerReportsExitWhenPTYReadReturnsEIO(t *testing.T) {
	s := newFakeSession()
	s.readErr = syscall.EIO
	server := httptest.NewServer(newTestHandler(&fakeFactory{session: s}, func(string) bool { return true }, 64<<10, time.Minute))
	defer server.Close()
	c, _, err := dialTerminal(context.Background(), wsURL(server.URL)+"?token=x", nil)
	if err != nil {
		t.Fatal(err)
	}
	close(s.done)
	typ, msg, err := c.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageText || string(msg) != `{"type":"exit","code":0}` {
		t.Fatalf("type=%v msg=%q", typ, msg)
	}
}

var _ terminal.Factory = (*fakeFactory)(nil)
var _ terminal.Session = (*fakeSession)(nil)
