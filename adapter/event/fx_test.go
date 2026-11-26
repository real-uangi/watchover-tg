package eventadapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/real-uangi/eventbus"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/bot/infra"
	"go.uber.org/fx"
)

type queuedEvent struct {
	topic string
	data  any
}

type testBus struct {
	eventbus.EventBus
	handlers map[string]eventbus.SubscribeHandler
	queued   []queuedEvent
	closed   bool
	onClose  func() error
}

func newTestBus() *testBus { return &testBus{handlers: make(map[string]eventbus.SubscribeHandler)} }
func (b *testBus) Subscribe(topic string, handler eventbus.SubscribeHandler) {
	b.handlers[topic] = handler
}
func (b *testBus) Publish(topic string, data any) error {
	if b.closed {
		return eventbus.ErrClosed
	}
	b.queued = append(b.queued, queuedEvent{topic: topic, data: data})
	return nil
}
func (b *testBus) Close(context.Context) error {
	b.closed = true
	if b.onClose != nil {
		return b.onClose()
	}
	return nil
}

type testLifecycle struct{ hooks []fx.Hook }

func (l *testLifecycle) Append(hook fx.Hook) { l.hooks = append(l.hooks, hook) }

type testStore struct {
	domain.Store
	onReceipt func(context.Context, int64) (bool, error)
	onMessage func(context.Context, domain.Message) error
	completed int64
}

func (*testStore) GetSettings(context.Context) (domain.Settings, error) {
	return domain.Settings{Token: "token", WebhookSecret: "secret"}, nil
}
func (s *testStore) HasReceipt(ctx context.Context, id int64) (bool, error) {
	return s.onReceipt(ctx, id)
}
func (s *testStore) Transaction(_ context.Context, f func(domain.Store) error) error { return f(s) }
func (s *testStore) PutReceipt(_ context.Context, id int64, _ time.Time) error {
	s.completed = id
	return nil
}
func (s *testStore) PutMessage(ctx context.Context, message domain.Message) error {
	return s.onMessage(ctx, message)
}

type testPublisher struct{ updates, replies *testBus }

func (p testPublisher) PublishUpdate(update domain.Update) error {
	return p.updates.Publish(domain.UpdateTopic, update)
}
func (p testPublisher) Publish(reply domain.Reply) error {
	return p.replies.Publish(domain.ReplyTopic, reply)
}

type testTelegram struct {
	domain.Telegram
	onReply func(context.Context, domain.Reply)
}

func (tg testTelegram) Reply(ctx context.Context, _ string, reply domain.Reply) (domain.TelegramMessage, error) {
	tg.onReply(ctx, reply)
	return domain.TelegramMessage{MessageID: 7, Date: time.Now().Unix()}, nil
}

func checkDeadline(t *testing.T, ctx context.Context, duration time.Duration) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) < duration-time.Second || time.Until(deadline) > duration || ctx.Err() != nil {
		t.Fatalf("unexpected worker context: deadline=%v error=%v", deadline, ctx.Err())
	}
}

func TestUpdateContinuesAfterWebhookContextEnds(t *testing.T) {
	replies, updates := newTestBus(), newTestBus()
	store := &testStore{onReceipt: func(ctx context.Context, id int64) (bool, error) {
		checkDeadline(t, ctx, 60*time.Second)
		if id != 42 {
			t.Fatalf("unexpected update: %d", id)
		}
		return false, nil
	}}
	service := application.NewService(store, nil, nil, testPublisher{updates: updates, replies: replies}, "")
	subscribe(&testLifecycle{}, replies, updates, service, &infra.Bootstrap{})
	requestCtx, cancel := context.WithCancel(context.Background())
	if err := service.ReceiveUpdate(requestCtx, "secret", domain.Update{ID: 42}); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if len(updates.queued) != 1 {
		t.Fatalf("queued updates: %d", len(updates.queued))
	}
	queued := updates.queued[0]
	if err := updates.handlers[queued.topic](queued.data); err != nil {
		t.Fatal(err)
	}
	if store.completed != 42 {
		t.Fatal("update did not complete after request cancellation")
	}
}

func TestUpdateFailureDoesNotExposePayload(t *testing.T) {
	replies, updates := newTestBus(), newTestBus()
	store := &testStore{onReceipt: func(context.Context, int64) (bool, error) {
		return false, domain.ErrUnavailable
	}}
	service := application.NewService(store, nil, nil, nil, "")
	subscribe(&testLifecycle{}, replies, updates, service, &infra.Bootstrap{})
	err := updates.handlers[domain.UpdateTopic](domain.Update{ID: 42, Message: &domain.TelegramMessage{Text: "private-credential"}})
	if !errors.Is(err, domain.ErrUnavailable) || !strings.Contains(err.Error(), "42") || strings.Contains(err.Error(), "private-credential") {
		t.Fatalf("unexpected failure: %v", err)
	}
}

func TestReplyKeepsIndependentDeadline(t *testing.T) {
	replies, updates := newTestBus(), newTestBus()
	store := &testStore{onMessage: func(ctx context.Context, message domain.Message) error {
		checkDeadline(t, ctx, 10*time.Second)
		if message.Text != "welcome" || message.TelegramMessageID != 7 {
			t.Fatalf("unexpected stored reply: %+v", message)
		}
		return nil
	}}
	tg := testTelegram{onReply: func(ctx context.Context, reply domain.Reply) {
		checkDeadline(t, ctx, 10*time.Second)
		if reply.ChatID != 42 {
			t.Fatalf("unexpected recipient: %d", reply.ChatID)
		}
	}}
	service := application.NewService(store, tg, nil, nil, "")
	subscribe(&testLifecycle{}, replies, updates, service, &infra.Bootstrap{})
	if err := replies.handlers[domain.ReplyTopic](domain.Reply{ChatID: 42, Text: "welcome"}); err != nil {
		t.Fatal(err)
	}
}

func TestStopDrainsUpdatesBeforeClosingReplyBus(t *testing.T) {
	replies, updates := newTestBus(), newTestBus()
	lifecycle := &testLifecycle{}
	subscribe(lifecycle, replies, updates, nil, &infra.Bootstrap{})
	updates.onClose = func() error {
		return replies.Publish(domain.ReplyTopic, domain.Reply{ChatID: 42, Text: "last reply"})
	}
	// Fx executes stop hooks in reverse registration order.
	for i := len(lifecycle.hooks) - 1; i >= 0; i-- {
		if err := lifecycle.hooks[i].OnStop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if !updates.closed || !replies.closed || len(replies.queued) != 1 {
		t.Fatal("queued update could not publish its reply before shutdown")
	}
}
