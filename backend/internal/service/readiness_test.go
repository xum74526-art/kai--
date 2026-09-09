package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"infinite-canvas/backend/internal/repository"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReadinessChecksEnabledDependencies(t *testing.T) {
	svc, closeDB := newReadinessTestService(t)
	defer closeDB()
	if !svc.lifecycle().start() {
		t.Fatal("worker lifecycle did not start")
	}

	report, err := svc.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness() error = %v", err)
	}
	if report.Status != readinessOK {
		t.Fatalf("status = %q, want %q", report.Status, readinessOK)
	}
	if _, ok := report.Checks["redis"]; ok {
		t.Fatal("disabled Redis must not be included in readiness checks")
	}
}

func TestReadinessFailsWhenDatabaseIsUnavailable(t *testing.T) {
	svc, closeDB := newReadinessTestService(t)
	if !svc.lifecycle().start() {
		t.Fatal("worker lifecycle did not start")
	}
	closeDB()

	report, err := svc.Readiness(context.Background())
	if err == nil {
		t.Fatal("Readiness() should fail after the database is closed")
	}
	if report.Checks["database"] != readinessFailed {
		t.Fatalf("database check = %q, want %q", report.Checks["database"], readinessFailed)
	}
}

func TestReadinessFailsWhenRequiredStorageIsUnavailable(t *testing.T) {
	svc, closeDB := newReadinessTestService(t)
	defer closeDB()
	if !svc.lifecycle().start() {
		t.Fatal("worker lifecycle did not start")
	}
	svc.dataDir = filepath.Join(t.TempDir(), "missing")

	report, err := svc.Readiness(context.Background())
	if err == nil {
		t.Fatal("Readiness() should fail when the data directory is missing")
	}
	if report.Checks["storage"] != readinessFailed {
		t.Fatalf("storage check = %q, want %q", report.Checks["storage"], readinessFailed)
	}
}

func TestReadinessChecksConfiguredRedis(t *testing.T) {
	svc, closeDB := newReadinessTestService(t)
	defer closeDB()
	if !svc.lifecycle().start() {
		t.Fatal("worker lifecycle did not start")
	}
	svc.coordinator.redis = redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 20 * time.Millisecond,
	})
	defer svc.coordinator.redis.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	report, err := svc.Readiness(ctx)
	if err == nil {
		t.Fatal("Readiness() should fail when configured Redis is unavailable")
	}
	if report.Checks["redis"] != readinessFailed {
		t.Fatalf("redis check = %q, want %q", report.Checks["redis"], readinessFailed)
	}
}

func TestReadinessFailsAfterShutdownBegins(t *testing.T) {
	svc, closeDB := newReadinessTestService(t)
	defer closeDB()
	if !svc.lifecycle().start() {
		t.Fatal("worker lifecycle did not start")
	}
	svc.BeginShutdown()

	report, err := svc.Readiness(context.Background())
	if err == nil {
		t.Fatal("Readiness() should fail after shutdown begins")
	}
	if report.Checks["worker"] != readinessFailed {
		t.Fatalf("worker check = %q, want %q", report.Checks["worker"], readinessFailed)
	}
}

func newReadinessTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	t.Setenv("REDIS_URL", "")
	dbPath := filepath.Join(t.TempDir(), "readiness.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	closeDB := func() {
		if !closed {
			closed = true
			if err := sqlDB.Close(); err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("close test database: %v", err)
			}
		}
	}
	return New(repository.New(db), t.TempDir()), closeDB
}
