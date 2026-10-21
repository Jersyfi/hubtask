// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// StructureRepository stores the structure layers of hubs, collections and the workspace. No
// statement names a tenant: row level security binds each to the transaction's, so another
// workspace's container is not found and its settings are not there.
type StructureRepository struct{}

func NewStructureRepository() StructureRepository { return StructureRepository{} }

var _ repository.Structures = StructureRepository{}

// SetContainerStructure writes the container's own layer and, on a collection, its own completion.
func (r StructureRepository) SetContainerStructure(
	ctx context.Context, container work.Container, expectedVersion int,
) error {
	queries, id, err := containerWrite(ctx, container.ID)
	if err != nil {
		return err
	}
	structure, completion, err := containerLayerDocument(container)
	if err != nil {
		return err
	}

	affected, err := queries.SetContainerStructure(ctx, sqlc.SetContainerStructureParams{
		CompletionPolicy: completion,
		Structure:        structure,
		UpdatedAt:        timestampOf(container.UpdatedAt),
		ID:               id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return containerWriteError(err, container, "writing the structure")
	}
	return containerConflict(affected, container, expectedVersion)
}

// HoldContainer holds the row FOR SHARE. A container another workspace owns is not found, like a
// missing one.
func (r StructureRepository) HoldContainer(ctx context.Context, id shared.ID) error {
	queries, key, err := containerWrite(ctx, id)
	if err != nil {
		return err
	}
	if _, err := queries.LockContainerShared(ctx, key); err != nil {
		if IsNoRows(err) {
			return shared.ErrNotFound.WithDetail("containers.not_found")
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("holding the container: %w", err))
	}
	return nil
}

// Heirs answers the live containers inheriting a hub's layer, or the workspace's.
func (r StructureRepository) Heirs(ctx context.Context, hubID shared.ID, ceiling int) ([]work.Container, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := optionalUUID(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListStructureHeirs(ctx, sqlc.ListStructureHeirsParams{
		HubID: hub,
		//nolint:gosec // G115: the ceiling is a small constant of the caller's
		Ceiling: int32(ceiling),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the containers that inherit a structure: %w", err))
	}
	heirs := make([]work.Container, 0, len(rows))
	for _, row := range rows {
		container, err := containerFrom(sqlc.FindContainerRow(row))
		if err != nil {
			return nil, err
		}
		heirs = append(heirs, container)
	}
	return heirs, nil
}

// Workspace answers the workspace's layer and the tenant row's version, held as asked.
func (r StructureRepository) Workspace(
	ctx context.Context, hold repository.LayerHold,
) (work.WorkspaceStructure, int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.WorkspaceStructure{}, 0, err
	}

	var raw string
	var version int32
	switch hold {
	case repository.LayerShared:
		row, readErr := queries.FindWorkspaceStructureForShare(ctx)
		raw, version, err = row.Structure, row.Version, readErr
	case repository.LayerExclusive:
		row, readErr := queries.FindWorkspaceStructureForUpdate(ctx)
		raw, version, err = row.Structure, row.Version, readErr
	default:
		row, readErr := queries.FindWorkspaceStructure(ctx)
		raw, version, err = row.Structure, row.Version, readErr
	}
	if err != nil {
		if IsNoRows(err) {
			return work.WorkspaceStructure{}, 0, shared.ErrNotFound.WithDetail("workspace.not_found")
		}
		return work.WorkspaceStructure{}, 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the workspace's structure: %w", err))
	}

	workspace, err := workspaceLayerFrom(raw)
	if err != nil {
		return work.WorkspaceStructure{}, 0, err
	}
	return workspace, int(version), nil
}

// SetWorkspace writes the workspace's layer under the tenant row's version.
func (r StructureRepository) SetWorkspace(
	ctx context.Context, workspace work.WorkspaceStructure, expectedVersion int, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	document, err := workspaceLayerDocument(workspace)
	if err != nil {
		return err
	}
	affected, err := queries.SetWorkspaceStructure(ctx, sqlc.SetWorkspaceStructureParams{
		Structure: document,
		UpdatedAt: timestampOf(at),
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the workspace's structure: %w", err))
	}
	if affected == 0 {
		return shared.ErrVersionConflict.
			WithDetail("workspace.version_conflict").
			WithParams(map[string]string{"expected_version": strconv.Itoa(expectedVersion)})
	}
	return nil
}
