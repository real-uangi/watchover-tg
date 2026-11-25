package infra

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/real-uangi/allingo/common/db"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"go.uber.org/fx"
)

type Bootstrap struct{}

func NewBootstrap(lc fx.Lifecycle, manager *db.Manager, store *Store) (*Bootstrap, error) {
	database := manager.GetDB()
	sqlDB, err := database.DB()
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) (startErr error) {
		defer func() {
			if startErr != nil {
				_ = sqlDB.Close()
			}
		}()
		if err := database.WithContext(ctx).AutoMigrate(&settingsRecord{}, &profileRecord{}, &chatRecord{}, &userRecord{}, &grantRecord{}, &memberRecord{}, &conversationRecord{}, &inviteRecord{}, &messageRecord{}, &receiptRecord{}); err != nil {
			return errors.New("bot schema initialization failed")
		}
		_, err := store.GetSettings(ctx)
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return domain.ErrUnavailable
		}
		return store.PutSettings(ctx, domain.Settings{Welcome: "欢迎使用 Watchover，请先验证身份后选择群组。", RetentionDays: 30, WebhookSecret: base64.RawURLEncoding.EncodeToString(secret)})
	}, OnStop: func(context.Context) error { return storageError(sqlDB.Close()) }})
	return &Bootstrap{}, nil
}
