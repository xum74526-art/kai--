package repository

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompleteUserLoginPreservesConcurrentAccountChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{
		ID: "user-1", Username: "user-one", DisplayName: "Before", PasswordHash: "hash-old",
		Role: model.UserRoleUser, Status: model.UserStatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	// 模拟登录读取用户后，管理员并发修改角色、资料和密码。
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"display_name": "After", "role": model.UserRoleAdmin, "password_hash": "hash-new",
	}).Error; err != nil {
		t.Fatal(err)
	}
	loggedInAt := time.Now().Add(time.Minute).Round(time.Millisecond)
	updated, err := New(db).CompleteUserLogin(user.ID, "", loggedInAt)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "After" || updated.Role != model.UserRoleAdmin || updated.PasswordHash != "hash-new" {
		t.Fatalf("concurrent account changes were overwritten: %#v", updated)
	}
	if updated.LastLoginAt == nil || updated.LastLoginAt.Before(loggedInAt.Add(-time.Second)) {
		t.Fatalf("last login was not updated: %#v", updated.LastLoginAt)
	}
}

func TestCompleteUserLoginRejectsConcurrentPasswordOrStatusChange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	users := []model.User{
		{ID: "password-user", Username: "password-user", PasswordHash: "hash-new", Role: model.UserRoleUser, Status: model.UserStatusActive},
		{ID: "disabled-user", Username: "disabled-user", PasswordHash: "hash-old", Role: model.UserRoleUser, Status: model.UserStatusDisabled},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	if _, err := repo.CompleteUserLogin("password-user", "hash-old", time.Now()); !errors.Is(err, ErrUserLoginStateChanged) {
		t.Fatalf("password change error = %v", err)
	}
	if _, err := repo.CompleteUserLogin("disabled-user", "", time.Now()); !errors.Is(err, ErrUserLoginStateChanged) {
		t.Fatalf("disabled account error = %v", err)
	}
	for _, id := range []string{"password-user", "disabled-user"} {
		stored, err := repo.User(id)
		if err != nil {
			t.Fatal(err)
		}
		if stored.LastLoginAt != nil {
			t.Fatalf("rejected login updated %s last login", id)
		}
	}
}
