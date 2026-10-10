// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// DeletedTemplateRepository reads and removes the templates a legal hold kept after their deletion
// (data-retention.md §4). Its own type rather than methods on TemplateRepository: the retention
// pass takes this half, and a port that carried both would let it write a template.
type DeletedTemplateRepository struct{}

var _ repository.DeletedTemplates = DeletedTemplateRepository{}

// Deleted returns one page of the deleted templates after the identifier given.
func (DeletedTemplateRepository) Deleted(
	ctx context.Context, after shared.ID, batch int,
) ([]repository.DeletedTemplate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := pageKey(after)
	if err != nil {
		return nil, err
	}

	rows, err := queries.DeletedTemplates(ctx, sqlc.DeletedTemplatesParams{
		After: key,
		//nolint:gosec // G115: a batch is the pass's page size, a few thousand at most
		Batch: int32(batch),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the deleted templates: %w", err))
	}

	deleted := make([]repository.DeletedTemplate, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		scope, err := optionalID(row.ScopeID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(row.HubID)
		if err != nil {
			return nil, err
		}
		deleted = append(deleted, repository.DeletedTemplate{ID: id, ScopeID: scope, HubID: hub})
	}
	return deleted, nil
}

// Remove deletes the named templates that are still deleted and answers which went.
func (DeletedTemplateRepository) Remove(ctx context.Context, ids []shared.ID) ([]shared.ID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return nil, err
	}

	rows, err := queries.RemoveDeletedTemplates(ctx, keys)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing deleted templates: %w", err))
	}
	removed := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		removed = append(removed, id)
	}
	return removed, nil
}
