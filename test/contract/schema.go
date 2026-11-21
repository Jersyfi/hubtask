// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

// Package contract checks responses against api/openapi.yaml. The specification is the source,
// not the result (ADR-0004) - so the test reads the specification and judges the response by it,
// never the other way round.
//
// The validator here covers the subset of JSON Schema the specification actually uses: $ref,
// type (including nullable unions), required, properties, items, enum, additionalProperties,
// and allOf, oneOf and anyOf. A full validator would be a dependency, and a dependency that silently
// accepts what it does not understand is worse than 150 lines that fail loudly on anything
// unfamiliar.
package contract

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
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
	// Deprecated marks a member that is still accepted and goes with the next major version.
	Deprecated bool `yaml:"deprecated"`
	// Since and RemovedIn are a deprecated member's day and the major version it goes with.
	Since     string `yaml:"x-deprecated-since"`
	RemovedIn string `yaml:"x-removed-in"`
	// AllOf is composed for a response. InputEnums refuses a request body that uses it rather than
	// reporting an empty set.
	AllOf []*schema `yaml:"allOf"`
	OneOf []*schema `yaml:"oneOf"`
	AnyOf []*schema `yaml:"anyOf"`
}

// specification is the part of the document this validator needs.
//
// A path holds more than its methods - `parameters` sits alongside them and is a list, not an
// operation - so the entries stay raw nodes and only the known methods are decoded.
type specification struct {
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas    map[string]*schema    `yaml:"schemas"`
		Parameters map[string]*parameter `yaml:"parameters"`
		Responses  map[string]*response  `yaml:"responses"`
	} `yaml:"components"`
}

// httpMethods are the keys of a path item that are operations (OpenAPI 3.1 §4.8.9).
var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// operation is one method of one path. Security is a pointer so that "declared as empty" can be
// told apart from "not declared": an empty list makes the operation public, an absent one leaves
// the document-wide requirement in force (OpenAPI 3.1 §4.8.2).
type operation struct {
	OperationID string       `yaml:"operationId"`
	Deprecated  bool         `yaml:"deprecated"`
	Security    *[]any       `yaml:"security"`
	Parameters  []*parameter `yaml:"parameters"`
	RequestBody *requestBody `yaml:"requestBody"`
	// Responses are keyed by status as the document writes it: "200", "4XX", "default".
	Responses map[string]*response `yaml:"responses"`
}

// response is one answer an operation declares: a schema per media type, or a $ref to one declared
// once under components.responses.
type response struct {
	Ref     string `yaml:"$ref"`
	Content map[string]struct {
		Schema *schema `yaml:"schema"`
	} `yaml:"content"`
}

// parameter is one declared input outside the body. It carries a $ref of its own, because most of
// them are declared once under components.parameters and referenced from every path that takes one.
type parameter struct {
	Ref        string  `yaml:"$ref"`
	Name       string  `yaml:"name"`
	In         string  `yaml:"in"`
	Deprecated bool    `yaml:"deprecated"`
	Schema     *schema `yaml:"schema"`
}

// requestBody is the body an operation reads. Only the schema per media type is needed: what the
// media type is called is checked elsewhere, and the enums are the same whichever one it is.
type requestBody struct {
	Content map[string]struct {
		Schema *schema `yaml:"schema"`
	} `yaml:"content"`
}

// route is a method and a path as the router registers them, which is also the form the metric
// labels and the public-route list use.
func (o *operation) route(method, path, basePath string) string {
	return strings.ToUpper(method) + " " + basePath + path
}

// Routes returns every operation the specification declares, as route templates.
func (s *specification) Routes(basePath string) (map[string]*operation, error) {
	routes := map[string]*operation{}
	for path, item := range s.Paths {
		for method, node := range item {
			if !slices.Contains(httpMethods, strings.ToLower(method)) {
				continue
			}
			var op operation
			if err := node.Decode(&op); err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("%s %s declares no operationId", method, path)
			}
			routes[op.route(method, path, basePath)] = &op
		}
	}
	return routes, nil
}

