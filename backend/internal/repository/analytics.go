package repository

import (
	"errors"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrProviderRequestOwnershipConflict = errors.New("provider request ownership conflict")

type AnalyticsFilter struct {
	From       time.Time
	To         time.Time
	UserID     string
	Model      string
	ChannelID  string
	Capability string
}

type APICallLogFilter struct {
	AnalyticsFilter
	Keyword string
	Status  string
	IDs     []string
	Page    int
	Limit   int
}

func (r *Repository) RecordUserActivity(userID string, event string, count int, now time.Time) error {
	if userID == "" {
		return nil
	}
	if count <= 0 {
		count = 1
	}
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	activity := model.UserDailyActivity{ID: userID + ":" + day.Format("2006-01-02"), Day: day, UserID: userID, CreatedAt: now, UpdatedAt: now}
	updates := map[string]any{"updated_at": now}
	switch event {
	case "login":
		activity.LoginCount = count
		updates["login_count"] = gorm.Expr("user_daily_activities.login_count + ?", count)
	case "task":
		activity.TaskCount = count
		updates["task_count"] = gorm.Expr("user_daily_activities.task_count + ?", count)
	case "agent_message":
		activity.AgentMessageCount = count
		updates["agent_message_count"] = gorm.Expr("user_daily_activities.agent_message_count + ?", count)
	case "canvas":
		activity.CanvasActive = true
		updates["canvas_active"] = true
	case "asset":
		activity.AssetCount = count
		updates["asset_count"] = gorm.Expr("user_daily_activities.asset_count + ?", count)
	case "resource":
		activity.ResourceCount = count
		updates["resource_count"] = gorm.Expr("user_daily_activities.resource_count + ?", count)
	default:
		return nil
	}
	if event != "login" {
		activity.FirstActiveAt = &now
		activity.LastActiveAt = &now
		updates["first_active_at"] = gorm.Expr("COALESCE(user_daily_activities.first_active_at, ?)", now)
		updates["last_active_at"] = now
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "day"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&activity).Error
}

func (r *Repository) AnalyticsTasks(filter AnalyticsFilter) ([]model.Task, error) {
	var tasks []model.Task
	query := r.db.Select("id", "user_id", "type", "status", "operation", "provider", "model", "started_at", "completed_at", "created_at").Where("created_at >= ? AND created_at < ?", filter.From, filter.To)
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Model != "" {
		query = query.Where("model = ?", filter.Model)
	}
	if filter.Capability != "" {
		if filter.Capability == "text" {
			query = query.Where("type LIKE ? OR type LIKE ? OR type LIKE ?", "%text%", "%storyboard%", "%agent%")
		} else {
			query = query.Where("type LIKE ?", "%"+filter.Capability+"%")
		}
	}
	return tasks, query.Find(&tasks).Error
}

func (r *Repository) AnalyticsAPICallLogs(filter AnalyticsFilter) ([]model.ApiCallLog, error) {
	var logs []model.ApiCallLog
	query := r.apiCallLogQuery(filter)
	return logs, query.Omit("RequestBody", "ResponseBody").Find(&logs).Error
}

func (r *Repository) AnalyticsActivities(filter AnalyticsFilter) ([]model.UserDailyActivity, error) {
	var activities []model.UserDailyActivity
	query := r.db.Where("day >= ? AND day < ?", filter.From, filter.To)
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	return activities, query.Find(&activities).Error
}

func (r *Repository) QueryAPICallLogs(filter APICallLogFilter) ([]model.ApiCallLog, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	var total int64
	// GORM 会把 Distinct 状态保留在同一查询链；计数和分页必须分别构造，避免 PostgreSQL 的 DISTINCT + ORDER BY 冲突。
	if err := r.filteredAPICallLogQuery(filter).Model(&model.ApiCallLog{}).Distinct("api_call_logs.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.ApiCallLog
	err := r.filteredAPICallLogQuery(filter).Omit("RequestBody", "ResponseBody").Order("api_call_logs.created_at desc").Offset((filter.Page - 1) * filter.Limit).Limit(filter.Limit).Find(&logs).Error
	return logs, total, err
}

func (r *Repository) ExportAPICallLogs(filter APICallLogFilter, limit int) ([]model.ApiCallLog, error) {
	if limit <= 0 || limit > 10_000 {
		limit = 10_000
	}
	query := r.filteredAPICallLogQuery(filter)
	if len(filter.IDs) > 0 {
		query = query.Where("api_call_logs.id IN ?", filter.IDs)
	}
	var logs []model.ApiCallLog
	err := query.Omit("RequestBody", "ResponseBody").Order("api_call_logs.created_at desc").Limit(limit).Find(&logs).Error
	return logs, err
}

func (r *Repository) filteredAPICallLogQuery(filter APICallLogFilter) *gorm.DB {
	query := visibleAPICallLogQuery(r.apiCallLogQuery(filter.AnalyticsFilter))
	if value := strings.TrimSpace(filter.Keyword); value != "" {
		pattern := "%" + strings.ToLower(value) + "%"
		query = query.
			Joins("LEFT JOIN users ON users.id = api_call_logs.user_id").
			Joins("LEFT JOIN model_channels ON model_channels.id = api_call_logs.channel_id").
			Where(
				"lower(api_call_logs.user_id) LIKE ? OR lower(users.username) LIKE ? OR lower(users.display_name) LIKE ? OR lower(api_call_logs.channel_id) LIKE ? OR lower(model_channels.name) LIKE ? OR lower(api_call_logs.model) LIKE ? OR lower(api_call_logs.path) LIKE ? OR lower(api_call_logs.provider_request_id) LIKE ?",
				pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern,
			)
	}
	if filter.Status != "" {
		query = query.Where("api_call_logs.status = ?", filter.Status)
	}
	return query
}

func (r *Repository) APICallLogTasks(ids []string) ([]model.Task, error) {
	if len(ids) == 0 {
		return []model.Task{}, nil
	}
	var tasks []model.Task
	err := r.db.Select("id", "user_id", "type", "status", "result_json").Where("id IN ?", ids).Find(&tasks).Error
	return tasks, err
}

func (r *Repository) LatestProviderRequestIDForTask(taskID string, billingOrderID string) (string, error) {
	var log model.ApiCallLog
	query := r.db.Select("provider_request_id").Where("task_id = ? AND provider_request_id <> ''", taskID)
	if strings.TrimSpace(billingOrderID) != "" {
		query = query.Where("billing_order_id = ?", billingOrderID)
	}
	err := query.
		Order("created_at desc").
		First(&log).Error
	return strings.TrimSpace(log.ProviderRequestID), err
}

// RememberSystemProviderRequestOwnership 在返回异步创建结果前固化归属。
// 同一渠道的上游任务 ID 只能属于一个用户，幂等重放只接受原归属。
func (r *Repository) RememberSystemProviderRequestOwnership(log model.ApiCallLog) error {
	if log.Source != "system-channel" || log.RequestKind != "create" || log.Status != model.ApiCallStatusSucceeded {
		return nil
	}
	userID := strings.TrimSpace(log.UserID)
	channelID := strings.TrimSpace(log.ChannelID)
	providerRequestID := strings.TrimSpace(log.ProviderRequestID)
	if userID == "" || channelID == "" || providerRequestID == "" {
		return nil
	}
	item := model.ProviderRequestOwnership{
		ID: newRepositoryID(), UserID: userID, ChannelID: channelID, ProviderRequestID: providerRequestID,
		TaskID: strings.TrimSpace(log.TaskID), BillingOrderID: strings.TrimSpace(log.BillingOrderID), CreatedAt: time.Now(),
	}
	if err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "provider_request_id"}},
		DoNothing: true,
	}).Create(&item).Error; err != nil {
		return err
	}
	var stored model.ProviderRequestOwnership
	if err := r.db.Where("channel_id = ? AND provider_request_id = ?", channelID, providerRequestID).First(&stored).Error; err != nil {
		return err
	}
	if stored.UserID != userID {
		return ErrProviderRequestOwnershipConflict
	}
	return nil
}

