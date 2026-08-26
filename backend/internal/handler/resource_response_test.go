package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/model"

	"github.com/gin-gonic/gin"
)

func TestResourceResponseSecurityKeepsPassiveMediaInline(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	applyResourceResponseSecurity(context, &model.Resource{ID: "image-1", Kind: "image", MimeType: "image/png", ObjectKey: "image.png"})
	if disposition := recorder.Header().Get("Content-Disposition"); disposition != "" {
		t.Fatalf("normal image disposition = %q", disposition)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("missing response hardening headers: %#v", recorder.Header())
	}
}

func TestResourceResponseSecurityForcesActiveContentDownload(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	applyResourceResponseSecurity(context, &model.Resource{ID: "unsafe-1", Kind: "image", MimeType: "text/html", ObjectKey: `users/u/image/payload.html`})
	disposition := recorder.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment;") || !strings.Contains(disposition, "unsafe-1.html") {
		t.Fatalf("unsafe disposition = %q", disposition)
	}
	if policy := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "sandbox") || !strings.Contains(policy, "default-src 'none'") {
		t.Fatalf("unsafe CSP = %q", policy)
	}
}
