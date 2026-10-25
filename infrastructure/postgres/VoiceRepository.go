// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/voice"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// VoiceRepository is a workspace's voice settings (ADR-0093).
type VoiceRepository struct{}

// NewVoiceRepository builds the repository.
func NewVoiceRepository() VoiceRepository { return VoiceRepository{} }

var _ repository.Settings = VoiceRepository{}

func (VoiceRepository) Find(ctx context.Context) (domain.Settings, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	var settings domain.Settings
	row, err := queries.FindVoiceSetting(ctx)
	switch {
	case IsNoRows(err):
	case err != nil:
		return domain.Settings{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the voice settings: %w", err))
	default:
		settings.Enabled = row.Enabled
		settings.SkillID = stringFrom(row.SkillID)
		if settings.DefaultCollectionID, err = optionalID(row.DefaultCollectionID); err != nil {
			return domain.Settings{}, err
		}
	}
	rows, err := queries.ListVoiceAliases(ctx)
	if err != nil {
		return domain.Settings{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the voice aliases: %w", err))
	}
	for _, each := range rows {
		alias := domain.Alias{Phrase: each.Phrase}
		if alias.ID, err = idFrom(each.ID); err != nil {
			return domain.Settings{}, err
		}
		if alias.CollectionID, err = idFrom(each.CollectionID); err != nil {
			return domain.Settings{}, err
		}
		if alias.LabelID, err = optionalID(each.LabelID); err != nil {
			return domain.Settings{}, err
		}
		settings.Aliases = append(settings.Aliases, alias)
	}
	return settings, nil
}

func (VoiceRepository) Save(
	ctx context.Context, settings domain.Settings, by shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	defaultID, err := optionalUUID(settings.DefaultCollectionID)
	if err != nil {
		return err
	}
	byID, err := optionalUUID(by)
	if err != nil {
		return err
	}
	if err := queries.UpsertVoiceSetting(ctx, sqlc.UpsertVoiceSettingParams{
		Enabled: settings.Enabled, DefaultCollectionID: defaultID,
		SkillID: optionalText(strings.TrimSpace(settings.SkillID)), UpdatedAt: timestampOf(at),
		UpdatedBy: byID,
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing the voice settings: %w", err))
	}
	if err := queries.DeleteVoiceAliases(ctx); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("replacing the voice aliases: %w", err))
	}
	for _, alias := range settings.Aliases {
		id, err := uuidOf(alias.ID)
		if err != nil {
			return err
		}
		collection, err := uuidOf(alias.CollectionID)
		if err != nil {
			return err
		}
		label, err := optionalUUID(alias.LabelID)
		if err != nil {
			return err
		}
		if err := queries.InsertVoiceAlias(ctx, sqlc.InsertVoiceAliasParams{
			ID: id, Phrase: strings.TrimSpace(alias.Phrase), Folded: domain.Fold(alias.Phrase),
			CollectionID: collection, LabelID: label, CreatedAt: timestampOf(at),
		}); err != nil {
			if isUniqueViolation(err) {
				return shared.ErrValidation.WithDetail("voice.alias_twice").
					WithFields(shared.FieldError{Path: "/aliases", Code: "voice.alias_twice"})
			}
			return shared.ErrUnavailable.WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("storing a voice alias: %w", err))
		}
	}
	return nil
}
