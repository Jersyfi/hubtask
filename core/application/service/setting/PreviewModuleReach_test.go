// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	automationrepo "github.com/Jersyfi/hubtask/core/application/repository/automation"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// people answers twelve in the workspace and five within the hub, and records where it was asked.
type people struct{ asked []shared.ID }

func (p *people) PeopleReaching(_ context.Context, hubID shared.ID) (int, error) {
	p.asked = append(p.asked, hubID)
	if hubID.IsZero() {
		return 12, nil
	}
	return 5, nil
}

// S-09's gap: switching a module off says how many people it leaves without it - everyone who
// reaches the workspace, or the hub - and nobody where the module is off already.
func TestAModuleSwitchedOffCountsThePeopleItLeavesWithoutIt(t *testing.T) {
	ctx := context.Background()
	modules, _, sink := fixture()
	counted := &people{}
	preview := settingservice.PreviewModuleReach{Modules: modules, People: counted, Rules: rules{}, Actions: actions{}}
	registry, err := usecase.NewRegistry(nil, preview.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	out, err := registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(), usecase.Input{"module": "PLANNING"})
	if err != nil {
		t.Fatal(err)
	}
	if out["on"] != false || out["people"] != 0 || len(counted.asked) != 0 {
		t.Errorf("a module off already reached %v, asked %v", out, counted.asked)
	}

	if _, err := (settingservice.SetModule{Modules: modules}).Execute(ctx, actor(), settingservice.SetModuleCommand{
		Module: module.Planning, Switch: settingservice.SwitchOn,
	}); err != nil {
		t.Fatal(err)
	}
	entries := len(sink.entries)
	out, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(), usecase.Input{"module": "PLANNING"})
	if err != nil || out["on"] != true || out["people"] != 12 {
		t.Errorf("the workspace answered %v, %v", out, err)
	}
	out, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "hub_id": hub.String()})
	if err != nil || out["people"] != 5 || out["hub_id"] != hub.String() {
		t.Errorf("the hub answered %v, %v", out, err)
	}
	if len(counted.asked) != 2 || !counted.asked[0].IsZero() || counted.asked[1] != hub {
		t.Errorf("asked %v", counted.asked)
	}
	if len(sink.entries) != entries {
		t.Errorf("the preview wrote %d audit entries", len(sink.entries)-entries)
	}

	_, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "hub_id": list.String()})
	if !refusedWith(err, "module.hub_only") {
		t.Errorf("a collection was answered: %v", err)
	}
}

// The preview is asked as the switch is.
func TestAModuleReachIsRefusedAsTheSwitchWouldBe(t *testing.T) {
	modules, _, _ := fixture()
	modules.Authorizer = authorizer{refuse: true}
	_, err := settingservice.PreviewModuleReach{Modules: modules, People: &people{}, Rules: rules{}, Actions: actions{}}.
		Execute(context.Background(), actor(), module.Planning, "", settingservice.SwitchOff)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("err = %v", err)
	}
}

// S-09's gap: leaving the workspace's module to the installation, whose default is off, counts
// the workspace's people; leaving a hub's to a workspace that has it on counts nobody.
func TestAModuleLeftToTheLevelAboveCountsOnlyWhenItTurnsOff(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	preview := settingservice.PreviewModuleReach{Modules: modules, People: &people{}, Rules: rules{}, Actions: actions{}}
	registry, err := usecase.NewRegistry(nil, preview.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	set := settingservice.SetModule{Modules: modules}
	for _, at := range []shared.ID{shared.ID(""), hub} {
		if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{
			Module: module.Planning, Switch: settingservice.SwitchOn, HubID: at,
		}); err != nil {
			t.Fatal(err)
		}
	}

	out, err := registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "switch": "INHERIT"})
	if err != nil || out["on"] != true || out["people"] != 12 {
		t.Errorf("leaving it to an installation that has it off answered %v, %v", out, err)
	}
	out, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "hub_id": hub.String(), "switch": "INHERIT"})
	if err != nil || out["on"] != true || out["people"] != 0 {
		t.Errorf("leaving it to a workspace that has it on answered %v, %v", out, err)
	}

	_, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "switch": "ON"})
	if err == nil {
		t.Error("a switch on was counted")
	}
}

