// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// TemplatePageRepository stores the marks of the pages a template brought (migration 0316).
type TemplatePageRepository struct{}

var _ repository.TemplatePages = TemplatePageRepository{}

// Mark marks the page; a key the hub marks already writes nothing.
func (TemplatePageRepository) Mark(ctx context.Context, hub, page shared.ID, key string, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	hubKey, err := uuidOf(hub)
	if err != nil {
		return false, err
	}
	pageKey, err := uuidOf(page)
	if err != nil {
		return false, err
	}
	n, err := queries.InsertTemplatePage(ctx, sqlc.InsertTemplatePageParams{
		PageID: pageKey, HubID: hubKey, Key: key, CreatedAt: timestampOf(at),
	})
	if err != nil {
		return false, sampleFailed("marking a template's page", err)
	}
	return n == 1, nil
}

// List answers the hub's marked pages not in the trash.
func (TemplatePageRepository) List(ctx context.Context, hub shared.ID) ([]repository.TemplatePage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hubKey, err := uuidOf(hub)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListTemplatePages(ctx, hubKey)
	if err != nil {
		return nil, sampleFailed("listing a template's pages", err)
	}
	out := make([]repository.TemplatePage, 0, len(rows))
	for _, row := range rows {
		page, err := idFrom(row.PageID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.TemplatePage{Key: row.Key, PageID: page, Title: row.Title, Icon: row.Icon})
	}
	return out, nil
}
