// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// FlowGate refuses an entry entering a column past the board's first without its required custom
// fields, where the collection's structure applies REQUIRED_BEYOND_FIRST_COLUMN (UC-WRK-31 check
// 2). Every writer that places an entry in a column asks it - an edit of `bucket_id`, a move, a
// creation - so REST, MCP, an automation action and a device's push meet the same refusal, from
// the one place that decides it.
//
// Asked inside the writer's transaction with the entry as it would be written, values included, so
// a creation that carries its fields passes and one that does not is refused whole.
type FlowGate struct {
	Buckets repository.Buckets
	Fields  repository.CustomFields
}

// ensureMayEnter answers nil where the entry may sit in the column. No column, the first column, a
// collection that does not apply the behaviour and an entry whose type carries no custom fields
// are never refused: a field the collection hides is one nobody could set.
func (g FlowGate) ensureMayEnter(
	ctx context.Context, structures StructureResolver, collection domain.Container, item domain.WorkItem,
	bucketID shared.ID,
) error {
	if bucketID.IsZero() || !structures.Wired() {
		// A resolver not wired answers Hubtask's default, which applies nothing by itself.
		return nil
	}
	effective, err := structures.within(repository.LayerRead).Of(ctx, collection)
	if err != nil {
		return err
	}
	if !slices.Contains(effective.Values.Automation, domain.AutomationRequiredBeyondFirstColumn) ||
		!slices.Contains(effective.Values.Fields[item.Type].Capabilities, domain.CapabilityCustomFields) {
		return nil
	}
	if g.Buckets == nil || g.Fields == nil {
		// The collection asks for the gate and it cannot be asked: refused rather than let through.
		return shared.ErrInternal.WithDetail("items.flow_gate_unwired")
	}
	board, err := g.Buckets.List(ctx, collection.ID)
	if err != nil {
		return err
	}
	if domain.FirstColumn(board, bucketID) {
		return nil
	}
	definitions, err := g.Fields.ListInScope(ctx, collection.ID)
	if err != nil {
		return err
	}
	if missing := domain.MissingRequiredFields(definitions, item); len(missing) > 0 {
		return domain.RequiredFieldsMissing(missing)
	}
	return nil
}
