// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func handler(calls *int) Handler {
	return HandlerFunc(func(context.Context, appshared.ActorContext, Input) (Output, error) {
		*calls++
		return Output{"ok": true}, nil
	})
}

func descriptor(name string, calls *int) Descriptor {
	return Descriptor{
		Name:       name,
		Summary:    "Creates something.",
		TokenScope: "containers:write",
		Input: []Field{
			{Name: "type", Kind: KindString, Required: true, Enum: []string{"HUB", "COLLECTION"}},
			{Name: "name", Kind: KindString, Required: true},
			{Name: "parent_id", Kind: KindID},
			{Name: "pinned", Kind: KindBool},
			{Name: "position", Kind: KindInt},
		},
		Audit:   AuditDeclaration{Action: "container.created", TargetType: "container", Required: true},
		Handler: handler(calls),
	}
}

// One name, three channel identities, derived rather than declared - which is what makes the
// parity test compare one list with itself rather than two lists that were never the same.
func TestTheChannelIdentitiesAreDerivedFromTheName(t *testing.T) {
	cases := map[string]struct{ rest, mcp, automation string }{
		"CreateContainer": {"createContainer", "create_container", "CREATE_CONTAINER"},
		"QueryItems":      {"queryItems", "query_items", "QUERY_ITEMS"},
		"SetDueDate":      {"setDueDate", "set_due_date", "SET_DUE_DATE"},
		"ListRuleRuns":    {"listRuleRuns", "list_rule_runs", "LIST_RULE_RUNS"},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			d := Descriptor{Name: name}
			if d.RESTOperation() != want.rest {
				t.Errorf("REST operation %s, want %s", d.RESTOperation(), want.rest)
			}
			if d.MCPTool() != want.mcp {
				t.Errorf("MCP tool %s, want %s", d.MCPTool(), want.mcp)
			}
			if d.AutomationAction() != want.automation {
				t.Errorf("automation action %s, want %s", d.AutomationAction(), want.automation)
			}
		})
	}
}

func TestTheRegistryFindsAnEntryThroughEveryChannel(t *testing.T) {
	calls := 0
	registry, err := NewRegistry(nil, descriptor("CreateContainer", &calls))
	if err != nil {
		t.Fatalf("the catalogue was refused: %v", err)
	}

	if _, found := registry.Lookup("CreateContainer"); !found {
		t.Error("the use case is not in the catalogue under its own name")
	}
	if _, found := registry.ByMCPTool("create_container"); !found {
		t.Error("no MCP tool for the use case")
	}
	if _, found := registry.ByAutomationAction("CREATE_CONTAINER"); !found {
		t.Error("no automation action for the use case")
	}
	if _, found := registry.ByMCPTool("create_collection"); found {
		t.Error("a tool nobody registered was found")
	}
}

// The middleware is applied to every entry on the way in, which is what makes the metric and the
// span structural rather than remembered (gate RT-12).
func TestEveryHandlerGoesThroughTheMiddleware(t *testing.T) {
	calls, observed := 0, []string{}
	observe := func(d Descriptor, next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in Input) (Output, error) {
			observed = append(observed, d.Name)
			return next.Invoke(ctx, actor, in)
		})
	}

	registry, err := NewRegistry(observe,
		descriptor("CreateContainer", &calls), descriptor("RenameContainer", &calls))
	if err != nil {
		t.Fatalf("the catalogue was refused: %v", err)
	}

	for _, name := range []string{"CreateContainer", "RenameContainer"} {
		if _, err := registry.Invoke(context.Background(), name, appshared.ActorContext{},
			Input{"type": "HUB", "name": "Private"}); err != nil {
			t.Fatalf("%s failed: %v", name, err)
		}
	}
	if !slices.Equal(observed, []string{"CreateContainer", "RenameContainer"}) {
		t.Errorf("not every invocation was observed: %v", observed)
	}
	if calls != 2 {
		t.Errorf("%d handler calls, want 2", calls)
	}
}

// Startup is where an incomplete entry has to fail: later means whoever needed the audit entry
// finds out instead (fail closed).
func TestAnIncompleteRegistrationIsRefusedAtStartup(t *testing.T) {
	calls := 0
	cases := map[string]func(*Descriptor){
		"without a name":                         func(d *Descriptor) { d.Name = "" },
		"with a name in the wrong case":          func(d *Descriptor) { d.Name = "create_container" },
		"without a summary":                      func(d *Descriptor) { d.Summary = " " },
		"without a handler":                      func(d *Descriptor) { d.Handler = nil },
		"with an audit obligation and no action": func(d *Descriptor) { d.Audit.Action = "" },
		"with an audit obligation and no target": func(d *Descriptor) { d.Audit.TargetType = "" },
		"with two fields of the same name": func(d *Descriptor) {
			d.Input = append(d.Input, Field{Name: "name", Kind: KindString})
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			broken := descriptor("CreateContainer", &calls)
			mutate(&broken)

			if _, err := NewRegistry(nil, broken); !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("error %v, want the registration to be refused", err)
			}
		})
	}

	t.Run("twice under the same name", func(t *testing.T) {
		_, err := NewRegistry(nil, descriptor("CreateContainer", &calls), descriptor("CreateContainer", &calls))
		if err == nil || shared.AsError(err).DetailCode != "usecase.duplicate_registration" {
			t.Errorf("error %v, want a duplicate registration", err)
		}
	})

	// A use case with no audit obligation needs no declaration - not every operation is evidence.
	t.Run("without an audit obligation", func(t *testing.T) {
		quiet := descriptor("QueryItems", &calls)
		quiet.Audit = AuditDeclaration{}
		if _, err := NewRegistry(nil, quiet); err != nil {
			t.Errorf("a use case without an audit obligation was refused: %v", err)
		}
	})
}

