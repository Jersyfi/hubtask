// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-20 check 8: the installation decides *Managed accounts allowed* like every other switch - a
// default and a lock, read from the document and answered back with both.
func TestTheInstallationDecidesManagedAccountsWithALock(t *testing.T) {
	level, err := instanceLevelFrom(usecase.Input{
		"sign_in": map[string]any{
			"managed_accounts": map[string]any{"value": false, "locked": true},
		},
	})
	if err != nil {
		t.Fatalf("the document was refused: %v", err)
	}
	if level.Policy.Patch.ManagedAccounts == nil || *level.Policy.Patch.ManagedAccounts {
		t.Errorf("managed_accounts read as %v", level.Policy.Patch.ManagedAccounts)
	}
	if !level.Policy.Locks[identity.SwitchManagedAccounts] {
		t.Error("the lock was not read")
	}
	if _, err := instanceLevelFrom(usecase.Input{
		"sign_in": map[string]any{"managed_accounts": map[string]any{"value": "no"}},
	}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a word for a flag answered %v", err)
	}

	settings, _ := instanceLevelOutput(level)["sign_in"].(usecase.Output)
	row, _ := settings["managed_accounts"].(usecase.Output)
	if row["set"] != true || row["value"] != false || row["locked"] != true {
		t.Errorf("the projection reads %v", row)
	}
}
