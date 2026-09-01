// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package shared holds what every use case needs regardless of its context: who is acting, in
// which tenant, with which permissions, and in which language the answer is to be rendered.
//
// The actor is a typed wrapper, not a bare context value. Business code asks the wrapper, never
// the context (project-structure.md §3) - which is what makes "who may do this" a question with
// exactly one answer and one place to look for it.
package shared

import (
	"context"
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ActorKind is the actor type the audit trail records (audit.md §2). Anonymous is the one value
// that never reaches an audit entry: an unauthenticated request performs no auditable action.
//
// An alias rather than a type of its own: an event, an audit entry and a request context have to
// spell the five kinds identically, so there is one definition of them, in the domain
// (core/domain/model/shared/Actor.go). The names are re-exported here because this is where
// application code reaches for them.
type ActorKind = shared.ActorKind

const (
	ActorAnonymous      = shared.ActorAnonymous
	ActorUser           = shared.ActorUser
	ActorServiceAccount = shared.ActorServiceAccount
	ActorAutomation     = shared.ActorAutomation
	ActorAIAgent        = shared.ActorAIAgent
	ActorSystem         = shared.ActorSystem
)

// ActorContext is the request as the application layer sees it.
//
// TenantID comes from authentication and from nowhere else - never from a path, a body, or a
// header a client controls (multi-tenancy.md §2.2). It is the value the transaction wrapper turns
// into `SET LOCAL app.tenant_id`, so anything that could set it from the request would be a way
// around row level security.
type ActorContext struct {
	Kind ActorKind
	// TenantID is empty exactly when the actor is anonymous.
	TenantID shared.ID
	// AccountID is the acting account. Empty for the system itself.
	AccountID shared.ID
	// AccountName is the label the audit trail records next to the identifier. It travels with
	// the actor because the trail denormalises it: an entry that only points at a foreign key
	// becomes unreadable once the account is deleted (audit.md §2, test AT-7). It is personal
	// data and goes nowhere else - not into a log, a metric, a span, or an event.
	AccountName string
	// TokenID identifies the credential used, for the audit trail and for revocation. Empty on an
	// interactive session.
	TokenID shared.ID
	// Scopes are the token's bounds. They are a second, independent limit on top of the role: a
	// token can never do more than its scopes allow, whatever role its owner holds (ADR-0005).
	Scopes []string
	// APIClient is the OAuth client the credential was issued to (H-05), empty for everything
	// that is not a grant session. It is what the audit trail records as the acting app.
	APIClient shared.ID
	// Locale is BCP 47, resolved request → account → tenant → installation (i18n-l10n.md §2).
	Locale string
	// TimeZone is an IANA name. Every relative date in a query and every reminder is computed in
	// it, so it travels with the actor rather than being looked up per use case.
	TimeZone string
}

// Anonymous is the actor of a request that carried no credential. It has a locale, because an
// error still has to be rendered in some language, and nothing else.
func Anonymous(locale, timeZone string) ActorContext {
	return ActorContext{Kind: ActorAnonymous, Locale: locale, TimeZone: timeZone}
}

// IsAuthenticated reports whether a credential was accepted. A tenant without an actor kind, or
// an actor kind without a tenant, is not authentication - it is a half-built context, and
// treating it as authenticated is how a boundary gets crossed.
func (a ActorContext) IsAuthenticated() bool {
	return a.Kind != "" && a.Kind != ActorAnonymous && !a.TenantID.IsZero()
}

// HasScope reports whether the credential carries the scope.
func (a ActorContext) HasScope(scope string) bool { return slices.Contains(a.Scopes, scope) }

// RequireScope is the second of the two bounds on an operation; the first is the role, and both
// have to allow it (ADR-0005). The refusal names the scope the client is missing, because a
// client that cannot tell which scope to ask for cannot fix the problem (api-guidelines.md §7).
func (a ActorContext) RequireScope(scope string) error {
	if !a.IsAuthenticated() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if !a.HasScope(scope) {
		return shared.ErrForbidden.
			WithDetail("access.insufficient_scope").
			WithParams(map[string]string{"scope": scope})
	}
	return nil
}

// PersistenceScope is what bounds a unit of work. It exists here so that the translation from
// "who is acting" to "which tenant the transaction runs as" happens once, rather than in every
// use case that opens one.
func (a ActorContext) PersistenceScope() persistence.Scope {
	return persistence.Scope{TenantID: a.TenantID, ActorID: a.AccountID}
}

type contextKey struct{ name string }

var actorKey = contextKey{"application.actor"}

// ContextWithActor carries the actor into the call chain. Only the inbound adapters call this:
// they are where a credential is turned into an actor, and a use case that could construct its
// own actor could grant itself a tenant.
func ContextWithActor(ctx context.Context, actor ActorContext) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}

// ActorFrom returns the actor of the request.
//
// The zero value is deliberately not an anonymous actor but an unusable one: a caller that
// forgot the middleware gets a context whose IsAuthenticated is false and whose tenant is empty,
// so the unit of work refuses it. Failing to find an actor must not look like finding a valid
// one.
func ActorFrom(ctx context.Context) (ActorContext, bool) {
	actor, ok := ctx.Value(actorKey).(ActorContext)
	return actor, ok
}
