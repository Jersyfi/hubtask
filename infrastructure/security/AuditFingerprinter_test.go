// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	workspaceA = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	workspaceB = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
)

func TestAnAuditFingerprintIsKeyedAndNamesItsScheme(t *testing.T) {
	fingerprints := NewAuditFingerprinter(secret.New(key))
	got := fingerprints.Fingerprint(workspaceA, "lena")

	digest, prefixed := strings.CutPrefix(got, AuditFingerprintScheme)
	if !prefixed {
		t.Fatalf("the fingerprint does not name its scheme: %q", got)
	}
	if len(digest) != 64 {
		t.Errorf("digest length %d, want 64 hex characters", len(digest))
	}
	// The weakness this replaces: anybody could compute the plain hash of a guess and find it.
	plain := sha256.Sum256([]byte("lena"))
	if digest == hex.EncodeToString(plain[:]) {
		t.Error("the fingerprint is the plain hash of the value - a guess can be checked without the key")
	}
	if strings.Contains(got, "lena") {
		t.Error("the fingerprint carries the value")
	}
}

// Comparability within one workspace and one secret is what the fingerprint is for.
func TestTheSameValueInOneWorkspaceFingerprintsAlike(t *testing.T) {
	first := NewAuditFingerprinter(secret.New(key)).Fingerprint(workspaceA, "Groceries")
	fingerprints := NewAuditFingerprinter(secret.New(key))

	if first != fingerprints.Fingerprint(workspaceA, "Groceries") {
		t.Fatal("one value fingerprinted twice gives two answers - no two entries could be compared")
	}
	if fingerprints.Fingerprint(workspaceA, "Groceries") == fingerprints.Fingerprint(workspaceA, "Shopping") {
		t.Error("two values share a fingerprint")
	}
}

// And nothing beyond it: another workspace, or another installation secret, answers differently,
// so a value cannot be followed across workspaces or recognised from another installation.
func TestAFingerprintIsBoundToItsWorkspaceAndItsSecret(t *testing.T) {
	one := NewAuditFingerprinter(secret.New(key))
	other := NewAuditFingerprinter(secret.New(key + "-but-another-installation"))

	if one.Fingerprint(workspaceA, "Groceries") == one.Fingerprint(workspaceB, "Groceries") {
		t.Error("two workspaces share the fingerprint of one value")
	}
	if one.Fingerprint(workspaceA, "Groceries") == other.Fingerprint(workspaceA, "Groceries") {
		t.Error("two installation secrets give the same fingerprint")
	}
}

// The purpose label keeps a fingerprint from ever equalling a token hash of the same input.
func TestAFingerprintIsNotATokenHash(t *testing.T) {
	fingerprint := NewAuditFingerprinter(secret.New(key)).Fingerprint(workspaceA, "hbt_pat_a")
	token := hex.EncodeToString(NewTokenHasher(secret.New(key)).Hash(workspaceA.String() + "\x00hbt_pat_a"))

	if strings.TrimPrefix(fingerprint, AuditFingerprintScheme) == token {
		t.Error("the fingerprint and the token hash share a key")
	}
}
