// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-PRV-05 check 12 at the REST door: the consent register answers who keeps their content out as
// the contract's ConsentRegisterEntry - and no other schema carries a person's objection. A person's
// own view (OwnAiPosition) carries their own choice; the workspace's AI source a count, never who.
func TestTheRegisterIsTheOnePlaceThatNamesWhoKeepsTheirContentOut(t *testing.T) {
	spec := contractSpec(t)
	since := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

	status, raw := readResponse(t, "/privacy/consents?purpose=ai_processing", catalogue.Output{"items": []catalogue.Output{
		{"account_id": "0192f000-0000-7000-8000-0000000000b1", "display_name": "Kim Keeper", "since": since, "source": "user"},
	}})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, raw)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || len(body.Data) != 1 {
		t.Fatalf("the register is not {data: [one]}: %s", raw)
	}
	problems, err := spec.validateAgainst("ConsentRegisterEntry", body.Data[0])
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("ConsentRegisterEntry: %s", problem)
	}

	allowed := map[string]bool{"OwnAiPosition": true, "ConsentRegisterEntry": true}
	for name, schema := range spec.Components.Schemas {
		if allowed[name] {
			continue
		}
		resolved, _ := spec.resolve(schema)
		for property := range resolved.Properties {
			if property == "keeping_out_count" {
				continue // a number, never who
			}
			if strings.Contains(property, "keeping_out") || strings.Contains(property, "objection") ||
				strings.Contains(property, "objects") || strings.Contains(property, "kept_out") {
				t.Errorf("%s carries %q: the register is the one place a person's objection is listed", name, property)
			}
		}
	}
}
