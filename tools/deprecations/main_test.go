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
	if len(entries) != 1 || entries[0].OperationID != "createThing" || entries[0].Field != "old" || entries[0].In != inBody ||
		entries[0].Method != "POST" || entries[0].ReplacedBy[0] != "renameThing" {
		t.Fatalf("the table is %+v", entries)
	}
}

const markOf = `
      deprecated: true
      x-deprecated-since: "2026-11-05"
      x-removed-in: v2
      x-replaced-by: [listThings]`

// A whole operation and a query parameter written in it are announced too: the first on every
// request to it, the second when a request names it.
func TestAMarkedOperationAndQueryParameterAreInTheTable(t *testing.T) {
	raw := strings.Replace(string(spec("", "")), "operationId: createThing", "operationId: createThing"+markOf+`
      parameters:
        - name: trashed
          in: query
          schema: { type: boolean }`+strings.ReplaceAll(markOf, "\n      ", "\n          "), 1)
	entries, err := table([]byte(raw))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(entries) != 2 || entries[0].In != inOperation || entries[0].Field != "" ||
		entries[1].In != inQuery || entries[1].Field != "trashed" || entries[1].ReplacedBy[0] != "listThings" {
		t.Fatalf("the table is %+v", entries)
	}
	if _, err := table([]byte(strings.Replace(raw, `x-removed-in: v2`, `x-removed-in: later`, 1))); err == nil {
		t.Error("an operation's incomplete mark was accepted")
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
		"a path parameter": []byte(strings.Replace(string(spec("", "")),
			"operationId: createThing", "operationId: createThing\n      parameters:\n        - { name: id, in: path, deprecated: true }", 1)),
		"a shared parameter": []byte(strings.Replace(string(spec("", "")),
			"    parameters: []", "    parameters:\n      - { name: old, in: query, deprecated: true }", 1)),
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
