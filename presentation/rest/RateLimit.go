// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The window every configured limit is expressed in (multi-tenancy.md §4, security.md §9).
const rateWindow = time.Minute

// bucketIdleTTL is how long an unused bucket is kept. Long enough that a client on a slow
// cadence is not handed a fresh budget every time, short enough that a scan of the address space
// does not grow the map for ever.
const bucketIdleTTL = 10 * time.Minute

// sweepInterval is how often idle buckets are dropped. The sweep runs inside a request rather
// than in a goroutine of its own: a background loop would need a lifecycle, a shutdown path and a
// SafeGo (rule 5), which is a lot of machinery for deleting entries from a map.
const sweepInterval = time.Minute

// RateLimiter is a token bucket per key.
//
// Per process, deliberately. A shared counter in PostgreSQL or Redis would be exact across
// replicas and would put a write on the hot path of every request, including the requests it is
// meant to refuse - which is how a rate limiter becomes the outage it was installed to prevent.
// The effective limit is therefore the configured one times the number of replicas, and that is
// the documented trade-off (observability-reliability.md §6).
type RateLimiter struct {
	mutex     sync.Mutex
	buckets   map[string]*rateBucket
	lastSweep time.Time
}

type rateBucket struct {
	// tokens is fractional, so a limit of 60 per minute refills smoothly rather than in one jump
	// at the top of each minute.
	tokens float64
	seen   time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: map[string]*rateBucket{}}
}

// Decision is what the caller needs to answer with, whether or not the request is allowed: the
// RateLimit headers belong on every response, not only on a 429 (api-guidelines.md §5).
type Decision struct {
	Allowed   bool
	Limit     int
	Remaining int
	// RetryAfter is how long until one token is available again. Zero when the request was
	// allowed.
	RetryAfter time.Duration
	// Reset is how long until the bucket is full again.
	Reset time.Duration
}

// Allow spends one token from the key's bucket.
//
// burst is the depth of the bucket: how much of a minute's budget may be spent at once. A limit
// without one would force every client into an even cadence, which no real client has.
func (l *RateLimiter) Allow(key string, limit, burst int, now time.Time) Decision {
	if limit <= 0 {
		// A limit of zero would refuse everything. Treated as "not configured", because a
		// configuration mistake must not lock an installation out of its own API; the
		// configuration validation already rejects it at startup.
		return Decision{Allowed: true, Limit: 0, Remaining: 0}
	}
	depth := float64(max(burst, 1))
	perSecond := float64(limit) / rateWindow.Seconds()

	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.sweep(now)

	bucket, known := l.buckets[key]
	if !known {
		bucket = &rateBucket{tokens: depth, seen: now}
		l.buckets[key] = bucket
	} else {
		elapsed := now.Sub(bucket.seen).Seconds()
		if elapsed > 0 {
			bucket.tokens = math.Min(depth, bucket.tokens+elapsed*perSecond)
		}
		bucket.seen = now
	}

	decision := Decision{
		Limit: limit,
		Reset: time.Duration((depth - bucket.tokens) / perSecond * float64(time.Second)),
	}
	if bucket.tokens >= 1 {
		bucket.tokens--
		decision.Allowed = true
		decision.Remaining = int(bucket.tokens)
		return decision
	}

	// Rounded up: telling a client to retry in zero seconds invites it straight back into
	// another refusal.
	decision.RetryAfter = time.Duration(math.Ceil((1-bucket.tokens)/perSecond)) * time.Second
	return decision
}

// sweep drops buckets nobody has touched. Called under the lock, at most once per interval.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	for key, bucket := range l.buckets {
		if now.Sub(bucket.seen) > bucketIdleTTL {
			delete(l.buckets, key)
		}
	}
}

// Bucket names the budget one request counts against. A zero Key means the level does not apply
// to this request - an unauthenticated one has no tenant to count against.
type Bucket struct {
	Key string
	// Limit is the budget per minute, Burst how much of it may be spent at once.
	Limit int
	Burst int
}

