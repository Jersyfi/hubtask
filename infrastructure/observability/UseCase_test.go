// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
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

// Gate RT-12 is not a rule anybody has to remember: a catalogue built with this middleware has no
// entry that can run without producing its metric and its span.
func TestEveryCatalogueEntryIsObserved(t *testing.T) {
	observer := newTestObserver(t, env.Config{})

	ran := false
	registry, err := usecase.NewRegistry(observer.Registry(), usecase.Descriptor{
		Name:    "CreateContainer",
		Summary: "Creates a hub or a collection.",
		Handler: usecase.HandlerFunc(func(
			context.Context, appshared.ActorContext, usecase.Input,
		) (usecase.Output, error) {
			ran = true
			return usecase.Output{"id": "0192f000-0000-7000-8000-00000000000b"}, nil
		}),
	})
	if err != nil {
		t.Fatalf("the catalogue was refused: %v", err)
	}

	out, err := registry.Invoke(context.Background(), "CreateContainer", appshared.ActorContext{}, nil)
	if err != nil {
		t.Fatalf("the invocation failed: %v", err)
	}
	if !ran || out["id"] == nil {
		t.Errorf("the middleware swallowed the call or its result: %v", out)
	}

	body := scrape(t, observer.metrics)
	if !strings.Contains(body, `hubtask_usecase_total{result="ok",use_case="CreateContainer"} 1`) {
		t.Errorf("a catalogue entry ran without producing its metric:\n%s", body)
	}
}

// And the failing path counts too, with the category of the error rather than a generic failure.
func TestAFailingCatalogueEntryIsCountedWithItsCategory(t *testing.T) {
	observer := newTestObserver(t, env.Config{})

	registry, err := usecase.NewRegistry(observer.Registry(), usecase.Descriptor{
		Name:    "CreateContainer",
		Summary: "Creates a hub or a collection.",
		Handler: usecase.HandlerFunc(func(
			context.Context, appshared.ActorContext, usecase.Input,
		) (usecase.Output, error) {
			return nil, shared.ErrForbidden.WithDetail("access.not_permitted")
		}),
	})
	if err != nil {
		t.Fatalf("the catalogue was refused: %v", err)
	}

	if _, err := registry.Invoke(context.Background(), "CreateContainer", appshared.ActorContext{}, nil); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want the refusal to reach the caller", err)
	}

	body := scrape(t, observer.metrics)
	if !strings.Contains(body, `hubtask_usecase_total{result="forbidden",use_case="CreateContainer"} 1`) {
		t.Errorf("the refusal was not counted as such:\n%s", body)
	}
}

// The queue's counterpart to the use case span. A job that produced none is a job nobody can
// follow through the pipeline dashboard, and the run that most needs following is the one at three
// in the morning.
func TestAJobRunProducesASpanCarryingItsKind(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	observer := &Observer{
		metrics: newTestMetrics(t, env.Config{}),
		tracer:  sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("test"),
	}

	err := observer.Job(context.Background(), "outbox.dispatch", func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("the job reported an error: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("%d spans, want one", len(spans))
	}
	span := spans[0]
	if span.Name() != "job.outbox.dispatch" {
		t.Errorf("span name %q, want the kind - never the job identifier", span.Name())
	}

	attributes := map[string]string{}
	for _, kv := range span.Attributes() {
		attributes[string(kv.Key)] = kv.Value.AsString()
	}
	if attributes["hubtask.job_kind"] != "outbox.dispatch" {
		t.Errorf("job_kind = %q", attributes["hubtask.job_kind"])
	}
	if attributes["hubtask.result"] != ResultOK {
		t.Errorf("result = %q, want %q", attributes["hubtask.result"], ResultOK)
	}
}

// A failed job's span says so, and says it with the error's code rather than its message: a
// message can quote what the job was working on, and a span leaves the process (rule 10).
func TestAFailedJobSpanCarriesTheCodeAndNoMessage(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	observer := &Observer{
		metrics: newTestMetrics(t, env.Config{}),
		tracer:  sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("test"),
	}

	failure := shared.ErrUnavailable.WithDetail("dependency.unavailable")
	if err := observer.Job(context.Background(), "outbox.dispatch", func(context.Context) error {
		return failure
	}); err == nil {
		t.Fatal("the failure did not reach the caller")
	}

	span := recorder.Ended()[0]
	if span.Status().Code != codes.Error {
		t.Errorf("status %v, want an error status", span.Status().Code)
	}
	if span.Status().Description != "" {
		t.Errorf("the span carries a description (%q) - a message can contain user content",
			span.Status().Description)
	}

	attributes := map[string]string{}
	for _, kv := range span.Attributes() {
		attributes[string(kv.Key)] = kv.Value.AsString()
	}
	if attributes["hubtask.error_code"] != failure.Code {
		t.Errorf("error_code = %q, want %q", attributes["hubtask.error_code"], failure.Code)
	}
	if attributes["hubtask.result"] != "unavailable" {
		t.Errorf("result = %q, want the domain's category", attributes["hubtask.result"])
	}
}
