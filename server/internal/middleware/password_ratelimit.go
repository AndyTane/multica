package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// PasswordAttemptLimiter is the single-process, in-memory limiter behind the
// local password login route.
//
// The shared RateLimit middleware is a no-op when Redis is absent, and the
// desktop deployment this route exists for has no Redis. Password login is the
// one auth surface where "no limiter" means unlimited offline-guessable
// credentials, so this limiter owns its own state instead of depending on a
// deployment service.
//
// Two budgets are tracked for one attempt: the account (normalized username)
// and the source IP. Counting happens before any bcrypt comparison, so an
// attacker cannot force the per-attempt hashing work at an unbounded rate.
// Successful logins deliberately do not clear either budget: a shared source
// budget that a success could reset would let one valid account erase the
// source's history for everybody behind that IP.
//
// Windows are fixed and per key: the first attempt opens a window, the window
// closes at now+window, and the next attempt after that starts a new one. This
// is coarser than a sliding window but keeps the state bounded and the code
// auditable.
type PasswordAttemptLimiter struct {
	mu           sync.Mutex
	window       time.Duration
	accountLimit int
	sourceLimit  int
	maxEntries   int
	now          func() time.Time
	nextSweep    time.Time
	accounts     map[string]*attemptWindow
	sources      map[string]*attemptWindow
}

type attemptWindow struct {
	count   int
	resetAt time.Time
}

const (
	// DefaultPasswordAccountLimit is the per-account attempts allowed per window.
	DefaultPasswordAccountLimit = 5
	// DefaultPasswordSourceLimit is the per-IP attempts allowed per window.
	DefaultPasswordSourceLimit = 20
	// DefaultPasswordLimitWindow is the width of one budget window.
	DefaultPasswordLimitWindow = time.Minute
	// defaultPasswordLimiterMaxEntries bounds each bucket. Reaching it takes a
	// distributed flood; the limiter then refuses keys it has not seen rather
	// than growing without bound.
	defaultPasswordLimiterMaxEntries = 50_000
)

// NewPasswordAttemptLimiter builds a limiter. Non-positive limits or window
// fall back to the documented defaults.
func NewPasswordAttemptLimiter(accountLimit, sourceLimit int, window time.Duration) *PasswordAttemptLimiter {
	if accountLimit <= 0 {
		accountLimit = DefaultPasswordAccountLimit
	}
	if sourceLimit <= 0 {
		sourceLimit = DefaultPasswordSourceLimit
	}
	if window <= 0 {
		window = DefaultPasswordLimitWindow
	}
	return &PasswordAttemptLimiter{
		window:       window,
		accountLimit: accountLimit,
		sourceLimit:  sourceLimit,
		maxEntries:   defaultPasswordLimiterMaxEntries,
		now:          time.Now,
		accounts:     make(map[string]*attemptWindow),
		sources:      make(map[string]*attemptWindow),
	}
}

// AllowAccount charges one attempt to the normalized account name. It returns
// the remaining wait when the budget is exhausted.
func (l *PasswordAttemptLimiter) AllowAccount(username string) (time.Duration, bool) {
	return l.allow(l.accounts, username, l.accountLimit)
}

// AllowSource charges one attempt to the client IP. trustedProxies controls
// whether X-Forwarded-For is believed; an empty list means it never is, so a
// spoofed header cannot buy a fresh budget.
func (l *PasswordAttemptLimiter) AllowSource(r *http.Request, trustedProxies []*net.IPNet) (time.Duration, bool) {
	return l.allow(l.sources, extractIP(r, trustedProxies), l.sourceLimit)
}

func (l *PasswordAttemptLimiter) allow(bucket map[string]*attemptWindow, key string, limit int) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	if w, ok := bucket[key]; ok && now.Before(w.resetAt) {
		if w.count >= limit {
			return w.resetAt.Sub(now), false
		}
		w.count++
		return 0, true
	}
	if len(bucket) >= l.maxEntries {
		// The cap only binds after a sweep failed to free anything. Refusing
		// the attempt is the fail-closed answer: an unbounded map is a
		// memory-exhaustion bug, but an unbounded retry stream is worse.
		return l.window, false
	}
	bucket[key] = &attemptWindow{count: 1, resetAt: now.Add(l.window)}
	return 0, true
}

// sweepLocked drops entries whose window has closed. It runs at most once per
// window, so the amortized cost per attempt stays O(1).
func (l *PasswordAttemptLimiter) sweepLocked(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	sweepBucket(l.accounts, now)
	sweepBucket(l.sources, now)
	l.nextSweep = now.Add(l.window)
}

func sweepBucket(bucket map[string]*attemptWindow, now time.Time) {
	for key, w := range bucket {
		if !now.Before(w.resetAt) {
			delete(bucket, key)
		}
	}
}

// WritePasswordRateLimited answers an attempt that spent its budget, in the
// same JSON shape as every other auth error, with a Retry-After the client can
// honour.
func WritePasswordRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := int((retryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "too many login attempts"})
}

// PasswordSourceLimit rejects a login attempt whose source IP has spent its
// attempt budget. The account budget is charged inside the handler, after the
// submitted username has been normalized; the source budget has everything it
// needs here, so it stays a route middleware.
func PasswordSourceLimit(limiter *PasswordAttemptLimiter, trustedProxies []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limiter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if retryAfter, ok := limiter.AllowSource(r, trustedProxies); !ok {
				WritePasswordRateLimited(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AttemptSourceLimit charges one attempt to the client source IP using the
// supplied in-process limiter and answers 429 with Retry-After when the budget
// is spent. It is the route-level half of the registration budgets: the source
// budget runs before any handler work, and trustedProxies is empty unless the
// operator explicitly listed the reverse proxy, so a forged X-Forwarded-For
// cannot buy a fresh budget.
func AttemptSourceLimit(limiter *PasswordAttemptLimiter, trustedProxies []*net.IPNet, errorMessage string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limiter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			retryAfter, ok := limiter.AllowSource(r, trustedProxies)
			if !ok {
				writeAttemptRateLimited(w, retryAfter, errorMessage)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAttemptRateLimited(w http.ResponseWriter, retryAfter time.Duration, message string) {
	seconds := int((retryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
