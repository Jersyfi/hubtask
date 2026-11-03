// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"encoding/json"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// A workspace's help links are kept in its sign-in document beside the four and read back as
// they went in; a workspace that set only a help link still stores a document.
func TestAWorkspacesHelpLinksAreStoredAndReadBack(t *testing.T) {
	links := identity.LegalLinks{SupportURL: "mailto:help@acme.example", DocsURL: "https://acme.example/docs"}
	stored := signInDocumentOf(identity.PolicyPatch{}, links)
	if stored == nil {
		t.Fatal("a workspace with only help links stored nothing")
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var read policyDocument
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatal(err)
	}
	_, back := signInPolicyOf(&read)
	if back != links {
		t.Errorf("read back %+v, want %+v", back, links)
	}
}
