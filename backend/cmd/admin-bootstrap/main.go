package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	adminUsernameEnv    = "CANVAS_ADMIN_USERNAME"
	adminEmailEnv       = "CANVAS_ADMIN_EMAIL"
	adminDisplayNameEnv = "CANVAS_ADMIN_DISPLAY_NAME"
	adminPasswordEnv    = "CANVAS_ADMIN_PASSWORD"
)

var (
	errAdminExists  = errors.New("数据库中已经存在管理员，拒绝重复初始化")
	usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)
)

type envLookup func(string) (string, bool)

type adminConfig struct {
	Username    string
	Email       string
	DisplayName string
	Password    []byte
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 1 {
		log.Fatal("管理员初始化不接受命令行参数；账号信息使用受控环境变量，密码使用标准输入或 CANVAS_ADMIN_PASSWORD")
	}
	user, err := execute(os.Stdin, processEnvLookup)
	if err != nil {
		log.Fatalf("管理员初始化失败：%v", err)
	}
	fmt.Printf("管理员初始化完成：username=%s email=%s\n", user.Username, user.Email)
}

func processEnvLookup(key string) (string, bool) {
	value, exists := os.LookupEnv(key)
	if key == adminPasswordEnv && exists {
		// 环境变量适合容器 Secret 注入，但仍可能被同权限进程观察；读取后立即从当前进程环境移除。
		_ = os.Unsetenv(key)
	}
	return value, exists
}

func execute(input io.Reader, lookup envLookup) (*model.User, error) {
	config, err := loadAdminConfig(input, lookup)
	if err != nil {
		return nil, err
	}
	defer clearBytes(config.Password)

	databaseConfig, err := loadDatabaseConfig(lookup)
	if err != nil {
		return nil, err
	}
	db, err := database.Open(databaseConfig)
	if err != nil {
		return nil, fmt.Errorf("连接数据库：%w", err)
	}
	// 初始化失败时不记录包含密码哈希的 SQL 参数；命令只输出经过收敛的错误。
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if err := database.ConfigurePool(db); err != nil {
		return nil, fmt.Errorf("配置数据库连接池：%w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接：%w", err)
	}
	defer sqlDB.Close()

	exists, err := adminExists(db)
	if err != nil {
		return nil, err
	}
	if exists {
		// 已有管理员时必须在迁移前退出，保证重复执行不会修改现有数据库。
		return nil, errAdminExists
	}
	if err := database.MigrateSchema(db); err != nil {
		return nil, fmt.Errorf("迁移数据库结构：%w", err)
	}
	return createAdmin(db, config, time.Now())
}

func loadDatabaseConfig(lookup envLookup) (database.Config, error) {
	driver, _ := lookup("CANVAS_DATABASE_DRIVER")
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		driver = "sqlite"
	}
	dsn, _ := lookup("DATABASE_URL")
	dsn = strings.TrimSpace(dsn)
	dataDir, _ := lookup("CANVAS_BACKEND_DATA_DIR")
	dataDir = strings.TrimSpace(dataDir)
	if driver == "sqlite" && dsn == "" && dataDir == "" {
		return database.Config{}, errors.New("SQLite 初始化必须显式配置 CANVAS_BACKEND_DATA_DIR，拒绝在默认目录创建新数据库")
	}
	if (driver == "postgres" || driver == "postgresql") && dsn == "" {
		return database.Config{}, errors.New("PostgreSQL 初始化必须配置 DATABASE_URL")
	}
	return database.Config{Driver: driver, DSN: dsn, DataDir: dataDir}, nil
}

func loadAdminConfig(input io.Reader, lookup envLookup) (adminConfig, error) {
	username, _ := lookup(adminUsernameEnv)
	email, _ := lookup(adminEmailEnv)
	displayName, _ := lookup(adminDisplayNameEnv)
	password, err := readPassword(input, lookup)
	if err != nil {
		return adminConfig{}, err
	}
	config := adminConfig{
		Username:    strings.TrimSpace(username),
		Email:       strings.ToLower(strings.TrimSpace(email)),
		DisplayName: strings.TrimSpace(displayName),
		Password:    password,
	}
	if config.DisplayName == "" {
		config.DisplayName = config.Username
	}
	if err := validateAdminConfig(config); err != nil {
		clearBytes(password)
		return adminConfig{}, err
	}
	return config, nil
}

