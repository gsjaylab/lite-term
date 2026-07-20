package ws

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/gsjaylab/liteterm/internal/terminal"
	"golang.org/x/crypto/ssh"
)

type resizeMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type exitMessage struct {
	Type string `json:"type"`
	Code int    `json:"code"`
}

type connectMessage struct {
	Type               string `json:"type"`
	Port               uint16 `json:"port"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	UseSavedCredential bool   `json:"useSavedCredential"`
	Remember           bool   `json:"remember"`
	Cols               uint16 `json:"cols"`
	Rows               uint16 `json:"rows"`
}

func decodeConnect(data []byte) (connectMessage, error) {
	var message connectMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		return message, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF || message.Type != "connect" || message.Port == 0 || !terminal.ValidUsername(message.Username) || len(message.Password) > 1024 || message.Cols < 2 || message.Cols > 300 || message.Rows < 1 || message.Rows > 150 {
		return message, errors.New("invalid connect message")
	}
	if message.UseSavedCredential && message.Password != "" {
		return message, errors.New("saved credentials cannot include a password")
	}
	if !message.UseSavedCredential && message.Password == "" {
		return message, errors.New("password required")
	}
	return message, nil
}

func decodeResize(data []byte) (resizeMessage, error) {
	var resize resizeMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	// 拒绝未知字段和同一帧中的第二个 JSON 值，使客户端与服务端对协议
	// 边界保持一致，也避免未来新增字段被旧版本静默误解。
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resize); err != nil {
		return resize, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF || resize.Type != "resize" || resize.Cols < 2 || resize.Cols > 300 || resize.Rows < 1 || resize.Rows > 150 {
		return resize, errors.New("invalid resize")
	}
	return resize, nil
}

func writeAll(w io.Writer, data []byte) error {
	// Session 写入允许合法的短写；必须循环到整帧完成，0 字节写入则视为
	// io.Writer 契约异常，避免无限自旋。
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus()
	}
	return 1
}
