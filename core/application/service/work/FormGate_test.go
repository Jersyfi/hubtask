// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// answers is a compiler whose every condition answers what the test says, and which records the
// channel it was asked under - the gate's part is putting the form together, the adapter's is CEL.
type answers struct {
	of       map[string]bool
	channels []any
}

func (a *answers) Compile(text string, _ expression.Environment, _ expression.Result) (expression.Program, error) {
	return answerProgram{text: text, of: a}, nil
}

type answerProgram struct {
	text string
	of   *answers
}

func (p answerProgram) Evaluate(ctx context.Context, in expression.Activation) (expression.Value, error) {
	channel, _, _ := in.Resolve(ctx, "channel")
	p.of.channels = append(p.of.channels, channel)
	return expression.Value{Bool: p.of.of[p.text]}, nil
}

func formedTypes(form domain.Form) *entryTypes {
	task := domain.SystemEntryType(domain.ItemTask)
	task.ID, task.TenantID, task.Version, task.Form = shared.MustParseID("0192f000-0000-7000-8000-0000000000e3"), tenantID, 1, form
	return &entryTypes{rows: []domain.EntryType{task}}
}

func formRefusal(t *testing.T, err error, detail string) *shared.Error {
	t.Helper()
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != detail {
		t.Fatalf("answered %v, want %s", err, detail)
	}
	return refusal
}

// UC-FLW-01 check 2, D-13: a type's form binds the creation - a required field, a validation, a
// channel it closes - whatever door the creation comes through.
func TestTheFormHoldsACreation(t *testing.T) {
	ctx := context.Background()
	compiler := &answers{of: map[string]bool{"short": false}}
	form := domain.Form{
		Fields:      []domain.FormField{{Key: "title"}, {Key: "notes", Required: true}},
		Validations: []domain.FormValidation{{Key: "short_title", Field: "title", Holds: "short"}},
		Channels:    []domain.FormChannelRule{{Channel: domain.ChannelMCP, Closed: true}},
	}
	h := newItemHarness()
	h.handler.Gate.Forms = FormGate{Types: formedTypes(form), Compiler: compiler}

	_, _, err := h.handler.Execute(ctx, itemActor(), taskCommand())
	refusal := formRefusal(t, err, "forms.refused")
	if len(refusal.Fields) != 2 || refusal.Fields[0].Path != "/notes" || refusal.Fields[0].Code != "fields.value_required" ||
		refusal.Fields[1].Code != "forms.validation_failed" || refusal.Fields[1].Params["validation"] != "short_title" {
		t.Fatalf("the refusal does not name each field: %+v", refusal.Fields)
	}

	compiler.of["short"] = true
	cmd := taskCommand()
	cmd.Notes = "Oat, not soy"
	if _, _, err := h.handler.Execute(ctx, itemActor(), cmd); err != nil {
		t.Fatalf("a creation that meets the form was refused: %v", err)
	}

	agent := itemActor()
	agent.Channel = string(domain.ChannelMCP)
	_, _, err = h.handler.Execute(ctx, agent, cmd)
	formRefusal(t, err, "forms.channel_closed")
	if compiler.channels[len(compiler.channels)-1] != "MCP" {
		t.Fatalf("the condition was not asked under the agent's channel: %v", compiler.channels)
	}
}

