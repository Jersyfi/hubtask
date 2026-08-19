// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// LifecycleRepository is the end of data's life: the instructions not to delete, and the record of
// what was deleted anyway (ADR-0020).
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below - which matters more here than anywhere else, because
// a legal hold read across the wrong boundary would not be a wrong answer but somebody else's
// obligation ignored.
type LifecycleRepository struct{}

func NewLifecycleRepository() LifecycleRepository { return LifecycleRepository{} }

var (
	_ repository.LegalHolds = LifecycleRepository{}
	_ repository.Removals   = LifecycleRepository{}
	_ repository.Expired    = LifecycleRepository{}
	_ repository.Policies   = LifecycleRepository{}
	_ repository.Runs       = LifecycleRepository{}
)

// Active returns the holds in force for this tenant.
func (r LifecycleRepository) Active(ctx context.Context) (domain.Holds, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ActiveLegalHolds(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the legal holds: %w", err))
	}

	holds := make(domain.Holds, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		// A tenant-wide hold names nothing, which is a NULL rather than a missing row.
		scopeID, err := optionalID(row.ScopeID)
		if err != nil {
			return nil, err
		}
		holds = append(holds, domain.LegalHold{
			ID: id, Scope: domain.HoldScope(row.ScopeKind), ScopeID: scopeID,
			Reason: row.Reason, PlacedAt: timeFrom(row.PlacedAt),
		})
	}
	return holds, nil
}

// Record writes a journal entry and a tombstone for each removal.
//
// Two statements per table rather than one per row: a retention run removes in batches of a
// thousand, and a statement per row would make the transaction as long as the batch. The removals
// are grouped by table here rather than at every call site, so that a purge of a hub - containers
// and entries in one act - stays a single call.
func (r LifecycleRepository) Record(
	ctx context.Context, removals []domain.Removal, deletedAt, purgeAfter time.Time,
) error {
	if len(removals) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	byEntity, reasons, order, err := groupRemovals(removals)
	if err != nil {
		return err
	}

	for _, entity := range order {
		ids := byEntity[entity]
		err := queries.RecordDeletions(ctx, sqlc.RecordDeletionsParams{
			Entity: entity, DeletedAt: timestampOf(deletedAt),
			Reason: string(reasons[entity]), EntityIds: ids,
		})
		if err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("journalling %d removals from %s: %w", len(ids), entity, err))
		}

		err = queries.RecordTombstones(ctx, sqlc.RecordTombstonesParams{
			Entity: entity, DeletedAt: timestampOf(deletedAt),
			PurgeAfter: timestampOf(purgeAfter), EntityIds: ids,
		})
		if err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("marking %d removals from %s: %w", len(ids), entity, err))
		}
	}
	return nil
}

// groupRemovals sorts the removals by the table they came from, keeping the order they arrived in.
//
// The order is kept because a purge works a subtree from the bottom up (data-retention.md §4.6) and
// the containers have to be journalled in the order the caller decided, not in map order - which in
// Go is deliberately not an order at all.
//
// One reason per table, and a second reason for the same table is a defect: a single call is one
// act, and an act with two reasons is two acts written as one.
func groupRemovals(
	removals []domain.Removal,
) (map[string][]pgtype.UUID, map[string]domain.DeletionReason, []string, error) {
	byEntity := map[string][]pgtype.UUID{}
	reasons := map[string]domain.DeletionReason{}
	order := make([]string, 0, 2)

	for _, removal := range removals {
		if removal.Entity == "" || removal.EntityID.IsZero() || removal.Reason == "" {
			return nil, nil, nil, shared.ErrInternal.WithDetail("lifecycle.removal_incomplete")
		}
		id, err := uuidOf(removal.EntityID)
		if err != nil {
			return nil, nil, nil, err
		}

		if _, seen := reasons[removal.Entity]; !seen {
			reasons[removal.Entity] = removal.Reason
			order = append(order, removal.Entity)
		}
		if reasons[removal.Entity] != removal.Reason {
			return nil, nil, nil, shared.ErrInternal.WithDetail("lifecycle.removal_reasons_mixed")
		}
		byEntity[removal.Entity] = append(byEntity[removal.Entity], id)
	}
	return byEntity, reasons, order, nil
}

