package infra

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"

	"github.com/real-uangi/watchover-tg/internal/auth/domain"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
)

type Verifier struct{ password [32]byte }

func NewVerifier(cfg *config.Config) domain.Verifier {
	return &Verifier{password: sha256.Sum256([]byte(cfg.AdminPassword))}
}

func (v *Verifier) Verify(password string) bool {
	digest := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(digest[:], v.password[:]) == 1
}

type Sessions struct {
	mu       sync.Mutex
	sessions map[[32]byte]time.Time
	now      func() time.Time
}

func NewSessions() domain.Sessions {
	return &Sessions{sessions: make(map[[32]byte]time.Time), now: time.Now}
}

func (s *Sessions) Create(_ context.Context) (domain.Session, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return domain.Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	now := s.now().UTC()
	expires := now.Add(12 * time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Clean expired sessions when logging in instead of running a background task.
	for key, expiry := range s.sessions {
		if !now.Before(expiry) {
			delete(s.sessions, key)
		}
	}
	s.sessions[sha256.Sum256([]byte(token))] = expires
	return domain.Session{Token: token, ExpiresAt: expires}, nil
}

func (s *Sessions) Get(_ context.Context, token string) (domain.Session, error) {
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[key]
	if !ok || !s.now().Before(expiry) {
		delete(s.sessions, key)
		return domain.Session{}, domain.ErrUnauthorized
	}
	return domain.Session{Token: token, ExpiresAt: expiry}, nil
}

func (s *Sessions) Delete(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sha256.Sum256([]byte(token)))
	return nil
}
