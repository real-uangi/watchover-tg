package domain

import (
	"context"
	"errors"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

type Session struct {
	Token     string
	ExpiresAt time.Time
}

type Verifier interface {
	Verify(password string) bool
}

type Sessions interface {
	Create(context.Context) (Session, error)
	Get(context.Context, string) (Session, error)
	Delete(context.Context, string) error
}
