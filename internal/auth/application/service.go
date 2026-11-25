package application

import (
	"context"

	"github.com/real-uangi/watchover-tg/internal/auth/domain"
)

type Service struct {
	verifier domain.Verifier
	sessions domain.Sessions
}

func NewService(verifier domain.Verifier, sessions domain.Sessions) *Service {
	return &Service{verifier: verifier, sessions: sessions}
}

func (s *Service) Login(ctx context.Context, password string) (domain.Session, error) {
	if !s.verifier.Verify(password) {
		return domain.Session{}, domain.ErrUnauthorized
	}
	return s.sessions.Create(ctx)
}

func (s *Service) Session(ctx context.Context, token string) (domain.Session, error) {
	return s.sessions.Get(ctx, token)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.sessions.Delete(ctx, token)
}
