package repository

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/model"
)

func newTaskAdmissionSQLiteRepository(t *testing.T) (*Repository, *gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "task-admission.db")
	db := openTaskAdmissionSQLite(t, path, 5000)
	if err := db.AutoMigrate(&model.Task{}, &model.TaskTextDelta{}, &model.CreditAccount{}, &model.BillingOrder{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CreditAccount{UserID: "admission-user", AvailableMicrocredits: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	return New(db), db, path
}

func openTaskAdmissionSQLite(t *testing.T, path string, busyTimeout int) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=%d", path, busyTimeout)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func taskAdmissionOrder(task *model.Task, id string) *model.BillingOrder {
	return &model.BillingOrder{
		ID: id, UserID: task.UserID, TaskID: task.ID, IdempotencyKey: "admission:" + id,
		Status: model.BillingStatusReserved, AmountMicrocredits: 100,
	}
}

func submitTaskAdmission(repo *Repository, task *model.Task, order *model.BillingOrder, retry bool, limit int) error {
	if retry {
		_, err := repo.RetryTaskWithBilling(task.UserID, task, order, limit)
		return err
	}
	if order != nil {
		task.BillingOrderID = order.ID
		return repo.CreateTaskWithCreditReservation(task, order, limit)
	}
	return repo.CreateTaskWithActiveLimit(task, limit)
}

func assertTaskAdmissionBilling(t *testing.T, db *gorm.DB, reservations int64) {
	t.Helper()
	var account model.CreditAccount
	if err := db.First(&account, "user_id = ?", "admission-user").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 1000-reservations*100 || account.ReservedMicrocredits != reservations*100 || account.Version != reservations {
		t.Fatalf("unexpected credit reservation: %#v", account)
	}
	for _, entity := range []any{&model.BillingOrder{}, &model.CreditLedgerEntry{}} {
		var count int64
		if err := db.Model(entity).Where("user_id = ?", "admission-user").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != reservations {
			t.Fatalf("%T count = %d, want %d", entity, count, reservations)
		}
	}
}

func TestTaskAdmissionSQLiteLocksBeforeReading(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paid  bool
		retry bool
	}{
		{name: "free-create"},
		{name: "paid-create", paid: true},
		{name: "free-retry", retry: true},
		{name: "paid-retry", paid: true, retry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, db, path := newTaskAdmissionSQLiteRepository(t)
			task := &model.Task{ID: "task", UserID: "admission-user", Status: model.TaskStatusQueued}
			if tc.retry {
				task.Status = model.TaskStatusFailed
				if err := db.Create(task).Error; err != nil {
					t.Fatal(err)
				}
			}
			peer := openTaskAdmissionSQLite(t, path, 0)
			attempted := false
			var competingError error
			// Without the admission write lock, this intervening WAL commit invalidates
			// the task-count snapshot and the following insert/reservation fails immediately.
			if err := db.Callback().Query().After("gorm:query").Register("test:competing-admission-write", func(tx *gorm.DB) {
				if attempted || tx.Statement.Table != "tasks" {
					return
				}
				attempted = true
				competingError = peer.Model(&model.CreditAccount{}).Where("user_id = ?", task.UserID).
					UpdateColumn("version", gorm.Expr("version + 1")).Error
			}); err != nil {
				t.Fatal(err)
			}
			var order *model.BillingOrder
			var reservations int64
			if tc.paid {
				order = taskAdmissionOrder(task, "order")
				reservations = 1
			}
			if err := submitTaskAdmission(repo, task, order, tc.retry, 2); err != nil {
				t.Fatalf("task admission failed: %v", err)
			}
			var sqliteError sqlite3.Error
			if !attempted || !errors.As(competingError, &sqliteError) || sqliteError.Code != sqlite3.ErrBusy {
				t.Fatalf("competing write attempted = %v, error = %v; want SQLITE_BUSY", attempted, competingError)
			}
			var saved []model.Task
			if err := db.Find(&saved).Error; err != nil {
				t.Fatal(err)
			}
			if len(saved) != 1 || saved[0].Status != model.TaskStatusQueued {
				t.Fatalf("admitted tasks = %#v, want one queued task", saved)
			}
			assertTaskAdmissionBilling(t, db, reservations)
		})
	}
}

