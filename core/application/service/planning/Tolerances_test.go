// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// settingStore keeps a level's entries in memory, by place and key.
type settingStore struct{ stored []settingrepo.Stored }

func (s *settingStore) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance}, setting.Layer{Level: setting.LevelPlan}, nil
}

func (s *settingStore) At(_ context.Context, places []settingrepo.Place, _ string) ([]settingrepo.Stored, error) {
	var out []settingrepo.Stored
	for _, row := range s.stored {
		for _, place := range places {
			if row.Place == place {
				out = append(out, row)
			}
		}
	}
	return out, nil
}

func (s *settingStore) Put(_ context.Context, _ shared.ID, place settingrepo.Place, key setting.Key, entry setting.Entry, at time.Time) error {
	_, _ = s.Remove(context.Background(), place, key)
	s.stored = append(s.stored, settingrepo.Stored{Place: place, Key: key, Entry: entry, UpdatedAt: at})
	return nil
}

func (s *settingStore) Remove(_ context.Context, place settingrepo.Place, key setting.Key) (bool, error) {
	for i, row := range s.stored {
		if row.Place == place && row.Key == key {
			s.stored = append(s.stored[:i], s.stored[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// UC-PLN-04 checks 5 and 6: a hub's tolerance reaches its collections and, locked, is not changed
// there; a plan whose forecast end slips past its latest baseline's by more than the tolerance is
// in exception, and the exception is recorded.
func TestAPlanBeyondItsToleranceIsInException(t *testing.T) {
	f := newFixture()
	f.planner.Settings = &settingStore{}
	ctx := context.Background()
	two := 2
	if _, err := (SetPlanTolerance{Planner: f.planner}).Execute(ctx, actor(), SetPlanToleranceCommand{PlanID: hubID, Days: &two, Lock: true}); err != nil {
		t.Fatal(err)
	}
	if asked := f.gate.asked[len(f.gate.asked)-1]; asked.Permission != service.PermissionStructure || asked.TargetID != hubID {
		t.Errorf("asked %+v, want STRUCTURE at the hub", asked)
	}
	five := 5
	if _, err := (SetPlanTolerance{Planner: f.planner}).Execute(ctx, actor(), SetPlanToleranceCommand{PlanID: collection, Days: &five}); err == nil {
		t.Error("a collection changed the hub's locked tolerance")
	}
	agreed := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC)
	f.baselines.taken = []domain.Baseline{{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000301")}}
	f.baselines.found = domain.Baseline{ID: f.baselines.taken[0].ID, Entries: []domain.BaselineEntry{{EntryID: launch, DueAt: &agreed}}}
	view, err := ReadPlanTolerance{Planner: f.planner}.Execute(ctx, actor(), ScheduleQuery{PlanID: collection})
	if err != nil {
		t.Fatal(err)
	}
	if view.Resolution.Source != setting.LevelHub || view.Resolution.LockedBy != setting.LevelHub {
		t.Errorf("resolution = %+v, want the hub's, locked", view.Resolution)
	}
	if e := view.Exception; e == nil || !e.Exceeded || e.Tolerance != 2 || e.SlipDays <= 2 {
		t.Fatalf("exception = %+v", view.Exception)
	}
	if last := f.audit.entries[len(f.audit.entries)-1]; last.Action != ToleranceExceededAction {
		t.Errorf("recorded %v, want the exception", last.Action)
	}

	// The tolerance alone - the configuration document's read, inside a read-only transaction -
	// answers the same resolution and records nothing.
	recorded := len(f.audit.entries)
	out, err := ReadPlanTolerance{Planner: f.planner}.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{
		"plan_id": collection.String(), "tolerance_only": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["source"] != "HUB" || out["time_tolerance_days"] != 2 || out["exception"] != nil {
		t.Errorf("the tolerance alone answered %v", out)
	}
	if len(f.audit.entries) != recorded {
		t.Errorf("reading the tolerance alone recorded %v", f.audit.entries[recorded:])
	}
}

// told records whom a plan's exception was sent to.
type told struct{ recipients []shared.ID }

func (t *told) PlanExceeded(_ context.Context, _, _, recipient, _ shared.ID, _ time.Time) error {
	t.recipients = append(t.recipients, recipient)
	return nil
}

// configuration.md §10 at the workspace: its tolerance, locked, reaches every hub and collection and
// is not changed there, and the exception goes to the person who decided it - the level above.
func TestTheWorkspacesToleranceReachesEveryPlanAndHearsItsException(t *testing.T) {
	f := newFixture()
	store := &settingStore{}
	notified := &told{}
	f.planner.Settings, f.planner.Exceptions = store, notified
	ctx := context.Background()
	out, err := SetWorkspaceTolerance{Planner: f.planner}.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{"time_tolerance_days": 2, "lock": true})
	if err != nil {
		t.Fatal(err)
	}
	if asked := f.gate.asked[len(f.gate.asked)-1]; asked.Permission != service.PermissionStructure || asked.TargetID != tenantID {
		t.Errorf("asked %+v, want STRUCTURE at the workspace", asked)
	}
	row := out[string(domain.TimeToleranceKey)].(usecase.Output)
	if row["value"] != 2 || row["source"] != string(setting.LevelWorkspace) || row["locked_by"] != string(setting.LevelWorkspace) {
		t.Fatalf("the workspace's tolerance %v", row)
	}
	three := 3
	if _, err := (SetPlanTolerance{Planner: f.planner}).Execute(ctx, actor(), SetPlanToleranceCommand{PlanID: hubID, Days: &three}); err == nil {
		t.Error("a hub changed the workspace's locked tolerance")
	}
	// The workspace's tolerance was decided by somebody other than the reader.
	decider := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000302")
	store.stored[0].Entry.By = decider
	agreed := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC)
	f.baselines.taken = []domain.Baseline{{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000301")}}
	f.baselines.found = domain.Baseline{ID: f.baselines.taken[0].ID, Entries: []domain.BaselineEntry{{EntryID: launch, DueAt: &agreed}}}
	view, err := ReadPlanTolerance{Planner: f.planner}.Execute(ctx, actor(), ScheduleQuery{PlanID: collection})
	if err != nil {
		t.Fatal(err)
	}
	if view.Resolution.Source != setting.LevelWorkspace || view.Exception == nil || !view.Exception.Exceeded {
		t.Fatalf("view %+v", view)
	}
	if len(notified.recipients) != 1 || notified.recipients[0] != decider {
		t.Errorf("told %v, want the person who decided the tolerance", notified.recipients)
	}
}
