package repository

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPasswordResetConsumesCodesAndRevokesEveryUserSession(t *testing.T) {
	db := newPasswordResetRepositoryTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", Email: "user@example.com", PasswordHash: "old-hash", Role: model.UserRoleUser, Status: model.UserStatusActive}
	otherUser := model.User{ID: "user-2", Username: "user-two", Email: "other@example.com", PasswordHash: "other-hash", Role: model.UserRoleUser, Status: model.UserStatusActive}
	codes := []model.EmailVerificationCode{
		{ID: "code-current", UserID: user.ID, Email: user.Email, Purpose: "password_reset", ExpiresAt: now.Add(time.Minute)},
		{ID: "code-other", UserID: user.ID, Email: user.Email, Purpose: "password_reset", ExpiresAt: now.Add(2 * time.Minute)},
		{ID: "code-registration", UserID: user.ID, Email: user.Email, Purpose: "registration", ExpiresAt: now.Add(2 * time.Minute)},
	}
	sessions := []model.AuthSession{
		{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)},
		{ID: "session-2", UserID: user.ID, ExpiresAt: now.Add(time.Hour)},
		{ID: "session-other", UserID: otherUser.ID, ExpiresAt: now.Add(time.Hour)},
	}
	for _, item := range []any{&user, &otherUser, &codes, &sessions} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	err := New(db).ResetPasswordWithEmailVerification(codes[0].ID, user.ID, user.Email, "password_reset", user.PasswordHash, "new-hash", now, 5)
	if err != nil {
		t.Fatal(err)
	}
	var storedUser model.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedUser.PasswordHash != "new-hash" {
		t.Fatalf("password hash = %q, want new-hash", storedUser.PasswordHash)
	}
	var activeResetCodes int64
	if err := db.Model(&model.EmailVerificationCode{}).Where("user_id = ? AND purpose = ? AND used_at IS NULL", user.ID, "password_reset").Count(&activeResetCodes).Error; err != nil {
		t.Fatal(err)
	}
	if activeResetCodes != 0 {
		t.Fatalf("active reset codes = %d, want 0", activeResetCodes)
	}
	var activeRegistrationCodes int64
	if err := db.Model(&model.EmailVerificationCode{}).Where("id = ? AND used_at IS NULL", "code-registration").Count(&activeRegistrationCodes).Error; err != nil {
		t.Fatal(err)
	}
	if activeRegistrationCodes != 1 {
		t.Fatalf("active registration codes = %d, want 1", activeRegistrationCodes)
	}
	var userSessions, otherSessions int64
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", user.ID).Count(&userSessions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", otherUser.ID).Count(&otherSessions).Error; err != nil {
		t.Fatal(err)
	}
	if userSessions != 0 || otherSessions != 1 {
		t.Fatalf("session counts = user:%d other:%d, want 0 and 1", userSessions, otherSessions)
	}
}

func TestPasswordResetRollsBackConsumedCodeWhenUserStateChanges(t *testing.T) {
	db := newPasswordResetRepositoryTestDB(t)
	now := time.Now().Round(time.Millisecond)
	user := model.User{ID: "user-1", Username: "user-one", Email: "user@example.com", PasswordHash: "changed-hash", Role: model.UserRoleUser, Status: model.UserStatusActive}
	code := model.EmailVerificationCode{ID: "code-1", UserID: user.ID, Email: user.Email, Purpose: "password_reset", ExpiresAt: now.Add(time.Minute)}
	session := model.AuthSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)}
	for _, item := range []any{&user, &code, &session} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	err := New(db).ResetPasswordWithEmailVerification(code.ID, user.ID, user.Email, "password_reset", "old-hash", "new-hash", now, 5)
	if !errors.Is(err, ErrPasswordResetStateChanged) {
		t.Fatalf("ResetPasswordWithEmailVerification() error = %v, want ErrPasswordResetStateChanged", err)
	}
	var storedUser model.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	var storedCode model.EmailVerificationCode
	if err := db.First(&storedCode, "id = ?", code.ID).Error; err != nil {
		t.Fatal(err)
	}
	var sessions int64
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", user.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if storedUser.PasswordHash != user.PasswordHash || storedCode.UsedAt != nil || sessions != 1 {
		t.Fatalf("partial reset persisted: user=%#v code=%#v", storedUser, storedCode)
	}
}

func TestPasswordResetFailedAttemptsStopAtFive(t *testing.T) {
	db := newPasswordResetRepositoryTestDB(t)
	now := time.Now().Round(time.Millisecond)
	code := model.EmailVerificationCode{ID: "code-1", UserID: "user-1", Email: "user@example.com", Purpose: "password_reset", ExpiresAt: now.Add(time.Minute)}
	if err := db.Create(&code).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	for attempt := 0; attempt < 5; attempt++ {
		if err := repo.IncrementEmailVerificationCodeFailedAttempts(code.ID, now, 5); err != nil {
			t.Fatalf("attempt %d error = %v", attempt+1, err)
		}
	}
	if err := repo.IncrementEmailVerificationCodeFailedAttempts(code.ID, now, 5); !errors.Is(err, ErrEmailVerificationCodeInvalid) {
		t.Fatalf("sixth attempt error = %v, want ErrEmailVerificationCodeInvalid", err)
	}
	var stored model.EmailVerificationCode
	if err := db.First(&stored, "id = ?", code.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.FailedAttempts != 5 {
		t.Fatalf("failed attempts = %d, want 5", stored.FailedAttempts)
	}
}

func newPasswordResetRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AuthSession{}, &model.EmailVerificationCode{}); err != nil {
		t.Fatal(err)
	}
	return db
}
