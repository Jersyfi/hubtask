// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package secret

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Threat T-18: secrets in logs. The type must mask on every common output path.
func TestSecretMasksOnEveryPath(t *testing.T) {
	s := New("super-secret-1234")

	cases := map[string]string{
		"%v": fmt.Sprintf("%v", s),
		// The %s path is exactly what is under test here, so the simplification staticcheck
		// suggests (calling String directly) would remove the test.
		"%s":     fmt.Sprintf("%s", s), //nolint:staticcheck // S1025: the verb is the subject of the test
		"%+v":    fmt.Sprintf("%+v", s),
		"%#v":    fmt.Sprintf("%#v", s),
		"struct": fmt.Sprintf("%+v", struct{ Key Secret }{s}),
	}
	for name, out := range cases {
		if strings.Contains(out, "super-secret") {
			t.Errorf("%s leaks the plaintext: %s", name, out)
		}
	}

	b, err := json.Marshal(struct {
		Key Secret `json:"key"`
	}{s})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(b), "super-secret") {
		t.Errorf("JSON leaks the plaintext: %s", b)
	}

	if got := s.Reveal(); got != "super-secret-1234" {
		t.Errorf("Reveal returned %q, expected the plaintext", got)
	}
}

func TestEmptySecret(t *testing.T) {
	var s Secret
	if !s.IsEmpty() {
		t.Error("the zero value must be empty")
	}
	if s.String() != "<empty>" {
		t.Errorf("expected <empty>, got %q", s.String())
	}
}
