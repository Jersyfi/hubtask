// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// ensureAmountOffered refuses an amount where the collection's structure in force does not carry
// AMOUNT for the entry's type (domain-model.md §2): the capability is opt-in, so the installation's
// profile offering it says nothing about this collection. The refusal is the profile's own, so a
// client reads the same `capability_not_supported` with the same parameters whichever gate spoke.
//
// Read inside the caller's transaction, with no hold: a structure written meanwhile is a change of
// what the collection offers, decided by its own writer, and this write sees one state or the other.
// A resolver not wired answers no structure, and no structure offers no amount - the gate fails
// closed rather than letting a value through it cannot judge.
func ensureAmountOffered(
	ctx context.Context, structures StructureResolver, collection domain.Container, itemType domain.ItemType,
) error {
	carried := []domain.Capability{}
	if structures.Wired() {
		effective, err := structures.within(repository.LayerRead).Of(ctx, collection)
		if err != nil {
			return err
		}
		carried = effective.Values.Fields[itemType].Capabilities
	}
	return domain.CapabilityProfile{Type: itemType, Capabilities: carried}.
		Require(domain.CapabilityAmount, "/amount")
}
