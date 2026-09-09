// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// EntryTargets answers the fingerprint of what a suggestion was made from.
//
// It reads the target through the **ordinary read use case**, which is the same decision acceptance
// makes on the write side and it buys the same thing: the permission check is the entry's own,
// asked once, in the place every other read asks it (rule 2, ADR-0005). This package therefore has
// exactly two ways to touch a workspace - `GetWorkItem` and whatever an acceptance names - and both
// of them are ordinary calls by the actor who made the request.
//
// Reading the target is also the visibility check. A suggestion about an entry should be exactly as
// readable as the entry, and asking the entry is how that stays one rule instead of two that drift:
// an entry somebody may not see answers not-found, and so does a suggestion about it (T-04's
// reasoning applied to a proposal).
type EntryTargets struct {
	Catalogue Catalogue
}

var _ Targets = EntryTargets{}

// The use cases a target is read through, one per target kind this build serves. A table for
// `appliers`' reason: what this package can read is exactly what it can name.
var readers = map[domain.TargetType]string{
	domain.TargetWorkItem: "GetWorkItem",
	// There is no GetJumbleEntry: the inbox is read as a list (G-10). The list is filtered to the
	// one entry here rather than a read being added to the contract for this, because a use case
	// existing only so that another package can fingerprint something is a use case nobody asked
	// for - and the list already carries the permission check this needs.
	domain.TargetJumbleEntry: "ListJumbleEntries",
}

// Digest answers the fingerprint of the target's current state.
//
// For a work item that is its title and its notes, and nothing else. A FIELDS suggestion proposes
// values for those, so those are what "the state it was made from" means: a due date changing
// underneath does not make a proposed title wrong, and fingerprinting every field would expire
// suggestions for reasons nobody would recognise.
func (t EntryTargets) Digest(
	ctx context.Context, actor appshared.ActorContext,
	targetType domain.TargetType, targetID shared.ID,
) ([]byte, error) {
	name, served := readers[targetType]
	if !served {
		// A target kind this build does not produce suggestions for yet - J-06 adds the jumble
		// entry's. Not-found rather than an internal error: from a caller's side there is no such
		// suggestion to answer, which is exactly true.
		return nil, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}

	if targetType == domain.TargetJumbleEntry {
		return t.entryDigest(ctx, actor, name, targetID)
	}

	out, err := t.Catalogue.Invoke(ctx, name, actor, usecase.Input{
		"item_id": targetID.String(),
	})
	if err != nil {
		return nil, err
	}
	return domain.Digest(out.String("title"), out.String("notes")), nil
}

// entryDigest fingerprints one jumble entry, found in the inbox listing.
//
// The subject and the body, which is what a proposal about an entry is made from - and what
// changes about an entry between the asking and the accepting is its *settlement*, not its text,
// so a settled entry is refused by the conversion rather than by the fingerprint.
func (t EntryTargets) entryDigest(
	ctx context.Context, actor appshared.ActorContext, name string, entryID shared.ID,
) ([]byte, error) {
	out, err := t.Catalogue.Invoke(ctx, name, actor, usecase.Input{"status": "NEW"})
	if err != nil {
		return nil, err
	}
	entries, _ := out["items"].([]usecase.Output)
	for _, entry := range entries {
		if entry.String("id") != entryID.String() {
			continue
		}
		return domain.Digest(entry.String("raw_subject"), entry.String("raw_body")), nil
	}
	// Settled, gone, or in a workspace the actor cannot see - one answer for all three, which is
	// what the inbox itself would say.
	return nil, shared.ErrNotFound.WithDetail("suggestions.not_found")
}
