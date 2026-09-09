package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	readinessOK     = "ok"
	readinessFailed = "failed"
)

// ReadinessReport 只公开组件状态，连接串、存储路径和底层错误不得进入响应。
type ReadinessReport struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Readiness 判断本实例能否安全接收流量并领取后台任务。Redis 只有启用时才检查。
func (s *Service) Readiness(ctx context.Context) (ReadinessReport, error) {
	report := ReadinessReport{
		Status: readinessOK,
		Checks: map[string]string{
			"worker":   readinessOK,
			"database": readinessOK,
			"storage":  readinessOK,
		},
	}
	var readinessErr error
	markFailed := func(component string, err error) {
		report.Status = readinessFailed
		report.Checks[component] = readinessFailed
		readinessErr = errors.Join(readinessErr, fmt.Errorf("%s readiness: %w", component, err))
	}

	if s == nil {
		markFailed("worker", errors.New("service is not initialized"))
		markFailed("database", errors.New("service is not initialized"))
		markFailed("storage", errors.New("service is not initialized"))
		return report, readinessErr
	}
	if !s.lifecycle().isReady() {
		markFailed("worker", errors.New("worker is not accepting work"))
	}
	if err := s.repo.Ping(ctx); err != nil {
		markFailed("database", err)
	}
	if err := checkRequiredLocalStorage(ctx, s.dataDir); err != nil {
		markFailed("storage", err)
	}
	if s.coordinator != nil && s.coordinator.redis != nil {
		report.Checks["redis"] = readinessOK
		if err := s.coordinator.redis.Ping(ctx).Err(); err != nil {
			markFailed("redis", err)
		}
	}
	return report, readinessErr
}

func checkRequiredLocalStorage(ctx context.Context, dataDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("data directory is not a directory")
	}
	probe, err := os.CreateTemp(dataDir, ".readiness-*")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	_, writeErr := probe.Write([]byte{0})
	closeErr := probe.Close()
	removeErr := os.Remove(filepath.Clean(probePath))
	return errors.Join(writeErr, closeErr, removeErr)
}