// SystemProviderRequestOwned 只认当前用户在当前系统渠道创建过的上游任务。
// 独立归属表服务新请求，账单与调用日志只兼容升级前已经受理的在途任务。
func (r *Repository) SystemProviderRequestOwned(userID string, channelID string, providerRequestID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	providerRequestID = strings.TrimSpace(providerRequestID)
	if userID == "" || channelID == "" || providerRequestID == "" {
		return false, nil
	}
	var ownership model.ProviderRequestOwnership
	err := r.db.Where("channel_id = ? AND provider_request_id = ?", channelID, providerRequestID).First(&ownership).Error
	if err == nil {
		return ownership.UserID == userID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	var count int64
	if err := r.db.Model(&model.BillingOrder{}).
		Where("user_id = ? AND channel_id = ? AND provider_request_id = ?", userID, channelID, providerRequestID).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := r.db.Model(&model.ApiCallLog{}).
		Where("user_id = ? AND channel_id = ? AND provider_request_id = ? AND source = ? AND request_kind = ?", userID, channelID, providerRequestID, "system-channel", "create").
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func visibleAPICallLogQuery(query *gorm.DB) *gorm.DB {
	// 视频轮询属于一次生成调用的内部阶段，管理端只展示聚合后的创建主记录。
	return query.Where("NOT (api_call_logs.capability = ? AND api_call_logs.request_kind IN ?)", "video", []string{"poll", "download"})
}

func (r *Repository) VideoAPICallRoot(log model.ApiCallLog) (*model.ApiCallLog, error) {
	var root model.ApiCallLog
	query := r.db.Where("capability = ? AND request_kind = ?", "video", "create")
	if log.BillingOrderID != "" {
		query = query.Where("billing_order_id = ?", log.BillingOrderID)
		if log.ProviderRequestID != "" {
			query = query.Where("provider_request_id = ?", log.ProviderRequestID)
		}
	} else if log.TaskID != "" {
		query = query.Where("task_id = ?", log.TaskID)
		if log.ProviderRequestID != "" {
			query = query.Where("provider_request_id = ?", log.ProviderRequestID)
		}
	} else {
		query = query.Where("user_id = ? AND channel_id = ? AND provider_request_id = ?", log.UserID, log.ChannelID, log.ProviderRequestID)
	}
	if err := query.Order("created_at desc").First(&root).Error; err != nil {
		return nil, err
	}
	return &root, nil
}

func (r *Repository) apiCallLogQuery(filter AnalyticsFilter) *gorm.DB {
	query := r.db.Where("api_call_logs.created_at >= ? AND api_call_logs.created_at < ?", filter.From, filter.To)
	if filter.UserID != "" {
		query = query.Where("api_call_logs.user_id = ?", filter.UserID)
	}
	if filter.Model != "" {
		query = query.Where("api_call_logs.model = ?", filter.Model)
	}
	if filter.ChannelID != "" {
		query = query.Where("api_call_logs.channel_id = ?", filter.ChannelID)
	}
	if filter.Capability != "" {
		query = query.Where("api_call_logs.capability = ?", filter.Capability)
	}
	return query
}

func (r *Repository) ModelPricings() ([]model.ModelPricing, error) {
	var items []model.ModelPricing
	return items, r.db.Order("model asc, capability asc").Find(&items).Error
}

func (r *Repository) ModelPricing(channelID string, modelName string, capability string) (*model.ModelPricing, error) {
	var pricing model.ModelPricing
	query := r.db.Where("model = ? AND capability = ?", modelName, capability)
	if channelID != "" {
		query = query.Where("channel_id IN ?", []string{channelID, ""}).Order("channel_id desc")
	} else {
		query = query.Where("channel_id = ?", "")
	}
	if err := query.First(&pricing).Error; err != nil {
		return nil, err
	}
	return &pricing, nil
}

func (r *Repository) ModelPricingByID(id string) (*model.ModelPricing, error) {
	var pricing model.ModelPricing
	if err := r.db.First(&pricing, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &pricing, nil
}

func (r *Repository) DeleteModelPricing(id string) error {
	return r.db.Delete(&model.ModelPricing{}, "id = ?", id).Error
}

func (r *Repository) CurrentQueuedTaskCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.Task{}).Where("status IN ?", []model.TaskStatus{model.TaskStatusQueued, model.TaskStatusRunning}).Count(&count).Error
	return count, err
}
