// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// The point of the default provider is that it refuses rather than answers. An empty result would
// satisfy "all core features stay available" and quietly break the other half of QS-09, which is
// that AI endpoints *say* 503 - so the refusal is what this test pins, both calls, by category and
// by detail code.
func TestTheDefaultProviderRefusesRatherThanAnsweringEmpty(t *testing.T) {
	provider := ai.Noop{}

	for _, call := range []struct {
		name string
		run  func() error
	}{
		{"complete", func() error {
			_, err := provider.Complete(context.Background(), port.CompletionRequest{})
			return err
		}},
		{"embed", func() error {
			_, err := provider.Embed(context.Background(), []string{"anything"})
			return err
		}},
	} {
		t.Run(call.name, func(t *testing.T) {
			err := call.run()
			if !errors.Is(err, shared.ErrUnavailable) {
				t.Fatalf("the default provider answered %v, not an unavailable dependency", err)
			}
			if got := shared.AsError(err).DetailCode; got != "ai.unavailable" {
				t.Errorf("detail code %q, want ai.unavailable - QS-09 names it", got)
			}
			if got := shared.AsError(err).Category; got != shared.CategoryUnavailable {
				t.Errorf("category %q, want unavailable so that it reaches the wire as 503", got)
			}
		})
	}
}

func TestTheDefaultProviderCanDoNothingAndSaysSo(t *testing.T) {
	capabilities := ai.Noop{}.Capabilities()

	if capabilities.Enabled() {
		t.Error("the default provider reports itself enabled")
	}
	if capabilities.Kind != ai.NoopKind {
		t.Errorf("kind %q, want %q - it is a metric label", capabilities.Kind, ai.NoopKind)
	}
	if capabilities.Completion || capabilities.Embedding {
		t.Error("the default provider claims an ability it does not have")
	}
}

// `disabled` is not `down`, and the difference is a decision somebody made against an outage
// somebody has. An installation nothing has called an AI provider from must not report itself
// degraded, or the alert catalogue teaches its operator to ignore the row.
func TestAnInstallationCallingNoProviderIsDisabledRatherThanDown(t *testing.T) {
	for _, testCase := range []struct {
		name string
		pool *ai.BreakerPool
	}{
		{"no pool at all", nil},
		{"a pool nothing has used", &ai.BreakerPool{New: func(string) ai.Breaker { return nil }}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := ai.NewProbe(testCase.pool).Check(context.Background())

			if result.Status != health.StatusDisabled {
				t.Fatalf("status %q, want %q", result.Status, health.StatusDisabled)
			}
			if len(result.Impact) != 0 {
				t.Errorf("it degrades %v; it should degrade nothing", result.Impact)
			}
			if result.ErrorCode != "" {
				t.Errorf("it reports the error %q", result.ErrorCode)
			}
		})
	}
}

func TestTheProbeIsOptionalAndNamedForTheMetric(t *testing.T) {
	probe := ai.NewProbe(nil)

	if probe.Required() {
		t.Error("the AI provider is required; the failure of an optional dependency must never block the write path")
	}
	if probe.Name() != port.Dependency {
		t.Errorf("probe name %q, want %q", probe.Name(), port.Dependency)
	}
}

// An endpoint whose breaker has opened is the one state that degrades a feature - and it degrades
// exactly one, named for what a person loses rather than for the vendor.
func TestAnOpenBreakerDegradesSuggestionsAndNothingElse(t *testing.T) {
	pool := &ai.BreakerPool{New: func(string) ai.Breaker {
		return resilience.NewBreaker(resilience.BreakerConfig{
			Dependency: port.Dependency, FailureThreshold: 2, SuccessThreshold: 1,
			OpenFor: 30 * time.Second,
		})
	}}
	breaker := pool.For("https://api.example.org/v1")
	dead := func(context.Context) error { return shared.ErrUnavailable }
	for range 2 {
		if err := breaker.Do(context.Background(), dead); err == nil {
			t.Fatal("the dead dependency answered")
		}
	}

	result := ai.NewProbe(pool).Check(context.Background())

	if result.Status != health.StatusDown {
		t.Fatalf("status %q, want %q", result.Status, health.StatusDown)
	}
	if len(result.Impact) != 1 || result.Impact[0] != port.Feature {
		t.Errorf("degraded features %v, want exactly [%s]", result.Impact, port.Feature)
	}
	if result.ErrorCode != "dependency.unavailable" {
		t.Errorf("error code %q, want dependency.unavailable", result.ErrorCode)
	}
}

// A pool that has handed out a breaker which is closed is an installation calling a provider that
// works, and that is `ok` rather than `disabled`.
func TestAWorkingEndpointIsOkRatherThanDisabled(t *testing.T) {
	pool := &ai.BreakerPool{New: func(string) ai.Breaker {
		return resilience.NewBreaker(resilience.BreakerConfig{Dependency: port.Dependency})
	}}
	pool.For("https://api.example.org/v1")

	if result := ai.NewProbe(pool).Check(context.Background()); result.Status != health.StatusOK {
		t.Fatalf("status %q, want %q", result.Status, health.StatusOK)
	}
}

// The pool is bounded, because its key comes from a tenant's configuration. What it must never do
// is grow without limit.
func TestThePoolIsBounded(t *testing.T) {
	pool := &ai.BreakerPool{Cap: 4, New: func(string) ai.Breaker { return nil }}

	for index := range 20 {
		pool.For("https://endpoint-" + strconv.Itoa(index) + ".example.org")
	}
	if size := pool.Size(); size > 4 {
		t.Errorf("the pool holds %d breakers with a cap of 4", size)
	}
}

// The same endpoint gets the same breaker, or a workspace's provider would start every call with a
// closed breaker and the guard would never engage.
func TestOneEndpointGetsOneBreaker(t *testing.T) {
	built := 0
	pool := &ai.BreakerPool{New: func(string) ai.Breaker {
		built++
		return nil
	}}

	pool.For("https://api.example.org/v1")
	pool.For("https://api.example.org/v1")
	pool.For("https://other.example.org/v1")

	if built != 2 {
		t.Errorf("%d breakers built for two endpoints", built)
	}
}
