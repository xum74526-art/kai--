package service

import (
	"path/filepath"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBeginTaskRouteAttemptDoesNotResubmitAmbiguousDispatch(t *testing.T) {
	for _, dispatchState := range []string{"accepted", "submission_unknown"} {
		t.Run(dispatchState, func(t *testing.T) {
			svc, db := newRouteRecoveryTestService(t)
			task := &model.Task{ID: "task-ambiguous", LogicalModelID: "logical-1", RouteRun: 1}
			if err := db.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			attempt := &model.RouteAttempt{ID: "attempt-1", TaskID: task.ID, RouteRun: task.RouteRun, AttemptNumber: 1, DispatchState: dispatchState, Status: "dispatching"}
			if err := db.Create(attempt).Error; err != nil {
				t.Fatal(err)
			}

			resumed, err := svc.beginTaskRouteAttempt(task)
			if resumed != nil || !isRouteDispatchUncertain(err) {
				t.Fatalf("beginTaskRouteAttempt() = (%#v, %v), want ambiguous dispatch stop", resumed, err)
			}
			var count int64
			if err := db.Model(&model.RouteAttempt{}).Where("task_id = ?", task.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("ambiguous dispatch created %d route attempts, want 1", count)
			}
		})
	}
}

func TestBeginTaskRouteAttemptRecoversPersistedProviderID(t *testing.T) {
	svc, db := newRouteRecoveryTestService(t)
	task := &model.Task{ID: "task-resume", LogicalModelID: "logical-1", RouteRun: 1}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	attempt := &model.RouteAttempt{ID: "attempt-1", TaskID: task.ID, RouteRun: task.RouteRun, AttemptNumber: 1, DispatchState: "submission_unknown", Status: "dispatching", ProviderRequestID: "provider-job-1"}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatal(err)
	}

	resumed, err := svc.beginTaskRouteAttempt(task)
	if err != nil {
		t.Fatalf("beginTaskRouteAttempt() error = %v", err)
	}
	if resumed == nil || resumed.ID != attempt.ID || task.ProviderRequestID != attempt.ProviderRequestID {
		t.Fatalf("provider task was not resumed: attempt=%#v taskProviderID=%q", resumed, task.ProviderRequestID)
	}
	var stored model.Task
	if err := db.First(&stored, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ProviderRequestID != attempt.ProviderRequestID {
		t.Fatalf("stored provider ID = %q, want %q", stored.ProviderRequestID, attempt.ProviderRequestID)
	}
}

func newRouteRecoveryTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	t.Setenv("REDIS_URL", "")
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "route-recovery.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close recovery test database: %v", err)
		}
	})
	if err := db.AutoMigrate(&model.Task{}, &model.RouteAttempt{}); err != nil {
		t.Fatal(err)
	}
	return New(repository.New(db), t.TempDir()), db
}
