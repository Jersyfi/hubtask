// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RetentionMarkingRepository is the two phases of data-retention.md §5 against the entries (E-07).
//
// One statement per anchor rather than one with the column as a parameter. The anchor is a value of
// the domain's closed set and never a byte of a request, which is rule 9 applied to the one place
// where it is tempting to bend: a period runs from a column, and a column name assembled from
// anything a caller sent would be the one string concatenation the rule exists to forbid.
type RetentionMarkingRepository struct{}

func NewRetentionMarkingRepository() RetentionMarkingRepository {
	return RetentionMarkingRepository{}
}

var _ repository.Marking = RetentionMarkingRepository{}

// Due answers entries whose anchor lies before the cutoff and which are not marked yet.
func (r RetentionMarkingRepository) Due(
	ctx context.Context, anchor domain.Anchor, cutoff time.Time, batch int,
) ([]repository.Candidate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	size := retentionBatch(batch)

	switch anchor {
	case domain.AnchorCompletedAt:
		rows, err := queries.RetentionCandidatesByCompletedAt(ctx,
			sqlc.RetentionCandidatesByCompletedAtParams{Cutoff: timestampOf(cutoff), Batch: size})
		if err != nil {
			return nil, candidateFailure(anchor, err)
		}
		return candidatesOf(rows, func(row sqlc.RetentionCandidatesByCompletedAtRow) candidateRow {
			return candidateRow{
				ID: row.ID, Type: row.Type, Path: row.Path, CollectionID: row.CollectionID,
				HubID: row.HubID, AnchoredAt: row.AnchoredAt, Title: row.Title,
			}
		})
	case domain.AnchorArchivedAt:
		rows, err := queries.RetentionCandidatesByArchivedAt(ctx,
			sqlc.RetentionCandidatesByArchivedAtParams{
				Cutoff: timestampOf(cutoff), Batch: size, OwnChain: false,
			})
		if err != nil {
			return nil, candidateFailure(anchor, err)
		}
		return candidatesOf(rows, func(row sqlc.RetentionCandidatesByArchivedAtRow) candidateRow {
			return candidateRow{
				ID: row.ID, Type: row.Type, Path: row.Path, CollectionID: row.CollectionID,
				HubID: row.HubID, AnchoredAt: row.AnchoredAt, Title: row.Title,
			}
		})
	case domain.AnchorDeletedAt:
		rows, err := queries.RetentionCandidatesByDeletedAt(ctx,
			sqlc.RetentionCandidatesByDeletedAtParams{
				Cutoff: timestampOf(cutoff), Batch: size, OwnChain: false,
			})
		if err != nil {
			return nil, candidateFailure(anchor, err)
		}
		return candidatesOf(rows, func(row sqlc.RetentionCandidatesByDeletedAtRow) candidateRow {
			return candidateRow{
				ID: row.ID, Type: row.Type, Path: row.Path, CollectionID: row.CollectionID,
				HubID: row.HubID, AnchoredAt: row.AnchoredAt, Title: row.Title,
			}
		})
	}
	// An anchor this build has no statement for. A defect rather than input: the catalogue and this
	// switch are kept in step by a test, so an anchor arriving here means one of the two has moved.
	return nil, shared.Internalf("postgres: no retention statement for the anchor %q", anchor)
}

// DueInChain is the same question restricted to what a rule's own first stage acted on.
func (r RetentionMarkingRepository) DueInChain(
	ctx context.Context, anchor domain.Anchor, ruleID shared.ID, cutoff time.Time, batch int,
) ([]repository.Candidate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rule, err := uuidOf(ruleID)
	if err != nil {
		return nil, err
	}
	size := retentionBatch(batch)

	switch anchor {
	case domain.AnchorArchivedAt:
		rows, err := queries.RetentionCandidatesByArchivedAt(ctx,
			sqlc.RetentionCandidatesByArchivedAtParams{
				Cutoff: timestampOf(cutoff), Batch: size, OwnChain: true, RuleID: rule,
			})
		if err != nil {
			return nil, candidateFailure(anchor, err)
		}
		return candidatesOf(rows, func(row sqlc.RetentionCandidatesByArchivedAtRow) candidateRow {
			return candidateRow{
				ID: row.ID, Type: row.Type, Path: row.Path, CollectionID: row.CollectionID,
				HubID: row.HubID, AnchoredAt: row.AnchoredAt, Title: row.Title,
			}
		})
	case domain.AnchorDeletedAt:
		rows, err := queries.RetentionCandidatesByDeletedAt(ctx,
			sqlc.RetentionCandidatesByDeletedAtParams{
				Cutoff: timestampOf(cutoff), Batch: size, OwnChain: true, RuleID: rule,
			})
		if err != nil {
			return nil, candidateFailure(anchor, err)
		}
		return candidatesOf(rows, func(row sqlc.RetentionCandidatesByDeletedAtRow) candidateRow {
			return candidateRow{
				ID: row.ID, Type: row.Type, Path: row.Path, CollectionID: row.CollectionID,
				HubID: row.HubID, AnchoredAt: row.AnchoredAt, Title: row.Title,
			}
		})
	}
	return nil, shared.Internalf("postgres: no chain statement for the anchor %q", anchor)
}

