// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// MOVE (N-06, offline-sync.md §4.2's hierarchy row): where an entry sits. The payload names the
// destination - `parent_id` (null for the top level), and optionally `collection_id`, `bucket_id`
// and the `order_key` the device computed there - and the mutation's reading is the hierarchy's.
// Last writer wins on the parent's clock; a move the clock accepts but that would close a cycle
// is refused with `sync.cycle_detected`, because a subtree containing its own ancestor is not a
// state a merge can produce (SY-12).

// moveFields are what a move's payload may name, and what is recorded under the move's reading.
var moveFields = []string{work.FieldParentID, work.FieldCollectionID, work.FieldBucketID, work.FieldOrderKey}

// cycleCodes are the hierarchy's refusals of a move into its own subtree, answered in the
// contract's word.
var cycleCodes = map[string]bool{"items.parent_is_self": true, "items.parent_in_own_subtree": true}

// move applies a MOVE.
func (p PushChanges) move(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	if m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	if _, named := m.Payload[work.FieldParentID]; !named {
		return Result{}, shared.ErrValidation.
			WithDetail("sync.fields_required").
			WithFields(shared.FieldError{Path: "/payload/parent_id", Code: "sync.fields_required"})
	}
	reading, err := shared.ParseHLC(m.HLC)
	if err != nil {
		return Result{}, err
	}
	for field := range m.Payload {
		if !isMoveField(field) {
			return Result{}, shared.ErrValidation.
				WithDetail("sync.field_unknown").
				WithParams(map[string]string{"field": field}).
				WithFields(shared.FieldError{Path: "/payload/" + field, Code: "sync.field_unknown"})
		}
	}

	before, err := p.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	clocks, err := p.clocksOf(ctx, actor, m.ItemID)
	if err != nil {
		return Result{}, err
	}
	if stored, stamped := clocks[work.FieldParentID]; stamped && !reading.After(stored) {
		// The server's hierarchy is later. Nothing is applied and the device adopts the server's
		// placement, which the state carries.
		return Result{OpID: m.OpID, Result: domain.Merged, EntityID: m.ItemID, ServerState: before}, nil
	}

	in := usecase.Input{"item_id": m.ItemID.String()}
	readings := map[string]shared.HLC{}
	for _, field := range moveFields {
		value, named := m.Payload[field]
		if !named {
			continue
		}
		readings[field] = reading
		switch field {
		case work.FieldParentID:
			// Present with an empty value is the top level; the use case reads presence, not
			// emptiness (MoveWorkItemCommand.ParentGiven).
			in["target_parent_id"] = stringOf(value)
		case work.FieldCollectionID:
			in["target_collection_id"] = stringOf(value)
		case work.FieldBucketID:
			in["target_bucket_id"] = stringOf(value)
		case work.FieldOrderKey:
			in["order_key"] = stringOf(value)
		}
	}

	applied := appshared.ContextWithReadings(ctx, readings)
	if _, err := p.Catalogue.Invoke(applied, "MoveWorkItem", actor, in); err != nil {
		if cycleCodes[shared.AsError(err).DetailCode] {
			return Result{}, shared.ErrConflict.
				WithDetail("sync.cycle_detected").
				WithFields(shared.FieldError{Path: "/payload/parent_id", Code: "sync.cycle_detected"})
		}
		return Result{}, err
	}

	after, err := p.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	result := domain.Merged
	if m.BaseVersion == nil || versionOf(before) == *m.BaseVersion {
		result = domain.Applied
	}
	return Result{OpID: m.OpID, Result: result, EntityID: m.ItemID, ServerState: after}, nil
}

func isMoveField(field string) bool {
	for _, known := range moveFields {
		if known == field {
			return true
		}
	}
	return false
}
