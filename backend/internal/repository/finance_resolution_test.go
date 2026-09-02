package repository

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveBillingOrderKeepsFundsResolutionAndAuditAtomic(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:finance-resolution-atomic?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CreditAccount{}, &model.BillingOrder{}, &model.CreditLedgerEntry{}, &model.AdminAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CreditAccount{UserID: "user-1", AvailableMicrocredits: 50, ReservedMicrocredits: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BillingOrder{
		ID: "order-1", UserID: "user-1", IdempotencyKey: "task:task-1", TaskID: "task-1",
		AmountMicrocredits: 100, ReservedAmountMicrocredits: 100, Status: model.BillingStatusUncertain,
	}).Error; err != nil {
		t.Fatal(err)
	}
	conflict := model.AdminAuditEvent{ID: "audit-conflict", ActorUserID: "admin-1", Action: "existing", TargetType: "user", TargetID: "user-1", CreatedAt: time.Now()}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	duplicateAudit := conflict
	duplicateAudit.Action = "billing.resolve"
	if err := repo.ResolveBillingOrder("order-1", "settle", "admin-1", "人工确认已扣费", "", &duplicateAudit); err == nil {
		t.Fatal("ResolveBillingOrder() error = nil, want audit conflict")
	}
	assertBillingResolutionState(t, db, model.BillingStatusUncertain, "", 50, 100, 0)

	audit := model.AdminAuditEvent{ID: "audit-resolution", ActorUserID: "admin-1", Action: "billing.resolve", TargetType: "user", TargetID: "user-1", CreatedAt: time.Now()}
	if err := repo.ResolveBillingOrder("order-1", "settle", "admin-1", "人工确认已扣费", "", &audit); err != nil {
		t.Fatal(err)
	}
	assertBillingResolutionState(t, db, model.BillingStatusSettled, "admin-1", 50, 0, 1)

	secondAudit := model.AdminAuditEvent{ID: "audit-second", ActorUserID: "admin-2", Action: "billing.resolve", TargetType: "user", TargetID: "user-1", CreatedAt: time.Now()}
	if err := repo.ResolveBillingOrder("order-1", "refund", "admin-2", "重复处理", "", &secondAudit); !errors.Is(err, ErrBillingStateConflict) {
		t.Fatalf("second ResolveBillingOrder() error = %v, want ErrBillingStateConflict", err)
	}
	var secondAuditCount int64
	if err := db.Model(&model.AdminAuditEvent{}).Where("id = ?", secondAudit.ID).Count(&secondAuditCount).Error; err != nil {
		t.Fatal(err)
	}
	if secondAuditCount != 0 {
		t.Fatal("second administrator unexpectedly replaced the resolution")
	}
}

func assertBillingResolutionState(t *testing.T, db *gorm.DB, status model.BillingStatus, resolvedBy string, available int64, reserved int64, terminalLedgerCount int64) {
	t.Helper()
	var order model.BillingOrder
	if err := db.First(&order, "id = ?", "order-1").Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != status || order.ResolvedBy != resolvedBy {
		t.Fatalf("billing order = %#v, want status=%s resolvedBy=%q", order, status, resolvedBy)
	}
	var account model.CreditAccount
	if err := db.First(&account, "user_id = ?", "user-1").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != available || account.ReservedMicrocredits != reserved {
		t.Fatalf("credit account = %#v, want available=%d reserved=%d", account, available, reserved)
	}
	var count int64
	if err := db.Model(&model.CreditLedgerEntry{}).Where("billing_order_id = ?", "order-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != terminalLedgerCount {
		t.Fatalf("terminal ledger count = %d, want %d", count, terminalLedgerCount)
	}
}
