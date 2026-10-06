// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// An operator opens the password for one workspace (ADR-0078 §3, SC-34).
//
// For a provider that is switched on but broken, nothing inside the workspace can help: the people
// who could switch the password back on cannot sign in to do it. The operator opens it for that one
// workspace, for a limited time, on somebody's request - and the opening is the fallback of ADR-0078
// §2 with a person's decision as its cause, read wherever the ways in are (identity.WaysIn).
//
// **Behind everything the control plane has**: the scope, the operator register, and a fresh step-up,
// because it widens the way into somebody else's workspace. Closing it early narrows the way in again
// and asks for the scope and the register only.
//
// **Recorded twice, in one transaction**: in the workspace's trail, where its administrators read
// who opened their door, for whom and why, and in the installation's journal, where the operator's own
// record of control-plane acts is - `LifecycleShift`'s precedent. The journal keeps the end and that a
// requester and a reason were given, never their texts: it is permanent and outlives the workspace,
// and the texts may name a person. Neither carries anything of the workspace's content, and the
// opening reads none (P-01).

const (
	OpenTenantPasswordName  = "OpenTenantPassword"
	CloseTenantPasswordName = "CloseTenantPassword"

	// TenantPasswordOpenedAction and TenantPasswordClosedAction are written into the workspace's own
	// trail. The opening is WARNING: it is the entry somebody asking "how did a password get in while
	// we only use our directory" is looking for.
	TenantPasswordOpenedAction audit.Action = "tenant.password_opened"
	TenantPasswordClosedAction audit.Action = "tenant.password_closed"

	journalPasswordOpened = "tenant.password_opened"
	journalPasswordClosed = "tenant.password_closed"
)

// How an opening ended, as the close's entries say it. Named without "password", NewOpening's
// reason: these are codes, not credentials.
const (
	// OpeningEndedByOperator is an operator closing it before its time.
	OpeningEndedByOperator = "OPERATOR"
	// OpeningEndedExpired is its time passing. The password closed at that moment whatever was
	// recorded; this is the entry that says so afterwards.
	OpeningEndedExpired = "EXPIRED"
)

// OpeningNotices tells a workspace's administrators about an opening of its password (ADR-0078 §3):
// when it opens, with its end, and when it closes, with how. Called inside the transaction that
// opened or closed it, so the act and its notices commit together.
type OpeningNotices interface {
	PasswordOpened(ctx context.Context, tenantID shared.ID, until time.Time) error
	PasswordClosed(ctx context.Context, tenantID shared.ID, ended string) error
}

// PasswordOpeningWriter is what opening and closing share.
type PasswordOpeningWriter struct {
	// Instance is the scope and the register (ADR-0070 §1), checked as for every instance act.
	Instance InstanceWriter
	Tenants  adminrepo.Tenants
	Journal  adminrepo.Journal
	Audit    audit.Sink
	// Jobs seeds the job that records the end once the time has passed - in the workspace's own
	// write, because nothing may enumerate workspaces to find openings that ended.
	Jobs JobQueue
	// Notices tells the workspace's administrators. Nil tells nobody, which is the shape of a test
	// about something else, never of the server (cmd/server wires it).
	Notices    OpeningNotices
	StepUp     stepup.Verifier
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Text brings the requester and the reason to normal form C (M-07).
	Text text.Normalizer
}

// OpenTenantPasswordCommand is the input, typed.
type OpenTenantPasswordCommand struct {
	TenantID shared.ID
	// Hours is how long it stands. The channel fills in the default day where the caller named none,
	// so that a zero somebody sent is refused rather than read as "the default".
	Hours     int
	Requester string
	Reason    string
	// StepUpToken is H-03's proof, consumed by this one request.
	StepUpToken string
}

// OpenTenantPassword opens the password for one named workspace.
type OpenTenantPassword struct{ Writer PasswordOpeningWriter }

