// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The manifest carries the help links for Help to show, and a workspace with only those set still
// answers a legal object rather than none.
func TestTheManifestCarriesTheHelpLinks(t *testing.T) {
	answer := legalLinksResponse(identity.LegalLinks{SupportURL: "mailto:help@acme.example", DocsURL: "https://acme.example/docs"})
	if answer == nil || answer.SupportUrl == nil || *answer.SupportUrl != "mailto:help@acme.example" ||
		answer.DocsUrl == nil || *answer.DocsUrl != "https://acme.example/docs" {
		t.Fatalf("the manifest answered %+v", answer)
	}
	if answer.ImprintUrl != nil {
		t.Errorf("an imprint was invented: %q", *answer.ImprintUrl)
	}
}