func TestAnUnknownUseCaseIsNotFoundRatherThanADefect(t *testing.T) {
	registry, err := NewRegistry(nil)
	if err != nil {
		t.Fatalf("an empty catalogue was refused: %v", err)
	}

	_, err = registry.Invoke(context.Background(), "DeleteEverything", appshared.ActorContext{}, nil)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error %v, want not found", err)
	}
}

// The input is checked once, in the registry, so a handler can trust its shape whichever channel
// the call arrived through.
func TestTheInputIsCheckedAgainstTheDeclaration(t *testing.T) {
	calls := 0
	registry, err := NewRegistry(nil, descriptor("CreateContainer", &calls))
	if err != nil {
		t.Fatalf("the catalogue was refused: %v", err)
	}

	cases := map[string]struct {
		in    Input
		path  string
		code  string
		valid bool
	}{
		"complete":                            {in: Input{"type": "HUB", "name": "Private"}, valid: true},
		"a missing required field":            {in: Input{"type": "HUB"}, path: "/name", code: "usecase.field_required"},
		"an empty required field":             {in: Input{"type": "HUB", "name": "  "}, path: "/name", code: "usecase.field_required"},
		"a value outside the enum":            {in: Input{"type": "PROJECT", "name": "x"}, path: "/type", code: "usecase.field_not_in_enum"},
		"a misspelled field":                  {in: Input{"type": "HUB", "name": "x", "parentid": "y"}, path: "/parentid", code: "usecase.field_unknown"},
		"a string where a boolean belongs":    {in: Input{"type": "HUB", "name": "x", "pinned": "yes"}, path: "/pinned", code: "usecase.field_type_invalid"},
		"a fraction where an integer belongs": {in: Input{"type": "HUB", "name": "x", "position": 1.5}, path: "/position", code: "usecase.field_type_invalid"},
		"a number where a string belongs":     {in: Input{"type": "HUB", "name": 7}, path: "/name", code: "usecase.field_type_invalid"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := registry.Invoke(context.Background(), "CreateContainer", appshared.ActorContext{}, c.in)
			if c.valid {
				if err != nil {
					t.Fatalf("a valid input was refused: %v", err)
				}
				return
			}
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want a validation error", err)
			}
			fields := shared.AsError(err).Fields
			if len(fields) != 1 || fields[0].Path != c.path || fields[0].Code != c.code {
				t.Errorf("findings %+v, want %s at %s", fields, c.code, c.path)
			}
		})
	}
}

// A whole number that arrived as a JSON float is still a whole number.
func TestAJSONNumberIsAnInteger(t *testing.T) {
	calls := 0
	registry, _ := NewRegistry(nil, descriptor("CreateContainer", &calls))

	if _, err := registry.Invoke(context.Background(), "CreateContainer", appshared.ActorContext{},
		Input{"type": "HUB", "name": "x", "position": float64(3)}); err != nil {
		t.Errorf("a whole number from JSON was refused: %v", err)
	}
}

func TestInputAccessors(t *testing.T) {
	in := Input{
		"name": "  Private  ", "parent_id": "0192f000-0000-7000-8000-00000000000b",
		"pinned": true, "position": float64(4), "broken_id": "not-a-uuid",
	}

	if in.String("name") != "Private" {
		t.Errorf("a string field is not trimmed: %q", in.String("name"))
	}
	if in.String("absent") != "" || in.Bool("absent") || in.Int("absent") != 0 {
		t.Error("an absent field is not the zero value")
	}
	if id, err := in.ID("parent_id"); err != nil || id.IsZero() {
		t.Errorf("an identifier was not read: %v %v", id, err)
	}
	if id, err := in.ID("absent"); err != nil || !id.IsZero() {
		t.Errorf("an absent identifier is not zero: %v %v", id, err)
	}
	if _, err := in.ID("broken_id"); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a malformed identifier was accepted: %v", err)
	}
	if !in.Bool("pinned") || in.Int("position") != 4 {
		t.Error("a boolean or a number was not read")
	}
}

// Two identical requests have to produce identical answers, so the findings are ordered.
func TestFindingsAreOrdered(t *testing.T) {
	calls := 0
	registry, _ := NewRegistry(nil, descriptor("CreateContainer", &calls))

	_, err := registry.Invoke(context.Background(), "CreateContainer", appshared.ActorContext{},
		Input{"zebra": 1, "alpha": 2})
	fields := shared.AsError(err).Fields

	paths := make([]string, 0, len(fields))
	for _, field := range fields {
		paths = append(paths, field.Path)
	}
	if !slices.IsSorted(paths) {
		t.Errorf("the findings are not in a stable order: %v", paths)
	}
}