// Execute opens it.
func (h OpenTenantPassword) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd OpenTenantPasswordCommand,
) (adminrepo.TenantRecord, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return adminrepo.TenantRecord{}, err
	}
	if cmd.TenantID.IsZero() {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}

	// What was typed first, then the proof: a refused reason must not burn a step-up the operator
	// then has to earn again.
	opening, err := domain.NewOpening(cmd.Hours, cmd.Requester, cmd.Reason, w.Clock.Now(), w.Text)
	if err != nil {
		return adminrepo.TenantRecord{}, err
	}
	// Outside the workspace's transaction: the proof lives in the operator's own workspace, and a
	// nested scope may not switch tenant (postgres.tenant_switch_in_transaction).
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
		return adminrepo.TenantRecord{}, err
	}

	var opened adminrepo.TenantRecord
	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := w.find(ctx)
		if err != nil {
			return err
		}
		switch record.Status {
		case domain.TenantPendingDeletion:
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		case domain.TenantSuspended:
			// Its people are refused before any password is asked for, so an opening would let
			// nobody in - while its administrators were mailed that everybody can sign in.
			return shared.ErrConflict.WithDetail("admin.password_opening_suspended")
		}

		now := w.Clock.Now()
		moved, err := w.Tenants.OpenPassword(ctx, opening, now)
		if err != nil {
			return err
		}
		if !moved {
			// The workspace was suspended or started leaving between the read and the write.
			return shared.ErrConflict.WithDetail("admin.password_opening_suspended")
		}

		if err := w.Audit.Append(ctx, audit.Entry{
			TenantID: cmd.TenantID, OccurredAt: now, Action: TenantPasswordOpenedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: cmd.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: openingChanges(opening),
		}); err != nil {
			return err
		}
		if err := w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: now, Action: journalPasswordOpened,
			TenantID: cmd.TenantID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
			// The end, and that who asked and why were given - never the texts. The journal is
			// permanent and outlives the workspace's hard delete; the requester may name a person and
			// the reason may too, so both live in the workspace's own trail and go with it.
			Details: map[string]any{
				"until":             opening.Until.Format(time.RFC3339),
				"requester_present": opening.Requester != "",
				"reason_present":    opening.Reason != "",
			},
		}); err != nil {
			return err
		}
		// The end, recorded when it comes. One job per workspace: a second opening moves a waiting
		// job no later, and the job comes back while the opening it finds is still running.
		if w.Jobs != nil {
			if _, err := w.Jobs.Enqueue(ctx, queue.Request{
				Kind: queue.KindPasswordOpeningEnd, TenantID: cmd.TenantID,
				DedupeKey: cmd.TenantID.String(), RunAt: opening.Until,
			}); err != nil {
				return err
			}
		}
		if w.Notices != nil {
			if err := w.Notices.PasswordOpened(ctx, cmd.TenantID, opening.Until); err != nil {
				return err
			}
		}

		record.PasswordOpening = opening
		opened = record
		return nil
	})
	if err != nil {
		return adminrepo.TenantRecord{}, err
	}
	return opened, nil
}

// openingChanges is what the trail says about an opening.
//
// The requester and the reason in clear, as the actor's label is (audit.md §4's one stated
// exception): they are the attribution the entry exists for - the workspace's administrators read
// it to tell an opening somebody asked for from one nobody did, and a fingerprint cannot be read.
// The operator is asked for a ticket reference rather than a name (data-catalog.md).
func openingChanges(opening domain.PasswordOpening) map[string]any {
	return audit.Changes(
		audit.Change{Field: "password_opened_until", Classification: audit.Open,
			To: opening.Until.Format(time.RFC3339)},
		audit.Change{Field: "requester", Classification: audit.Open, To: opening.Requester},
		audit.Change{Field: "reason", Classification: audit.Open, To: opening.Reason},
	)
}

// find reads the transaction's own workspace, refusing one that is not there.
func (w PasswordOpeningWriter) find(ctx context.Context) (adminrepo.TenantRecord, error) {
	record, err := w.Tenants.Find(ctx)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
		}
		return adminrepo.TenantRecord{}, err
	}
	return record, nil
}

// close ends the opening inside the caller's transaction and records how: early by `actor`, or at
// its time, `due`, by the installation itself. False where there was nothing to end - closing an
// opening that is not there is the state the caller wanted, and records nothing.
func (w PasswordOpeningWriter) close(
	ctx context.Context, record adminrepo.TenantRecord, actor appshared.ActorContext,
	due time.Time, ended string,
) (bool, error) {
	now := w.Clock.Now()
	closed, err := w.Tenants.ClosePassword(ctx, due, now)
	if err != nil || !closed {
		return false, err
	}
	until := record.PasswordOpening.Until.Format(time.RFC3339)
	if err := w.Audit.Append(ctx, audit.Entry{
		TenantID: record.ID, OccurredAt: now, Action: TenantPasswordClosedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: tenantTarget, TargetID: record.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "password_opened_until", Classification: audit.Open, From: until},
			audit.Change{Field: "ended", Classification: audit.Open, To: ended},
		),
	}); err != nil {
		return false, err
	}
	if err := w.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: w.IDs.NewID(), OccurredAt: now, Action: journalPasswordClosed,
		TenantID: record.ID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
		Details: map[string]any{"until": until, "ended": ended},
	}); err != nil {
		return false, err
	}
	if w.Notices != nil {
		if err := w.Notices.PasswordClosed(ctx, record.ID, ended); err != nil {
			return false, err
		}
	}
	return true, nil
}

// EndPasswordOpening records the end of an opening whose time has passed (KindPasswordOpeningEnd).
//
// The password closed at that moment whatever this does: the end is honoured where the opening is
// read. What is left is what a person can see - the trail entry, the journal entry, the notices -
// written here, in the workspace's own transaction, once.
type EndPasswordOpening struct{ Writer PasswordOpeningWriter }

