// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var otherDeskHub = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005fa")

// memPolicies keeps the policies by key.
type memPolicies struct{ rows []domain.SLAPolicy }

func (m *memPolicies) Insert(_ context.Context, p domain.SLAPolicy) error {
	m.rows = append(m.rows, p)
	return nil
}
func (m *memPolicies) Update(_ context.Context, p domain.SLAPolicy, _ int) error {
	for i := range m.rows {
		if m.rows[i].Key == p.Key {
			m.rows[i] = p
		}
	}
	return nil
}
func (m *memPolicies) Find(_ context.Context, key string) (domain.SLAPolicy, error) {
	for _, row := range m.rows {
		if row.Key == key {
			return row, nil
		}
	}
	return domain.SLAPolicy{}, shared.ErrNotFound
}
func (m *memPolicies) List(context.Context) ([]domain.SLAPolicy, error) { return m.rows, nil }
func (m *memPolicies) Delete(context.Context, string) error             { return nil }

// readsDeskHub lets the caller read every hub but the other one.
type readsDeskHub struct{}

func (readsDeskHub) Permitted(_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		out[i] = path[len(path)-1].ID != otherDeskHub
	}
	return out, nil
}

func hubPolicyDesk(f fixture, policies *memPolicies, auth *gate) Desk {
	return Desk{
		Calendars: f.clocks.Calendars, Policies: policies, Containers: placedIn{}, Authorizer: auth,
		Visibility: readsDeskHub{}, Audit: discard{}, UnitOfWork: inline{}, Clock: at(monday), IDs: f.ids,
		Text: text.Composing{},
	}
}

func hubDefine(key string, hub shared.ID) DefineSLAPolicyCommand {
	return DefineSLAPolicyCommand{
		Key: key, Name: "Desk incidents", CalendarKey: "office", EntryTypes: []string{"incident"},
		Goals: []domain.SLAGoal{{Kind: domain.GoalFirstResponse, Minutes: 30}}, Position: 9, Enabled: true, HubID: hub,
	}
}

func askedAtHub(auth *gate, hub shared.ID) bool {
	if len(auth.asked) == 0 {
		return false
	}
	path := auth.asked[len(auth.asked)-1].Path
	return len(path) == 2 && path[1] == identity.HubScope(hub)
}

// UC-SVC-02 check 13: a hub's own policy is defined with STRUCTURE on that hub, keeps its hub,
// is listed only to who may read the hub, and measures the hub's entries before the workspace's.
func TestAHubDefinesItsOwnSlaPolicy(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	shared0, _ := f.clocks.Policies.List(ctx)
	policies := &memPolicies{rows: append([]domain.SLAPolicy{}, shared0...)}
	auth := &gate{allow: true}
	desk := hubPolicyDesk(f, policies, auth)
	define := DefineSlaPolicy{Desk: desk}

	own, err := define.Execute(ctx, reader(), hubDefine("desk_incidents", deskHub))
	if err != nil || own.HubID != deskHub || !askedAtHub(auth, deskHub) {
		t.Fatalf("defining at the hub: %+v, %v, asked %+v", own, err, auth.asked)
	}
	if _, err := define.Execute(ctx, reader(), hubDefine("desk_incidents", "")); detailOf(err) != "sla_policies.hub_fixed" {
		t.Errorf("moving the policy to the workspace: %v", err)
	}
	if !askedAtHub(auth, deskHub) {
		t.Errorf("replacing a hub's policy was asked elsewhere: %+v", auth.asked[len(auth.asked)-1].Path)
	}
	if _, err := define.Execute(ctx, reader(), hubDefine("in_a_list", collection)); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a policy at a collection: %v", err)
	}
	if _, err := define.Execute(ctx, reader(), hubDefine("hidden", otherDeskHub)); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a hub the fixture does not hold: %v", err)
	}
	policies.rows = append(policies.rows, domain.SLAPolicy{Key: "hidden", HubID: otherDeskHub, Name: "Hidden"})

	listed, err := (ListSlaPolicies{Desk: desk}).Execute(ctx, reader())
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, p := range listed {
		keys[p.Key] = true
	}
	if !keys["incidents"] || !keys["desk_incidents"] || keys["hidden"] {
		t.Errorf("listed %v", keys)
	}
	if out := PolicyOutput(own); out["hub_id"] != deskHub.String() {
		t.Errorf("output %v", out)
	}

	// The entry sits in the hub's collection: its first response is the hub's promise.
	f.clocks.Policies, f.clocks.Containers = policies, placedIn{}
	f.deliver(t, event.ItemCreated, monday, nil)
	if got := f.records.goal(domain.GoalFirstResponse).PolicyKey; got != "desk_incidents" {
		t.Errorf("measured by %q, want the hub's own policy", got)
	}
}

// UC-SVC-02 check 13: Make a copy here gives the hub its own policy, naming its source; another
// hub's policy is no source.
func TestAHubCopiesASharedSlaPolicyHere(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	shared0, _ := f.clocks.Policies.List(ctx)
	policies := &memPolicies{rows: append(append([]domain.SLAPolicy{}, shared0...), domain.SLAPolicy{Key: "hidden", HubID: otherDeskHub})}
	auth := &gate{allow: true}
	copyHere := CopySlaPolicyHere{Desk: hubPolicyDesk(f, policies, auth)}
	copied, err := copyHere.Execute(ctx, reader(), CopySLAPolicyHereCommand{SourceKey: "incidents", HubID: deskHub, Key: "desk_incidents"})
	if err != nil || copied.HubID != deskHub || copied.CopiedFrom != "incidents" || copied.Name != "Incidents" || !askedAtHub(auth, deskHub) {
		t.Fatalf("copy %+v, %v", copied, err)
	}
	if _, err := copyHere.Execute(ctx, reader(), CopySLAPolicyHereCommand{SourceKey: "hidden", HubID: deskHub, Key: "taken"}); detailOf(err) != "sla_policies.not_found" {
		t.Errorf("another hub's policy was copied: %v", err)
	}
}
