// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// T-21, the threat the calendar feed adds: the only credential in this system that travels in a
// URL. What this file asserts is the half that can be asserted without a database - that the token
// is nothing but a calendar feed's, that it cannot be turned into anything else, and that it does
// not print itself. The rest of the row's evidence is the feed suite in test/integration and the
// route's own tests.

const feedInstallationSecret = "an installation secret of at least the length the config demands"

func feedToken(t *testing.T, tenant shared.ID, seed byte) integration.FeedToken {
	t.Helper()

	entropy := make([]byte, integration.FeedTokenSecretBytes)
	for i := range entropy {
		entropy[i] = seed + byte(i)
	}
	token, err := integration.NewFeedToken(tenant, entropy)
	if err != nil {
		t.Fatalf("minting the feed token: %v", err)
	}
	return token
}

// A feed token is not an API credential. It is refused by the parser that reads bearer tokens, so
// it cannot open a single authenticated route however it is presented.
func TestAFeedTokenIsNotAnAPICredential(t *testing.T) {
	token := feedToken(t, tenantID, 0x10)

	if _, err := identity.ParseToken(token.Secret()); err == nil {
		t.Fatal("a calendar feed token parses as a personal access token")
	}

	// Nor the other way around: a personal access token is not a feed URL.
	pat, err := identity.NewToken(tenantID, make([]byte, identity.TokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := integration.ParseFeedToken(pat.Secret()); err == nil {
		t.Error("a personal access token parses as a calendar feed token")
	}
}

// The purpose label, which is what keeps one leak from becoming two: the value stored for a feed
// is not the value stored for a personal access token, even for the same string and the same
// installation secret (security.md §5).
func TestAFeedHashCannotBeReplayedAsAnotherCredential(t *testing.T) {
	installation := secret.New(feedInstallationSecret)
	presented := feedToken(t, tenantID, 0x20).Secret()

	feedHash := security.NewFeedTokenHasher(installation).Hash(presented)
	tokenHash := security.NewTokenHasher(installation).Hash(presented)

	if string(feedHash) == string(tokenHash) {
		t.Error("the two hashers agree, so a stolen calendar_feed row is a stolen access_token row")
	}
	if len(feedHash) != 32 {
		t.Errorf("the stored value is %d bytes rather than an HMAC-SHA-256", len(feedHash))
	}
}

// Rule 10 treats the token like content, and this type is what enforces it: %v over a struct
// prints unexported fields, so leaving String off would not have been enough.
func TestAFeedTokenNeverPrintsItself(t *testing.T) {
	token := feedToken(t, tenantID, 0x30)
	secretHalf := strings.TrimPrefix(token.Secret(), integration.FeedTokenPrefix)

	encoded, err := token.MarshalText()
	if err != nil {
		t.Fatalf("marshalling failed: %v", err)
	}
	for _, printed := range []string{
		token.String(), token.GoString(), string(encoded),
	} {
		if strings.Contains(printed, secretHalf) {
			t.Errorf("the credential printed itself: %s", printed)
		}
	}
}

// A token rewritten to name another tenant hashes to something else, so it finds nothing: the
// hash covers the whole string, tenant half included.
func TestAFeedTokenCannotBeRewrittenToAnotherTenant(t *testing.T) {
	hasher := security.NewFeedTokenHasher(secret.New(feedInstallationSecret))
	mine := feedToken(t, tenantID, 0x40)

	rewritten := strings.Replace(mine.Secret(),
		strings.ReplaceAll(tenantID.String(), "-", ""),
		strings.ReplaceAll(strangerID.String(), "-", ""), 1)
	if rewritten == mine.Secret() {
		t.Fatal("the fixture did not rewrite anything")
	}
	if string(hasher.Hash(rewritten)) == string(hasher.Hash(mine.Secret())) {
		t.Error("a token naming another tenant hashes alike")
	}
}
