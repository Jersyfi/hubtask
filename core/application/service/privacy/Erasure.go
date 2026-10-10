// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The erasure (Art. 17, QS-19). Risk R-09 - overlooked derived data - is what the shape of this
// file is about: every storage location in the data catalogue is served by a named step, the steps
// are counted, and what could not be served is a failure rather than a silence.

const (
	// ErasedAction is the entry the erasure leaves in the trail it is exempt from.
	ErasedAction audit.Action = "dsr.erased"

	// FormerUser is the marker an anonymised account carries where a name was.
	//
	// A marker rather than an empty string, because the audit trail and the item history both
	// carry a denormalised label: a workspace reading "  " where somebody used to be is worse than
	// one reading "former user", and the label is what makes an entry still legible (audit.md §2).
	FormerUser = "former user"

	// pseudonymPrefix is what the audit trail answers instead of the erased actor's name. It says
	// nothing about the person and stays the same for all their entries, which is what lets an
	// auditor tell one actor's entries from another's without knowing who either was.
	pseudonymPrefix = "former-user-"
)

// Eraser carries out an erasure that has been decided.
//
// The application layer's half of the `privacy.request` job for an erasure case: the worker owns
// the queue and the retries, and what an erasure *is* - which storage locations, in which order,
// with what recorded - lives here.
type Eraser struct {
	Requests   repository.Requests
	Erasure    repository.Erasure
	Pseudonyms repository.Pseudonyms
	// Holds are the legal holds in force; one wins over the erasure as far as it reaches
	// (data-protection.md §4.1). Read in the erasure's own transaction, under the shared hold lock,
	// so a hold placed meanwhile either waits for the erasure or is read by it.
	Holds lifecyclerepo.LegalHolds
	// Kept records what each hold kept, in the same transaction.
	Kept repository.Kept
	// Subjects restricts an account a hold keeps (Art. 18).
	Subjects repository.Subjects
	// Removals writes the journal entry and the tombstone every removal owes, through the one
	// engine every removal in this system goes through (ADR-0020 §6). A comment removed without
	// them would come back from a restore, or be recreated by a device that was offline.
	Removals   lifecyclerepo.Removals
	Objects    storage.ObjectStore
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// TombstoneWindow is how long a removal's marker has to outlive it: the maximum offline
	// window, after which a device has to resynchronise from scratch anyway.
	TombstoneWindow time.Duration
	// LastMember trashes a private hub the erased person was the last member of (ADR-0073 §5).
	// Optional: wired without it, the retention pass finds the hub instead.
	LastMember LastMember
}

// LastMember is the work context's answer to a person leaving: which private hubs they are a member
// of, asked before their memberships go, and the trash for those nobody is left in, in the
// erasure's transaction.
type LastMember interface {
	HeldBy(ctx context.Context, accountID shared.ID) ([]shared.ID, error)
	AfterMemberLeft(ctx context.Context, tenantID shared.ID, named []shared.ID) (int, error)
}

// Erased is what one erasure did, location by location. It is what the audit entry carries and
// what an operator reads afterwards: "the person is gone" is not a statement anybody can check,
// and a count per location is.
type Erased struct {
	Mode          domain.ErasureMode
	Credentials   int
	Notifications int
	Assignments   int
	Comments      int
	// Intake is what the person sent in by mail: removed in a full deletion, and stripped of the
	// address in the mode that keeps the workspace's content.
	Intake int
	Media  int
	// AccountRemoved, AccountAnonymised and AccountKept are the three ends, and at most one of them
	// is true - none for a case about an address nobody here holds.
	AccountRemoved    bool
	AccountAnonymised bool
	AccountKept       bool
	// Kept is what each legal hold kept, empty when none kept anything.
	Kept []domain.Kept
}

// Erase carries out the case's erasure, as far as no legal hold keeps it.
//
// The order is the one the data catalogue's deletion paths imply, and it is deliberate: the case is
// locked and the holds are read first, so that what is kept is decided once and under the lock a
// hold placed meanwhile waits for; the credentials go next, so that nothing can act as the person
// half way through; the derived records next; the person's own content after that, with its journal
// entries and tombstones; the bytes outside the transaction, because a bucket is an external
// dependency (observability-reliability.md §8); and the account row last, because everything above
// names it. What each hold kept is recorded in the same transaction that decided it.
func (e Eraser) Erase(
	ctx context.Context, actor appshared.ActorContext, request domain.Request,
) (Erased, error) {
	return e.erase(ctx, actor, request, ErasedAction)
}

