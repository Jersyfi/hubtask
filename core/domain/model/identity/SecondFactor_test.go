// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"fmt"
	"testing"
)

// UC-ID-22 check 5, UC-ID-03 check 5: the last factor of either kind stays where the rule demands one.
func TestTheLastRequiredFactorOfEitherKindStays(t *testing.T) {
	for _, c := range []struct {
		app                    bool
		keys                   int
		required               bool
		held, appGoes, keyGoes bool
	}{
		{false, 0, false, false, false, false},
		{false, 0, true, false, false, false},
		{true, 0, false, true, true, false},
		{true, 0, true, true, false, false},
		{false, 1, false, true, false, true},
		{false, 1, true, true, false, false},
		{false, 2, true, true, false, true},
		{true, 1, true, true, true, true},
		{true, 2, false, true, true, true},
	} {
		t.Run(fmt.Sprintf("app=%v keys=%d required=%v", c.app, c.keys, c.required), func(t *testing.T) {
			f := SecondFactor{App: c.app, Keys: c.keys}
			if f.Held() != c.held {
				t.Errorf("held %v", f.Held())
			}
			if f.AppRemovable(c.required) != c.appGoes {
				t.Errorf("the app removable %v", f.AppRemovable(c.required))
			}
			if f.KeyRemovable(c.required) != c.keyGoes {
				t.Errorf("a key removable %v", f.KeyRemovable(c.required))
			}
		})
	}
}
