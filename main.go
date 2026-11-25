package main

import (
	"github.com/gin-gonic/gin"
	"github.com/real-uangi/allingo/common"
	"github.com/real-uangi/allingo/common/app"
	"github.com/real-uangi/allingo/common/db"
	"github.com/real-uangi/allingo/common/log"
	"github.com/real-uangi/allingo/kv"
	"github.com/real-uangi/allingo/task"
	"github.com/real-uangi/eventbus"
	eventadapter "github.com/real-uangi/watchover-tg/adapter/event"
	httpadapter "github.com/real-uangi/watchover-tg/adapter/http"
	"github.com/real-uangi/watchover-tg/adapter/schedule"
	"github.com/real-uangi/watchover-tg/internal/auth"
	"github.com/real-uangi/watchover-tg/internal/bot"
	"github.com/real-uangi/watchover-tg/internal/bot/infra"
	"github.com/real-uangi/watchover-tg/internal/shared"
	"github.com/real-uangi/watchover-tg/web"
	"go.uber.org/fx"
	_ "time/tzdata"
)

var appOptions = []fx.Option{
	fx.WithLogger(log.NewFxLogger), shared.Module, db.Module, kv.Module, task.Module, auth.Module, bot.Module,
	fx.Provide(web.Assets), eventadapter.Module, schedule.Module,
	// Dependency order, rather than module listing, makes HTTP stop before
	// cleanup/event handlers and makes those stop before the database closes.
	fx.Decorate(func(engine *gin.Engine, _ eventbus.EventBus, _ *infra.Bootstrap, _ *schedule.Jobs, _ *eventadapter.Ready) *gin.Engine {
		return engine
	}),
	httpadapter.Module, common.Module,
}

func main() {
	for _, option := range appOptions {
		app.Current().Option(option)
	}
	app.Current().Run()
}