// Items returns the entries whose time in the trash is up, deepest first.
func (r LifecycleRepository) Items(
	ctx context.Context, cutoff time.Time, batchSize int,
) ([]repository.ExpiredItem, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ExpiredTrashItems(ctx, sqlc.ExpiredTrashItemsParams{
		Cutoff: timestampOf(cutoff), BatchSize: batchLimit(batchSize),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the expired entries: %w", err))
	}

	expired := make([]repository.ExpiredItem, 0, len(rows))
	for _, row := range rows {
		id, collection, err := idPair(row.ID, row.CollectionID)
		if err != nil {
			return nil, err
		}
		// A collection always sits in a hub (I-C1), so this is absent only for a row the schema
		// should not hold - and an empty identifier is what the hold check reads as "no hub named".
		hub, err := optionalID(row.HubID)
		if err != nil {
			return nil, err
		}
		expired = append(expired, repository.ExpiredItem{
			ID: id, Type: work.ItemType(row.Type), Path: row.Path,
			CollectionID: collection, HubID: hub, DeletedAt: timeFrom(row.DeletedAt),
		})
	}
	return expired, nil
}

// Containers returns the hubs and collections whose time in the trash is up, collections first.
func (r LifecycleRepository) Containers(
	ctx context.Context, cutoff time.Time, batchSize int,
) ([]repository.ExpiredContainer, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ExpiredTrashContainers(ctx, sqlc.ExpiredTrashContainersParams{
		Cutoff: timestampOf(cutoff), BatchSize: batchLimit(batchSize),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the expired containers: %w", err))
	}

	expired := make([]repository.ExpiredContainer, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		parent, err := optionalID(row.ParentID)
		if err != nil {
			return nil, err
		}
		expired = append(expired, repository.ExpiredContainer{
			ID: id, Type: work.ContainerType(row.Type), ParentID: parent,
			DeletedAt: timeFrom(row.DeletedAt),
		})
	}
	return expired, nil
}

// batchLimit bounds what can be asked of the database, whatever arrives from above.
//
// The second bound rather than the first: the caller clamps the configured batch, so a value
// reaching here outside the range means it did not - and an unbounded read on a deletion path is
// the one place where trusting a caller's arithmetic is worst.
func batchLimit(size int) int32 {
	switch {
	case size < 1:
		return 1
	case size > maxPurgeBatch:
		return maxPurgeBatch
	default:
		return int32(size)
	}
}

// maxPurgeBatch is the ceiling on one read of what may be removed. data-retention.md §5 puts the
// default at a thousand objects per transaction; this is the hard limit around that knob.
const maxPurgeBatch = 10000

// Ensure writes the documented defaults for a tenant that has none.
func (r LifecycleRepository) Ensure(ctx context.Context, policies []domain.Policy) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	for _, policy := range policies {
		err := queries.EnsureRetentionPolicy(ctx, sqlc.EnsureRetentionPolicyParams{
			DataKind:   string(policy.DataKind),
			RetainDays: dayColumn(policy.RetainDays),
			MinDays:    dayColumn(policy.MinDays),
		})
		if err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("seeding the %s policy: %w", policy.DataKind, err))
		}
	}
	return nil
}

// Find returns the period in force for one kind.
func (r LifecycleRepository) Find(
	ctx context.Context, kind domain.DataKind,
) (domain.Policy, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Policy{}, err
	}

	row, err := queries.FindRetentionPolicy(ctx, string(kind))
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant: row level security removed it
			// from the result, and the caller must not be able to tell the two apart.
			return domain.Policy{}, shared.ErrNotFound.WithDetail("lifecycle.policy_not_found")
		}
		return domain.Policy{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the %s policy: %w", kind, err))
	}

	policy := domain.Policy{
		DataKind:   domain.DataKind(row.DataKind),
		RetainDays: int(row.RetainDays),
		MinDays:    int(row.MinDays),
	}
	if row.MaxDays != nil {
		maximum := int(*row.MaxDays)
		policy.MaxDays = &maximum
	}
	return policy, nil
}

// dayColumn narrows a period for the column that holds it.
//
// Bounded rather than converted: a period is a number of days, and the column's own CHECK refuses a
// negative one - but a value arriving from outside this process should not be able to become one by
// overflowing on the way in. A century is far beyond any period a tenant would set and still nowhere
// near the column's range.
func dayColumn(days int) int32 {
	switch {
	case days < 0:
		return 0
	case days > maxRetainDays:
		return maxRetainDays
	default:
		return int32(days)
	}
}

// maxRetainDays is a hundred years. Not a policy limit but a safety one, like MaxPoolConns: a
// four-digit period is a plan, and a six-digit one is a typo.
const maxRetainDays = 36500

// Start opens the log entry for one retention run.
func (r LifecycleRepository) Start(
	ctx context.Context, id shared.ID, kind domain.DataKind, startedAt time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}

	err = queries.StartRetentionRun(ctx, sqlc.StartRetentionRunParams{
		ID: key, DataKind: string(kind), StartedAt: timestampOf(startedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("opening the retention run: %w", err))
	}
	return nil
}

// Finish closes the log entry with what the run did.
func (r LifecycleRepository) Finish(
	ctx context.Context, id shared.ID, result repository.RunResult,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}

	// An object keyed by reason rather than a total: "twelve were kept" is not something an operator
	// can act on, and "twelve were kept by a legal hold" is. Always an object, empty when nothing was
	// kept, so a reader never has to tell an absent map from an empty one.
	reasons := result.Blocked
	if reasons == nil {
		reasons = map[string]int{}
	}
	encoded, err := json.Marshal(reasons)
	if err != nil {
		return shared.ErrInternal.
			WithDetail("lifecycle.run_unrecordable").
			WithCause(fmt.Errorf("encoding the blocked reasons: %w", err))
	}

	blocked := 0
	for _, count := range reasons {
		blocked += count
	}

	affected, err := queries.FinishRetentionRun(ctx, sqlc.FinishRetentionRunParams{
		Matched:        countColumn(result.Matched),
		Affected:       countColumn(result.Removed),
		Blocked:        countColumn(blocked),
		BlockedReasons: encoded,
		FinishedAt:     timestampOf(result.FinishedAt),
		Status:         string(result.Status),
		ID:             key,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("closing the retention run: %w", err))
	}
	if affected == 0 {
		// The row this run opened is gone, or belongs to somebody else. Either way the run has no
		// log, which is the one thing this method exists to prevent.
		return shared.ErrInternal.WithDetail("lifecycle.run_unrecordable")
	}
	return nil
}

// countColumn narrows a count for the column that holds it. Bounded rather than converted, for the
// reason dayColumn is: a number arriving from outside should not become a negative one on the way in.
func countColumn(count int) int32 {
	switch {
	case count < 0:
		return 0
	case count > maxRunCount:
		return maxRunCount
	default:
		return int32(count)
	}
}

// maxRunCount is what one run's counters are capped at. Far beyond any batch this system reads and
// well inside the column's range.
const maxRunCount = 1 << 30
