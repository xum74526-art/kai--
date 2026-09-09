package repository

import (
	"errors"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DisableUserWithAudit 把停用、会话撤销、未归并文本增量清理和审计写入放在同一事务。
// expected 只保护安全属性，避免管理员并发改角色或状态时基于过期数据完成停用。
func (r *Repository) DisableUserWithAudit(expected *model.User, event *model.AdminAuditEvent, now time.Time) error {
	if expected == nil || event == nil || event.TargetType != "user" || event.TargetID != expected.ID {
		return errors.New("admin user disable records are inconsistent")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", expected.ID)
		if r.Dialect() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var current model.User
		if err := query.First(&current).Error; err != nil {
			return err
		}
		if current.Role != expected.Role || current.Status != expected.Status {
			return ErrUserAdminStateChanged
		}
		if err := tx.Delete(&model.AuthSession{}, "user_id = ?", expected.ID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.TaskTextDelta{}, "user_id = ?", expected.ID).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).
			Where("id = ? AND role = ? AND status = ?", expected.ID, expected.Role, expected.Status).
			Updates(map[string]any{"status": model.UserStatusDisabled, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrUserAdminStateChanged
		}
		return tx.Create(event).Error
	})
}
