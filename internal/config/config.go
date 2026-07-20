package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

var ErrPublicListener = errors.New("LITETERM_LISTEN must use a loopback address")

type Config struct {
	Listen          string
	DataDir         string
	MaxMessageBytes int64
	IdleTimeout     time.Duration
}

func Load() (Config, error) {
	listen := os.Getenv("LITETERM_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:8189"
	}
	dataDir := os.Getenv("LITETERM_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(os.TempDir(), "liteterm-dev")
	}
	if filepath.IsAbs(listen) {
		// 绝对路径表示 fnOS 统一网关使用的 Unix Socket；其访问控制由
		// listener 权限和网关身份 Header 共同完成。
		return Config{
			Listen:          listen,
			DataDir:         dataDir,
			MaxMessageBytes: 64 << 10,
			IdleTimeout:     30 * time.Minute,
		}, nil
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return Config{}, err
	}
	// TCP 只用于显式开启的开发模式，禁止误配置为对局域网公开的终端服务。
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return Config{}, ErrPublicListener
	}
	return Config{
		Listen:          listen,
		DataDir:         dataDir,
		MaxMessageBytes: 64 << 10,
		IdleTimeout:     30 * time.Minute,
	}, nil
}
