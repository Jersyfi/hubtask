// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"bytes"
	"testing"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const installationSecret = "an installation secret of at least the length the config demands"

func TestAFeedHashIsDeterministicAndCoversTheWholeToken(t *testing.T) {
	hasher := NewFeedTokenHasher(secret.New(installationSecret))

	token := "hbt_cal_018f2a1b0000700080000000000000ab_" +
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	if !bytes.Equal(hasher.Hash(token), hasher.Hash(token)) {
		t.Fatal("the same token hashed to two values, and the lookup is an index seek on one")
	}

	// The tenant half is covered, so a token rewritten to name another tenant matches nothing.
	rewritten := "hbt_cal_018f2a1b0000700080000000000000ff_" +
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	if bytes.Equal(hasher.Hash(token), hasher.Hash(rewritten)) {
		t.Error("two tenants' tokens hashed alike")
	}
}

// The purpose label is what security.md §5 asks for: one installation secret, one derivation per
// use, and no value produced for one purpose usable as a value of another.
func TestAFeedHashIsNotAnAccessTokenHash(t *testing.T) {
	installation := secret.New(installationSecret)
	token := "hbt_cal_018f2a1b0000700080000000000000ab_" +
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

	if bytes.Equal(NewFeedTokenHasher(installation).Hash(token),
		NewTokenHasher(installation).Hash(token)) {
		t.Error("the two hashers agree, and one leak would then be two")
	}
}

// A different installation secret is a different feed altogether: a database restored onto a
// server whose key differs serves nothing rather than serving somebody else's calendar.
func TestAFeedHashIsBoundToTheInstallation(t *testing.T) {
	token := "hbt_cal_018f2a1b0000700080000000000000ab_" +
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

	first := NewFeedTokenHasher(secret.New(installationSecret)).Hash(token)
	second := NewFeedTokenHasher(secret.New("another installation secret entirely, at length")).
		Hash(token)
	if bytes.Equal(first, second) {
		t.Error("the pepper made no difference")
	}
}
