// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// settingsStore keeps the workspace's entries and an installation layer a test may lock.
type settingsStore struct {
	instance setting.Layer
	stored   []settingrepo.Stored
}

func (s *settingsStore) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return s.instance, setting.Layer{Level: setting.LevelPlan}, nil
}

func (s *settingsStore) At(context.Context, []settingrepo.Place, string) ([]settingrepo.Stored, error) {
	return s.stored, nil
}

func (s *settingsStore) Put(_ context.Context, _ shared.ID, place settingrepo.Place, key setting.Key, entry setting.Entry, _ time.Time) error {
	s.stored = []settingrepo.Stored{{Place: place, Key: key, Entry: entry}}
	return nil
}

func (s *settingsStore) Remove(context.Context, settingrepo.Place, setting.Key) (bool, error) {
	return false, nil
}

func policyFixture() (fixture, *settingsStore) {
	f := newFixture()
	settings := &settingsStore{instance: setting.Layer{Level: setting.LevelInstance}}
	f.desk.Settings = settings
	return f, settings
}

func throughMCP(actor appshared.ActorContext) appshared.ActorContext {
	actor.Channel = string(work.ChannelMCP)
	return actor
}

// UC-AGT-02 check 10: with agents.registered_only on, MCP refuses a person's own token and a service
// account no agent names, and records it; off - the default - and on any other door, they pass.
func TestRegisteredOnlyRefusesUnregisteredCallersOnMCP(t *testing.T) {
	f, _ := policyFixture()
	read := usecase.Descriptor{Name: "GetWorkItem", ReadOnly: true}
	ran := 0
	next := usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
		ran++
		return usecase.Output{}, nil
	})
	handler := Gate{Desk: f.desk}.Wrap(nil)(read, next)
	robot := appshared.ActorContext{Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: shared.ID("01936f2a-7c1e-7000-8000-00000000dead"),
		Scopes: []string{"items:read"}}
	callers := map[string]appshared.ActorContext{"a person's own token": person(ben), "an unregistered service account": robot}

	for name, actor := range callers {
		if _, err := handler.Invoke(context.Background(), throughMCP(actor), usecase.Input{}); err != nil {
			t.Fatalf("%s with the switch off: %v", name, err)
		}
	}
	out, err := SetAgentPolicy{Desk: f.desk}.Descriptor().Handler.Invoke(context.Background(), person(admin),
		usecase.Input{"registered_only": true})
	if err != nil {
		t.Fatalf("switching on: %v", err)
	}
	if row, _ := out[string(domain.RegisteredOnlyKey)].(usecase.Output); row["value"] != true || row["source"] != string(setting.LevelWorkspace) {
		t.Errorf("answered %v", out)
	}
	before := ran
	for name, actor := range callers {
		if _, err := handler.Invoke(context.Background(), throughMCP(actor), usecase.Input{}); code(err) != "agents.registration_required" {
			t.Errorf("%s on MCP answered %v, want agents.registration_required", name, err)
		}
		if _, err := handler.Invoke(context.Background(), actor, usecase.Input{}); err != nil {
			t.Errorf("%s on REST was refused: %v", name, err)
		}
	}
	if ran != before+len(callers) {
		t.Errorf("ran %d acts, want only the REST ones", ran-before)
	}
	refused := 0
	for _, entry := range f.trail.entries {
		if entry.Action == UnregisteredRefusedAction {
			refused++
		}
	}
	if refused != len(callers) {
		t.Errorf("%d refusals recorded, want %d", refused, len(callers))
	}
}

// A level above that switched it on and locked it is not switched off by the workspace.
func TestRegisteredOnlyLockedAboveStaysOn(t *testing.T) {
	f, settings := policyFixture()
	settings.instance.Entries = map[setting.Key]setting.Entry{
		domain.RegisteredOnlyKey: {Value: true, Hold: setting.HoldLock},
	}
	if _, err := (SetAgentPolicy{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(admin),
		usecase.Input{"registered_only": false}); err == nil {
		t.Fatal("the workspace switched off what the installation locked on")
	}
	out, err := ReadAgentPolicy{Desk: f.desk}.Descriptor().Handler.Invoke(context.Background(), person(admin), usecase.Input{})
	row, _ := out[string(domain.RegisteredOnlyKey)].(usecase.Output)
	if err != nil || row["value"] != true || row["locked_by"] != string(setting.LevelInstance) {
		t.Errorf("read %v (%v), want on and locked by the installation", out, err)
	}
	if _, err := (SetAgentPolicy{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(ben),
		usecase.Input{"registered_only": true}); code(err) != "access.not_permitted" {
		t.Errorf("a member set the policy: %v", err)
	}
}
