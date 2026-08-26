package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCollectOwnedAssetDocumentReferences(t *testing.T) {
	resources := map[string]struct{}{}
	tasks := map[string]struct{}{}
	raw := `{
		"data":{"storageKey":"resource:resource-1","url":"/api/resources/resource-2/file"},
		"metadata":{"taskId":"task-1","referenceResourceIds":["resource-3"],"errorDetails":"failed near /api/resources/not-a-reference/file"}
	}`
	if err := collectOwnedAssetDocumentReferences(raw, resources, tasks); err != nil {
		t.Fatal(err)
	}
	for _, resourceID := range []string{"resource-1", "resource-2", "resource-3"} {
		if _, exists := resources[resourceID]; !exists {
			t.Fatalf("resource %q was not collected: %#v", resourceID, resources)
		}
	}
	if _, exists := tasks["task-1"]; !exists {
		t.Fatalf("origin task was not collected: %#v", tasks)
	}
	if _, exists := resources["not-a-reference"]; exists {
		t.Fatalf("diagnostic text must not be treated as an actual resource reference: %#v", resources)
	}
	persistedIDs, err := referencedResourceIDs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(persistedIDs, ","), "not-a-reference") {
		t.Fatalf("persisted reference extraction accepted diagnostic text: %#v", persistedIDs)
	}
}

func TestDocumentReferencesResourcesUsesExactResourceID(t *testing.T) {
	candidates := map[string]struct{}{"resource-1": {}}
	if documentReferencesResources(`{"storageKey":"resource:resource-10"}`, candidates) {
		t.Fatal("resource-1 must not match resource-10")
	}
	if !documentReferencesResources(`{"storageKey":"resource:resource-1"}`, candidates) {
		t.Fatal("exact resource ID should match")
	}
}

func TestResourceOccupiedMessageNamesBusinessRecord(t *testing.T) {
	message := resourceOccupiedMessage([]resourceUsage{
		{Kind: "画布", ID: "canvas-1", Title: "广告分镜"},
		{Kind: "画布", ID: "canvas-1", Title: "广告分镜"},
	})
	if !strings.Contains(message, "画布「广告分镜」") || !strings.Contains(message, "解除引用") {
		t.Fatalf("message = %q", message)
	}
}

