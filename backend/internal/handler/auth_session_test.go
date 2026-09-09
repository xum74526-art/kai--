package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthSessionOnlyTreatsUnauthorizedAsSignedOut(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuthSession{}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(db), t.TempDir())
	router := gin.New()
	router.Use(RequestIDMiddleware())
	RegisterAuthRoutes(router.Group("/api"), svc)

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/auth/session", nil))
	if anonymous.Code != http.StatusOK {
		t.Fatalf("anonymous status = %d, body = %s", anonymous.Code, anonymous.Body.String())
	}
	var anonymousBody struct {
		Code int `json:"code"`
		Data struct {
			User *model.User `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(anonymous.Body.Bytes(), &anonymousBody); err != nil {
		t.Fatal(err)
	}
	if anonymousBody.Code != 0 || anonymousBody.Data.User != nil {
		t.Fatalf("anonymous response = %#v", anonymousBody)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	failed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: "session-id.session-token"})
	router.ServeHTTP(failed, request)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("database failure status = %d, body = %s", failed.Code, failed.Body.String())
	}
	response := decodeFailureEnvelope(t, failed)
	if response.Msg != internalErrorMessage || !validServerRequestID(response.RequestID) {
		t.Fatalf("database failure response = %#v", response)
	}
}
