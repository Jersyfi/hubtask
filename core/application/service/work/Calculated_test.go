// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// product is a compiler whose every formula multiplies the inputs it reads, and cannot answer
// where one is empty - the calculator's part is choosing fields and values, the adapter's is CEL.
type product struct{ inputs []string }

func (p product) Compile(string, expression.Environment, expression.Result) (expression.Program, error) {
	return p, nil
}

func (p product) Evaluate(ctx context.Context, in expression.Activation) (expression.Value, error) {
	entry, _, _ := in.Resolve(ctx, "entry")
	values := entry.(map[string]any)["custom_fields"].(map[string]any)
	result := 1.0
	for _, key := range p.inputs {
		number, held := values[key].(float64)
		if !held {
			return expression.Value{}, errors.New("no such key")
		}
		result *= number
	}
	return expression.Value{Number: result}, nil
}

func definitionOf(key string, kind domain.CustomFieldKind, collection shared.ID, types ...domain.ItemType) domain.CustomFieldDefinition {
	if len(types) == 0 {
		types = []domain.ItemType{domain.ItemTask}
	}
	definition := domain.CustomFieldDefinition{Key: key, Kind: kind, CollectionID: collection, AppliesTo: types}
	if kind == domain.CustomFieldCalculated {
		definition.Formula = "entry.custom_fields.hours * entry.custom_fields.rate"
	}
	return definition
}

// UC-WRK-12: a CALCULATED field takes the value its formula gives over the entry's number fields,
// none where an input is empty, and only on the types it applies to.
func TestACalculatedFieldFollowsItsInputs(t *testing.T) {
	ctx := context.Background()
	compiler := product{inputs: []string{"hours", "rate"}}
	inScope := []domain.CustomFieldDefinition{
		definitionOf("hours", domain.CustomFieldNumber, ""),
		definitionOf("rate", domain.CustomFieldMoney, ""),
		definitionOf("cost", domain.CustomFieldCalculated, ""),
		definitionOf("effort", domain.CustomFieldCalculated, "", domain.ItemActivity),
	}
	if got := formulaInputs(inScope); len(got) != 2 {
		t.Fatalf("the inputs are %v", got)
	}

	item := domain.WorkItem{Type: domain.ItemTask, CustomFields: map[string]any{"hours": 3, "rate": 12.5}}
	calculations := calculate(ctx, compiler, inScope, item)
	if len(calculations) != 1 || calculations[0].definition.Key != "cost" || calculations[0].value != 37.5 {
		t.Fatalf("calculated %+v", calculations)
	}
	item.CustomFields = map[string]any{"hours": 3.0}
	if calculations := calculate(ctx, compiler, inScope, item); calculations[0].value != nil {
		t.Errorf("an empty input gave %v", calculations[0].value)
	}
	if calculate(ctx, nil, inScope, item) != nil {
		t.Error("a calculator without a compiler calculated")
	}
}

// The definition an entry is judged by is its collection's own over the workspace's (FindInScope).
func TestACollectionsOwnDefinitionReplacesTheWorkspaces(t *testing.T) {
	collection := shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	effective := effectiveDefinitions([]domain.CustomFieldDefinition{
		definitionOf("hours", domain.CustomFieldText, ""),
		definitionOf("rate", domain.CustomFieldNumber, ""),
		definitionOf("hours", domain.CustomFieldNumber, collection),
	})
	if len(effective) != 2 || effective[0].Kind != domain.CustomFieldNumber || effective[0].CollectionID != collection {
		t.Fatalf("effective %+v", effective)
	}
}
