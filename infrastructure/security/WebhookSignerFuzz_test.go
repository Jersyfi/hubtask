// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SG-8 for the outbound signature check: a header value and a body anybody can put in front of it.
//
// An ordinary test over its seeds in `make gate-unit`, a fuzz target in the nightly. Four
// properties:
//
//  1. Nothing presented makes Verify or SignedAt panic.
//  2. Verify accepts a header only when it carries the documented formula's HMAC of `<t>.<body>`
//     for its own `t` - recomputed here from the parsed parts, so a header that verifies without
//     being the formula is a forgery the check let through.
//  3. What Sign produces for the fuzzed body verifies, and stops verifying the moment the body
//     gains one byte or the secret changes.
//  4. A header that verifies reads back a timestamp; SignedAt never invents one for a header
//     Verify cannot parse.
func FuzzVerifySignature(f *testing.F) {
	signer := NewWebhookSigner()
	good := signer.Sign(signing, signedAt, body)
	stampPart, signaturePart, _ := strings.Cut(good, ",")
	for _, seed := range []struct {
		header string
		body   []byte
	}{
		{good, body},
		{good, append(append([]byte{}, body...), ' ')},
		{signaturePart + "," + stampPart, body},
		{good + ",v2=00ff", body},
		{"t=1787832000", body},
		{"v1=abcdef", body},
		{"t=,v1=", nil},
		{"t=1787832000,v1=zz", body},
		{"t=-1,v1=00", nil},
		{"t=99999999999999999999,v1=00", nil},
		{",,,", nil},
		{"t=1=2,v1=3=4", nil},
		{"", nil},
		{" t = 1 , v1 = 00 ", []byte{0}},
	} {
		f.Add(seed.header, seed.body)
	}

	other := secret.New("another-subscription's-secret")
	f.Fuzz(func(t *testing.T, header string, delivered []byte) {
		verified := signer.Verify(header, delivered, signing)
		stamp, signature, parsed := parseSignature(header)
		if verified {
			if !parsed || !bytes.Equal(signature, mac(signing, stamp, delivered)) {
				t.Fatalf("%q verified without being the formula's HMAC of its own t", header)
			}
			if signer.Verify(header, delivered, other) {
				t.Fatalf("%q verified under a secret that did not sign it", header)
			}
		}
		if at := signer.SignedAt(header); !at.IsZero() && !parsed {
			t.Fatalf("%q has no signature and still read back %v", header, at)
		}

		at := time.Unix(int64(len(header))*7919, 0)
		signed := signer.Sign(signing, at, delivered)
		if !signer.Verify(signed, delivered, signing) {
			t.Fatalf("what Sign wrote for %x does not verify", delivered)
		}
		if signer.Verify(signed, append(append([]byte{}, delivered...), 0), signing) {
			t.Fatalf("a body one byte longer than %x still verifies", delivered)
		}
		if signer.Verify(signed, delivered, other) {
			t.Fatal("a signature verified under another secret")
		}
		if got := signer.SignedAt(signed); !got.Equal(at.UTC()) {
			t.Fatalf("the timestamp reads back as %v, want %v", got, at.UTC())
		}
	})
}
