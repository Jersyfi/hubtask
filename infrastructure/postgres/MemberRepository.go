// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ItemMemberRepository stores which accounts an entry carries, and the tags that decide it after a
// merge.
//
// Two tables behind one type, exactly as ItemLabelRepository holds them. `item_member` is the
// membership every read goes through, and `set_element` is the OR-set tag that survives a merge
// (offline-sync.md §4.2). They are written together, in one method each, because writing one
// without the other is the failure: membership with no tag merges as last writer wins and loses a
// concurrent change, a tag with no membership is invisible to every read.
//
// The set the tags are written under is `members` rather than `labels`, which is the whole of the
// difference between this type and that one - the statements behind both are the same three, and
// `set_name` has been a parameter of them since B-09 for exactly this second caller.
type ItemMemberRepository struct{}

func NewItemMemberRepository() ItemMemberRepository { return ItemMemberRepository{} }

var _ repository.ItemMembers = ItemMemberRepository{}

// List returns the accounts an entry carries.
func (r ItemMemberRepository) List(ctx context.Context, itemID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListItemMembers(ctx, item)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the members of %s: %w", itemID, err))
	}

	members := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		members = append(members, id)
	}
	return members, nil
}

// Add puts an account on an entry and records the addition's tag.
func (r ItemMemberRepository) Add(
	ctx context.Context, itemID, accountID shared.ID, tag shared.HLC,
) error {
	queries, item, account, err := itemMemberWrite(ctx, itemID, accountID)
	if err != nil {
		return err
	}

	if err := queries.AddItemMember(ctx, sqlc.AddItemMemberParams{
		ItemID:    item,
		AccountID: account,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("adding a member to %s: %w", itemID, err))
	}

	// The tag second, in the same transaction: a membership without one merges as last writer wins
	// over the whole set, which is the loss the OR-set exists to prevent.
	if err := queries.RecordSetElementAdded(ctx, sqlc.RecordSetElementAddedParams{
		ItemID:    item,
		SetName:   string(work.SetMembers),
		ElementID: account,
		Tag:       optionalText(tag.String()),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the member tag of %s: %w", itemID, err))
	}
	return nil
}

// Remove takes an account off an entry, records the removal's tag, and reports whether the entry
// carried it at all.
func (r ItemMemberRepository) Remove(
	ctx context.Context, itemID, accountID shared.ID, tag shared.HLC,
) (bool, error) {
	queries, item, account, err := itemMemberWrite(ctx, itemID, accountID)
	if err != nil {
		return false, err
	}

	affected, err := queries.RemoveItemMember(ctx, sqlc.RemoveItemMemberParams{
		ItemID:    item,
		AccountID: account,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing a member from %s: %w", itemID, err))
	}

	// The tag is written whether or not a row was there. A device that removes somebody this
	// replica never saw added has still made a decision another replica has to merge against, and a
	// removal nobody recorded is one the re-add would silently win.
	if err := queries.RecordSetElementRemoved(ctx, sqlc.RecordSetElementRemovedParams{
		ItemID:    item,
		SetName:   string(work.SetMembers),
		ElementID: account,
		Tag:       optionalText(tag.String()),
	}); err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the member tag of %s: %w", itemID, err))
	}
	return affected != 0, nil
}

// Elements returns every tag of one entry's member set.
func (r ItemMemberRepository) Elements(
	ctx context.Context, itemID shared.ID,
) ([]work.SetElement, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListSetElements(ctx, sqlc.ListSetElementsParams{
		ItemID:  item,
		SetName: string(work.SetMembers),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the member tags of %s: %w", itemID, err))
	}

	elements := make([]work.SetElement, 0, len(rows))
	for _, row := range rows {
		elementID, err := idFrom(row.ElementID)
		if err != nil {
			return nil, err
		}
		added, err := tagFrom(row.AddTag)
		if err != nil {
			return nil, err
		}
		removed, err := tagFrom(row.RemoveTag)
		if err != nil {
			return nil, err
		}
		elements = append(elements, work.SetElement{
			ElementID: elementID, AddedAt: added, RemovedAt: removed,
		})
	}
	return elements, nil
}

// itemMemberWrite is the preamble both writes share.
func itemMemberWrite(
	ctx context.Context, itemID, accountID shared.ID,
) (*sqlc.Queries, pgtype.UUID, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	return queries, item, account, nil
}
