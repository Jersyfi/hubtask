// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CodeForgeRepository stores a hub's forge hooks and an entry's code links (migration 0275)
// through the unit of work's transaction (rule 3).
type CodeForgeRepository struct{}

// NewCodeForgeRepository answers the store.
func NewCodeForgeRepository() CodeForgeRepository { return CodeForgeRepository{} }

var _ repository.Store = CodeForgeRepository{}

func forgeFailed(what string, err error) error {
	return fmt.Errorf("postgres: code forge: %s: %w", what, err)
}

func hookOf(r sqlc.ListCodeHooksRow) forge.Hook {
	var k cmdbIDs
	return forge.Hook{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Kind: forge.Kind(r.Kind), Name: r.Name, Enabled: r.Enabled,
		Salt: r.Salt, RunAs: k.of(r.RunAs), CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt),
		LastDeliveryAt: optionalTime(r.LastDeliveryAt),
	}
}

func linkOf(r sqlc.ListCodeLinksRow) forge.Link {
	var k cmdbIDs
	return forge.Link{
		ID: k.of(r.ID), ItemID: k.of(r.ItemID), HubID: k.of(r.HubID), HookID: k.of(r.HookID),
		Kind: forge.LinkKind(r.Kind), Repository: r.Repository, Ref: r.Ref, Title: r.Title, URL: r.Url,
		Author: r.Author, State: forge.State(r.State), HappenedAt: timeFrom(r.HappenedAt), UpdatedAt: timeFrom(r.UpdatedAt),
	}
}

func (CodeForgeRepository) ListHooks(ctx context.Context, hubID shared.ID) ([]forge.Hook, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCodeHooks(ctx, hub)
	if err != nil {
		return nil, forgeFailed("listing hooks", err)
	}
	out := make([]forge.Hook, 0, len(rows))
	for _, r := range rows {
		out = append(out, hookOf(r))
	}
	return out, nil
}

func (CodeForgeRepository) CountHooks(ctx context.Context, hubID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountCodeHooks(ctx, hub)
	if err != nil {
		return 0, forgeFailed("counting hooks", err)
	}
	return int(n), nil
}

func (CodeForgeRepository) FindHook(ctx context.Context, id shared.ID) (forge.Hook, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return forge.Hook{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return forge.Hook{}, shared.ErrNotFound.WithDetail("forge.hook_not_found")
	}
	r, err := q.FindCodeHook(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return forge.Hook{}, shared.ErrNotFound.WithDetail("forge.hook_not_found")
	}
	if err != nil {
		return forge.Hook{}, forgeFailed("reading a hook", err)
	}
	return hookOf(sqlc.ListCodeHooksRow(r)), nil
}

func (CodeForgeRepository) PutHook(ctx context.Context, h forge.Hook) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := uuidsOf([]shared.ID{h.ID, h.HubID, h.RunAs})
	if err != nil {
		return err
	}
	err = q.PutCodeHook(ctx, sqlc.PutCodeHookParams{
		ID: ids[0], HubID: ids[1], Kind: string(h.Kind), Name: h.Name, Enabled: h.Enabled, Salt: h.Salt,
		RunAs: ids[2], CreatedAt: timestampOf(h.CreatedAt), UpdatedAt: timestampOf(h.UpdatedAt),
		LastDeliveryAt: optionalTimestamp(h.LastDeliveryAt),
	})
	if err != nil {
		return forgeFailed("writing a hook", err)
	}
	return nil
}

func (CodeForgeRepository) SeeDelivery(ctx context.Context, hookID shared.ID, digest []byte, at, before time.Time) (bool, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(hookID)
	if err != nil {
		return false, err
	}
	if err := q.ForgetCodeDeliveries(ctx, sqlc.ForgetCodeDeliveriesParams{HookID: id, Before: timestampOf(before)}); err != nil {
		return false, forgeFailed("forgetting deliveries", err)
	}
	n, err := q.SeeCodeDelivery(ctx, sqlc.SeeCodeDeliveryParams{HookID: id, Digest: digest, At: timestampOf(at)})
	if err != nil {
		return false, forgeFailed("remembering a delivery", err)
	}
	return n == 1, nil
}

func (CodeForgeRepository) FindLink(ctx context.Context, itemID, hookID shared.ID, kind forge.LinkKind, repo, ref string) (forge.Link, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return forge.Link{}, err
	}
	ids, err := uuidsOf([]shared.ID{itemID, hookID})
	if err != nil {
		return forge.Link{}, err
	}
	r, err := q.FindCodeLink(ctx, sqlc.FindCodeLinkParams{ItemID: ids[0], HookID: ids[1], Kind: string(kind), Repository: repo, Ref: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		return forge.Link{}, shared.ErrNotFound.WithDetail("forge.link_not_found")
	}
	if err != nil {
		return forge.Link{}, forgeFailed("reading a link", err)
	}
	return linkOf(sqlc.ListCodeLinksRow(r)), nil
}

func (CodeForgeRepository) PutLink(ctx context.Context, l forge.Link) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := uuidsOf([]shared.ID{l.ID, l.ItemID, l.HubID, l.HookID})
	if err != nil {
		return err
	}
	err = q.PutCodeLink(ctx, sqlc.PutCodeLinkParams{
		ID: ids[0], ItemID: ids[1], HubID: ids[2], HookID: ids[3], Kind: string(l.Kind), Repository: l.Repository,
		Ref: l.Ref, Title: l.Title, Url: l.URL, Author: l.Author, State: string(l.State),
		HappenedAt: timestampOf(l.HappenedAt), UpdatedAt: timestampOf(l.UpdatedAt),
	})
	if err != nil {
		return forgeFailed("writing a link", err)
	}
	return nil
}

func (CodeForgeRepository) ListLinks(ctx context.Context, itemID shared.ID, max int) ([]forge.Link, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCodeLinks(ctx, sqlc.ListCodeLinksParams{ItemID: item, MaxRows: int32(max)})
	if err != nil {
		return nil, forgeFailed("listing links", err)
	}
	out := make([]forge.Link, 0, len(rows))
	for _, r := range rows {
		out = append(out, linkOf(r))
	}
	return out, nil
}
