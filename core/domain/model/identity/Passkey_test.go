// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAPasskeysNameIsTrimmedAndBounded(t *testing.T) {
	for _, c := range []struct {
		raw, want string
		ok        bool
	}{
		{"iPhone", "iPhone", true},
		{"  Laptop at work \t", "Laptop at work", true},
		{"Schlüssel in der Schublade", "Schlüssel in der Schublade", true},
		{"\U0001F469\u200d\U0001F4BB laptop", "\U0001F469\u200d\U0001F4BB laptop", true},
		{strings.Repeat("ä", MaxPasskeyNameLength), strings.Repeat("ä", MaxPasskeyNameLength), true},
		{strings.Repeat("ä", MaxPasskeyNameLength+1), "", false},
		{"", "", false},
		{"   ", "", false},
		{"key\nline", "", false},
		{"key\x00", "", false},
		{"key\u202eevil", "", false},
		{"key\u2066isolated\u2069", "", false},
		{"\xff\xfe", "", false},
	} {
		got, err := PasskeyName(c.raw)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("%q: got %q, %v", c.raw, got, err)
			}
			continue
		}
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "auth.passkey_name_invalid" {
			t.Errorf("%q: accepted as %q (%v)", c.raw, got, err)
		}
	}
}
