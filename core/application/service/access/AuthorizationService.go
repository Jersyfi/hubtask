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
