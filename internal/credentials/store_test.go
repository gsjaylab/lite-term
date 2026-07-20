package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreSeparatesSubjectsAndPersistsCredential(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	want := Credential{Port: 2222, Username: "admin", Password: "secret"}
	if err := store.Save("1000", want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("1000")
	if err != nil || got != want {
		t.Fatalf("credential=%+v err=%v", got, err)
	}
	if _, err := store.Load("1001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other subject err=%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "credentials"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestFileStoreOverwritesAndDeletes(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("1000", Credential{Port: 22, Username: "one", Password: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("1000", Credential{Port: 22, Username: "two", Password: "second"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("1000")
	if err != nil || got.Username != "two" || got.Password != "second" {
		t.Fatalf("credential=%+v err=%v", got, err)
	}
	if err := store.Delete("1000"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("1000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	if err := store.Delete("1000"); err != nil {
		t.Fatal(err)
	}
}
