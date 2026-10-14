// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SG-8 for the credential an inbound delivery presents - the whole of the authorisation on the
// webhook intake and on an INBOUND_WEBHOOK rule's address, read from a URL anybody can send.
//
// An ordinary test over its seeds in `make gate-unit`, a fuzz target in the nightly. Four
// properties:
//
//  1. Nothing presented makes the parser panic.
//  2. Every refusal is the one refusal: unauthenticated, `automation.inbound_token_malformed` -
//     a parser that said which check failed would be an oracle for the format.
//  3. What it accepts is the shape it mints: the prefix, a tenant that is not zero, and a token
//     that reads back as itself, so the hash covers exactly what arrived.
//  4. An accepted token never prints itself (rule 10).
func FuzzParseInboundToken(f *testing.F) {
	minted, err := NewInboundToken(inboundTenant, []byte(strings.Repeat("k", InboundTokenSecretBytes)))
	if err != nil {
		f.Fatalf("minting the seed: %v", err)
	}
	valid := minted.Secret()
	for _, seed := range []string{
		valid,
		"",
		InboundTokenPrefix,
		InboundTokenPrefix + "_",
		strings.TrimPrefix(valid, InboundTokenPrefix),
		valid + "=",
		valid[:len(valid)-1],
		strings.Replace(valid, "_", "-", 2),
		strings.ToUpper(valid),
		InboundTokenPrefix + strings.Repeat("0", tenantHexLength) + "_" + strings.Repeat("A", feedSecretLength),
		InboundTokenPrefix + strings.Repeat("g", tenantHexLength) + "_" + strings.Repeat("A", feedSecretLength),
		InboundTokenPrefix + strings.Repeat("a", tenantHexLength) + "_" + strings.Repeat("/", feedSecretLength),
		"hbt_feed_" + strings.TrimPrefix(valid, InboundTokenPrefix),
		valid + "\x00",
		"\xff\xfe" + valid,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, presented string) {
		token, err := ParseInboundToken(presented)
		if err != nil {
			if !errors.Is(err, shared.ErrUnauthenticated) ||
				shared.AsError(err).DetailCode != "automation.inbound_token_malformed" {
				t.Fatalf("a refusal of %q is %v, not the one refusal", presented, err)
			}
			if !token.IsZero() {
				t.Fatalf("a refused %q still answered a token", presented)
			}
			return
		}

		if !strings.HasPrefix(presented, InboundTokenPrefix) {
			t.Fatalf("%q was accepted without the prefix", presented)
		}
		if token.TenantID().IsZero() {
			t.Fatalf("%q was accepted with no tenant", presented)
		}
		if token.Secret() != presented {
			t.Fatalf("%q reads back as %q", presented, token.Secret())
		}
		again, err := ParseInboundToken(token.Secret())
		if err != nil || again.TenantID() != token.TenantID() {
			t.Fatalf("%q does not parse again to the same tenant: %v", presented, err)
		}
		for _, printed := range []string{token.String(), fmt.Sprintf("%v", token), fmt.Sprintf("%#v", token)} {
			if strings.Contains(printed, presented) {
				t.Fatalf("the token printed itself: %q", printed)
			}
		}
	})
}
