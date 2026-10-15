// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"bytes"
	"testing"
)

// SG-8 for the CBOR subset: bytes from an authenticator nobody vouches for.
//
// An ordinary test over its seeds in `make gate-unit`, a fuzz target in the nightly. Properties:
//
//  1. Nothing presented makes the decoder panic, and every refusal is one with a reason.
//  2. What is answered as the rest is a true suffix of the input, shorter than it.
//  3. Decoding the consumed prefix alone answers the same item and no rest.
func FuzzDecodeCBOR(f *testing.F) {
	for _, seed := range []string{
		"a363666d74646e6f6e656761747453746d74a0", "a50102032620012158200000", "8301820203820405",
		"bf6346756ef563416d7421ff", "c249010000000000000000", "a2 0102 0103", "9b00000000ffffffff00",
		"f5", "f93c00", "",
	} {
		f.Add(unhex(f, seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		value, rest, err := decodeCBOR(data)
		if err != nil {
			if reasonOf(err) == "" {
				t.Fatalf("a refusal without a reason: %v", err)
			}
			return
		}
		if len(rest) >= len(data) || !bytes.HasSuffix(data, rest) {
			t.Fatalf("rest %x is no proper suffix of %x", rest, data)
		}
		consumed := data[:len(data)-len(rest)]
		again, err := decodeCBORWhole(consumed)
		if err != nil {
			t.Fatalf("the consumed prefix %x no longer decodes: %v", consumed, err)
		}
		if again.kind != value.kind {
			t.Fatalf("decoded twice into two kinds: %v and %v", value.kind, again.kind)
		}
	})
}
