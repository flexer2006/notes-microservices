package di

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func Wait(ctx context.Context, timeout time.Duration, hooks ...func(context.Context) error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var wgp sync.WaitGroup
	for _, hook := range hooks {
		wgp.Add(1)
		go func(fn func(context.Context) error) {
			defer wgp.Done()
			_ = fn(shutdownCtx)
		}(hook)
	}
	done := make(chan struct{})
	go func() {
		wgp.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
	}
}
