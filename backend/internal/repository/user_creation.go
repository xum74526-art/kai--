package repository

import (
	"errors"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrEmailVerificationCodeInvalid = errors.New("email verification code is no longer valid")

var ErrUserAdminStateChanged = errors.New("user admin state changed")

type AdminUserUpdate struct {
	DisplayName  string
	Email        string
	PasswordHash *string
	Role         model.UserRole
	Status       model.UserStatus
	UpdatedAt    time.Time
}

// CreateRegisteredUser 把验证码消费、用户、积分账户/首登奖励和首次会话放在同一事务中。
// 任一写入失败都会恢复验证码，避免客户端收到失败后留下无法继续注册的半成品账号。
func (r *Repository) CreateRegisteredUser(
	user *model.User,
	verificationCodeID string,
	usedAt time.Time,
	account *model.CreditAccount,
	ledger *model.CreditLedgerEntry,
	session *model.AuthSession,
) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.EmailVerificationCode{}).
			Where("id = ? AND used_at IS NULL AND expires_at > ?", verificationCodeID, usedAt).
			Update("used_at", usedAt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEmailVerificationCodeInvalid
		}
		if err := createUserBootstrapRecords(tx, user, account, ledger); err != nil {
			return err
		}
		if session == nil {
			return errors.New("registration session is required")
		}
		return tx.Create(session).Error
	})
}

// CreateAdminUserWithAudit 保证后台创建账号不会出现“用户已创建但积分账户或审计缺失”的部分成功。
func (r *Repository) CreateAdminUserWithAudit(
	user *model.User,
	account *model.CreditAccount,
	ledger *model.CreditLedgerEntry,
	event *model.AdminAuditEvent,
) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := createUserBootstrapRecords(tx, user, account, ledger); err != nil {
			return err
		}
		if event == nil {
			return errors.New("admin audit event is required")
		}
		return tx.Create(event).Error
	})
}

func createUserBootstrapRecords(tx *gorm.DB, user *model.User, account *model.CreditAccount, ledger *model.CreditLedgerEntry) error {
	if user == nil || account == nil || account.UserID != user.ID {
		return errors.New("user bootstrap records are inconsistent")
	}
	if err := tx.Create(user).Error; err != nil {
		return err
	}
	if err := tx.Create(account).Error; err != nil {
		return err
	}
	if ledger == nil {
		return nil
	}
	if ledger.UserID != user.ID {
		return errors.New("signup ledger owner is inconsistent")
	}
	return tx.Create(ledger).Error
}

// CompleteUserLoginWithSession 把登录状态 CAS 与会话创建放在同一事务中。
// 管理员并发改密或停用账号时，要么先撤销本事务创建的会话，要么让 CAS 失败。
func (r *Repository) CompleteUserLoginWithSession(
	userID string,
	expectedPasswordHash string,
	loggedInAt time.Time,
	session *model.AuthSession,
) (*model.User, error) {
	if session == nil || session.UserID != userID {
		return nil, errors.New("login session owner is inconsistent")
	}
	var user model.User
	err := r.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&model.User{}).
			Where("id = ? AND status = ?", userID, model.UserStatusActive)
		if expectedPasswordHash != "" {
			query = query.Where("password_hash = ?", expectedPasswordHash)
		}
		result := query.Updates(map[string]any{"last_login_at": loggedInAt, "updated_at": loggedInAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrUserLoginStateChanged
		}
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		if user.Status != model.UserStatusActive || (expectedPasswordHash != "" && user.PasswordHash != expectedPasswordHash) {
			return ErrUserLoginStateChanged
		}
		return tx.Create(session).Error
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserWithAudit 只更新管理员允许编辑的字段，并把安全属性变更、会话撤销与审计放在同一事务。
// expected 不比较 LastLoginAt/UpdatedAt，避免正常登录活动让管理员更新产生伪冲突。
func (r *Repository) UpdateUserWithAudit(
	expected *model.User,
	update AdminUserUpdate,
	revokeSessions bool,
	event *model.AdminAuditEvent,
) (*model.User, error) {
	if expected == nil || event == nil || event.TargetID != expected.ID {
		return nil, errors.New("admin user update records are inconsistent")
	}
	var updated model.User
	err := r.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", expected.ID)
		if r.Dialect() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var current model.User
		if err := query.First(&current).Error; err != nil {
			return err
		}
		if current.DisplayName != expected.DisplayName || current.Email != expected.Email ||
			current.PasswordHash != expected.PasswordHash || current.Role != expected.Role || current.Status != expected.Status {
			return ErrUserAdminStateChanged
		}
		changes := map[string]any{
			"display_name": update.DisplayName,
			"email":        update.Email,
			"role":         update.Role,
			"status":       update.Status,
			"updated_at":   update.UpdatedAt,
		}
		if update.PasswordHash != nil {
			changes["password_hash"] = *update.PasswordHash
		}
		result := tx.Model(&model.User{}).Where("id = ?", expected.ID).Updates(changes)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrUserAdminStateChanged
		}
		if revokeSessions {
			if err := tx.Delete(&model.AuthSession{}, "user_id = ?", expected.ID).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		return tx.First(&updated, "id = ?", expected.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}
