package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/handler"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	dataDir := env("CANVAS_BACKEND_DATA_DIR", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(database.Config{
		Driver:  env("CANVAS_DATABASE_DRIVER", "sqlite"),
		DSN:     os.Getenv("DATABASE_URL"),
		DataDir: dataDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := database.ConfigurePool(db); err != nil {
		log.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		log.Fatal(err)
	}

	repo := repository.New(db)
	addr := env("CANVAS_BACKEND_ADDR", ":8080")
	capabilities := service.RuntimeCapabilitiesForDeployment(addr, os.Getenv("CANVAS_DESKTOP_LOCAL_CHANNELS_ENABLED"))
	svc := service.NewWithRuntimeCapabilities(repo, dataDir, capabilities)
	if err := svc.ValidateRuntime(); err != nil {
		log.Fatal(err)
	}
	if err := svc.EnsureSystemChannelModels(); err != nil {
		log.Fatal(err)
	}
	if err := svc.EnsureDefaultPromptTemplates(); err != nil {
		log.Fatal(err)
	}
	if err := svc.EnsureBuiltinProjectWorkflowTemplate(); err != nil {
		log.Fatal(err)
	}
	if err := svc.EnsureBuiltinSkills(); err != nil {
		log.Fatal(err)
	}
	if summary, err := svc.MigrateLegacyStorage(); err != nil {
		log.Printf("storage migration skipped after error: %v", err)
	} else if summary.Backup != "" {
		log.Printf("storage migration completed: tasks=%d assets=%d projects=%d backup=%s", summary.Tasks, summary.Assets, summary.Projects, summary.Backup)
	}
	r := gin.New()
	if err := configureTrustedProxies(r, os.Getenv("CANVAS_TRUSTED_PROXIES")); err != nil {
		log.Fatal(err)
	}
	r.Use(handler.RequestIDMiddleware())
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s\" %d %s %s\n", param.ClientIP, param.TimeStamp.Format(time.RFC3339), param.Method, redactCanvasSharePath(param.Path), param.StatusCode, param.Latency, param.ErrorMessage)
	}), gin.Recovery())
	corsMiddleware, err := cors()
	if err != nil {
		log.Fatal(err)
	}
	r.Use(corsMiddleware)
	handler.ConfigureRuntime(svc)
	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 0, "data": gin.H{"status": "ok"}, "msg": "ok"})
	})
	api.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		report, err := svc.Readiness(ctx)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "data": report, "msg": "服务尚未就绪", "requestId": c.Writer.Header().Get("X-Request-ID")})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": report, "msg": "ok"})
	})
	handler.RegisterOAuthCallbackRoutes(r, svc)
	handler.RegisterAuthRoutes(api, svc)
	handler.RegisterFeatureAvailabilityRoutes(api, svc)
	handler.RegisterResponseInterceptionRoutes(api, svc)
	handler.RegisterAdminRoutes(api, svc)
	handler.RegisterAdminAnalyticsRoutes(api, svc)
	handler.RegisterAnnouncementRoutes(api, svc)
	handler.RegisterFinanceRoutes(api, svc)
	handler.RegisterLibTVRoutes(api, svc)
	handler.RegisterTapNowRoutes(api, svc)
	// 登录态模型目录代理：避免浏览器直连各上游时分别处理 CORS。
	handler.RegisterChannelModelRoutes(api, svc)
	handler.RegisterLogicalModelRoutes(api, svc)
	handler.RegisterSystemProxyRoutes(api, svc)
	handler.RegisterCustomRelayRoutes(api, svc)
	handler.RegisterTaskRoutes(api, svc)
	handler.RegisterSessionRoutes(api, svc)
	handler.RegisterSkillRoutes(api, svc)
	handler.RegisterUserDataRoutes(api, svc)
	handler.RegisterPluginRoutes(api, svc)
	projectAPI := api.Group("")
	projectAPI.Use(handler.RequireFeature(svc, service.FeatureShortDrama))
	handler.RegisterProjectRoutes(projectAPI, svc)
	handler.RegisterCanvasShareRoutes(api, svc)

	shutdownTimeout, err := durationEnv("CANVAS_SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	// 所有可能失败的启动配置必须先完成校验，避免配置错误时 worker 已经
	// 领取并标记任务，随后进程又被 log.Fatal 直接终止。
	svc.StartWorker()
	log.Printf("KAI影 backend listening on %s", addr)
	if err := serveUntilShutdown(server, svc, shutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

func configureTrustedProxies(r *gin.Engine, raw string) error {
	proxies := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		// 不能用全网段恢复 Gin 的旧默认行为；只允许部署者填写实际反向代理
		// 的 IP/CIDR，避免客户端伪造 X-Forwarded-For 绕过登录与注册限流。
		if value == "*" || value == "0.0.0.0/0" || value == "::/0" {
			return fmt.Errorf("CANVAS_TRUSTED_PROXIES must contain exact proxy IPs or narrow CIDRs, got %q", value)
		}
		proxies = append(proxies, value)
	}
	if err := r.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("CANVAS_TRUSTED_PROXIES is invalid: %w", err)
	}
	return nil
}

