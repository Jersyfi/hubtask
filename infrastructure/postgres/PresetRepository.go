// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PresetRepository keeps the saved presets (db/queries/Preset.sql). Their settings are stored in
// the document a layer's settings key holds (StructureDocument.go), so a preset and a layer read
// and write one shape.
type PresetRepository struct{}

func NewPresetRepository() PresetRepository { return PresetRepository{} }

var _ repository.Presets = PresetRepository{}

func presetFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func presetNotFound(id shared.ID) error {
	return shared.ErrNotFound.WithDetail("presets.not_found").
		WithParams(map[string]string{"preset_id": id.String()})
}

// Find answers one saved preset. Another tenant's is not found (multi-tenancy.md §2).
func (PresetRepository) Find(ctx context.Context, id shared.ID) (work.Preset, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Preset{}, err
	}
	rowID, err := uuidOf(id)
	if err != nil {
		return work.Preset{}, presetNotFound(id)
	}
	row, err := queries.FindStructurePreset(ctx, rowID)
	if err != nil {
		if IsNoRows(err) {
			return work.Preset{}, presetNotFound(id)
		}
		return work.Preset{}, presetFailed("reading a preset", err)
	}
	return presetFrom(row.ID, row.Origin, row.OwnerID, row.Name, row.Settings, row.Version)
}

// ListOffered answers the workspace's presets and the owner's own.
func (PresetRepository) ListOffered(ctx context.Context, ownerID shared.ID) ([]work.Preset, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	owner, err := optionalUUID(ownerID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListStructurePresetsOffered(ctx, owner)
	if err != nil {
		return nil, presetFailed("listing the presets", err)
	}
	presets := make([]work.Preset, 0, len(rows))
	for _, row := range rows {
		preset, err := presetFrom(row.ID, row.Origin, row.OwnerID, row.Name, row.Settings, row.Version)
		if err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}
	return presets, nil
}

// Count answers how many presets of the origin there are.
func (PresetRepository) Count(ctx context.Context, origin work.PresetOrigin, ownerID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	owner, err := optionalUUID(ownerID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountStructurePresets(ctx, sqlc.CountStructurePresetsParams{
		Origin: string(origin), OwnerID: owner,
	})
	if err != nil {
		return 0, presetFailed("counting the presets", err)
	}
	return int(count), nil
}

// Insert stores a new saved preset.
func (PresetRepository) Insert(ctx context.Context, preset work.Preset, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(preset.ID)
	if err != nil {
		return err
	}
	owner, err := optionalUUID(preset.OwnerID)
	if err != nil {
		return err
	}
	settings, err := presetSettingsOf(preset)
	if err != nil {
		return err
	}
	if err := queries.InsertStructurePreset(ctx, sqlc.InsertStructurePresetParams{
		ID: id, Origin: string(preset.Origin), OwnerID: owner, Name: preset.Name,
		Settings: settings, CreatedAt: timestampOf(at),
	}); err != nil {
		return presetFailed("saving a preset", err)
	}
	return nil
}

// Update writes the preset's name and values.
func (PresetRepository) Update(ctx context.Context, preset work.Preset, expectedVersion int, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(preset.ID)
	if err != nil {
		return err
	}
	settings, err := presetSettingsOf(preset)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateStructurePreset(ctx, sqlc.UpdateStructurePresetParams{
		Name: preset.Name, Settings: settings, UpdatedAt: timestampOf(at), ID: id,
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // a row version is far below 2^31
	})
	if err != nil {
		return presetFailed("changing a preset", err)
	}
	return presetConflictIfUntouched(affected, preset.ID, expectedVersion)
}

// Delete removes the preset.
func (PresetRepository) Delete(ctx context.Context, id shared.ID, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	rowID, err := uuidOf(id)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteStructurePreset(ctx, sqlc.DeleteStructurePresetParams{
		ID: rowID, ExpectedVersion: int32(expectedVersion), //nolint:gosec // a row version is far below 2^31
	})
	if err != nil {
		return presetFailed("withdrawing a preset", err)
	}
	return presetConflictIfUntouched(affected, id, expectedVersion)
}

// presetConflictIfUntouched answers a version conflict where nothing was written: the row moved
// on, is gone, or is another tenant's - which a caller must not tell apart.
func presetConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.WithDetail("presets.version_conflict").
		WithParams(map[string]string{
			"preset_id": id.String(), "expected_version": strconv.Itoa(expectedVersion),
		})
}

func presetSettingsOf(preset work.Preset) ([]byte, error) {
	document := valuesDocument(preset.Values)
	if document == nil {
		document = &structureSettingsDocument{}
	}
	settings, err := json.Marshal(document)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("presets.unencodable").
			WithCause(fmt.Errorf("encoding the preset %s: %w", preset.ID, err))
	}
	return settings, nil
}

func presetFrom(
	rowID pgtype.UUID, origin string, rowOwner pgtype.UUID, name string, settings []byte, version int32,
) (work.Preset, error) {
	id, err := idFrom(rowID)
	if err != nil {
		return work.Preset{}, err
	}
	owner, err := optionalID(rowOwner)
	if err != nil {
		return work.Preset{}, err
	}
	var document structureSettingsDocument
	if err := json.Unmarshal(settings, &document); err != nil {
		return work.Preset{}, shared.ErrInternal.WithDetail("presets.undecodable").
			WithCause(fmt.Errorf("decoding the preset %s: %w", id, err))
	}
	return work.Preset{
		Key: work.SavedPresetKey(id), Origin: work.PresetOrigin(origin), ID: id, Name: name,
		OwnerID: owner, Values: valuesFrom(&document), Version: int(version),
	}, nil
}
