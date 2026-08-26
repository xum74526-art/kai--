package service

import (
	"testing"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompleteLinuxDOAuthSessionRejectsConcurrentDisableWithoutCreatingSession(t *testing.T) {
	svc, db := newLinuxDOConsistencyTestService(t)
	stored := model.User{ID: "user-1", Username: "oauth-user", Role: model.UserRoleUser, Status: model.UserStatusDisabled}
	identity := model.UserIdentity{ID: "identity-1", UserID: stored.ID, Provider: "linuxdo", Subject: "subject-1"}
	seedOAuthLoginUser(t, db, stored, identity)
	stale := stored
	stale.Status = model.UserStatusActive

	if _, err := svc.completeLinuxDOAuthSession(&stale, &identity, false); err == nil {
		t.Fatal("completeLinuxDOAuthSession() error = nil")
	}
	var sessions int64
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", stored.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("session count = %d, want 0", sessions)
	}
}

func TestCompleteLinuxDOAuthSessionUsesLatestRoleWhenAdminChangesIt(t *testing.T) {
	svc, db := newLinuxDOConsistencyTestService(t)
	stored := model.User{ID: "user-1", Username: "oauth-user", Role: model.UserRoleUser, Status: model.UserStatusActive}
	identity := model.UserIdentity{ID: "identity-1", UserID: stored.ID, Provider: "linuxdo", Subject: "subject-1"}
	seedOAuthLoginUser(t, db, stored, identity)
	stale := stored
	stale.Role = model.UserRoleAdmin

	result, err := svc.completeLinuxDOAuthSession(&stale, &identity, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Role != model.UserRoleUser {
		t.Fatalf("session user role = %q, want latest role %q", result.User.Role, model.UserRoleUser)
	}
	var sessions int64
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", stored.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("session count = %d, want 1", sessions)
	}
}

func newLinuxDOConsistencyTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(database.Models()...); err != nil {
		t.Fatal(err)
	}
	return New(repository.New(db), t.TempDir()), db
}

func seedOAuthLoginUser(t *testing.T, db *gorm.DB, user model.User, identity model.UserIdentity) {
	t.Helper()
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	identity.CreatedAt = now
	identity.UpdatedAt = now
	account := model.CreditAccount{UserID: user.ID, AvailableMicrocredits: 1, CreatedAt: now, UpdatedAt: now}
	reference := "signup:" + user.ID
	ledger := model.CreditLedgerEntry{ID: "ledger-" + user.ID, UserID: user.ID, Type: model.CreditLedgerSignupBonus, ReferenceKey: &reference, CreatedAt: now}
	for _, item := range []any{&user, &identity, &account, &ledger} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
}
