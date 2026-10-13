package outbound

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestLookupOutboundHostUsesFreshCache(t *testing.T) {
	cache := newOutboundDNSCache()
	calls := 0
	lookup := func(context.Context, string) ([]net.IP, error) {
		calls++
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	for range 2 {
		addresses, err := lookupOutboundHostWithCache(context.Background(), "api.example.com", cache, lookup)
		if err != nil || len(addresses) != 1 || !addresses[0].Equal(net.ParseIP("8.8.8.8")) {
			t.Fatalf("addresses = %v, err = %v", addresses, err)
		}
		addresses[0][0] ^= 1
	}
	if calls != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls)
	}
}

func TestLookupOutboundHostFallsBackToRecentAddress(t *testing.T) {
	cache := newOutboundDNSCache()
	cache.entries["api.example.com"] = outboundDNSCacheEntry{
		addresses:  []net.IP{net.ParseIP("8.8.4.4")},
		resolvedAt: time.Now().Add(-outboundDNSFreshTTL - time.Minute),
	}
	calls := 0
	addresses, err := lookupOutboundHostWithCache(context.Background(), "api.example.com", cache, func(context.Context, string) ([]net.IP, error) {
		calls++
		return nil, errors.New("temporary DNS failure")
	})
	if err != nil || len(addresses) != 1 || !addresses[0].Equal(net.ParseIP("8.8.4.4")) {
		t.Fatalf("addresses = %v, err = %v", addresses, err)
	}
	if calls != outboundDNSLookupAttempts {
		t.Fatalf("lookup calls = %d, want %d", calls, outboundDNSLookupAttempts)
	}
}

func TestOutboundHostPolicyStillValidatesCachedAddresses(t *testing.T) {
	previous := sharedOutboundDNSCache
	sharedOutboundDNSCache = newOutboundDNSCache()
	t.Cleanup(func() { sharedOutboundDNSCache = previous })
	sharedOutboundDNSCache.store("cache-policy.example", []net.IP{net.ParseIP("192.168.1.20")})
	if _, err := resolveOutboundHostWithPolicy(context.Background(), "cache-policy.example", false); err == nil {
		t.Fatal("cached private addresses must not bypass the outbound policy")
	}
	if _, err := resolveOutboundHostWithPolicy(context.Background(), "cache-policy.example", true); err != nil {
		t.Fatalf("explicit trusted host failed: %v", err)
	}
}
