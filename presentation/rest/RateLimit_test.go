// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var start = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

func TestTheBurstIsSpendableAtOnceAndThenRefills(t *testing.T) {
	limiter := NewRateLimiter()

	// 60 a minute is one a second, with five spendable at once.
	for i := range 5 {
		if decision := limiter.Allow("k", 60, 5, start); !decision.Allowed {
			t.Fatalf("request %d of the burst was refused", i+1)
		}
	}
	if decision := limiter.Allow("k", 60, 5, start); decision.Allowed {
		t.Fatal("the burst was exceeded and the request went through")
	}

	// One second later exactly one token is back.
	if decision := limiter.Allow("k", 60, 5, start.Add(time.Second)); !decision.Allowed {
		t.Error("the bucket did not refill")
	}
	if decision := limiter.Allow("k", 60, 5, start.Add(time.Second)); decision.Allowed {
		t.Error("the bucket refilled by more than it should")
	}
}

func TestTheBucketDoesNotFillPastItsDepth(t *testing.T) {
	limiter := NewRateLimiter()
	limiter.Allow("k", 60, 5, start)

	// An hour of silence must not buy an hour of budget.
	allowed := 0
	for range 100 {
		if limiter.Allow("k", 60, 5, start.Add(time.Hour)).Allowed {
			allowed++
		}
	}
	if allowed != 5 {
		t.Errorf("%d requests allowed at once, want the burst of 5", allowed)
	}
}

func TestKeysDoNotShareABudget(t *testing.T) {
	limiter := NewRateLimiter()

	for range 5 {
		limiter.Allow("one", 60, 5, start)
	}
	if !limiter.Allow("two", 60, 5, start).Allowed {
		t.Error("one key's spending refused another key")
	}
}

func TestARefusalSaysWhenToComeBack(t *testing.T) {
	limiter := NewRateLimiter()
	limiter.Allow("k", 60, 1, start)

	decision := limiter.Allow("k", 60, 1, start)
	if decision.Allowed {
		t.Fatal("the second request was allowed")
	}
	if decision.RetryAfter <= 0 {
		t.Errorf("retry after %v - a client told to wait no time comes straight back", decision.RetryAfter)
	}
}

// A limit of zero would refuse every request. The configuration rejects it at startup; here it
// reads as "not configured", because locking an installation out of its own API is the worse of
// the two failures.
func TestAnUnsetLimitDoesNotRefuse(t *testing.T) {
	limiter := NewRateLimiter()

	for range 100 {
		if !limiter.Allow("k", 0, 0, start).Allowed {
			t.Fatal("an unset limit refused a request")
		}
	}
}

func TestIdleBucketsAreDropped(t *testing.T) {
	limiter := NewRateLimiter()
	limiter.Allow("k", 60, 5, start)

	if len(limiter.buckets) != 1 {
		t.Fatalf("%d buckets after one request", len(limiter.buckets))
	}
	// A different key, long after: the sweep runs and the idle bucket goes.
	limiter.Allow("other", 60, 5, start.Add(bucketIdleTTL+time.Minute))

	if _, still := limiter.buckets["k"]; still {
		t.Error("an idle bucket survived the sweep - the map grows without bound")
	}
}

func serveLimited(t *testing.T, limited Limited, r *http.Request) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	limited.Next = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })

	response := httptest.NewRecorder()
	limited.ServeHTTP(response, r)
	return response, reached
}

func TestTheBudgetIsReportedOnEveryAnswer(t *testing.T) {
	limited := Limited{
		Limiter: NewRateLimiter(), Level: "credential",
		Bucket: CredentialBucket(60, 600, 10),
		Clock:  func() time.Time { return start },
	}

	response, reached := serveLimited(t, limited,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil))

	if !reached {
		t.Fatal("the handler was not reached")
	}
	if got := response.Header().Get("RateLimit-Limit"); got != "60" {
		t.Errorf("RateLimit-Limit = %q, want the anonymous budget", got)
	}
	if response.Header().Get("RateLimit-Remaining") == "" {
		t.Error("no RateLimit-Remaining - a client cannot slow down before it is refused")
	}
}

// A token is somebody the installation knows and gets the larger budget.
func TestACredentialGetsTheTokenBudget(t *testing.T) {
	limited := Limited{
		Limiter: NewRateLimiter(), Level: "credential",
		Bucket: CredentialBucket(60, 600, 10),
		Clock:  func() time.Time { return start },
	}

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/containers", nil)
	r.Header.Set("Authorization", "Bearer "+credential)

	response, _ := serveLimited(t, limited, r)

	if got := response.Header().Get("RateLimit-Limit"); got != "600" {
		t.Errorf("RateLimit-Limit = %q, want the token budget", got)
	}
}

func TestAnExhaustedBudgetIs429WithRetryAfter(t *testing.T) {
	limiter := NewRateLimiter()
	limited := Limited{
		Limiter: limiter, Level: "credential",
		Bucket: CredentialBucket(60, 600, 2),
		Clock:  func() time.Time { return start },
	}
	newRequest := func() *http.Request {
		return httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil)
	}

	for range 2 {
		serveLimited(t, limited, newRequest())
	}
	response, reached := serveLimited(t, limited, newRequest())

	if reached {
		t.Error("the handler ran although the budget was spent")
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", response.Code)
	}
	if after, err := strconv.Atoi(response.Header().Get("Retry-After")); err != nil || after < 1 {
		t.Errorf("Retry-After = %q", response.Header().Get("Retry-After"))
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.Code != "rate_limited" {
		t.Errorf("code %q", problem.Code)
	}
	if problem.Params["level"] != "credential" {
		t.Errorf("params %v - the refusal does not say which limit bit", problem.Params)
	}
}

