// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
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
	Media         int
	// AccountRemoved and AccountAnonymised are the two ends, and exactly one of them is true.
	AccountRemoved    bool
	AccountAnonymised bool
}

// Erase carries out the case's erasure.
//
// The order is the one the data catalogue's deletion paths imply, and it is deliberate: the
// credentials go first, so that nothing can act as the person half way through; the derived
// records next; the person's own content after that, with its journal entries and tombstones; the
// bytes outside the transaction, because a bucket is an external dependency
// (observability-reliability.md §8); and the account row last, because everything above names it.
func (e Eraser) Erase(
	ctx context.Context, actor appshared.ActorContext, request domain.Request,
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

		assignments, err := e.Erasure.ReleaseAssignments(ctx, subject, now)
		if err != nil {
			return err
		}
		erased.Assignments = assignments

		if request.ErasureMode == domain.ModeFullDelete {
			removed, err := e.removeContributions(ctx, subject, now)
			if err != nil {
				return err
			}
			erased.Comments = removed
		}

		orphaned, err = e.Erasure.OrphanedMedia(ctx, subject)
		if err != nil {
			return err
		}

		// The trail is exempt from erasure and cannot be edited in place, so what happens to it is
		// a substitution at the boundary (audit.md §6). The mapping is written here, in the same
		// transaction as the erasure, because a mapping written afterwards is a window in which
		// the trail still answers a name.
		if err := e.Pseudonyms.Assign(
			ctx, subject, pseudonymFor(subject), string(lifecycle.DeletedByErasure), now,
		); err != nil {
			return err
		}

		return e.finishAccount(ctx, subject, request.ErasureMode, now, &erased)
	})
	if err != nil {
		return Erased{}, err
	}

	// The bytes, outside every transaction. A medium whose row is gone and whose bytes are not is
	// a file nothing in this system knows about; the other order would be a row pointing at
	// nothing, which the reconciliation would then hunt for ever.
	erased.Media = e.discardBytes(ctx, actor, orphaned, now)

	if err := e.record(ctx, actor, request, erased, now); err != nil {
		return Erased{}, err
	}
	return erased, nil
}

// removeContributions takes the person's own comments, each with the journal entry and the
// tombstone it owes.
func (e Eraser) removeContributions(
	ctx context.Context, subject shared.ID, now time.Time,
) (int, error) {
	authored, err := e.Erasure.AuthoredComments(ctx, subject)
	if err != nil {
		return 0, err
	}
	if len(authored) == 0 {
		return 0, nil
	}

	removals := make([]lifecycle.Removal, 0, len(authored))
	for _, comment := range authored {
		removals = append(removals, lifecycle.Removal{
			Entity: "comment", EntityID: comment.ID, Reason: lifecycle.DeletedByErasure,
		})
	}
	if err := e.Removals.Record(ctx, removals, now, now.Add(e.TombstoneWindow)); err != nil {
		return 0, err
	}

	removed, err := e.Erasure.DeleteAuthoredComments(ctx, subject)
	if err != nil {
		return 0, err
	}
	return removed, nil
}

// finishAccount is the last step: the row goes, or it stays and loses everything of the person's.
func (e Eraser) finishAccount(
	ctx context.Context, subject shared.ID, mode domain.ErasureMode, now time.Time, erased *Erased,
) error {
	if mode == domain.ModeAnonymize {
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
// is the job that exists for exactly this (C-06, data-protection.md §5). Failing the whole erasure
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
	erased Erased, now time.Time,
) error {
	return e.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return e.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: ErasedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityCritical,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: requestTarget, TargetID: request.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			// Counts rather than identifiers, and no name anywhere: what an auditor needs is that
			// every location was served and how much went from each, which is checkable - "the
			// person is gone" is not.
			Changes: audit.Changes(
				audit.Change{Field: "mode", Classification: audit.Open, To: string(erased.Mode)},
				audit.Change{Field: "credentials", Classification: audit.Open, To: erased.Credentials},
				audit.Change{Field: "notifications", Classification: audit.Open, To: erased.Notifications},
				audit.Change{Field: "assignments", Classification: audit.Open, To: erased.Assignments},
				audit.Change{Field: "comments", Classification: audit.Open, To: erased.Comments},
				audit.Change{Field: "media", Classification: audit.Open, To: erased.Media},
				audit.Change{
					Field: "account", Classification: audit.Open,
					To: accountOutcome(erased),
				},
			),
			LegalBasis: LegalBasisOf(domain.KindErasure),
		})
	})
}

func accountOutcome(erased Erased) string {
	switch {
	case erased.AccountRemoved:
		return "deleted"
	case erased.AccountAnonymised:
		return "anonymised"
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
