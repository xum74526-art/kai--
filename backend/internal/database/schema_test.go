package database

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestAssetIDColumnsUseSharedLimit(t *testing.T) {
	tests := []struct {
		value any
		field string
	}{
		{value: &model.Asset{}, field: "ID"},
		{value: &model.ProjectAssetLink{}, field: "AssetID"},
		{value: &model.ProjectAssetCandidate{}, field: "ResolvedAssetID"},
		{value: &model.AssetVersion{}, field: "AssetID"},
	}

	for _, test := range tests {
		parsed, err := schema.Parse(test.value, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse schema: %v", err)
		}
		field := parsed.LookUpField(test.field)
		if field == nil {
			t.Fatalf("field %s not found in %s", test.field, parsed.Table)
		}
		if field.Size != model.AssetIDMaxLength {
			t.Fatalf("%s.%s size = %d, want %d", parsed.Table, field.DBName, field.Size, model.AssetIDMaxLength)
		}
	}
}

func TestPostgresAssetIDMigrationsCoverEveryAssetIDColumn(t *testing.T) {
	want := map[string]bool{
		"assets.id":                                  false,
		"project_asset_links.asset_id":               false,
		"project_asset_candidates.resolved_asset_id": false,
		"asset_versions.asset_id":                    false,
	}
	for _, migration := range assetIDColumnMigrations {
		key := migration.table + "." + migration.column
		if _, exists := want[key]; !exists {
			t.Fatalf("unexpected asset ID migration %s", key)
		}
		if !strings.Contains(migration.statement, fmt.Sprintf("varchar(%d)", model.AssetIDMaxLength)) {
			t.Fatalf("asset ID migration %s does not use limit %d", key, model.AssetIDMaxLength)
		}
		want[key] = true
	}
	for key, covered := range want {
		if !covered {
			t.Fatalf("missing asset ID migration %s", key)
		}
	}
}

func TestSingleAdminConstraintPreservesExistingAdminAndRejectsSecond(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:single-admin?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	first := model.User{ID: "admin-1", Username: "admin-one", Role: model.UserRoleAdmin, Status: model.UserStatusActive}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create existing admin: %v", err)
	}
	if err := MigrateSchema(db); err != nil {
		t.Fatalf("migrate schema with existing admin: %v", err)
	}
	if err := db.Create(&model.User{ID: "admin-2", Username: "admin-two", Role: model.UserRoleAdmin, Status: model.UserStatusActive}).Error; err == nil {
		t.Fatal("create second admin error = nil")
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", first.ID).Error; err != nil {
		t.Fatalf("read preserved admin: %v", err)
	}
	if stored.Role != model.UserRoleAdmin {
		t.Fatalf("existing admin role = %q, want %q", stored.Role, model.UserRoleAdmin)
	}
}

func TestSingleAdminConstraintRejectsAmbiguousLegacyData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:duplicate-admins?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	users := []model.User{
		{ID: "admin-1", Username: "admin-one", Role: model.UserRoleAdmin, Status: model.UserStatusActive},
		{ID: "admin-2", Username: "admin-two", Role: model.UserRoleAdmin, Status: model.UserStatusDisabled},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureSingleAdminConstraint(db); err == nil || !strings.Contains(err.Error(), "2 个管理员") {
		t.Fatalf("ensureSingleAdminConstraint() error = %v", err)
	}
}
