// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// What a legal hold keeps from an erasure, stored (data-protection.md §4.1): the person's rows on
// entries with where those entries are, the deletes scoped to what was judged, and the record of
// what each hold kept. Methods of PrivacyRepository, in a file of their own because they are one
// subject.

var _ repository.Kept = PrivacyRepository{}

// countOf narrows a count to the column, bounded so the narrowing cannot wrap into a negative the
// CHECK would refuse.
func countOf(n int) int32 {
	switch {
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	default:
		return int32(n)
	}
}

// Lock takes the case's row for the rest of the transaction.
func (r PrivacyRepository) Lock(ctx context.Context, id shared.ID) (bool, error) {
	queries, key, err := r.accountQuery(ctx, id)
	if err != nil {
		return false, err
	}
	if _, err := queries.LockDataSubjectRequest(ctx, key); err != nil {
		if IsNoRows(err) {
			return false, nil
		}
		return false, erasureFailed("locking a case", err)
	}
	return true, nil
}

// Contributions answers the person's rows on entries, and where each entry is.
func (r PrivacyRepository) Contributions(
	ctx context.Context, accountID shared.ID,
) ([]repository.Contribution, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ErasureContributions(ctx, id)
	if err != nil {
		return nil, erasureFailed("reading what an account contributed", err)
	}

	contributions := make([]repository.Contribution, 0, len(rows))
	for _, row := range rows {
		contribution := repository.Contribution{
			Kind: repository.ContributionKind(row.Kind), Path: row.Path,
		}
		for _, field := range []struct {
			into  *shared.ID
			value pgtype.UUID
		}{
			{&contribution.ID, row.ID}, {&contribution.ItemID, row.ItemID},
			{&contribution.CollectionID, row.CollectionID}, {&contribution.HubID, row.HubID},
			{&contribution.ItemCreatedBy, row.ItemCreatedBy},
		} {
			value, err := optionalID(field.value)
			if err != nil {
				return nil, err
			}
			*field.into = value
		}
		contributions = append(contributions, contribution)
	}
	return contributions, nil
}

// CountIntake answers how much of the intake carries the person's address.
func (r PrivacyRepository) CountIntake(ctx context.Context, accountID shared.ID) (int, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountIntakeOf(ctx, id)
	if err != nil {
		return 0, erasureFailed("counting the intake of an account", err)
	}
	return int(count), nil
}

// KeepsAccount reports whether a case still keeps this account because of a hold.
func (r PrivacyRepository) KeepsAccount(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return false, err
	}
	kept, err := queries.ErasureKeepsAccount(ctx, id)
	if err != nil {
		return false, erasureFailed("reading whether an erasure keeps an account", err)
	}
	return kept, nil
}

// DeleteComments removes the person's comments named.
func (r PrivacyRepository) DeleteComments(
	ctx context.Context, accountID shared.ID, ids []shared.ID,
) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return 0, err
	}
	rows, err := queries.DeleteCommentsOf(ctx, sqlc.DeleteCommentsOfParams{AuthorID: id, Ids: keys})
	if err != nil {
		return 0, erasureFailed("removing the comments of an account", err)
	}
	return int(rows), nil
}

// ReleaseAssignmentsOn hands the entries named back to nobody.
func (r PrivacyRepository) ReleaseAssignmentsOn(
	ctx context.Context, accountID shared.ID, itemIDs []shared.ID, at time.Time,
) (int, error) {
	if len(itemIDs) == 0 {
		return 0, nil
	}
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}
	keys, err := uuidsOf(itemIDs)
	if err != nil {
		return 0, err
	}
	rows, err := queries.ClearAssignmentsOn(ctx, sqlc.ClearAssignmentsOnParams{
		AccountID: id, Ids: keys, UpdatedAt: timestampOf(at),
	})
	if err != nil {
		return 0, erasureFailed("releasing the assignments of an account", err)
	}
	return int(rows), nil
}