// Execute ends what is due and answers how long until it should look again: zero where nothing is
// left to wait for - the opening ended now, or was closed before, or never was.
func (h EndPasswordOpening) Execute(ctx context.Context, tenantID shared.ID) (time.Duration, error) {
	w := h.Writer
	var again time.Duration
	err := w.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			record, err := w.Tenants.Find(ctx)
			if errors.Is(err, shared.ErrNotFound) {
				// The workspace went while the job waited; there is nobody left to tell.
				return nil
			}
			if err != nil {
				return err
			}
			now := w.Clock.Now()
			opening := record.PasswordOpening
			switch {
			case opening.Until.IsZero():
				// Closed early, and the close was recorded then.
				return nil
			case opening.InForce(now):
				// A later opening replaced the one this job was seeded for.
				again = opening.Until.Sub(now)
				return nil
			}
			closed, err := w.close(ctx, record, appshared.ActorContext{
				Kind: appshared.ActorSystem, TenantID: tenantID, AccountName: "the installation",
			}, now, OpeningEndedExpired)
			if err != nil || closed {
				return err
			}
			// Nothing was due when the statement ran: an operator opened the password again between
			// the read and the close. That opening's own job collapsed into this running one, so this
			// one comes back at its end - or nobody would record it and tell the administrators.
			again, err = h.standingFor(ctx, now)
			return err
		})
	return again, err
}

// standingFor reads the row again and answers how long the opening found there still runs; zero
// where none does.
func (h EndPasswordOpening) standingFor(ctx context.Context, now time.Time) (time.Duration, error) {
	record, err := h.Writer.Tenants.Find(ctx)
	if errors.Is(err, shared.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !record.PasswordOpening.InForce(now) {
		return 0, nil
	}
	return record.PasswordOpening.Until.Sub(now), nil
}

// CloseTenantPassword ends an opening before its time.
type CloseTenantPassword struct{ Writer PasswordOpeningWriter }

// Execute closes it. A workspace with no opening answers as it is.
func (h CloseTenantPassword) Execute(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
) (adminrepo.TenantRecord, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return adminrepo.TenantRecord{}, err
	}
	if tenantID.IsZero() {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}

	var closed adminrepo.TenantRecord
	scope := persistence.Scope{TenantID: tenantID, ActorID: actor.AccountID}
	err := w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := w.find(ctx)
		if err != nil {
			return err
		}
		if opening := record.PasswordOpening; !opening.Until.IsZero() {
			// An opening whose time has passed ended then, not now: the row only still holds it
			// because the job that records the end has not run yet. Recorded as what happened.
			ended := OpeningEndedByOperator
			if !opening.InForce(w.Clock.Now()) {
				ended = OpeningEndedExpired
			}
			if _, err := w.close(ctx, record, actor, time.Time{}, ended); err != nil {
				return err
			}
		}
		record.PasswordOpening = domain.PasswordOpening{}
		closed = record
		return nil
	})
	if err != nil {
		return adminrepo.TenantRecord{}, err
	}
	return closed, nil
}

// Descriptor is the catalogue entry.
func (h OpenTenantPassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: OpenTenantPasswordName,
		Summary: "Opens the password for one workspace for a limited time - 24 hours unless said " +
			"otherwise, at most 168 - for a provider that is switched on but broken: every account " +
			"there that holds a password signs in with it, whatever the workspace's switch and any " +
			"installation lock say. Records who asked and why. Demands a step-up.",
		SideEffects: "Writes the opening on the tenant row; records it in the workspace's trail and " +
			"the instance journal. Reads and changes nothing of the workspace's content.",
		TokenScope: adminTenantsScope,
		StepUp:     "always - it widens the way into somebody else's workspace (ADR-0078 §3)",
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{Name: "hours", Kind: usecase.KindInt,
				Description: "How long the opening stands, from now: 1 to 168, 24 when absent."},
			{Name: "requester", Kind: usecase.KindString, Required: true,
				Description: "Who asked for it - a ticket reference rather than a name, where possible."},
			{Name: "reason", Kind: usecase.KindString, Required: true,
				Description: "Why the password is opened."},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof POST /auth/step-up answered, consumed by this one request.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantPasswordOpenedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h OpenTenantPassword) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	hours := domain.PasswordOpeningDefaultHours
	if in.Present("hours") {
		hours = in.Int("hours")
	}
	record, err := h.Execute(ctx, actor, OpenTenantPasswordCommand{
		TenantID: tenantID, Hours: hours,
		Requester: in.String("requester"), Reason: in.String("reason"),
		StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	return adminTenantOutput(record), nil
}

// Descriptor is the catalogue entry.
func (h CloseTenantPassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CloseTenantPasswordName,
		Summary: "Ends an operator's opening of the password for one workspace before its time; the " +
			"workspace's own rules decide again from the next request. Closing where nothing is " +
			"open changes nothing.",
		SideEffects: "Clears the opening on the tenant row; records it in the workspace's trail and " +
			"the instance journal.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantPasswordClosedAction, TargetType: tenantTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CloseTenantPassword) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	record, err := h.Execute(ctx, actor, tenantID)
	if err != nil {
		return nil, err
	}
	return adminTenantOutput(record), nil
}
