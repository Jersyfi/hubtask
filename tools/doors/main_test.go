// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const contract = `
x-hubtask-cli-groups:
  thing: "the things"
paths:
  /things/{thingId}:
    parameters:
      - name: thingId
        in: path
        required: true
        schema: { type: string, format: uuid }
    patch:
      operationId: changeThing
      x-hubtask-cli: "thing set"
      parameters:
        - $ref: "#/components/parameters/IfMatch"
        - name: dry_run
          in: query
          schema: { type: boolean }
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: "#/components/schemas/ThingChange" }
      responses:
        "200":
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Thing" }
  /things:
    get:
      operationId: listThings
      x-hubtask-cli: "thing ls"
      responses:
        "200":
          content:
            application/json:
              schema:
                type: object
                properties:
                  data: { type: array, items: { $ref: "#/components/schemas/Thing" } }
    post:
      operationId: createThing
components:
  parameters:
    IfMatch: { name: If-Match, in: header, schema: { type: string } }
  schemas:
    ThingChange:
      type: object
      required: [name]
      properties:
        name: { type: string, description: "What it is called. Unique." }
        read_only: { type: [boolean, "null"] }
        tags: { type: array, items: { type: string } }
        shape: { type: object }
    Thing:
      type: object
      properties:
        id: { type: string, format: uuid }
        name: { type: string }
        tags: { type: array, items: { type: string } }
`

// Only an operation that names a command gets one, and every part of it is read from the
// operation: the path's parameters, the query, the body's members, If-Match and the columns.
func TestACommandIsReadFromTheOperationThatNamesIt(t *testing.T) {
	commands, groups, err := table([]byte(contract))
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("%d commands, want 2 (createThing names none)", len(commands))
	}
	list, set := commands[0], commands[1]
	if list.Verb != "ls" || !list.List || strings.Join(list.Columns, ",") != "id,name" {
		t.Errorf("the list is %+v", list)
	}
	if set.Method != "PATCH" || set.Path != "/things/{thingId}" || !set.IfMatch || !set.BodyRequired {
		t.Errorf("the change is %+v", set)
	}
	if len(set.PathParams) != 1 || set.PathParams[0].Kind != "uuid" {
		t.Errorf("the path parameters are %+v", set.PathParams)
	}
	if len(set.Query) != 1 || set.Query[0].Kind != "boolean" {
		t.Errorf("the query is %+v", set.Query)
	}
	kinds := map[string]string{}
	for _, p := range set.Body {
		kinds[p.Name] = p.Kind
	}
	want := map[string]string{"name": "string", "read_only": "boolean", "tags": "strings", "shape": "json"}
	for name, kind := range want {
		if kinds[name] != kind {
			t.Errorf("%s is %q, want %q", name, kinds[name], kind)
		}
	}
	if set.Body[0].Name != "name" || !set.Body[0].Required || set.Body[0].Help != "What it is called" {
		t.Errorf("the first member is %+v", set.Body[0])
	}
	if groups["thing"] != "the things" {
		t.Errorf("the group's summary is %q", groups["thing"])
	}
}

func TestACommandNamedTwiceFails(t *testing.T) {
	twice := strings.Replace(contract, `x-hubtask-cli: "thing ls"`, `x-hubtask-cli: "thing set"`, 1)
	if _, _, err := table([]byte(twice)); err == nil {
		t.Fatal("two operations named the same command and nothing failed")
	}
}

// The committed table is what the contract generates now (rule 11).
func TestTheCommittedTableIsTheContracts(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	commands, groups, err := table(raw)
	if err != nil {
		t.Fatal(err)
	}
	source, err := render(commands, groups)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../cmd/hubctl/Doors.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) != string(source) {
		t.Fatal("cmd/hubctl/Doors.gen.go is not what api/openapi.yaml generates; run make generate")
	}
}

// The committed REST mapping is what the contract and the generated router produce now.
func TestTheCommittedMappingIsTheContracts(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	server, err := os.ReadFile("../../presentation/openapi/Api.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	doors, err := restTable(raw, server)
	if err != nil {
		t.Fatal(err)
	}
	source, err := renderREST(doors)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../presentation/rest/Doors.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) != string(source) {
		t.Fatal("presentation/rest/Doors.gen.go is not what api/openapi.yaml generates; run make generate")
	}
}

// An operation that takes a step-up proof passes it on; the use case decides it.
func TestAStepUpOperationPassesTheProof(t *testing.T) {
	marked := strings.Replace(contract, `      x-hubtask-cli: "thing set"`, `      x-hubtask-cli: "thing set"
      x-hubtask-rest: generated`, 1)
	marked = strings.Replace(marked, `        - $ref: "#/components/parameters/IfMatch"`, `        - $ref: "#/components/parameters/IfMatch"
        - { name: X-Hubtask-Step-Up, in: header, schema: { type: string } }`, 1)
	server := []byte("package openapi\ntype ChangeThingApplicationMergePatchPlusJSONRequestBody = ThingChange\ntype ServerInterface interface {\n\tChangeThing(w http.ResponseWriter, r *http.Request, thingId string)\n}\n")
	doors, err := restTable([]byte(marked), server)
	if err != nil {
		t.Fatal(err)
	}
	if len(doors) != 1 || !doors[0].StepUp || !doors[0].IfMatch {
		t.Fatalf("the doors are %+v", doors)
	}
}

// An answer that is allOf another schema holds that schema's members: a detail's version is the
// record's, and the ETag is drawn from it.
func TestAComposedAnswerHoldsItsPartsMembers(t *testing.T) {
	var doc document
	if err := yaml.Unmarshal([]byte(`
