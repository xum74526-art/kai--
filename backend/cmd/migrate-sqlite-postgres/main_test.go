package main

import (
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// legacyTaskSchema 模拟尚未引入创作页物化上下文字段的旧 SQLite tasks 表。
type legacyTaskSchema struct {
	ID        string `gorm:"primaryKey"`
	UserID    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (legacyTaskSchema) TableName() string {
	return "tasks"
}

func TestMigrationCoverageMatchesRuntimeSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	if err := verifyMigrationCoverage(db); err != nil {
		t.Fatalf("migration coverage: %v", err)
	}
}

func TestSourceColumnsForMigrationSupportsLegacyTaskSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	if err := db.AutoMigrate(&legacyTaskSchema{}); err != nil {
		t.Fatalf("create legacy task schema: %v", err)
	}
	legacy := legacyTaskSchema{ID: "task-1", UserID: "user-1", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy task: %v", err)
	}

	columns, err := sourceColumnsForMigration[model.Task](db)
	if err != nil {
		t.Fatalf("sourceColumnsForMigration() error = %v", err)
	}
	for _, missing := range []string{"id", "user_id", "created_at", "updated_at"} {
		if !containsColumn(columns, missing) {
			t.Fatalf("legacy source projection misses %q: %#v", missing, columns)
		}
	}
	for _, unexpected := range []string{"client_context_source", "client_conversation_id", "client_message_id"} {
		if containsColumn(columns, unexpected) {
			t.Fatalf("legacy source projection must omit absent %q: %#v", unexpected, columns)
		}
	}

	var tasks []model.Task
	if err := db.Select(columns).Order("id").Find(&tasks).Error; err != nil {
		t.Fatalf("read legacy task through projected current model: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != legacy.ID || tasks[0].ClientContextSource != "" {
		t.Fatalf("legacy task projection = %#v", tasks)
	}

	target, err := gorm.Open(sqlite.Open("file:"+t.Name()+"-target?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open current-schema target SQLite: %v", err)
	}
	if err := target.AutoMigrate(&model.Task{}); err != nil {
		t.Fatalf("create current task schema: %v", err)
	}
	count, err := migrateTable[model.Task]("tasks").run(db, target, true)
	if err != nil {
		t.Fatalf("migrate legacy task schema into current schema: %v", err)
	}
	if count != 1 {
		t.Fatalf("migrated task count = %d, want 1", count)
	}
	var copied model.Task
	if err := target.First(&copied, "id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("read copied task: %v", err)
	}
	if copied.UserID != legacy.UserID || copied.ClientContextSource != "" || copied.ClientConversationID != "" || copied.ClientMessageID != "" {
		t.Fatalf("copied legacy task = %#v", copied)
	}
}

func containsColumn(columns []string, expected string) bool {
	for _, column := range columns {
		if column == expected {
			return true
		}
	}
	return false
}