// rules holds the workspace's switched-on rules, answered two per page.
type rules struct{ enabled []automation.Rule }

func (r rules) List(_ context.Context, query automationrepo.Query) (automationrepo.Page, error) {
	if query.Enabled == nil || !*query.Enabled {
		return automationrepo.Page{}, errors.New("asked for the rules that are off")
	}
	start := 0
	if query.Cursor != "" {
		start, _ = strconv.Atoi(query.Cursor)
	}
	end := min(start+2, len(r.enabled))
	page := automationrepo.Page{Rules: r.enabled[start:end], HasMore: end < len(r.enabled)}
	if page.HasMore {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

// actions says TAKE_STEP is the workflows module's and ADD_LABEL the base product's.
type actions struct{}

func (actions) ByAutomationAction(kind string) (usecase.Descriptor, bool) {
	switch kind {
	case "TAKE_STEP":
		return usecase.Descriptor{Name: "TakeStep", Module: module.Workflows}, true
	case "ADD_LABEL":
		return usecase.Descriptor{Name: "AddLabel"}, true
	}
	return usecase.Descriptor{}, false
}

func ruleAt(scope automation.Scope, trigger automation.TriggerKind, kinds ...string) automation.Rule {
	rule := automation.Rule{Enabled: true, Scope: scope, Trigger: automation.Trigger{Kind: trigger}}
	for _, kind := range kinds {
		rule.Actions = append(rule.Actions, automation.Action{Kind: kind})
	}
	return rule
}

// UC-INS-21 check 4: switching a module off also says how many switched-on rules go quiet - an
// action of the module is skipped, a trigger of it never fires - where the switch reaches: every
// rule for the workspace, and for a hub the workspace's rules and those of the hub and its
// collections.
func TestAModuleSwitchedOffCountsTheRulesItQuiets(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	other := shared.MustParseID("018f0000-0000-7000-8000-000000000009")
	preview := settingservice.PreviewModuleReach{Modules: modules, People: &people{}, Actions: actions{}, Rules: rules{enabled: []automation.Rule{
		ruleAt(automation.Scope{Type: automation.ScopeTenant}, automation.TriggerManual, "ADD_LABEL", "TAKE_STEP"),
		ruleAt(automation.Scope{Type: automation.ScopeHub, ID: hub}, automation.TriggerStepTaken, "ADD_LABEL"),
		ruleAt(automation.Scope{Type: automation.ScopeCollection, ID: list}, automation.TriggerManual, "TAKE_STEP"),
		ruleAt(automation.Scope{Type: automation.ScopeHub, ID: other}, automation.TriggerManual, "TAKE_STEP"),
		ruleAt(automation.Scope{Type: automation.ScopeTenant}, automation.TriggerManual, "ADD_LABEL"),
	}}}
	if _, err := (settingservice.SetModule{Modules: modules}).Execute(ctx, actor(), settingservice.SetModuleCommand{
		Module: module.Workflows, Switch: settingservice.SwitchOn,
	}); err != nil {
		t.Fatal(err)
	}

	reach, err := preview.Execute(ctx, actor(), module.Workflows, "", settingservice.SwitchOff)
	if err != nil || reach.Rules != 4 {
		t.Errorf("the workspace counted %d rules, %v", reach.Rules, err)
	}
	reach, err = preview.Execute(ctx, actor(), module.Workflows, hub, settingservice.SwitchOff)
	if err != nil || reach.Rules != 3 {
		t.Errorf("the hub counted %d rules, %v", reach.Rules, err)
	}
	reach, err = preview.Execute(ctx, actor(), module.Planning, "", settingservice.SwitchOff)
	if err != nil || reach.Rules != 0 {
		t.Errorf("a module off already counted %d rules, %v", reach.Rules, err)
	}
}