// Mark writes what is coming, when, and under which rule.
func (r RetentionMarkingRepository) Mark(
	ctx context.Context, ids []shared.ID, ruleID shared.ID,
	action domain.Action, effectiveAt time.Time,
) (int, error) {
	queries, keys, err := batchOf(ctx, ids)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	rule, err := uuidOf(ruleID)
	if err != nil {
		return 0, err
	}

	marked, err := queries.MarkItemsForRetention(ctx, sqlc.MarkItemsForRetentionParams{
		Ids: keys, RuleID: rule, Action: optionalText(string(action)),
		EffectiveAt: timestampOf(effectiveAt),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("marking %d entries: %w", len(ids), err))
	}
	return int(marked), nil
}

// Block records what a rule would do and what is stopping it.
func (r RetentionMarkingRepository) Block(
	ctx context.Context, ids []shared.ID, ruleID shared.ID,
	action domain.Action, reason string,
) (int, error) {
	queries, keys, err := batchOf(ctx, ids)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	rule, err := uuidOf(ruleID)
	if err != nil {
		return 0, err
	}

	blocked, err := queries.BlockItemsForRetention(ctx, sqlc.BlockItemsForRetentionParams{
		Ids: keys, RuleID: rule, Action: optionalText(string(action)),
		BlockedBy: optionalText(reason),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording what holds %d entries back: %w", len(ids), err))
	}
	return int(blocked), nil
}

// MarkedDue answers the entries whose grace period has run out.
func (r RetentionMarkingRepository) MarkedDue(
	ctx context.Context, now time.Time, batch int,
) ([]repository.Candidate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.RetentionMarkedItemsDue(ctx, sqlc.RetentionMarkedItemsDueParams{
		Now: timestampOf(now), Batch: retentionBatch(batch),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the marked entries: %w", err))
	}

	return candidatesOf(rows, func(row sqlc.RetentionMarkedItemsDueRow) candidateRow {
		return candidateRow{
			ID: row.ID, Type: row.Type, Path: row.Path, CollectionID: row.CollectionID,
			HubID: row.HubID, AnchoredAt: row.RetentionPendingUntil, Title: row.Title,
			Pending: row.RetentionPendingUntil, Rule: row.RetentionRuleID,
			Action: row.RetentionAction,
		}
	})
}

// Marking answers one entry's marking.
func (r RetentionMarkingRepository) Marking(
	ctx context.Context, id shared.ID,
) (repository.Candidate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Candidate{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.Candidate{}, err
	}

	row, err := queries.FindItemRetention(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return repository.Candidate{}, shared.ErrNotFound.WithDetail("items.not_found").
				WithParams(map[string]string{"item_id": id.String()})
		}
		return repository.Candidate{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an entry's retention: %w", err))
	}

	rule, err := optionalID(row.RetentionRuleID)
	if err != nil {
		return repository.Candidate{}, err
	}
	return repository.Candidate{
		ID: id, Pending: timeFrom(row.RetentionPendingUntil), Rule: rule,
		Action:    domain.Action(stringFrom(row.RetentionAction)),
		BlockedBy: stringFrom(row.RetentionBlockedBy),
	}, nil
}

// Clear takes entries out of the running period.
func (r RetentionMarkingRepository) Clear(
	ctx context.Context, ids []shared.ID, keepRule bool, now time.Time,
) (int, error) {
	queries, keys, err := batchOf(ctx, ids)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	cleared, err := queries.ClearItemRetention(ctx, sqlc.ClearItemRetentionParams{
		Ids: keys, KeepRule: keepRule, Now: timestampOf(now),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the retention of %d entries: %w", len(ids), err))
	}
	return int(cleared), nil
}

// Archive is the act of an ARCHIVE stage.
func (r RetentionMarkingRepository) Archive(
	ctx context.Context, ids []shared.ID, at time.Time,
) (int, error) {
	queries, keys, err := batchOf(ctx, ids)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	archived, err := queries.ArchiveItemsForRetention(ctx, sqlc.ArchiveItemsForRetentionParams{
		Ids: keys, At: timestampOf(at),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("archiving %d entries: %w", len(ids), err))
	}
	return int(archived), nil
}

// Trash is the act of a TRASH stage.
func (r RetentionMarkingRepository) Trash(
	ctx context.Context, ids []shared.ID, batchID shared.ID, at time.Time,
) (int, error) {
	queries, keys, err := batchOf(ctx, ids)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	batch, err := uuidOf(batchID)
	if err != nil {
		return 0, err
	}
	trashed, err := queries.TrashItemsForRetention(ctx, sqlc.TrashItemsForRetentionParams{
		Ids: keys, BatchID: batch, At: timestampOf(at),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("trashing %d entries: %w", len(ids), err))
	}
	return int(trashed), nil
}

// RetainedDescendants is §4.6: how many entries below each of these are not going in this pass.
func (r RetentionMarkingRepository) RetainedDescendants(
	ctx context.Context, ids, going []shared.ID,
) (map[shared.ID]int, error) {
	queries, keys, err := batchOf(ctx, ids)
	if err != nil || len(keys) == 0 {
		return map[shared.ID]int{}, err
	}
	leaving, err := idsOf(going)
	if err != nil {
		return nil, err
	}

	rows, err := queries.CountRetainedDescendants(ctx, sqlc.CountRetainedDescendantsParams{
		Ids: keys, Going: leaving,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting what is kept below %d entries: %w", len(ids), err))
	}

	counted := make(map[shared.ID]int, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		counted[id] = int(row.Retained)
	}
	return counted, nil
}

// CountDue is how many entries in a rule's scope are past its cutoff.
func (r RetentionMarkingRepository) CountDue(
	ctx context.Context, anchor domain.Anchor, scope domain.Scope, cutoff time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	scopeID, err := optionalUUID(scope.ID)
	if err != nil {
		return 0, err
	}

	var due int64
	switch anchor {
	case domain.AnchorCompletedAt:
		due, err = queries.CountRetentionCandidatesByCompletedAt(ctx,
			sqlc.CountRetentionCandidatesByCompletedAtParams{
				Cutoff: timestampOf(cutoff), ScopeKind: string(scope.Kind), ScopeID: scopeID,
			})
	case domain.AnchorArchivedAt:
		due, err = queries.CountRetentionCandidatesByArchivedAt(ctx,
			sqlc.CountRetentionCandidatesByArchivedAtParams{
				Cutoff: timestampOf(cutoff), ScopeKind: string(scope.Kind), ScopeID: scopeID,
			})
	case domain.AnchorDeletedAt:
		due, err = queries.CountRetentionCandidatesByDeletedAt(ctx,
			sqlc.CountRetentionCandidatesByDeletedAtParams{
				Cutoff: timestampOf(cutoff), ScopeKind: string(scope.Kind), ScopeID: scopeID,
			})
	default:
		return 0, shared.Internalf("postgres: no retention count for the anchor %q", anchor)
	}
	if err != nil {
		return 0, candidateFailure(anchor, err)
	}
	return int(due), nil
}

// CountScope is the denominator of the five-per-cent switch.
func (r RetentionMarkingRepository) CountScope(
	ctx context.Context, scope domain.Scope,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	scopeID, err := optionalUUID(scope.ID)
	if err != nil {
		return 0, err
	}
	held, err := queries.CountRetentionScope(ctx, sqlc.CountRetentionScopeParams{
		ScopeKind: string(scope.Kind), ScopeID: scopeID,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the scope of a retention rule: %w", err))
	}
	return int(held), nil
}

// candidateRow is the shape every candidate statement answers in. sqlc gives each of them a result
// type of its own and none of them share a method, which is what the mapping function is for.
type candidateRow struct {
	ID           pgtype.UUID
	Type         sqlc.ItemType
	Path         string
	CollectionID pgtype.UUID
	HubID        pgtype.UUID
	AnchoredAt   pgtype.Timestamptz
	Title        string
	Pending      pgtype.Timestamptz
	Rule         pgtype.UUID
	Action       *string
}

func candidatesOf[T any](rows []T, take func(T) candidateRow) ([]repository.Candidate, error) {
	out := make([]repository.Candidate, 0, len(rows))
	for _, row := range rows {
		mapped := take(row)
		id, err := idFrom(mapped.ID)
		if err != nil {
			return nil, err
		}
		collectionID, err := optionalID(mapped.CollectionID)
		if err != nil {
			return nil, err
		}
		hubID, err := optionalID(mapped.HubID)
		if err != nil {
			return nil, err
		}
		rule, err := optionalID(mapped.Rule)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.Candidate{
			ID: id, Type: work.ItemType(mapped.Type), Path: mapped.Path,
			CollectionID: collectionID, HubID: hubID,
			AnchoredAt: timeFrom(mapped.AnchoredAt), Title: mapped.Title,
			Pending: timeFrom(mapped.Pending), Rule: rule,
			Action: domain.Action(stringFrom(mapped.Action)),
		})
	}
	return out, nil
}

func candidateFailure(anchor domain.Anchor, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("reading the retention candidates by %s: %w", anchor, err))
}

// batchOf is what every batched statement starts with: the transaction, and the identities as the
// array the statement takes.
func batchOf(ctx context.Context, ids []shared.ID) (*sqlc.Queries, []pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, nil, err
	}
	keys, err := idsOf(ids)
	if err != nil {
		return nil, nil, err
	}
	return queries, keys, nil
}

// DefaultRetentionBatch is the thousand objects per transaction data-retention.md §5 asks for.
const DefaultRetentionBatch = 1000

// maxRetentionBatch bounds what a caller may ask for, which is also what keeps the conversion below
// honest: a batch of two billion is a request for the memory bound to be ignored.
const maxRetentionBatch = 10_000

func retentionBatch(batch int) int32 {
	if batch <= 0 || batch > maxRetentionBatch {
		batch = DefaultRetentionBatch
	}
	return int32(batch)
}
