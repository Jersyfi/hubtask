// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
)

// UC-AST-12 checks 1 and 2: the register and every move keep the moment, who moved the asset and
// to whom or which stockroom, read back oldest first; an asset registered before the moves were
// kept answers its state alone, with no date invented.
func TestAnAssetKeepsEachMovesMoment(t *testing.T) {
	f, a := assuredFixture()
	register, move := RegisterCiAsset{Inventory: f.inv}.Descriptor(), MoveCiAsset{Inventory: f.inv}.Descriptor()
	read := ReadCiAsset{Inventory: f.inv}.Descriptor()
	if _, err := call(t, register, as(anna), usecase.Input{"ci_id": host.String(), "supplier": "Office supplies", "state": "ORDERED"}); err != nil {
		t.Fatal(err)
	}
	*f.now = f.now.Add(24 * time.Hour)
	if _, err := call(t, move, as(anna), usecase.Input{"ci_id": host.String(), "state": "IN_STOCK"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, register, as(anna), usecase.Input{"ci_id": host.String(), "assigned_to": bert.String()}); err != nil {
		t.Fatal(err)
	}
	*f.now = f.now.Add(24 * time.Hour)
	out, err := call(t, move, as(anna), usecase.Input{"ci_id": host.String(), "state": "DEPLOYED"})
	if err != nil {
		t.Fatal(err)
	}
	if moves := out["moves"].([]usecase.Output); len(moves) != 3 {
		t.Fatalf("the move answers the life so far: %v", moves)
	}
	out, err = call(t, read, as(bert), usecase.Input{"ci_id": host.String()})
	if err != nil {
		t.Fatal(err)
	}
	moves := out["moves"].([]usecase.Output)
	want := []struct {
		state string
		at    time.Time
	}{{"ORDERED", start}, {"IN_STOCK", start.Add(24 * time.Hour)}, {"DEPLOYED", start.Add(48 * time.Hour)}}
	if len(moves) != len(want) {
		t.Fatalf("moves: %v", moves)
	}
	for i, w := range want {
		if moves[i]["state"] != w.state || !moves[i]["at"].(time.Time).Equal(w.at) || moves[i]["by"] != anna.String() {
			t.Errorf("move %d: %v, want %s at %s", i, moves[i], w.state, w.at)
		}
	}
	if moves[2]["assigned_to"] != bert.String() || moves[0]["assigned_to"] != nil {
		t.Errorf("to whom: %v", moves)
	}

	a.assets[vm1] = domain.Asset{CIID: vm1, HubID: hub, State: domain.AssetDeployed, Version: 1}
	out, err = call(t, read, as(bert), usecase.Input{"ci_id": vm1.String()})
	if err != nil || out["state"] != "DEPLOYED" || len(out["moves"].([]usecase.Output)) != 0 {
		t.Errorf("an asset from before: %v %v", out, err)
	}
	if _, err := call(t, read, as(bert), usecase.Input{"ci_id": vault.String()}); err == nil {
		t.Error("Bert read the life of an asset in a hub out of his reach")
	}
}
