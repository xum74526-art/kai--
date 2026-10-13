package outbound

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

const (
	outboundValidationTimeout = 10 * time.Second
	outboundDNSFreshTTL       = 30 * time.Minute
	outboundDNSStaleTTL       = 6 * time.Hour
	outboundDNSLookupTimeout  = 3 * time.Second
	outboundDNSRetryDelay     = 150 * time.Millisecond
	outboundDNSLookupAttempts = 2
)

var sharedOutboundDNSCache = newOutboundDNSCache()

type outboundDNSCacheEntry struct {
	addresses  []net.IP
	resolvedAt time.Time
}

// 只缓存地址，不缓存安全判断；每次使用仍需复核当前出口白名单。
type outboundDNSCache struct {
	mu      sync.RWMutex
	entries map[string]outboundDNSCacheEntry
	locks   map[string]chan struct{}
}

func newOutboundDNSCache() *outboundDNSCache {
	return &outboundDNSCache{entries: make(map[string]outboundDNSCacheEntry), locks: make(map[string]chan struct{})}
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
	c.mu.RLock()
	entry, ok := c.entries[host]
	c.mu.RUnlock()
	if !ok || len(entry.addresses) == 0 || time.Since(entry.resolvedAt) > maxAge {
		return nil, false
	}
	return cloneIPAddresses(entry.addresses), true
}

func (c *outboundDNSCache) store(host string, addresses []net.IP) {
	if len(addresses) == 0 {
		return
	}
	c.mu.Lock()
	c.entries[host] = outboundDNSCacheEntry{addresses: cloneIPAddresses(addresses), resolvedAt: time.Now()}
	c.mu.Unlock()
}

func (c *outboundDNSCache) acquire(ctx context.Context, host string) (func(), error) {
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

func lookupOutboundHostWithCache(ctx context.Context, host string, cache *outboundDNSCache, lookup func(context.Context, string) ([]net.IP, error)) ([]net.IP, error) {
	if literal := net.ParseIP(host); literal != nil {
		return []net.IP{literal}, nil
	}
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
	// DNS 短时抖动可用近期成功地址兜底；首次解析失败不猜测或硬编码 IP。
	if addresses, ok := cache.load(host, outboundDNSStaleTTL); ok {
		return addresses, nil
	}
	if lastErr == nil {
		lastErr = errors.New("DNS lookup returned no addresses")
	}
	return nil, lastErr
}