// Limited refuses a request that has spent its budget.
//
// One type, wired more than once: before authentication it counts per credential or per client
// address, afterwards per tenant (security.md §9, "multi-level"). Counting per credential needs
// no database - the key is a fingerprint of the presented string - which is what keeps a flood of
// invalid tokens from costing a lookup each.
type Limited struct {
	Next    http.Handler
	Limiter *RateLimiter
	// Level names the bucket in the refusal, so an operator reading a 429 knows which limit bit.
	// It also separates the key spaces, so one address cannot spend a tenant's budget.
	Level string
	// Bucket decides which budget this request counts against, and how large it is.
	Bucket func(*http.Request) Bucket
	// Clock is injectable so the tests do not have to sleep. Nil means the system clock.
	Clock func() time.Time
}

func (l Limited) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket := l.Bucket(r)
	if bucket.Key == "" {
		l.Next.ServeHTTP(w, r)
		return
	}

	decision := l.Limiter.Allow(l.Level+":"+bucket.Key, bucket.Limit, bucket.Burst, l.now())
	writeRateLimitHeaders(w, decision)

	if !decision.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(decision.RetryAfter.Seconds())))
		WriteProblem(w,
			shared.ErrRateLimited.
				WithDetail("access.rate_limit_exceeded").
				WithParams(map[string]string{"level": l.Level}),
			correlation.RequestIDFrom(r.Context()))
		return
	}
	l.Next.ServeHTTP(w, r)
}

func (l Limited) now() time.Time {
	if l.Clock == nil {
		return time.Now()
	}
	return l.Clock()
}

// writeRateLimitHeaders reports the budget on every answer, so a well-behaved client can slow
// down before it is refused rather than after.
func writeRateLimitHeaders(w http.ResponseWriter, decision Decision) {
	if decision.Limit <= 0 {
		return
	}
	header := w.Header()
	header.Set("RateLimit-Limit", strconv.Itoa(decision.Limit))
	header.Set("RateLimit-Remaining", strconv.Itoa(decision.Remaining))
	header.Set("RateLimit-Reset", strconv.Itoa(int(math.Ceil(decision.Reset.Seconds()))))
}

// CredentialBucket counts per presented credential, and per client address when there is none.
// The two carry different budgets: an anonymous caller is a stranger, a token is somebody the
// installation knows (multi-tenancy.md §4).
//
// The credential is fingerprinted rather than used as the key. The map ends up in a heap dump,
// and a heap dump with live tokens in it is a second incident on top of the first (rule 10).
func CredentialBucket(anonymousPerMinute, tokenPerMinute, burst int) func(*http.Request) Bucket {
	return func(r *http.Request) Bucket {
		if credential, err := bearerCredential(r); err == nil && credential != "" {
			return Bucket{Key: "t" + fingerprint(credential), Limit: tokenPerMinute, Burst: burst}
		}
		return Bucket{Key: "a" + ClientAddress(r), Limit: anonymousPerMinute, Burst: burst}
	}
}

// TenantBucket counts per tenant, so that one busy token cannot spend a whole tenant's budget and
// one busy tenant cannot spend the installation's. It applies only once authentication has run.
func TenantBucket(perMinute, burst int) func(*http.Request) Bucket {
	return func(r *http.Request) Bucket {
		actor, ok := appshared.ActorFrom(r.Context())
		if !ok || !actor.IsAuthenticated() {
			return Bucket{}
		}
		return Bucket{Key: actor.TenantID.String(), Limit: perMinute, Burst: burst}
	}
}

// fingerprintLength is how much of the digest is kept. 16 hex digits is 64 bits: far too much to
// collide across the buckets of one process, and far too little to attack the token behind it.
const fingerprintLength = 16

// fingerprint reduces a credential to a bucket key. Plain SHA-256 without a pepper is enough
// here - the value never leaves the process and is never compared against anything stored, so the
// only property that matters is that two different tokens land in two different buckets.
func fingerprint(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])[:fingerprintLength]
}

// ClientAddress is the peer of the connection, never X-Forwarded-For.
//
// That header is written by the client and can only be believed when a proxy that overwrites it
// sits in front - and this process cannot know whether one does. Trusting it would turn the
// anonymous limit into decoration, because every request could claim a fresh address. The cost is
// that everything behind one ingress shares a bucket, which is the safe direction to be wrong in;
// a trusted-proxy setting belongs with the deployment work that knows the topology.
func ClientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
