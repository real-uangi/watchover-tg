package shared

import (
	"github.com/real-uangi/allingo/common/log"
	"github.com/real-uangi/eventbus"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
	"go.uber.org/fx"
)

var Module = fx.Module("shared", fx.Provide(config.Load, newBus))

func newBus() eventbus.EventBus {
	eventbus.WithLogger(log.For[eventbus.EventBus]())
	bus := eventbus.New(&eventbus.Config{Dispatchers: 1, DispatcherQueueSize: 64, Executors: 2, ExecutorQueueSize: 64})
	return bus
}
