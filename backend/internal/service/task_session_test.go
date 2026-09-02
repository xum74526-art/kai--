package service

import (
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureTaskSessionScopeRejectsForeignAndMismatchedSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Session{}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db)}
	if err := db.Create(&model.Session{ID: "session-1", UserID: "user-1", ProjectID: "project-1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ensureTaskSessionScope("user-2", "session-1", "project-1"); err == nil {
		t.Fatal("foreign session should be rejected")
	}
	if err := svc.ensureTaskSessionScope("user-1", "session-1", "project-2"); err == nil {
		t.Fatal("session/project mismatch should be rejected")
	}
	if err := svc.ensureTaskSessionScope("user-1", "session-1", "project-1"); err != nil {
		t.Fatalf("owned matching session error = %v", err)
	}
}
