// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// ResultOK is the `result` value of a use case that succeeded. Every other value is an error
// category of the domain error model, lower-cased - see ResultClass.
const ResultOK = "ok"

// Observer gives one use case execution its metric and its span - the two signals the Definition
// of Done requires of every use case, and what gate RT-12 checks for (ADR-0016 §6).
//
// It is a wrapper rather than two calls at each call site, because two calls are two chances to
// forget one, and the one that gets forgotten is the error path.
type Observer struct {
	metrics *Metrics
	tracer  trace.Tracer
}

func NewObserver(metrics *Metrics, tracing *Tracing) *Observer {
	return &Observer{metrics: metrics, tracer: tracing.Tracer("usecase")}
}

// UseCase runs fn as one observed use case. name is the registry name (CreateContainer), never
// anything derived from input - it is a metric label and a span name.
func (o *Observer) UseCase(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, span := o.tracer.Start(ctx, name)
	defer span.End()

	span.SetAttributes(attribute.String("hubtask.use_case", name))

	err := fn(ctx)
	result := ResultClass(err)

	if err != nil {
		// The status carries no description. A message can quote a title, a note, or a
		// comment, and a span goes to a third-party backend (rule 10, ADR-0018).
		span.SetStatus(codes.Error, "")
		if domainErr := shared.AsError(err); domainErr != nil {
			span.SetAttributes(attribute.String("hubtask.error_code", domainErr.Code))
		}
	}
	span.SetAttributes(attribute.String("hubtask.result", result))

	o.metrics.UseCase(ctx, name, result, correlation.TenantFrom(ctx))
	return err
}

// ResultClass is the `result` label of hubtask_usecase_total: ok, or the error category in lower
// case (observability-reliability.md §4.1).
//
// It derives rather than translates, and that is the whole design. A mapping table would be a
// second classification of the same failure, and the two would disagree the moment the domain
// grows a tenth category - which would then be folded into `internal` by a `default` branch and
// vanish. Deriving means a new category appears in the metrics by itself.
//
// The coarse view an alert wants is a query, not a label: `result=~"internal|unavailable"`.
// Folding rate_limited into internal here would save that regular expression and cost the
// distinction between a defect and a limit doing its job - which is the one an alert must not
// get wrong (§1, "alert on symptoms").
//
// AsError normalises anything unclassified to INTERNAL, so the result is total: every error
// produces exactly one of the values the domain defines.
func ResultClass(err error) string {
	if err == nil {
		return ResultOK
	}
	return strings.ToLower(string(shared.AsError(err).Category))
}

// Registry returns the middleware the use case catalogue is built with.
//
// This is what makes gate RT-12 structural rather than a rule people remember: the composition
// root passes it to usecase.NewRegistry, every entry's handler is wrapped on the way in, and
// there is then no path to a use case that produces neither a metric nor a span - whether the
// call arrived through REST, through MCP, or from an automation rule.
func (o *Observer) Registry() usecase.Middleware {
	return func(descriptor usecase.Descriptor, next usecase.Handler) usecase.Handler {
		return usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			var out usecase.Output
			err := o.UseCase(ctx, descriptor.Name, func(ctx context.Context) error {
				var err error
				out, err = next.Invoke(ctx, actor, in)
				return err
			})
			return out, err
		})
	}
}

// Job runs one job execution as an observed unit (ADR-0008, §3.3).
//
// It is the queue's counterpart to UseCase, and it exists for the same reason: a job that produced
// no span is a job nobody can follow through the pipeline dashboard, and the failure that most
// needs following is the one that happened at three in the morning.
//
// What it does not do yet is continue the trace of whatever caused the job. §3.3 wants the
// traceparent persisted in the event and the job row, so that a chain of HTTP request -> event ->
// automation rule -> webhook is one trace; that needs a column and belongs with the dispatcher
// work in 0.5.0. Until then a job span is its own root, which is enough to see what a job did and
// how long it took, and not enough to see what asked for it.
func (o *Observer) Job(ctx context.Context, kind string, fn func(context.Context) error) error {
	// The span is named after the kind rather than the job: a name per job identifier would be a
	// new operation in every tracing backend for every row (§3.2 applies to span names too).
	ctx, span := o.tracer.Start(ctx, "job."+kind)
	defer span.End()

	span.SetAttributes(attribute.String("hubtask.job_kind", kind))
	if tenant := correlation.TenantFrom(ctx); tenant != "" {
		span.SetAttributes(attribute.String("hubtask.tenant_id", tenant))
	}

	err := fn(ctx)
	if err != nil {
		// As above: no description on the status. A message can quote what the job was working
		// on, and a span leaves the process (rule 10).
		span.SetStatus(codes.Error, "")
		if domainErr := shared.AsError(err); domainErr != nil {
			span.SetAttributes(attribute.String("hubtask.error_code", domainErr.Code))
		}
	}
	span.SetAttributes(attribute.String("hubtask.result", ResultClass(err)))
	return err
}
