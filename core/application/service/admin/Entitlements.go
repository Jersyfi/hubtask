// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// EntitlementGate refuses an operation whose feature the caller's workspace's plan leaves out
// (multi-tenancy.md §4.2): one place, on the registry every door comes through - REST, MCP and a
// rule's action - rather than a check in each of the operations that declare a feature.
//
// It asks before the operation's own authorisation does. What a plan leaves out is no secret in
// the workspace - `/meta/capabilities` answers it to every signed-in person there - and the
// refusal names only the feature.
type EntitlementGate struct {
	Plans      repository.Entitlements
	UnitOfWork persistence.UnitOfWork
}

// LeftOut answers what the actor's workspace's plan leaves out, in its own read - which joins the
// transaction the caller already runs in, as a rule's action does. An actor without a workspace,
// or a gate wired without a reader, reads an empty set.
func (g EntitlementGate) LeftOut(ctx context.Context, actor appshared.ActorContext) (entitlement.LeftOut, error) {
	if g.Plans == nil || g.UnitOfWork == nil || actor.TenantID.IsZero() {
		return nil, nil
	}
	var leftOut entitlement.LeftOut
	err := g.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		leftOut, err = g.Plans.LeftOut(ctx)
		return err
	})
	return leftOut, err
}

// Wrap puts the gate inside the composition root's middleware, so that the metric and the span
// still count an operation the plan refuses.
func (g EntitlementGate) Wrap(observe usecase.Middleware) usecase.Middleware {
	return func(descriptor usecase.Descriptor, next usecase.Handler) usecase.Handler {
		handler := next
		if descriptor.Entitlement != "" {
			feature := descriptor.Entitlement
			handler = usecase.HandlerFunc(func(
				ctx context.Context, actor appshared.ActorContext, in usecase.Input,
			) (usecase.Output, error) {
				leftOut, err := g.LeftOut(ctx, actor)
				if err != nil {
					return nil, err
				}
				if err := leftOut.Require(feature); err != nil {
					return nil, err
				}
				return next.Invoke(ctx, actor, in)
			})
		}
		if observe == nil {
			return handler
		}
		return observe(descriptor, handler)
	}
}
