package service

import "infinite-canvas/backend/internal/model"

// SystemProviderRequestOwned 是系统渠道异步轮询的归属边界。
// handler 不直接访问数据库，其他轮询协议后续也可复用同一合同。
func (s *Service) SystemProviderRequestOwned(userID string, channelID string, providerRequestID string) (bool, error) {
	return s.repo.SystemProviderRequestOwned(userID, channelID, providerRequestID)
}

// RememberSystemProviderRequestOwnership 先保存轻量归属，再允许调用日志进入配额较大的存储路径。
func (s *Service) RememberSystemProviderRequestOwnership(log model.ApiCallLog) error {
	return s.repo.RememberSystemProviderRequestOwnership(log)
}
