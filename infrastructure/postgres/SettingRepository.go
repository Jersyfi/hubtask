// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SettingRepository is the settings catalogue's storage shape (migration 0208) and the two tables
// above it in the same shape: instance_setting and plan_setting.
type SettingRepository struct{}

var _ settingrepo.Settings = SettingRepository{}

// Above answers the installation's and the workspace's plan's entries under a key prefix.
func (SettingRepository) Above(ctx context.Context, prefix string) (domain.Layer, domain.Layer, error) {
	instance := domain.Layer{Level: domain.LevelInstance, Entries: map[domain.Key]domain.Entry{}}
	plan := domain.Layer{Level: domain.LevelPlan, Entries: map[domain.Key]domain.Entry{}}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return instance, plan, err
	}
	installed, err := queries.InstanceSettingsUnder(ctx, prefix)
	if err != nil {
		return instance, plan, queryFailed("reading the installation's settings", err)
	}
	for _, row := range installed {
		value, err := settingValueOf(row.Key, row.Value)
		if err != nil {
			return instance, plan, err
		}
		hold := domain.HoldSet
		if row.LockOrigin != "OPEN" {
			hold = domain.HoldLock
		}
		instance.Entries[domain.Key(row.Key)] = domain.Entry{Value: value, Hold: hold}
	}
	planned, err := queries.PlanSettingsOfWorkspaceUnder(ctx, prefix)
	if err != nil {
		return instance, plan, queryFailed("reading the plan's settings", err)
	}
	for _, row := range planned {
		value, err := settingValueOf(row.Key, row.Value)
		if err != nil {
			return instance, plan, err
		}
		hold := domain.HoldSet
		if row.Locked {
			hold = domain.HoldLock
		}
		plan.Entries[domain.Key(row.Key)] = domain.Entry{Value: value, Hold: hold}
	}
	return instance, plan, nil
}

// At answers the entries under a prefix stored at the places.
func (SettingRepository) At(ctx context.Context, places []settingrepo.Place, prefix string) ([]settingrepo.Stored, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	params := sqlc.ListSettingValuesParams{Prefix: prefix}
	for _, place := range places {
		scope, err := uuidOf(place.ScopeID)
		if err != nil {
			return nil, err
		}
		params.Levels = append(params.Levels, string(place.Level))
		params.Scopes = append(params.Scopes, scope)
	}
	rows, err := queries.ListSettingValues(ctx, params)
	if err != nil {
		return nil, queryFailed("reading settings", err)
	}
	stored := make([]settingrepo.Stored, 0, len(rows))
	for _, row := range rows {
		scope, err := idFrom(row.ScopeID)
		if err != nil {
			return nil, err
		}
		by, err := optionalID(row.SetBy)
		if err != nil {
			return nil, err
		}
		value, err := settingValueOf(row.Key, row.Value)
		if err != nil {
			return nil, err
		}
		allowed, err := allowedOf(row.Key, row.Allowed)
		if err != nil {
			return nil, err
		}
		stored = append(stored, settingrepo.Stored{
			Place: settingrepo.Place{Level: domain.Level(row.Level), ScopeID: scope},
			Key:   domain.Key(row.Key),
			Entry: domain.Entry{Value: value, Hold: domain.Hold(row.Hold), Allowed: allowed, By: by},
			UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return stored, nil
}

// Put stores one entry.
func (SettingRepository) Put(
	ctx context.Context, id shared.ID, place settingrepo.Place, key domain.Key, entry domain.Entry, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	rowID, err := uuidOf(id)
	if err != nil {
		return err
	}
	scope, err := uuidOf(place.ScopeID)
	if err != nil {
		return err
	}
	by, err := optionalUUID(entry.By)
	if err != nil {
		return err
	}
	params := sqlc.UpsertSettingValueParams{
		ID: rowID, Level: string(place.Level), ScopeID: scope, Key: string(key),
		Hold: string(entry.Hold), SetBy: by, UpdatedAt: timestampOf(at),
	}
	if entry.Value != nil {
		if params.Value, err = json.Marshal(entry.Value); err != nil {
			return fmt.Errorf("encoding the setting %s: %w", key, err)
		}
	}
	if entry.Hold == domain.HoldRestrict {
		if params.Allowed, err = json.Marshal(allowedDocument{
			Minimum: entry.Allowed.Minimum, Maximum: entry.Allowed.Maximum, Options: entry.Allowed.Options,
		}); err != nil {
			return fmt.Errorf("encoding the setting %s: %w", key, err)
		}
	}
	if err := queries.UpsertSettingValue(ctx, params); err != nil {
		return queryFailed("storing a setting", err)
	}
	return nil
}

// Remove deletes one entry.
func (SettingRepository) Remove(ctx context.Context, place settingrepo.Place, key domain.Key) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	scope, err := uuidOf(place.ScopeID)
	if err != nil {
		return false, err
	}
	removed, err := queries.DeleteSettingValue(ctx, sqlc.DeleteSettingValueParams{
		Level: string(place.Level), ScopeID: scope, Key: string(key),
	})
	if err != nil {
		return false, queryFailed("removing a setting", err)
	}
	return removed > 0, nil
}

type allowedDocument struct {
	Minimum *int     `json:"minimum,omitempty"`
	Maximum *int     `json:"maximum,omitempty"`
	Options []string `json:"options,omitempty"`
}

// settingValueOf decodes a stored value into the shapes the catalogue carries: a whole number as
// an int, a list of words as []string, a flag and a word as themselves.
func settingValueOf(key string, raw []byte) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, malformedSetting(key, err)
	}
	switch value := decoded.(type) {
	case float64:
		if value == float64(int(value)) {
			return int(value), nil
		}
		return value, nil
	case []any:
		words := make([]string, 0, len(value))
		for _, each := range value {
			word, isWord := each.(string)
			if !isWord {
				return decoded, nil
			}
			words = append(words, word)
		}
		return words, nil
	}
	return decoded, nil
}

func allowedOf(key string, raw []byte) (domain.Allowed, error) {
	if len(raw) == 0 {
		return domain.Allowed{}, nil
	}
	var document allowedDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.Allowed{}, malformedSetting(key, err)
	}
	return domain.Allowed{Minimum: document.Minimum, Maximum: document.Maximum, Options: document.Options}, nil
}
