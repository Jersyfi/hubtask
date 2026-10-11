// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-ID-16 check 3 at the REST door: the administrators' list answers each private hub as the
// contract's PrivateHubSummary, and the schema has no field a hub's name or content could travel in.
func TestThePrivateHubListMatchesTheSchemaAndNamesNothing(t *testing.T) {
	spec := contractSpec(t)
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	trashed := created.Add(24 * time.Hour)

	status, raw := readResponse(t, "/private-hubs", catalogue.Output{"data": []catalogue.Output{
		{
			"id": "0192f000-0000-7000-8000-0000000000b1", "owners": []string{"0192f000-0000-7000-8000-00000000000d"},
			"collections": 2, "entries": 14, "attachment_bytes": int64(2048), "created_at": created,
		},
		{
			"id": "0192f000-0000-7000-8000-0000000000b2", "owners": []string{},
			"collections": 1, "entries": 3, "attachment_bytes": int64(0), "created_at": created,
			"trashed_at": trashed, "purge_on": "2026-10-31",
		},
	}})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, raw)
	}

	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("the list is not an array: %s", raw)
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2: %s", len(rows), raw)
	}
	for _, row := range rows {
		problems, err := spec.validateAgainst("PrivateHubSummary", row)
		if err != nil {
			t.Fatalf("%v", err)
		}
		for _, problem := range problems {
			t.Errorf("PrivateHubSummary: %s", problem)
		}
	}

	summary, _ := spec.resolve(spec.Components.Schemas["PrivateHubSummary"])
	for _, forbidden := range []string{"name", "description", "title", "icon"} {
		if _, carried := summary.Properties[forbidden]; carried {
			t.Errorf("PrivateHubSummary has a %q field", forbidden)
		}
	}
}
