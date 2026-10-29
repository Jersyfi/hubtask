// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression_test

import (
	"context"
	"testing"
	"time"

	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
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
	environment := workservice.FormulaEnvironment([]string{"hours", "rate"})

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
