// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// InstallationPresetRepository keeps the installation's presets (db/queries/InstallationPreset.sql).
// Their settings are a layer's settings document (StructureDocument.go), as a saved preset's are.
type InstallationPresetRepository struct{}

func NewInstallationPresetRepository() InstallationPresetRepository {
	return InstallationPresetRepository{}
}

var _ repository.InstallationPresets = InstallationPresetRepository{}

func presetNameTaken() error {
	return shared.ErrConflict.WithDetail("presets.name_taken").
		WithFields(shared.FieldError{Path: "/name", Code: "presets.name_taken"})
}

// List answers every preset by name.
func (InstallationPresetRepository) List(ctx context.Context) ([]work.InstallationPreset, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListInstallationPresets(ctx)
	if err != nil {
		return nil, presetFailed("listing the installation's presets", err)
	}
	presets := make([]work.InstallationPreset, 0, len(rows))
	for _, row := range rows {
		preset, err := installationPresetFrom(row.ID, row.Name, row.Settings, row.IsDefault, row.Locks, row.Version)
		if err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}
	return presets, nil
}

// Find answers one.
func (InstallationPresetRepository) Find(ctx context.Context, id shared.ID) (work.InstallationPreset, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.InstallationPreset{}, err
	}
	rowID, err := uuidOf(id)
	if err != nil {
		return work.InstallationPreset{}, presetNotFound(id)
	}
	row, err := queries.FindInstallationPreset(ctx, rowID)
	if err != nil {
		if IsNoRows(err) {
			return work.InstallationPreset{}, presetNotFound(id)
		}
		return work.InstallationPreset{}, presetFailed("reading an installation preset", err)
	}
	return installationPresetFrom(row.ID, row.Name, row.Settings, row.IsDefault, row.Locks, row.Version)
}

// Default answers the default preset, where there is one.
func (InstallationPresetRepository) Default(ctx context.Context) (work.InstallationPreset, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.InstallationPreset{}, false, err
	}
	row, err := queries.DefaultInstallationPreset(ctx)
	if err != nil {
		if IsNoRows(err) {
			return work.InstallationPreset{}, false, nil
		}
		return work.InstallationPreset{}, false, presetFailed("reading the installation's default preset", err)
	}
	preset, err := installationPresetFrom(row.ID, row.Name, row.Settings, row.IsDefault, row.Locks, row.Version)
	return preset, err == nil, err
}

// Count answers how many there are.
func (InstallationPresetRepository) Count(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountInstallationPresets(ctx)
	if err != nil {
		return 0, presetFailed("counting the installation's presets", err)
	}
	return int(count), nil
}

// Insert stores a new preset.
func (InstallationPresetRepository) Insert(
	ctx context.Context, preset work.InstallationPreset, by shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(preset.ID)
	if err != nil {
		return err
	}
	author, err := optionalUUID(by)
	if err != nil {
		return err
	}
	settings, err := presetSettingsOf(preset.Preset)
	if err != nil {
		return err
	}
	inserted, err := queries.InsertInstallationPreset(ctx, sqlc.InsertInstallationPresetParams{
		ID: id, Name: preset.Name, Settings: settings, IsDefault: preset.IsDefault,
		Locks: lockStrings(preset.Locks), Now: timestampOf(at), UpdatedBy: author,
	})
	if err != nil {
		return presetFailed("saving an installation preset", err)
	}
	if inserted == 0 {
		return presetNameTaken()
	}
	return nil
}

// Update writes a preset whole.
func (InstallationPresetRepository) Update(
	ctx context.Context, preset work.InstallationPreset, expected *int, by shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(preset.ID)
	if err != nil {
		return err
	}
	author, err := optionalUUID(by)
	if err != nil {
		return err
	}
	settings, err := presetSettingsOf(preset.Preset)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateInstallationPreset(ctx, sqlc.UpdateInstallationPresetParams{
		Name: preset.Name, Settings: settings, IsDefault: preset.IsDefault, Locks: lockStrings(preset.Locks),
		Now: timestampOf(at), UpdatedBy: author, ID: id, ExpectedVersion: versionOf(expected),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return presetNameTaken()
		}
		return presetFailed("changing an installation preset", err)
	}
	return installationConflictIfUntouched(affected, preset.ID, expected)
}

// ClearDefault makes every preset but one no default.
func (InstallationPresetRepository) ClearDefault(ctx context.Context, except, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	kept, err := optionalUUID(except)
	if err != nil {
		return err
	}
	author, err := optionalUUID(by)
	if err != nil {
		return err
	}
	if err := queries.ClearInstallationDefault(ctx, sqlc.ClearInstallationDefaultParams{
		Now: timestampOf(at), UpdatedBy: author, ExceptID: kept,
	}); err != nil {
		return presetFailed("lifting the installation's default", err)
	}
	return nil
}

// Delete removes a preset.
func (InstallationPresetRepository) Delete(ctx context.Context, id shared.ID, expected *int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	rowID, err := uuidOf(id)
	if err != nil {
		return presetNotFound(id)
	}
	affected, err := queries.DeleteInstallationPreset(ctx, sqlc.DeleteInstallationPresetParams{
		ID: rowID, ExpectedVersion: versionOf(expected),
	})
	if err != nil {
		return presetFailed("withdrawing an installation preset", err)
	}
	return installationConflictIfUntouched(affected, id, expected)
}

// installationConflictIfUntouched answers why nothing was written: no version was expected, so
// the preset is gone; or it moved on.
func installationConflictIfUntouched(affected int64, id shared.ID, expected *int) error {
	if affected != 0 {
		return nil
	}
	if expected == nil {
		return presetNotFound(id)
	}
	return presetConflictIfUntouched(affected, id, *expected)
}

func versionOf(expected *int) *int32 {
	if expected == nil {
		return nil
	}
	version := int32(*expected) //nolint:gosec // a row version is far below 2^31
	return &version
}

func lockStrings(locks []work.LockKey) []string {
	out := make([]string, 0, len(locks))
	for _, key := range locks {
		out = append(out, string(key))
	}
	return out
}

func installationPresetFrom(
	rowID pgtype.UUID, name string, settings []byte, isDefault bool, locks []string, version int32,
) (work.InstallationPreset, error) {
	id, err := idFrom(rowID)
	if err != nil {
		return work.InstallationPreset{}, err
	}
	var document structureSettingsDocument
	if err := json.Unmarshal(settings, &document); err != nil {
		return work.InstallationPreset{}, shared.ErrInternal.WithDetail("presets.undecodable").
			WithCause(fmt.Errorf("decoding the installation preset %s: %w", id, err))
	}
	keys := make([]work.LockKey, 0, len(locks))
	for _, key := range locks {
		keys = append(keys, work.LockKey(key))
	}
	return work.InstallationPreset{
		Preset: work.Preset{
			Key: work.SavedPresetKey(id), Origin: work.PresetOriginInstallation, ID: id, Name: name,
			Values: valuesFrom(&document), Version: int(version),
		},
		IsDefault: isDefault, Locks: keys,
	}, nil
}
