package fnos

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenCreatesRestrictiveUnixSocketAndCleansUp(t *testing.T) {
	dir := shortTempDir(t)
	path := filepath.Join(dir, "app.sock")
	listener, cleanup, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode=%o", got)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket remains after cleanup: %v", err)
	}
}

func TestListenReplacesStaleUnixSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "app.sock")
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}

	listener, cleanup, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Cleanup(func() { _ = cleanup() })
}

func TestListenRefusesToReplaceRegularFile(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "app.sock")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Listen(path); err == nil {
		t.Fatal("Listen accepted a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("regular file changed: data=%q err=%v", data, err)
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "liteterm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestListenSupportsLoopbackTCP(t *testing.T) {
	listener, cleanup, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}
