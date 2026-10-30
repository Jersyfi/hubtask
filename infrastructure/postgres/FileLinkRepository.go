// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// FileLinkRepository stores an entry's files in a store (migration 0276) through the unit of
// work's transaction (rule 3).
type FileLinkRepository struct{}

// NewFileLinkRepository answers the store.
func NewFileLinkRepository() FileLinkRepository { return FileLinkRepository{} }

var _ repository.Store = FileLinkRepository{}

func fileLinkFailed(what string, err error) error {
	return fmt.Errorf("postgres: file link: %s: %w", what, err)
}

func fileLinkOf(r sqlc.ListFileLinksRow) filelink.Link {
	var k cmdbIDs
	return filelink.Link{
		ID: k.of(r.ID), ItemID: k.of(r.ItemID), Provider: filelink.Provider(r.Provider), Address: r.Address,
		Name: r.Name, AddedBy: k.of(r.AddedBy), AddedAt: timeFrom(r.AddedAt),
	}
}

func (FileLinkRepository) List(ctx context.Context, itemID shared.ID) ([]filelink.Link, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFileLinks(ctx, item)
	if err != nil {
		return nil, fileLinkFailed("listing", err)
	}
	out := make([]filelink.Link, 0, len(rows))
	for _, r := range rows {
		out = append(out, fileLinkOf(r))
	}
	return out, nil
}

func (FileLinkRepository) Count(ctx context.Context, itemID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountFileLinks(ctx, item)
	if err != nil {
		return 0, fileLinkFailed("counting", err)
	}
	return int(n), nil
}

func (FileLinkRepository) Find(ctx context.Context, id shared.ID) (filelink.Link, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return filelink.Link{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return filelink.Link{}, shared.ErrNotFound.WithDetail("filelink.not_found")
	}
	r, err := q.FindFileLink(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return filelink.Link{}, shared.ErrNotFound.WithDetail("filelink.not_found")
	}
	if err != nil {
		return filelink.Link{}, fileLinkFailed("reading", err)
	}
	return fileLinkOf(sqlc.ListFileLinksRow(r)), nil
}

func (FileLinkRepository) Add(ctx context.Context, l filelink.Link) (bool, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := uuidsOf([]shared.ID{l.ID, l.ItemID, l.AddedBy})
	if err != nil {
		return false, err
	}
	n, err := q.InsertFileLink(ctx, sqlc.InsertFileLinkParams{
		ID: ids[0], ItemID: ids[1], Provider: string(l.Provider), Address: l.Address, Name: l.Name,
		AddedBy: ids[2], AddedAt: timestampOf(l.AddedAt),
	})
	if err != nil {
		return false, fileLinkFailed("writing", err)
	}
	return n == 1, nil
}

func (FileLinkRepository) Remove(ctx context.Context, id shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return shared.ErrNotFound.WithDetail("filelink.not_found")
	}
	n, err := q.DeleteFileLink(ctx, key)
	if err != nil {
		return fileLinkFailed("removing", err)
	}
	if n == 0 {
		return shared.ErrNotFound.WithDetail("filelink.not_found")
	}
	return nil
}
