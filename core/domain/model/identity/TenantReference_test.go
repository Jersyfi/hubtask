// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
)

func TestATenantReferenceIsTakenAsItIs(t *testing.T) {
	for reference, valid := range map[string]bool{
		"cus_8f2a91":             true,
		"sub-2026-0042":          true,
		"Kunde 4711 / Familie":   true,
		strings.Repeat("x", 200): true,
		"":                       false,
		strings.Repeat("x", 201): false,
		" cus_8f2a91":            false,
		"cus_8f2a91 ":            false,
		"cus\n8f2a91":            false,
		"cus\t8f2a91":            false,
		"cus\u20288f2a91":        false,
		"cus\u200b8f2a91":        false,
	} {
		if got := ValidTenantReference(reference) == nil; got != valid {
			t.Errorf("ValidTenantReference(%q) valid = %t, want %t", reference, got, valid)
		}
	}
}
