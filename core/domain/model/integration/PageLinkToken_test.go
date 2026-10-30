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

// UC-DOC-08 check 4: a public link's token names its tenant, reads back, and never prints itself.
func TestAPageLinkTokenReadsBackAndMasks(t *testing.T) {
	tenant := shared.ID("0b6f1f9e-3c1d-4f43-9a8e-2b7c5d1e6a40")
	token, err := NewPageLinkToken(tenant, bytes.Repeat([]byte{7}, PageLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePageLinkToken(token.Secret())
	if err != nil || parsed.TenantID() != tenant || parsed.Secret() != token.Secret() {
		t.Fatalf("parsed %v, %v", parsed.TenantID(), err)
	}
	if printed := fmt.Sprintf("%v %s %#v", token, token, token); strings.Contains(printed, token.Secret()[len(PageLinkTokenPrefix):]) {
		t.Fatalf("the token printed itself: %s", printed)
	}
	for name, raw := range map[string]string{
		"a feed token": strings.Replace(token.Secret(), PageLinkTokenPrefix, FeedTokenPrefix, 1),
		"short":        token.Secret()[:len(token.Secret())-1],
		"no tenant":    PageLinkTokenPrefix + "_" + strings.Repeat("a", 43),
		"bad alphabet": token.Secret()[:len(token.Secret())-1] + "+",
	} {
		if _, err := ParsePageLinkToken(raw); err == nil {
			t.Errorf("%s was read", name)
		}
	}
	if _, err := NewPageLinkToken(tenant, []byte{1}); err == nil {
		t.Error("a short secret minted a token")
	}
}
