package repository

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDisableUserWithAuditCommitsAllSecurityChanges(t *testing.T) {
	db := newUserDisableTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	other := model.User{ID: "user-2", Username: "user-two", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	sessions := []model.AuthSession{
		{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)},
		{ID: "session-2", UserID: other.ID, ExpiresAt: now.Add(time.Hour)},
	}
	deltas := []model.TaskTextDelta{
		{ID: "delta-1", UserID: user.ID, TaskID: "task-1", Sequence: 1, Content: "a", ByteCount: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "delta-2", UserID: other.ID, TaskID: "task-2", Sequence: 1, Content: "b", ByteCount: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	}
	for _, item := range []any{&user, &other, &sessions, &deltas} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	event := model.AdminAuditEvent{ID: "audit-1", ActorUserID: "admin-1", Action: "user.disable", TargetType: "user", TargetID: user.ID, CreatedAt: now}

	if err := New(db).DisableUserWithAudit(&user, &event, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	stored, err := New(db).User(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.UserStatusDisabled {
		t.Fatalf("stored status = %q, want disabled", stored.Status)
	}
	assertUserDisableCount(t, db, &model.AuthSession{}, user.ID, 0)
	assertUserDisableCount(t, db, &model.TaskTextDelta{}, user.ID, 0)
	assertUserDisableCount(t, db, &model.AuthSession{}, other.ID, 1)
	assertUserDisableCount(t, db, &model.TaskTextDelta{}, other.ID, 1)
	var audits int64
	if err := db.Model(&model.AdminAuditEvent{}).Where("id = ? AND target_id = ?", event.ID, user.ID).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit count = %d, want 1", audits)
	}
}

func TestDisableUserWithAuditRollsBackWhenAuditFails(t *testing.T) {
	db := newUserDisableTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	session := model.AuthSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)}
	delta := model.TaskTextDelta{ID: "delta-1", UserID: user.ID, TaskID: "task-1", Sequence: 1, Content: "a", ByteCount: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	conflict := model.AdminAuditEvent{ID: "audit-conflict", ActorUserID: "admin-1", Action: "existing", TargetType: "user", TargetID: "other", CreatedAt: now}
	for _, item := range []any{&user, &session, &delta, &conflict} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	event := model.AdminAuditEvent{ID: conflict.ID, ActorUserID: "admin-1", Action: "user.disable", TargetType: "user", TargetID: user.ID, CreatedAt: now}

	if err := New(db).DisableUserWithAudit(&user, &event, now.Add(time.Minute)); err == nil {
		t.Fatal("DisableUserWithAudit() error = nil")
	}
	stored, err := New(db).User(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.UserStatusActive {
		t.Fatalf("status after rollback = %q, want active", stored.Status)
	}
	assertUserDisableCount(t, db, &model.AuthSession{}, user.ID, 1)
	assertUserDisableCount(t, db, &model.TaskTextDelta{}, user.ID, 1)
}

func TestDisableUserWithAuditRejectsStaleSecurityState(t *testing.T) {
	db := newUserDisableTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	session := model.AuthSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("status", model.UserStatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	event := model.AdminAuditEvent{ID: "audit-1", ActorUserID: "admin-1", Action: "user.disable", TargetType: "user", TargetID: user.ID, CreatedAt: now}

	err := New(db).DisableUserWithAudit(&user, &event, now.Add(time.Minute))
	if !errors.Is(err, ErrUserAdminStateChanged) {
		t.Fatalf("DisableUserWithAudit() error = %v, want ErrUserAdminStateChanged", err)
	}
	assertUserDisableCount(t, db, &model.AuthSession{}, user.ID, 1)
	var audits int64
	if err := db.Model(&model.AdminAuditEvent{}).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatalf("audit count = %d, want 0", audits)
	}
}

func newUserDisableTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AuthSession{}, &model.TaskTextDelta{}, &model.AdminAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertUserDisableCount(t *testing.T, db *gorm.DB, value any, userID string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(value).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%T count for %s = %d, want %d", value, userID, count, want)
	}
}
