// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
)

// bodySchema is the part of the contract's schemas this test reads: members, references and
// compositions.
type bodySchema struct {
	Ref        string                `yaml:"$ref"`
	ReadOnly   bool                  `yaml:"readOnly"`
	AllOf      []bodySchema          `yaml:"allOf"`
	Properties map[string]bodySchema `yaml:"properties"`
}

// Every member a generated door's body may carry reaches its use case as an input the use case
// declares - spread, flattened or as one whole document - because the catalogue refuses an
// undeclared input (usecase.field_unknown) and a contract member that is refused is a contract
// that lies. A read-only member is the answer's: a client that saves what it read sends it back,
// and the door drops it.
func TestEveryGeneratedBodyMemberIsADeclaredInput(t *testing.T) {
	// A summary's ask shares AiAsk with the asks that apply, and a summary has nowhere to apply
	// to: `apply` is refused there. Removing it from that body is a contract change, not this test's.
	refused := map[string]bool{"aiSummarizeContainer.apply": true}
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths      map[string]map[string]yaml.Node `yaml:"paths"`
		Components struct {
			Schemas map[string]bodySchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	var members func(s bodySchema, into map[string]bodySchema)
	members = func(s bodySchema, into map[string]bodySchema) {
		if s.Ref != "" {
			members(contract.Components.Schemas[strings.TrimPrefix(s.Ref, "#/components/schemas/")], into)
		}
		for _, part := range s.AllOf {
			members(part, into)
		}
		for name, member := range s.Properties {
			into[name] = member
		}
	}
	declared := map[string]map[string]bool{}
	for _, descriptor := range usecases.Descriptors() {
		fields := map[string]bool{}
		for _, field := range descriptor.Input {
			fields[field.Name] = true
		}
		declared[descriptor.Name] = fields
	}
	for _, item := range contract.Paths {
		for method, node := range item {
			if method == "parameters" {
				continue
			}
			var op struct {
				OperationID string `yaml:"operationId"`
				RequestBody *struct {
					Content map[string]struct {
						Schema bodySchema `yaml:"schema"`
					} `yaml:"content"`
				} `yaml:"requestBody"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatal(err)
			}
			door, generated := restDoors[op.OperationID]
			if !generated || op.RequestBody == nil || door.BodyAs != "" {
				continue
			}
			fields := declared[door.UseCase]
			for media, content := range op.RequestBody.Content {
				if media == "application/x-www-form-urlencoded" {
					// A form is the controller's own method's (x-hubtask-rest-form), not the door's.
					continue
				}
				body := map[string]bodySchema{}
				members(content.Schema, body)
				for name, member := range body {
					if member.ReadOnly {
						if !slices.Contains(door.ReadOnly, name) {
							t.Errorf("%s: the read-only member %s would reach the use case", op.OperationID, name)
						}
						continue
					}
					if name == "expand" && door.Expand != nil {
						for word, input := range door.Expand {
							if !fields[input] {
								t.Errorf("%s: the word %s reaches %s as %q, which it does not declare", op.OperationID, word, door.UseCase, input)
							}
						}
						continue
					}
					if inputs, flat := door.Flatten[name]; flat {
						// Every nested member is named - a deeper one by its dotted path - and each
						// name is a declared input: a member the door would leave out is a lie too.
						nested := map[string]bodySchema{}
						members(member, nested)
						for inner := range nested {
							deeper := false
							for path := range inputs {
								deeper = deeper || strings.HasPrefix(path, inner+".")
							}
							if _, named := inputs[inner]; !named && !deeper && !slices.Contains(door.Dropped, name+"."+inner) {
								t.Errorf("%s: %s.%s reaches %s under no name", op.OperationID, name, inner, door.UseCase)
							}
						}
						for inner, input := range inputs {
							if !fields[input] {
								t.Errorf("%s: %s.%s reaches %s as %q, which it does not declare", op.OperationID, name, inner, door.UseCase, input)
							}
						}
						continue
					}
					if slices.Contains(door.WriterMembers, name) {
						// The writer's: the file's format and zone, which no use case reads.
						if fields[name] {
							t.Errorf("%s: %s is the writer's, but %s reads it", op.OperationID, name, door.UseCase)
						}
						continue
					}
					if slices.Contains(door.Dropped, name) {
						if fields[name] {
							t.Errorf("%s: %s is dropped, but %s reads it", op.OperationID, name, door.UseCase)
						}
						continue
					}
					if !fields[name] && !refused[op.OperationID+"."+name] {
						t.Errorf("%s: the body member %s is not an input %s declares", op.OperationID, name, door.UseCase)
					}
				}
			}
		}
	}
}
