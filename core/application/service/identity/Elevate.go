// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const ElevateSessionName = "ElevateSession"

// SessionElevatedAction is a session raised to the control plane's scope, and the moment it falls
// back. Warning rather than notice: for the hour it lasts, one browser tab can provision, suspend
// and delete every workspace on the installation, and a reader scanning the trail should see it.
const SessionElevatedAction audit.Action = "instance.session_elevated"

// journalSessionElevated is the same act in the installation's own journal (audit.md §6), where an
// act about the installation rather than about one workspace belongs.
const journalSessionElevated = "instance.session_elevated"

// ElevateSession raises the caller's own session to the control plane's scope for an hour.
//
// **This deliberately weakens the rule that `admin:tenants` is never carried by a session.** It
// weakens it to: only for a registered operator, only after a fresh proof, only for an hour, only on
// the session that proved it, and written down. What it buys is that nobody has to mint a long-lived
// all-powerful token and paste it into a browser to change a switch - which is the outcome the
// strict rule produces in practice, and which is worse (ADR-0070 §4).
//
// The personal access token stays exactly as it is, for automation.
type ElevateSession struct {
	Writer SessionWriter
	// Operators is the register. Without it there is nobody this could be granted to, so a nil one
	// refuses rather than permitting - the opposite of every other optional dependency here, and
	// deliberately: the others make a feature absent, this one would make a bound absent.
	Operators repository.Operators
	// Journal is the installation's own record. Both ends of an elevation are written into it.
	Journal    adminrepo.Journal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Elevation is what a raised session answers: until when, and how long that is from now.
type Elevation struct {
	Until time.Time
	// Remaining is on the screen, because an hour that nobody can see the end of is an hour
	// somebody is surprised by.
	Remaining time.Duration
}

// Execute raises the session.
func (h ElevateSession) Execute(
	ctx context.Context, actor appshared.ActorContext, stepUpToken string,
) (Elevation, error) {
	w := h.Writer
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() || actor.TokenID.IsZero() {
		return Elevation{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if h.Operators == nil {
		return Elevation{}, shared.ErrNotFound.WithDetail("route.operation_not_available")
	}

	// The register first, and the proof second. A person the register does not name learns that
	// before being asked to type a password, which is the courteous order and also the one that
	// spends no credential on a call that was going to be refused.
	var held bool
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := h.Operators.Holds(ctx, actor.AccountID)
			held = read
			return err
		})
	if err != nil {
		return Elevation{}, err
	}
	if !held {
		return Elevation{}, shared.ErrForbidden.WithDetail("admin.operator_required")
	}

	if err := w.requireStepUp(ctx, actor, stepUpToken); err != nil {
		return Elevation{}, err
	}

	now := h.Clock.Now()
	until := now.Add(domain.ElevationLifetime).UTC()

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		raised, err := w.Sessions.Elevate(ctx, actor.TokenID, actor.AccountID, until, now)
		if err != nil {
			return err
		}
		if !raised {
			// Not the caller's session, or not live any more. Revoke's indistinguishable answer.
			return shared.ErrForbidden.WithDetail("auth.step_up_session_required")
		}
		if err := w.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     SessionElevatedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityWarning,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: sessionTarget,
			TargetID:   actor.TokenID,
			Changes: audit.Changes(audit.Change{
				Field: "elevated_until", Classification: audit.Open,
				To: until.Format(time.RFC3339),
			}),
		}); err != nil {
			return err
		}
		return h.record(ctx, actor, until, now)
	})
	if err != nil {
		return Elevation{}, err
	}
	return Elevation{Until: until, Remaining: until.Sub(now)}, nil
}

// record writes the installation's own entry. Both ends of an elevation are in it: this one names
// when it starts *and* when it will fall back, so a reader of the journal needs no second entry to
// know the window - and there is nothing to write at the other end, because the fall-back is a
// comparison rather than an act.
func (h ElevateSession) record(
	ctx context.Context, actor appshared.ActorContext, until, now time.Time,
) error {
	if h.Journal == nil {
		return nil
	}
	return h.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: h.IDs.NewID(), OccurredAt: now, Action: journalSessionElevated,
		TenantID: actor.TenantID, ActorLabel: actor.AccountName,
		Details: map[string]any{
			"account_id": actor.AccountID.String(),
			"session_id": actor.TokenID.String(),
			"until":      until.Format(time.RFC3339),
		},
	})
}

// Descriptor is the catalogue entry.
func (h ElevateSession) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ElevateSessionName,
		Summary: "Raises the caller's own session to the control plane's scope for an hour, for a " +
			"registered operator who passes a fresh step-up. It does not slide - activity " +
			"extends a session's own horizon and never this - it dies with the session, and a " +
			"second hour needs a second proof. Both the act and the moment it falls back are in " +
			"the installation's journal.",
		SideEffects: "Raises the session, writes an audit entry and a journal entry.",
		Input: []usecase.Field{
			{
				Name: "step_up_token", Kind: usecase.KindString, Required: true,
				Description: "The proof from `/auth/step-up`. It is consumed by this call.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: SessionElevatedAction, TargetType: sessionTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An elevation is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ElevateSession) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	elevation, err := h.Execute(ctx, actor, in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"elevated_until":    elevation.Until.UTC(),
		"remaining_seconds": int(elevation.Remaining.Seconds()),
	}, nil
}
