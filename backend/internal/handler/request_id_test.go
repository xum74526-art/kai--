package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddlewareGeneratesServerOwnedID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/ok", func(c *gin.Context) {
		ok(c, gin.H{"requestId": requestID(c)})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ok", nil)
	request.Header.Set(requestIDHeader, "client-controlled-value")
	router.ServeHTTP(recorder, request)

	id := recorder.Header().Get(requestIDHeader)
	if !validServerRequestID(id) {
		t.Fatalf("response request id = %q, want 32 lowercase hex characters", id)
	}
	if id == request.Header.Get(requestIDHeader) {
		t.Fatal("middleware reflected a client-controlled request id")
	}
}

func TestFailureResponseAndHeaderShareRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/failure", func(c *gin.Context) {
		failInternal(c, http.StatusInternalServerError, errors.New("private diagnostic"))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))

	var response failureEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !validServerRequestID(response.RequestID) || response.RequestID != recorder.Header().Get(requestIDHeader) {
		t.Fatalf("body request id = %q, header = %q", response.RequestID, recorder.Header().Get(requestIDHeader))
	}
}
