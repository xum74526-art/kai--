package service

import (
	"os"
	"path/filepath"
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestOpenResourceRangeRejectsEscapingLocalObjectKeys(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "resources"), 0o750); err != nil {
		t.Fatal(err)
	}
	service := &Service{dataDir: dataDir}
	for _, objectKey := range []string{
		"../outside.txt",
		filepath.ToSlash(filepath.Join(dataDir, "outside.txt")),
		"C:/Windows/system.ini",
	} {
		t.Run(objectKey, func(t *testing.T) {
			resource := &model.Resource{Status: model.ResourceStatusReady, Provider: "local", ObjectKey: objectKey}
			stream, err := service.openResourceRange("user-1", resource, "")
			if err == nil {
				if stream != nil && stream.Body != nil {
					_ = stream.Body.Close()
				}
				t.Fatalf("openResourceRange(%q) accepted an escaping object key", objectKey)
			}
		})
	}
}

func TestLocalResourceReadAndDeleteRejectSymlinkOutsideRoot(t *testing.T) {
	dataDir := t.TempDir()
	resourceRoot := filepath.Join(dataDir, "resources")
	outsideDir := filepath.Join(dataDir, "outside")
	if err := os.MkdirAll(resourceRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o750); err != nil {
		t.Fatal(err)
	}
	protectedPath := filepath.Join(outsideDir, "protected.txt")
	if err := os.WriteFile(protectedPath, []byte("keep"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(resourceRoot, "escape")); err != nil {
		t.Skipf("当前环境不允许创建符号链接：%v", err)
	}

	service := &Service{dataDir: dataDir}
	resource := &model.Resource{Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "escape/protected.txt"}
	if stream, err := service.openResourceRange("user-1", resource, ""); err == nil {
		if stream != nil && stream.Body != nil {
			_ = stream.Body.Close()
		}
		t.Fatal("openResourceRange accepted a symlink outside the resources root")
	}
	if err := service.deleteLocalResourceObject("escape/protected.txt"); err == nil {
		t.Fatal("deleteLocalResourceObject accepted a symlink outside the resources root")
	}
	if _, err := os.Stat(protectedPath); err != nil {
		t.Fatalf("outside file was changed: %v", err)
	}
}
