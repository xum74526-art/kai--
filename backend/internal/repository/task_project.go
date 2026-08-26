package repository

import (
	"errors"
	"strings"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTaskProjectUnavailable         = errors.New("task project is unavailable")
	ErrCanvasParentProjectUnavailable = errors.New("canvas parent project is unavailable")
)

// TaskProjectRequirement 只携带本次请求预检得到的瞬时事实，不写入 Task。
// 已识别过的项目/画布必须在创建事务中仍存在；未知历史 scope 则保留原兼容行为。
type TaskProjectRequirement struct {
	MustExist bool
}

// requireActiveTaskProject 与项目归档、删除共用 Project 主体行锁。
// Task.ProjectID 兼容独立画布 ID 和业务项目 ID；无法识别的历史 scope 保持原行为，不在这里拒绝。
func (r *Repository) requireActiveTaskProject(tx *gorm.DB, task *model.Task, requirement TaskProjectRequirement) error {
	if task == nil {
		return ErrTaskProjectUnavailable
	}
	scopeID := strings.TrimSpace(task.ProjectID)
	if scopeID == "" {
		if requirement.MustExist {
			return ErrTaskProjectUnavailable
		}
		return nil
	}

	var canvas model.CanvasProject
	canvasErr := tx.Select("id", "project_id").First(&canvas, "id = ? AND user_id = ?", scopeID, task.UserID).Error
	if canvasErr == nil {
		if strings.TrimSpace(canvas.ProjectID) == "" {
			return nil
		}
		return r.lockActiveProject(tx, task.UserID, canvas.ProjectID, true)
	}
	if !errors.Is(canvasErr, gorm.ErrRecordNotFound) {
		return canvasErr
	}
	return r.lockActiveProject(tx, task.UserID, scopeID, requirement.MustExist)
}

// lockActiveProject 在 required=true 时把缺失项目视为冲突；画布已明确关联项目后，不能降级为历史未知 scope。
func (r *Repository) lockActiveProject(tx *gorm.DB, userID string, projectID string, required bool) error {
	var project model.Project
	query := tx.Select("id", "status").Where("id = ? AND user_id = ?", projectID, userID)
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if required {
				return ErrTaskProjectUnavailable
			}
			return nil
		}
		return err
	}
	if project.Status != model.ProjectStatusActive {
		return ErrTaskProjectUnavailable
	}
	return nil
}

// lockProjectForDeletion 必须在项目删除事务的第一步调用，与任务创建形成确定的锁顺序。
func (r *Repository) lockProjectForDeletion(tx *gorm.DB, userID string, projectID string) error {
	var project model.Project
	query := tx.Select("id").Where("id = ? AND user_id = ?", projectID, userID)
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return query.First(&project).Error
}

// lockCanvasParentProject 与项目删除共用父项目行锁，确保画布写入和项目删除有确定顺序。
// 这里只验证归属和存在性，不改变已归档项目仍可保存既有画布数据的历史合同。
func (r *Repository) lockCanvasParentProject(tx *gorm.DB, userID string, projectID string) error {
	var project model.Project
	query := tx.Select("id").Where("id = ? AND user_id = ?", projectID, userID)
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCanvasParentProjectUnavailable
		}
		return err
	}
	return nil
}