func (e Eraser) erase(
	ctx context.Context, actor appshared.ActorContext, request domain.Request, action audit.Action,
) (Erased, error) {
	if request.SubjectAccountID.IsZero() {
		// A case about an address nobody here holds. There is nothing in this workspace to erase,
		// and saying so is the honest answer rather than an error: the person asked, and the
		// answer is that this workspace holds nothing of theirs.
		return Erased{Mode: request.ErasureMode}, nil
	}
	if !request.ErasureMode.Valid() {
		return Erased{}, shared.ErrConflict.WithDetail(domain.CodeErasureModeRequired)
	}

	subject := request.SubjectAccountID
	now := e.Clock.Now()
	erased := Erased{Mode: request.ErasureMode}
	var orphaned []repository.Medium

	err := e.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		plan, err := e.plan(ctx, request)
		if err != nil {
			return err
		}
		erased.Kept = plan.Kept

		if request.ErasureMode == domain.ModeFullDelete {
			// What the workspace would be left with. `automation_rule.run_as` is `ON DELETE
			// RESTRICT`, so without this the deletion reaches the database and comes back as a
			// foreign key violation - a dependency error, in a case with a statutory deadline,
			// saying nothing about what to do (PG-2 found exactly that). Refusing with the count is
			// the answer somebody can act on: the rules are re-pointed at another account or
			// removed, and the case is carried out afterwards. Asked of every full deletion, held or
			// not (check 5): a kept account is deleted by the remainder later.
			//
			// Deleting the rules here instead would be this system destroying the workspace's
			// automation because one person left, which is not a decision an erasure takes alone.
			running, err := e.Erasure.AutomationsRunningAs(ctx, subject)
			if err != nil {
				return err
			}
			if running > 0 {
				return shared.ErrConflict.
					WithDetail(domain.CodeErasureBlockedByRule).
					WithParams(map[string]string{"rules": strconv.Itoa(running)})
			}
		}

		// Credentials and notifications go whatever is kept: they are no evidence, and no hold in
		// this system reaches them (data-protection.md §4.1). A kept account keeps its password,
		// so the person still signs in.
		credentials, err := e.Erasure.RevokeCredentials(ctx, subject)
		if err != nil {
			return err
		}
		erased.Credentials = credentials

		notifications, err := e.Erasure.DiscardNotifications(ctx, subject)
		if err != nil {
			return err
		}
		erased.Notifications = notifications

		assignments, err := e.Erasure.ReleaseAssignmentsOn(ctx, subject, plan.ReleaseOn, now)
		if err != nil {
			return err
		}
		erased.Assignments = assignments

		if !plan.KeepIntake {
			intake, err := e.intake(ctx, subject, request.ErasureMode)
			if err != nil {
				return err
			}
			erased.Intake = intake
		}

		removed, err := e.removeComments(ctx, subject, plan.DeleteComments, now)
		if err != nil {
			return err
		}
		erased.Comments = removed

		orphaned, err = e.Erasure.OrphanedMedia(ctx, subject)
		if err != nil {
			return err
		}

		// Asked before the account goes: a deletion takes its memberships with it by cascade, and
		// afterwards nothing says which hubs they were on.
		var held []shared.ID
		if e.LastMember != nil {
			if held, err = e.LastMember.HeldBy(ctx, subject); err != nil {
				return err
			}
		}
		if err := e.finishAccount(ctx, subject, plan.Account, now, &erased); err != nil {
			return err
		}
		if e.LastMember != nil && (erased.AccountAnonymised || erased.AccountRemoved) {
			if _, err := e.LastMember.AfterMemberLeft(ctx, actor.TenantID, held); err != nil {
				return err
			}
		}
		return e.Kept.RecordKept(ctx, request.ID, plan.Kept, now)
	})
	if err != nil {
		return Erased{}, err
	}

	// The bytes, outside every transaction. A medium whose row is gone and whose bytes are not is
	// a file nothing in this system knows about; the other order would be a row pointing at
	// nothing, which the reconciliation would then hunt for ever.
	erased.Media = e.discardBytes(ctx, actor, orphaned, now)

	if err := e.record(ctx, actor, request, erased, action, now); err != nil {
		return Erased{}, err
	}
	return erased, nil
}

// plan locks the case, reads the holds and the person's rows on entries, and decides what goes.
func (e Eraser) plan(ctx context.Context, request domain.Request) (domain.Plan, error) {
	found, err := e.Requests.Lock(ctx, request.ID)
	if err != nil {
		return domain.Plan{}, err
	}
	if !found {
		return domain.Plan{}, shared.ErrNotFound.WithDetail(domain.CodeRequestNotFound)
	}
	return e.decide(ctx, request.SubjectAccountID, request.ErasureMode)
}

