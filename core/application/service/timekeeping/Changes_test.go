// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{} }

func (c *changes) last(entity string) changelog.Change {
	for i := len(c.recorded) - 1; i >= 0; i-- {
		if c.recorded[i].Entity == entity {
			return c.recorded[i]
		}
	}
	return changelog.Change{}
}

// UC-TIM-01 offline: a person's records and running timer reach the change log addressed to them
// under no container - a removal as well -, and the kinds of work and the policy as one record each
// under the workspace.
func TestTimeIsToldToItsPersonsDevices(t *testing.T) {
	f := newFixture()
	log := &changes{}
	f.keeper.Changes, f.keeper.HLC = log, hlc{}
	ctx := context.Background()

	r := f.log(t, ana, checkout, "2026-11-02", 90)
	got := log.last(RecordEntity)
	if got.EntityID != r.ID || got.Op != changelog.Upsert || !got.ContainerID.IsZero() || got.Payload["account_id"] != ana.String() || got.Payload["minutes"] != 90 {
		t.Fatalf("a record's change = %+v", got)
	}
	if err := (DeleteTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), r.ID, r.Version); err != nil {
		t.Fatal(err)
	}
	if got := log.last(RecordEntity); got.Op != changelog.Delete || got.EntityID != r.ID || got.Payload["account_id"] != ana.String() {
		t.Errorf("a removal = %+v, want it addressed to its person", got)
	}

	if _, err := (StartTimer{Keeper: f.keeper}).Execute(ctx, as(ana), checkout, "development", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := log.last(TimerEntity); got.EntityID != ana || got.Op != changelog.Upsert || got.Payload["entry_id"] != checkout.String() {
		t.Errorf("a timer started = %+v", got)
	}
	f.clock.now = monday.Add(20 * time.Minute)
	stopped, err := StopTimer{Keeper: f.keeper}.Execute(ctx, as(ana), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if got := log.last(TimerEntity); got.Op != changelog.Delete || got.Payload["account_id"] != ana.String() {
		t.Errorf("a timer stopped = %+v, want its removal", got)
	}
	if got := log.last(RecordEntity); got.EntityID != stopped.ID || got.Op != changelog.Upsert {
		t.Errorf("the stopped timer's record = %+v", got)
	}

	if _, err := (DefineWorkType{Keeper: f.keeper}).Execute(ctx, as(lead), domain.WorkType{Key: "travel", Name: "Travel"}); err != nil {
		t.Fatal(err)
	}
	got = log.last(WorkTypesEntity)
	kinds, _ := got.Payload["data"].([]usecase.Output)
	found := false
	for _, k := range kinds {
		found = found || k["key"] == "travel"
	}
	if got.EntityID != tenantID || !found {
		t.Errorf("the kinds of work = %+v, want the workspace's list with the new one", got)
	}
}
