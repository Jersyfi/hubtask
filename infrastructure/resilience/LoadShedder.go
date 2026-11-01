// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Class says how much a piece of work can wait. It is the only input the load shedder has
// besides the current load, and it is deliberately coarse: two answers an endpoint can give
// correctly, rather than a priority number nobody can calibrate.
type Class string

const (
	// ClassInteractive is a user waiting for an answer. Never shed here - a person who cannot
	// tick off a task retries by hand, which adds load rather than removing it. Overload of the
	// interactive path is what rate limits and the bulkhead are for.
	ClassInteractive Class = "interactive"
	// ClassDeferrable is work that can be repeated later without anybody watching: bulk
	// operations, exports, search, imports (observability-reliability.md §6).
	ClassDeferrable Class = "deferrable"
)

// LoadShedderConfig configures the admission threshold.
type LoadShedderConfig struct {
	// Limit is the number of in-flight calls above which deferrable work is rejected. It is a
	// concurrency limit, not a rate: what tips latency over is how much is running at once.
	Limit int
	// RetryAfter is what the client is told to wait. It reaches the response as a parameter,
	// which the REST layer turns into the Retry-After header (api-guidelines.md §6).
	RetryAfter time.Duration
	// OnShed is called for each rejected call, for the metric.
	OnShed func(class Class)
	// TenantShare is the divisor of Limit that one workspace's deferrable calls may hold at once
	// (AdmitTenant). Zero is four: one workspace takes at most a quarter of the threshold, so its
	// flood cannot by itself push the process over it and shed everybody else.
	TenantShare int
}

// LoadShedder rejects deferrable work while the process is busy, before latency tips over for
// everyone (observability-reliability.md §6).
//
// It counts every admitted call, interactive work included: the load an export competes with is
// the whole load, not just the load from other exports. What differs is who gets turned away.
type LoadShedder struct {
	cfg      LoadShedderConfig
	inflight atomic.Int64

	mu     sync.Mutex
	tenant map[string]int
}

// NewLoadShedder builds a shedder. A limit below one would shed every deferrable call, so it is
// raised to one - a threshold that rejects everything is a defect, not a configuration.
func NewLoadShedder(cfg LoadShedderConfig) *LoadShedder {
	if cfg.Limit < 1 {
		cfg.Limit = 1
	}
	if cfg.RetryAfter <= 0 {
		cfg.RetryAfter = 5 * time.Second
	}
	if cfg.TenantShare < 1 {
		cfg.TenantShare = 4
	}
	return &LoadShedder{cfg: cfg, tenant: map[string]int{}}
}

// Admit registers a call and reports whether it may run. The returned release must be called
// when the work is done - a middleware defers it, because the response is written long after
// the decision was made.
//
// On rejection the release is a no-op rather than nil, so that a caller can defer it
// unconditionally instead of guarding every path.
func (s *LoadShedder) Admit(class Class) (release func(), err error) {
	current := s.inflight.Add(1)
	if class != ClassInteractive && current > int64(s.cfg.Limit) {
		s.inflight.Add(-1)
		if s.cfg.OnShed != nil {
			s.cfg.OnShed(class)
		}
		return func() {}, ShedError(class, s.cfg.RetryAfter)
	}

	var released atomic.Bool
	return func() {
		// Guarded against a double release: a middleware that both defers the release and
		// calls it on an early return would otherwise let the counter drift below zero, and
		// from then on nothing is ever shed.
		if released.CompareAndSwap(false, true) {
			s.inflight.Add(-1)
		}
	}, nil
}

// Do runs fn if the current load allows it.
func (s *LoadShedder) Do(ctx context.Context, class Class, fn func(context.Context) error) error {
	release, err := s.Admit(class)
	if err != nil {
		return err
	}
	defer release()
	return fn(ctx)
}

// AdmitTenant registers one deferrable call of a workspace and refuses it while that workspace
// already holds its share of the threshold. It is the second stage, after authentication has
// named the workspace: Admit protects the process, this protects the other workspaces on it from
// one workspace's flood (observability-reliability.md §6).
//
// The map holds only workspaces with a call in flight, so it is bounded by the in-flight count
// rather than by the number of workspaces.
func (s *LoadShedder) AdmitTenant(tenant string) (release func(), err error) {
	share := max(s.cfg.Limit/s.cfg.TenantShare, 1)
	s.mu.Lock()
	if s.tenant[tenant] >= share {
		s.mu.Unlock()
		return func() {}, ShedError(ClassDeferrable, s.cfg.RetryAfter)
	}
	s.tenant[tenant]++
	s.mu.Unlock()

	var released atomic.Bool
	return func() {
		if !released.CompareAndSwap(false, true) {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.tenant[tenant] <= 1 {
			delete(s.tenant, tenant)
			return
		}
		s.tenant[tenant]--
	}, nil
}

// InFlight is how many calls are currently admitted. The gauge reads it.
func (s *LoadShedder) InFlight() int64 { return s.inflight.Load() }

// Limit is the threshold above which deferrable work is shed.
func (s *LoadShedder) Limit() int { return s.cfg.Limit }

// ShedError is what a rejected call gets: UNAVAILABLE, hence 503, with the wait in a parameter
// so the REST layer can set Retry-After. Not RATE_LIMITED - the client did nothing wrong and
// has no budget to respect; the process is busy.
func ShedError(class Class, retryAfter time.Duration) *shared.Error {
	// At least a second: Retry-After: 0 invites the client to come straight back, which is the
	// opposite of what shedding is for.
	seconds := max(int(retryAfter.Round(time.Second).Seconds()), 1)
	return shared.ErrUnavailable.
		WithDetail("capacity.shed").
		WithParams(map[string]string{
			"class":               string(class),
			"retry_after_seconds": strconv.Itoa(seconds),
		})
}
