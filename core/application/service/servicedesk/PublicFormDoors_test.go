// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func publicFormDoors(t *testing.T, s publicScene) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil,
		ReadPublicFormsPolicy{Forms: s.forms}.Descriptor(), SetPublicFormsPolicy{Forms: s.forms}.Descriptor(),
		CreatePublicForm{Forms: s.forms}.Descriptor(), ReadPublicForm{Forms: s.forms}.Descriptor(),
		DeletePublicForm{Forms: s.forms}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// UC-SVC-13 check 1 through the doors REST and MCP call: the workspace's switch is read and set
// only by who manages the workspace's members, never by an agent, and each switch is audited.
func TestThePublicFormsSwitchIsSetByAPersonThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	s := newPublicScene()
	s.settings.on = false
	trail := &trailOf{}
	auth := &gate{allow: true}
	s.forms.Portal.Desk.Audit, s.forms.Portal.Desk.Authorizer = trail, auth
	doors := publicFormDoors(t, s)

	read, err := doors.Invoke(ctx, ReadPublicFormsPolicyName, reader(), usecase.Input{})
	if err != nil || read["allowed"] != false || read["source"] != "WORKSPACE" {
		t.Fatalf("reading the switch: %v %v", read, err)
	}

	agent := reader()
	agent.Kind = appshared.ActorAIAgent
	if _, err := doors.Invoke(ctx, SetPublicFormsPolicyName, agent, usecase.Input{"allowed": true}); detailOf(err) != "public_forms.agent_decides_policy_not" {
		t.Fatalf("an agent switched public forms: %v", err)
	}
	auth.allow = false
	if _, err := doors.Invoke(ctx, SetPublicFormsPolicyName, reader(), usecase.Input{"allowed": true}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("switching without the right: %v", err)
	}
	if _, err := doors.Invoke(ctx, ReadPublicFormsPolicyName, reader(), usecase.Input{}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("reading without the right: %v", err)
	}
	if s.settings.on || len(trail.entries) != 0 {
		t.Fatalf("a refused switch was stored or audited: on=%v, %d entries", s.settings.on, len(trail.entries))
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, SetPublicFormsPolicyName, reader(), usecase.Input{}); err == nil {
		t.Fatal("a switch without its value was accepted")
	}

	set, err := doors.Invoke(ctx, SetPublicFormsPolicyName, reader(), usecase.Input{"allowed": true})
	if err != nil || set["allowed"] != true || !s.settings.on {
		t.Fatalf("switching on: %v %v", set, err)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != PublicFormsPolicySetAction ||
		trail.entries[0].Changes[string(domain.PublicFormsKey)].(map[string]any)["to"] != "true" {
		t.Fatalf("the trail of the switch: %+v", trail.entries)
	}

	unstored := s.forms
	unstored.Settings = nil
	if _, err := (SetPublicFormsPolicy{Forms: unstored}).Execute(ctx, reader(), true); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("a switch without a settings store: %v", err)
	}
	if r, err := unstored.allowed(ctx, tenantID); err != nil || r.Source == "" {
		t.Fatalf("without a settings store the switch resolves to its default: %+v %v", r, err)
	}
}

// UC-SVC-13 check 1 through the doors: a collection's form is made by who manages its members and
// its token answered once, read back without its token, and taken away; a refused maker, an agent
// and a missing collection write nothing.
func TestAPublicFormIsMadeReadAndTakenAwayThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	s := newPublicScene()
	trail := &trailOf{}
	auth := &gate{allow: true}
	s.forms.Portal.Desk.Audit, s.forms.Portal.Desk.Authorizer = trail, auth
	doors := publicFormDoors(t, s)
	at := usecase.Input{"collection_id": collection.String()}

	if _, err := doors.Invoke(ctx, ReadPublicFormName, reader(), at); detailOf(err) != "public_forms.link_not_found" {
		t.Fatalf("a collection without a form answered %v", err)
	}
	agent := reader()
	agent.Kind = appshared.ActorAIAgent
	if _, err := doors.Invoke(ctx, CreatePublicFormName, agent, at); detailOf(err) != "public_forms.agent_decides_policy_not" {
		t.Fatalf("an agent made a public form: %v", err)
	}
	auth.allow = false
	if _, err := doors.Invoke(ctx, CreatePublicFormName, reader(), at); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("making without the right: %v", err)
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, CreatePublicFormName, reader(), usecase.Input{"collection_id": "nope"}); err == nil {
		t.Fatal("a collection that is no id was accepted")
	}
	if _, err := (CreatePublicForm{Forms: s.forms}).Execute(ctx, reader(), ""); err == nil {
		t.Fatal("a form without its collection was made")
	}
	if len(s.links.rows) != 0 || len(trail.entries) != 0 {
		t.Fatalf("a refused form was written: %d links, %d entries", len(s.links.rows), len(trail.entries))
	}

	made, err := doors.Invoke(ctx, CreatePublicFormName, reader(), at)
	if err != nil || made.String("token") == "" || made["collection_id"] != collection.String() {
		t.Fatalf("making: %v %v", made, err)
	}
	read, err := doors.Invoke(ctx, ReadPublicFormName, reader(), at)
	if err != nil || read["created_by"] != handlerID.String() {
		t.Fatalf("reading: %v %v", read, err)
	}
	if _, carries := read["token"]; carries {
		t.Fatal("reading answered the token again")
	}

	auth.allow = false
	if _, err := doors.Invoke(ctx, DeletePublicFormName, reader(), at); !errors.Is(err, shared.ErrNotFound) || len(s.links.rows) != 1 {
		t.Fatalf("taking away without the right: %v", err)
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, DeletePublicFormName, reader(), at); err != nil || len(s.links.rows) != 0 {
		t.Fatalf("taking away: %v", err)
	}
	if _, err := doors.Invoke(ctx, DeletePublicFormName, reader(), at); detailOf(err) != "public_forms.link_not_found" {
		t.Fatalf("taking away twice: %v", err)
	}
	var actions []string
	for _, e := range trail.entries {
		actions = append(actions, string(e.Action))
	}
	if len(actions) != 2 || actions[0] != string(PublicFormCreatedAction) || actions[1] != string(PublicFormDeletedAction) {
		t.Fatalf("the trail: %v", actions)
	}

	unlinked := s.forms
	unlinked.Links = nil
	if _, err := (CreatePublicForm{Forms: unlinked}).Execute(ctx, reader(), collection); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("making without a link store: %v", err)
	}
	if _, err := (ReadPublicForm{Forms: unlinked}).Execute(ctx, reader(), collection); detailOf(err) != "public_forms.link_not_found" {
		t.Fatalf("reading without a link store: %v", err)
	}
	if err := (DeletePublicForm{Forms: unlinked}).Execute(ctx, reader(), collection); detailOf(err) != "public_forms.link_not_found" {
		t.Fatalf("taking away without a link store: %v", err)
	}
}