// Two clients behind two addresses must not share a budget, and two tokens must not either.
func TestBudgetsAreSeparatedByCredentialAndAddress(t *testing.T) {
	limiter := NewRateLimiter()
	limited := Limited{
		Limiter: limiter, Level: "credential",
		Bucket: CredentialBucket(60, 600, 1),
		Clock:  func() time.Time { return start },
	}

	first := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil)
	first.RemoteAddr = "192.0.2.1:1234"
	second := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil)
	second.RemoteAddr = "192.0.2.2:1234"

	serveLimited(t, limited, first)
	if _, reached := serveLimited(t, limited, second); !reached {
		t.Error("one address spent another address's budget")
	}
	if _, reached := serveLimited(t, limited, first); reached {
		t.Error("the first address was not limited")
	}
}

// Before authentication there is no tenant, and a level that does not apply must let the request
// through rather than inventing a bucket for it.
func TestTheTenantLevelSkipsAnUnauthenticatedRequest(t *testing.T) {
	limited := Limited{
		Limiter: NewRateLimiter(), Level: "tenant",
		Bucket: TenantBucket(3000, 100),
		Clock:  func() time.Time { return start },
	}

	response, reached := serveLimited(t, limited,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil))

	if !reached {
		t.Fatal("an anonymous request was refused by the tenant limit")
	}
	if response.Header().Get("RateLimit-Limit") != "" {
		t.Error("a budget was reported for a level that does not apply")
	}
}

func TestTheTenantLevelCountsPerTenant(t *testing.T) {
	limited := Limited{
		Limiter: NewRateLimiter(), Level: "tenant",
		Bucket: TenantBucket(60, 1),
		Clock:  func() time.Time { return start },
	}

	authenticated := func(tenant string) *http.Request {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/containers", nil)
		actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: shared.ID(tenant)}
		return r.WithContext(appshared.ContextWithActor(r.Context(), actor))
	}

	serveLimited(t, limited, authenticated("018f2a1b-0000-7000-8000-0000000000ab"))
	if _, reached := serveLimited(t, limited, authenticated("018f2a1b-0000-7000-8000-0000000000cd")); !reached {
		t.Error("one tenant spent another tenant's budget")
	}
	if _, reached := serveLimited(t, limited, authenticated("018f2a1b-0000-7000-8000-0000000000ab")); reached {
		t.Error("the first tenant was not limited")
	}
}

// X-Forwarded-For is written by the client. Believing it would let one caller claim a fresh
// address per request and walk straight through the anonymous limit.
func TestTheClientAddressIgnoresForwardedHeaders(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil)
	r.RemoteAddr = "192.0.2.7:4242"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.Header.Set("X-Real-Ip", "203.0.113.9")

	if got := ClientAddress(r); got != "192.0.2.7" {
		t.Errorf("client address = %q, want the peer of the connection", got)
	}
}

// The bucket key must not be the credential itself: the map ends up in a heap dump.
func TestTheCredentialIsNotItsOwnBucketKey(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/containers", nil)
	r.Header.Set("Authorization", "Bearer "+credential)

	key := CredentialBucket(60, 600, 10)(r).Key
	if key == "" {
		t.Fatal("no bucket for a presented credential")
	}
	if key == credential || len(key) > 32 {
		t.Errorf("bucket key = %q - the token itself is the key", key)
	}
}

// The workspace's own per-token ceiling (H-08): it engages only where one is configured, keys
// per credential, and a workspace without one leaves the level inert.
func TestTheOverrideBucketEngagesOnlyWhereConfigured(t *testing.T) {
	bucket := OverrideBucket(5)

	r := request(t, "/containers")
	if b := bucket(r); b.Key != "" {
		t.Errorf("an anonymous request got a bucket: %+v", b)
	}

	actor := authenticatedActor()
	actor.TokenID = shared.ID("018f2a1b-0000-7000-8000-0000000000ee")
	r = r.WithContext(appshared.ContextWithActor(r.Context(), actor))
	if b := bucket(r); b.Key != "" {
		t.Errorf("a workspace without an override got a bucket: %+v", b)
	}

	actor.RateLimitPerMinute = 42
	r = request(t, "/containers")
	r = r.WithContext(appshared.ContextWithActor(r.Context(), actor))
	b := bucket(r)
	if b.Limit != 42 || b.Burst != 5 {
		t.Errorf("bucket %+v", b)
	}
	if !strings.Contains(b.Key, actor.TokenID.String()) {
		t.Errorf("the ceiling is per token; key %q does not name one", b.Key)
	}

	// And through the limiter it engages: the burst passes, the next is refused, and a minute
	// later the configured rate has refilled the budget.
	limiter := NewRateLimiter()
	moment := time.Now()
	for i := 0; i < b.Burst; i++ {
		if d := limiter.Allow("token:"+b.Key, b.Limit, b.Burst, moment); !d.Allowed {
			t.Fatalf("request %d refused inside the burst", i)
		}
	}
	if d := limiter.Allow("token:"+b.Key, b.Limit, b.Burst, moment); d.Allowed {
		t.Error("the ceiling did not engage at its bound")
	}
	if d := limiter.Allow("token:"+b.Key, b.Limit, b.Burst, moment.Add(time.Minute)); !d.Allowed {
		t.Error("the configured rate did not refill the budget")
	}
}
