package handler

import (
	"mime"
	"path"
	"strings"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/service"

	"github.com/gin-gonic/gin"
)

const resourceSandboxPolicy = "sandbox; default-src 'none'; base-uri 'none'; form-action 'none'"

// applyResourceResponseSecurity 保持普通图片、音频和视频内联，同时把文档和
// 可执行格式锁定为附件。CSP 与 nosniff 也保护升级前 MIME 记录不准确的资源。
func applyResourceResponseSecurity(c *gin.Context, resource *model.Resource) {
	c.Header("Content-Security-Policy", resourceSandboxPolicy)
	c.Header("X-Content-Type-Options", "nosniff")
	if service.ResourceCanRenderInline(resource) {
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": resourceDownloadName(resource),
	}))
}

func resourceDownloadName(resource *model.Resource) string {
	name := "download"
	objectKey := ""
	if resource != nil {
		if value := strings.TrimSpace(resource.ID); value != "" {
			name = value
		}
		objectKey = resource.ObjectKey
	}
	extension := strings.ToLower(path.Ext(objectKey))
	if len(extension) > 12 || strings.Trim(extension, ".abcdefghijklmnopqrstuvwxyz0123456789") != "" {
		extension = ""
	}
	return name + extension
}
