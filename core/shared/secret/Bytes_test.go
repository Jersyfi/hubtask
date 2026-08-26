// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package secret

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Threat T-18 again, for the type key material travels in. The plaintext of a key is the one
// value in this system whose disclosure cannot be recovered from by revoking anything.
func TestBytesMaskOnEveryPath(t *testing.T) {
	key := NewBytes([]byte("0123456789abcdef0123456789abcdef"))

	cases := map[string]string{
		"%v": fmt.Sprintf("%v", key),
		// The %s path is exactly what is under test, so calling String directly - which is what
		// staticcheck suggests - would remove the test.
		"%s":     fmt.Sprintf("%s", key), //nolint:staticcheck // S1025: the verb is the subject
		"%+v":    fmt.Sprintf("%+v", key),
		"%#v":    fmt.Sprintf("%#v", key),
		"struct": fmt.Sprintf("%+v", struct{ Key Bytes }{key}),
	}
	for name, out := range cases {
		if strings.Contains(out, "abcdef") {
			t.Errorf("%s leaks the material: %s", name, out)
		}
	}

	encoded, err := json.Marshal(struct {
		Key Bytes `json:"key"`
	}{key})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(encoded), "abcdef") {
		t.Errorf("JSON leaks the material: %s", encoded)
	}

	if string(key.Reveal()) != "0123456789abcdef0123456789abcdef" {
		t.Error("Reveal did not return the material")
	}
}

// The length is the one fact about a key that may be logged, and it has to be, because
// "16 bytes where 32 were expected" is what an operator needs in order to fix a configuration.
func TestBytesReportTheirLengthAndEmptiness(t *testing.T) {
	var zero Bytes
	if !zero.IsEmpty() || zero.Len() != 0 {
		t.Error("the zero value is not empty")
	}
	if zero.String() != "<empty>" {
		t.Errorf("the zero value prints as %q", zero.String())
	}

	key := NewBytes(make([]byte, 32))
	if key.IsEmpty() || key.Len() != 32 {
		t.Errorf("a 32-byte key reports empty=%v len=%d", key.IsEmpty(), key.Len())
	}
}

func TestBytesCompareInConstantTime(t *testing.T) {
	key := NewBytes([]byte("0123456789abcdef0123456789abcdef"))

	if !key.Equal(NewBytes([]byte("0123456789abcdef0123456789abcdef"))) {
		t.Error("two equal keys did not compare equal")
	}
	// A prefix, which is what a byte-by-byte comparison would answer fastest on.
	if key.Equal(NewBytes([]byte("0123456789abcdef0123456789abcdeg"))) {
		t.Error("two different keys compared equal")
	}
	if key.Equal(NewBytes([]byte("0123456789abcdef"))) {
		t.Error("a shorter key compared equal")
	}
	if key.Equal(Bytes{}) {
		t.Error("a key compared equal to nothing")
	}
}
