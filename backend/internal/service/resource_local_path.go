package service

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// resolveLocalResourcePath 确保数据库中的对象键始终落在本地 resources 根目录内。
// 对象键统一使用斜杠；拒绝非规范或平台相关路径，避免迁移主机后出现不同解释。
func (s *Service) resolveLocalResourcePath(objectKey string) (string, error) {
	key, err := normalizeLocalResourceObjectKey(objectKey)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(filepath.Join(s.dataDir, "resources"))
	if err != nil {
		return "", fmt.Errorf("解析本地资源目录失败：%w", err)
	}
	target := filepath.Join(root, filepath.FromSlash(key))
	if !pathWithinResourceRoot(root, target) {
		return "", errors.New("本地资源路径超出允许目录")
	}
	if err := validateResolvedResourcePath(root, target); err != nil {
		return "", err
	}
	return target, nil
}

func normalizeLocalResourceObjectKey(objectKey string) (string, error) {
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return "", errors.New("本地资源路径为空")
	}
	if key != objectKey {
		return "", errors.New("本地资源路径不合法")
	}
	if strings.Contains(key, "\\") || path.IsAbs(key) || filepath.IsAbs(filepath.FromSlash(key)) || windowsAbsolutePath(key) {
		return "", errors.New("本地资源路径必须是相对路径")
	}
	cleaned := path.Clean(key)
	if cleaned != key || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("本地资源路径不合法")
	}
	return cleaned, nil
}

func windowsAbsolutePath(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	return (value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')
}

func pathWithinResourceRoot(root string, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func validateResolvedResourcePath(root string, target string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		// 根目录不存在时不可能存在可跟随的后代；读取会正常返回不存在，删除视为已完成。
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查本地资源目录失败：%w", err)
	}
	resolvedTarget, err := nearestExistingResolvedPath(target)
	if err != nil {
		return fmt.Errorf("检查本地资源路径失败：%w", err)
	}
	if !pathWithinResourceRoot(resolvedRoot, resolvedTarget) {
		return errors.New("本地资源真实路径超出允许目录")
	}
	return nil
}

func nearestExistingResolvedPath(target string) (string, error) {
	current := target
	for {
		if _, err := os.Lstat(current); err == nil {
			return filepath.EvalSymlinks(current)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
