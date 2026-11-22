// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// countingBucket answers preflights from its rules and counts every request that reaches it.
type countingBucket struct {
	fakeBucket
	dials atomic.Int64
}

func (b *countingBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.dials.Add(1)
	b.fakeBucket.ServeHTTP(w, r)
}

// UC-INS-17 check 6: a bucket that refuses the browser's upload is named in the health report with
// the origin, when it was checked - the Clock's time - and how often it is asked again; once the
// rule is set the warning is gone with the next check, which runs every CORSCheckEvery. Reading
// the report never dials.
func TestTheBucketCORSWarningIsServedFromTheLastCheck(t *testing.T) {
	if CORSCheckEvery != 15*time.Minute {
		t.Fatalf("CORSCheckEvery = %v; deployment.md §6.1 and the apply command name 15 minutes", CORSCheckEvery)
	}
	bucket := &countingBucket{}
	server := httptest.NewServer(bucket)
	defer server.Close()
	store := s3Against(t, server.URL, true)
	at := time.Date(2026, 11, 22, 9, 30, 0, 0, time.UTC)
	origins := OriginsFor("https://tasks.example.org", "single", "")
	watch := NewCORSWatch(store, origins.Check, clock.Fixed(at))

	if got := watch.Warnings(t.Context()); len(got) != 0 {
		t.Fatalf("before the first check = %+v, want nothing claimed", got)
	}
	watch.Check(t.Context())
	dials := bucket.dials.Load()

	for range 5 {
		got := watch.Warnings(t.Context())
		if len(got) != 1 || got[0].Code != CORSMissingCode || got[0].Severity != "warn" ||
			got[0].Params["bucket"] != "hubtask-media" ||
			got[0].Params["origin"] != "https://tasks.example.org" ||
			got[0].Params["checked_at"] != "2026-11-22T09:30:00Z" ||
			got[0].Params["minutes"] != "15" {
			t.Fatalf("warnings = %+v", got)
		}
	}
	if bucket.dials.Load() != dials {
		t.Errorf("reading the report dialled the bucket %d times", bucket.dials.Load()-dials)
	}

	bucket.mu.Lock()
	bucket.rules = []CORSRule{HubtaskRule(origins.Rule)}
	bucket.mu.Unlock()
	if got := watch.Warnings(t.Context()); len(got) != 1 {
		t.Errorf("the held result changed without a check: %+v", got)
	}
	watch.Check(t.Context())
	if got := watch.Warnings(t.Context()); len(got) != 0 {
		t.Errorf("after the rule was set and checked = %+v, want none", got)
	}
}

// An unreachable bucket is no CORS finding: the held answer stays as it was.
func TestAnUnreachableBucketKeepsTheLastAnswer(t *testing.T) {
	server := httptest.NewServer(&fakeBucket{})
	store := s3Against(t, server.URL, true)
	watch := NewCORSWatch(store, []string{"https://tasks.example.org"}, clock.Fixed(time.Unix(0, 0)))
	watch.Check(t.Context())
	server.Close()
	watch.Check(t.Context())
	if got := watch.Warnings(t.Context()); len(got) != 1 {
		t.Errorf("after an unreachable check = %+v, want the refusal still held", got)
	}

	never := NewCORSWatch(s3Against(t, "http://127.0.0.1:1", true),
		[]string{"https://tasks.example.org"}, clock.Fixed(time.Unix(0, 0)))
	never.Check(t.Context())
	if got := never.Warnings(t.Context()); len(got) != 0 {
		t.Errorf("a bucket never reached = %+v, want no CORS finding", got)
	}
}

// deployment.md §6.1: the preflight goes around the resilient wrapper, so refusals and an
// unreachable bucket alike leave the breaker media health reads closed.
func TestThePreflightLeavesTheStorageBreakerClosed(t *testing.T) {
	refusing := httptest.NewServer(&fakeBucket{})
	defer refusing.Close()
	for _, target := range []string{refusing.URL, "http://127.0.0.1:1"} {
		raw := s3Against(t, target, true)
		breaker := resilience.NewBreaker(resilience.BreakerConfig{Dependency: s3Dependency, FailureThreshold: 1})
		store := NewResilientStore(raw, breaker, resilience.NewBulkhead(resilience.BulkheadConfig{Name: "s3", Capacity: 1}))
		var _ port.ObjectStore = store
		watch := NewCORSWatch(raw, []string{"https://tasks.example.org"}, clock.Fixed(time.Unix(0, 0)))
		for range 5 {
			watch.Check(t.Context())
		}
		if state := breaker.State(); state != resilience.BreakerClosed {
			t.Errorf("%s: breaker = %v after the preflights, want closed", target, state)
		}
		if result := NewProbe(breaker).Check(t.Context()); result.ErrorCode != "" {
			t.Errorf("%s: media health = %+v", target, result)
		}
	}

	// The same breaker does open on the wrapped store's own failure - the test above is not one
	// a breaker that never opens would pass.
	raw := s3Against(t, "http://127.0.0.1:1", true)
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Dependency: s3Dependency, FailureThreshold: 1})
	store := NewResilientStore(raw, breaker, resilience.NewBulkhead(resilience.BulkheadConfig{Name: "s3", Capacity: 1}))
	_ = store.Put(t.Context(), port.Upload{Key: "a/b", Content: bytes.NewReader(nil)})
	if breaker.State() == resilience.BreakerClosed {
		t.Errorf("the wrapped store's failure did not open the breaker")
	}
}
