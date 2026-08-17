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
