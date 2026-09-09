package service

import (
	"net/http"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
)

func TestDeleteUserDisablesAccountAndRevokesTransientState(t *testing.T) {
	db := newBulkUserTestDB(t)
	now := time.Now().Round(time.Millisecond)
	actor := model.User{ID: "admin-1", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}
	user := model.User{ID: "user-1", Username: "user-one", Role: model.UserRoleUser, Status: model.UserStatusActive}
	session := model.AuthSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)}
	delta := model.TaskTextDelta{ID: "delta-1", UserID: user.ID, TaskID: "task-1", Sequence: 1, Content: "a", ByteCount: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	for _, item := range []any{&actor, &user, &session, &delta} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := (&Service{repo: repository.New(db)}).DeleteUser(&actor, user.ID); err != nil {
		t.Fatal(err)
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.UserStatusDisabled {
		t.Fatalf("stored status = %q, want disabled", stored.Status)
	}
	for _, check := range []struct {
		value any
		want  int64
	}{
		{value: &model.AuthSession{}, want: 0},
		{value: &model.TaskTextDelta{}, want: 0},
		{value: &model.AdminAuditEvent{}, want: 1},
	} {
		var count int64
		query := db.Model(check.value)
		if _, ok := check.value.(*model.AdminAuditEvent); ok {
			query = query.Where("action = ? AND target_id = ?", "user.disable", user.ID)
		} else {
			query = query.Where("user_id = ?", user.ID)
		}
		if err := query.Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%T count = %d, want %d", check.value, count, check.want)
		}
	}
}

func TestDeleteUserRejectsCurrentAdministrator(t *testing.T) {
	db := newBulkUserTestDB(t)
	actor := model.User{ID: "admin-1", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatal(err)
	}

	err := (&Service{repo: repository.New(db)}).DeleteUser(&actor, actor.ID)
	appErr, ok := err.(*AppError)
	if !ok || appErr.Status != http.StatusBadRequest {
		t.Fatalf("DeleteUser() error = %#v, want 400 AppError", err)
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.UserStatusActive {
		t.Fatalf("administrator status = %q, want active", stored.Status)
	}
}
