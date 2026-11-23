// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type changes struct {
	recorded []changelog.Change
	fail     error
}

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	if c.fail != nil {
		return c.fail
	}
	c.recorded = append(c.recorded, change)
	return nil
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{Counter: 1} }

// switchOf reads one module's row of a record's payload.
func switchOf(t *testing.T, payload map[string]any, key module.Key) map[string]any {
	t.Helper()
	rows, _ := payload["modules"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["module"] == string(key) {
			return row
		}
	}
	t.Fatalf("%s missing from the record", key)
	return nil
}

// UC-INS-21 check 12: a switch records one `setting` change - the workspace's under no container,
// so every member's device receives it, a hub's under the hub, so whoever reads the hub does.
func TestASwitchIsRecordedForTheOtherDevices(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	log := &changes{}
	modules.Changes, modules.HLC = log, hlc{}
	set := settingservice.SetModule{Modules: modules}

	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn}); err != nil {
		t.Fatal(err)
	}
	if len(log.recorded) != 1 {
		t.Fatalf("a workspace switch recorded %d changes, want 1", len(log.recorded))
	}
	workspace := log.recorded[0]
	if workspace.Entity != settingservice.Entity || workspace.Op != changelog.Upsert || workspace.EntityID != tenant ||
		!workspace.ContainerID.IsZero() || workspace.TenantID != tenant || workspace.ActorID != lena {
		t.Errorf("the workspace's record is %+v", workspace)
	}
	if workspace.Payload["hub_id"] != nil {
		t.Errorf("the workspace's record names a hub: %v", workspace.Payload["hub_id"])
	}
	if row := switchOf(t, workspace.Payload, module.Planning); row["on"] != true {
		t.Errorf("the workspace's record says %v of the module switched on", row)
	}

	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOff, HubID: hub}); err != nil {
		t.Fatal(err)
	}
	if len(log.recorded) != 2 {
		t.Fatalf("a hub switch recorded %d changes in all, want 2", len(log.recorded))
	}
	narrowed := log.recorded[1]
	if narrowed.Entity != settingservice.Entity || narrowed.EntityID != hub || narrowed.ContainerID != hub {
		t.Errorf("the hub's record is %+v", narrowed)
	}
	if narrowed.Payload["hub_id"] != hub.String() {
		t.Errorf("the hub's record names %v", narrowed.Payload["hub_id"])
	}
	if row := switchOf(t, narrowed.Payload, module.Planning); row["on"] != false {
		t.Errorf("the hub's record says %v of the module switched off there", row)
	}
	for _, raw := range narrowed.Payload["modules"].([]any) {
		for field, value := range raw.(map[string]any) {
			if _, flag := value.(bool); !flag && field != "module" {
				t.Errorf("the record carries %q = %v: keys and booleans only", field, value)
			}
		}
	}
}

// A module switched on with the one it needs is one record of the place, not one per module.
func TestAModuleAndWhatItNeedsAreOneRecord(t *testing.T) {
	modules, _, _ := fixture()
	log := &changes{}
	modules.Changes, modules.HLC = log, hlc{}
	var needing module.Key
	for _, key := range module.Keys() {
		if len(key.Needs()) > 0 {
			needing = key
			break
		}
	}
	if needing == "" {
		t.Fatal("no module needs another: the case this test is for is gone")
	}
	if _, err := (settingservice.SetModule{Modules: modules}).Execute(context.Background(), actor(),
		settingservice.SetModuleCommand{Module: needing, Switch: settingservice.SwitchOn}); err != nil {
		t.Fatal(err)
	}
	if len(log.recorded) != 1 {
		t.Fatalf("recorded %d changes, want 1", len(log.recorded))
	}
	for _, key := range append([]module.Key{needing}, needing.Needs()...) {
		if row := switchOf(t, log.recorded[0].Payload, key); row["on"] != true {
			t.Errorf("%s is %v in the record", key, row)
		}
	}
}

// UC-INS-22 check 7: a change of the workspace's name or look writes the workspace's one `setting`
// record - under no container, so every member's device receives it - carrying what a switch's
// record carries, the switches as they stand, and nothing else (ADR-0108, ADR-0112).
func TestTheWorkspacesLookIsAnnouncedAsItsSwitchesRecord(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	log := &changes{}
	modules.Changes, modules.HLC = log, hlc{}

	if err := modules.AnnounceWorkspace(ctx, actor()); err != nil {
		t.Fatal(err)
	}
	if len(log.recorded) != 1 {
		t.Fatalf("announcing recorded %d changes, want 1", len(log.recorded))
	}
	announced := log.recorded[0]
	if announced.Entity != settingservice.Entity || announced.Op != changelog.Upsert || announced.EntityID != tenant ||
		!announced.ContainerID.IsZero() || announced.TenantID != tenant || announced.ActorID != lena {
		t.Errorf("the workspace's record is %+v", announced)
	}
	want, err := modules.Records(ctx, actor(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(announced.Payload, want[tenant]) {
		t.Errorf("the record carries %v, want the switches' record %v", announced.Payload, want[tenant])
	}

	modules.Changes = nil
	if err := modules.AnnounceWorkspace(ctx, actor()); err != nil || len(log.recorded) != 1 {
		t.Errorf("without a change log it answered %v and recorded %d", err, len(log.recorded))
	}
}

// A record that cannot be written fails the switch: a switch the other devices never hear of
// would be the stale screen the record is for.
func TestASwitchFailsWithItsRecord(t *testing.T) {
	modules, _, _ := fixture()
	broken := errors.New("log down")
	modules.Changes, modules.HLC = &changes{fail: broken}, hlc{}
	_, err := (settingservice.SetModule{Modules: modules}).Execute(context.Background(), actor(),
		settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn})
	if !errors.Is(err, broken) {
		t.Fatalf("got %v", err)
	}
}

// What the walk hands a device: the workspace's record and each hub's, a hub that says nothing
// following the workspace.
func TestRecordsAnswerTheWorkspaceAndEachHub(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	set := settingservice.SetModule{Modules: modules}
	for _, cmd := range []settingservice.SetModuleCommand{
		{Module: module.Planning, Switch: settingservice.SwitchOn},
		{Module: module.Planning, Switch: settingservice.SwitchOff, HubID: hub},
	} {
		if _, err := set.Execute(ctx, actor(), cmd); err != nil {
			t.Fatal(err)
		}
	}
	other := shared.MustParseID("018f0000-0000-7000-8000-000000000009")
	records, err := modules.Records(ctx, actor(), []shared.ID{hub, other})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want the workspace's and two hubs'", len(records))
	}
	if row := switchOf(t, records[tenant], module.Planning); row["on"] != true {
		t.Errorf("the workspace: %v", row)
	}
	if row := switchOf(t, records[hub], module.Planning); row["on"] != false {
		t.Errorf("the hub that narrowed: %v", row)
	}
	if row := switchOf(t, records[other], module.Planning); row["on"] != true {
		t.Errorf("the hub that says nothing: %v", row)
	}
	if records[other]["hub_id"] != other.String() || records[tenant]["hub_id"] != nil {
		t.Errorf("the records name %v and %v", records[other]["hub_id"], records[tenant]["hub_id"])
	}
}
