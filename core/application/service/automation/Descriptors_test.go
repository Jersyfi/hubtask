// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The channel-neutral side: what REST, MCP and an automation action all send, and what they are all
// answered. The shapes travel as nested documents because that is what they are - a trigger is one
// object whose fields belong to its kind.

func validInput() usecase.Input {
	return usecase.Input{
		"name":   "Escalate overdue approvals",
		"scope":  map[string]any{"type": "TENANT"},
		"run_as": serviceID.String(),
		"trigger": map[string]any{
			"kind": "EVENT", "event_type": string(event.ItemOverdue),
		},
		"actions": []any{
			map[string]any{"kind": "ADD_LABEL", "params": map[string]any{"label_id": "x"}},
		},
	}
}

func permittedHarness() *harness {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)
	return h
}

func TestTheInputIsReadIntoTheAggregate(t *testing.T) {
	h := permittedHarness()

	in := validInput()
	in["scope"] = map[string]any{"type": "HUB", "id": hubID.String()}
	in["trigger"] = map[string]any{
		"kind": "EVENT", "event_type": string(event.ItemUpdated),
		"changed_fields": []any{"due_at", " "},
	}
	in["throttle"] = map[string]any{"max_runs_per_hour": float64(100)}
	in["on_error"] = "CONTINUE"

	out, err := CreateRule{Writer: h.writer}.invoke(context.Background(), writerActor(), in)
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	stored := h.store.rows[shared.ID(out.String("id"))]
	if stored.Scope.Type != domain.ScopeHub || stored.Scope.ID != hubID {
		t.Errorf("scope %+v, want the hub", stored.Scope)
	}
	if stored.Trigger.EventType != event.ItemUpdated {
		t.Errorf("event type %q", stored.Trigger.EventType)
	}
	// The blank one is dropped rather than stored: a filter on "" would match nothing and read as
	// though it matched everything.
	if len(stored.Trigger.ChangedFields) != 1 || stored.Trigger.ChangedFields[0] != "due_at" {
		t.Errorf("changed fields %v, want just due_at", stored.Trigger.ChangedFields)
	}
	if stored.Throttle.MaxRunsPerHour != 100 {
		t.Errorf("throttle %d, want 100", stored.Throttle.MaxRunsPerHour)
	}
	if stored.OnError != domain.OnErrorContinue {
		t.Errorf("on_error %q, want CONTINUE", stored.OnError)
	}
}

// A rule saved from `{"trigger": "EVENT"}` would be a rule with no trigger at all, and the caller
// would be told nothing.
func TestADocumentThatIsNotADocumentIsRefusedByField(t *testing.T) {
	cases := map[string]struct {
		mutate func(usecase.Input)
		path   string
	}{
		"a trigger that is a string": {
			mutate: func(in usecase.Input) { in["trigger"] = "EVENT" },
			path:   "/trigger",
		},
		"a scope that is a string": {
			mutate: func(in usecase.Input) { in["scope"] = "TENANT" },
			path:   "/scope",
		},
		"an action that is a string": {
			mutate: func(in usecase.Input) { in["actions"] = []any{"ADD_LABEL"} },
			path:   "/actions/0",
		},
		"a condition that is a string": {
			mutate: func(in usecase.Input) { in["conditions"] = []any{"x == 1"} },
			path:   "/conditions/0",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := permittedHarness()

			in := validInput()
			tc.mutate(in)

			_, err := CreateRule{Writer: h.writer}.invoke(context.Background(), writerActor(), in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want ErrValidation", err)
			}
			if code := fieldCodes(t, err)[tc.path]; code != "automation.object_invalid" {
				t.Errorf("%s says %q, want automation.object_invalid", tc.path, code)
			}
		})
	}
}

func TestAnUnparseableScopeIdentifierIsRefused(t *testing.T) {
	h := permittedHarness()

	in := validInput()
	in["scope"] = map[string]any{"type": "HUB", "id": "not-a-uuid"}

	_, err := CreateRule{Writer: h.writer}.invoke(context.Background(), writerActor(), in)
	if code := fieldCodes(t, err)["/scope/id"]; code != "automation.scope_id_invalid" {
		t.Errorf("the refusal says %q, want automation.scope_id_invalid", code)
	}
}

// One projection, so REST, MCP and an automation action cannot come to describe a rule differently.
func TestTheProjectionAnswersTheTriggersOwnFieldsAndNoOthers(t *testing.T) {
	h := permittedHarness()

	out, err := CreateRule{Writer: h.writer}.invoke(
		context.Background(), writerActor(), validInput())
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	trigger, ok := out["trigger"].(map[string]any)
	if !ok {
		t.Fatalf("trigger is %v, want a document", out["trigger"])
	}
	if trigger["kind"] != "EVENT" || trigger["event_type"] != string(event.ItemOverdue) {
		t.Errorf("trigger %v", trigger)
	}
	for _, absent := range []string{"rrule", "timezone", "anchor", "offset", "changed_fields"} {
		if _, present := trigger[absent]; present {
			t.Errorf("%q is answered for an event trigger", absent)
		}
	}

	if out["enabled"] != false {
		t.Errorf("enabled is %v, want false on a rule just written", out["enabled"])
	}
	scope, _ := out["scope"].(map[string]any)
	if scope["type"] != "TENANT" {
		t.Errorf("scope %v", scope)
	}
	// A tenant scope names no identifier, and the projection does not invent one.
	if _, present := scope["id"]; present {
		t.Errorf("a tenant scope answered an id: %v", scope)
	}
}

