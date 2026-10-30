// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// DirectoryRepository is the directory's identifiers on account and account_group (migration
// 0256). Like every repository here it takes no tenant: row level security bounds it.
type DirectoryRepository struct{}

func NewDirectoryRepository() DirectoryRepository { return DirectoryRepository{} }

var _ repository.DirectoryLinks = DirectoryRepository{}

func directoryFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("directory: %s: %w", what, err))
}

func (DirectoryRepository) AccountExternalIDs(ctx context.Context, ids []shared.ID) (map[shared.ID]string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return nil, err
	}
	rows, err := queries.AccountExternalIDs(ctx, keys)
	if err != nil {
		return nil, directoryFailed("reading people's identifiers", err)
	}
	out := make(map[shared.ID]string, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		out[id] = row.ExternalID
	}
	return out, nil
}

func (DirectoryRepository) FindAccountByExternalID(ctx context.Context, externalID string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	row, err := queries.FindAccountIDByExternalID(ctx, &externalID)
	if err != nil {
		if IsNoRows(err) {
			return "", shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return "", directoryFailed("finding a person by identifier", err)
	}
	return idFrom(row)
}

func (DirectoryRepository) SetAccountExternalID(
	ctx context.Context, accountID shared.ID, externalID string, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	moved, err := queries.SetAccountExternalID(ctx, sqlc.SetAccountExternalIDParams{
		ExternalID: optionalText(externalID), Now: timestampOf(at), ID: id,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.WithDetail("directory.external_id_taken")
		}
		return directoryFailed("writing a person's identifier", err)
	}
	if moved == 0 {
		return shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return nil
}

func (DirectoryRepository) SetAccountStatus(
	ctx context.Context, accountID shared.ID, status identity.AccountStatus, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	moved, err := queries.SetAccountDirectoryStatus(ctx, sqlc.SetAccountDirectoryStatusParams{
		Status: sqlc.AccountStatus(status), Now: timestampOf(at), ID: id,
	})
	if err != nil {
		return false, directoryFailed("switching a person", err)
	}
	return moved > 0, nil
}

func (DirectoryRepository) PageAccounts(ctx context.Context, offset, limit int) ([]shared.ID, int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountDirectoryAccounts(ctx)
	if err != nil {
		return nil, 0, directoryFailed("counting people", err)
	}
	rows, err := queries.PageDirectoryAccounts(ctx, sqlc.PageDirectoryAccountsParams{
		RowOffset: int32(offset), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, 0, directoryFailed("paging people", err)
	}
	ids, err := idsFrom(rows)
	return ids, int(total), err
}

func (DirectoryRepository) GroupExternalIDs(ctx context.Context, ids []shared.ID) (map[shared.ID]string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return nil, err
	}
	rows, err := queries.GroupExternalIDs(ctx, keys)
	if err != nil {
		return nil, directoryFailed("reading groups' identifiers", err)
	}
	out := make(map[shared.ID]string, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		out[id] = row.ExternalID
	}
	return out, nil
}

func (DirectoryRepository) FindGroupByExternalID(ctx context.Context, externalID string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	row, err := queries.FindGroupIDByExternalID(ctx, &externalID)
	return groupIDOrNotFound(row, err, "finding a group by identifier")
}

func (DirectoryRepository) FindGroupByName(ctx context.Context, name string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	row, err := queries.FindGroupIDByName(ctx, name)
	return groupIDOrNotFound(row, err, "finding a group by name")
}

func groupIDOrNotFound(row pgtype.UUID, err error, what string) (shared.ID, error) {
	if err != nil {
		if IsNoRows(err) {
			return "", shared.ErrNotFound.WithDetail("groups.not_found")
		}
		return "", directoryFailed(what, err)
	}
	return idFrom(row)
}

func (DirectoryRepository) SetGroupExternalID(ctx context.Context, groupID shared.ID, externalID string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(groupID)
	if err != nil {
		return err
	}
	moved, err := queries.SetGroupExternalID(ctx, sqlc.SetGroupExternalIDParams{
		ExternalID: optionalText(externalID), ID: id,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.WithDetail("directory.external_id_taken")
		}
		return directoryFailed("writing a group's identifier", err)
	}
	if moved == 0 {
		return shared.ErrNotFound.WithDetail("groups.not_found")
	}
	return nil
}

func (DirectoryRepository) PageGroups(ctx context.Context, offset, limit int) ([]shared.ID, int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountDirectoryGroups(ctx)
	if err != nil {
		return nil, 0, directoryFailed("counting groups", err)
	}
	rows, err := queries.PageDirectoryGroups(ctx, sqlc.PageDirectoryGroupsParams{
		RowOffset: int32(offset), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, 0, directoryFailed("paging groups", err)
	}
	ids, err := idsFrom(rows)
	return ids, int(total), err
}
