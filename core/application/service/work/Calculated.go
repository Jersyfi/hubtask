// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// A CALCULATED field's formula (domain-model.md §3.5): compiled against the fields it may read when
// the field is defined, and evaluated whenever a writer changes an entry's values.

// FormulaEnvironment is what a formula may read: `entry.custom_fields`, typed to the keys of the
// number and boolean fields in scope (IsFormulaInput), so a key that is not one is refused when the
// formula is written. Without any, the map declares a name no key can have and every selection is
// refused.
func FormulaEnvironment(inputs []string) expression.Environment {
	custom := make([]expression.Variable, 0, len(inputs))
	for _, key := range inputs {
		custom = append(custom, expression.Variable{Name: key, Kind: expression.KindString})
	}
	if len(custom) == 0 {
		custom = []expression.Variable{{Name: "\x00", Kind: expression.KindString}}
	}
	return expression.Environment{Variables: []expression.Variable{
		{Name: "entry", Kind: expression.KindMap, Description: "The entry's own values.", Fields: []expression.Variable{
			{Name: "custom_fields", Kind: expression.KindMap, Fields: custom},
		}},
	}}
}

// formulaInputs are the keys among the definitions a formula may read.
func formulaInputs(definitions []domain.CustomFieldDefinition) []string {
	var keys []string
	for _, definition := range definitions {
		if definition.Kind.IsFormulaInput() {
			keys = append(keys, definition.Key)
		}
	}
	return keys
}

// compileFormula holds a CALCULATED definition's formula to what it may read and to a number.
func compileFormula(compiler expression.Compiler, formula string, inScope []domain.CustomFieldDefinition) error {
	if compiler == nil {
		return shared.ErrInternal.WithDetail("fields.compiler_unwired")
	}
	_, err := compiler.Compile(formula, FormulaEnvironment(formulaInputs(inScope)), expression.Number)
	if err == nil {
		return nil
	}
	var coded *shared.Error
	params := map[string]string{}
	if errors.As(err, &coded) {
		for key, value := range coded.Params {
			params[key] = value
		}
		if coded.DetailCode != "" {
			params["reason"] = coded.DetailCode
		}
	}
	return shared.ErrValidation.WithDetail("fields.formula_invalid").WithParams(params).
		WithFields(shared.FieldError{Path: "/formula", Code: "fields.formula_invalid", Params: params})
}

// calculation is one CALCULATED field's value as the entry's values now stand.
type calculation struct {
	definition domain.CustomFieldDefinition
	value      any
}

// calculate answers every CALCULATED field the entry's type carries, with the value its formula
// gives over the entry's values now. A formula that cannot answer - a field it reads is empty, a
// division by zero - gives no value, never a refusal: the write that changed an input is the
// person's, and a formula is not theirs to be refused by.
func calculate(
	ctx context.Context, compiler expression.Compiler, inScope []domain.CustomFieldDefinition, item domain.WorkItem,
) []calculation {
	if compiler == nil {
		return nil
	}
	inputs := formulaInputs(inScope)
	values := make(map[string]any, len(inputs))
	for _, key := range inputs {
		if value, held := item.CustomFields[key]; held && value != nil {
			values[key] = formulaValue(value)
		}
	}
	activation := formulaActivation{entry: map[string]any{"custom_fields": values}}
	environment := FormulaEnvironment(inputs)

	var calculations []calculation
	for _, definition := range inScope {
		if definition.Kind != domain.CustomFieldCalculated || !definition.Carries(item.Type) {
			continue
		}
		program, err := compiler.Compile(definition.Formula, environment, expression.Number)
		if err != nil {
			// A field the formula read was deleted since: it cannot answer any more.
			calculations = append(calculations, calculation{definition: definition})
			continue
		}
		result, err := program.Evaluate(ctx, activation)
		calculations = append(calculations, calculation{
			definition: definition, value: domain.CalculatedValue(result.Number, err == nil),
		})
	}
	return calculations
}

// formulaValue is one input as a formula reads it: every number a double, so that two fields of
// one formula meet in one arithmetic whatever the document decoded them as.
func formulaValue(value any) any {
	switch number := value.(type) {
	case int:
		return float64(number)
	case int64:
		return float64(number)
	case int32:
		return float64(number)
	case float32:
		return float64(number)
	}
	return value
}

// formulaActivation answers the one name a formula reads.
type formulaActivation struct {
	entry map[string]any
}

func (a formulaActivation) Resolve(_ context.Context, name string) (any, bool, error) {
	if name == "entry" {
		return a.entry, true, nil
	}
	return nil, false, nil
}
