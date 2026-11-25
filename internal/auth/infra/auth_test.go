package infra

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/real-uangi/watchover-tg/internal/auth/application"
	"github.com/real-uangi/watchover-tg/internal/auth/domain"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
)

func TestLoginExpiryAndRevocation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	store := NewSessions().(*Sessions)
	store.now = func() time.Time { return now }
	service := application.NewService(NewVerifier(&config.Config{AdminPassword: "correct-password"}), store)
	if _, err := service.Login(ctx, "wrong-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	session, err := service.Login(ctx, "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || !session.ExpiresAt.Equal(now.Add(12*time.Hour)) {
		t.Fatalf("invalid session expiry: %v", session.ExpiresAt)
	}
	now = now.Add(11 * time.Hour)
	current, err := service.Session(ctx, session.Token)
	if err != nil || !current.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatalf("lookup must not extend expiry: %v", err)
	}
	now = now.Add(time.Hour)
	if _, err := service.Session(ctx, session.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("session remains valid at expiry: %v", err)
	}
	second, err := service.Login(ctx, "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == session.Token {
		t.Fatal("session token reused")
	}
	if err := service.Logout(ctx, second.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Session(ctx, second.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked session: %v", err)
	}
	if _, err := NewSessions().Get(ctx, session.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("session survived a new store: %v", err)
	}
}

func TestConcurrentSessionUse(t *testing.T) {
	store := NewSessions()
	ctx := context.Background()
	var workers sync.WaitGroup
	for range 24 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			session, err := store.Create(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := store.Get(ctx, session.Token); err != nil {
				t.Error(err)
			}
			if err := store.Delete(ctx, session.Token); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
}