// decide is the reading half of the plan, which the preview shares.
func (e Eraser) decide(
	ctx context.Context, subject shared.ID, mode domain.ErasureMode,
) (domain.Plan, error) {
	holds, err := e.Holds.Active(ctx)
	if err != nil {
		return domain.Plan{}, err
	}
	contributions, err := e.Erasure.Contributions(ctx, subject)
	if err != nil {
		return domain.Plan{}, err
	}
	rows := make([]domain.Row, 0, len(contributions))
	for _, contribution := range contributions {
		rows = append(rows, domain.Row{
			Kind: domain.RowKind(contribution.Kind), ID: contribution.ID, ItemID: contribution.ItemID,
			Path: contribution.Path, CollectionID: contribution.CollectionID,
			HubID: contribution.HubID, ItemCreatedBy: contribution.ItemCreatedBy,
		})
	}
	intake := 0
	if _, held := holds.OnTenant(); held {
		if intake, err = e.Erasure.CountIntake(ctx, subject); err != nil {
			return domain.Plan{}, err
		}
	}
	return domain.PlanErasure(domain.PlanInput{
		Subject: subject, Mode: mode, Holds: holds, Rows: rows, Intake: intake,
	}), nil
}

// intake serves the one location that knows the person by address rather than by account.
//
// The catalogue's path for `jumble_entry` is `RETENTION`, 90 days. That is right for a message
// nobody ever converted; it is not an answer to an erasure, and PG-2 is what made the difference
// visible - the address and the text were still there after everything else had gone.
func (e Eraser) intake(
	ctx context.Context, subject shared.ID, mode domain.ErasureMode,
) (int, error) {
	if mode == domain.ModeFullDelete {
		return e.Erasure.DiscardIntake(ctx, subject)
	}
	return e.Erasure.ReleaseIntake(ctx, subject)
}

// removeComments takes the comments the plan names, each with the journal entry and the tombstone
// it owes.
func (e Eraser) removeComments(
	ctx context.Context, subject shared.ID, ids []shared.ID, now time.Time,
) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	removals := make([]lifecycle.Removal, 0, len(ids))
	for _, id := range ids {
		removals = append(removals, lifecycle.Removal{
			Entity: "comment", EntityID: id, Reason: lifecycle.DeletedByErasure,
		})
	}
	if err := e.Removals.Record(ctx, removals, now, now.Add(e.TombstoneWindow)); err != nil {
		return 0, err
	}
	return e.Erasure.DeleteComments(ctx, subject, ids)
}

// finishAccount is the last step: the row goes, stays and loses everything of the person's, or -
// under a hold on the person or the workspace - stays as it is and is restricted.
func (e Eraser) finishAccount(
	ctx context.Context, subject shared.ID, fate domain.AccountFate, now time.Time, erased *Erased,
) error {
	if fate == domain.AccountKept {
		// Restricted rather than erased (Art. 18): stored, out of automatic processing, still
		// signing in. No pseudonym yet - the trail names the person while the account does.
		restricted, err := e.Subjects.SetStatus(ctx, subject, string(identity.AccountRestricted), now)
		if err != nil {
			return err
		}
		erased.AccountKept = restricted
		return nil
	}

	// The trail is exempt from erasure and cannot be edited in place, so what happens to it is a
	// substitution at the boundary (audit.md §6). The mapping is written here, in the same
	// transaction as the erasure, because a mapping written afterwards is a window in which the
	// trail still answers a name.
	if err := e.Pseudonyms.Assign(
		ctx, subject, pseudonymFor(subject), string(lifecycle.DeletedByErasure), now,
	); err != nil {
		return err
	}

	if fate == domain.AccountAnonymised {
		anonymised, err := e.Erasure.Anonymise(ctx, subject, FormerUser, now)
		if err != nil {
			return err
		}
		erased.AccountAnonymised = anonymised
		return nil
	}

	// A full deletion owes the same two records every removal owes: without them a restore brings
	// the person back, or a device that was offline pushes a change for an account this
	// installation decided was gone.
	if err := e.Removals.Record(ctx, []lifecycle.Removal{{
		Entity: "account", EntityID: subject, Reason: lifecycle.DeletedByErasure,
	}}, now, now.Add(e.TombstoneWindow)); err != nil {
		return err
	}

	removed, err := e.Erasure.Delete(ctx, subject)
	if err != nil {
		return err
	}
	erased.AccountRemoved = removed
	return nil
}

