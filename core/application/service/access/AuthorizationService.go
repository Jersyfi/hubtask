// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package access decides whether an actor may do something.
//
// This is the one place authorisation happens (CLAUDE.md rule 2, ADR-0005). Not in an adapter,
// not in a repository, not in a middleware: a check in an adapter covers the channel it sits in,
// and the same use case reached through MCP or through an automation rule would then be checked
// by nobody. It also has to be one place for the audit trail's sake - a refusal is recorded here,
// which is what makes `outcome=DENIED` complete without every developer having to remember it
// (audit.md §7).
package access

import (
	"context"
	"log/slog"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Request is one permission question, plus what the trail needs in order to record the answer if
// it is no.
//
// There is no target label. A container's name is user content, and rule 10 keeps user content
// out of the audit trail; the name reaches the entry as a fingerprint through the `changes`
// field, which is enough to compare two entries and not enough to read one.
type Request struct {
	// Permission is what is being asked for.
	Permission service.Permission
	// Path runs from the tenant downwards. It decides which memberships count.
	Path []identity.Scope
	// Action is the audit code of the operation being attempted, e.g. `container.created`. A
	// refusal is recorded against the action that was refused, not against a generic "denied".
	Action audit.Action
	// TokenScope is the second, independent bound (ADR-0005): the role may allow it and the
	// token still may not. Empty means the operation needs no particular scope.
	TokenScope string
	TargetType string
	TargetID   shared.ID
}

// Service answers permission questions and records the refusals.
type Service struct {
	Memberships repository.Memberships
	UnitOfWork  persistence.UnitOfWork
	Audit       audit.Sink
	Clock       clock.Clock
}

// Authorize returns nil when the actor may proceed, and a typed refusal otherwise.
//
// It must be called *before* the transaction that performs the operation, not inside it. A
// refusal writes an audit entry, and an entry written inside the caller's transaction would be
// rolled back together with the refusal - leaving exactly the record an auditor is looking for
// missing (test AT-3).
func (s Service) Authorize(ctx context.Context, actor appshared.ActorContext, request Request) error {
	if !actor.IsAuthenticated() {
		// No tenant, so no entry could be written and none is owed: an unauthenticated request
		// performs no auditable action. The credential itself was already judged, and its failure
		// recorded, by authentication.
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if request.TokenScope != "" {
		if err := actor.RequireScope(request.TokenScope); err != nil {
			s.recordRefusal(ctx, actor, request, "scope")
			return err
		}
	}

	var memberships []identity.Membership
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		memberships, err = s.Memberships.Along(ctx, actor.AccountID, request.Path)
		return err
	})
	if err != nil {
		// Not a refusal: nobody was denied anything, the question could not be answered. Reporting
		// it as forbidden would send a client off to fix a permission that is not the problem.
		return err
	}

	if !service.Allows(memberships, request.Path, request.Permission) {
		s.recordRefusal(ctx, actor, request, "permission")
		return shared.ErrForbidden.
			WithDetail("access.not_permitted").
			WithParams(map[string]string{"permission": string(request.Permission)})
	}
	return nil
}

// Permitted answers the same question as Authorize for many paths at once: which of these may the
// actor do this to.
//
// It exists for the one read the single check cannot serve. A list anchored to a container asks about
// one path and goes through Authorize; the hub level is anchored to nothing, and a check at the tenant
// scope would refuse everybody whose membership sits on a hub rather than on the workspace - which is
// most of the point of hub-scoped memberships (domain-model.md §3.2). So the rows are read first and
// the ones the actor may not see are dropped.
//
// One membership read for the whole page, not one per row. The port's comment is explicit that the
// query may be generous and must not be unbounded, and the union of a page's scopes is bounded by the
// page size - which the use case has already clamped to the contract's maximum.
//
// Nothing here is audited. A row left out of a list is not a denied access: nobody was refused
// anything, the actor asked what they can see and was told. The refusal that *is* recorded is the
// token scope, because that one refuses the whole operation rather than narrowing its answer
// (audit.md §4).
func (s Service) Permitted(
	ctx context.Context, actor appshared.ActorContext, request Request, paths [][]identity.Scope,
) ([]bool, error) {
	if !actor.IsAuthenticated() {
		return nil, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if request.TokenScope != "" {
		if err := actor.RequireScope(request.TokenScope); err != nil {
			s.recordRefusal(ctx, actor, request, "scope")
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}

	var memberships []identity.Membership
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		memberships, err = s.Memberships.Along(ctx, actor.AccountID, union(paths))
		return err
	})
	if err != nil {
		// Not a refusal: nobody was denied anything, the question could not be answered.
		return nil, err
	}

	allowed := make([]bool, len(paths))
	for i, path := range paths {
		allowed[i] = service.Allows(memberships, path, request.Permission)
	}
	return allowed, nil
}

