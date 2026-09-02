package service

import (
	"encoding/json"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeTaskInputMakesTypedProviderConfigBillable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.ModelChannel{}, &model.ChannelModel{}); err != nil {
		t.Fatal(err)
	}
	channel := model.ModelChannel{ID: "channel-1", Scope: model.ChannelScopeSystem, Enabled: true}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	channelModel := model.ChannelModel{
		ID: "model-1", ChannelID: "channel-1", ModelKey: "text-model", Capability: "text",
		BillingMode: "fixed_request", UnitPriceMicrocredits: 100_000, PriceConfigured: true, Enabled: true,
	}
	if err := db.Create(&channelModel).Error; err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db)}
	input, err := normalizeTaskInput(map[string]any{
		"mode":   "text",
		"config": providerConfig{ChannelID: "channel-1", Model: "text-model", APIKey: "system"},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := svc.taskBillingOrder("user-1", &model.Task{ID: "task-1", Type: "agent_storyboard"}, input)
	if err != nil {
		t.Fatal(err)
	}
	if order == nil || order.ChannelID != "channel-1" || order.AmountMicrocredits != 100_000 {
		t.Fatalf("taskBillingOrder() = %#v", order)
	}
}

func TestTaskBillingOrderRejectsDisabledSystemChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.ModelChannel{}, &model.ChannelModel{}); err != nil {
		t.Fatal(err)
	}
	channel := model.ModelChannel{ID: "channel-disabled", Scope: model.ChannelScopeSystem, Enabled: false}
	channelModel := model.ChannelModel{
		ID: "model-disabled-parent", ChannelID: channel.ID, ModelKey: "text-model", Capability: "text",
		BillingMode: "fixed_request", UnitPriceMicrocredits: 100_000, PriceConfigured: true, Enabled: true,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&channelModel).Error; err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db)}
	input, err := normalizeTaskInput(map[string]any{
		"mode":   "text",
		"config": providerConfig{ChannelID: channel.ID, Model: channelModel.ModelKey, APIKey: "system"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.taskBillingOrder("user-1", &model.Task{ID: "task-disabled-parent", Type: "agent_storyboard"}, input); err == nil {
		t.Fatal("taskBillingOrder() error = nil, want disabled parent rejection")
	}
}

func TestNormalizeTaskInputStillAllowsSecretProtection(t *testing.T) {
	input, err := normalizeTaskInput(map[string]any{
		"config": providerConfig{BaseURL: "https://example.com", APIKey: "private-key", Model: "text-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	config, ok := input["config"].(map[string]any)
	if !ok || config["apiKey"] != "private-key" {
		t.Fatalf("normalized config = %#v", input["config"])
	}
	svc := &Service{dataDir: t.TempDir()}
	if err := svc.protectTaskSecrets(input); err != nil {
		t.Fatal(err)
	}
	protected, _ := config["apiKey"].(string)
	if protected == "private-key" || !strings.HasPrefix(protected, encryptedSettingPrefix) {
		t.Fatalf("protected apiKey = %q", protected)
	}
}

func TestTaskSecretProtectionEncryptsAndRestoresProviderCredentials(t *testing.T) {
	input, err := normalizeTaskInput(map[string]any{
		"config": providerConfig{
			BaseURL:   "https://example.com",
			APIKey:    "private-api-key",
			SecretKey: "private-secret-key",
			Headers:   []OutboundHeader{{Name: "X-Api-Key", Value: "private-header-key"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{dataDir: t.TempDir()}
	if err := svc.protectTaskSecrets(input); err != nil {
		t.Fatal(err)
	}
	config, _ := input["config"].(map[string]any)
	for _, key := range []string{"apiKey", "secretKey"} {
		value, _ := config[key].(string)
		if !strings.HasPrefix(value, encryptedSettingPrefix) {
			t.Fatalf("protected %s = %q", key, value)
		}
	}
	headers, _ := config["headers"].([]any)
	if len(headers) != 1 {
		t.Fatalf("protected headers = %#v", config["headers"])
	}
	header, _ := headers[0].(map[string]any)
	if value, _ := header["value"].(string); !strings.HasPrefix(value, encryptedSettingPrefix) {
		t.Fatalf("protected header value = %q", value)
	}

	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := svc.decryptTaskInputJSON(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var restored struct {
		Config providerConfig `json:"config"`
	}
	if err := json.Unmarshal([]byte(decrypted), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Config.APIKey != "private-api-key" || restored.Config.SecretKey != "private-secret-key" {
		t.Fatalf("restored provider credentials = %#v", restored.Config)
	}
	if len(restored.Config.Headers) != 1 || restored.Config.Headers[0].Value != "private-header-key" {
		t.Fatalf("restored headers = %#v", restored.Config.Headers)
	}
}

func TestTaskInputRejectsInlineMedia(t *testing.T) {
	input, err := normalizeTaskInput(map[string]any{
		"referenceImages": []providerMedia{{DataURL: testReferenceImageDataURL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsInlineMediaDataURL(input) {
		t.Fatal("containsInlineMediaDataURL() = false")
	}
}

func TestCreateSessionRemovesDraftWhenTaskCreationFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.Asset{}, &model.CanvasProject{}, &model.Session{}, &model.Message{}, &model.Task{}, &model.TaskLog{}, &model.Result{}, &model.ApiCallLog{}); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := db.Create(&model.Task{ID: newID(), UserID: "user-1", Status: model.TaskStatusQueued, Prompt: "queued"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := &Service{repo: repository.New(db), dataDir: t.TempDir()}
	if _, err := svc.CreateSession("user-1", CreateSessionRequest{Prompt: "new session"}); err == nil {
		t.Fatal("CreateSession() error = nil")
	}
	var sessionCount int64
	var messageCount int64
	if err := db.Model(&model.Session{}).Where("user_id = ?", "user-1").Count(&sessionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Message{}).Where("user_id = ?", "user-1").Count(&messageCount).Error; err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 || messageCount != 0 {
		t.Fatalf("draft counts = sessions:%d messages:%d", sessionCount, messageCount)
	}
}
