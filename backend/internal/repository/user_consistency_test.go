package repository

import (
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateRegisteredUserRollsBackVerificationAndBootstrapOnSessionFailure(t *testing.T) {
	db := newUserConsistencyTestDB(t)
	now := time.Now().Round(time.Millisecond)
	code := model.EmailVerificationCode{ID: "code-1", Email: "user@example.com", Purpose: "registration", ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	conflict := model.AuthSession{ID: "session-conflict", UserID: "existing-user", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&code).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "user-one", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	account := model.CreditAccount{UserID: user.ID, AvailableMicrocredits: 10, CreatedAt: now, UpdatedAt: now}
	reference := "signup:" + user.ID
	ledger := model.CreditLedgerEntry{ID: "ledger-1", UserID: user.ID, Type: model.CreditLedgerSignupBonus, AmountMicrocredits: 10, ReferenceKey: &reference, CreatedAt: now}
	session := model.AuthSession{ID: conflict.ID, UserID: user.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}

	if err := New(db).CreateRegisteredUser(&user, code.ID, now, &account, &ledger, &session); err == nil {
		t.Fatal("CreateRegisteredUser() error = nil")
	}
	var storedCode model.EmailVerificationCode
	if err := db.First(&storedCode, "id = ?", code.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedCode.UsedAt != nil {
		t.Fatalf("verification code was consumed after rollback: %v", storedCode.UsedAt)
	}
	assertUserBootstrapRowCounts(t, db, user.ID, 0, 0, 0)
}

func TestCreateAdminUserRollsBackBootstrapOnAuditFailure(t *testing.T) {
	db := newUserConsistencyTestDB(t)
	now := time.Now().Round(time.Millisecond)
	conflict := model.AdminAuditEvent{ID: "audit-conflict", ActorUserID: "admin-1", Action: "existing", TargetType: "user", TargetID: "existing", CreatedAt: now}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "user-one", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	account := model.CreditAccount{UserID: user.ID, CreatedAt: now, UpdatedAt: now}
	event := model.AdminAuditEvent{ID: conflict.ID, ActorUserID: "admin-1", Action: "user.create", TargetType: "user", TargetID: user.ID, CreatedAt: now}

	if err := New(db).CreateAdminUserWithAudit(&user, &account, nil, &event); err == nil {
		t.Fatal("CreateAdminUserWithAudit() error = nil")
	}
	assertUserBootstrapRowCounts(t, db, user.ID, 0, 0, 0)
}

func TestCreateOAuthUserRollsBackIdentityAndBootstrapOnSessionFailure(t *testing.T) {
	db := newUserConsistencyTestDB(t)
	now := time.Now().Round(time.Millisecond)
	conflict := model.AuthSession{ID: "session-conflict", UserID: "existing-user", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "oauth-user", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	identity := model.UserIdentity{ID: "identity-1", UserID: user.ID, Provider: "linuxdo", Subject: "subject-1", CreatedAt: now, UpdatedAt: now}
	account := model.CreditAccount{UserID: user.ID, AvailableMicrocredits: 10, CreatedAt: now, UpdatedAt: now}
	reference := "signup:" + user.ID
	ledger := model.CreditLedgerEntry{ID: "ledger-1", UserID: user.ID, Type: model.CreditLedgerSignupBonus, AmountMicrocredits: 10, ReferenceKey: &reference, CreatedAt: now}
	session := model.AuthSession{ID: conflict.ID, UserID: user.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}

	if err := New(db).CreateOAuthUserWithSession(&user, &identity, &account, &ledger, &session); err == nil {
		t.Fatal("CreateOAuthUserWithSession() error = nil")
	}
	assertUserBootstrapRowCounts(t, db, user.ID, 0, 0, 0)
	var identities int64
	if err := db.Model(&model.UserIdentity{}).Where("user_id = ?", user.ID).Count(&identities).Error; err != nil {
		t.Fatal(err)
	}
	if identities != 0 {
		t.Fatalf("identity count = %d, want 0", identities)
	}
}

func TestCompleteUserLoginWithSessionRollsBackLastLoginOnSessionFailure(t *testing.T) {
	db := newUserConsistencyTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", PasswordHash: "hash-old", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	conflict := model.AuthSession{ID: "session-conflict", UserID: user.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	session := model.AuthSession{ID: conflict.ID, UserID: user.ID, ExpiresAt: now.Add(2 * time.Hour), CreatedAt: now, UpdatedAt: now}
	if _, err := New(db).CompleteUserLoginWithSession(user.ID, user.PasswordHash, now.Add(time.Minute), &session); err == nil {
		t.Fatal("CompleteUserLoginWithSession() error = nil")
	}
	stored, err := New(db).User(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastLoginAt != nil {
		t.Fatalf("last login was committed without a session: %v", stored.LastLoginAt)
	}
}

func TestUpdateUserWithAuditRollsBackSecurityChangeAndSessionRevocation(t *testing.T) {
	db := newUserConsistencyTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", Email: "before@example.com", DisplayName: "Before", PasswordHash: "hash-old", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	session := model.AuthSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	conflict := model.AdminAuditEvent{ID: "audit-conflict", ActorUserID: "admin-1", Action: "existing", TargetType: "user", TargetID: "existing", CreatedAt: now}
	for _, item := range []any{&user, &session, &conflict} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	newHash := "hash-new"
	event := model.AdminAuditEvent{ID: conflict.ID, ActorUserID: "admin-1", Action: "user.update", TargetType: "user", TargetID: user.ID, CreatedAt: now}
	_, err := New(db).UpdateUserWithAudit(&user, AdminUserUpdate{
		DisplayName: user.DisplayName, Email: user.Email, PasswordHash: &newHash,
		Role: user.Role, Status: model.UserStatusDisabled, UpdatedAt: now.Add(time.Minute),
	}, true, &event)
	if err == nil {
		t.Fatal("UpdateUserWithAudit() error = nil")
	}
	stored, err := New(db).User(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != user.PasswordHash || stored.Status != user.Status {
		t.Fatalf("security update was partially committed: %#v", stored)
	}
	var sessions int64
	if err := db.Model(&model.AuthSession{}).Where("id = ?", session.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("session count = %d, want 1", sessions)
	}
}

func newUserConsistencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.UserIdentity{}, &model.AuthSession{}, &model.EmailVerificationCode{},
		&model.CreditAccount{}, &model.CreditLedgerEntry{}, &model.AdminAuditEvent{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertUserBootstrapRowCounts(t *testing.T, db *gorm.DB, userID string, users int64, accounts int64, ledgers int64) {
	t.Helper()
	checks := []struct {
		model  any
		column string
		want   int64
	}{
		{model: &model.User{}, column: "id", want: users},
		{model: &model.CreditAccount{}, column: "user_id", want: accounts},
		{model: &model.CreditLedgerEntry{}, column: "user_id", want: ledgers},
	}
	for _, check := range checks {
		var count int64
		if err := db.Model(check.model).Where(check.column+" = ?", userID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%T count = %d, want %d", check.model, count, check.want)
		}
	}
}