// discardBytes removes what the object store holds, and counts what actually went.
//
// A medium the store will not release keeps its row and is left to the media reconciliation, which
// is the job that exists for exactly this (data-protection.md §5). Failing the whole erasure
// over one file would leave the erasure half done and the case still open.
func (e Eraser) discardBytes(
	ctx context.Context, actor appshared.ActorContext, media []repository.Medium, now time.Time,
) int {
	discarded := 0
	for _, medium := range media {
		if e.Objects != nil {
			if err := e.Objects.Delete(ctx, medium.StorageKey); err != nil {
				slog.WarnContext(ctx, "an erased medium's bytes could not be removed",
					slog.String("media_id", medium.ID.String()),
					slog.String("error", shared.AsError(err).Code))
				continue
			}
		}

		err := e.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			if err := e.Removals.Record(ctx, []lifecycle.Removal{{
				Entity: "media_object", EntityID: medium.ID, Reason: lifecycle.DeletedByErasure,
			}}, now, now.Add(e.TombstoneWindow)); err != nil {
				return err
			}
			return e.Erasure.DiscardMedium(ctx, medium.ID)
		})
		if err != nil {
			slog.WarnContext(ctx, "an erased medium's row could not be removed",
				slog.String("media_id", medium.ID.String()),
				slog.String("error", shared.AsError(err).Code))
			continue
		}
		discarded++
	}
	return discarded
}

// record writes the entry the erasure owes, into the trail the erasure does not touch.
func (e Eraser) record(
	ctx context.Context, actor appshared.ActorContext, request domain.Request,
	erased Erased, action audit.Action, now time.Time,
) error {
	changes := []audit.Change{
		{Field: "mode", Classification: audit.Open, To: string(erased.Mode)},
		{Field: "credentials", Classification: audit.Open, To: erased.Credentials},
		{Field: "notifications", Classification: audit.Open, To: erased.Notifications},
		{Field: "assignments", Classification: audit.Open, To: erased.Assignments},
		{Field: "comments", Classification: audit.Open, To: erased.Comments},
		{Field: "intake", Classification: audit.Open, To: erased.Intake},
		{Field: "media", Classification: audit.Open, To: erased.Media},
		{Field: "account", Classification: audit.Open, To: accountOutcome(erased)},
	}
	if pending := pendingParts(erased.Kept); len(pending) > 0 {
		// What the holds kept, by hold, and why it may be kept: the hold's identifier says where to
		// look; its reason stays on the hold, written for the hold's own readers.
		holds := make([]string, 0, len(pending))
		var kept domain.Kept
		for _, part := range pending {
			holds = append(holds, part.HoldID.String())
			kept.Entries += part.Entries
			kept.Comments += part.Comments
			kept.Assignments += part.Assignments
			kept.Intake += part.Intake
		}
		changes = append(changes,
			audit.Change{Field: "kept_holds", Classification: audit.Open, To: strings.Join(holds, ",")},
			audit.Change{Field: "kept_entries", Classification: audit.Open, To: kept.Entries},
			audit.Change{Field: "kept_comments", Classification: audit.Open, To: kept.Comments},
			audit.Change{Field: "kept_assignments", Classification: audit.Open, To: kept.Assignments},
			audit.Change{Field: "kept_intake", Classification: audit.Open, To: kept.Intake},
			audit.Change{Field: "kept_legal_basis", Classification: audit.Open, To: domain.KeptLegalBasis},
		)
	}

	return e.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return e.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: action, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityCritical,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: requestTarget, TargetID: request.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			// Counts rather than identifiers, and no name anywhere: what an auditor needs is that
			// every location was served and how much went from each, which is checkable - "the
			// person is gone" is not.
			Changes:    audit.Changes(changes...),
			LegalBasis: LegalBasisOf(domain.KindErasure),
		})
	})
}

// pendingParts are the parts a hold still keeps.
func pendingParts(kept []domain.Kept) []domain.Kept {
	pending := make([]domain.Kept, 0, len(kept))
	for _, part := range kept {
		if part.Pending() {
			pending = append(pending, part)
		}
	}
	return pending
}

func accountOutcome(erased Erased) string {
	switch {
	case erased.AccountRemoved:
		return "deleted"
	case erased.AccountAnonymised:
		return "anonymised"
	case erased.AccountKept:
		return "kept_restricted"
	default:
		return "absent"
	}
}

// pseudonymFor is the label the trail answers instead of an erased actor's name.
//
// Derived from the identifier rather than random, so that a retried erasure produces the same
// label, and short enough to read in a table. It says nothing about the person: it is the
// identifier they already had, which an auditor could see anyway, written in a form that is
// obviously not a name.
func pseudonymFor(accountID shared.ID) string {
	digest := hex.EncodeToString([]byte(accountID.String()))
	if len(digest) > 12 {
		digest = digest[len(digest)-12:]
	}
	return fmt.Sprintf("%s%s", pseudonymPrefix, digest)
}
