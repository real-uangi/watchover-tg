package eventadapter

import (
	"context"
	"errors"
	"github.com/real-uangi/eventbus"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/bot/infra"
	"go.uber.org/fx"
	"time"
)

type Ready struct{}

var Module = fx.Module("event", fx.Provide(subscribe))

func subscribe(lc fx.Lifecycle, bus eventbus.EventBus, service *application.Service, _ *infra.Bootstrap) *Ready {
	bus.Subscribe(domain.ReplyTopic, func(data any) error {
		reply, ok := data.(domain.Reply)
		if !ok {
			return errors.New("invalid bot reply event")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return service.DeliverReply(ctx, reply)
	})
	lc.Append(fx.Hook{OnStop: bus.Close})
	return &Ready{}
}
