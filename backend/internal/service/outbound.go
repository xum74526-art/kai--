package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpproxy"
)

const (
	maxOutboundRedirects      = 5
	maxOutboundHeaderCount    = 32
	maxOutboundHeaderBytes    = 16 << 10
	outboundValidationTimeout = 10 * time.Second
	outboundDNSFreshTTL       = 30 * time.Minute
	outboundDNSStaleTTL       = 6 * time.Hour
	outboundDNSLookupTimeout  = 3 * time.Second
	outboundDNSRetryDelay     = 150 * time.Millisecond
	outboundDNSLookupAttempts = 2
	CustomRelayHeadersHeader  = "X-Canvas-Upstream-Headers"
	DefaultOutboundUserAgent  = "Beidou-KAIYing/1.0 (+https://github.com/xum74526-art/kai--)"
)

type OutboundHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var (
	outboundTransport          = newOutboundTransport(resolveOutboundHost)
	directOutboundTransport    = newDirectOutboundTransport(resolveOutboundHost)
	customRelayTransport       = newDirectOutboundTransport(resolveCustomRelayHost)
	publicResourceTransport    = newDirectOutboundTransport(resolvePublicOutboundHost)
	sharedOutboundDNSCache     = newOutboundDNSCache()
	blockedCustomRelayPrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
)

type outboundDNSCacheEntry struct {
	addresses  []net.IP
	resolvedAt time.Time
}

// outboundDNSCache 只缓存解析结果，调用方仍需逐次执行私网和保留地址校验。
// 每个主机的解析锁避免并发生成任务在 DNS 抖动时同时放大失败。
type outboundDNSCache struct {
	mu      sync.RWMutex
	entries map[string]outboundDNSCacheEntry
	locks   map[string]chan struct{}
}

func newOutboundDNSCache() *outboundDNSCache {
	return &outboundDNSCache{
		entries: make(map[string]outboundDNSCacheEntry),
		locks:   make(map[string]chan struct{}),
	}
}

func cloneIPAddresses(addresses []net.IP) []net.IP {
	cloned := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address != nil {
			cloned = append(cloned, append(net.IP(nil), address...))
		}
	}
	return cloned
}

func (c *outboundDNSCache) load(host string, maxAge time.Duration) ([]net.IP, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[host]
	c.mu.RUnlock()
	if !ok || len(entry.addresses) == 0 || time.Since(entry.resolvedAt) > maxAge {
		return nil, false
	}
	return cloneIPAddresses(entry.addresses), true
}

func (c *outboundDNSCache) store(host string, addresses []net.IP) {
	if c == nil || len(addresses) == 0 {
		return
	}
	c.mu.Lock()
	c.entries[host] = outboundDNSCacheEntry{addresses: cloneIPAddresses(addresses), resolvedAt: time.Now()}
	c.mu.Unlock()
}

