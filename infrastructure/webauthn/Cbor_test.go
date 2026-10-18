// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// The examples of RFC 8949 Appendix A that lie inside the subset, decoded and read back.
func TestTheRFCExamplesInsideTheSubsetDecode(t *testing.T) {
	ints := []struct {
		hex  string
		want int64
	}{
		{"00", 0}, {"01", 1}, {"0a", 10}, {"17", 23}, {"1818", 24}, {"1819", 25}, {"1864", 100},
		{"1903e8", 1000}, {"1a000f4240", 1000000}, {"1b000000e8d4a51000", 1000000000000},
		{"20", -1}, {"29", -10}, {"3863", -100}, {"3903e7", -1000},
	}
	for _, c := range ints {
		v, err := decodeCBORWhole(unhex(t, c.hex))
		if err != nil {
			t.Fatalf("%s: %v", c.hex, err)
		}
		got, ok := v.int()
		if !ok || got != c.want {
			t.Errorf("%s: got %d (%v), want %d", c.hex, got, ok, c.want)
		}
	}

	// 18446744073709551615 and -18446744073709551616 decode, but are no int64.
	for _, h := range []string{"1bffffffffffffffff", "3bffffffffffffffff"} {
		v, err := decodeCBORWhole(unhex(t, h))
		if err != nil {
			t.Fatalf("%s: %v", h, err)
		}
		if _, ok := v.int(); ok {
			t.Errorf("%s: read as an int64", h)
		}
	}

	texts := map[string]string{"60": "", "6161": "a", "6449455446": "IETF", "62225c": "\"\\",
		"62c3bc": "ü", "63e6b0b4": "水", "64f0908591": "\U00010151"}
	for h, want := range texts {
		v, err := decodeCBORWhole(unhex(t, h))
		if err != nil || v.kind != cborText || string(v.bytes) != want {
			t.Errorf("%s: got %q %v", h, v.bytes, err)
		}
	}

	v, err := decodeCBORWhole(unhex(t, "4401020304"))
	if err != nil || v.kind != cborBytes || !bytes.Equal(v.bytes, []byte{1, 2, 3, 4}) {
		t.Errorf("h'01020304': %v %v", v.bytes, err)
	}

	for h, want := range map[string]cborValue{
		"f4": {kind: cborBool, boolean: false}, "f5": {kind: cborBool, boolean: true}, "f6": {kind: cborNull},
	} {
		v, err := decodeCBORWhole(unhex(t, h))
		if err != nil || v.kind != want.kind || v.boolean != want.boolean {
			t.Errorf("%s: %+v %v", h, v, err)
		}
	}

	// [1, [2, 3], [4, 5]] and the 25-element array.
	v, err = decodeCBORWhole(unhex(t, "8301820203820405"))
	if err != nil || len(v.array) != 3 || len(v.array[1].array) != 2 {
		t.Errorf("nested array: %+v %v", v, err)
	}
	v, err = decodeCBORWhole(unhex(t, "98190102030405060708090a0b0c0d0e0f101112131415161718181819"))
	if err != nil || len(v.array) != 25 {
		t.Errorf("25 items: %d %v", len(v.array), err)
	}

	// {1: 2, 3: 4} and {"a": 1, "b": [2, 3]}.
	v, err = decodeCBORWhole(unhex(t, "a201020304"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.lookup(3); !ok || got.n != 4 {
		t.Errorf("{1:2,3:4}[3] = %+v", got)
	}
	v, err = decodeCBORWhole(unhex(t, "a26161016162820203"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.field("b"); !ok || len(got.array) != 2 {
		t.Errorf(`{"a":1,"b":[2,3]}["b"] = %+v`, got)
	}
	if _, ok := v.field("c"); ok {
		t.Error("an absent key was found")
	}
}

// Everything outside the subset, and every malformed input, is refused with its reason.
func TestWhatLiesOutsideTheSubsetIsRefused(t *testing.T) {
	cases := []struct {
		name, hex, reason string
	}{
		{"empty", "", "cbor.truncated"},
		{"a float", "f93c00", "cbor.simple_value"},
		{"a double", "fb3ff199999999999a", "cbor.simple_value"},
		{"undefined", "f7", "cbor.simple_value"},
		{"simple(16)", "f0", "cbor.simple_value"},
		{"simple(255)", "f8ff", "cbor.simple_value"},
		{"a tag", "c074323031332d30332d32315432303a30343a30305a", "cbor.tag"},
		{"a bignum", "c249010000000000000000", "cbor.tag"},
		{"an indefinite byte string", "5f42010243030405ff", "cbor.indefinite_length"},
		{"an indefinite array", "9fff", "cbor.indefinite_length"},
		{"an indefinite map", "bf6346756ef563416d7421ff", "cbor.indefinite_length"},
		{"a break alone", "ff", "cbor.indefinite_length"},
		{"reserved additional information", "1c", "cbor.reserved"},
		{"a truncated argument", "19 01", "cbor.truncated"},
		{"a truncated byte string", "44 0102", "cbor.truncated"},
		{"a byte string longer than the input can be", "5b ffffffffffffffff 00", "cbor.truncated"},
		{"an array claiming more items than bytes", "9b 00000000ffffffff 00", "cbor.truncated"},
		{"a map claiming more pairs than bytes", "bb 00000000ffffffff 0000", "cbor.truncated"},
		{"a truncated map", "a2 0102 03", "cbor.truncated"},
		{"text that is not UTF-8", "62 c328", "cbor.text_not_utf8"},
		{"a byte-string key", "a1 4101 02", "cbor.map_key_kind"},
		{"an array key", "a1 80 02", "cbor.map_key_kind"},
		{"a duplicate integer key", "a2 0102 0103", "cbor.duplicate_key"},
		{"a duplicate text key", "a2 616101 616102", "cbor.duplicate_key"},
		{"a trailing byte", "01 02", "cbor.trailing_bytes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := decodeCBORWhole(unhex(t, c.hex))
			if got := reasonOf(err); got != c.reason {
				t.Errorf("reason %q, want %q (%v)", got, c.reason, err)
			}
		})
	}
}

func TestNestingDeeperThanTheBoundIsRefused(t *testing.T) {
	deep := append(bytes.Repeat([]byte{0x81}, maxCBORDepth), 0x01)
	if _, err := decodeCBORWhole(deep); err != nil {
		t.Errorf("depth %d refused: %v", maxCBORDepth, err)
	}
	deeper := append(bytes.Repeat([]byte{0x81}, maxCBORDepth+1), 0x01)
	if got := reasonOf(func() error { _, err := decodeCBORWhole(deeper); return err }()); got != "cbor.too_deep" {
		t.Errorf("depth %d: reason %q", maxCBORDepth+1, got)
	}
}

func TestAnInputLargerThanTheBoundIsRefused(t *testing.T) {
	big := append([]byte{0x5a, 0, 1, 0, 0}, make([]byte, maxCBORSize)...)
	if got := reasonOf(func() error { _, err := decodeCBORWhole(big); return err }()); got != "cbor.too_large" {
		t.Errorf("reason %q", got)
	}
}

func TestTheRestAfterAnItemIsAnswered(t *testing.T) {
	v, rest, err := decodeCBOR(unhex(t, "a10102 ff ee"))
	if err != nil || len(v.pairs) != 1 || !bytes.Equal(rest, []byte{0xff, 0xee}) {
		t.Errorf("%+v %x %v", v, rest, err)
	}
}
