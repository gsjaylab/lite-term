package terminal

import (
	"context"
	"errors"
	"io"
)

type Session interface {
	io.Reader
	io.Writer
	Resize(cols, rows uint16) error
	Wait() error
	Close() error
}

type Factory interface {
	Start(context.Context, Credentials, uint16, uint16) (Session, error)
}

type Credentials struct {
	Port     uint16
	Username string
	Password string
}

var ErrInvalidSize = errors.New("terminal size outside allowed range")
