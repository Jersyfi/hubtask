// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"strings"
	"testing"
)

// spec is a minimal specification with one body that references Thing; extra is spliced into
// Thing's properties, and tail after the components.
func spec(extra, tail string) []byte {
	return []byte(`
paths:
  /things:
    parameters: []
    post:
      operationId: createThing
      requestBody:
        content:
          application/json:
            schema: { $ref: "#/components/schemas/Thing" }
components:
  schemas:
    Thing:
      type: object
      properties:
        name: { type: string }
` + extra + tail)
}

const markedField = `        old:
          type: string
          deprecated: true
          x-deprecated-since: "2026-10-03"
          x-removed-in: v2
          x-replaced-by: [renameThing]
`

func TestAMarkedRequestFieldIsInTheTable(t *testing.T) {
	entries, err := table(spec(markedField, ""))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(entries) != 1 || entries[0].OperationID != "createThing" || entries[0].Field != "old" ||
		entries[0].Method != "POST" || entries[0].ReplacedBy[0] != "renameThing" {
		t.Fatalf("the table is %+v", entries)
	}
}

// A mark the table cannot carry stops the generator: in the specification and nowhere else, it
// would be a deprecation nobody is told about.
func TestAMarkNothingCanAnnounceIsRefused(t *testing.T) {
	cases := map[string][]byte{
		"a response-only schema": spec(markedField, `    Answer:
      type: object
      properties:
        legacy: { type: string, deprecated: true }
`),
		"a nested member": spec(`        inner:
          type: object
          properties:
            deep: { type: string, deprecated: true }
`, ""),
		"a deprecated operation": []byte(strings.Replace(string(spec("", "")),
			"operationId: createThing", "operationId: createThing\n      deprecated: true", 1)),
	}
	for name, raw := range cases {
		if _, err := table(raw); err == nil || !strings.Contains(err.Error(), "nothing can announce it") {
			t.Errorf("%s: answered %v, want the unannounceable mark refused", name, err)
		}
	}
}

func TestAnIncompleteMarkIsRefused(t *testing.T) {
	for name, broken := range map[string]string{
		"no day":              strings.Replace(markedField, `x-deprecated-since: "2026-10-03"`, `x-deprecated-since: ""`, 1),
		"a day that isn't":    strings.Replace(markedField, `"2026-10-03"`, `"3 October"`, 1),
		"no version":          strings.Replace(markedField, "x-removed-in: v2", "x-removed-in: soon", 1),
		"nothing instead":     strings.Replace(markedField, "x-replaced-by: [renameThing]", "x-replaced-by: []", 1),
		"a sunset that isn't": markedField + "          x-sunset: next spring\n",
	} {
		if _, err := table(spec(broken, "")); err == nil {
			t.Errorf("%s: an incomplete mark was accepted", name)
		}
	}
}

// The real specification reads, and every row it gives is markedField - what make generate writes.
func TestTheContractReads(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	entries, err := table(raw)
	if err != nil {
		t.Fatalf("the contract: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the contract declares no deprecated field - the reading is broken")
	}
}
