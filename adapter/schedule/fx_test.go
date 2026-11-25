package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type waitingCleaner struct {
	started chan struct{}
	calls   atomic.Int32
}

func (c *waitingCleaner) Cleanup(ctx context.Context) error {
	c.calls.Add(1)
	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}
func TestStopCancelsAndWaitsAndRejectsLateCallbacks(t *testing.T) {
	cleaner := &waitingCleaner{started: make(chan struct{})}
	jobs := makeJobs(cleaner)
	done := make(chan error, 1)
	go func() { done <- jobs.run() }()
	select {
	case <-cleaner.started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	// A second callback does not create overlapping work.
	if err := jobs.run(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := jobs.stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("cleanup not drained")
	}
	if err := jobs.run(); err != nil {
		t.Fatal(err)
	}
	if cleaner.calls.Load() != 1 {
		t.Fatal("late callback reached database")
	}
	if err := jobs.stop(ctx); err != nil {
		t.Fatal(err)
	}
}