// RecordKept makes the case's pending parts exactly these.
func (r PrivacyRepository) RecordKept(
	ctx context.Context, requestID shared.ID, kept []domain.Kept, at time.Time,
) error {
	queries, request, err := r.accountQuery(ctx, requestID)
	if err != nil {
		return err
	}

	still := make([]pgtype.UUID, 0, len(kept))
	for _, part := range kept {
		hold, err := uuidOf(part.HoldID)
		if err != nil {
			return err
		}
		scopeID, err := optionalUUID(part.HoldScopeID)
		if err != nil {
			return err
		}
		if err := queries.UpsertErasureKept(ctx, sqlc.UpsertErasureKeptParams{
			RequestID: request, HoldID: hold, HoldScope: string(part.HoldScope), HoldScopeID: scopeID,
			Account: part.Account, Entries: countOf(part.Entries), Comments: countOf(part.Comments),
			Assignments: countOf(part.Assignments), Intake: countOf(part.Intake),
			RecordedAt: timestampOf(at),
		}); err != nil {
			return erasureFailed("recording what a hold kept", err)
		}
		still = append(still, hold)
	}

	if _, err := queries.EraseErasureKept(ctx, sqlc.EraseErasureKeptParams{
		RequestID: request, ErasedAt: timestampOf(at), StillKeeping: still,
	}); err != nil {
		return erasureFailed("recording what was erased after a hold", err)
	}
	return nil
}

// KeptOf answers the parts of each case named.
func (r PrivacyRepository) KeptOf(
	ctx context.Context, requestIDs []shared.ID,
) (map[shared.ID][]domain.Kept, error) {
	parts := map[shared.ID][]domain.Kept{}
	if len(requestIDs) == 0 {
		return parts, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(requestIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ErasureKeptOf(ctx, keys)
	if err != nil {
		return nil, erasureFailed("reading what holds kept", err)
	}

	for _, row := range rows {
		request, err := idFrom(row.RequestID)
		if err != nil {
			return nil, err
		}
		hold, err := idFrom(row.HoldID)
		if err != nil {
			return nil, err
		}
		scopeID, err := optionalID(row.HoldScopeID)
		if err != nil {
			return nil, err
		}
		part := domain.Kept{
			HoldID: hold, HoldScope: lifecycle.HoldScope(row.HoldScope), HoldScopeID: scopeID,
			Account: row.Account, Entries: int(row.Entries), Comments: int(row.Comments),
			Assignments: int(row.Assignments), Intake: int(row.Intake),
			RecordedAt: timeFrom(row.RecordedAt), ErasedAt: timeFrom(row.ErasedAt),
			BlockedCode: stringFrom(row.BlockedCode),
		}
		if len(row.BlockedParams) > 0 {
			if err := json.Unmarshal(row.BlockedParams, &part.BlockedParams); err != nil {
				return nil, shared.Internalf("postgres: a kept part's parameters are unreadable")
			}
		}
		parts[request] = append(parts[request], part)
	}
	return parts, nil
}

// PendingKept answers the cases something is still kept for.
func (r PrivacyRepository) PendingKept(
	ctx context.Context, holdID shared.ID,
) ([]repository.PendingKept, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hold, err := optionalUUID(holdID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PendingErasureKept(ctx, hold)
	if err != nil {
		return nil, erasureFailed("reading what holds still keep", err)
	}

	pending := make([]repository.PendingKept, 0, len(rows))
	for _, row := range rows {
		request, err := idFrom(row.RequestID)
		if err != nil {
			return nil, err
		}
		holdID, err := idFrom(row.HoldID)
		if err != nil {
			return nil, err
		}
		pending = append(pending, repository.PendingKept{RequestID: request, HoldID: holdID})
	}
	return pending, nil
}

// BlockKept records why the rest of a case could not be erased.
func (r PrivacyRepository) BlockKept(
	ctx context.Context, requestID shared.ID, code string, params map[string]string,
) error {
	queries, request, err := r.accountQuery(ctx, requestID)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encoding why a remainder waits: %w", err)
	}
	if _, err := queries.BlockErasureKept(ctx, sqlc.BlockErasureKeptParams{
		RequestID: request, BlockedCode: &code, BlockedParams: encoded,
	}); err != nil {
		return erasureFailed("recording why a remainder waits", err)
	}
	return nil
}
