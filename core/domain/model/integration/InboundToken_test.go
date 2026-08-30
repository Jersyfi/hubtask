// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The address an INBOUND_WEBHOOK rule answers on (G-08). The same shape as the calendar feed's
// token and the same tests, because the two credentials make the same promises - and the day one
// of them stops making one, this is where it shows.

var inboundTenant = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")

func inboundSecret() []byte {
	entropy := make([]byte, InboundTokenSecretBytes)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	return entropy
}

func mintedInbound(t *testing.T) InboundToken {
	t.Helper()

	token, err := NewInboundToken(inboundTenant, inboundSecret())
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	return token
}

// The tenant travels inside the credential, because the lookup needs one before it can happen: the
// table is behind row level security, and a route with no authentication has no other honest source
// of a tenant context (multi-tenancy.md §2.2).
func TestAnInboundTokenNamesItsTenantAndSurvivesAParse(t *testing.T) {
	minted := mintedInbound(t)

	if !strings.HasPrefix(minted.Secret(), InboundTokenPrefix) {
		t.Errorf("token %q carries no scannable prefix", minted.Secret())
	}
	if minted.TenantID() != inboundTenant {
		t.Errorf("tenant %q, want the one it was minted for", minted.TenantID())
	}

	parsed, err := ParseInboundToken(minted.Secret())
	if err != nil {
		t.Fatalf("parsing a token this package minted: %v", err)
	}
	if parsed.TenantID() != inboundTenant || parsed.Secret() != minted.Secret() {
		t.Errorf("parsed %q for tenant %q", parsed.Secret(), parsed.TenantID())
	}
	if parsed.IsZero() {
		t.Error("a parsed token reports itself empty")
	}
}

// Every failure is the same error. A parser that said which check failed would be an oracle for the
// format, and the format is the one part of a token somebody guessing does not have to guess at.
func TestEveryMalformedInboundTokenAnswersTheSameRefusal(t *testing.T) {
	good := mintedInbound(t).Secret()
	body := good[len(InboundTokenPrefix):]
	_, secret, _ := strings.Cut(body, "_")

	for name, raw := range map[string]string{
		"empty":            "",
		"only the prefix":  InboundTokenPrefix,
		"another prefix":   strings.Replace(good, InboundTokenPrefix, FeedTokenPrefix, 1),
		"no separator":     InboundTokenPrefix + strings.Repeat("a", 32) + secret,
		"a short tenant":   InboundTokenPrefix + strings.Repeat("a", 31) + "_" + secret,
		"a short secret":   InboundTokenPrefix + strings.Repeat("a", 32) + "_" + secret[:10],
		"not base64url":    good[:len(good)-1] + "*",
		"a bad tenant hex": InboundTokenPrefix + strings.Repeat("z", 32) + "_" + secret,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseInboundToken(raw)
			if !errors.Is(err, shared.ErrUnauthenticated) {
				t.Fatalf("error %v, want the one refusal the parser gives", err)
			}
			if code := shared.AsError(err).DetailCode; code != "automation.inbound_token_malformed" {
				t.Errorf("code %q distinguishes one failure from another", code)
			}
		})
	}
}

// Minting refuses what it cannot make a credential out of. Internal shapes rather than a caller's
// mistake - nothing outside this system chooses either value.
func TestMintingAnInboundAddressRefusesWhatCannotBeACredential(t *testing.T) {
	if _, err := NewInboundToken("", inboundSecret()); err == nil {
		t.Error("a token was minted for no tenant")
	}
	if _, err := NewInboundToken(inboundTenant, []byte("short")); err == nil {
		t.Error("a token was minted from too little entropy")
	}
}

// This credential travels in a URL, where an access log, a proxy and a browser history all keep a
// copy of whatever is printed. Every way of printing it masks, and reading it is a deliberate act
// with a name (rule 10, security.md §4 T-21).
func TestAnInboundTokenNeverPrintsItself(t *testing.T) {
	minted := mintedInbound(t)

	for name, printed := range map[string]string{
		"String":   minted.String(),
		"GoString": minted.GoString(),
		"%v":       fmt.Sprintf("%v", minted),
		"%+v":      fmt.Sprintf("%+v", minted),
		"%#v":      fmt.Sprintf("%#v", minted),
	} {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(printed, minted.Secret()) {
				t.Errorf("%s printed the credential: %s", name, printed)
			}
			if !strings.Contains(printed, InboundTokenPrefix) {
				t.Errorf("%s says nothing about what was redacted: %s", name, printed)
			}
		})
	}

	// MarshalText is asked directly rather than through an encoder: the domain does no
	// serialisation, so an encoding/json import here would be the thing the layer gate forbids.
	// What matters is that the method a marshaller would call answers the mask.
	marshalled, err := minted.MarshalText()
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if strings.Contains(string(marshalled), minted.Secret()) {
		t.Errorf("MarshalText wrote the credential: %s", marshalled)
	}

	var empty InboundToken
	if empty.String() != "" || !empty.IsZero() {
		t.Errorf("the empty token prints %q", empty.String())
	}
	text, err := empty.MarshalText()
	if err != nil || len(text) != 0 {
		t.Errorf("the empty token encodes as %q (%v)", text, err)
	}
}

// The two prefixes are distinct on purpose, so that a scanner and a reader can both tell at a
// glance what a leaked string opens - and so that neither parser accepts the other's token.
func TestTheTwoUrlCredentialsCannotBeMistakenForEachOther(t *testing.T) {
	if InboundTokenPrefix == FeedTokenPrefix {
		t.Fatal("the inbound token and the calendar feed share a prefix")
	}

	feed, err := NewFeedToken(inboundTenant, inboundSecret())
	if err != nil {
		t.Fatalf("minting a feed token: %v", err)
	}
	if _, err := ParseInboundToken(feed.Secret()); err == nil {
		t.Error("a calendar feed token parses as an inbound address")
	}
	if _, err := ParseFeedToken(mintedInbound(t).Secret()); err == nil {
		t.Error("an inbound address parses as a calendar feed token")
	}
}
