package repository

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTokenUsageAmountSettlesArkVideoCompletionTokens(t *testing.T) {
	amount, err := tokenUsageAmount(model.BillingOrder{
		Capability:                   "video",
		OutputTokenPriceMicrocredits: 16_000_000,
		MultiplierBasisPoints:        10_000,
	}, &BillingUsage{OutputTokens: 108900})
	if err != nil {
		t.Fatalf("tokenUsageAmount() error = %v", err)
	}
	if amount != 1_742_400 {
		t.Fatalf("tokenUsageAmount() = %d", amount)
	}
}

func TestBillingUsageReadsAsyncVideoPollUsage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:finance-token-poll?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ApiCallLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ApiCallLog{
		ID: "poll-log-1", BillingOrderID: "order-1", RequestKind: "poll", Billable: false,
		Status: model.ApiCallStatusSucceeded, UsageAvailable: true, OutputTokens: 108900,
	}).Error; err != nil {
		t.Fatal(err)
	}
	usage, err := billingUsage(db, "order-1")
	if err != nil {
		t.Fatalf("billingUsage() error = %v", err)
	}
	if usage.OutputTokens != 108900 {
		t.Fatalf("billingUsage() = %#v", usage)
	}
}

func TestTokenUsageAmountRejectsVideoWithoutOutputUsage(t *testing.T) {
	_, err := tokenUsageAmount(model.BillingOrder{Capability: "video", OutputTokenPriceMicrocredits: 16_000_000, MultiplierBasisPoints: 10_000}, &BillingUsage{})
	if !errors.Is(err, ErrBillingUsageUnavailable) {
		t.Fatalf("tokenUsageAmount() error = %v", err)
	}
}

func TestSettleArkVideoTokenOrderFromPollUsage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:finance-token-settle?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CreditAccount{}, &model.BillingOrder{}, &model.ApiCallLog{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	const reserved = int64(1_916_640)
	if err := db.Create(&model.CreditAccount{UserID: "user-1", ReservedMicrocredits: reserved}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BillingOrder{
		ID: "order-1", UserID: "user-1", IdempotencyKey: "task:task-1", Capability: "video", BillingMode: "token",
		AmountMicrocredits: reserved, ReservedAmountMicrocredits: reserved, OutputTokenPriceMicrocredits: 16_000_000,
		MultiplierBasisPoints: 10_000, Status: model.BillingStatusRunning,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ApiCallLog{
		ID: "poll-log-1", BillingOrderID: "order-1", RequestKind: "poll", Billable: false,
		Status: model.ApiCallStatusSucceeded, UsageAvailable: true, OutputTokens: 108900,
	}).Error; err != nil {
		t.Fatal(err)
	}

	repo := &Repository{db: db}
	if err := repo.SettleBillingOrder("order-1", "ark-task-1"); err != nil {
		t.Fatalf("SettleBillingOrder() error = %v", err)
	}
	if err := repo.SettleBillingOrder("order-1", "ark-task-1"); err != nil {
		t.Fatalf("repeated SettleBillingOrder() error = %v", err)
	}
	var order model.BillingOrder
	if err := db.First(&order, "id = ?", "order-1").Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != model.BillingStatusSettled || order.ActualAmountMicrocredits != 1_742_400 || order.RefundedAmountMicrocredits != 174_240 {
		t.Fatalf("settled order = %#v", order)
	}
	var account model.CreditAccount
	if err := db.First(&account, "user_id = ?", "user-1").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 174_240 || account.ReservedMicrocredits != 0 {
		t.Fatalf("settled account = %#v", account)
	}
	var ledgerCount int64
	if err := db.Model(&model.CreditLedgerEntry{}).Where("billing_order_id = ?", "order-1").Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 2 {
		t.Fatalf("billing ledger count = %d, want consume plus token refund", ledgerCount)
	}
}

func TestRefundBillingOrderReleasesOnlyItsReservationOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:finance-refund-once?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CreditAccount{}, &model.BillingOrder{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CreditAccount{UserID: "user-1", AvailableMicrocredits: 50, ReservedMicrocredits: 320}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BillingOrder{
		ID: "order-1", UserID: "user-1", IdempotencyKey: "task:task-1", AmountMicrocredits: 100,
		ReservedAmountMicrocredits: 120, Status: model.BillingStatusRunning,
	}).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	if err := repo.RefundBillingOrder("order-1", "上游未发出请求"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RefundBillingOrder("order-1", "重复回调"); err != nil {
		t.Fatal(err)
	}
	var account model.CreditAccount
	if err := db.First(&account, "user_id = ?", "user-1").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 170 || account.ReservedMicrocredits != 200 {
		t.Fatalf("refunded account = %#v", account)
	}
	var ledgerCount int64
	if err := db.Model(&model.CreditLedgerEntry{}).Where("billing_order_id = ? AND type = ?", "order-1", model.CreditLedgerRefund).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("refund ledger count = %d, want 1", ledgerCount)
	}
}
