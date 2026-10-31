// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-SVC-13 check 1: a public form link's token names its tenant, reads back, and never prints itself.
func TestAFormLinkTokenReadsBackAndMasks(t *testing.T) {
	tenant := shared.ID("0b6f1f9e-3c1d-4f43-9a8e-2b7c5d1e6a40")
	token, err := NewFormLinkToken(tenant, bytes.Repeat([]byte{7}, FormLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFormLinkToken(token.Secret())
	if err != nil || parsed.TenantID() != tenant || parsed.Secret() != token.Secret() {
		t.Fatalf("parsed %v, %v", parsed.TenantID(), err)
	}
	if printed := fmt.Sprintf("%v %s %#v", token, token, token); strings.Contains(printed, token.Secret()[len(FormLinkTokenPrefix):]) {
		t.Fatalf("the token printed itself: %s", printed)
	}
	for name, raw := range map[string]string{
		"a page link":  strings.Replace(token.Secret(), FormLinkTokenPrefix, PageLinkTokenPrefix, 1),
		"short":        token.Secret()[:len(token.Secret())-1],
		"no tenant":    FormLinkTokenPrefix + "_" + strings.Repeat("a", 43),
		"bad alphabet": token.Secret()[:len(token.Secret())-1] + "+",
	} {
		if _, err := ParseFormLinkToken(raw); err == nil {
			t.Errorf("%s was read", name)
		}
	}
	if _, err := NewFormLinkToken(tenant, []byte{1}); err == nil {
		t.Error("a short secret minted a token")
	}
}
