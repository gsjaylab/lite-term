package config

import (
	"errors"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LITETERM_LISTEN", "")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Listen != "127.0.0.1:8189" {
		t.Fatalf("listen=%q", got.Listen)
	}
	if got.MaxMessageBytes != 64<<10 {
		t.Fatalf("max=%d", got.MaxMessageBytes)
	}
	if got.IdleTimeout != 30*time.Minute {
		t.Fatalf("idle=%s", got.IdleTimeout)
	}
}

func TestLoadRejectsPublicListener(t *testing.T) {
	t.Setenv("LITETERM_LISTEN", "0.0.0.0:8189")
	if _, err := Load(); !errors.Is(err, ErrPublicListener) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadAcceptsAbsoluteUnixSocket(t *testing.T) {
	t.Setenv("LITETERM_LISTEN", "/tmp/liteterm-test/app.sock")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Listen != "/tmp/liteterm-test/app.sock" {
		t.Fatalf("listen=%q", got.Listen)
	}
}

func TestLoadRejectsRelativeUnixSocket(t *testing.T) {
	t.Setenv("LITETERM_LISTEN", "app.sock")
	if _, err := Load(); err == nil {
		t.Fatal("relative socket path accepted")
	}
}
