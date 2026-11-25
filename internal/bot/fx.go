package bot

import (
	"github.com/real-uangi/eventbus"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/bot/infra"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
	"go.uber.org/fx"
)

var Module = fx.Module("bot", fx.Provide(
	infra.NewBootstrap, infra.NewCipher,
	infra.NewStore, func(store *infra.Store) domain.Store { return store }, infra.NewTelegram, infra.NewVerifier,
	newPublisher, newService,
))

type publisher struct{ bus eventbus.EventBus }

func newPublisher(bus eventbus.EventBus) domain.Publisher { return &publisher{bus: bus} }
func (p *publisher) Publish(r domain.Reply) error {
	// Copy the keyboard so the queued payload is immutable after Publish.
	buttons := make([][]domain.Button, len(r.Buttons))
	for i := range r.Buttons {
		buttons[i] = append([]domain.Button(nil), r.Buttons[i]...)
	}
	r.Buttons = buttons
	if err := p.bus.Publish(domain.ReplyTopic, r); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func newService(store domain.Store, tg domain.Telegram, verifier domain.Verifier, pub domain.Publisher, cfg *config.Config) *application.Service {
	return application.NewService(store, tg, verifier, pub, cfg.Origin)
}
