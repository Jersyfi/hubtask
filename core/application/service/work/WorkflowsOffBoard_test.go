// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	setting "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// workspaceModules is the settings store as the module gate reads it: the workspace's own
// switches, nothing above it.
type workspaceModules struct{ on map[module.Key]bool }

func (w workspaceModules) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance, Entries: map[setting.Key]setting.Entry{}},
		setting.Layer{Level: setting.LevelPlan, Entries: map[setting.Key]setting.Entry{}}, nil
}

func (w workspaceModules) At(_ context.Context, places []settingrepo.Place, _ string) ([]settingrepo.Stored, error) {
	var out []settingrepo.Stored
	for _, place := range places {
		if place.Level != setting.LevelWorkspace {
			continue
		}
		for key, on := range w.on {
			out = append(out, settingrepo.Stored{Place: place, Key: key.SettingKey(), Entry: setting.Entry{Value: on}})
		}
	}
	return out, nil
}

func (workspaceModules) Put(context.Context, shared.ID, settingrepo.Place, setting.Key, setting.Entry, time.Time) error {
	return errors.New("the gate writes nothing")
}

func (workspaceModules) Remove(context.Context, settingrepo.Place, setting.Key) (bool, error) {
	return false, errors.New("the gate writes nothing")
}

func (workspaceModules) HubsWithout(context.Context, setting.Key) ([]shared.ID, error) {
	return nil, nil
}

func (workspaceModules) PinHubs(context.Context, settingrepo.HubPin, time.Time) ([]shared.ID, error) {
	return nil, nil
}

// A board works with the Workflows module off (UC-FLW-06 check 5, UC-WRK-05 check 6): through the
// registry with the module gate, the base product's column write - PATCH /items/{id} with
// bucket_id - moves the entry, the done column completes it and a move out of it reopens it,
// while the board's drop belongs to the module and is refused with module.off. Switched on, the
// drop moves again, so the refusal is the gate's and not the fixture's.
func TestABoardMovesItsEntriesWithWorkflowsOff(t *testing.T) {
	h := coupledUpdateHarness()
	h.handler.Completion.Boards = h.handler.Boards
	drop := DropItemOnColumn{Update: h.handler, Step: TakeStep{Completion: h.handler.Completion}}
	switches := workspaceModules{on: map[module.Key]bool{module.Workflows: false}}
	gate := settingservice.ModuleGate{
		Modules: settingservice.Modules{Settings: switches, Containers: h.containers, UnitOfWork: h.uow},
		Items:   h.items,
	}
	registry, err := usecase.NewRegistry(gate.Wrap(nil), h.handler.Descriptor(), drop.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if h.handler.Descriptor().Module != "" {
		t.Fatalf("the column write belongs to %s; it is the base product's", h.handler.Descriptor().Module)
	}
	move := func(column shared.ID) (bool, shared.ID) {
		t.Helper()
		if _, err := registry.Invoke(t.Context(), UpdateWorkItemName, actorFixture(), usecase.Input{
			"item_id": taskID.String(), "bucket_id": column.String(),
		}); err != nil {
			t.Fatalf("the column write into %s with Workflows off: %v", column, err)
		}
		stored := h.items.stored[taskID]
		return stored.Completion.IsCompleted, stored.BucketID
	}

	if done, in := move(doingColumn); done || in != doingColumn {
		t.Errorf("into a middle column: in %s, done %v", in, done)
	}
	if done, in := move(doneColumn); !done || in != doneColumn {
		t.Errorf("into the done column: in %s, done %v; wants completed", in, done)
	}
	if done, in := move(todoColumn); done || in != todoColumn {
		t.Errorf("out of the done column: in %s, done %v; wants reopened", in, done)
	}

	_, err = registry.Invoke(t.Context(), DropOnColumnName, actorFixture(), usecase.Input{
		"item_id": taskID.String(), "bucket_id": doneColumn.String(),
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != module.OffCode {
		t.Fatalf("the drop with Workflows off: %v; wants %s", err, module.OffCode)
	}
	if h.items.stored[taskID].BucketID != todoColumn {
		t.Error("the refused drop moved the entry")
	}

	switches.on[module.Workflows] = true
	if _, err := registry.Invoke(t.Context(), DropOnColumnName, actorFixture(), usecase.Input{
		"item_id": taskID.String(), "bucket_id": doneColumn.String(),
	}); err != nil {
		t.Errorf("the drop with Workflows on: %v", err)
	}
	if stored := h.items.stored[taskID]; stored.BucketID != doneColumn || !stored.Completion.IsCompleted {
		t.Errorf("the drop with Workflows on: in %s, done %v", stored.BucketID, stored.Completion.IsCompleted)
	}
}
