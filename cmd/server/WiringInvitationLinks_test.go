// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
)

// Invitations travel by mail exactly where `features.mail` says mail goes out, and as a link where
// it does not: the screen reads the manifest, the server reads the same configuration, and the two
// readings are one method.
func TestInvitationsTravelAsLinksExactlyWhereNoMailIsConfigured(t *testing.T) {
	withMail := envport.Config{BaseURL: "https://tasks.example", Mail: envport.MailConfig{Host: "smtp.example"}}
	if links := invitationLinks(withMail, nil, nil); links.Minter != nil {
		t.Error("an installation with a mail server hands invitations over as links")
	}

	without := envport.Config{BaseURL: "https://home.example"}
	links := invitationLinks(without, nil, nil)
	if links.Minter == nil {
		t.Fatal("an installation without a mail server would queue mail that never leaves")
	}
	if links.BaseURL != "https://home.example" {
		t.Errorf("base %q, want the base the mail's link names", links.BaseURL)
	}
}