func readPassword(input io.Reader, lookup envLookup) ([]byte, error) {
	if value, exists := lookup(adminPasswordEnv); exists {
		return []byte(value), nil
	}
	if input == nil {
		return nil, errors.New("未配置管理员密码；请通过标准输入或 CANVAS_ADMIN_PASSWORD 提供")
	}
	scanner := bufio.NewScanner(io.LimitReader(input, 4097))
	scanner.Buffer(make([]byte, 128), 4096)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("读取管理员密码：%w", err)
		}
		return nil, errors.New("标准输入中没有管理员密码")
	}
	password := bytes.TrimSuffix(scanner.Bytes(), []byte{'\r'})
	result := append([]byte(nil), password...)
	if scanner.Scan() {
		clearBytes(result)
		return nil, errors.New("标准输入只能包含一行管理员密码")
	}
	if err := scanner.Err(); err != nil {
		clearBytes(result)
		return nil, fmt.Errorf("读取管理员密码：%w", err)
	}
	return result, nil
}

func validateAdminConfig(config adminConfig) error {
	if !usernamePattern.MatchString(config.Username) {
		return errors.New("管理员用户名需为 3-32 位字母、数字、下划线或连字符")
	}
	if config.Email == "" {
		return errors.New("管理员邮箱不能为空")
	}
	if len(config.Email) > 160 {
		return errors.New("管理员邮箱不能超过 160 个字符")
	}
	address, err := mail.ParseAddress(config.Email)
	if err != nil || address.Name != "" || !strings.EqualFold(address.Address, config.Email) {
		return errors.New("管理员邮箱格式不正确")
	}
	if config.DisplayName == "" || len([]rune(config.DisplayName)) > 40 || containsControl(config.DisplayName) {
		return errors.New("管理员显示名称需为 1-40 个可见字符")
	}
	return validateAdminPassword(config.Password)
}

func validateAdminPassword(password []byte) error {
	if !utf8.Valid(password) {
		return errors.New("管理员密码必须是有效的 UTF-8 文本")
	}
	if len(password) > 72 {
		return errors.New("管理员密码不能超过 72 个字节")
	}
	value := string(password)
	if value != strings.TrimSpace(value) {
		return errors.New("管理员密码首尾不能包含空白字符")
	}
	if len([]rune(value)) < 12 {
		return errors.New("管理员密码至少 12 位")
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, value := range value {
		switch {
		case unicode.IsUpper(value):
			hasUpper = true
		case unicode.IsLower(value):
			hasLower = true
		case unicode.IsDigit(value):
			hasDigit = true
		case unicode.IsControl(value):
			return errors.New("管理员密码不能包含控制字符")
		case unicode.IsPunct(value) || unicode.IsSymbol(value):
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return errors.New("管理员密码必须同时包含大写字母、小写字母、数字和特殊字符")
	}
	return nil
}

func adminExists(db *gorm.DB) (bool, error) {
	if !db.Migrator().HasTable(&model.User{}) {
		return false, nil
	}
	var count int64
	if err := db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查现有管理员：%w", err)
	}
	return count > 0, nil
}

func createAdmin(db *gorm.DB, config adminConfig, now time.Time) (*model.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword(config.Password, bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("生成管理员密码哈希：%w", err)
	}
	userID, err := secureID()
	if err != nil {
		return nil, err
	}
	auditID, err := secureID()
	if err != nil {
		return nil, err
	}
	user := model.User{
		ID:           userID,
		Username:     config.Username,
		Email:        config.Email,
		DisplayName:  config.DisplayName,
		Role:         model.UserRoleAdmin,
		Status:       model.UserStatusActive,
		PasswordHash: string(passwordHash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	account := model.CreditAccount{UserID: user.ID, CreatedAt: now, UpdatedAt: now}
	audit := model.AdminAuditEvent{
		ID: auditID, ActorUserID: user.ID, Action: "admin.bootstrap",
		TargetType: "user", TargetID: user.ID, Summary: "受控命令初始化唯一管理员",
		MetadataJSON: `{"source":"admin-bootstrap"}`, CreatedAt: now,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var adminCount int64
		if err := tx.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&adminCount).Error; err != nil {
			return err
		}
		if adminCount != 0 {
			return errAdminExists
		}
		var usernameCount int64
		if err := tx.Model(&model.User{}).Where("username = ?", user.Username).Count(&usernameCount).Error; err != nil {
			return err
		}
		if usernameCount != 0 {
			return fmt.Errorf("用户名 %q 已存在", user.Username)
		}
		var emailCount int64
		if err := tx.Model(&model.User{}).Where("lower(email) = ?", user.Email).Count(&emailCount).Error; err != nil {
			return err
		}
		if emailCount != 0 {
			return errors.New("管理员邮箱已被其他账号使用")
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		return tx.Create(&audit).Error
	})
	if err != nil {
		// 两个初始化进程并发时由数据库唯一索引决定胜者，失败进程统一按重复初始化处理。
		if exists, checkErr := adminExists(db); checkErr == nil && exists {
			return nil, errAdminExists
		}
		return nil, fmt.Errorf("创建管理员：%w", err)
	}
	return &user, nil
}

func secureID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("生成安全标识：%w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func containsControl(value string) bool {
	for _, current := range value {
		if unicode.IsControl(current) {
			return true
		}
	}
	return false
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