func TestTheOtherOperationsReachTheirUseCases(t *testing.T) {
	h := permittedHarness()

	created, err := CreateRule{Writer: h.writer}.invoke(
		context.Background(), writerActor(), validInput())
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	id := created.String("id")

	got, err := GetRule{Writer: h.writer}.invoke(
		context.Background(), writerActor(), usecase.Input{"rule_id": id})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if got.String("id") != id {
		t.Errorf("read back %q, want %q", got.String("id"), id)
	}

	renamed := "Escalate, quietly"
	updated, err := UpdateRule{Writer: h.writer}.invoke(context.Background(), writerActor(),
		usecase.Input{
			"rule_id": id, "name": renamed,
			"trigger":  map[string]any{"kind": "MANUAL"},
			"actions":  []any{map[string]any{"kind": "ADD_LABEL"}},
			"throttle": map[string]any{"max_runs_per_hour": 10},
			"on_error": "RETRY",
		})
	if err != nil {
		t.Fatalf("editing: %v", err)
	}
	if updated.String("name") != renamed {
		t.Errorf("name %q, want %q", updated.String("name"), renamed)
	}

	enabled, err := EnableRule{Writer: h.writer}.invoke(
		context.Background(), writerActor(), usecase.Input{"rule_id": id})
	if err != nil {
		t.Fatalf("enabling: %v", err)
	}
	if enabled["enabled"] != true {
		t.Error("the rule is still off")
	}

	disabled, err := DisableRule{Writer: h.writer}.invoke(
		context.Background(), writerActor(), usecase.Input{"rule_id": id})
	if err != nil {
		t.Fatalf("disabling: %v", err)
	}
	if disabled["enabled"] != false {
		t.Error("the rule is still on")
	}

	listed, err := ListRules{Writer: h.writer}.invoke(
		context.Background(), writerActor(), usecase.Input{"enabled": false, "size": 10})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("%d rules listed, want one", len(rows))
	}
	if _, present := listed["page"]; !present {
		t.Error("the paging block api-guidelines.md §4 declares is missing")
	}

	if _, err := (DeleteRule{Writer: h.writer}).invoke(
		context.Background(), writerActor(), usecase.Input{"rule_id": id}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if _, err := (GetRule{Writer: h.writer}).invoke(
		context.Background(), writerActor(), usecase.Input{"rule_id": id},
	); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a deleted rule is still found: %v", err)
	}
}

// An identifier that is not one is the caller's mistake, and every operation says so the same way.
func TestAMalformedIdentifierIsRefused(t *testing.T) {
	h := permittedHarness()

	in := usecase.Input{"rule_id": "not-a-uuid"}
	for name, call := range map[string]func() error{
		"get": func() error {
			_, err := (GetRule{Writer: h.writer}).invoke(context.Background(), writerActor(), in)
			return err
		},
		"update": func() error {
			_, err := (UpdateRule{Writer: h.writer}).invoke(context.Background(), writerActor(), in)
			return err
		},
		"enable": func() error {
			_, err := (EnableRule{Writer: h.writer}).invoke(context.Background(), writerActor(), in)
			return err
		},
		"disable": func() error {
			_, err := (DisableRule{Writer: h.writer}).invoke(context.Background(), writerActor(), in)
			return err
		},
		"delete": func() error {
			_, err := (DeleteRule{Writer: h.writer}).invoke(context.Background(), writerActor(), in)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, shared.ErrValidation) {
				t.Errorf("error %v, want ErrValidation", err)
			}
		})
	}
}

// The catalogue entries themselves: the parity gate reads them, and so does the manifest.
func TestEveryDescriptorDeclaresItsScopeAndItsAudit(t *testing.T) {
	descriptors := []usecase.Descriptor{
		CreateRule{}.Descriptor(), GetRule{}.Descriptor(), ListRules{}.Descriptor(),
		UpdateRule{}.Descriptor(), EnableRule{}.Descriptor(), DisableRule{}.Descriptor(),
		DeleteRule{}.Descriptor(),
	}
	for _, descriptor := range descriptors {
		t.Run(descriptor.Name, func(t *testing.T) {
			if descriptor.TokenScope != automationScope {
				t.Errorf("scope %q, want %q", descriptor.TokenScope, automationScope)
			}
			if descriptor.Audit.Action == "" || descriptor.Audit.TargetType != ruleTarget {
				t.Errorf("audit declaration %+v", descriptor.Audit)
			}
			if descriptor.Handler == nil {
				t.Error("no handler")
			}
			// A write declares its obligation rather than leaving it to a judgement (gate SG-13).
			if !descriptor.ReadOnly && !descriptor.Audit.Required {
				t.Error("a write declares no required audit entry")
			}
		})
	}
}