// D-13: an edit through any writer is held to the form as the entry stood - a read-only or hidden
// field is not changed, a required one not cleared - and an old entry is not refused for what it
// already lacked.
func TestTheFormHoldsAnEdit(t *testing.T) {
	ctx := context.Background()
	compiler := &answers{of: map[string]bool{"locked": true}}
	gate := FormGate{Types: formedTypes(domain.Form{Fields: []domain.FormField{
		{Key: "notes", ReadOnlyWhen: "locked"}, {Key: "custom_fields.severity", Required: true},
	}}), Compiler: compiler}
	collection := domain.Container{ID: collectionID}
	before := domain.WorkItem{Type: domain.ItemTask, Title: "a", Notes: "n", CustomFields: map[string]any{"severity": "high"}}

	renamed := before
	renamed.Title = "b"
	if err := gate.checkEdit(ctx, itemActor(), collection, before, renamed); err != nil {
		t.Fatalf("a rename was refused: %v", err)
	}
	noted := before
	noted.Notes = "m"
	refusal := formRefusal(t, gate.checkEdit(ctx, itemActor(), collection, before, noted), "forms.refused")
	if refusal.Fields[0].Code != "forms.field_read_only" {
		t.Fatalf("a read-only field changed: %+v", refusal.Fields)
	}
	cleared := before
	cleared.CustomFields = map[string]any{}
	refusal = formRefusal(t, gate.checkEdit(ctx, itemActor(), collection, before, cleared), "forms.refused")
	if refusal.Fields[0].Path != "/custom_fields/severity" {
		t.Fatalf("a required field was cleared: %+v", refusal.Fields)
	}
	old := cleared
	old.Title = "c"
	if err := gate.checkEdit(ctx, itemActor(), collection, cleared, old); err != nil {
		t.Fatalf("an entry that already lacked a field was refused a rename: %v", err)
	}
}

// A channel's preset fills the field it hides, and a default fills one the request left out.
func TestTheFormStartsANewEntry(t *testing.T) {
	gate := FormGate{Types: formedTypes(domain.Form{
		Fields: []domain.FormField{{Key: "custom_fields.severity", Default: "low"}, {Key: "custom_fields.source"}},
		Channels: []domain.FormChannelRule{{Channel: domain.ChannelMail, Hidden: []string{"custom_fields.source"},
			Presets: map[string]any{"custom_fields.source": "mail"}}},
	}), Compiler: &answers{}}
	mail := itemActor()
	mail.Channel = string(domain.ChannelMail)
	starting, err := gate.starting(context.Background(), mail, domain.Container{ID: collectionID},
		domain.WorkItem{Type: domain.ItemTask}, map[string]any{"source": "forged"})
	if err != nil {
		t.Fatal(err)
	}
	if starting["severity"] != "low" || starting["source"] != "mail" {
		t.Fatalf("starting values: %v", starting)
	}
}

// UC-ID-27 check 1: a restricted field is written only by whoever holds its permission - a custom
// role naming it, or a shipped owner or administrator - on every door, with or without a form; a
// write that leaves it as it was is no write of it.
func TestARestrictedFieldIsWrittenOnlyWithItsPermission(t *testing.T) {
	ctx := context.Background()
	fields := newCustomFieldStore()
	fields.stored[shared.MustParseID("0192f000-0000-7000-8000-0000000000f7")] = domain.CustomFieldDefinition{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f7"), TenantID: tenantID,
		Key: "cost", Kind: domain.CustomFieldNumber, Restricted: true,
	}
	collection := domain.Container{ID: collectionID}
	before := domain.WorkItem{Type: domain.ItemTask, Title: "a", CustomFields: map[string]any{"cost": 10.0}}
	priced := before
	priced.CustomFields = map[string]any{"cost": 12.0}
	renamed := before
	renamed.Title = "b"

	member := FormGate{Fields: fields, Roles: roleOf{role: identity.RoleMember}}
	refusal := formRefusal(t, member.checkEdit(ctx, itemActor(), collection, before, priced), "fields.restricted")
	if refusal.Params["key"] != "cost" {
		t.Fatalf("the refusal names %v", refusal.Params)
	}
	if err := member.checkEdit(ctx, itemActor(), collection, before, renamed); err != nil {
		t.Fatalf("a rename that leaves the field was refused: %v", err)
	}
	formRefusal(t, member.checkCreate(ctx, itemActor(), collection, priced), "fields.restricted")

	finance := FormGate{Fields: fields, Roles: roleOf{role: identity.RoleMember, custom: []string{"finance"},
		permissions: []service.Permission{service.FieldWrite("cost")}}}
	if err := finance.checkEdit(ctx, itemActor(), collection, before, priced); err != nil {
		t.Fatalf("the field's permission was refused: %v", err)
	}
	admin := FormGate{Fields: fields, Roles: roleOf{role: identity.RoleAdmin}}
	if err := admin.checkEdit(ctx, itemActor(), collection, before, priced); err != nil {
		t.Fatalf("an administrator was refused: %v", err)
	}
}
