package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// TestClientIPKey pins the network-prefix aggregation behind issue #954: an
// IPv6 client owns its /64 (so rotating the low 64 bits an ISP hands out does
// not mint a fresh rate-limit bucket), IPv4 stays per-address, IPv4-mapped
// IPv6 normalises to the IPv4 key, and an unparseable value is passed through
// verbatim rather than collapsed onto a shared bucket.
func TestClientIPKey(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{name: "IPv4 is per-address (/32)", ip: "203.0.113.7", want: "203.0.113.7"},
		{name: "IPv4-mapped IPv6 normalises to IPv4", ip: "::ffff:203.0.113.7", want: "203.0.113.7"},
		{name: "IPv6 aggregates to /64", ip: "2001:db8:1:2:3:4:5:6", want: "2001:db8:1:2::/64"},
		{name: "same IPv6 /64, different host bits", ip: "2001:db8:1:2:ffff::abcd", want: "2001:db8:1:2::/64"},
		{name: "adjacent IPv6 /64 is a distinct bucket", ip: "2001:db8:1:3::1", want: "2001:db8:1:3::/64"},
		{name: "full 128-bit address still masks to /64", ip: "2001:db8:abcd:1234:5678:9abc:def0:1234", want: "2001:db8:abcd:1234::/64"},
		{name: "unparseable value is verbatim, never merged", ip: "not-an-ip", want: "not-an-ip"},
		{name: "empty stays empty", ip: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientIPKey(tt.ip); got != tt.want {
				t.Errorf("clientIPKey(%q) = %q, want %q", tt.ip, got, tt.want)
			}
		})
	}
}

// newStrictRouter builds a gin engine whose single route is guarded by a limiter
// with one token and no meaningful refill, so bucket membership is directly
// observable: the first request for a key is 200 and every later one is 429.
func newStrictRouter(t *testing.T, trustedProxies []string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		t.Fatalf("SetTrustedProxies(%v): %v", trustedProxies, err)
	}
	router.Use(RateLimitMiddleware(NewIPRateLimiter(rate.Every(time.Hour), 1)))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func doRequest(router *gin.Engine, remoteAddr, xff string) int {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	router.ServeHTTP(w, req)
	return w.Code
}

// TestRateLimitMiddleware_IPv6RotationSharesBucket is the regression test for
// the reported bypass: rotating source addresses within one ISP-issued /64 must
// not yield a fresh bucket per address.
func TestRateLimitMiddleware_IPv6RotationSharesBucket(t *testing.T) {
	router := newStrictRouter(t, nil)

	if code := doRequest(router, "[2001:db8:1:2::1]:4000", ""); code != http.StatusOK {
		t.Fatalf("first address in /64: got %d, want 200", code)
	}
	// A different address in the *same* /64 must hit the same exhausted bucket.
	if code := doRequest(router, "[2001:db8:1:2:ffff::abcd]:4001", ""); code != http.StatusTooManyRequests {
		t.Fatalf("rotated address in the same /64 must share the bucket: got %d, want 429", code)
	}
	// A genuinely different /64 gets its own bucket.
	if code := doRequest(router, "[2001:db8:1:3::1]:4002", ""); code != http.StatusOK {
		t.Fatalf("address in an adjacent /64 must have its own bucket: got %d, want 200", code)
	}
}

// TestRateLimitMiddleware_ProxyTopology_DerivesRealClientIP exercises the real
// proxy topology the shipped image runs (#954): the bundled nginx connects to
// the backend over loopback, so the request's RemoteAddr is the trusted
// 127.0.0.1 hop while the actual client rides in X-Forwarded-For. Two different
// forwarded clients must get independent buckets; without granting the loopback
// hop trust, both would collapse into one 127.0.0.1 bucket.
func TestRateLimitMiddleware_ProxyTopology_DerivesRealClientIP(t *testing.T) {
	router := newStrictRouter(t, []string{"127.0.0.1/32", "::1/128"})

	if code := doRequest(router, "127.0.0.1:4000", "198.51.100.7"); code != http.StatusOK {
		t.Fatalf("first forwarded client: got %d, want 200", code)
	}
	if code := doRequest(router, "127.0.0.1:4001", "198.51.100.8"); code != http.StatusOK {
		t.Fatalf("a second forwarded client must not share the first's bucket: got %d, want 200", code)
	}
	if code := doRequest(router, "127.0.0.1:4002", "198.51.100.7"); code != http.StatusTooManyRequests {
		t.Fatalf("first forwarded client replayed: got %d, want 429", code)
	}
}

// TestRateLimitMiddleware_ProxyTopology_IgnoresClientSuppliedXFF pins that a
// forged leftmost X-Forwarded-For entry cannot move a client into a fresh
// bucket. nginx's proxy_add_x_forwarded_for appends the real peer to whatever
// the client sent, and walking right-to-left past the trusted loopback hop
// lands on that appended address, not the forgery.
func TestRateLimitMiddleware_ProxyTopology_IgnoresClientSuppliedXFF(t *testing.T) {
	router := newStrictRouter(t, []string{"127.0.0.1/32", "::1/128"})

	// Same real client (198.51.100.7), two different forged leading values.
	if code := doRequest(router, "127.0.0.1:4000", "1.2.3.4, 198.51.100.7"); code != http.StatusOK {
		t.Fatalf("first request with a forged leading XFF: got %d, want 200", code)
	}
	if code := doRequest(router, "127.0.0.1:4001", "9.9.9.9, 198.51.100.7"); code != http.StatusTooManyRequests {
		t.Fatalf("rotating the forged leading XFF must not escape the bucket: got %d, want 429", code)
	}
}

// TestRateLimitMiddleware_CatchAllProxyTrustsForgedXFF documents *why*
// config.Validate refuses a catch-all 0.0.0.0/0 / ::/0 trusted proxy: with every
// source trusted, gin accepts the client-supplied leftmost X-Forwarded-For, so
// rotating that header mints a fresh bucket per request. If a future change
// stops rejecting the catch-all, this test shows the bypass it would re-enable.
func TestRateLimitMiddleware_CatchAllProxyTrustsForgedXFF(t *testing.T) {
	router := newStrictRouter(t, []string{"0.0.0.0/0"})

	if code := doRequest(router, "127.0.0.1:4000", "203.0.113.7"); code != http.StatusOK {
		t.Fatalf("first forged XFF: got %d, want 200", code)
	}
	if code := doRequest(router, "127.0.0.1:4001", "203.0.113.8"); code != http.StatusOK {
		t.Fatalf("catch-all trust means a rotated forged XFF escapes the bucket: got %d, want 200", code)
	}
}
