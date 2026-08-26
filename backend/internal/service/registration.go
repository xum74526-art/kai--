package service

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

const registrationSettingKey = "registration"

type RegistrationSettingRequest struct {
	Enabled bool `json:"enabled"`
}

type PublicRegistrationSetting struct {
	Enabled   bool      `json:"enabled"`
	UpdatedBy string    `json:"updatedBy"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type registrationSettingValue struct {
	Enabled bool `json:"enabled"`
}

func (s *Service) AdminRegistrationSetting(actor *model.User) (*PublicRegistrationSetting, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	setting, value, err := s.readRegistrationSetting()
	if err != nil {
		return nil, err
	}
	return publicRegistrationSetting(setting, value), nil
}

func (s *Service) UpdateRegistrationSetting(actor *model.User, req RegistrationSettingRequest) (*PublicRegistrationSetting, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	s.registrationMu.Lock()
	defer s.registrationMu.Unlock()
	if req.Enabled {
		emailEnabled, err := s.EmailEnabled()
		if err != nil {
			return nil, err
		}
		if !emailEnabled {
			return nil, BadAuthRequest("开放公开注册前，请先启用并完整配置 SMTP 注册邮件")
		}
	}
	encoded, err := json.Marshal(registrationSettingValue{Enabled: req.Enabled})
	if err != nil {
		return nil, err
	}
	current, _, err := s.readRegistrationSetting()
	if err != nil {
		return nil, err
	}
	setting := model.SystemSetting{Key: registrationSettingKey, ValueJSON: string(encoded), UpdatedBy: actor.ID}
	if current != nil {
		setting.CreatedAt = current.CreatedAt
	}
	if err := s.repo.SaveSystemSetting(&setting); err != nil {
		return nil, err
	}
	return publicRegistrationSetting(&setting, registrationSettingValue{Enabled: req.Enabled}), nil
}

func (s *Service) RegistrationEnabled() (bool, error) {
	_, value, err := s.readRegistrationSetting()
	return value.Enabled, err
}

// PasswordRegistrationEnabled 在全局新用户准入之上叠加本地邮箱验证码能力。
// Linux.do 已由上游完成身份验证，不应因本地 SMTP 临时不可用而拒绝首次登录。
func (s *Service) PasswordRegistrationEnabled() (bool, error) {
	registrationEnabled, err := s.RegistrationEnabled()
	if err != nil || !registrationEnabled {
		return false, err
	}
	emailEnabled, err := s.EmailEnabled()
	if err != nil {
		return false, err
	}
	// 环境变量或旧数据可能绕过后台保存校验；SMTP 不可用时本地密码注册保持关闭。
	return emailEnabled, nil
}

func (s *Service) readRegistrationSetting() (*model.SystemSetting, registrationSettingValue, error) {
	setting, err := s.repo.SystemSetting(registrationSettingKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, registrationSettingValue{Enabled: registrationEnabledFromEnvironment()}, nil
	}
	if err != nil {
		return nil, registrationSettingValue{}, err
	}
	value := registrationSettingValue{}
	if strings.TrimSpace(setting.ValueJSON) == "" || json.Unmarshal([]byte(setting.ValueJSON), &value) != nil {
		return nil, registrationSettingValue{}, errors.New("用户注册配置格式无效")
	}
	return setting, value, nil
}

func publicRegistrationSetting(setting *model.SystemSetting, value registrationSettingValue) *PublicRegistrationSetting {
	result := &PublicRegistrationSetting{Enabled: value.Enabled}
	if setting != nil {
		result.UpdatedBy = setting.UpdatedBy
		result.CreatedAt = setting.CreatedAt
		result.UpdatedAt = setting.UpdatedAt
	}
	return result
}

func registrationEnabledFromEnvironment() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CANVAS_REGISTRATION_ENABLED")))
	return value == "1" || value == "true" || value == "yes"
}
