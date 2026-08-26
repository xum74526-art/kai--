package service

import (
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPublicAuthUserIncludesLinuxDOIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserIdentity{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "canvas-user", DisplayName: "Canvas User", Role: model.UserRoleUser, Status: model.UserStatusActive}
	identity := model.UserIdentity{ID: "identity-1", UserID: user.ID, Provider: "linuxdo", Subject: "123456", ProviderUsername: "linux-user", AvatarURL: "https://example.com/avatar.png"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}

	result, err := (&Service{repo: repository.New(db)}).PublicAuthUser(&user)
	if err != nil {
		t.Fatal(err)
	}
	if result.AvatarURL != identity.AvatarURL || result.IdentityProvider != "linuxdo" || result.IdentityID != identity.Subject || result.IdentityUsername != identity.ProviderUsername {
		t.Fatalf("PublicAuthUser() = %#v", result)
	}
}

func TestPublicAuthUserKeepsLocalUserWithoutIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserIdentity{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "local-user", DisplayName: "Local User"}

	result, err := (&Service{repo: repository.New(db)}).PublicAuthUser(&user)
	if err != nil {
		t.Fatal(err)
	}
	if result.Username != user.Username || result.AvatarURL != "" || result.IdentityProvider != "" || result.IdentityID != "" {
		t.Fatalf("PublicAuthUser() = %#v", result)
	}
}

func TestEmptyDatabaseDisablesPublicRegistrationByDefault(t *testing.T) {
	t.Setenv("CANVAS_REGISTRATION_ENABLED", "")
	db := newRegistrationTestDB(t)
	svc := New(repository.New(db), t.TempDir())

	settings, err := svc.PublicAuthSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.FirstUser || settings.RegistrationEnabled {
		t.Fatalf("PublicAuthSettings() = %+v", settings)
	}
	if _, err := svc.Register(RegisterRequest{Username: "first-user", Password: "strong-password"}); err == nil {
		t.Fatal("Register() on empty database error = nil")
	}
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("user count = %d, want 0", count)
	}
}

func TestPublicRegistrationAlwaysCreatesRegularUser(t *testing.T) {
	db := newRegistrationTestDB(t)
	dataDir := t.TempDir()
	svc := New(repository.New(db), dataDir)
	settings := []model.SystemSetting{
		{Key: registrationSettingKey, ValueJSON: `{"enabled":true}`},
		{Key: emailSettingKey, ValueJSON: `{"enabled":true,"host":"smtp.example.com","port":587,"encryption":"starttls","fromEmail":"noreply@example.com"}`},
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	email := "first@example.com"
	code := "123456"
	codeHash, err := svc.emailVerificationCodeHash(email, code)
	if err != nil {
		t.Fatal(err)
	}
	verification := model.EmailVerificationCode{
		ID: "code-1", Email: email, CodeHash: codeHash, Purpose: registrationEmailPurpose,
		ExpiresAt: time.Now().Add(time.Minute), CreatedAt: time.Now(),
	}
	if err := db.Create(&verification).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.Register(RegisterRequest{Username: "first-user", Email: email, EmailCode: code, Password: "strong-password"})
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Role != model.UserRoleUser {
		t.Fatalf("registered role = %q, want %q", result.User.Role, model.UserRoleUser)
	}
	var adminCount int64
	if err := db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&adminCount).Error; err != nil {
		t.Fatal(err)
	}
	if adminCount != 0 {
		t.Fatalf("admin count = %d, want 0", adminCount)
	}
	var accountCount, sessionCount int64
	if err := db.Model(&model.CreditAccount{}).Where("user_id = ?", result.User.ID).Count(&accountCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.AuthSession{}).Where("user_id = ?", result.User.ID).Count(&sessionCount).Error; err != nil {
		t.Fatal(err)
	}
	if accountCount != 1 || sessionCount != 1 {
		t.Fatalf("registration bootstrap counts: account=%d session=%d", accountCount, sessionCount)
	}
	var storedCode model.EmailVerificationCode
	if err := db.First(&storedCode, "id = ?", verification.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedCode.UsedAt == nil {
		t.Fatal("registration verification code was not consumed")
	}
}

func TestEnablingRegistrationRequiresUsableEmailSetting(t *testing.T) {
	db := newRegistrationTestDB(t)
	svc := New(repository.New(db), t.TempDir())
	admin := &model.User{ID: "admin-1", Role: model.UserRoleAdmin}

	if _, err := svc.UpdateRegistrationSetting(admin, RegistrationSettingRequest{Enabled: true}); err == nil {
		t.Fatal("UpdateRegistrationSetting() error = nil")
	}
	var registrationRows int64
	if err := db.Model(&model.SystemSetting{}).Where("key = ?", registrationSettingKey).Count(&registrationRows).Error; err != nil {
		t.Fatal(err)
	}
	if registrationRows != 0 {
		t.Fatalf("registration setting rows = %d, want 0", registrationRows)
	}
	enabled, err := svc.RegistrationEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("registration is enabled without SMTP")
	}
}

func TestDisablingEmailRequiresClosingRegistrationFirst(t *testing.T) {
	db := newRegistrationTestDB(t)
	settings := []model.SystemSetting{
		{Key: registrationSettingKey, ValueJSON: `{"enabled":true}`},
		{Key: emailSettingKey, ValueJSON: `{"enabled":true,"host":"smtp.example.com","port":587,"encryption":"starttls","fromEmail":"noreply@example.com"}`},
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(db), t.TempDir())
	admin := &model.User{ID: "admin-1", Role: model.UserRoleAdmin}

	if _, err := svc.UpdateEmailSetting(admin, EmailSettingRequest{Enabled: false}); err == nil {
		t.Fatal("UpdateEmailSetting() error = nil")
	}
	emailEnabled, err := svc.EmailEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !emailEnabled {
		t.Fatal("SMTP was disabled while public registration remained enabled")
	}
}

func TestLegacyRegistrationFlagKeepsOAuthOpenButPasswordRegistrationClosed(t *testing.T) {
	db := newRegistrationTestDB(t)
	if err := db.Create(&model.SystemSetting{Key: registrationSettingKey, ValueJSON: `{"enabled":true}`}).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(db), t.TempDir())

	enabled, err := svc.RegistrationEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("global new-user registration was coupled to SMTP readiness")
	}
	passwordEnabled, err := svc.PasswordRegistrationEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if passwordEnabled {
		t.Fatal("password registration bypassed SMTP readiness")
	}
	settings, err := svc.PublicAuthSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.RegistrationEnabled || settings.EmailEnabled {
		t.Fatalf("PublicAuthSettings() = %+v, want global registration open and password email unavailable", settings)
	}
	if _, err := svc.Register(RegisterRequest{Username: "password-user", Email: "user@example.com", EmailCode: "123456", Password: "strong-password"}); err == nil {
		t.Fatal("Register() without SMTP error = nil")
	}
}

func newRegistrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.UserIdentity{}, &model.AuthSession{}, &model.EmailVerificationCode{},
		&model.SystemSetting{}, &model.CreditAccount{}, &model.CreditLedgerEntry{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}
