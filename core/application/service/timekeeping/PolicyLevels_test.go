// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

// above is the installation's and the plan's say; the workspace's own is its row.
type above struct{ instance, plan map[setting.Key]setting.Entry }

func (a above) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance, Entries: a.instance}, setting.Layer{Level: setting.LevelPlan, Entries: a.plan}, nil
}
func (above) At(context.Context, []settingrepo.Place, string) ([]settingrepo.Stored, error) {
	return nil, nil
}
func (above) Put(context.Context, shared.ID, settingrepo.Place, setting.Key, setting.Entry, time.Time) error {
	return nil
}
func (above) Remove(context.Context, settingrepo.Place, setting.Key) (bool, error) { return false, nil }

// A plan that locks aggregated visibility and the installation's locked period bind every
// workspace: the read names where each part comes from, a change of the locked part is refused,
// a write into the installation's locked period is refused, and restating the lock is a save.
func TestTheInstallationAndThePlanDecideAboveTheWorkspacesPolicy(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.keeper.Settings = above{
		instance: map[setting.Key]setting.Entry{domain.LockedThroughKey: {Value: "2026-11-02", Hold: setting.HoldSet}},
		plan:     map[setting.Key]setting.Entry{domain.VisibilityKey: {Value: "AGGREGATED", Hold: setting.HoldLock}},
	}
	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)

	read, err := ReadTimePolicy{Keeper: f.keeper}.Execute(ctx, as(ana))
	if err != nil || read.Policy.Visibility != domain.VisibilityAggregated || dayText(read.Policy.LockedThrough) != "2026-11-02" {
		t.Fatalf("read %+v %v", read.Policy, err)
	}
	out := PolicyOutput(read)
	settings, _ := out["settings"].(usecase.Output)
	visibility, _ := settings[string(domain.VisibilityKey)].(usecase.Output)
	locked, _ := settings[string(domain.LockedThroughKey)].(usecase.Output)
	if visibility["source"] != "PLAN" || visibility["locked_by"] != "PLAN" || locked["source"] != "INSTANCE" || locked["locked_by"] != nil {
		t.Fatalf("settings %v", settings)
	}
	if _, err := (LogTime{Keeper: f.keeper}).Execute(ctx, as(ana), RecordFields{
		EntryID: checkout, WorkType: "development", Day: day(t, "2026-11-02"), Minutes: 30,
	}, time.UTC); detail(err) != "time.period_locked" {
		t.Fatalf("a record in the installation's locked period: %v", err)
	}

	if _, err := (SetTimePolicy{Keeper: f.keeper}).Execute(ctx, as(lead), domain.Policy{Visibility: domain.VisibilityFull}, 0); detail(err) != "setting.locked" {
		t.Fatalf("full visibility under the plan's lock: %v", err)
	}
	if f.store.policy.Version != 0 {
		t.Fatal("a refused policy was stored")
	}
	set, err := SetTimePolicy{Keeper: f.keeper}.Execute(ctx, as(lead), domain.Policy{Visibility: domain.VisibilityAggregated}, 0)
	if err != nil || set.Policy.Version != 1 || set.Visibility.Source != setting.LevelPlan {
		t.Fatalf("restating the lock %+v %v", set, err)
	}
	if set.LockedThrough.Source != setting.LevelInstance || dayText(set.Policy.LockedThrough) != "2026-11-02" {
		t.Fatalf("a row without a locked day leaves it to the installation: %+v", set.LockedThrough)
	}
}
