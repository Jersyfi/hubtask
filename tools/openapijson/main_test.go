// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestConvertKeepsOrderAndTypes(t *testing.T) {
	in := `
openapi: 3.1.0
paths:
  /b:
    get:
      responses:
        "404": { description: gone }
        "200": { description: ok }
  /a: {}
components:
  schemas:
    Thing:
      type: object
      required: [id]
      properties:
        id: { type: string, example: "0001" }
        count: { type: integer, default: 3 }
        ratio: { type: number, example: 0.5 }
        on: { type: boolean, default: true }
        none: { type: [string, "null"], example: null }
        note: |
          two
          lines <b>
`
	out, err := Convert([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	// Order: /b before /a, 404 before 200, exactly as written.
	if strings.Index(text, `"/b"`) > strings.Index(text, `"/a"`) {
		t.Errorf("path order not preserved:\n%s", text)
	}
	if strings.Index(text, `"404"`) > strings.Index(text, `"200"`) {
		t.Errorf("response order not preserved:\n%s", text)
	}
	// Types: the quoted "0001" stays a string, 3 an integer, 0.5 a float, true a bool, null null.
	for _, want := range []string{`"example": "0001"`, `"default": 3`, `"example": 0.5`, `"default": true`, `"example": null`, `"note": "two\nlines <b>\n"`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in:\n%s", want, text)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
}

func TestConvertTheRealSpecification(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Skip("no specification beside the tool")
	}
	out, err := Convert(raw)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if parsed.OpenAPI == "" || len(parsed.Paths) == 0 {
		t.Fatalf("the document lost its shape: %+v", parsed)
	}
}

func TestConvertRefusesMergeKeys(t *testing.T) {
	if _, err := Convert([]byte("base: &b { a: 1 }\nx:\n  <<: *b\n")); err == nil {
		t.Fatal("a merge key should be refused rather than expanded silently")
	}
}
