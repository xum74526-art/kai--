package repository

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPreviousTaskLeaseCannotWriteAfterReclaim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:task-lease-fencing?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}); err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt := time.Now().Add(time.Minute)
	current := model.Task{
		ID: "task-1", UserID: "user-1", Status: model.TaskStatusRunning, Stage: "new worker",
		LeaseOwner: "worker:new", LeaseExpiresAt: &leaseExpiresAt, Attempts: 2,
	}
	if err := db.Create(&current).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)

	if err := repo.UpdateTaskProgress(current.ID, "worker:old", 1, "stale progress", 80); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("UpdateTaskProgress() error = %v, want ErrTaskStateConflict", err)
	}
	stale := current
	stale.LeaseOwner = "worker:old"
	stale.Attempts = 1
	stale.Status = model.TaskStatusSucceeded
	if err := repo.SaveTaskCompletion(&stale, model.TaskStatusRunning, nil, nil, nil); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("SaveTaskCompletion() error = %v, want ErrTaskStateConflict", err)
	}
	updated, err := repo.UpdateTaskTerminalState(current.ID, model.TaskStatusRunning, "worker:old", 1, model.TaskStatusFailed, "任务失败", "stale failure", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("stale lease unexpectedly wrote terminal state")
	}
	var stored model.Task
	if err := db.First(&stored, "id = ?", current.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Stage != "new worker" || stored.Status != model.TaskStatusRunning || stored.LeaseOwner != "worker:new" || stored.Attempts != 2 {
		t.Fatalf("stored task was changed by stale lease: %#v", stored)
	}
}

func TestExpiredTaskLeaseCannotBeRenewedOrUpdateProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:task-lease-expired?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}); err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Second)
	task := model.Task{ID: "task-1", UserID: "user-1", Status: model.TaskStatusRunning, LeaseOwner: "worker:lease", LeaseExpiresAt: &expiredAt, Attempts: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	if err := repo.RenewTaskLease(task.ID, task.LeaseOwner, task.Attempts, time.Minute); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("RenewTaskLease() error = %v, want ErrTaskStateConflict", err)
	}
	if err := repo.UpdateTaskProgress(task.ID, task.LeaseOwner, task.Attempts, "stale progress", 80); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("UpdateTaskProgress() error = %v, want ErrTaskStateConflict", err)
	}
}

func TestSaveTaskCompletionPromotesOnlyCurrentLeaseStagedResources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:task-resource-staging?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.Resource{}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Minute)
	task := model.Task{
		ID: "task-resource", UserID: "user-1", Status: model.TaskStatusRunning,
		LeaseOwner: "worker:lease-1", LeaseExpiresAt: &expiresAt, Attempts: 1,
	}
	resource := model.Resource{
		ID: "resource-staged", UserID: task.UserID, Status: model.ResourceStatusPending,
		ProducerTaskID: task.ID, ProducerLeaseOwner: task.LeaseOwner,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	stale := model.Resource{
		ID: "resource-stale", UserID: task.UserID, Status: model.ResourceStatusPending,
		ProducerTaskID: task.ID, ProducerLeaseOwner: "worker:old-lease",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	completed := task
	completed.Status = model.TaskStatusSucceeded
	completedAt := time.Now()
	completed.CompletedAt = &completedAt
	repo := New(db)
	if err := repo.SaveTaskCompletion(&completed, model.TaskStatusRunning, nil, nil, nil, []string{stale.ID}); !errors.Is(err, ErrResourceReferenceUnavailable) {
		t.Fatalf("SaveTaskCompletion() error = %v, want ErrResourceReferenceUnavailable for an old lease resource", err)
	}
	var stored model.Resource
	if err := db.First(&stored, "id = ?", stale.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.ResourceStatusPending {
		t.Fatalf("stale resource status = %q, want pending", stored.Status)
	}
	if err := repo.SaveTaskCompletion(&completed, model.TaskStatusRunning, nil, nil, nil, []string{resource.ID}); err != nil {
		t.Fatalf("SaveTaskCompletion() error = %v", err)
	}
	stored = model.Resource{}
	if err := db.First(&stored, "id = ?", resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.ResourceStatusReady || stored.ProducerTaskID != task.ID || stored.ProducerLeaseOwner != "" {
		t.Fatalf("resource = %#v, want ready resource with retained task provenance", stored)
	}
}
