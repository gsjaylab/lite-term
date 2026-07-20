package app

import (
	"context"
	"testing"
	"time"
)

func TestShutdownCancelsAndWaitsForTerminalBeforeStoppingHTTP(t *testing.T) {
	canceled := make(chan struct{})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	httpStopped := make(chan struct{})
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- shutdown(func() { close(canceled) }, func() {
			select {
			case <-canceled:
			default:
				t.Error("terminal wait began before root cancellation")
			}
			close(cleanupStarted)
			<-releaseCleanup
		}, func(ctx context.Context) error {
			close(httpStopped)
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("HTTP shutdown has no deadline")
			} else if remaining := time.Until(deadline); remaining >= time.Second || deadline.Before(started) {
				t.Errorf("HTTP shutdown did not receive remaining budget: %v", remaining)
			}
			return nil
		}, time.Second)
	}()
	<-cleanupStarted
	select {
	case <-httpStopped:
		t.Fatal("HTTP shutdown started before terminal cleanup completed")
	case err := <-done:
		t.Fatalf("shutdown returned while terminal cleanup was active: %v", err)
	default:
	}
	time.Sleep(10 * time.Millisecond)
	close(releaseCleanup)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownSharesOriginalDeadlineWithHTTP(t *testing.T) {
	const budget = 80 * time.Millisecond
	started := time.Now()
	err := shutdown(func() {}, func() { time.Sleep(30 * time.Millisecond) }, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("HTTP shutdown has no deadline")
		}
		if elapsed := deadline.Sub(started); elapsed > budget+10*time.Millisecond {
			t.Fatalf("deadline budget restarted: %v", elapsed)
		}
		return nil
	}, budget)
	if err != nil {
		t.Fatal(err)
	}
}
