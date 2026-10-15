// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// A subscription narrowed by a condition and a scope (UC-INT-01 check 9, automation.md §3.1).
//
// The condition is the automation's expression language over what the dispatcher may know without
// reading the work context: the event, its CloudEvent `data`, the actor and the moment
// (condition.WebhookEnvironment). The scope is a rule's: the workspace, one hub - its collections
// included - or one collection, and writing one asks the automation permission there.

// compileFilter answers whether a condition can be compiled, as a refusal naming where it cannot.
// Compiled and discarded: the dispatcher compiles its own, and keeping a program here would be a
// cache with the wrong lifetime.
func (w Writer) compileFilter(filter string) error {
	if filter == "" {
		return nil
	}
	if w.Compiler == nil {
		// Fail closed, as the rules do: a build with no evaluator wired cannot promise that a
		// condition means what it says, and storing one on that promise is the failure this
		// prevents.
		return shared.ErrUnavailable.WithDetail("webhooks.filter_unavailable")
	}
	_, err := w.Compiler.Compile(filter, condition.WebhookEnvironment(), expression.Boolean)
	if err == nil {
		return nil
	}
	finding := shared.FieldError{Path: "/filter", Code: expression.CodeSyntax}
	var coded *shared.Error
	if errors.As(err, &coded) {
		finding.Code = coded.DetailCode
		if len(coded.Params) > 0 {
			finding.Params = coded.Params
		}
	}
	return shared.ErrValidation.WithDetail("webhooks.filter_invalid").WithFields(finding)
}

// checkScope answers whether the container a scope names is this workspace's, and of the level the
// scope says. Asked after the authoriser, inside the workspace's transaction: the authoriser judges
// a path it cannot complete as it was given, so a container of another workspace would otherwise
// pass for an administrator of this one.
func (w Writer) checkScope(ctx context.Context, scope domain.WebhookScope) error {
	if scope.IsWorkspace() {
		return nil
	}
	if w.Containers == nil {
		return shared.ErrUnavailable.WithDetail("webhooks.scope_unavailable")
	}
	container, err := w.Containers.Find(ctx, scope.ID)
	if errors.Is(err, shared.ErrNotFound) {
		return appshared.ContainerNotFound(scope.ID)
	}
	if err != nil {
		return err
	}
	want := work.ContainerHub
	if scope.Level == domain.ScopeCollection {
		want = work.ContainerCollection
	}
	if container.Type != want {
		return shared.ErrValidation.WithDetail("webhooks.scope_type_mismatch").
			WithParams(map[string]string{"type": string(scope.Level)}).
			WithFields(shared.FieldError{Path: "/scope/type", Code: "webhooks.scope_type_mismatch"})
	}
	return nil
}

// authorizeAt is the one permission question a webhook write asks, at the scope it concerns.
//
// AUTOMATION rather than MANAGE_MEMBERS: subscribing an external system to the event stream and
// writing a rule that reacts to it are the same power over the same stream.
func (w Writer) authorizeAt(
	ctx context.Context, actor appshared.ActorContext, scope domain.WebhookScope,
	action audit.Action, target shared.ID,
) error {
	return w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       scope.Path(),
		Action:     action,
		TokenScope: automationScope,
		TargetType: webhookTarget,
		TargetID:   target,
	})
}

// authorizeReadAt is the same question with the auditor's answer added (A-4): where a workspace's
// events are being sent is configuration.
func (w Writer) authorizeReadAt(
	ctx context.Context, actor appshared.ActorContext, scope domain.WebhookScope,
	action audit.Action, target shared.ID,
) error {
	return w.Authorizer.Authorize(ctx, actor, readRequest(scope, action, target))
}

// permitsReadAt is the listing's quiet question: may this caller see subscriptions at this scope -
// answered without an entry, as the rules' listing does.
func (w Writer) permitsReadAt(
	ctx context.Context, actor appshared.ActorContext, scope domain.WebhookScope,
) (bool, error) {
	return w.Authorizer.Permits(ctx, actor, readRequest(scope, "", ""))
}

func readRequest(scope domain.WebhookScope, action audit.Action, target shared.ID) access.Request {
	return access.Request{
		Permission:  service.PermissionAutomation,
		Alternative: service.PermissionReadConfiguration,
		Path:        scope.Path(),
		Action:      action,
		TokenScope:  automationScope,
		TargetType:  webhookTarget,
		TargetID:    target,
	}
}

// scopeOf reads the scope of the subscription an operation names, so that it is authorised where
// the subscription listens. A subscription that is not there - or is another workspace's - answers
// the workspace, and the operation's own read then answers it as absent, which is what one that
// never existed answers too (T-04).
func (w Writer) scopeOf(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.WebhookScope, error) {
	scope := domain.WebhookScope{Level: domain.ScopeTenant}
	if id.IsZero() {
		return scope, nil
	}
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, id)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		scope = stored.Subscription.Scope
		return nil
	})
	return scope, err
}

// authorizeSubscription asks the write's question where the named subscription listens.
func (w Writer) authorizeSubscription(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID,
) error {
	scope, err := w.scopeOf(ctx, actor, id)
	if err != nil {
		return err
	}
	return w.authorizeAt(ctx, actor, scope, action, id)
}

// authorizeSubscriptionRead asks the read's question where the named subscription listens.
func (w Writer) authorizeSubscriptionRead(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID,
) error {
	scope, err := w.scopeOf(ctx, actor, id)
	if err != nil {
		return err
	}
	return w.authorizeReadAt(ctx, actor, scope, action, id)
}

// scopeInput reads `scope` as the channels send it: {type: TENANT|HUB|COLLECTION, id}. Absent is
// nil - the workspace on a creation, nothing changed on an update.
func scopeInput(value any) (*domain.WebhookScope, error) {
	if value == nil {
		return nil, nil
	}
	document, ok := value.(map[string]any)
	if !ok {
		return nil, shared.ErrValidation.WithDetail("webhooks.scope_type_unknown").
			WithFields(shared.FieldError{Path: "/scope", Code: "webhooks.scope_type_unknown"})
	}
	level, _ := document["type"].(string)
	scope := domain.WebhookScope{Level: domain.ScopeLevel(level)}
	if raw, _ := document["id"].(string); raw != "" {
		parsed, err := shared.ParseID(raw)
		if err != nil {
			return nil, shared.ErrValidation.WithDetail("webhooks.scope_id_invalid").
				WithFields(shared.FieldError{Path: "/scope/id", Code: "webhooks.scope_id_invalid"})
		}
		scope.ID = parsed
	}
	valid, err := domain.ValidWebhookScope(scope)
	if err != nil {
		return nil, err
	}
	return &valid, nil
}

// scopeOutput is the scope as every channel reads it.
func scopeOutput(scope domain.WebhookScope) map[string]any {
	if scope.IsWorkspace() {
		return map[string]any{"type": string(domain.ScopeTenant)}
	}
	return map[string]any{"type": string(scope.Level), "id": scope.ID.String()}
}
