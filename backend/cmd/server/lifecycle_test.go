package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleStub struct {
	begun      atomic.Bool
	shutdown   atomic.Bool
	waitForCtx bool
}

func (l *lifecycleStub) BeginShutdown() {
	l.begun.Store(true)
}

func (l *lifecycleStub) Shutdown(ctx context.Context) error {
	if !l.begun.Load() {
		return errors.New("Shutdown called before BeginShutdown")
	}
	l.shutdown.Store(true)
	if l.waitForCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func TestShutdownRuntimeStopsListenerAndLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	lifecycle := &lifecycleStub{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := shutdownRuntime(ctx, server, lifecycle); err != nil {
		t.Fatalf("shutdownRuntime() error = %v", err)
	}
	if !lifecycle.begun.Load() || !lifecycle.shutdown.Load() {
		t.Fatalf("lifecycle was not drained: begun=%v shutdown=%v", lifecycle.begun.Load(), lifecycle.shutdown.Load())
	}
	select {
	case err := <-serveResult:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP listener did not stop")
	}
}

func TestShutdownRuntimeHonorsSharedDeadline(t *testing.T) {
	server := &http.Server{}
	lifecycle := &lifecycleStub{waitForCtx: true}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := shutdownRuntime(ctx, server, lifecycle)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdownRuntime() error = %v, want deadline exceeded", err)
	}
	if !lifecycle.begun.Load() || !lifecycle.shutdown.Load() {
		t.Fatal("bounded shutdown did not invoke the service lifecycle")
	}
}

func TestDurationEnvRejectsInvalidShutdownTimeout(t *testing.T) {
	t.Setenv("CANVAS_SHUTDOWN_TIMEOUT", "not-a-duration")
	if _, err := durationEnv("CANVAS_SHUTDOWN_TIMEOUT", 30*time.Second); err == nil {
		t.Fatal("durationEnv() accepted an invalid duration")
	}
	t.Setenv("CANVAS_SHUTDOWN_TIMEOUT", "0s")
	if _, err := durationEnv("CANVAS_SHUTDOWN_TIMEOUT", 30*time.Second); err == nil {
		t.Fatal("durationEnv() accepted a non-positive duration")
	}
}