// IsPublic reports an operation the specification exempts from authentication.
func (o *operation) IsPublic() bool { return o.Security != nil && len(*o.Security) == 0 }

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
	if len(node.AllOf) > 0 {
		if node, problems = s.composed(node, 0); len(problems) > 0 {
			return problems
		}
	}
	if alternatives := append(slices.Clone(node.OneOf), node.AnyOf...); len(alternatives) > 0 {
		return s.checkAlternatives(path, alternatives, value)
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

// composed merges an allOf into one object schema: the members' properties and required names
// together, and what the node itself declares beside them. Merging rather than checking each
// member alone is what keeps "a field nobody declared" meaningful - a field of the second member
// is not a surprise to the first.
func (s *specification) composed(node *schema, depth int) (*schema, []string) {
	if depth > 10 {
		return nil, []string{"the specification nests allOf more than ten deep"}
	}
	merged := *node
	merged.AllOf = nil
	merged.Properties = maps.Clone(node.Properties)
	if merged.Properties == nil {
		merged.Properties = map[string]*schema{}
	}
	for _, member := range node.AllOf {
		part, problems := s.resolve(member)
		if len(problems) > 0 {
			return nil, problems
		}
		if part == nil {
			continue
		}
		if len(part.AllOf) > 0 {
			if part, problems = s.composed(part, depth+1); len(problems) > 0 {
				return nil, problems
			}
		}
		maps.Copy(merged.Properties, part.Properties)
		merged.Required = append(slices.Clone(merged.Required), part.Required...)
		if merged.Type == nil {
			merged.Type = part.Type
		}
		if merged.Items == nil {
			merged.Items = part.Items
		}
		if merged.AdditionalProperties == nil {
			merged.AdditionalProperties = part.AdditionalProperties
		}
		if len(merged.Enum) == 0 {
			merged.Enum = part.Enum
		}
		merged.OneOf = append(slices.Clone(merged.OneOf), part.OneOf...)
		merged.AnyOf = append(slices.Clone(merged.AnyOf), part.AnyOf...)
	}
	return &merged, nil
}

// checkAlternatives passes a value that fits one member of a oneOf or anyOf, and otherwise
// reports the member it came closest to. "Exactly one" is not held: the document uses oneOf for
// "this or null", where the members cannot both fit.
func (s *specification) checkAlternatives(path string, alternatives []*schema, value any) []string {
	var closest []string
	for i, alternative := range alternatives {
		problems := s.check(path, alternative, value)
		if len(problems) == 0 {
			return nil
		}
		if i == 0 || len(problems) < len(closest) {
			closest = problems
		}
	}
	return closest
}

func (s *specification) checkObject(path string, node *schema, value map[string]any) []string {
	var problems []string

	for _, name := range node.Required {
		if _, ok := value[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: the required field %q is missing", at(path), name))
		}
	}

	// Free-form maps (params, limits) are declared with `additionalProperties: true` and have no
	// property list: anything goes, and there is nothing left to check.
	if node.AdditionalProperties == true && len(node.Properties) == 0 {
		return problems
	}
	// A typed map (features) declares additionalProperties as a schema. Every value is checked
	// against it, and no name is a surprise - the names are the point of such a map.
	if declared := additionalSchema(node); declared != nil && len(node.Properties) == 0 {
		for name, field := range value {
			problems = append(problems, s.check(path+"."+name, declared, field)...)
		}
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

// additionalSchema reads `additionalProperties` when it is a schema rather than a boolean. The
// document is decoded into `any`, so the node arrives as a map and is re-decoded here - narrower
// than teaching the schema type to be either, and it keeps the unmarshalling in one shape.
func additionalSchema(node *schema) *schema {
	raw, ok := node.AdditionalProperties.(map[string]any)
	if !ok {
		return nil
	}
	encoded, err := yaml.Marshal(raw)
	if err != nil {
		return nil
	}
	var declared schema
	if err := yaml.Unmarshal(encoded, &declared); err != nil {
		return nil
	}
	return &declared
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

// Operations returns every operation the specification declares, keyed by its operationId. That
// identifier is what a use case descriptor names as its REST operation, so it is the join between
// the contract and the catalogue.
func (s *specification) Operations() (map[string]*operation, error) {
	operations := map[string]*operation{}
	for path, item := range s.Paths {
		for method, node := range item {
			if !slices.Contains(httpMethods, strings.ToLower(method)) {
				continue
			}
			var op operation
			if err := node.Decode(&op); err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("%s %s declares no operationId", method, path)
			}
			operations[op.OperationID] = &op
		}
	}
	return operations, nil
}

// InputEnums returns the closed sets an operation declares for its inputs, keyed by the name the
// caller sends: the properties of the request body, and the query parameters.
//
// Only the top level of a body is read. A nested object is flattened by the controller before the
// use case sees it (`scope` becomes `scope_container_id`), so its properties have no field of the
// same name to be compared against.
func (s *specification) InputEnums(op *operation) (map[string][]string, error) {
	enums := map[string][]string{}

	for _, declared := range op.Parameters {
		param, err := s.resolveParameter(declared)
		if err != nil {
			return nil, err
		}
		if param.In != "query" || param.Schema == nil {
			continue
		}
		if values := enumOf(param.Schema); len(values) > 0 {
			enums[param.Name] = values
		}
	}

	if op.RequestBody == nil {
		return enums, nil
	}
	for media, content := range op.RequestBody.Content {
		body, problems := s.resolve(content.Schema)
		if len(problems) > 0 {
			return nil, fmt.Errorf("%s, %s: %s", op.OperationID, media, strings.Join(problems, "; "))
		}
		if body == nil {
			continue
		}
		if len(body.Properties) == 0 && len(body.AllOf) > 0 {
			// Every allOf in the document describes a response today. If one ever describes a
			// request body, this reader would report "no enums declared" and the gate would go
			// quiet on it - so it says so instead.
			return nil, fmt.Errorf(
				"%s: the request body is an allOf, which this reader does not compose", op.OperationID)
		}
		for name, property := range body.Properties {
			resolved, problems := s.resolve(property)
			if len(problems) > 0 {
				return nil, fmt.Errorf("%s, %s: %s", op.OperationID, name, strings.Join(problems, "; "))
			}
			if values := enumOf(resolved); len(values) > 0 {
				enums[name] = values
			}
		}
	}
	return enums, nil
}

// resolveParameter follows a parameter's $ref into components.parameters. Most of them are
// declared once and referenced from every path that takes one.
func (s *specification) resolveParameter(declared *parameter) (*parameter, error) {
	if declared == nil {
		return &parameter{}, nil
	}
	if declared.Ref == "" {
		return declared, nil
	}
	name, ok := strings.CutPrefix(declared.Ref, "#/components/parameters/")
	if !ok {
		return nil, fmt.Errorf("unsupported parameter $ref %q", declared.Ref)
	}
	resolved, ok := s.Components.Parameters[name]
	if !ok {
		return nil, fmt.Errorf("$ref points at the unknown parameter %q", declared.Ref)
	}
	return resolved, nil
}

// enumOf reads a schema's enum as the values a caller may send. A `null` entry is not one of them:
// it says the field is clearable, which a descriptor expresses by being optional and reading an
// empty value as absent. A non-string entry is one this comparison could not make either way.
func enumOf(node *schema) []string {
	values := make([]string, 0, len(node.Enum))
	for _, option := range node.Enum {
		if text, ok := option.(string); ok {
			values = append(values, text)
		}
	}
	return values
}

// alternativesBesideAShape reports a node check would judge in part: one with a oneOf or an anyOf
// and a type, properties, items, an enum or required names of its own, which check leaves unread.
func alternativesBesideAShape(node *schema) bool {
	if len(node.OneOf)+len(node.AnyOf) == 0 {
		return false
	}
	return node.Type != nil || len(node.Properties) > 0 || node.Items != nil || len(node.Enum) > 0 ||
		len(node.Required) > 0 || node.AdditionalProperties != nil
}
