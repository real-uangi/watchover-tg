package shared

import (
	"context"
	"testing"
	"time"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

func TestBlockedUpdateDoesNotBlockReplies(t *testing.T) {
	replies := newBus()
	updates := newUpdateBus(replies)
	release, started, delivered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := updates.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := replies.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	updates.Subscribe(domain.UpdateTopic, func(any) error {
		close(started)
		<-release
		return nil
	})
	replies.Subscribe(domain.ReplyTopic, func(any) error {
		close(delivered)
		return nil
	})
	if err := updates.Publish(domain.UpdateTopic, domain.Update{ID: 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("update worker did not start")
	}
	if err := replies.Publish(domain.ReplyTopic, domain.Reply{ChatID: 1, Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("blocked update prevented reply delivery")
	}
}
