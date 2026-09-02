package repository

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateTaskRejectsForeignSessionBeforePersisting(t *testing.T) {
	repo, db := newTaskSessionRepository(t)
	session := model.Session{ID: "session-1", UserID: "user-2", ProjectID: "project-1"}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-1", UserID: "user-1", SessionID: session.ID, ProjectID: session.ProjectID, Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, TaskProjectRequirement{}); !errors.Is(err, ErrTaskSessionUnavailable) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want unavailable session", err)
	}
	assertTaskNotCreated(t, db, task.ID)
}

func TestCreateTaskRejectsSessionProjectMismatch(t *testing.T) {
	repo, db := newTaskSessionRepository(t)
	session := model.Session{ID: "session-1", UserID: "user-1", ProjectID: "project-1"}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-1", UserID: session.UserID, SessionID: session.ID, ProjectID: "project-2", Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, TaskProjectRequirement{}); !errors.Is(err, ErrTaskSessionProjectMismatch) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want project mismatch", err)
	}
	assertTaskNotCreated(t, db, task.ID)
}

func TestCreateTaskAcceptsOwnedSessionInSameProject(t *testing.T) {
	repo, db := newTaskSessionRepository(t)
	session := model.Session{ID: "session-1", UserID: "user-1", ProjectID: "project-1"}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-1", UserID: session.UserID, SessionID: session.ID, ProjectID: session.ProjectID, Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, TaskProjectRequirement{}); err != nil {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v", err)
	}
}

func TestRetryTaskResetsSessionInsideBillingTransaction(t *testing.T) {
	repo, db := newTaskSessionRepository(t)
	session := model.Session{ID: "session-retry", UserID: "user-1", ProjectID: "project-1", Status: model.SessionStatusCompleted, CanvasOpsJSON: `[{"op":"stale"}]`}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-retry", UserID: session.UserID, SessionID: session.ID, ProjectID: session.ProjectID, Type: "text", Status: model.TaskStatusFailed, InputJSON: `{}`}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := repo.RetryTaskWithBilling(task.UserID, &task, nil, 3, TaskProjectRequirement{}); err != nil {
		t.Fatalf("RetryTaskWithBilling() error = %v", err)
	}
	var stored model.Session
	if err := db.First(&stored, "id = ?", session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.SessionStatusActive || stored.CanvasOpsJSON != "" {
		t.Fatalf("session after retry = status %q, canvasOps %q", stored.Status, stored.CanvasOpsJSON)
	}
}

func newTaskSessionRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Project{}, &model.CanvasProject{}, &model.Session{}, &model.Task{}, &model.TaskTextDelta{}); err != nil {
		t.Fatal(err)
	}
	return New(db), db
}

func assertTaskNotCreated(t *testing.T, db *gorm.DB, taskID string) {
	t.Helper()
	var count int64
	if err := db.Model(&model.Task{}).Where("id = ?", taskID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("task count = %d, want 0", count)
	}
}
