package auth

import (
	"github.com/real-uangi/watchover-tg/internal/auth/application"
	"github.com/real-uangi/watchover-tg/internal/auth/infra"
	"go.uber.org/fx"
)

var Module = fx.Module("auth", fx.Provide(infra.NewVerifier, infra.NewSessions, application.NewService))
