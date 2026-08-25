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