// CanSee answers the same membership question about somebody other than the actor: is there a role
// anywhere on this path that this account holds?
//
// It exists because an assignment is a decision about a second person. Giving an entry to somebody
// who gets a 404 on it is a piece of work nobody can do, and - once C-04 lands - a contributor's
// write right pointing at nothing, so the account has to hold a membership along the path
// (domain-model.md §3.2, service.EffectiveRole). Read rather than write, because that is what
// "can see it" means; every role in the matrix reads, so this is in practice "holds a role at all",
// and it is asked as the permission rather than as the presence of a row so that a role added later
// without a read right does not silently become assignable.
//
// It is here rather than in the use case for the reason this whole package exists: an answer about
// who may reach what is one answer, and a second implementation of it in a work-management service
// is a second place for it to be wrong (ADR-0005).
//
// The transaction is the actor's, so what it can see is the actor's tenant. An account of another
// tenant therefore holds no memberships this query can find and comes back false - which is the
// same answer as an account that does not exist, and deliberately: the caller must not be able to
// tell the two apart (T-04, multi-tenancy.md §2).
//
// Nothing is audited. Nobody was refused anything - the actor's own permission was decided by
// Authorize before this runs, and this is a question about an argument they passed (audit.md §4).
func (s Service) CanSee(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []identity.Scope,
) (bool, error) {
	if accountID.IsZero() {
		return false, nil
	}

	var memberships []identity.Membership
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		memberships, err = s.Memberships.Along(ctx, accountID, path)
		return err
	})
	if err != nil {
		// Not an answer of "no": the question could not be asked. Reporting it as a refusal would
		// send a client off to grant a permission that is not the problem.
		return false, err
	}
	return service.Allows(memberships, path, service.PermissionRead), nil
}

// RoleAlong returns the actor's own effective role along the path, and whether they hold one at
// all.
//
// It exists for the qualifiers the permission matrix deliberately does not fold in
// (service.Permission): "only the author or an administrator may change a comment" is a rule
// about *which* role the actor holds, not about a permission a role carries, and the use case
// that enforces it needs the role to say so. Here rather than in that use case for the reason
// CanSee is here: an answer about who may reach what is one answer (ADR-0005).
//
// Nothing is audited. Nobody was refused anything - the caller asked a question, and whatever it
// refuses on the answer is its own act to record.
func (s Service) RoleAlong(
	ctx context.Context, actor appshared.ActorContext, path []identity.Scope,
) (identity.Role, bool, error) {
	var memberships []identity.Membership
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		memberships, err = s.Memberships.Along(ctx, actor.AccountID, path)
		return err
	})
	if err != nil {
		return "", false, err
	}

	role, found := service.EffectiveRole(memberships, path)
	return role, found, nil
}

// union flattens the paths into the scopes to ask about, without duplicates. The resolution ignores
// whatever is not on the path it is checking, so asking about all of them at once is safe - and it is
// the difference between one query per page and one per row.
func union(paths [][]identity.Scope) []identity.Scope {
	seen := make(map[identity.Scope]bool)
	var scopes []identity.Scope

	for _, path := range paths {
		for _, scope := range path {
			if !seen[scope] {
				seen[scope] = true
				scopes = append(scopes, scope)
			}
		}
	}
	return scopes
}

// recordRefusal writes the DENIED entry in a transaction of its own.
//
// A failed write does not turn into a different answer for the client - the refusal stands either
// way - but it is an error rather than a warning: the trail is evidence, and a gap in it is an
// operational problem even though nobody's request was affected.
func (s Service) recordRefusal(ctx context.Context, actor appshared.ActorContext, request Request, reason string) {
	entry := audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: s.Clock.Now(),
		Action:     request.Action,
		Outcome:    audit.OutcomeDenied,
		// A refusal is worth more than a note and less than an alarm: one of them is somebody
		// clicking the wrong thing, a hundred of them is an attempt.
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		TargetType: request.TargetType,
		TargetID:   request.TargetID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "denied_by", Classification: audit.Open, To: reason},
			audit.Change{Field: "permission", Classification: audit.Open, To: string(request.Permission)},
		),
	}

	err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return s.Audit.Append(ctx, entry)
	})
	if err != nil {
		slog.ErrorContext(ctx, "recording a denied access failed",
			slog.String("action", string(request.Action)),
			slog.String("error", err.Error()))
	}
}
