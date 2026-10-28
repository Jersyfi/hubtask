// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The writers' half of the gate: what a collection offers the entry about to be written.

// entryOffer is what a writer holds for one entry: what its collection offers, and the levels
// above it.
type entryOffer struct {
	offered   domain.Offered
	ancestors []domain.ItemType
	wired     bool
}

// requireLevel refuses a write on an entry of a hidden level, or under one.
func (o entryOffer) requireLevel(itemType domain.ItemType, path string) error {
	if !o.wired {
		return nil
	}
	return o.offered.RequireLevel(itemType, o.ancestors, path)
}

// narrow is the profile the writer gates its values with.
func (o entryOffer) narrow(profile domain.CapabilityProfile) domain.CapabilityProfile {
	if !o.wired {
		return profile
	}
	return o.offered.Narrow(profile)
}

// offeredIn resolves what a collection offers, inside the caller's transaction. The layers are read
// without a hold: a value written just before a narrowing commits is hidden a moment later, exactly
// like one written before it, so there is nothing a lock would protect.
func (g EntryGate) offeredIn(ctx context.Context, collection domain.Container) (domain.Offered, error) {
	reading := g.Structure.within(repository.LayerRead)
	bound, err := reading.Bound(ctx)
	if err != nil {
		return domain.Offered{}, err
	}
	effective, err := reading.Of(ctx, collection)
	if err != nil {
		return domain.Offered{}, err
	}
	return domain.NewOffered(bound, effective), nil
}

// forEntry is what the collection offers an existing entry.
func (g EntryGate) forEntry(
	ctx context.Context, collection domain.Container, item domain.WorkItem,
) (entryOffer, error) {
	if !g.Wired() {
		return entryOffer{}, nil
	}
	offered, err := g.offeredIn(ctx, collection)
	if err != nil {
		return entryOffer{}, err
	}
	ancestors, err := g.chains(ctx, offered, []domain.WorkItem{item})
	if err != nil {
		return entryOffer{}, err
	}
	return entryOffer{offered: offered, ancestors: ancestors[item.ID], wired: true}, nil
}

// gatedProfile is the profile in force for an entry's type, narrowed to what its collection offers,
// after refusing an entry whose level is hidden. Every value writer reads its profile here.
func (g EntryGate) gatedProfile(
	ctx context.Context, profiles metarepo.CapabilityProfiles, collection domain.Container,
	item domain.WorkItem,
) (domain.CapabilityProfile, error) {
	profile, err := profileOf(ctx, profiles, item.Type)
	if err != nil {
		return domain.CapabilityProfile{}, err
	}
	offer, err := g.forEntry(ctx, collection, item)
	if err != nil {
		return domain.CapabilityProfile{}, err
	}
	if err := offer.requireLevel(item.Type, ""); err != nil {
		return domain.CapabilityProfile{}, err
	}
	return narrowByEntryType(ctx, g.EntryTypes, item, offer.narrow(profile))
}

// chains answers the types above each entry, nearest first, in one statement for all of them. Not
// read at all where the collection hides no level, since no chain can then hide anything.
func (g EntryGate) chains(
	ctx context.Context, offered domain.Offered, items []domain.WorkItem,
) (map[shared.ID][]domain.ItemType, error) {
	chains := make(map[shared.ID][]domain.ItemType, len(items))
	if !offered.HidesALevel() {
		return chains, nil
	}
	return g.chainsOf(ctx, items)
}

// requireShown refuses a write on an entry whose level, or an ancestor's, its collection hides: a
// rename, completing or reopening, reordering - anything but the lifecycle.
func (g EntryGate) requireShown(ctx context.Context, collection domain.Container, item domain.WorkItem) error {
	offer, err := g.forEntry(ctx, collection, item)
	if err != nil {
		return err
	}
	return offer.requireLevel(item.Type, "")
}

// refuseHiddenRemoval refuses taking away a value the collection hides, as writing one is refused:
// hidden is kept exactly as it was, so that widening shows it again (domain-model.md §2).
func refuseHiddenRemoval(profile domain.CapabilityProfile, capability domain.Capability, path string) error {
	if !slices.Contains(profile.Hidden, capability) {
		return nil
	}
	return profile.Require(capability, path)
}

// gatedProfileFor is gatedProfile for a writer that holds the entry and not its collection: the
// collection is read in the same transaction.
func (g EntryGate) gatedProfileFor(
	ctx context.Context, profiles metarepo.CapabilityProfiles, item domain.WorkItem,
) (domain.CapabilityProfile, error) {
	if !g.Wired() {
		profile, err := profileOf(ctx, profiles, item.Type)
		if err != nil {
			return domain.CapabilityProfile{}, err
		}
		return narrowByEntryType(ctx, g.EntryTypes, item, profile)
	}
	collection, err := findCollection(ctx, g.Structure.Containers, item.CollectionID)
	if err != nil {
		return domain.CapabilityProfile{}, err
	}
	return g.gatedProfile(ctx, profiles, collection, item)
}

// placing is what the collection offers an entry about to be placed under parent - nil for the top
// of the collection: the levels above it are the parent's and the parent's own.
func (g EntryGate) placing(
	ctx context.Context, collection domain.Container, parent *domain.WorkItem,
) (entryOffer, error) {
	if !g.Wired() {
		return entryOffer{}, nil
	}
	offered, err := g.offeredIn(ctx, collection)
	if err != nil {
		return entryOffer{}, err
	}
	offer := entryOffer{offered: offered, wired: true}
	if parent == nil {
		return offer, nil
	}
	above, err := g.chains(ctx, offered, []domain.WorkItem{*parent})
	if err != nil {
		return entryOffer{}, err
	}
	offer.ancestors = append([]domain.ItemType{parent.Type}, above[parent.ID]...)
	return offer, nil
}