func (c *outboundDNSCache) acquire(ctx context.Context, host string) (func(), error) {
	if c == nil {
		return func() {}, nil
	}
	c.mu.Lock()
	lock := c.locks[host]
	if lock == nil {
		lock = make(chan struct{}, 1)
		c.locks[host] = lock
	}
	c.mu.Unlock()
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func ValidateOutboundURL(rawURL string) (*url.URL, error) {
	if len(strings.TrimSpace(rawURL)) > 4096 {
		return nil, BadAuthRequest("外部服务地址过长")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" || !parsed.IsAbs() {
		return nil, BadAuthRequest("外部服务地址无效")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, BadAuthRequest("外部服务地址只支持 http/https")
	}
	if parsed.User != nil {
		return nil, BadAuthRequest("外部服务地址不允许包含认证信息")
	}
	if parsed.Fragment != "" {
		return nil, BadAuthRequest("外部服务地址不允许包含片段")
	}
	if err := validateOutboundHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

// 用户自定义渠道必须使用更严格的出口策略：不接受 URL 凭据，且仅部署者精确配置的
// 主机可以路由到私网或使用会明文传输 API Key 的 HTTP。
func ValidateCustomRelayURL(rawURL string) (*url.URL, error) {
	if len(strings.TrimSpace(rawURL)) > 4096 {
		return nil, BadAuthRequest("自定义渠道地址过长")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" || !parsed.IsAbs() {
		return nil, BadAuthRequest("自定义渠道地址无效")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return nil, BadAuthRequest("自定义渠道地址只支持 http/https")
	}
	if scheme == "http" && !allowedPrivateUpstreamHost(parsed.Hostname()) {
		return nil, BadAuthRequest("自定义渠道 HTTP 仅允许访问已配置的可信上游主机")
	}
	if parsed.User != nil {
		return nil, BadAuthRequest("自定义渠道地址不允许包含认证信息")
	}
	if parsed.Fragment != "" {
		return nil, BadAuthRequest("自定义渠道地址不允许包含片段")
	}
	if err := validateCustomRelayHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

// ValidatePublicResourceURL 用于用户可提交的远程素材地址。它永不继承模型上游的
// 私网放行配置，避免运维为内部模型开白名单后意外开放资源导入 SSRF。
func ValidatePublicResourceURL(rawURL string) (*url.URL, error) {
	if len(strings.TrimSpace(rawURL)) > 4096 {
		return nil, BadAuthRequest("远程资源地址过长")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" || !parsed.IsAbs() {
		return nil, BadAuthRequest("远程资源地址无效")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, BadAuthRequest("远程资源地址只支持 http/https")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, BadAuthRequest("远程资源地址不允许包含认证信息或片段")
	}
	ctx, cancel := context.WithTimeout(context.Background(), outboundValidationTimeout)
	defer cancel()
	if _, err := resolvePublicOutboundHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func OutboundHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: outboundTransport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxOutboundRedirects {
				return errors.New("外部服务重定向次数过多")
			}
			_, err := ValidateOutboundURL(req.URL.String())
			return err
		},
	}
}

// DirectOutboundHTTPClient 用于用户可控地址和供应商返回的跨域资源地址。
// 它保留与系统上游相同的 SSRF 白名单，但禁止环境代理在代理端重新解析目标域名。
func DirectOutboundHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: directOutboundTransport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxOutboundRedirects {
				return errors.New("外部服务重定向次数过多")
			}
			_, err := ValidateOutboundURL(req.URL.String())
			return err
		},
	}
}

func CustomRelayHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: customRelayTransport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("自定义渠道中转不允许重定向")
		},
	}
}

func PublicResourceHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: publicResourceTransport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxOutboundRedirects {
				return errors.New("远程资源重定向次数过多")
			}
			_, err := ValidatePublicResourceURL(req.URL.String())
			return err
		},
	}
}

// ApplyDefaultOutboundHeaders 为所有服务端出网请求提供可识别的产品标识，
// 避免 Go 默认 User-Agent 被部分 WAF 或中转网关拦截。
func ApplyDefaultOutboundHeaders(req *http.Request) {
	if req != nil && strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
		req.Header.Set("User-Agent", DefaultOutboundUserAgent)
	}
}

// NormalizeOutboundHeaders 是自定义 Header 的安全边界：允许业务/WAF 校验头和 User-Agent，
// 但禁止覆盖鉴权、请求体协议、代理内部头及 hop-by-hop 头。
func NormalizeOutboundHeaders(headers []OutboundHeader) ([]OutboundHeader, error) {
	if len(headers) > maxOutboundHeaderCount {
		return nil, BadAuthRequest(fmt.Sprintf("自定义请求头最多支持 %d 项", maxOutboundHeaderCount))
	}
	seen := make(map[string]struct{}, len(headers))
	normalized := make([]OutboundHeader, 0, len(headers))
	totalBytes := 0
	for _, item := range headers {
		name := strings.TrimSpace(item.Name)
		value := strings.TrimSpace(item.Value)
		if name == "" && value == "" {
			continue
		}
		if name == "" || !validOutboundHeaderName(name) {
			return nil, BadAuthRequest("自定义请求头名称无效，请使用标准 HTTP Header 名称")
		}
		name = http.CanonicalHeaderKey(name)
		lowerName := strings.ToLower(name)
		if blockedOutboundHeader(lowerName) {
			return nil, BadAuthRequest("请求头 " + name + " 由系统管理，不允许自定义")
		}
		if _, exists := seen[lowerName]; exists {
			return nil, BadAuthRequest("自定义请求头不能重复：" + name)
		}
		if !validOutboundHeaderValue(value) {
			return nil, BadAuthRequest("请求头 " + name + " 的值包含非法控制字符")
		}
		if len(name) > 128 || len(value) > 4096 {
			return nil, BadAuthRequest("单个自定义请求头名称或值过长")
		}
		totalBytes += len(name) + len(value)
		if totalBytes > maxOutboundHeaderBytes {
			return nil, BadAuthRequest("自定义请求头总大小不能超过 16KB")
		}
		seen[lowerName] = struct{}{}
		normalized = append(normalized, OutboundHeader{Name: name, Value: value})
	}
	return normalized, nil
}

