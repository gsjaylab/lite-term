package fnos

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// Listen 根据 fnOS 配置创建开发用 TCP 或生产用 Unix Socket 监听器。
func Listen(address string) (net.Listener, func() error, error) {
	if !filepath.IsAbs(address) {
		listener, err := net.Listen("tcp", address)
		return listener, func() error { return nil }, err
	}
	if info, err := os.Lstat(address); err == nil {
		// 只删除上次异常退出遗留的 Socket。若路径已被替换成普通文件或
		// 软链则拒绝启动，避免覆盖应用包内的其他数据。
		if info.Mode()&os.ModeSocket == 0 {
			return nil, nil, fmt.Errorf("refusing to replace non-socket path %s", address)
		}
		if err := os.Remove(address); err != nil {
			return nil, nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	listener, err := net.Listen("unix", address)
	if err != nil {
		return nil, nil, err
	}
	if err := os.Chmod(address, 0o600); err != nil {
		// 权限收紧失败时不能带着宽松的 umask 默认权限继续提供终端服务。
		_ = listener.Close()
		_ = os.Remove(address)
		return nil, nil, fmt.Errorf("restrict socket permissions: %w", err)
	}
	cleanup := func() error {
		err := os.Remove(address)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return listener, cleanup, nil
}
