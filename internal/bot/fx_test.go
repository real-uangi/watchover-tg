package bot

import (
	"errors"
	"testing"

	"github.com/real-uangi/eventbus"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

type recordingBus struct {
	eventbus.EventBus
	topic string
	data  any
	err   error
}

func (b *recordingBus) Publish(topic string, data any) error {
	b.topic, b.data = topic, data
	return b.err
}

func TestPublisherSeparatesUpdatesAndReplies(t *testing.T) {
	replies, updates := &recordingBus{}, &recordingBus{}
	publisher := newPublisher(replies, updates)
	update := domain.Update{ID: 42, Message: &domain.TelegramMessage{Text: "credential"}}
	if err := publisher.PublishUpdate(update); err != nil {
		t.Fatal(err)
	}
	if updates.topic != domain.UpdateTopic || updates.data.(domain.Update).Message.Text != "credential" || replies.data != nil {
		t.Fatal("update was not queued on the update bus")
	}
	reply := domain.Reply{ChatID: 42, Text: "menu", Buttons: [][]domain.Button{{{Text: "verify", Data: "v:profile"}}}}
	if err := publisher.Publish(reply); err != nil {
		t.Fatal(err)
	}
	reply.Buttons[0][0].Text = "changed"
	queued := replies.data.(domain.Reply)
	if replies.topic != domain.ReplyTopic || queued.Buttons[0][0].Text != "verify" || updates.topic != domain.UpdateTopic {
		t.Fatal("reply routing or queued keyboard changed")
	}
	updates.err = errors.New("queue unavailable")
	if err := publisher.PublishUpdate(update); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unsafe queue error: %v", err)
	}
}