func TestDeleteLocalResourceObjectRemovesOnlyResourceDirectoryFile(t *testing.T) {
	dataDir := t.TempDir()
	resourcePath := filepath.Join(dataDir, "resources", "users", "user-1", "image", "asset.png")
	if err := os.MkdirAll(filepath.Dir(resourcePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resourcePath, []byte("image"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := &Service{dataDir: dataDir}
	if err := service.deleteLocalResourceObject("users/user-1/image/asset.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resourcePath); !os.IsNotExist(err) {
		t.Fatalf("resource file still exists or stat failed unexpectedly: %v", err)
	}

	outsidePath := filepath.Join(dataDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("keep"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := service.deleteLocalResourceObject("../outside.txt"); err == nil {
		t.Fatal("path traversal should be rejected")
	}
	if _, err := os.Stat(outsidePath); err != nil {
		t.Fatalf("outside file was changed: %v", err)
	}
}

func TestDeleteAssetPersistsRetryableTombstoneBeforePhysicalDeletion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(database.Models()...); err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db), dataDir: t.TempDir()}
	resource := model.Resource{
		ID: "resource-1", UserID: "user-1", Kind: "image", Status: model.ResourceStatusReady,
		Provider: "unsupported", ObjectKey: "users/user-1/image/missing.png",
	}
	asset := model.Asset{
		ID: "asset-1", UserID: "user-1", Kind: "image", Status: model.AssetVersionStatusConfirmed,
		Title: "产品图", PayloadJSON: `{"storageKey":"resource:resource-1"}`,
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.deleteUserAssetWithResources(asset.UserID, asset.ID); err == nil || !strings.Contains(err.Error(), "可重试") {
		t.Fatalf("deleteUserAssetWithResources() error = %v, want retryable failure", err)
	}
	var storedResource model.Resource
	if err := db.First(&storedResource, "id = ?", resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedResource.Status != model.ResourceStatusDeleted {
		t.Fatalf("resource status = %q, want deletion tombstone", storedResource.Status)
	}
	if err := db.First(&model.Asset{}, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("asset should remain while physical cleanup is pending: %v", err)
	}
	visible, err := svc.repo.Resources(asset.UserID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("deleted resources should be hidden from normal listing: %+v", visible)
	}

	storedResource.Provider = "local"
	if err := db.Save(&storedResource).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.deleteUserAssetWithResources(asset.UserID, asset.ID); err != nil {
		t.Fatalf("retry deleteUserAssetWithResources() error = %v", err)
	}
	if err := db.First(&model.Asset{}, "id = ?", asset.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("asset lookup after completed deletion = %v", err)
	}
	if err := db.First(&model.Resource{}, "id = ?", resource.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("resource lookup after completed deletion = %v", err)
	}
}

func TestResourceReferenceWriteAndDeletionUseSameReadyLockProtocol(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(database.Models()...); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	resource := model.Resource{ID: "resource-lock-1", UserID: "user-1", Kind: "image", Status: model.ResourceStatusReady, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	asset := model.Asset{ID: "asset-lock-1", UserID: "user-1", Kind: "image", Status: model.AssetVersionStatusConfirmed, Title: "待删除素材", PayloadJSON: `{"storageKey":"resource:resource-lock-1"}`}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}

	// 模拟删除方已完成旧快照检查后，引用写入方先取得资源锁并提交。
	initial, err := repo.ResourceReferenceSnapshot(asset.UserID, asset.ID, []string{resource.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Documents) != 0 || len(initial.Direct) != 0 {
		t.Fatalf("initial snapshot unexpectedly occupied: %+v", initial)
	}
	canvas := model.CanvasProject{
		ID: "canvas-lock-1", UserID: asset.UserID, Title: "新引用", PayloadJSON: `{"storageKey":"resource:resource-lock-1"}`,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.UpsertCanvasProject(&canvas, []string{resource.ID}); err != nil {
		t.Fatalf("ready reference write failed: %v", err)
	}

	owned := map[string]struct{}{resource.ID: {}}
	err = repo.PrepareAssetResourceDeletion(asset.UserID, asset.ID, []string{resource.ID}, func(snapshot repository.ResourceReferenceSnapshot) error {
		if message := resourceOccupiedMessage(resourceUsagesFromSnapshot(snapshot, owned, map[string]struct{}{})); message != "" {
			return BadAuthRequest(message)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "引用") {
		t.Fatalf("final deletion snapshot error = %v, want occupied reference", err)
	}
	var stored model.Resource
	if err := db.First(&stored, "id = ?", resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.ResourceStatusReady {
		t.Fatalf("resource status after rejected deletion = %q, want ready", stored.Status)
	}

	if err := db.Delete(&model.CanvasProject{}, "id = ? AND user_id = ?", canvas.ID, canvas.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.PrepareAssetResourceDeletion(asset.UserID, asset.ID, []string{resource.ID}, nil); err != nil {
		t.Fatalf("prepare deletion after releasing reference: %v", err)
	}
	blockedCanvas := model.CanvasProject{
		ID: "canvas-lock-2", UserID: asset.UserID, Title: "迟到引用", PayloadJSON: `{"storageKey":"resource:resource-lock-1"}`,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.UpsertCanvasProject(&blockedCanvas, []string{resource.ID}); !errors.Is(err, repository.ErrResourceReferenceUnavailable) {
		t.Fatalf("reference after tombstone error = %v, want ErrResourceReferenceUnavailable", err)
	}
	var count int64
	if err := db.Model(&model.CanvasProject{}).Where("id = ?", blockedCanvas.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("blocked reference write persisted %d rows", count)
	}
	task := model.Task{
		ID: "task-lock-1", UserID: asset.UserID, Type: "canvas_image", Status: model.TaskStatusQueued,
		Prompt: "引用正在删除的素材", InputJSON: `{"referenceResourceId":"resource-lock-1"}`, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.CreateTaskWithActiveLimit(&task, 5, repository.TaskProjectRequirement{}, []string{resource.ID}); !errors.Is(err, repository.ErrResourceReferenceUnavailable) {
		t.Fatalf("task reference after tombstone error = %v, want ErrResourceReferenceUnavailable", err)
	}
	if err := db.Model(&model.Task{}).Where("id = ?", task.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("blocked task reference persisted %d rows", count)
	}
}
