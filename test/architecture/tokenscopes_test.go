// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"slices"
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/meta"
)

// The manifest may not offer a scope the mint would refuse.
//
// Both read `catalogue.Scopes()`, and the whole value of that is that they cannot come apart -
// which is only true while both keep reading it. This is the test that notices if one of them
// grows a list of its own: `token_scopes` exists so that a client stops compiling one in, and a
// manifest that advertised a scope `CreateAccessToken` rejects would be worse than no field at
// all, because the client would then offer a control the server refuses by name.

func TestTheManifestOffersExactlyTheScopesTheMintAccepts(t *testing.T) {
	t.Parallel()

	declared := usecases.Scopes()
	if len(declared) == 0 {
		t.Fatal("the catalogue declares no scopes at all, which means the reading is broken")
	}

	// What the manifest answers: the same slice, through the field the composition root fills.
	manifest := meta.Capabilities{TokenScopes: declared}
	if !slices.Equal(manifest.TokenScopes, declared) {
		t.Errorf("the manifest answers %v, want %v", manifest.TokenScopes, declared)
	}

	// What the mint accepts: every one of them, and nothing outside them.
	writer := identity.AccessTokenWriter{KnownScopes: declared}
	for _, scope := range declared {
		if !slices.Contains(writer.KnownScopes, scope) {
			t.Errorf("the manifest offers %q and the mint does not know it", scope)
		}
	}
}

func TestTheScopesAreSortedAndDistinct(t *testing.T) {
	t.Parallel()

	// Sorted because a client renders them in the order they arrive, and an order that changed
	// with the descriptor registration order would reshuffle a list of checkboxes between builds.
	// Distinct because two identical entries are two checkboxes for one thing.
	declared := usecases.Scopes()
	if !slices.IsSorted(declared) {
		t.Errorf("the scopes are not sorted: %v", declared)
	}
	seen := map[string]bool{}
	for _, scope := range declared {
		if scope == "" {
			t.Error("an empty scope is declared")
		}
		if seen[scope] {
			t.Errorf("%q is declared twice", scope)
		}
		seen[scope] = true
	}
}
