package repository

import (
	"strconv"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTasksForCreatePageMessageReturnsCompleteOwnedBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasIndex(&model.Task{}, "idx_tasks_user_create_page_context") {
		t.Fatal("create-page task context composite index was not created")
	}
	repo := New(db)
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	batch := []model.Task{
		createPageContextTask("task-1", "user-1", "conversation-1", "message-1", createdAt),
		createPageContextTask("task-2", "user-1", "conversation-1", "message-1", createdAt.Add(time.Second)),
		createPageContextTask("task-other-message", "user-1", "conversation-1", "message-2", createdAt.Add(2*time.Second)),
		createPageContextTask("task-other-user", "user-2", "conversation-1", "message-1", createdAt.Add(3*time.Second)),
		{ID: "task-other-source", UserID: "user-1", ClientContextSource: "canvas", ClientConversationID: "conversation-1", ClientMessageID: "message-1", Status: model.TaskStatusQueued, CreatedAt: createdAt.Add(4 * time.Second), UpdatedAt: createdAt.Add(4 * time.Second)},
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}

	tasks, err := repo.TasksForCreatePageMessage("user-1", "conversation-1", "message-1", 100)
	if err != nil {
		t.Fatalf("TasksForCreatePageMessage() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want complete two-task batch: %#v", len(tasks), tasks)
	}
	if tasks[0].ID != "task-2" || tasks[1].ID != "task-1" {
		t.Fatalf("task order = [%q, %q], want [task-2, task-1]", tasks[0].ID, tasks[1].ID)
	}
	for _, task := range tasks {
		if task.ClientContextSource != model.TaskClientContextSourceCreatePage || task.ClientConversationID != "conversation-1" || task.ClientMessageID != "message-1" {
			t.Fatalf("materialized context was not projected: %#v", task)
		}
	}
}

func TestTasksForCreatePageMessageUsesHardLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	batch := make([]model.Task, 0, createPageMessageTasksMaxLimit+1)
	for index := 0; index <= createPageMessageTasksMaxLimit; index++ {
		batch = append(batch, createPageContextTask(
			"task-limit-"+strconv.Itoa(index), "user-1", "conversation-1", "message-1", createdAt.Add(time.Duration(index)*time.Second),
		))
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}

	tasks, err := New(db).TasksForCreatePageMessage("user-1", "conversation-1", "message-1", createPageMessageTasksMaxLimit+1)
	if err != nil {
		t.Fatalf("TasksForCreatePageMessage() error = %v", err)
	}
	if len(tasks) != createPageMessageTasksMaxLimit {
		t.Fatalf("task count = %d, want hard limit %d", len(tasks), createPageMessageTasksMaxLimit)
	}
	if tasks[0].ID != "task-limit-100" || tasks[len(tasks)-1].ID != "task-limit-1" {
		t.Fatalf("hard-limited tasks should prefer latest rows: first=%q last=%q", tasks[0].ID, tasks[len(tasks)-1].ID)
	}
}

func createPageContextTask(id string, userID string, conversationID string, messageID string, createdAt time.Time) model.Task {
	return model.Task{
		ID: id, UserID: userID, ClientContextSource: model.TaskClientContextSourceCreatePage,
		ClientConversationID: conversationID, ClientMessageID: messageID,
		Status: model.TaskStatusQueued, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}
