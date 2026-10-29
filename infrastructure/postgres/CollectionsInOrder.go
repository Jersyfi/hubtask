// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

var _ repository.Collections = ContainerRepository{}

// InOrder answers the collections across the hubs, in the tree's order.
//
// The cursor carries the whole sort - the hub's rank, the hub, the collection's rank - and the
// collection as its identifier, so a boundary between two hubs is as exact as one inside a hub.
func (r ContainerRepository) InOrder(
	ctx context.Context, query repository.CollectionQuery,
) (repository.CollectionPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.CollectionPage{}, err
	}

	params := sqlc.ListCollectionsInOrderParams{
		IncludePrivate:  query.IncludePrivate,
		IncludeArchived: query.IncludeArchived,
		PageSize:        pageProbe(query.Page.Size),
	}
	if query.Page.Cursor != "" {
		position, err := r.cursors.Decode(query.Page.Cursor)
		if err != nil {
			return repository.CollectionPage{}, err
		}
		if len(position.Keys) != 3 {
			return repository.CollectionPage{}, shared.ErrValidation.
				WithDetail("shared.cursor_invalid").
				WithFields(shared.FieldError{Path: "/cursor", Code: "shared.cursor_invalid"})
		}
		hubID, err := uuidOf(shared.ID(position.Keys[1]))
		if err != nil {
			return repository.CollectionPage{}, err
		}
		id, err := uuidOf(position.ID)
		if err != nil {
			return repository.CollectionPage{}, err
		}
		hubKey, key := position.Keys[0], position.Keys[2]
		params.CursorHubOrderKey, params.CursorHubID = &hubKey, hubID
		params.CursorOrderKey, params.CursorID = &key, id
	}

	rows, err := queries.ListCollectionsInOrder(ctx, params)
	if err != nil {
		return repository.CollectionPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the collections in order: %w", err))
	}

	type placed struct {
		collection repository.CollectionInHub
		hubKey     string
	}
	read := make([]placed, 0, len(rows))
	for _, row := range rows {
		collection, err := containerFrom(findRowOf(row))
		if err != nil {
			return repository.CollectionPage{}, err
		}
		read = append(read, placed{
			collection: repository.CollectionInHub{
				Collection: collection, HubName: row.HubName, HubPrivate: row.HubPrivate,
			},
			hubKey: row.HubOrderKey,
		})
	}

	kept, info := pageOf(read, query.Page.Size, r.cursors, func(last placed) security.Position {
		return security.Position{
			Keys: []string{last.hubKey, last.collection.Collection.ParentID.String(), last.collection.Collection.OrderKey},
			ID:   last.collection.Collection.ID,
		}
	})
	page := repository.CollectionPage{Collections: make([]repository.CollectionInHub, 0, len(kept)), Info: info}
	for _, row := range kept {
		page.Collections = append(page.Collections, row.collection)
	}
	return page, nil
}

// findRowOf is the container half of a row the ordered listing reads, in the shape every container
// read maps from.
func findRowOf(row sqlc.ListCollectionsInOrderRow) sqlc.FindContainerRow {
	return sqlc.FindContainerRow{
		ID: row.ID, TenantID: row.TenantID, Type: row.Type, ParentID: row.ParentID, Name: row.Name,
		Description: row.Description, Icon: row.Icon, ColorToken: row.ColorToken, OrderKey: row.OrderKey,
		CompletionPolicy: row.CompletionPolicy, AutoAssignStrategy: row.AutoAssignStrategy,
		AutoAssignCandidates: row.AutoAssignCandidates, AutoAssignEnabled: row.AutoAssignEnabled,
		ArchivedAt: row.ArchivedAt, ParentArchivedAt: row.ParentArchivedAt, DeletedAt: row.DeletedAt,
		TrashBatchID: row.TrashBatchID, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt, Version: row.Version, Private: row.Private,
	}
}
