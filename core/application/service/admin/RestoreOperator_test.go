// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-03 checks 2 and 3: the machine adds the account an address names and records it as done
// from the machine; an address that matches nothing is the one refusal, and changes nothing.
func TestTheMachineRestoresAnOperatorByAddress(t *testing.T) {
	register := newRegister()
	register.addresses = map[string]shared.ID{"demo\x00jerome@example.eu": adaAccount}
	writer, _, journal := newInstanceWriter(register)
	restore := RestoreOperator{Writer: writer}

	restored, err := restore.Execute(t.Context(), "demo", "jerome@example.eu")
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if !restored.Added || restored.AccountID != adaAccount || !register.accounts[adaAccount] {
		t.Errorf("restored = %+v, register = %v", restored, register.accounts)
	}
	if len(journal.entries) != 1 || journal.entries[0].ActorLabel != "machine" ||
		journal.entries[0].Action != "instance.operator_added" {
		t.Errorf("journal = %+v", journal.entries)
	}

	again, err := restore.Execute(t.Context(), "demo", "jerome@example.eu")
	if err != nil || again.Added {
		t.Errorf("a second run answered %+v, %v", again, err)
	}
	if len(journal.entries) != 1 {
		t.Error("a second run recorded a change that did not happen")
	}

	for _, pair := range [][2]string{{"demo", "nobody@example.eu"}, {"elsewhere", "jerome@example.eu"}} {
		_, err := restore.Execute(t.Context(), pair[0], pair[1])
		if shared.AsError(err).DetailCode != "admin.operator_unknown_account" {
			t.Errorf("%v answered %v", pair, err)
		}
	}
	if _, err := restore.Execute(t.Context(), "", "jerome@example.eu"); shared.AsError(err).DetailCode != "admin.operator_incomplete" {
		t.Errorf("a missing workspace answered %v", err)
	}
}