components:
  schemas:
    Group:
      type: object
      properties:
        version: { type: integer }
    GroupDetail:
      allOf:
        - $ref: "#/components/schemas/Group"
        - type: object
          properties:
            members: { type: array, items: { type: string } }
`), &doc); err != nil {
		t.Fatal(err)
	}
	detail := doc.composed(schema{Ref: "#/components/schemas/GroupDetail"})
	if _, held := detail.Properties["version"]; !held || detail.Type != "object" {
		t.Errorf("composed %+v", detail)
	}
	if _, held := detail.Properties["members"]; !held {
		t.Errorf("the detail's own members are missing: %+v", detail)
	}
}

// A door draws only what a request carries: the host, the workspace header, the user agent and
// the remote address; any other name is a mistake in the contract.
func TestARequestInputIsOneARequestCarries(t *testing.T) {
	server := []byte("package openapi\ntype ChangeThingApplicationMergePatchPlusJSONRequestBody = ThingChange\ntype ServerInterface interface {\n\tChangeThing(w http.ResponseWriter, r *http.Request, thingId string)\n}\n")
	for request, fails := range map[string]bool{"[host, tenant_header]": false, "[cookie]": true} {
		marked := strings.Replace(contract, `      x-hubtask-cli: "thing set"`, `      x-hubtask-cli: "thing set"
      x-hubtask-rest: generated
      x-hubtask-rest-request: `+request, 1)
		doors, err := restTable([]byte(marked), server)
		if (err != nil) != fails {
			t.Fatalf("%s: %v", request, err)
		}
		if !fails && (len(doors) != 1 || len(doors[0].Request) != 2) {
			t.Errorf("the doors are %+v", doors)
		}
	}
}

// A cache mark is one the adapter honours, and a whole-answer mark needs the mapper that draws it.
func TestACacheAndAWholeAnswerAreDeclaredMarks(t *testing.T) {
	server := []byte("package openapi\ntype ChangeThingApplicationMergePatchPlusJSONRequestBody = ThingChange\ntype ServerInterface interface {\n\tChangeThing(w http.ResponseWriter, r *http.Request, thingId string)\n}\n")
	for mark, fails := range map[string]bool{
		"x-hubtask-rest-cache: no-store":                                         false,
		"x-hubtask-rest-cache: private":                                          true,
		"x-hubtask-rest-list: whole":                                             true,
		"x-hubtask-rest-list: whole\n      x-hubtask-rest-answer: thingResponse": false,
	} {
		marked := strings.Replace(contract, `      x-hubtask-cli: "thing set"`, `      x-hubtask-cli: "thing set"
      x-hubtask-rest: generated
      `+mark, 1)
		doors, err := restTable([]byte(marked), server)
		if (err != nil) != fails {
			t.Fatalf("%s: %v", mark, err)
		}
		if !fails && strings.Contains(mark, "cache") && !doors[0].NoStore {
			t.Errorf("%s: the door is %+v", mark, doors[0])
		}
	}
}
