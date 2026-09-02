package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ensureTaskSessionScope 在路由和计费前校验请求携带的会话，事务层仍会二次确认。
func (s *Service) ensureTaskSessionScope(userID string, sessionID string, projectID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	session, err := s.repo.SessionForUser(userID, sessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BadAuthRequest("会话不存在或无权访问")
	}
	if err != nil {
		return fmt.Errorf("读取任务会话失败：%w", err)
	}
	if strings.TrimSpace(session.ProjectID) != strings.TrimSpace(projectID) {
		return BadAuthRequest("会话与任务所属项目不一致")
	}
	return nil
}
