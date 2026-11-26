package eventadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/real-uangi/eventbus"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/bot/infra"
	"go.uber.org/fx"
)

type Ready struct{}

var Module = fx.Module("event", fx.Provide(
	fx.Annotate(subscribe, fx.ParamTags("", "", `name:"updates"`, "", "")),
))

func subscribe(lc fx.Lifecycle, bus, updates eventbus.EventBus, service *application.Service, _ *infra.Bootstrap) *Ready {
	updates.Subscribe(domain.UpdateTopic, func(data any) error {
		update, ok := data.(domain.Update)
		if !ok {
			return errors.New("invalid bot update event")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := service.HandleUpdate(ctx, update); err != nil {
			return fmt.Errorf("update %d processing failed: %w", update.ID, err)
		}
		return nil
	})
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
	lc.Append(fx.Hook{OnStop: updates.Close})
	return &Ready{}
}