func ApplyOutboundHeaders(req *http.Request, headers []OutboundHeader) {
	if req == nil {
		return
	}
	for _, item := range headers {
		req.Header.Set(item.Name, item.Value)
	}
}

func EncodeOutboundHeadersJSON(headers []OutboundHeader) (string, error) {
	normalized, err := NormalizeOutboundHeaders(headers)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	return string(data), err
}

func ParseOutboundHeadersJSON(raw string) ([]OutboundHeader, error) {
	if strings.TrimSpace(raw) == "" {
		return []OutboundHeader{}, nil
	}
	var headers []OutboundHeader
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil, errors.New("渠道自定义请求头配置损坏")
	}
	return NormalizeOutboundHeaders(headers)
}

func DecodeRelayOutboundHeaders(encoded string) ([]OutboundHeader, error) {
	if strings.TrimSpace(encoded) == "" {
		return []OutboundHeader{}, nil
	}
	if len(encoded) > (maxOutboundHeaderBytes*2)+1024 {
		return nil, BadAuthRequest("自定义请求头配置过大")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, BadAuthRequest("自定义请求头编码无效")
	}
	var headers []OutboundHeader
	if err := json.Unmarshal(data, &headers); err != nil {
		return nil, BadAuthRequest("自定义请求头格式无效")
	}
	return NormalizeOutboundHeaders(headers)
}

func validOutboundHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for index := 0; index < len(name); index++ {
		char := name[index]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			continue
		}
		return false
	}
	return true
}

func validOutboundHeaderValue(value string) bool {
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '\t' || char >= 32 && char != 127 {
			continue
		}
		return false
	}
	return true
}

func blockedOutboundHeader(name string) bool {
	switch name {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "host", "content-length", "content-type", "accept", "connection", "proxy-connection", "keep-alive", "transfer-encoding", "te", "trailer", "upgrade", "forwarded", "x-goog-api-key":
		return true
	}
	return strings.HasPrefix(name, "x-canvas-") || strings.HasPrefix(name, "x-forwarded-")
}

func newOutboundTransport(resolveHost func(context.Context, string) ([]net.IP, error)) *http.Transport {
	return newOutboundTransportWithProxy(resolveHost, true)
}

func newDirectOutboundTransport(resolveHost func(context.Context, string) ([]net.IP, error)) *http.Transport {
	// 用户可控地址必须由本进程完成解析和 IP 钉扎；环境代理会在代理端重新解析目标。
	return newOutboundTransportWithProxy(resolveHost, false)
}

func newOutboundTransportWithProxy(resolveHost func(context.Context, string) ([]net.IP, error), allowEnvironmentProxy bool) *http.Transport {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	configureOutboundDialer(dialer)
	var proxy func(*http.Request) (*url.URL, error)
	if allowEnvironmentProxy {
		proxy = outboundProxyFromEnvironment
	}
	return &http.Transport{
		Proxy: proxy,
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			// 代理地址由部署者通过环境变量配置；目标 URL 仍在请求前经过 SSRF 校验。
			if allowEnvironmentProxy && configuredProxyHost(host) {
				return dialer.DialContext(ctx, network, address)
			}
			addresses, err := resolveHost(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, resolvedAddress := range addresses {
				connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolvedAddress.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				lastErr = dialErr
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			return nil, lastErr
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func outboundProxyFromEnvironment(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
}

func configuredProxyHost(host string) bool {
	host = normalizeOutboundHost(host)
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		parsed, err := url.Parse(raw)
		if err == nil && normalizeOutboundHost(parsed.Hostname()) == host {
			return true
		}
	}
	return false
}

func validateOutboundHost(host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), outboundValidationTimeout)
	defer cancel()
	_, err := resolveOutboundHost(ctx, host)
	return err
}

func validateCustomRelayHost(host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), outboundValidationTimeout)
	defer cancel()
	_, err := resolveCustomRelayHost(ctx, host)
	return err
}

func resolveOutboundHost(ctx context.Context, host string) ([]net.IP, error) {
	host = normalizeOutboundHost(host)
	allowPrivateHost := allowPrivateUpstreams() || allowedPrivateUpstreamHost(host)
	addresses, err := resolveOutboundHostWithPolicy(ctx, host, allowPrivateHost)
	if err != nil {
		return nil, err
	}
	if !allowPrivateHost {
		for _, ip := range addresses {
			if blockedCustomRelayIP(ip) {
				return nil, BadAuthRequest("不允许访问保留地址或特殊用途地址")
			}
		}
	}
	return addresses, nil
}

