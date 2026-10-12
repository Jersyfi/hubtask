// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// The workspace's AI source over REST: the choice travels in, and an offered model comes back by
// its name, processor and jurisdiction - the mapper has no field for anything more.
func TestTheAiSourceIsChosenAndAnsweredOverRest(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"source": "OFFERED", "offered_model_id": "0192f000-0000-7000-8000-0000000000e1",
		"sources": "EITHER", "installation_sources": "EITHER", "sources_origin": "INSTANCE",
		"min_jurisdiction": "EEA", "min_jurisdiction_origin": "INSTANCE", "min_jurisdiction_lock": "INSTANCE",
		"version": 2,
		"offered": []usecase.Output{{
			"id": "0192f000-0000-7000-8000-0000000000e1", "display_name": "Large (EU)",
			"processor": "A processor", "jurisdiction": "EEA",
			"base_url": "https://must-not-appear.example",
		}},
	}}

	recorder := callBackup(t, registry, http.MethodPut, "/ai-source",
		`{"source": "OFFERED", "offered_model_id": "0192f000-0000-7000-8000-0000000000e1", "sources": "EITHER"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ChooseAiSource" || registry.in["source"] != "OFFERED" || registry.in["sources"] != "EITHER" ||
		registry.in["offered_model_id"] != "0192f000-0000-7000-8000-0000000000e1" {
		t.Errorf("the use case received %s %v", registry.name, registry.in)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	offered, _ := body["offered"].([]any)
	first, _ := offered[0].(map[string]any)
	if first["display_name"] != "Large (EU)" || len(first) != 4 {
		t.Errorf("the offered model is answered as %v", first)
	}
	if body["min_jurisdiction_lock"] != "INSTANCE" || body["source"] != "OFFERED" {
		t.Errorf("answered %v", body)
	}
}
