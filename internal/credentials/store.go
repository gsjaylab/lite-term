package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrNotFound = errors.New("saved credential not found")

type Credential struct {
	Port     uint16 `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Store interface {
	Load(subject string) (Credential, error)
	Save(subject string, credential Credential) error
	Delete(subject string) error
}

type FileStore struct {
	dir string
	mu  sync.RWMutex
}

func NewFileStore(dir string) (*FileStore, error) {
	// fnOS 的 TRIM_PKGVAR 由应用账号独占；仍显式收紧权限，防止凭据
	// 因系统 umask 或迁移后的旧目录权限而被其他本机用户读取。
	dir = filepath.Join(dir, "credentials")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) path(subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}

func (s *FileStore) Load(subject string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path(subject))
	if errors.Is(err, os.ErrNotExist) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	var credential Credential
	if err := json.Unmarshal(data, &credential); err != nil {
		return Credential{}, fmt.Errorf("decode saved credential: %w", err)
	}
	return credential, nil
}

func (s *FileStore) Save(subject string, credential Credential) error {
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 同目录临时文件加 Rename 保证进程崩溃时旧记录仍然完整。
	temporary, err := os.CreateTemp(s.dir, ".credential-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path(subject))
}

func (s *FileStore) Delete(subject string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(subject))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
