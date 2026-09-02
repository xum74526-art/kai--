package repository

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSystemProviderRequestOwnedUsesUserAndChannelScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProviderRequestOwnership{}, &model.ApiCallLog{}, &model.BillingOrder{}); err != nil {
		t.Fatal(err)
	}
	log := model.ApiCallLog{ID: "log-1", UserID: "user-1", ChannelID: "channel-1", ProviderRequestID: "provider-1", Source: "system-channel", RequestKind: "create"}
	if err := db.Create(&log).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	owned, err := repo.SystemProviderRequestOwned(log.UserID, log.ChannelID, log.ProviderRequestID)
	if err != nil || !owned {
		t.Fatalf("owned request = %v, error = %v", owned, err)
	}
	for _, scope := range [][3]string{
		{"user-2", log.ChannelID, log.ProviderRequestID},
		{log.UserID, "channel-2", log.ProviderRequestID},
		{log.UserID, log.ChannelID, "provider-2"},
	} {
		owned, err = repo.SystemProviderRequestOwned(scope[0], scope[1], scope[2])
		if err != nil || owned {
			t.Fatalf("foreign scope %v owned = %v, error = %v", scope, owned, err)
		}
	}
}

func TestSystemProviderRequestOwnedFallsBackToBillingOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProviderRequestOwnership{}, &model.ApiCallLog{}, &model.BillingOrder{}); err != nil {
		t.Fatal(err)
	}
	order := model.BillingOrder{ID: "order-1", UserID: "user-1", ChannelID: "channel-1", ProviderRequestID: "provider-1", IdempotencyKey: "request-1"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	owned, err := New(db).SystemProviderRequestOwned(order.UserID, order.ChannelID, order.ProviderRequestID)
	if err != nil || !owned {
		t.Fatalf("billing ownership = %v, error = %v", owned, err)
	}
}

func TestRememberSystemProviderRequestOwnershipIsIdempotentAndRejectsForeignOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProviderRequestOwnership{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	log := model.ApiCallLog{
		UserID: "user-1", ChannelID: "channel-1", ProviderRequestID: "provider-1",
		Source: "system-channel", RequestKind: "create", Status: model.ApiCallStatusSucceeded,
	}
	if err := repo.RememberSystemProviderRequestOwnership(log); err != nil {
		t.Fatal(err)
	}
	if err := repo.RememberSystemProviderRequestOwnership(log); err != nil {
		t.Fatalf("idempotent ownership error = %v", err)
	}
	log.UserID = "user-2"
	if err := repo.RememberSystemProviderRequestOwnership(log); !errors.Is(err, ErrProviderRequestOwnershipConflict) {
		t.Fatalf("foreign ownership error = %v, want ErrProviderRequestOwnershipConflict", err)
	}
	owned, err := repo.SystemProviderRequestOwned(log.UserID, log.ChannelID, log.ProviderRequestID)
	if err != nil || owned {
		t.Fatalf("foreign owner lookup = %v, error = %v", owned, err)
	}
}
