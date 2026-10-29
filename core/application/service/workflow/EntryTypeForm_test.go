// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"context"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// definedFields answers the custom field keys the workspace defines; the rest of the port is not
// asked by designing a form.
type definedFields struct {
	repository.CustomFields
	keys []string
}

func (d definedFields) KindsByKey(_ context.Context, keys []string, _ shared.ID) (map[string][]work.CustomFieldKind, error) {
	out := map[string][]work.CustomFieldKind{}
	for _, key := range keys {
		for _, defined := range d.keys {
			if key == defined {
				out[key] = []work.CustomFieldKind{work.CustomFieldSelect}
			}
		}
	}
	return out, nil
}

// typedCompiler refuses a condition naming a custom field outside the environment it is given:
// the adapter's typed map, reduced to what the use case is responsible for passing.
type typedCompiler struct{}

func (typedCompiler) Compile(text string, env expression.Environment, _ expression.Result) (expression.Program, error) {
	declared := map[string]bool{}
	for _, variable := range env.Variables {
		for _, field := range variable.Fields {
			for _, custom := range field.Fields {
				declared[custom.Name] = true
			}
		}
	}
	if name, found := strings.CutPrefix(text, "entry.custom_fields."); found && !declared[strings.Fields(name)[0]] {
		return nil, shared.ErrValidation.WithDetail(expression.CodeUnknownName)
	}
	return nil, nil
}

// UC-FLW-01 check 2: a type is given its form - fields from the workspace's vocabulary, conditions
// checked in the type's own environment - and answers it on every read.
func TestATypeIsGivenItsForm(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	set := workflow.SetEntryTypeForm{Registry: f.registry, Desk: workflow.FormDesk{
		Fields: definedFields{keys: []string{"severity"}}, Compiler: typedCompiler{},
	}}
	form := map[string]any{
		"sections": []any{map[string]any{"key": "triage", "name": "Triage"}},
		"fields": []any{
			map[string]any{"key": "title"},
			map[string]any{"key": "custom_fields.severity", "section": "triage", "required": true},
			map[string]any{"key": "due_date", "required_when": `entry.custom_fields.severity == "high"`},
		},
		"channels": []any{map[string]any{"channel": "MCP", "closed": true}},
	}
	out, err := set.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{"key": "task", "form": form})
	if err != nil {
		t.Fatalf("a form was refused: %v", err)
	}
	answered := out["form"].(map[string]any)
	if fields := answered["fields"].([]any); len(fields) != 3 {
		t.Fatalf("the form was not answered whole: %v", answered)
	}
	if stored := f.types.rows["task"]; len(stored.Form.Fields) != 3 || !stored.Form.Channel(work.ChannelMCP).Closed {
		t.Fatalf("the system type's form was not stored on its first row: %+v", stored.Form)
	}
	if len(f.trail.entries) == 0 || f.trail.entries[len(f.trail.entries)-1].Action != workflow.EntryTypeFormSetAction {
		t.Fatal("no audit entry for the form")
	}

	unknown := map[string]any{"fields": []any{map[string]any{"key": "custom_fields.colour"}}}
	if _, err := set.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{"key": "task", "form": unknown}); detail(err) != "forms.field_unknown" {
		t.Fatalf("a field nobody defined was accepted: %v", err)
	}
	misspelt := map[string]any{"fields": []any{
		map[string]any{"key": "custom_fields.severity", "hidden_when": `entry.custom_fields.severty == "x"`},
	}}
	if _, err := set.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{"key": "task", "form": misspelt}); detail(err) != expression.CodeUnknownName {
		t.Fatalf("a condition outside the type's environment was accepted: %v", err)
	}
	if _, err := set.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{"key": "task", "form": "x"}); detail(err) != "forms.malformed" {
		t.Fatalf("a form of the wrong shape was accepted: %v", err)
	}
}
