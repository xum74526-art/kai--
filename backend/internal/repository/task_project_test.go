package repository

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateTaskRechecksArchivedProjectInsideTransaction(t *testing.T) {
	repo, db := newTaskProjectRepository(t)
	project := model.Project{ID: "project-1", UserID: "user-1", Name: "短剧", Status: model.ProjectStatusActive}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	// 模拟 service 预检通过后，另一个请求先完成归档。
	if err := db.Model(&model.Project{}).Where("id = ?", project.ID).Update("status", model.ProjectStatusArchived).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-1", UserID: project.UserID, ProjectID: project.ID, Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, TaskProjectRequirement{MustExist: true}); !errors.Is(err, ErrTaskProjectUnavailable) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want project conflict", err)
	}
	var count int64
	if err := db.Model(&model.Task{}).Where("id = ?", task.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("task count = %d, want 0", count)
	}
}

func TestCreateTaskRechecksCanvasParentProjectInsideTransaction(t *testing.T) {
	repo, db := newTaskProjectRepository(t)
	project := model.Project{ID: "project-1", UserID: "user-1", Name: "短剧", Status: model.ProjectStatusArchived}
	canvas := model.CanvasProject{ID: "canvas-1", UserID: project.UserID, ProjectID: project.ID, Title: "分镜"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&canvas).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-1", UserID: project.UserID, ProjectID: canvas.ID, Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, TaskProjectRequirement{MustExist: true}); !errors.Is(err, ErrTaskProjectUnavailable) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want project conflict", err)
	}
}

func TestCreateTaskRejectsScopeDeletedAfterRecognizedPrecheck(t *testing.T) {
	repo, db := newTaskProjectRepository(t)
	project := model.Project{ID: "project-delete-race", UserID: "user-1", Name: "短剧", Status: model.ProjectStatusActive}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	requirement := TaskProjectRequirement{MustExist: true}

	// 模拟 service 已识别 scope 后，删除请求先物理移除项目。
	if err := db.Delete(&model.Project{}, "id = ? AND user_id = ?", project.ID, project.UserID).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "task-delete-race", UserID: project.UserID, ProjectID: project.ID, Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, requirement); !errors.Is(err, ErrTaskProjectUnavailable) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want deleted project conflict", err)
	}
}

func TestCreateTaskKeepsUnknownHistoricalScopeCompatibility(t *testing.T) {
	repo, _ := newTaskProjectRepository(t)
	task := model.Task{ID: "task-legacy-scope", UserID: "user-1", ProjectID: "legacy-unknown", Type: "text", Status: model.TaskStatusQueued}
	if err := repo.CreateTaskWithActiveLimit(&task, 3, TaskProjectRequirement{}); err != nil {
		t.Fatalf("CreateTaskWithActiveLimit() legacy scope error = %v", err)
	}
}

func TestUpsertCanvasProjectRejectsDeletedParent(t *testing.T) {
	repo, db := newTaskProjectRepository(t)
	project := model.Project{ID: "project-canvas-race", UserID: "user-1", Name: "短剧", Status: model.ProjectStatusActive}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&model.Project{}, "id = ? AND user_id = ?", project.ID, project.UserID).Error; err != nil {
		t.Fatal(err)
	}

	canvas := model.CanvasProject{ID: "canvas-delete-race", UserID: project.UserID, ProjectID: project.ID, Title: "分镜"}
	if err := repo.UpsertCanvasProject(&canvas); !errors.Is(err, ErrCanvasParentProjectUnavailable) {
		t.Fatalf("UpsertCanvasProject() error = %v, want deleted parent conflict", err)
	}
	var count int64
	if err := db.Model(&model.CanvasProject{}).Where("id = ?", canvas.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("canvas count = %d, want 0", count)
	}
}

func TestUpsertCanvasProjectDoesNotAttachExistingCanvasToDeletedParent(t *testing.T) {
	repo, db := newTaskProjectRepository(t)
	project := model.Project{ID: "project-existing-canvas", UserID: "user-1", Name: "短剧", Status: model.ProjectStatusActive}
	canvas := model.CanvasProject{ID: "canvas-existing", UserID: project.UserID, Title: "独立画布"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&canvas).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&model.Project{}, "id = ? AND user_id = ?", project.ID, project.UserID).Error; err != nil {
		t.Fatal(err)
	}
	canvas.ProjectID = project.ID
	if err := repo.UpsertCanvasProject(&canvas); !errors.Is(err, ErrCanvasParentProjectUnavailable) {
		t.Fatalf("UpsertCanvasProject() error = %v, want deleted parent conflict", err)
	}
	var stored model.CanvasProject
	if err := db.First(&stored, "id = ?", canvas.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ProjectID != "" {
		t.Fatalf("stored project id = %q, want independent canvas", stored.ProjectID)
	}
}

func newTaskProjectRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Project{}, &model.CanvasProject{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	return New(db), db
}
