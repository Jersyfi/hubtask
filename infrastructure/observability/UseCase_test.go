// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"errors"
	"regexp"
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

// Every category keeps its own value. The two rows that matter are rate_limited and unavailable:
// counted as `internal` they would report a defect that did not happen, and an alert on "our
// fault" would page on a system doing exactly what it was configured to do (§1, §4.1).
func TestEveryCategoryKeepsItsOwnResult(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "no error", err: nil, want: "ok"},
		{name: "validation", err: shared.ErrValidation, want: "validation"},
		{name: "a malformed request is still validation", err: shared.ErrMalformedRequest, want: "validation"},
		{name: "not found", err: shared.ErrNotFound, want: "not_found"},
		{name: "gone", err: shared.ErrGone, want: "gone"},
		{name: "conflict", err: shared.ErrConflict, want: "conflict"},
		{name: "a version conflict is still a conflict", err: shared.ErrVersionConflict, want: "conflict"},
		{name: "forbidden", err: shared.ErrForbidden, want: "forbidden"},
		{name: "unauthenticated", err: shared.ErrUnauthenticated, want: "unauthenticated"},
		{name: "rate limited", err: shared.ErrRateLimited, want: "rate_limited"},
		{name: "unavailable", err: shared.ErrUnavailable, want: "unavailable"},
		{name: "internal", err: shared.ErrInternal, want: "internal"},
		// Anything that is not a domain error is ours by definition: an error nobody typed is
		// an error nobody expected.
		{name: "a foreign error", err: errors.New("boom"), want: "internal"},
		// A category nobody defined must not travel to the metrics as itself - AsError
		// normalises it, which is what keeps the label set closed.
		{name: "an invented category", err: shared.New("MADE_UP", "whatever"), want: "internal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResultClass(tc.err); got != tc.want {
				t.Errorf("ResultClass = %q, want %q", got, tc.want)
			}
		})
	}
}

// The label set is closed and it is the domain that closes it. This walks every category the
// domain defines, so a tenth one added later cannot quietly disappear into `internal` - it turns
// up here as a value the catalogue does not list yet.
func TestTheResultSetIsExactlyTheDomainCategories(t *testing.T) {
	// The values documented in observability-reliability.md §4.1. The list is here so that the
	// document and the code have to be changed together.
	documented := map[string]bool{
		"ok": true, "validation": true, "not_found": true, "conflict": true,
		"forbidden": true, "unauthenticated": true, "gone": true,
		"rate_limited": true, "unavailable": true, "internal": true,
	}

	produced := map[string]bool{ResultOK: true}
	for _, category := range shared.Categories() {
		produced[ResultClass(shared.New(category, "any"))] = true
	}

	for value := range produced {
		if !documented[value] {
			t.Errorf("the result value %q is produced but not documented in §4.1", value)
		}
	}
	for value := range documented {
		if !produced[value] {
			t.Errorf("the result value %q is documented in §4.1 but nothing produces it", value)
		}
	}
}

// A label value has to be a metric token, whatever the domain calls its categories: lower case,
// no spaces, nothing that would need quoting in a PromQL matcher.
func TestEveryResultValueIsAMetricToken(t *testing.T) {
	token := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	values := []string{ResultOK}
	for _, category := range shared.Categories() {
		values = append(values, ResultClass(shared.New(category, "any")))
	}
	for _, value := range values {
		if !token.MatchString(value) {
			t.Errorf("the result value %q is not a metric token", value)
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
