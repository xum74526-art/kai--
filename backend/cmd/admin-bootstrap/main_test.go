package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const validPassword = "StrongAdmin#2026"

func TestExecuteCreatesOnlyOneAdminWithBootstrapRecords(t *testing.T) {
	dataDir := t.TempDir()
	lookup := mapLookup(map[string]string{
		"CANVAS_BACKEND_DATA_DIR": dataDir,
		adminUsernameEnv:          "root_admin",
		adminEmailEnv:             "admin@example.com",
		adminDisplayNameEnv:       "主管理员",
	})
	created, err := execute(strings.NewReader(validPassword+"\n"), lookup)
	if err != nil {
		t.Fatalf("execute first bootstrap: %v", err)
	}
	if created.Role != model.UserRoleAdmin || created.Status != model.UserStatusActive {
		t.Fatalf("created admin state = %q/%q", created.Role, created.Status)
	}
	if bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte(validPassword)) != nil {
		t.Fatal("stored password hash does not match input")
	}

	db, err := database.Open(database.Config{Driver: "sqlite", DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var adminCount, accountCount, auditCount int64
	if err := db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&adminCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.CreditAccount{}).Where("user_id = ?", created.ID).Count(&accountCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.AdminAuditEvent{}).Where("action = ? AND target_id = ?", "admin.bootstrap", created.ID).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if adminCount != 1 || accountCount != 1 || auditCount != 1 {
		t.Fatalf("bootstrap counts: admins=%d accounts=%d audits=%d", adminCount, accountCount, auditCount)
	}

	before := *created
	if _, err := execute(strings.NewReader("AnotherStrong#2026\n"), lookup); !errors.Is(err, errAdminExists) {
		t.Fatalf("execute second bootstrap error = %v, want errAdminExists", err)
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Username != before.Username || stored.Email != before.Email || stored.PasswordHash != before.PasswordHash || !stored.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("repeat bootstrap changed the existing administrator")
	}
}

func TestCreateAdminLeavesExistingAdminUntouched(t *testing.T) {
	db := migratedTestDB(t, "existing-admin")
	now := time.Now().UTC().Truncate(time.Second)
	existing := model.User{
		ID: "existing-admin", Username: "existing_admin", Email: "existing@example.com",
		DisplayName: "现有管理员", Role: model.UserRoleAdmin, Status: model.UserStatusActive,
		PasswordHash: "existing-hash", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	_, err := createAdmin(db, validAdminConfig(), now.Add(time.Hour))
	if !errors.Is(err, errAdminExists) {
		t.Fatalf("createAdmin error = %v, want errAdminExists", err)
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", existing.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != existing.PasswordHash || !stored.UpdatedAt.Equal(existing.UpdatedAt) {
		t.Fatal("existing administrator was modified")
	}
}

func TestSingleAdminIndexRejectsDirectSecondAdmin(t *testing.T) {
	db := migratedTestDB(t, "single-admin-command")
	created, err := createAdmin(db, validAdminConfig(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second := model.User{
		ID: "second-admin", Username: "second_admin", Email: "second@example.com",
		Role: model.UserRoleAdmin, Status: model.UserStatusActive,
	}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("database accepted a second administrator")
	}
	var count int64
	if err := db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || created.Role != model.UserRoleAdmin {
		t.Fatalf("admin count = %d", count)
	}
}

func TestLoadAdminConfigValidatesCredentials(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]string
		password string
	}{
		{name: "missing username", values: map[string]string{adminEmailEnv: "admin@example.com"}, password: validPassword},
		{name: "invalid email", values: map[string]string{adminUsernameEnv: "root_admin", adminEmailEnv: "not-an-email"}, password: validPassword},
		{name: "short password", values: map[string]string{adminUsernameEnv: "root_admin", adminEmailEnv: "admin@example.com"}, password: "Short#1"},
		{name: "missing uppercase", values: map[string]string{adminUsernameEnv: "root_admin", adminEmailEnv: "admin@example.com"}, password: "lowercase#2026"},
		{name: "missing special", values: map[string]string{adminUsernameEnv: "root_admin", adminEmailEnv: "admin@example.com"}, password: "StrongAdmin2026"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := loadAdminConfig(strings.NewReader(test.password+"\n"), mapLookup(test.values)); err == nil {
				t.Fatal("loadAdminConfig error = nil")
			}
		})
	}
}

func TestReadPasswordUsesControlledEnvironmentWithoutLoggingOrArguments(t *testing.T) {
	password, err := readPassword(strings.NewReader("ignored\n"), mapLookup(map[string]string{adminPasswordEnv: validPassword}))
	if err != nil {
		t.Fatal(err)
	}
	defer clearBytes(password)
	if string(password) != validPassword {
		t.Fatal("environment password was not used")
	}
	if _, err := readPassword(strings.NewReader("first\nsecond\n"), mapLookup(nil)); err == nil {
		t.Fatal("multiple password lines should be rejected")
	}
}

func TestLoadDatabaseConfigRequiresExplicitSQLiteTarget(t *testing.T) {
	if _, err := loadDatabaseConfig(mapLookup(nil)); err == nil {
		t.Fatal("implicit SQLite target should be rejected")
	}
	config, err := loadDatabaseConfig(mapLookup(map[string]string{"CANVAS_BACKEND_DATA_DIR": "safe-data"}))
	if err != nil {
		t.Fatal(err)
	}
	if config.Driver != "sqlite" || config.DataDir != "safe-data" {
		t.Fatalf("database config = %+v", config)
	}
}

func migratedTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func validAdminConfig() adminConfig {
	return adminConfig{
		Username: "root_admin", Email: "admin@example.com", DisplayName: "主管理员",
		Password: []byte(validPassword),
	}
}

func mapLookup(values map[string]string) envLookup {
	return func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	}
}
