package repository

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSaveLogicalModelBundleAllocatesMonotonicRevisionSequence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:logical-model-revision-sequence?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.LogicalModel{}, &model.LogicalModelRevision{}, &model.LogicalModelRoute{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)

	item := &model.LogicalModel{ID: "LMODEL_1", Code: "demo", Name: "Demo", Capability: "text", Enabled: true, PricePolicy: "unified", BillingMode: "fixed_request", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	first := &model.LogicalModelRevision{ID: "REVISION_1", LogicalModelID: item.ID, CapabilitySpecJSON: `{"version":1,"capability":"text"}`, DefaultOptionsJSON: `{}`, CreatedAt: time.Now()}
	if err := repo.SaveLogicalModelBundle(item, first, nil, true); err != nil {
		t.Fatal(err)
	}

	var updated model.LogicalModel
	if err := db.First(&updated, "id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	updated.Name = "Demo 2"
	updated.UpdatedAt = time.Now()
	second := &model.LogicalModelRevision{ID: "REVISION_2", LogicalModelID: item.ID, CapabilitySpecJSON: `{"version":1,"capability":"text"}`, DefaultOptionsJSON: `{}`, CreatedAt: time.Now()}
	if err := repo.SaveLogicalModelBundle(&updated, second, nil, false); err != nil {
		t.Fatal(err)
	}

	var revisions []model.LogicalModelRevision
	if err := db.Order("version asc").Find(&revisions).Error; err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 || revisions[0].Version != 1 || revisions[1].Version != 2 {
		t.Fatalf("revision versions = %#v, want [1 2]", revisions)
	}
	if updated.RevisionSequence != 2 || updated.ActiveRevisionID != "REVISION_2" {
		t.Fatalf("updated model state = %#v", updated)
	}
}

func TestArchiveLogicalModelPreservesPublishedHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:logical-model-delete-history?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.LogicalModel{}, &model.LogicalModelRevision{}, &model.LogicalModelRoute{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)

	item := &model.LogicalModel{ID: "LMODEL_DELETE", Code: "delete-demo", Name: "Delete Demo", Capability: "text", Enabled: true, PricePolicy: "unified", BillingMode: "fixed_request", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	revision := &model.LogicalModelRevision{ID: "REVISION_DELETE", LogicalModelID: item.ID, CapabilitySpecJSON: `{"version":1,"capability":"text"}`, DefaultOptionsJSON: `{}`, CreatedAt: time.Now()}
	routes := []model.LogicalModelRoute{{ID: "ROUTE_DELETE", ChannelModelID: "CMODEL_DELETE", Enabled: true, Weight: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	if err := repo.SaveLogicalModelBundle(item, revision, routes, true); err != nil {
		t.Fatal(err)
	}

	if err := repo.ArchiveLogicalModel(item.ID, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	var archived model.LogicalModel
	if err := db.First(&archived, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("logical model lookup error = %v", err)
	}
	if archived.ArchivedAt == nil || archived.Enabled {
		t.Fatalf("archived logical model = %#v", archived)
	}
	items, err := repo.LogicalModels(true)
	if err != nil {
		t.Fatalf("LogicalModels() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("LogicalModels() = %#v, want archived model hidden", items)
	}
	for label, target := range map[string]any{"revision": &model.LogicalModelRevision{}, "route": &model.LogicalModelRoute{}} {
		if err := db.First(target).Error; err != nil {
			t.Fatalf("%s history was not preserved: %v", label, err)
		}
	}
}

func TestArchiveLogicalModelRejectsActiveTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:logical-model-delete-active-task?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.LogicalModel{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	item := model.LogicalModel{ID: "LMODEL_ACTIVE", Code: "active-demo", Name: "Active Demo", Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Task{ID: "TASK_ACTIVE", LogicalModelID: item.ID, Status: model.TaskStatusRunning, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}

	if err := repo.ArchiveLogicalModel(item.ID, nil, time.Now()); !errors.Is(err, ErrLogicalModelInUse) {
		t.Fatalf("ArchiveLogicalModel() error = %v, want ErrLogicalModelInUse", err)
	}
	var preserved model.LogicalModel
	if err := db.First(&preserved, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("active logical model was removed: %v", err)
	}
	if !preserved.Enabled {
		t.Fatal("active logical model disable was not rolled back")
	}
}

func TestCreateTaskRejectsArchivedLogicalModel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:logical-model-archived-task?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.LogicalModel{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	item := model.LogicalModel{ID: "LMODEL_ARCHIVED_TASK", Code: "archived-task", Name: "Archived Task", Enabled: true, ActiveRevisionID: "REVISION_ARCHIVED_TASK", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.ArchiveLogicalModel(item.ID, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	task := &model.Task{ID: "TASK_ARCHIVED_MODEL", UserID: "USER_1", LogicalModelID: item.ID, LogicalModelRevisionID: item.ActiveRevisionID, Status: model.TaskStatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.CreateTaskWithActiveLimit(task, 5, TaskProjectRequirement{}); !errors.Is(err, ErrLogicalModelUnavailable) {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v, want ErrLogicalModelUnavailable", err)
	}
}

func TestCreateTaskRequiresCompleteLogicalModelRoute(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*gorm.DB, *model.Task) error
	}{
		{
			name: "revision_ownership",
			mutate: func(db *gorm.DB, task *model.Task) error {
				return db.Model(&model.LogicalModelRevision{}).
					Where("id = ?", task.LogicalModelRevisionID).
					Update("logical_model_id", "LMODEL_OTHER").Error
			},
		},
		{
			name: "disabled_route",
			mutate: func(db *gorm.DB, task *model.Task) error {
				return db.Model(&model.LogicalModelRoute{}).Where("id = ?", task.RouteID).Update("enabled", false).Error
			},
		},
		{
			name: "disabled_parent_channel",
			mutate: func(db *gorm.DB, _ *model.Task) error {
				return db.Model(&model.ModelChannel{}).Where("id = ?", "CHANNEL_ADMISSION").Update("enabled", false).Error
			},
		},
		{
			name: "disabled_channel_model",
			mutate: func(db *gorm.DB, task *model.Task) error {
				return db.Model(&model.ChannelModel{}).Where("id = ?", task.ChannelModelID).Update("enabled", false).Error
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, task := newLogicalTaskAdmissionFixture(t, "logical-admission-"+tt.name)
			if err := tt.mutate(db, task); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskWithActiveLimit(task, 5, TaskProjectRequirement{}); !errors.Is(err, ErrLogicalModelUnavailable) {
				t.Fatalf("CreateTaskWithActiveLimit() error = %v, want ErrLogicalModelUnavailable", err)
			}
			var taskCount int64
			if err := db.Model(&model.Task{}).Count(&taskCount).Error; err != nil {
				t.Fatal(err)
			}
			if taskCount != 0 {
				t.Fatalf("task count = %d, want 0", taskCount)
			}
		})
	}
}

func TestCreateTaskAcceptsCompleteLogicalModelRoute(t *testing.T) {
	repo, _, task := newLogicalTaskAdmissionFixture(t, "logical-admission-valid")
	if err := repo.CreateTaskWithActiveLimit(task, 5, TaskProjectRequirement{}); err != nil {
		t.Fatalf("CreateTaskWithActiveLimit() error = %v", err)
	}
}

func TestCreateTaskValidatesLogicalModelBeforeCreditReservation(t *testing.T) {
	repo, db, task := newLogicalTaskAdmissionFixture(t, "logical-admission-credit")
	if err := db.Model(&model.ModelChannel{}).Where("id = ?", "CHANNEL_ADMISSION").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	account := model.CreditAccount{UserID: task.UserID, AvailableMicrocredits: 1_000, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	order := &model.BillingOrder{
		ID: "ORDER_ADMISSION", UserID: task.UserID, IdempotencyKey: "logical-admission-credit", TaskID: task.ID,
		AmountMicrocredits: 100, ReservedAmountMicrocredits: 100, Status: model.BillingStatusReserved,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.CreateTaskWithCreditReservation(task, order, 5, TaskProjectRequirement{}); !errors.Is(err, ErrLogicalModelUnavailable) {
		t.Fatalf("CreateTaskWithCreditReservation() error = %v, want ErrLogicalModelUnavailable", err)
	}
	var preserved model.CreditAccount
	if err := db.First(&preserved, "user_id = ?", task.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if preserved.AvailableMicrocredits != account.AvailableMicrocredits || preserved.ReservedMicrocredits != 0 {
		t.Fatalf("credit account changed before admission: %#v", preserved)
	}
	var orderCount int64
	if err := db.Model(&model.BillingOrder{}).Count(&orderCount).Error; err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 {
		t.Fatalf("billing order count = %d, want 0", orderCount)
	}
	var taskCount int64
	if err := db.Model(&model.Task{}).Count(&taskCount).Error; err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
	var ledgerCount int64
	if err := db.Model(&model.CreditLedgerEntry{}).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 0 {
		t.Fatalf("credit ledger count = %d, want 0", ledgerCount)
	}
}

func TestRetryArchivedLogicalModelRequiresOriginalSnapshot(t *testing.T) {
	t.Run("original snapshot", func(t *testing.T) {
		repo, db, task := newLogicalTaskAdmissionFixture(t, "logical-retry-archived-valid")
		task.Status = model.TaskStatusFailed
		if err := db.Create(task).Error; err != nil {
			t.Fatal(err)
		}
		if err := repo.ArchiveLogicalModel(task.LogicalModelID, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.RetryTaskWithBilling(task.UserID, task, nil, 5, TaskProjectRequirement{}); err != nil {
			t.Fatalf("RetryTaskWithBilling() error = %v", err)
		}
		var queued model.Task
		if err := db.First(&queued, "id = ?", task.ID).Error; err != nil {
			t.Fatal(err)
		}
		if queued.Status != model.TaskStatusQueued || queued.RouteRun != 2 {
			t.Fatalf("retried task = %#v, want queued route run 2", queued)
		}
	})

	t.Run("changed snapshot", func(t *testing.T) {
		repo, db, task := newLogicalTaskAdmissionFixture(t, "logical-retry-archived-changed")
		task.Status = model.TaskStatusFailed
		if err := db.Create(task).Error; err != nil {
			t.Fatal(err)
		}
		if err := repo.ArchiveLogicalModel(task.LogicalModelID, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		prepared := *task
		prepared.RouteID = "ROUTE_MISSING"
		if _, err := repo.RetryTaskWithBilling(task.UserID, &prepared, nil, 5, TaskProjectRequirement{}); !errors.Is(err, ErrLogicalModelUnavailable) {
			t.Fatalf("RetryTaskWithBilling() error = %v, want ErrLogicalModelUnavailable", err)
		}
		var preserved model.Task
		if err := db.First(&preserved, "id = ?", task.ID).Error; err != nil {
			t.Fatal(err)
		}
		if preserved.Status != model.TaskStatusFailed || preserved.RouteRun != task.RouteRun {
			t.Fatalf("rejected retry changed task = %#v", preserved)
		}
	})
}

func newLogicalTaskAdmissionFixture(t *testing.T, databaseName string) (*Repository, *gorm.DB, *model.Task) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+databaseName+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.LogicalModel{},
		&model.LogicalModelRevision{},
		&model.LogicalModelRoute{},
		&model.ModelChannel{},
		&model.ChannelModel{},
		&model.Task{},
		&model.TaskTextDelta{},
		&model.CreditAccount{},
		&model.BillingOrder{},
		&model.CreditLedgerEntry{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	logicalModel := model.LogicalModel{
		ID:               "LMODEL_ADMISSION",
		Code:             "admission",
		Name:             "Admission",
		Capability:       "image",
		Enabled:          true,
		ActiveRevisionID: "REVISION_ADMISSION",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	revision := model.LogicalModelRevision{ID: logicalModel.ActiveRevisionID, LogicalModelID: logicalModel.ID, CreatedAt: now}
	channel := model.ModelChannel{ID: "CHANNEL_ADMISSION", Scope: model.ChannelScopeSystem, Enabled: true, CreatedAt: now, UpdatedAt: now}
	channelModel := model.ChannelModel{ID: "CMODEL_ADMISSION", ChannelID: channel.ID, ModelKey: "image-model", Capability: "image", Enabled: true, CreatedAt: now, UpdatedAt: now}
	route := model.LogicalModelRoute{
		ID:                     "ROUTE_ADMISSION",
		LogicalModelRevisionID: revision.ID,
		ChannelModelID:         channelModel.ID,
		Enabled:                true,
		Weight:                 1,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	for _, item := range []any{&logicalModel, &revision, &channel, &channelModel, &route} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	task := &model.Task{
		ID:                     "TASK_ADMISSION",
		UserID:                 "USER_ADMISSION",
		LogicalModelID:         logicalModel.ID,
		LogicalModelRevisionID: revision.ID,
		RouteID:                route.ID,
		ChannelModelID:         channelModel.ID,
		RouteRun:               1,
		Status:                 model.TaskStatusQueued,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	return New(db), db, task
}