func TestTaskAdmissionSQLiteConcurrentCreate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paid  bool
		limit int
	}{
		{name: "free-batch", limit: 2},
		{name: "paid-batch", paid: true, limit: 2},
		{name: "free-limit", limit: 1},
		{name: "paid-limit", paid: true, limit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, db, _ := newTaskAdmissionSQLiteRepository(t)
			start := make(chan struct{})
			results := make(chan error, 2)
			for i := range 2 {
				go func() {
					task := &model.Task{ID: fmt.Sprintf("task-%d", i), UserID: "admission-user", Status: model.TaskStatusQueued}
					var order *model.BillingOrder
					if tc.paid {
						order = taskAdmissionOrder(task, "order-"+task.ID)
					}
					<-start
					results <- submitTaskAdmission(repo, task, order, false, tc.limit)
				}()
			}
			close(start)
			succeeded := 0
			for range 2 {
				err := <-results
				if err == nil {
					succeeded++
				} else if !errors.Is(err, ErrActiveTaskLimit) {
					t.Errorf("unexpected admission error: %v", err)
				}
			}
			var count int64
			if err := db.Model(&model.Task{}).Where("status = ?", model.TaskStatusQueued).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if succeeded != tc.limit || count != int64(tc.limit) {
				t.Fatalf("successful admissions = %d, queued tasks = %d, want %d", succeeded, count, tc.limit)
			}
			var reservations int64
			if tc.paid {
				reservations = int64(tc.limit)
			}
			assertTaskAdmissionBilling(t, db, reservations)
		})
	}
}

func TestTaskAdmissionSQLiteRollsBackReservationOnRejectedWrite(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%t", retry), func(t *testing.T) {
			repo, db, _ := newTaskAdmissionSQLiteRepository(t)
			task := &model.Task{ID: "task", UserID: "admission-user", Status: model.TaskStatusQueued}
			if err := submitTaskAdmission(repo, task, taskAdmissionOrder(task, "original-order"), false, 2); err != nil {
				t.Fatal(err)
			}
			// Use a distinct order key so rejection happens after a second reservation:
			// duplicate task ID on create, or queued state on retry.
			err := submitTaskAdmission(repo, task, taskAdmissionOrder(task, "rejected-order"), retry, 2)
			if err == nil || (retry && !errors.Is(err, ErrTaskNotRetryable)) {
				t.Fatalf("unexpected rejected admission error: %v", err)
			}
			var saved model.Task
			if err := db.First(&saved, "id = ?", task.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.BillingOrderID != "original-order" || saved.Status != model.TaskStatusQueued || saved.RouteRun != 0 {
				t.Fatalf("rejected admission changed existing task: %#v", saved)
			}
			assertTaskAdmissionBilling(t, db, 1)
		})
	}
}

func TestTaskAdmissionSQLiteConcurrentRetryReservesOnce(t *testing.T) {
	repo, db, _ := newTaskAdmissionSQLiteRepository(t)
	task := model.Task{ID: "task", UserID: "admission-user", Status: model.TaskStatusFailed}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := range 2 {
		go func() {
			prepared := task
			order := taskAdmissionOrder(&prepared, fmt.Sprintf("retry-order-%d", i))
			<-start
			results <- submitTaskAdmission(repo, &prepared, order, true, 2)
		}()
	}
	close(start)
	succeeded, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrTaskNotRetryable):
			rejected++
		default:
			t.Errorf("unexpected retry error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("retries succeeded = %d, rejected = %d; want one each", succeeded, rejected)
	}
	var saved model.Task
	if err := db.First(&saved, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != model.TaskStatusQueued || saved.RouteRun != 1 {
		t.Fatalf("retried task = %#v, want queued with route_run = 1", saved)
	}
	assertTaskAdmissionBilling(t, db, 1)
}
