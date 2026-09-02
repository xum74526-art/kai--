package repository

import (
	"errors"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTaskSessionUnavailable     = errors.New("task session is unavailable")
	ErrTaskSessionProjectMismatch = errors.New("task session project does not match")
)

// requireTaskSessionScope 在任务写事务中重新确认会话归属及项目范围。
// 不能只依赖 service 预检，否则删除、换账号或伪造 sessionId 都可能发生在扣费前后。
func (r *Repository) requireTaskSessionScope(tx *gorm.DB, task *model.Task) error {
	if task == nil {
		return ErrTaskSessionUnavailable
	}
	sessionID := strings.TrimSpace(task.SessionID)
	if sessionID == "" {
		return nil
	}
	var session model.Session
	query := tx.Select("id", "project_id").Where("id = ? AND user_id = ?", sessionID, task.UserID)
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskSessionUnavailable
		}
		return err
	}
	if strings.TrimSpace(session.ProjectID) != strings.TrimSpace(task.ProjectID) {
		return ErrTaskSessionProjectMismatch
	}
	return nil
}

// resetTaskSessionForRetry 与任务重新入队、积分预留处于同一事务。
// 这样会话保存失败时不会向客户端返回失败，却留下已经扣费并入队的任务。
func (r *Repository) resetTaskSessionForRetry(tx *gorm.DB, task *model.Task) error {
	if task == nil || strings.TrimSpace(task.SessionID) == "" {
		return nil
	}
	result := tx.Model(&model.Session{}).
		Where("id = ? AND user_id = ?", strings.TrimSpace(task.SessionID), task.UserID).
		Updates(map[string]any{
			"status":          model.SessionStatusActive,
			"canvas_ops_json": "",
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskSessionUnavailable
	}
	return nil
}
