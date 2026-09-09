package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

type serviceLifecycle interface {
	BeginShutdown()
	Shutdown(context.Context) error
}

func serveUntilShutdown(server *http.Server, lifecycle serviceLifecycle, timeout time.Duration) error {
	if server == nil || lifecycle == nil {
		return errors.New("server lifecycle is not initialized")
	}
	if timeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.ListenAndServe()
	}()

	select {
	case err := <-serveResult:
		lifecycle.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		drainErr := lifecycle.Shutdown(ctx)
		if errors.Is(err, http.ErrServerClosed) {
			return drainErr
		}
		return errors.Join(fmt.Errorf("HTTP server stopped unexpectedly: %w", err), drainErr)
	case <-signalCtx.Done():
		log.Printf("shutdown signal received; stopping new requests and task claims")
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		shutdownErr := shutdownRuntime(ctx, server, lifecycle)
		select {
		case err := <-serveResult:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				shutdownErr = errors.Join(shutdownErr, err)
			}
		case <-ctx.Done():
			shutdownErr = errors.Join(shutdownErr, ctx.Err())
		}
		return shutdownErr
	}
}

// shutdownRuntime 在同一个期限内排空 HTTP 请求和已经领取的任务。必须先
// BeginShutdown，使就绪状态立即失效，并在入口摘流期间停止领取新任务。
func shutdownRuntime(ctx context.Context, server *http.Server, lifecycle serviceLifecycle) error {
	lifecycle.BeginShutdown()
	httpResult := make(chan error, 1)
	workerResult := make(chan error, 1)
	go func() { httpResult <- server.Shutdown(ctx) }()
	go func() { workerResult <- lifecycle.Shutdown(ctx) }()

	var shutdownErr error
	for completed := 0; completed < 2; {
		select {
		case err := <-httpResult:
			completed++
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown HTTP server: %w", err))
			}
			httpResult = nil
		case err := <-workerResult:
			completed++
			if err != nil {
				shutdownErr = errors.Join(shutdownErr, err)
			}
			workerResult = nil
		case <-ctx.Done():
			// Shutdown stops accepting immediately but can wait on long handlers.
			// Close is the final bounded fallback after the shared deadline.
			closeErr := server.Close()
			if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
				shutdownErr = errors.Join(shutdownErr, closeErr)
			}
			return errors.Join(shutdownErr, ctx.Err())
		}
	}
	return shutdownErr
}
