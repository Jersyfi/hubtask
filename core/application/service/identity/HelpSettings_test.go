// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A workspace sets its own support contact and documentation, read from the same flat write as
// the four links, unless the installation locked them (user-experience.md §5.8).
func TestAWorkspaceSetsItsHelpLinksUnlessTheInstallationLockedThem(t *testing.T) {
	change, err := policyPatchFrom(map[string]any{
		"support_url": "mailto:help@acme.example", "docs_url": "https://acme.example/docs",
	})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	links, moved, err := WorkspaceWriter{}.applyLegal(legalResolved(domain.LegalLayer{}, domain.LegalLinks{}), domain.LegalLinks{}, change.Legal)
	if err != nil {
		t.Fatalf("an open help link was refused: %v", err)
	}
	if links.SupportURL != "mailto:help@acme.example" || links.DocsURL != "https://acme.example/docs" || len(moved) != 2 {
		t.Errorf("the workspace holds %+v, moved %v", links, moved)
	}

	locked := domain.LegalLayer{
		Links: domain.LegalLinks{SupportURL: "https://host.example/help"},
		Locks: map[domain.LegalLink]bool{domain.LinkSupport: true},
	}
	_, _, err = WorkspaceWriter{}.applyLegal(legalResolved(locked, domain.LegalLinks{}), domain.LegalLinks{}, change.Legal)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a locked support contact was overwritten: %v", err)
	}
}
