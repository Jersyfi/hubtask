// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RecurrenceRepository stores the series beside the entries (D-04).
//
// Two rows move together here, which is why the pointer's write lives in this adapter rather than
// in the application: a rule and the entry that points at it are one state, and a caller that had
// to remember both would eventually not.
type RecurrenceRepository struct{}

func NewRecurrenceRepository() RecurrenceRepository { return RecurrenceRepository{} }

var _ repository.Recurrences = RecurrenceRepository{}

// FindForItem returns the entry's series.
func (r RecurrenceRepository) FindForItem(
	ctx context.Context, itemID shared.ID,
) (work.RecurrenceRule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.RecurrenceRule{}, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return work.RecurrenceRule{}, err
	}

	row, err := queries.FindRecurrenceRuleForItem(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return work.RecurrenceRule{}, shared.ErrNotFound
		}
		return work.RecurrenceRule{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the series of %s: %w", itemID, err))
	}
	return recurrenceFrom(
		row.ID, row.TenantID, row.SourceItemID, row.Rrule, row.TimeZone, row.Mode,
		row.HorizonDays, row.EndsAt, row.MaxCount, row.LastMaterializedAt,
		row.CreatedAt, row.UpdatedAt, row.Version,
	)
}

// Insert writes the series and points the entry at it.
func (r RecurrenceRepository) Insert(ctx context.Context, rule work.RecurrenceRule) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(rule.ID)
	if err != nil {
		return err
	}
	itemID, err := uuidOf(rule.ItemID)
	if err != nil {
		return err
	}

	err = queries.InsertRecurrenceRule(ctx, sqlc.InsertRecurrenceRuleParams{
		ID:           id,
		SourceItemID: itemID,
		Rrule:        rule.RRULE,
		TimeZone:     rule.TimeZone,
		Mode:         rule.Mode.String(),
		//nolint:gosec // G115: the horizon is bounded at a year by the domain before it arrives here
		HorizonDays: int32(rule.HorizonDays),
		EndsAt:      optionalTimestamp(rule.EndsAt),
		MaxCount:    seriesCount(rule.MaxCount),
		CreatedAt:   timestampOf(rule.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the series %s: %w", rule.ID, err))
	}
	return r.point(ctx, rule.ItemID, rule.ID)
}

// Update writes the whole document under the optimistic lock.
func (r RecurrenceRepository) Update(
	ctx context.Context, rule work.RecurrenceRule, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(rule.ID)
	if err != nil {
		return err
	}
	if rule.UpdatedAt == nil {
		// The domain stamps every change; a write without the stamp is this code disagreeing with
		// itself rather than a request that can be fixed.
		return shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the change to series %s carries no stamp", rule.ID))
	}

	affected, err := queries.UpdateRecurrenceRule(ctx, sqlc.UpdateRecurrenceRuleParams{
		Rrule:    rule.RRULE,
		TimeZone: rule.TimeZone,
		Mode:     rule.Mode.String(),
		//nolint:gosec // G115: the horizon is bounded at a year by the domain before it arrives here
		HorizonDays: int32(rule.HorizonDays),
		EndsAt:      optionalTimestamp(rule.EndsAt),
		MaxCount:    seriesCount(rule.MaxCount),
		UpdatedAt:   timestampOf(*rule.UpdatedAt),
		ID:          id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the series %s: %w", rule.ID, err))
	}
	return recurrenceConflictIfUntouched(affected, rule.ID, expectedVersion)
}

// Delete removes the series and clears the entry's pointer.
func (r RecurrenceRepository) Delete(
	ctx context.Context, rule work.RecurrenceRule, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(rule.ID)
	if err != nil {
		return err
	}

	affected, err := queries.DeleteRecurrenceRule(ctx, sqlc.DeleteRecurrenceRuleParams{
		ID: id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the series %s: %w", rule.ID, err))
	}
	if err := recurrenceConflictIfUntouched(affected, rule.ID, expectedVersion); err != nil {
		return err
	}
	return r.point(ctx, rule.ItemID, "")
}

// point writes the entry's pointer at its series, or clears it. The foreign key would not catch a
// pointer left standing - the column carries none, one column for a reference the schema keeps
// deliberately loose - so the pair is written here, in one transaction, or not at all.
func (r RecurrenceRepository) point(ctx context.Context, itemID, ruleID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return err
	}
	rule, err := optionalUUID(ruleID)
	if err != nil {
		return err
	}

	if _, err := queries.SetWorkItemRecurrence(ctx, sqlc.SetWorkItemRecurrenceParams{
		RecurrenceRuleID: rule,
		ID:               id,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("pointing entry %s at its series: %w", itemID, err))
	}
	return nil
}

// ClaimToMaterialize takes the series whose window may owe something, locking the rows.
func (r RecurrenceRepository) ClaimToMaterialize(
	ctx context.Context, now time.Time, limit int,
) ([]work.RecurrenceRule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ClaimRulesToMaterialize(ctx, sqlc.ClaimRulesToMaterializeParams{
		Now: timestampOf(now),
		//nolint:gosec // G115: the batch is this process's own constant, not a value from a request
		BatchSize: int32(limit),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming the series to materialise: %w", err))
	}

	rules := make([]work.RecurrenceRule, 0, len(rows))
	for _, row := range rows {
		rule, err := recurrenceFrom(
			row.ID, row.TenantID, row.SourceItemID, row.Rrule, row.TimeZone, row.Mode,
			row.HorizonDays, row.EndsAt, row.MaxCount, row.LastMaterializedAt,
			row.CreatedAt, row.UpdatedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// Advance moves the watermark under the compare-and-set, and reports whether this caller moved it.
//
// The rule carries the watermark it was read with, which is what the statement compares against:
// the caller passes the rule it decided from rather than a value it might have derived twice.
func (r RecurrenceRepository) Advance(
	ctx context.Context, rule work.RecurrenceRule, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(rule.ID)
	if err != nil {
		return false, err
	}

	affected, err := queries.AdvanceRecurrenceWatermark(ctx, sqlc.AdvanceRecurrenceWatermarkParams{
		MaterializedAt: timestampOf(at),
		ID:             id,
		Expected:       optionalTimestamp(rule.LastMaterializedAt),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("advancing the series %s: %w", rule.ID, err))
	}
	return affected != 0, nil
}

// OpenOccurrences counts what a series still has open.
func (r RecurrenceRepository) OpenOccurrences(
	ctx context.Context, ruleID shared.ID,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(ruleID)
	if err != nil {
		return 0, err
	}

	count, err := queries.CountOpenOccurrences(ctx, id)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the open occurrences of %s: %w", ruleID, err))
	}
	return int(count), nil
}

// LatestCompletion answers when the series was last completed.
func (r RecurrenceRepository) LatestCompletion(
	ctx context.Context, ruleID shared.ID,
) (*time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(ruleID)
	if err != nil {
		return nil, err
	}

	completed, err := queries.LatestOccurrenceCompletion(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the last completion of %s: %w", ruleID, err))
	}
	return optionalTime(completed), nil
}

// recurrenceConflictIfUntouched is the shared answer for a write that matched nothing: the row
// moved on, or - through row level security - was never this tenant's to move. One answer for
// both, deliberately (multi-tenancy.md §2).
func recurrenceConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("recurrence.version_conflict").
		WithParams(map[string]string{
			"recurrence_rule_id": id.String(), "expected_version": fmt.Sprint(expectedVersion),
		})
}

// seriesCount maps the domain's "no limit" onto the column's NULL. Zero is not a count: a series
// that produces no occurrence is not a series.
func seriesCount(count int) *int32 {
	if count <= 0 {
		return nil
	}
	//nolint:gosec // G115: the count is bounded by the expansion check before it reaches here
	value := int32(count)
	return &value
}

// recurrenceFrom maps a stored row onto the domain's rule.
func recurrenceFrom(
	id, tenantID, itemID pgtype.UUID, rule, zone, mode string, horizon int32,
	endsAt pgtype.Timestamptz, maxCount *int32,
	lastMaterializedAt, createdAt, updatedAt pgtype.Timestamptz, version int32,
) (work.RecurrenceRule, error) {
	ruleID, err := idFrom(id)
	if err != nil {
		return work.RecurrenceRule{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return work.RecurrenceRule{}, err
	}
	item, err := idFrom(itemID)
	if err != nil {
		return work.RecurrenceRule{}, err
	}
	if !createdAt.Valid {
		return work.RecurrenceRule{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	count := 0
	if maxCount != nil {
		count = int(*maxCount)
	}
	return work.RecurrenceRule{
		ID:                 ruleID,
		TenantID:           tenant,
		ItemID:             item,
		RRULE:              rule,
		TimeZone:           zone,
		Mode:               work.RecurrenceMode(mode),
		HorizonDays:        int(horizon),
		EndsAt:             optionalTime(endsAt),
		MaxCount:           count,
		LastMaterializedAt: optionalTime(lastMaterializedAt),
		CreatedAt:          timeFrom(createdAt),
		UpdatedAt:          optionalTime(updatedAt),
		Version:            int(version),
	}, nil
}
