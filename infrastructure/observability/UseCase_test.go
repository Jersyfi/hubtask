// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

func newTestObserver(t *testing.T, cfg env.Config) *Observer {
	t.Helper()
	metrics := newTestMetrics(t, cfg)
	tracing, err := NewTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("building the tracing: %v", err)
	}
	t.Cleanup(func() { _ = tracing.Shutdown(context.Background()) })
	return &Observer{metrics: metrics, tracer: tracing.Tracer("usecase")}
}

// The Definition of Done in one test: a use case that runs produces a metric.
func TestAUseCaseProducesItsMetric(t *testing.T) {
	observer := newTestObserver(t, env.Config{})

	err := observer.UseCase(context.Background(), "CreateContainer", func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("the use case failed: %v", err)
	}

	body := scrape(t, observer.metrics)
	if !strings.Contains(body, `hubtask_usecase_total{result="ok",use_case="CreateContainer"} 1`) {
		t.Errorf("the use case was not counted:\n%s", body)
	}
}

// The error path is the one that gets forgotten when the metric is a second call at the call
// site. Here it cannot be: the wrapper counts whatever fn returns.
func TestTheErrorPathIsCountedToo(t *testing.T) {
	observer := newTestObserver(t, env.Config{})
	want := shared.ErrConflict

	err := observer.UseCase(context.Background(), "MoveItem", func(context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("the wrapper swallowed the error: %v", err)
	}

	body := scrape(t, observer.metrics)
	if !strings.Contains(body, `result="conflict"`) {
		t.Errorf("the conflict was not classified:\n%s", body)
	}
}

func TestTheResultClassCoversEveryCategory(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "no error", err: nil, want: ResultOK},
		{name: "validation", err: shared.ErrValidation, want: ResultValidation},
		{name: "not found", err: shared.ErrNotFound, want: ResultValidation},
		{name: "gone", err: shared.ErrGone, want: ResultValidation},
		{name: "conflict", err: shared.ErrConflict, want: ResultConflict},
		{name: "version conflict", err: shared.ErrVersionConflict, want: ResultConflict},
		{name: "forbidden", err: shared.ErrForbidden, want: ResultForbidden},
		{name: "unauthenticated", err: shared.ErrUnauthenticated, want: ResultForbidden},
		{name: "rate limited", err: shared.ErrRateLimited, want: ResultInternal},
		{name: "unavailable", err: shared.ErrUnavailable, want: ResultInternal},
		{name: "internal", err: shared.ErrInternal, want: ResultInternal},
		// Anything that is not a domain error is ours by definition: an error nobody typed is
		// an error nobody expected.
		{name: "a foreign error", err: errors.New("boom"), want: ResultInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResultClass(tc.err); got != tc.want {
				t.Errorf("ResultClass = %q, want %q", got, tc.want)
			}
		})
	}
}

// The result label may only ever hold the five values of §4 - otherwise the series count per use
// case grows with the error catalogue.
func TestTheResultLabelStaysWithinTheFiveClasses(t *testing.T) {
	allowed := map[string]bool{
		ResultOK: true, ResultValidation: true, ResultConflict: true,
		ResultForbidden: true, ResultInternal: true,
	}

	for _, err := range []error{
		nil, shared.ErrValidation, shared.ErrMalformedRequest, shared.ErrUnauthenticated,
		shared.ErrForbidden, shared.ErrNotFound, shared.ErrConflict, shared.ErrVersionConflict,
		shared.ErrGone, shared.ErrRateLimited, shared.ErrUnavailable, shared.ErrInternal,
		errors.New("boom"),
	} {
		if got := ResultClass(err); !allowed[got] {
			t.Errorf("%v produced the unlisted result class %q", err, got)
		}
	}
}

// A panic inside a use case must not escape as a panic-shaped hole in the metrics: the wrapper
// leaves the recovery to the caller, but the span has to be closed either way.
func TestAPanicInsideAUseCaseStillEndsTheSpan(t *testing.T) {
	observer := newTestObserver(t, env.Config{})

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic did not travel outwards")
			}
		}()
		_ = observer.UseCase(context.Background(), "Explode", func(context.Context) error {
			panic("boom")
		})
	}()
}
