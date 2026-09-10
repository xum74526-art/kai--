package repository

import (
	"errors"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

var ErrPasswordResetStateChanged = errors.New("password reset state changed")

// CreateEmailVerificationCodeForActiveUser 与注销事务锁定同一用户行，避免邮件发送中的晚到写入
// 在账号匿名化完成后重新留下邮箱和验证码摘要。
func (r *Repository) CreateEmailVerificationCodeForActiveUser(record *model.EmailVerificationCode, expectedPasswordHash string) error {
	if record == nil || record.UserID == "" || record.Email == "" {
		return gorm.ErrInvalidData
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := lockActiveUser(tx, record.UserID); err != nil {
			return err
		}
		var user model.User
		if err := tx.Select("id", "email", "password_hash").First(&user, "id = ?", record.UserID).Error; err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(record.Email)) || user.PasswordHash != expectedPasswordHash {
			return ErrUserWriteUnavailable
		}
		return tx.Create(record).Error
	})
}

// IncrementEmailVerificationCodeFailedAttempts 原子累计失败次数，避免并发猜码绕过上限。
func (r *Repository) IncrementEmailVerificationCodeFailedAttempts(id string, now time.Time, maxAttempts int) error {
	result := r.db.Model(&model.EmailVerificationCode{}).
		Where("id = ? AND used_at IS NULL AND expires_at > ? AND failed_attempts < ?", id, now, maxAttempts).
		UpdateColumn("failed_attempts", gorm.Expr("failed_attempts + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrEmailVerificationCodeInvalid
	}
	return nil
}

// ResetPasswordWithEmailVerification 把验证码消费、密码更新和全部会话撤销放在同一事务中。
func (r *Repository) ResetPasswordWithEmailVerification(
	verificationCodeID string,
	userID string,
	email string,
	purpose string,
	expectedPasswordHash string,
	passwordHash string,
	now time.Time,
	maxAttempts int,
) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		codeResult := tx.Model(&model.EmailVerificationCode{}).
			Where("id = ? AND user_id = ? AND email = ? AND purpose = ? AND used_at IS NULL AND expires_at > ? AND failed_attempts < ?",
				verificationCodeID, userID, email, purpose, now, maxAttempts).
			Update("used_at", now)
		if codeResult.Error != nil {
			return codeResult.Error
		}
		if codeResult.RowsAffected != 1 {
			return ErrEmailVerificationCodeInvalid
		}
		userResult := tx.Model(&model.User{}).
			Where("id = ? AND status = ? AND lower(email) = lower(?) AND password_hash = ?", userID, model.UserStatusActive, email, expectedPasswordHash).
			Updates(map[string]any{"password_hash": passwordHash, "updated_at": now})
		if userResult.Error != nil {
			return userResult.Error
		}
		if userResult.RowsAffected != 1 {
			return ErrPasswordResetStateChanged
		}
		if err := tx.Delete(&model.AuthSession{}, "user_id = ?", userID).Error; err != nil {
			return err
		}
		// 旧邮件中的其他验证码必须与旧会话一起失效，避免本次改密后再次被使用。
		return tx.Model(&model.EmailVerificationCode{}).
			Where("user_id = ? AND purpose = ? AND used_at IS NULL", userID, purpose).
			Update("used_at", now).Error
	})
}
