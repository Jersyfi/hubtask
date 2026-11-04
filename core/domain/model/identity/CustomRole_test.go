// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A role's name is a label in every grant list and every role picker: on one line, and without a
// character the database cannot store (U+0000) or a list cannot show.
func TestARoleNameHoldsNoControlCharacter(t *testing.T) {
	const roleID = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	for name, given := range map[string]string{
		"a line break": "Lead\nReviewer",
		"a zero byte":  "Lead\x00",
		"a tab":        "Lead\tReviewer",
		"a C1 control": "Lead\u0085Reviewer",
		"a delete":     "Lead\u007f",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewCustomRole(roleID, tenantID, "lead", given, nil)
			if err == nil || shared.AsError(err).DetailCode != "roles.name_invalid" {
				t.Fatalf("error %v, want roles.name_invalid", err)
			}
		})
	}
	if _, err := NewCustomRole(roleID, tenantID, "lead", "Lead – Prüfer", nil); err != nil {
		t.Fatalf("an ordinary name refused: %v", err)
	}
}
