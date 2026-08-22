// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The generated list is checked against the design system by CI, which regenerates it and fails on
// a diff (ADR-0029). What is worth asserting here is the other half: that the validator actually
// uses the list, and that it refuses everything else. A colour the domain accepts but the design
// system does not define is a label nothing can render.

func TestIsLabelTokenAcceptsEveryGeneratedToken(t *testing.T) {
	t.Parallel()

	if len(shared.LabelTokens) == 0 {
		t.Fatal("no label tokens were generated - has make tokens run?")
	}
	for _, token := range shared.LabelTokens {
		if !shared.IsLabelToken(string(token)) {
			t.Errorf("IsLabelToken(%q) = false, want true for a generated token", token)
		}
	}
}

func TestIsLabelTokenRefusesAnythingElse(t *testing.T) {
	t.Parallel()

	// A hex value is in the list because it is the mistake the token exists to prevent: a client
	// that sends `#8A2438` instead of a name must be refused, not stored.
	for _, name := range []string{"", "Teal", "TEAL", "teal ", "turquoise", "#8A2438", "slate;drop"} {
		if shared.IsLabelToken(name) {
			t.Errorf("IsLabelToken(%q) = true, want false", name)
		}
	}
}

func TestLabelTokensAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[shared.LabelToken]bool, len(shared.LabelTokens))
	for _, token := range shared.LabelTokens {
		if seen[token] {
			t.Errorf("the label token %q is listed twice", token)
		}
		seen[token] = true
	}
}
