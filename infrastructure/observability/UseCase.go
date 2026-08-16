// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Result classes of hubtask_usecase_total (observability-reliability.md §4). Five values, so the
// series count per use case is five and not "however many error codes exist this month".
const (
	ResultOK         = "ok"
	ResultValidation = "validation"
	ResultConflict   = "conflict"
	ResultForbidden  = "forbidden"
	ResultInternal   = "internal"
)

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

	o.metrics.UseCase(ctx, name, result, TenantFromContext(ctx))
	return err
}

// ResultClass reduces an error to one of the five classes of §4.
//
// The domain has nine categories and the catalogue lists five values, so the collapse happens
// here rather than by inventing labels: everything the caller got wrong that is not a conflict
// or a permission counts as validation, and everything the server owes counts as internal. The
// finer picture is in the HTTP metrics, which keep the status class, and in the audit trail.
func ResultClass(err error) string {
	if err == nil {
		return ResultOK
	}
	domainErr := shared.AsError(err)
	if domainErr == nil {
		return ResultInternal
	}
	switch domainErr.Category {
	case shared.CategoryValidation, shared.CategoryNotFound, shared.CategoryGone:
		return ResultValidation
	case shared.CategoryConflict:
		return ResultConflict
	case shared.CategoryForbidden, shared.CategoryUnauthenticated:
		return ResultForbidden
	case shared.CategoryRateLimited, shared.CategoryUnavailable, shared.CategoryInternal:
		return ResultInternal
	default:
		return ResultInternal
	}
}
