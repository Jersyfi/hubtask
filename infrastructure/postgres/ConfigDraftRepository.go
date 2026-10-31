// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ConfigDraftRepository stores the drafts of the configuration's shared blocks (migration 0308).
type ConfigDraftRepository struct{}

var _ repository.Drafts = ConfigDraftRepository{}

func draftFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Save writes the draft under the optimistic lock; false is a draft moved since expected.
func (ConfigDraftRepository) Save(ctx context.Context, draft repository.Draft, expected int) (repository.Draft, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Draft{}, false, err
	}
	by, err := uuidOf(draft.SavedBy)
	if err != nil {
		return repository.Draft{}, false, err
	}
	block, err := json.Marshal(draft.Block)
	if err != nil {
		return repository.Draft{}, false, shared.ErrValidation.WithDetail("config.document_invalid").WithCause(err)
	}
	row, err := queries.SaveConfigDraft(ctx, sqlc.SaveConfigDraftParams{
		Section: draft.Section, Key: draft.Key, Block: block, SavedBy: by, SavedAt: timestampOf(draft.SavedAt),
		ExpectedVersion: int32(expected), //nolint:gosec // a version is far below 2³¹
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Draft{}, false, nil
	}
	if err != nil {
		return repository.Draft{}, false, draftFailed("saving a configuration draft", err)
	}
	saved, err := draftFrom(row.Section, row.Key, row.Block, row.SavedBy, row.SavedAt, row.Version)
	return saved, err == nil, err
}

// List answers the workspace's drafts by section and key.
func (ConfigDraftRepository) List(ctx context.Context) ([]repository.Draft, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListConfigDrafts(ctx)
	if err != nil {
		return nil, draftFailed("listing configuration drafts", err)
	}
	out := make([]repository.Draft, 0, len(rows))
	for _, row := range rows {
		draft, err := draftFrom(row.Section, row.Key, row.Block, row.SavedBy, row.SavedAt, row.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, draft)
	}
	return out, nil
}

// Discard removes the draft; false where there was none.
func (ConfigDraftRepository) Discard(ctx context.Context, section, key string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	n, err := queries.DiscardConfigDraft(ctx, sqlc.DiscardConfigDraftParams{Section: section, Key: key})
	if err != nil {
		return false, draftFailed("discarding a configuration draft", err)
	}
	return n == 1, nil
}

// Published removes the draft where it holds exactly the block applied.
func (ConfigDraftRepository) Published(ctx context.Context, section, key string, block domain.Block) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(block)
	if err != nil {
		return shared.ErrValidation.WithDetail("config.document_invalid").WithCause(err)
	}
	if err := queries.PublishedConfigDraft(ctx, sqlc.PublishedConfigDraftParams{Section: section, Key: key, Block: encoded}); err != nil {
		return draftFailed("removing a published configuration draft", err)
	}
	return nil
}

func draftFrom(section, key string, raw []byte, savedBy pgtype.UUID, savedAt pgtype.Timestamptz, version int32) (repository.Draft, error) {
	var block domain.Block
	if err := json.Unmarshal(raw, &block); err != nil {
		return repository.Draft{}, draftFailed("reading a configuration draft", err)
	}
	by, err := idFrom(savedBy)
	if err != nil {
		return repository.Draft{}, err
	}
	return repository.Draft{Section: section, Key: key, Block: block, SavedBy: by, SavedAt: savedAt.Time, Version: int(version)}, nil
}
