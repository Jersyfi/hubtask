// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"

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
// `acceptance`'s reason: what this package can read is exactly what it can name.
var readers = map[domain.TargetType]string{
	domain.TargetWorkItem: "GetWorkItem",
	// One entry by its identifier, whatever its state. A page of the listing searched for it found
	// nothing behind fifty newer arrivals.
	domain.TargetJumbleEntry: "GetJumbleEntry",
	// A collection, for the summary of how it stands.
	domain.TargetContainer: "GetContainer",
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
	if targetType == domain.TargetAccount {
		// A draft for the person who asked: theirs alone, and nothing to read - the digest is the
		// account, and a draft is never stale, since the save checks everything again. The scope
		// its kind is read under was asked by the caller, who knows the kind.
		if err := ownAccount(actor, targetID); err != nil {
			return nil, err
		}
		return domain.Digest(targetID.String(), ""), nil
	}
	name, served := readers[targetType]
	if !served {
		// A target kind this build does not produce suggestions for. Not-found rather than an
		// internal error: from a caller's side there is no such suggestion to answer, which is
		// exactly true.
		return nil, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}

	if targetType == domain.TargetJumbleEntry {
		return t.entryDigest(ctx, actor, name, targetID)
	}
	if targetType == domain.TargetContainer {
		return t.containerDigest(ctx, actor, name, targetID)
	}

	out, err := t.Catalogue.Invoke(ctx, name, actor, usecase.Input{
		"item_id": targetID.String(),
	})
	if err != nil {
		return nil, err
	}
	return domain.Digest(out.String("title"), out.String("notes")), nil
}

// containerDigest fingerprints a collection: its name, and nothing else.
//
// Deliberately *not* the entries it holds, although a summary of how a collection stands is made
// from them. A fingerprint over the contents would go stale the moment anybody ticked anything off
// - which is a collection working normally - and a stale suggestion cannot even be dismissed, so a
// summary would become unanswerable within minutes of being written. What staleness protects
// against is a proposal applied to a target it was not made from, and the target here is the
// collection.
func (t EntryTargets) containerDigest(
	ctx context.Context, actor appshared.ActorContext, name string, containerID shared.ID,
) ([]byte, error) {
	out, err := t.Catalogue.Invoke(ctx, name, actor, usecase.Input{
		"container_id": containerID.String(),
	})
	if err != nil {
		return nil, err
	}
	return domain.Digest(out.String("name"), ""), nil
}

// entryDigest fingerprints one jumble entry: its subject and its body, which is what a proposal
// about it is made from. What changes about an entry between the asking and the accepting is its
// *settlement*, not its text, so a settled entry is refused by the conversion rather than by the
// fingerprint - and the read answers an entry in every state, so that settling it does not hide
// the record of what AI had proposed.
//
// Settled, gone, or in a workspace the actor cannot see answer one not-found, which is what the
// inbox itself would say.
func (t EntryTargets) entryDigest(
	ctx context.Context, actor appshared.ActorContext, name string, entryID shared.ID,
) ([]byte, error) {
	out, err := t.Catalogue.Invoke(ctx, name, actor, usecase.Input{"entry_id": entryID.String()})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound.WithDetail("suggestions.not_found")
		}
		return nil, err
	}
	return domain.Digest(out.String("raw_subject"), out.String("raw_body")), nil
}
