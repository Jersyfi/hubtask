// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A door answers through the generated type, so a member the use case leaves null must stay null:
// a workspace goal has no hub and an ungraded goal no grade, and a zero UUID or a 0 % grade read
// back would be a hub nobody can open and a grade nobody gave.
func TestAGoalAnswersItsNullMembersAsNull(t *testing.T) {
	out := usecase.Output{
		"id": "01a0e2e0-0000-7000-8000-000000000001", "period_id": "01a0e2e0-0000-7000-8000-000000000002",
		"hub_id": nil, "parent_id": nil, "owner_id": nil, "title": "t", "description": "",
		"status": "NONE", "confidence": nil, "grade": nil, "progress": nil, "key_results": []any{},
		"entries": map[string]any{"total": 0, "done": 0}, "child_count": 0, "version": 1,
	}
	var answer openapi.Goal
	if err := remarshal(out, &answer); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	var read map[string]any
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{"hub_id", "parent_id", "owner_id", "confidence", "grade", "progress"} {
		value, present := read[member]
		if !present || value != nil {
			t.Errorf("%s reads %v, want null", member, value)
		}
	}
}

// OpenAPI 3.1 has no `nullable` keyword: the generator ignores it and answers a null as the
// type's zero value. A null is spelled in the type (`type: [string, "null"]`) or as an `anyOf`.
func TestTheContractSpellsNullInTheType(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if found := regexp.MustCompile(`(?m)(^\s*|[{,]\s*)nullable:\s*true`).FindAll(raw, -1); len(found) > 0 {
		t.Errorf("the contract uses the keyword nullable %d times, which OpenAPI 3.1 ignores", len(found))
	}
}