func resolveCustomRelayHost(ctx context.Context, host string) ([]net.IP, error) {
	host = normalizeOutboundHost(host)
	allowPrivateHost := allowedPrivateUpstreamHost(host)
	addresses, err := resolveOutboundHostWithPolicy(ctx, host, allowPrivateHost)
	if err != nil {
		return nil, err
	}
	if !allowPrivateHost {
		for _, ip := range addresses {
			if blockedCustomRelayIP(ip) {
				return nil, BadAuthRequest("不允许访问保留地址或特殊用途地址")
			}
		}
	}
	return addresses, nil
}

func resolvePublicOutboundHost(ctx context.Context, host string) ([]net.IP, error) {
	host = normalizeOutboundHost(host)
	addresses, err := resolveOutboundHostWithPolicy(ctx, host, false)
	if err != nil {
		return nil, err
	}
	for _, ip := range addresses {
		if blockedCustomRelayIP(ip) {
			return nil, BadAuthRequest("不允许访问保留地址或特殊用途地址")
		}
	}
	return addresses, nil
}

func resolveOutboundHostWithPolicy(ctx context.Context, host string, allowPrivateHost bool) ([]net.IP, error) {
	if host == "" {
		return nil, BadAuthRequest("外部服务域名无效")
	}
	if !allowPrivateHost && (host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return nil, BadAuthRequest("不允许访问本机或内网地址")
	}
	var addresses []net.IP
	if literal := net.ParseIP(host); literal != nil {
		addresses = []net.IP{literal}
	} else {
		var err error
		addresses, err = lookupOutboundHostWithCache(ctx, host, sharedOutboundDNSCache, func(lookupCtx context.Context, lookupHost string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(lookupCtx, "ip", lookupHost)
		})
		if err != nil {
			publicErr := BadAuthRequest("外部服务域名解析失败")
			publicErr.Retryable = true
			publicErr.Cause = err
			return nil, publicErr
		}
	}
	if len(addresses) == 0 {
		return nil, BadAuthRequest("外部服务域名没有可用地址")
	}
	if !allowPrivateHost {
		for _, ip := range addresses {
			if blockedOutboundIP(ip) {
				return nil, BadAuthRequest("不允许访问本机、内网或链路本地地址")
			}
		}
	}
	return addresses, nil
}

func lookupOutboundHostWithCache(ctx context.Context, host string, cache *outboundDNSCache, lookup func(context.Context, string) ([]net.IP, error)) ([]net.IP, error) {
	if addresses, ok := cache.load(host, outboundDNSFreshTTL); ok {
		return addresses, nil
	}
	release, err := cache.acquire(ctx, host)
	if err != nil {
		return nil, err
	}
	defer release()
	if addresses, ok := cache.load(host, outboundDNSFreshTTL); ok {
		return addresses, nil
	}

	var lastErr error
	for attempt := 0; attempt < outboundDNSLookupAttempts; attempt++ {
		lookupCtx, cancel := context.WithTimeout(ctx, outboundDNSLookupTimeout)
		addresses, lookupErr := lookup(lookupCtx, host)
		cancel()
		if len(addresses) > 0 {
			cache.store(host, addresses)
			return cloneIPAddresses(addresses), nil
		}
		lastErr = lookupErr
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt+1 < outboundDNSLookupAttempts {
			timer := time.NewTimer(outboundDNSRetryDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}
	// 短时 DNS 故障不应中断已在运行的生成任务；仅回退到近期解析成功且仍会
	// 经过当前 SSRF 规则复核的地址，不缓存失败结果。
	if addresses, ok := cache.load(host, outboundDNSStaleTTL); ok {
		return addresses, nil
	}
	if lastErr == nil {
		lastErr = errors.New("DNS lookup returned no addresses")
	}
	return nil, lastErr
}

func blockedOutboundIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

func blockedCustomRelayIP(ip net.IP) bool {
	if blockedOutboundIP(ip) {
		return true
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	for _, prefix := range blockedCustomRelayPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func allowPrivateUpstreams() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS")))
	return value == "1" || value == "true" || value == "yes"
}

// allowedPrivateUpstreamHost lets operators pin only explicitly trusted upstream
// hostnames to an internal route without disabling SSRF protection for every URL.
func allowedPrivateUpstreamHost(host string) bool {
	host = normalizeOutboundHost(host)
	if host == "" {
		return false
	}
	for _, configured := range strings.Split(os.Getenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS"), ",") {
		if normalizeOutboundHost(configured) == host {
			return true
		}
	}
	return false
}

func normalizeOutboundHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
