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

// UC-ID-16 check 7: the installation decides *Private hubs allowed* like every other switch - a
// flag, a default and a lock, read from the document and answered back with its kind.
func TestTheInstallationDecidesPrivateHubsWithALock(t *testing.T) {
	level, err := instanceLevelFrom(usecase.Input{
		"sign_in": map[string]any{
			"private_hubs": map[string]any{"value": false, "locked": true},
		},
	})
	if err != nil {
		t.Fatalf("the document was refused: %v", err)
	}
	if level.Policy.Patch.PrivateHubs == nil || *level.Policy.Patch.PrivateHubs {
		t.Errorf("private_hubs read as %v", level.Policy.Patch.PrivateHubs)
	}
	if !level.Policy.Locks[identity.SwitchPrivateHubs] {
		t.Error("the lock was not read")
	}
	if _, err := instanceLevelFrom(usecase.Input{
		"sign_in": map[string]any{"private_hubs": map[string]any{"value": "no"}},
	}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a word for a flag answered %v", err)
	}

	settings, _ := instanceLevelOutput(level)["sign_in"].(usecase.Output)
	row, _ := settings["private_hubs"].(usecase.Output)
	if row["set"] != true || row["value"] != false || row["locked"] != true || row["kind"] != string(identity.KindFlag) {
		t.Errorf("the projection reads %v", row)
	}
}
