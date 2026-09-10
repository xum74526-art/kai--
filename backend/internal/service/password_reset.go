package service

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/gorm"
)

const (
	passwordResetCodeTTL      = 10 * time.Minute
	passwordResetMaxAttempts  = 5
	passwordResetResendWindow = time.Minute
)

type PasswordResetConfirmRequest struct {
	Email       string `json:"email"`
	EmailCode   string `json:"emailCode"`
	NewPassword string `json:"newPassword"`
}

// SendPasswordResetEmailCode 对不存在、禁用或仅第三方登录的账号统一返回受理，避免暴露账号状态。
func (s *Service) SendPasswordResetEmailCode(rawEmail string) error {
	email := normalizeEmail(rawEmail)
	if err := validateEmail(email); err != nil {
		return err
	}
	_, setting, err := s.readEmailSetting()
	if err != nil {
		return err
	}
	if !setting.Enabled || validateEmailSetting(setting) != nil {
		return Forbidden("平台尚未启用密码找回，请联系管理员")
	}
	user, err := s.repo.UserByEmail(email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if user.Status != model.UserStatusActive || strings.TrimSpace(user.PasswordHash) == "" {
		return nil
	}

	s.emailCodeMu.Lock()
	defer s.emailCodeMu.Unlock()
	if latest, latestErr := s.repo.LatestEmailVerificationCode(email, passwordResetEmailPurpose); latestErr == nil && time.Since(latest.CreatedAt) < passwordResetResendWindow {
		// 与未知邮箱保持相同的受理响应，冷却由服务端静默执行。
		return nil
	} else if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
		return latestErr
	}
	code, err := randomNumericCode(6)
	if err != nil {
		return err
	}
	codeHash, err := s.emailVerificationCodeHashForPurpose(passwordResetEmailPurpose, email, code)
	if err != nil {
		return err
	}
	now := time.Now()
	record := model.EmailVerificationCode{
		ID: newID(), UserID: user.ID, Email: email, CodeHash: codeHash, Purpose: passwordResetEmailPurpose,
		ExpiresAt: now.Add(passwordResetCodeTTL), CreatedAt: now,
	}
	if err := s.repo.CreateEmailVerificationCodeForActiveUser(&record, user.PasswordHash); err != nil {
		if errors.Is(err, repository.ErrUserWriteUnavailable) {
			// 账号状态变化仍保持统一受理响应，不能借由竞态暴露邮箱是否存在。
			return nil
		}
		return err
	}
	if err := sendSMTPMail(setting, email, "北斗 KAI影密码重置验证码", passwordResetEmailBody(code)); err != nil {
		cleanupErr := s.repo.DeleteEmailVerificationCode(record.ID)
		// 对外仍保持统一受理响应；服务端保留可诊断日志，且不记录邮箱或验证码。
		log.Printf("password reset email delivery failed: user_id=%s error_type=%T cleanup_error_type=%T", user.ID, err, cleanupErr)
		return nil
	}
	if cleanupErr := s.repo.DeleteExpiredEmailVerificationCodes(now.Add(-24 * time.Hour)); cleanupErr != nil {
		log.Printf("expired password reset code cleanup failed: error_type=%T", cleanupErr)
	}
	return nil
}

func (s *Service) ConfirmPasswordReset(req PasswordResetConfirmRequest) error {
	email := normalizeEmail(req.Email)
	if err := validateEmail(email); err != nil {
		return err
	}
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}
	emailEnabled, err := s.EmailEnabled()
	if err != nil {
		return err
	}
	if !emailEnabled {
		return Forbidden("平台尚未启用密码找回，请联系管理员")
	}
	code := strings.TrimSpace(req.EmailCode)
	if len(code) != 6 {
		return invalidPasswordResetCode()
	}
	record, err := s.repo.LatestEmailVerificationCode(email, passwordResetEmailPurpose)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return invalidPasswordResetCode()
	}
	if err != nil {
		return err
	}
	now := time.Now()
	if record.UserID == "" || now.After(record.ExpiresAt) || record.FailedAttempts >= passwordResetMaxAttempts {
		return invalidPasswordResetCode()
	}
	user, err := s.repo.User(record.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return invalidPasswordResetCode()
	}
	if err != nil {
		return err
	}
	if user.Status != model.UserStatusActive || normalizeEmail(user.Email) != email || strings.TrimSpace(user.PasswordHash) == "" {
		return invalidPasswordResetCode()
	}
	codeHash, err := s.emailVerificationCodeHashForPurpose(passwordResetEmailPurpose, email, code)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(codeHash), []byte(record.CodeHash)) {
		if err := s.repo.IncrementEmailVerificationCodeFailedAttempts(record.ID, now, passwordResetMaxAttempts); err != nil && !errors.Is(err, repository.ErrEmailVerificationCodeInvalid) {
			return err
		}
		return invalidPasswordResetCode()
	}
	passwordHash, err := hashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.repo.ResetPasswordWithEmailVerification(record.ID, user.ID, email, passwordResetEmailPurpose, user.PasswordHash, passwordHash, now, passwordResetMaxAttempts); err != nil {
		if errors.Is(err, repository.ErrEmailVerificationCodeInvalid) || errors.Is(err, repository.ErrPasswordResetStateChanged) {
			return invalidPasswordResetCode()
		}
		return fmt.Errorf("重置密码：%w", err)
	}
	return nil
}

func invalidPasswordResetCode() error {
	return BadAuthRequest("验证码无效或已过期，请重新获取")
}

func passwordResetEmailBody(code string) string {
	return "你正在重置北斗 KAI影登录密码。\n\n验证码：" + code + "\n\n验证码 10 分钟内有效。若非本人操作，请忽略本邮件。"
}
