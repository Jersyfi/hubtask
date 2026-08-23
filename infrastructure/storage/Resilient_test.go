// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	health "github.com/Jersyfi/hubtask/core/port/health"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// flakyStore fails on demand, so the composition around it can be watched.
type flakyStore struct {
	failing atomic.Bool
	calls   atomic.Int64
	release chan struct{}
}

func (f *flakyStore) Put(ctx context.Context, _ port.Upload) error {
	f.calls.Add(1)
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.failing.Load() {
		return shared.ErrUnavailable.
			WithDetail("dependency.unavailable").
			WithParams(map[string]string{"dependency": s3Dependency})
	}
	return nil
}

func (f *flakyStore) Get(context.Context, string) (port.Object, error) {
	return port.Object{}, shared.ErrNotFound
}

func (f *flakyStore) Delete(context.Context, string) error { return nil }

func put(store port.ObjectStore) error {
	return store.Put(context.Background(), port.Upload{
		Key: "media/one", Content: strings.NewReader("x"), Size: 1, ContentType: "text/plain",
	})
}

func TestTheBreakerCutsOffADeadEndpointAndTheProbeSaysSo(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	var offset atomic.Int64
	clock := func() time.Time { return now.Add(time.Duration(offset.Load())) }

	inner := &flakyStore{}
	inner.failing.Store(true)
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: s3Dependency, FailureThreshold: 2, SuccessThreshold: 1,
		OpenFor: 30 * time.Second, Now: clock,
	})
	store := NewResilientStore(inner, breaker,
		resilience.NewBulkhead(resilience.BulkheadConfig{Name: "s3", Capacity: 2}))
	probe := NewProbe(breaker)

	if got := probe.Check(context.Background()); got.Status != health.StatusOK {
		t.Fatalf("a fresh breaker reports %+v", got)
	}

	for range 2 {
		if err := put(store); err == nil {
			t.Fatal("the dead endpoint answered")
		}
	}

	before := inner.calls.Load()
	err := put(store)
	if got := shared.AsError(err).DetailCode; got != "dependency.circuit_open" {
		t.Fatalf("the open breaker answered %q", got)
	}
	if inner.calls.Load() != before {
		t.Fatal("the open breaker still called the endpoint")
	}

	report := probe.Check(context.Background())
	if report.Status != health.StatusDown || report.ErrorCode != "dependency.unavailable" {
		t.Fatalf("the probe reports %+v", report)
	}
	if len(report.Impact) != 1 || report.Impact[0] != "media" {
		t.Fatalf("the impact is %v, want the media feature", report.Impact)
	}
	if report.Since.IsZero() {
		t.Fatal("the report carries no timestamp")
	}

	// Recovery without a restart (QS-11): the cool-down passes, the endpoint returns, one probe
	// call closes the breaker.
	inner.failing.Store(false)
	offset.Store(int64(31 * time.Second))
	if err := put(store); err != nil {
		t.Fatalf("the recovered endpoint was refused: %v", err)
	}
	if got := probe.Check(context.Background()); got.Status != health.StatusOK ||
		len(got.Impact) != 0 {
		t.Fatalf("after recovery the probe reports %+v", got)
	}
}

func TestAFullPoolFailsFastAndIsNotTheEndpointsFault(t *testing.T) {
	inner := &flakyStore{release: make(chan struct{})}
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: s3Dependency, FailureThreshold: 2,
	})
	store := NewResilientStore(inner, breaker,
		resilience.NewBulkhead(resilience.BulkheadConfig{Name: "s3", Capacity: 1}))

	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})
	concurrency.Go(context.Background(), "test.storage.occupant", func(context.Context) {
		defer wg.Done()
		close(started)
		_ = put(store)
	})
	<-started
	// Give the first call its slot before contending for it.
	for inner.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}

	err := put(store)
	if got := shared.AsError(err).DetailCode; got != "dependency.saturated" {
		t.Fatalf("the full pool answered %q", got)
	}

	close(inner.release)
	wg.Wait()

	// The saturation was this process's state, not the dependency's: the breaker must not have
	// counted it, so the next call still reaches the endpoint.
	if err := put(store); err != nil {
		t.Fatalf("the pool's saturation was charged to the breaker: %v", err)
	}
}
