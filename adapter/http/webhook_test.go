package httpadapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

type webhookPublisher struct {
	domain.Publisher
	updates []domain.Update
	calls   int
	err     error
}

func (p *webhookPublisher) PublishUpdate(update domain.Update) error {
	p.calls++
	if p.err != nil {
		return p.err
	}
	p.updates = append(p.updates, update)
	return nil
}

func TestWebhookAcknowledgesQueuedUpdateBeforeProcessing(t *testing.T) {
	publisher := &webhookPublisher{}
	// The store only supports settings reads: processing has not begun when
	// the HTTP response is returned and the update is still in the fake queue.
	bot := application.NewService(&routeStore{}, &routeTelegram{}, nil, publisher, "https://watch.example")
	engine := routeEngineWithBot(t, bot)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook", strings.NewReader(`{"update_id":42,"message":{"message_id":7,"chat":{"id":9,"type":"private"},"text":"/start"}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Telegram-Bot-Api-Secret-Token", "webhook-secret")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("webhook response: %d %s", w.Code, w.Body)
	}
	if publisher.calls != 1 || len(publisher.updates) != 1 {
		t.Fatalf("publish calls=%d queued=%d", publisher.calls, len(publisher.updates))
	}
	update := publisher.updates[0]
	if update.ID != 42 || update.Message == nil || update.Message.MessageID != 7 || update.Message.Chat.ID != 9 || update.Message.Text != "/start" {
		t.Fatal("queued update does not match the webhook request")
	}
}

func TestWebhookRejectsInvalidRequestsBeforeQueueing(t *testing.T) {
	for _, tc := range []struct {
		name, body, secret string
		settingsErr        error
		publishErr         error
		code, calls        int
	}{
		{name: "invalid JSON", body: `{"update_id":`, secret: "webhook-secret", code: http.StatusBadRequest},
		{name: "oversized JSON", body: `{"update_id":42,"message":{"text":"` + strings.Repeat("x", 1<<20) + `"}}`, secret: "webhook-secret", code: http.StatusRequestEntityTooLarge},
		{name: "missing secret", body: `{"update_id":42}`, code: http.StatusForbidden},
		{name: "wrong secret", body: `{"update_id":42}`, secret: "wrong-secret", code: http.StatusForbidden},
		{name: "settings unavailable", body: `{"update_id":42}`, secret: "webhook-secret", settingsErr: domain.ErrUnavailable, code: http.StatusServiceUnavailable},
		{name: "queue unavailable", body: `{"update_id":42}`, secret: "webhook-secret", publishErr: domain.ErrUnavailable, code: http.StatusServiceUnavailable, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			publisher := &webhookPublisher{err: tc.publishErr}
			bot := application.NewService(&routeStore{settingsErr: tc.settingsErr}, &routeTelegram{}, nil, publisher, "https://watch.example")
			engine := routeEngineWithBot(t, bot)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Telegram-Bot-Api-Secret-Token", tc.secret)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("status=%d want=%d: %s", w.Code, tc.code, w.Body)
			}
			if publisher.calls != tc.calls || len(publisher.updates) != 0 {
				t.Fatalf("publish calls=%d queued=%d", publisher.calls, len(publisher.updates))
			}
			var envelope struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Code != w.Code {
				t.Fatalf("HTTP/code mismatch: %s", w.Body)
			}
		})
	}
}
