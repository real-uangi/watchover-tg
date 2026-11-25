package schedule

import (
	"context"
	"github.com/real-uangi/allingo/task"
	eventadapter "github.com/real-uangi/watchover-tg/adapter/event"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/infra"
	"go.uber.org/fx"
	"sync"
	"time"
)

var Module = fx.Module("schedule", fx.Provide(newJobs), fx.Invoke(register))

type cleaner interface{ Cleanup(context.Context) error }
type Jobs struct {
	service cleaner
	mu      sync.Mutex
	stopped bool
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	drained chan struct{}
}

func newJobs(service *application.Service, _ *infra.Bootstrap) *Jobs { return makeJobs(service) }
func makeJobs(service cleaner) *Jobs {
	ctx, cancel := context.WithCancel(context.Background())
	return &Jobs{service: service, ctx: ctx, cancel: cancel, drained: make(chan struct{})}
}
func register(lc fx.Lifecycle, manager *task.TaskManager, jobs *Jobs, _ *eventadapter.Ready) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			_, err := manager.Add("WatchoverMessageCleanup", "CRON_TZ=Asia/Shanghai 0 3 * * *", jobs.run)
			return err
		}, OnStop: jobs.stop,
	})
}
func (j *Jobs) run() error {
	j.mu.Lock()
	if j.stopped || j.running {
		j.mu.Unlock()
		return nil
	}
	j.running = true
	j.mu.Unlock()
	defer func() {
		j.mu.Lock()
		j.running = false
		if j.stopped {
			close(j.drained)
		}
		j.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(j.ctx, 5*time.Minute)
	defer cancel()
	return j.service.Cleanup(ctx)
}
func (j *Jobs) stop(ctx context.Context) error {
	j.mu.Lock()
	if !j.stopped {
		j.stopped = true
		j.cancel()
		if !j.running {
			close(j.drained)
		}
	}
	j.mu.Unlock()
	select {
	case <-j.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
