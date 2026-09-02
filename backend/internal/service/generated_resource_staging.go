package service

import (
	"errors"
	"fmt"
	"log"
	"time"

	"infinite-canvas/backend/internal/model"
)

const stagedGeneratedResourceCleanupInterval = 5 * time.Minute

// CleanupStagedGeneratedResources 回收进程中断、租约换手或任务失败后仍未提交的生成媒体。
// 只有带 ProducerTaskID 且已失去所属租约的资源才会进入此流程，普通上传不受影响。
func (s *Service) CleanupStagedGeneratedResources() (int, error) {
	resources, err := s.repo.StaleGeneratedTaskResources(time.Now(), 100)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	var cleanupErr error
	for index := range resources {
		resource := &resources[index]
		if err := s.deleteStoredResourceObject(resource.UserID, resource); err != nil {
			resource.Status = model.ResourceStatusFailed
			resource.Error = fmt.Sprintf("暂存生成资源回收失败：%v", err)
			resource.UpdatedAt = time.Now()
			if saveErr := s.repo.SaveResource(resource); saveErr != nil {
				cleanupErr = errors.Join(cleanupErr, err, fmt.Errorf("记录暂存资源回收失败：%w", saveErr))
			} else {
				cleanupErr = errors.Join(cleanupErr, err)
			}
			continue
		}
		if err := s.repo.DeleteResource(resource.UserID, resource.ID); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("删除暂存资源记录失败：%w", err))
			continue
		}
		cleaned++
	}
	return cleaned, cleanupErr
}

func (s *Service) startStagedGeneratedResourceCleanup() {
	cleanup := func() {
		cleaned, err := s.CleanupStagedGeneratedResources()
		if err != nil {
			log.Printf("staged generated resource cleanup failed: %v", err)
			return
		}
		if cleaned > 0 {
			log.Printf("staged generated resource cleanup removed %d resource(s)", cleaned)
		}
	}
	cleanup()
	go func() {
		ticker := time.NewTicker(stagedGeneratedResourceCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cleanup()
		}
	}()
}
