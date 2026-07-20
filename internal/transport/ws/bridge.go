package ws

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/gsjaylab/liteterm/internal/terminal"
)

type bridge struct {
	conn    *websocket.Conn
	session terminal.Session
	idle    time.Duration
	maximum time.Duration
}

type outcome struct {
	status websocket.StatusCode
	reason string
	exited bool
}

func (b *bridge) run(parent context.Context, shutdown <-chan struct{}) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var closeSession sync.Once
	cleanup := func() { closeSession.Do(func() { _ = b.session.Close() }) }
	defer cleanup()

	results := make(chan outcome, 2)
	activity := make(chan struct{}, 1)
	touch := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}

	go b.pumpOutput(ctx, results, touch)
	go b.pumpInput(ctx, results, touch)

	timer := time.NewTimer(b.idle)
	defer timer.Stop()
	// deadline 不随输入输出活动重置，因此持续活跃的客户端也必须在达到
	// 绝对时限后释放 SSH Session。
	deadline := time.NewTimer(b.maximum)
	defer deadline.Stop()
	for {
		select {
		case result := <-results:
			// 先取消另一条泵并回收 SSH Session，再发送最终关闭帧，保证清理只有一个所有者。
			cancel()
			cleanup()
			b.close(result)
			return
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(b.idle)
		case <-timer.C:
			cancel()
			cleanup()
			_ = b.conn.Close(websocket.StatusPolicyViolation, "idle timeout")
			return
		case <-deadline.C:
			cancel()
			cleanup()
			_ = b.conn.Close(websocket.StatusPolicyViolation, "maximum session duration reached")
			return
		case <-shutdown:
			cancel()
			cleanup()
			b.conn.CloseNow()
			return
		}
	}
}

func (b *bridge) pumpOutput(ctx context.Context, results chan<- outcome, touch func()) {
	buf := make([]byte, 32<<10)
	for {
		n, readErr := b.session.Read(buf)
		if n > 0 {
			if err := b.conn.Write(ctx, websocket.MessageBinary, buf[:n]); err != nil {
				results <- outcome{}
				return
			}
			touch()
		}
		if readErr != nil {
			// 另一条泵或超时路径已经决定关闭原因时，Close 会让 Session Read
			// 返回 EOF。此时不能再发送 exit 消息，否则它会抢在真正的
			// WebSocket 关闭帧前到达客户端，造成关闭语义不稳定。
			if ctx.Err() != nil {
				return
			}
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, syscall.EIO) {
				results <- outcome{status: websocket.StatusInternalError, reason: "terminal read failed"}
				return
			}
			payload, _ := json.Marshal(exitMessage{Type: "exit", Code: exitCode(b.session.Wait())})
			if err := b.conn.Write(ctx, websocket.MessageText, payload); err != nil {
				results <- outcome{}
				return
			}
			results <- outcome{exited: true}
			return
		}
	}
}

func (b *bridge) pumpInput(ctx context.Context, results chan<- outcome, touch func()) {
	for {
		typ, data, err := b.conn.Read(ctx)
		if err != nil {
			status := websocket.CloseStatus(err)
			if strings.Contains(err.Error(), "read limited at") {
				status = websocket.StatusMessageTooBig
			}
			if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				status = -1
			}
			results <- outcome{status: status}
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if err := writeAll(b.session, data); err != nil {
				results <- outcome{status: websocket.StatusInternalError, reason: "terminal write failed"}
				return
			}
		case websocket.MessageText:
			resize, err := decodeResize(data)
			if err != nil {
				results <- outcome{status: websocket.StatusPolicyViolation, reason: "invalid resize"}
				return
			}
			if err := b.session.Resize(resize.Cols, resize.Rows); err != nil {
				results <- outcome{status: websocket.StatusInternalError, reason: "terminal resize failed"}
				return
			}
		default:
			results <- outcome{status: websocket.StatusPolicyViolation, reason: "invalid message type"}
			return
		}
		touch()
	}
}

func (b *bridge) close(result outcome) {
	if result.exited {
		_ = b.conn.Close(websocket.StatusNormalClosure, "terminal exited")
	} else if result.status >= 1000 {
		_ = b.conn.Close(result.status, result.reason)
	} else {
		b.conn.CloseNow()
	}
}
