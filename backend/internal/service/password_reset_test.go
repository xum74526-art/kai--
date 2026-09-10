package service

import (
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPasswordResetWorksWhenPublicRegistrationIsClosed(t *testing.T) {
	db, svc := newPasswordResetTestService(t)
	user, code := createPasswordResetFixture(t, db, svc, passwordResetEmailPurpose)
	for _, id := range []string{"session-1", "session-2"} {
		if err := db.Create(&model.AuthSession{ID: id, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.ConfirmPasswordReset(PasswordResetConfirmRequest{Email: user.Email, EmailCode: code, NewPassword: "new-strong-password"}); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.repo.User(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("new-strong-password")) != nil {
		t.Fatal("new password was not stored")
	}
	var sessions int64
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", user.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("sessions = %d, want 0", sessions)
	}
	var storedCode model.EmailVerificationCode
	if err := db.First(&storedCode, "email = ? AND purpose = ?", user.Email, passwordResetEmailPurpose).Error; err != nil {
		t.Fatal(err)
	}
	if storedCode.UsedAt == nil {
		t.Fatal("password reset code was not consumed")
	}
}

func TestPasswordResetPurposeIsIsolatedFromRegistrationCodes(t *testing.T) {
	db, svc := newPasswordResetTestService(t)
	user, code := createPasswordResetFixture(t, db, svc, registrationEmailPurpose)
	if err := svc.ConfirmPasswordReset(PasswordResetConfirmRequest{Email: user.Email, EmailCode: code, NewPassword: "new-strong-password"}); err == nil {
		t.Fatal("registration code reset password")
	}
}

func TestPasswordResetStopsAfterFiveWrongCodes(t *testing.T) {
	db, svc := newPasswordResetTestService(t)
	user, code := createPasswordResetFixture(t, db, svc, passwordResetEmailPurpose)
	for attempt := 0; attempt < passwordResetMaxAttempts; attempt++ {
		if err := svc.ConfirmPasswordReset(PasswordResetConfirmRequest{Email: user.Email, EmailCode: "000000", NewPassword: "new-strong-password"}); err == nil {
			t.Fatalf("wrong code attempt %d succeeded", attempt+1)
		}
	}
	if err := svc.ConfirmPasswordReset(PasswordResetConfirmRequest{Email: user.Email, EmailCode: code, NewPassword: "new-strong-password"}); err == nil {
		t.Fatal("correct code succeeded after attempt limit")
	}
	var storedCode model.EmailVerificationCode
	if err := db.First(&storedCode, "email = ? AND purpose = ?", user.Email, passwordResetEmailPurpose).Error; err != nil {
		t.Fatal(err)
	}
	if storedCode.FailedAttempts != passwordResetMaxAttempts || storedCode.UsedAt != nil {
		t.Fatalf("stored code = %#v", storedCode)
	}
}

func TestPasswordResetSettingsAndUnknownEmailDoNotDependOnRegistration(t *testing.T) {
	db, svc := newPasswordResetTestService(t)
	settings, err := svc.PublicAuthSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.RegistrationEnabled || !settings.PasswordResetEnabled {
		t.Fatalf("settings = %#v", settings)
	}
	if err := svc.SendPasswordResetEmailCode("missing@example.com"); err != nil {
		t.Fatal(err)
	}
	var codeCount int64
	if err := db.Model(&model.EmailVerificationCode{}).Where("purpose = ?", passwordResetEmailPurpose).Count(&codeCount).Error; err != nil {
		t.Fatal(err)
	}
	if codeCount != 0 {
		t.Fatalf("unknown email code count = %d, want 0", codeCount)
	}
}

func newPasswordResetTestService(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AuthSession{}, &model.EmailVerificationCode{}, &model.SystemSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SystemSetting{Key: emailSettingKey, ValueJSON: `{"enabled":true,"host":"smtp.example.com","port":587,"encryption":"starttls","fromEmail":"noreply@example.com"}`}).Error; err != nil {
		t.Fatal(err)
	}
	return db, New(repository.New(db), t.TempDir())
}

func createPasswordResetFixture(t *testing.T, db *gorm.DB, svc *Service, purpose string) (model.User, string) {
	t.Helper()
	passwordHash, err := hashPassword("old-strong-password")
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "user-one", Email: "user@example.com", PasswordHash: passwordHash, Role: model.UserRoleUser, Status: model.UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	code := "123456"
	codeHash, err := svc.emailVerificationCodeHashForPurpose(purpose, user.Email, code)
	if err != nil {
		t.Fatal(err)
	}
	verification := model.EmailVerificationCode{
		ID: "code-1", UserID: user.ID, Email: user.Email, CodeHash: codeHash, Purpose: purpose,
		ExpiresAt: time.Now().Add(time.Minute), CreatedAt: time.Now(),
	}
	if err := db.Create(&verification).Error; err != nil {
		t.Fatal(err)
	}
	return user, code
}