func redactCanvasSharePath(path string) string {
	const prefix = "/api/public/canvas-shares/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	remainder := strings.TrimPrefix(path, prefix)
	if index := strings.IndexByte(remainder, '/'); index >= 0 {
		return prefix + ":token" + remainder[index:]
	}
	return prefix + ":token"
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", key, value)
	}
	return duration, nil
}

const corsAllowedHeaders = "Accept, Content-Type, Authorization, X-Requested-With, X-Canvas-Scene, X-Idempotency-Key, X-Canvas-Upstream-URL, X-Canvas-Upstream-Format, X-Canvas-Upstream-Headers, X-Canvas-Allow-Local-Channel, X-Canvas-Upstream-Base-URL"

const corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"

type corsPolicy struct {
	origins map[string]struct{}
}

func cors() (gin.HandlerFunc, error) {
	policy, err := parseCORSPolicy(os.Getenv("CANVAS_CORS_ORIGINS"))
	if err != nil {
		return nil, err
	}
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" && credentialedUnsafeRequest(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "data": nil, "msg": "登录态写请求必须携带有效来源"})
			return
		}
		if origin != "" && !allowedOriginWithPolicy(c, origin, policy) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "data": nil, "msg": "不允许的跨域来源"})
			return
		}
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
		}
		c.Header("Access-Control-Allow-Headers", corsAllowedHeaders)
		c.Header("Access-Control-Allow-Methods", corsAllowedMethods)
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}, nil
}

func allowedOrigin(c *gin.Context, origin string) bool {
	policy, err := parseCORSPolicy(os.Getenv("CANVAS_CORS_ORIGINS"))
	if err != nil {
		return false
	}
	return allowedOriginWithPolicy(c, origin, policy)
}

func parseCORSPolicy(raw string) (corsPolicy, error) {
	policy := corsPolicy{origins: make(map[string]struct{})}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if value == "*" {
			return corsPolicy{}, fmt.Errorf("CANVAS_CORS_ORIGINS cannot use * when credentialed requests are enabled")
		}
		normalized, err := normalizeCORSOrigin(value)
		if err != nil {
			return corsPolicy{}, fmt.Errorf("CANVAS_CORS_ORIGINS contains invalid origin %q: %w", value, err)
		}
		policy.origins[normalized] = struct{}{}
	}
	return policy, nil
}

func normalizeCORSOrigin(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("origin is empty")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("origin must be an http or https origin")
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin must not contain a path, query, or fragment")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func allowedOriginWithPolicy(c *gin.Context, origin string, policy corsPolicy) bool {
	normalizedOrigin, err := normalizeCORSOrigin(origin)
	if err != nil {
		return false
	}
	if _, ok := policy.origins[normalizedOrigin]; ok {
		return true
	}
	parsed, err := url.Parse(normalizedOrigin)
	if err != nil {
		return false
	}
	// 反向代理应把外部 Host 写入 Request.Host。协议优先读取真实 TLS；代理场景只信任
	// 部署者配置的公网地址，不读取客户端可伪造的 X-Forwarded-Proto/Host。
	requestOrigin, ok := requestOriginForCORS(c)
	if ok && normalizedOrigin == requestOrigin {
		return true
	}
	if len(policy.origins) > 0 {
		return false
	}
	// 未显式配置 CORS 时只给纯本地开发兜底；公网 API 不能因为访问者的页面
	// 来自 localhost 就信任该来源。
	originHost := strings.ToLower(parsed.Hostname())
	requestHost := ""
	if c != nil && c.Request != nil {
		requestURL := &url.URL{Scheme: "http", Host: strings.TrimSpace(c.Request.Host)}
		requestHost = strings.ToLower(requestURL.Hostname())
	}
	return localCORSHost(originHost) && localCORSHost(requestHost)
}

func localCORSHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func credentialedUnsafeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	value, err := c.Cookie(service.SessionCookieName)
	return err == nil && strings.TrimSpace(value) != ""
}

func requestOriginForCORS(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil || strings.TrimSpace(c.Request.Host) == "" {
		return "", false
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if configured, err := url.Parse(strings.TrimSpace(os.Getenv("CANVAS_PUBLIC_BASE_URL"))); err == nil && configured != nil && (configured.Scheme == "http" || configured.Scheme == "https") && strings.EqualFold(configured.Host, strings.TrimSpace(c.Request.Host)) {
		scheme = strings.ToLower(configured.Scheme)
	}
	normalized, err := normalizeCORSOrigin(scheme + "://" + strings.TrimSpace(c.Request.Host))
	return normalized, err == nil
}
