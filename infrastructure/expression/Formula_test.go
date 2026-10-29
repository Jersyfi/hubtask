// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression_test

import (
	"context"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/infrastructure/expression"
)

type formulaEntry map[string]any

func (e formulaEntry) Resolve(_ context.Context, name string) (any, bool, error) {
	if name != "entry" {
		return nil, false, nil
	}
	return map[string]any{"custom_fields": map[string]any(e)}, true, nil
}

// A CALCULATED field's formula against the real adapter (UC-WRK-12): it reads the number fields
// of its scope, a key that is not one is refused when it is written, and an empty input is an
// evaluation that cannot answer rather than a zero.
func TestAFormulaReadsTheNumberFieldsOfItsScope(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The shape work.FormulaEnvironment gives a formula, written out: an adapter's test does not
	// reach into the application layer (project-structure.md §2).
	environment := port.Environment{Variables: []port.Variable{
		{Name: "entry", Kind: port.KindMap, Fields: []port.Variable{
			{Name: "custom_fields", Kind: port.KindMap, Fields: []port.Variable{
				{Name: "hours", Kind: port.KindString}, {Name: "rate", Kind: port.KindString},
			}},
		}},
	}}

	program, err := expression.New().Compile("entry.custom_fields.hours * entry.custom_fields.rate", environment, port.Number)
	if err != nil {
		t.Fatalf("a formula over two number fields was refused: %v", err)
	}
	value, err := program.Evaluate(ctx, formulaEntry{"hours": 2.5, "rate": 80.0})
	if err != nil || value.Number != 200 {
		t.Fatalf("2.5 h at 80 answered %v, %v", value.Number, err)
	}
	if _, err := program.Evaluate(ctx, formulaEntry{"hours": 2.5}); err == nil {
		t.Error("a formula with an empty input answered")
	}
	if _, err := expression.New().Compile("entry.custom_fields.title * 2.0", environment, port.Number); err == nil {
		t.Error("a formula reading a field that is not a number of its scope compiled")
	}
	if _, err := expression.New().Compile(`"a" + "b"`, environment, port.Number); err == nil {
		t.Error("a formula producing text compiled")
	}
}

// A whole number meets a number field in a formula's arithmetic and comparison: `hours * 2` and
// `hours > 1` answer as `hours * 2.0` would, on the left as on the right.
func TestAFormulaMixesWholeNumbersWithNumberFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	environment := port.Environment{Variables: []port.Variable{
		{Name: "entry", Kind: port.KindMap, Fields: []port.Variable{
			{Name: "custom_fields", Kind: port.KindMap, Fields: []port.Variable{{Name: "hours", Kind: port.KindString}}},
		}},
	}}
	for text, want := range map[string]float64{
		"entry.custom_fields.hours * 2":           5,
		"2 * entry.custom_fields.hours":           5,
		"entry.custom_fields.hours + 1":           3.5,
		"10 - entry.custom_fields.hours":          7.5,
		"entry.custom_fields.hours / 2":           1.25,
		"entry.custom_fields.hours > 1 ? 1 : 0":   1,
		"(entry.custom_fields.hours * 2 + 1) / 3": 2,
	} {
		program, err := expression.New().Compile(text, environment, port.Number)
		if err != nil {
			t.Fatalf("%q was refused: %v", text, err)
		}
		value, err := program.Evaluate(ctx, formulaEntry{"hours": 2.5})
		if err != nil || value.Number != want {
			t.Fatalf("%q answered %v, %v; want %v", text, value.Number, err, want)
		}
	}
}
