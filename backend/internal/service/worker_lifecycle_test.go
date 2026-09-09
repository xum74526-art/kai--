package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
)

func TestWorkerLifecycleStopsClaimsAtShutdownBoundary(t *testing.T) {
	lifecycle := newTaskWorkerLifecycle()
	if !lifecycle.start() {
		t.Fatal("worker lifecycle did not start")
	}
	lifecycle.beginShutdown()
	var called atomic.Bool
	_, _, _, err := lifecycle.claimTask(func() (*model.Task, error) {
		called.Store(true)
		return &model.Task{ID: "task-after-shutdown"}, nil
	})
	if !errors.Is(err, errWorkerShuttingDown) {
		t.Fatalf("claimTask() error = %v, want errWorkerShuttingDown", err)
	}
	if called.Load() {
		t.Fatal("repository claim ran after the shutdown boundary")
	}
}

func TestWorkerLifecycleMarksNotReadyBeforeBlockedClaimReturns(t *testing.T) {
	lifecycle := newTaskWorkerLifecycle()
	if !lifecycle.start() {
		t.Fatal("worker lifecycle did not start")
	}
	claimStarted := make(chan struct{})
	releaseClaim := make(chan struct{})
	claimResult := make(chan error, 1)
	go func() {
		_, _, _, err := lifecycle.claimTask(func() (*model.Task, error) {
			close(claimStarted)
			<-releaseClaim
			return &model.Task{ID: "task-claimed-during-shutdown"}, nil
		})
		claimResult <- err
	}()
	<-claimStarted

	shutdownMarked := make(chan struct{})
	go func() {
		lifecycle.beginShutdown()
		close(shutdownMarked)
	}()
	select {
	case <-shutdownMarked:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("beginShutdown waited for a blocked database claim")
	}
	if lifecycle.isReady() {
		t.Fatal("lifecycle remained ready after shutdown began")
	}
	close(releaseClaim)
	if err := <-claimResult; !errors.Is(err, errWorkerShuttingDown) {
		t.Fatalf("claimTask() error = %v, want errWorkerShuttingDown", err)
	}
}

func TestWorkerLifecycleDrainsCompletedTaskWithoutCancel(t *testing.T) {
	lifecycle := newTaskWorkerLifecycle()
	if !lifecycle.start() {
		t.Fatal("worker lifecycle did not start")
	}
	_, taskCtx, done, err := lifecycle.claimTask(func() (*model.Task, error) {
		return &model.Task{ID: "task-drained"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	done()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.shutdown(ctx); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	if taskCtx.Err() != nil {
		t.Fatalf("naturally drained task context was canceled: %v", context.Cause(taskCtx))
	}
}

func TestWorkerLifecycleCancelsRemainingTaskWithShutdownCause(t *testing.T) {
	lifecycle := newTaskWorkerLifecycle()
	if !lifecycle.start() {
		t.Fatal("worker lifecycle did not start")
	}
	_, taskCtx, done, err := lifecycle.claimTask(func() (*model.Task, error) {
		return &model.Task{ID: "task-running"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	causeSeen := make(chan error, 1)
	go func() {
		<-taskCtx.Done()
		causeSeen <- context.Cause(taskCtx)
		done()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := lifecycle.shutdown(ctx); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	select {
	case cause := <-causeSeen:
		if !errors.Is(cause, errWorkerShuttingDown) {
			t.Fatalf("task cancellation cause = %v, want errWorkerShuttingDown", cause)
		}
	case <-time.After(time.Second):
		t.Fatal("running task did not receive the shutdown cancellation cause")
	}
}

func TestWorkerLifecycleStopsBackgroundLoops(t *testing.T) {
	lifecycle := newTaskWorkerLifecycle()
	if !lifecycle.start() {
		t.Fatal("worker lifecycle did not start")
	}
	loopStopped := make(chan struct{})
	if !lifecycle.goLoop(func(ctx context.Context) {
		<-ctx.Done()
		close(loopStopped)
	}) {
		t.Fatal("background loop was not registered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.shutdown(ctx); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	select {
	case <-loopStopped:
	case <-time.After(time.Second):
		t.Fatal("background loop did not stop")
	}
}
