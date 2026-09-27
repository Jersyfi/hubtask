// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/port/text"
)

// The list ships, and it is not empty. A list quietly emptied by a refactoring would be a switch
// that says it is on and refuses nothing.
func TestTheCommonPasswordListShips(t *testing.T) {
	if CommonPasswordCount() < 80 {
		t.Errorf("%d entries - the list was emptied", CommonPasswordCount())
	}
}

// What it catches, and what it deliberately does not.
func TestTheCommonListCatchesTheFamiliesAndNotAPassphrase(t *testing.T) {
	for _, refused := range []string{
		"password", "P@ssw0rd", "Passwort2026!", "letmein now please", "qwertz123456",
		"Sommer2026!!", "correct horse battery staple", "CorrectHorseBatteryStaple",
		"correct-horse-battery-staple", "hubtask-admin-1", "changeme please",
	} {
		if !IsCommonPassword(text.Composing{}, refused) {
			t.Errorf("%q was not caught", refused)
		}
	}

	for _, accepted := range []string{
		"seven blue lanterns", "marmalade harbour 2026", "nine violet lanterns 8",
		"a quiet Tuesday in March", "",
		// Too short to carry an entry, and too short to pass the length rule either.
		"abc",
	} {
		if IsCommonPassword(text.Composing{}, accepted) {
			t.Errorf("%q was caught, and nothing in the list is in it", accepted)
		}
	}
}

// A caller with no normaliser gets the same answer for every ASCII password, which is what the
// compatibility shim needs and what every entry in this list is.
func TestTheListAnswersWithoutANormaliser(t *testing.T) {
	if !IsCommonPassword(nil, "Passwort2026") {
		t.Error("a folded entry was missed without a normaliser")
	}
}
