package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeCoordinatorWaitsUntilChannelSlotIsReleased(t *testing.T) {
	coordinator := &runtimeCoordinator{instanceID: "test", localRate: map[string]localRateEntry{}, localSlots: map[string]map[string]time.Time{}}
	releaseFirst, acquired, err := coordinator.acquire(context.Background(), "channel:one", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = (%v, %v), want acquired", acquired, err)
	}

	result := make(chan error, 1)
	go func() {
		releaseSecond, waitErr := coordinator.acquireWithWait(context.Background(), "channel:one", 1, time.Minute)
		if waitErr == nil {
			releaseSecond()
		}
		result <- waitErr
	}()

	select {
	case err := <-result:
		t.Fatalf("second acquire returned before release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseFirst()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("second acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second acquire did not resume after release")
	}
}

func TestRuntimeCoordinatorStopsWaitingWhenContextIsCancelled(t *testing.T) {
	coordinator := &runtimeCoordinator{instanceID: "test", localRate: map[string]localRateEntry{}, localSlots: map[string]map[string]time.Time{}}
	release, acquired, err := coordinator.acquire(context.Background(), "channel:one", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = (%v, %v), want acquired", acquired, err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.acquireWithWait(ctx, "channel:one", 1, time.Minute); err == nil {
		t.Fatal("acquireWithWait() error = nil after cancellation")
	}
}

func TestRuntimeCoordinatorRenewableLocalSlotExtendsLeaseUntilRelease(t *testing.T) {
	coordinator := &runtimeCoordinator{instanceID: "test", localRate: map[string]localRateEntry{}, localSlots: map[string]map[string]time.Time{}}
	lease, acquired, err := coordinator.acquireLease(context.Background(), "workers", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquireLease() = (%v, %v), want acquired", acquired, err)
	}

	coordinator.localMu.Lock()
	// 将剩余租期缩短后续租，避免依赖测试机时钟精度判断是否真的延长了 TTL。
	coordinator.localSlots["workers"][lease.token] = time.Now().Add(10 * time.Second)
	initialExpiry := coordinator.localSlots["workers"][lease.token]
	coordinator.localMu.Unlock()
	if err := lease.Renew(context.Background()); err != nil {
		t.Fatalf("Renew() error = %v", err)
	}
	coordinator.localMu.Lock()
	renewedExpiry := coordinator.localSlots["workers"][lease.token]
	coordinator.localMu.Unlock()
	if !renewedExpiry.After(initialExpiry) {
		t.Fatalf("renewed expiry = %v, want after %v", renewedExpiry, initialExpiry)
	}

	second, acquired, err := coordinator.acquireLease(context.Background(), "workers", 1, time.Minute)
	if err != nil {
		t.Fatalf("second acquireLease() error = %v", err)
	}
	if acquired {
		second.Release()
		t.Fatal("second acquireLease() acquired while renewed slot is active")
	}

	lease.Release()
	second, acquired, err = coordinator.acquireLease(context.Background(), "workers", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquireLease() after release = (%v, %v), want acquired", acquired, err)
	}
	second.Release()
}

func TestWorkerGlobalSlotRenewFailsClosedWhenLeaseIsLost(t *testing.T) {
	coordinator := &runtimeCoordinator{instanceID: "test", localRate: map[string]localRateEntry{}, localSlots: map[string]map[string]time.Time{}}
	lease, acquired, err := coordinator.acquireLease(context.Background(), "workers", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquireLease() = (%v, %v), want acquired", acquired, err)
	}
	defer lease.Release()

	coordinator.localMu.Lock()
	delete(coordinator.localSlots["workers"], lease.token)
	coordinator.localMu.Unlock()
	done := make(chan struct{})
	if err := renewWorkerGlobalSlotLease(lease, done); !errors.Is(err, errRuntimeSlotLeaseLost) {
		t.Fatalf("renewWorkerGlobalSlotLease() error = %v, want lost lease", err)
	}
}
