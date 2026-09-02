package service

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaskProjectPrecheckStillRejectsProjectDeletedBeforeCreateTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Project{}, &model.CanvasProject{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	svc := &Service{repo: repo}
	project := model.Project{ID: "project-precheck-race", UserID: "user-1", Name: "短剧", Status: model.ProjectStatusActive}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	requirement, err := svc.ensureTaskProjectActive(project.UserID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !requirement.MustExist {
		t.Fatal("recognized project precheck did not require transactional existence")
	}
	if err := db.Delete(&model.Project{}, "id = ? AND user_id = ?", project.ID, project.UserID).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-precheck-race", UserID: project.UserID, ProjectID: project.ID, Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, requirement); !errors.Is(err, repository.ErrTaskProjectUnavailable) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want deleted project conflict", err)
	}
}

func TestUnknownTaskProjectScopeIsRejectedForNewTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Project{}, &model.CanvasProject{}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db)}
	if _, err := svc.ensureTaskProjectActive("user-1", "unknown-project"); err == nil {
		t.Fatal("unknown project scope must be rejected for a new task")
	}
}
