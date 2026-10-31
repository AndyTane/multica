package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// clock is a hand-driven time source for the limiter tests.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestLimiter(t *testing.T, accountLimit, sourceLimit int, window time.Duration) (*PasswordAttemptLimiter, *clock) {
	t.Helper()
	c := &clock{now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	l := NewPasswordAttemptLimiter(accountLimit, sourceLimit, window)
	l.now = c.Now
	return l, c
}

func TestPasswordAttemptLimiterAccountBudget(t *testing.T) {
	l, _ := newTestLimiter(t, 5, 20, time.Minute)
	for attempt := 1; attempt <= 5; attempt++ {
		if retry, ok := l.AllowAccount("tane"); !ok {
			t.Fatalf("attempt %d rejected, want allowed; retry=%v", attempt, retry)
		}
	}
	retry, ok := l.AllowAccount("tane")
	if ok {
		t.Fatal("attempt 6 allowed, want rejected: the account budget is 5 per minute")
	}
	if retry != time.Minute {
		t.Fatalf("retry after = %v, want %v (the rest of the window)", retry, time.Minute)
	}
	// A different account has its own budget, and the source budget is
	// untouched by account rejections.
	if _, ok := l.AllowAccount("other"); !ok {
		t.Fatal("another account was rejected: budgets must be per account")
	}
	if _, ok := l.AllowSource(httptest.NewRequest(http.MethodPost, "/auth/password", nil), nil); !ok {
		t.Fatal("source budget consumed by account attempts")
	}
}

func TestPasswordAttemptLimiterAccountWindowRecovers(t *testing.T) {
	l, c := newTestLimiter(t, 2, 20, time.Minute)
	l.AllowAccount("tane")
	l.AllowAccount("tane")
	if _, ok := l.AllowAccount("tane"); ok {
		t.Fatal("budget not exhausted after two attempts")
	}
	c.Advance(time.Minute + time.Second)
	if _, ok := l.AllowAccount("tane"); !ok {
		t.Fatal("budget did not recover after the window closed")
	}
}

func TestPasswordAttemptLimiterSuccessDoesNotRefillSourceBudget(t *testing.T) {
	l, _ := newTestLimiter(t, 1_000, 3, time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/auth/password", nil)
	req.RemoteAddr = "192.0.2.10:4321"
	for attempt := 1; attempt <= 3; attempt++ {
		if _, ok := l.AllowSource(req, nil); !ok {
			t.Fatalf("attempt %d rejected, want allowed", attempt)
		}
	}
	if _, ok := l.AllowSource(req, nil); ok {
		t.Fatal("fourth attempt allowed: the shared source budget must not refill on its own")
	}
}

func TestPasswordAttemptLimiterSourceIgnoresSpoofedForwardedFor(t *testing.T) {
	l, _ := newTestLimiter(t, 1_000, 2, time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/auth/password", nil)
	req.RemoteAddr = "192.0.2.10:4321"
	// No trusted proxies configured: X-Forwarded-For must be ignored, or a
	// client could mint a fresh budget with every request.
	for attempt, spoof := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		req.Header.Set("X-Forwarded-For", spoof)
		_, ok := l.AllowSource(req, nil)
		if attempt < 2 && !ok {
			t.Fatalf("attempt %d rejected, want allowed", attempt+1)
		}
		if attempt == 2 && ok {
			t.Fatal("third attempt with a spoofed X-Forwarded-For allowed: the header must not buy a new budget")
		}
	}
}

func TestPasswordAttemptLimiterSourceHonoursTrustedProxyForwardedFor(t *testing.T) {
	l, _ := newTestLimiter(t, 1_000, 1, time.Minute)
	proxied := httptest.NewRequest(http.MethodPost, "/auth/password", nil)
	proxied.RemoteAddr = "10.0.0.5:4321"
	trusted := ParseTrustedProxies("10.0.0.0/8")
	proxied.Header.Set("X-Forwarded-For", "203.0.113.1")
	if _, ok := l.AllowSource(proxied, trusted); !ok {
		t.Fatal("first proxied client rejected")
	}
	// A second client behind the same trusted proxy gets its own budget.
	proxied.Header.Set("X-Forwarded-For", "203.0.113.2")
	if _, ok := l.AllowSource(proxied, trusted); !ok {
		t.Fatal("second proxied client rejected: forwarded address must scope the budget when the proxy is trusted")
	}
	// The first client is still out of budget.
	proxied.Header.Set("X-Forwarded-For", "203.0.113.1")
	if _, ok := l.AllowSource(proxied, trusted); ok {
		t.Fatal("first proxied client allowed again after spending its budget")
	}
}

func TestPasswordAttemptLimiterEvictsExpiredEntries(t *testing.T) {
	l, c := newTestLimiter(t, 5, 5, time.Minute)
	for i := 0; i < 100; i++ {
		l.AllowAccount("account-" + strconv.Itoa(i))
	}
	c.Advance(2 * time.Minute)
	// The sweep runs on the next allowance, keeping the maps bounded.
	l.AllowAccount("fresh")
	if len(l.accounts) != 1 {
		t.Fatalf("accounts map has %d entries after the window closed, want 1: expired entries must be swept", len(l.accounts))
	}
	if len(l.sources) != 0 {
		t.Fatalf("sources map has %d entries after the window closed, want 0", len(l.sources))
	}
}

func TestPasswordAttemptLimiterCapacityFailsClosed(t *testing.T) {
	l, _ := newTestLimiter(t, 5, 5, time.Minute)
	l.maxEntries = 2
	if _, ok := l.AllowAccount("one"); !ok {
		t.Fatal("first key rejected")
	}
	if _, ok := l.AllowAccount("two"); !ok {
		t.Fatal("second key rejected")
	}
	retry, ok := l.AllowAccount("three")
	if ok {
		t.Fatal("third distinct key allowed while the table is full: refusing is the memory-bound answer")
	}
	if retry <= 0 {
		t.Fatalf("retry after = %v, want a positive wait", retry)
	}
}

func TestWritePasswordRateLimited(t *testing.T) {
	rec := httptest.NewRecorder()
	WritePasswordRateLimited(rec, 1500*time.Millisecond)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want a value rounded up to whole seconds", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}

func TestPasswordSourceLimitMiddleware(t *testing.T) {
	l, _ := newTestLimiter(t, 1_000, 1, time.Minute)
	handler := PasswordSourceLimit(l, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	first := httptest.NewRequest(http.MethodPost, "/auth/password", nil)
	first.RemoteAddr = "192.0.2.20:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, first)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want 204", rec.Code)
	}
	second := httptest.NewRequest(http.MethodPost, "/auth/password", nil)
	second.RemoteAddr = "192.0.2.20:2222"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, second)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 response has no Retry-After header")
	}
}

func TestPasswordSourceLimitNilLimiterIsNoop(t *testing.T) {
	handler := PasswordSourceLimit(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/password", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: a nil limiter disables the middleware", rec.Code)
	}
}
