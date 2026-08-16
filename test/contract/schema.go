// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

// Package contract checks responses against api/openapi.yaml. The specification is the source,
// not the result (ADR-0004) - so the test reads the specification and judges the response by it,
// never the other way round.
//
// The validator here covers the subset of JSON Schema the specification actually uses: $ref,
// type (including nullable unions), required, properties, items, enum, and
// additionalProperties. A full validator would be a dependency, and a dependency that silently
// accepts what it does not understand is worse than 150 lines that fail loudly on anything
// unfamiliar.
package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const specPath = "../../api/openapi.yaml"

// schema is one node of the specification.
type schema struct {
	Ref                  string             `yaml:"$ref"`
	Type                 any                `yaml:"type"`
	Required             []string           `yaml:"required"`
	Properties           map[string]*schema `yaml:"properties"`
	Items                *schema            `yaml:"items"`
	Enum                 []any              `yaml:"enum"`
	AdditionalProperties any                `yaml:"additionalProperties"`
}

// specification is the part of the document this validator needs.
type specification struct {
	Components struct {
		Schemas map[string]*schema `yaml:"schemas"`
	} `yaml:"components"`
}

func loadSpec() (*specification, error) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("reading the specification: %w", err)
	}
	var spec specification
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("parsing the specification: %w", err)
	}
	if len(spec.Components.Schemas) == 0 {
		return nil, fmt.Errorf("%s declares no schemas", specPath)
	}
	return &spec, nil
}

// validateAgainst decodes body as JSON and checks it against the named schema. It returns every
// finding rather than the first: a response is easier to fix once than four times.
func (s *specification) validateAgainst(name string, body []byte) ([]string, error) {
	root, ok := s.Components.Schemas[name]
	if !ok {
		return nil, fmt.Errorf("the specification declares no schema %q", name)
	}

	var value any
	// Numbers stay json.Number, so an integer field holding 3.5 is a finding rather than a
	// float64 that looks like an integer.
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("the response is not JSON: %w", err)
	}

	return s.check("", root, value), nil
}

func (s *specification) resolve(node *schema) (*schema, []string) {
	seen := 0
	for node != nil && node.Ref != "" {
		seen++
		if seen > 10 {
			return nil, []string{"the specification contains a $ref cycle"}
		}
		name, ok := strings.CutPrefix(node.Ref, "#/components/schemas/")
		if !ok {
			return nil, []string{fmt.Sprintf("unsupported $ref %q", node.Ref)}
		}
		next, ok := s.Components.Schemas[name]
		if !ok {
			return nil, []string{fmt.Sprintf("$ref points at the unknown schema %q", name)}
		}
		node = next
	}
	return node, nil
}

func (s *specification) check(path string, node *schema, value any) []string {
	node, problems := s.resolve(node)
	if len(problems) > 0 {
		return problems
	}
	if node == nil {
		return nil
	}

	types := typesOf(node.Type)
	if len(types) > 0 && !matchesAnyType(types, value) {
		return []string{fmt.Sprintf("%s: %s is not %s", at(path), describe(value), strings.Join(types, " or "))}
	}
	if value == nil {
		return nil
	}

	if len(node.Enum) > 0 && !inEnum(node.Enum, value) {
		problems = append(problems, fmt.Sprintf("%s: %v is outside the enum %v", at(path), value, node.Enum))
	}

	switch typed := value.(type) {
	case map[string]any:
		problems = append(problems, s.checkObject(path, node, typed)...)
	case []any:
		for i, item := range typed {
			problems = append(problems, s.check(fmt.Sprintf("%s[%d]", path, i), node.Items, item)...)
		}
	}
	return problems
}

func (s *specification) checkObject(path string, node *schema, value map[string]any) []string {
	var problems []string

	for _, name := range node.Required {
		if _, ok := value[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: the required field %q is missing", at(path), name))
		}
	}

	// Free-form maps (params) are declared with additionalProperties and have no property list.
	if node.AdditionalProperties == true && len(node.Properties) == 0 {
		return problems
	}

	for name, field := range value {
		declared, ok := node.Properties[name]
		if !ok {
			// The direction that catches drift: a field the server sends and the specification
			// does not know is a contract the client was never told about (ADR-0004).
			problems = append(problems, fmt.Sprintf("%s: the field %q is not in the specification", at(path), name))
			continue
		}
		problems = append(problems, s.check(path+"."+name, declared, field)...)
	}
	return problems
}

// typesOf normalises `type: string` and `type: [string, "null"]` to one list.
func typesOf(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, t := range typed {
			if name, ok := t.(string); ok {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}

func matchesAnyType(types []string, value any) bool {
	for _, name := range types {
		if matchesType(name, value) {
			return true
		}
	}
	return false
}

func matchesType(name string, value any) bool {
	switch name {
	case "null":
		return value == nil
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "number":
		return isNumber(value)
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := number.Int64()
		return err == nil
	default:
		return false
	}
}

func isNumber(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := number.Float64()
	return err == nil
}

func inEnum(options []any, value any) bool {
	for _, option := range options {
		if option == nil && value == nil {
			return true
		}
		if fmt.Sprint(option) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

func describe(value any) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%T(%v)", value, value)
}

func at(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

// decode parses a response body for the assertions a schema cannot make.
func decode(t interface{ Fatalf(string, ...any) }, body []byte) map[string]any {
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	return out
}
