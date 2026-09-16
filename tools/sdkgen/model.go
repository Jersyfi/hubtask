// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The model: as much of the contract as a thin client needs, in the document's own order.
//
// Not a general OpenAPI library - the tool reads a fifth of the specification and a dependency
// for the other four fifths would be a supply chain decision (CLAUDE.md). The YAML is walked as a
// node tree so that the operations and the fields come out in the order they were written, which
// is the order a reader of the generated client meets them in.

// Param is one path, query or header parameter.
type Param struct {
	Name        string
	In          string
	Required    bool
	Description string
	// Type is the parameter's schema as the Python side spells it; TypeScript reads the
	// generated `paths` types and never needs it.
	Type string
}

// Body is a request body: its one content type, and the named schema where it has one.
type Body struct {
	ContentType string
	SchemaName  string
	Required    bool
}

// Answer is the successful response a method decodes: the status, and the content type where
// the response carries a body.
type Answer struct {
	Status      string
	ContentType string
	SchemaName  string
}

// Operation is one method of the client.
type Operation struct {
	ID          string
	Method      string
	Path        string
	Summary     string
	Description string
	Deprecated  bool
	Public      bool
	PathParams  []Param
	Query       []Param
	Headers     []Param
	Body        *Body
	Answer      *Answer
}

// Property is one field of an object schema, as the Python TypedDict needs it.
type Property struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// Schema is one named component schema.
type Schema struct {
	Name        string
	Description string
	// Properties is set for an object; Alias for everything else (an enum, a string, an array).
	Properties []Property
	Alias      string
}

// Document is what both generators read.
type Document struct {
	Title      string
	Version    string
	Operations []Operation
	Schemas    []Schema
}

/* ── YAML node helpers ─────────────────────────────────────────────────────────────────── */

func mapping(node *yaml.Node) []struct{ Key, Value *yaml.Node } {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return mapping(node.Alias)
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]struct{ Key, Value *yaml.Node }, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		out = append(out, struct{ Key, Value *yaml.Node }{node.Content[i], node.Content[i+1]})
	}
	return out
}

func get(node *yaml.Node, key string) *yaml.Node {
	for _, entry := range mapping(node) {
		if entry.Key.Value == key {
			return entry.Value
		}
	}
	return nil
}

func text(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func boolean(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Value == "true"
}

func items(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

/* ── Reading ───────────────────────────────────────────────────────────────────────────── */

type reader struct {
	root       *yaml.Node
	parameters *yaml.Node
	schemas    *yaml.Node
}

// Read parses the specification into the model.
func Read(raw []byte) (*Document, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("sdkgen: parse: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("sdkgen: expected one document")
	}
	r := &reader{root: doc.Content[0]}
	components := get(r.root, "components")
	r.parameters = get(components, "parameters")
	r.schemas = get(components, "schemas")

	out := &Document{
		Title:   text(get(get(r.root, "info"), "title")),
		Version: text(get(get(r.root, "info"), "version")),
	}
	for _, path := range mapping(get(r.root, "paths")) {
		shared := items(get(path.Value, "parameters"))
		for _, method := range mapping(path.Value) {
			switch method.Key.Value {
			case "get", "post", "put", "patch", "delete":
			default:
				continue
			}
			operation, err := r.operation(path.Key.Value, method.Key.Value, method.Value, shared)
			if err != nil {
				return nil, err
			}
			out.Operations = append(out.Operations, operation)
		}
	}
	for _, schema := range mapping(r.schemas) {
		out.Schemas = append(out.Schemas, r.schema(schema.Key.Value, schema.Value))
	}
	if len(out.Operations) == 0 {
		return nil, fmt.Errorf("sdkgen: the document declares no operation")
	}
	return out, nil
}

func (r *reader) resolveParameter(node *yaml.Node) *yaml.Node {
	if ref := text(get(node, "$ref")); ref != "" {
		return get(r.parameters, refName(ref))
	}
	return node
}

func (r *reader) operation(path, method string, node *yaml.Node, shared []*yaml.Node) (Operation, error) {
	op := Operation{
		ID:          text(get(node, "operationId")),
		Method:      strings.ToUpper(method),
		Path:        path,
		Summary:     text(get(node, "summary")),
		Description: text(get(node, "description")),
		Deprecated:  boolean(get(node, "deprecated")),
	}
	if op.ID == "" {
		return op, fmt.Errorf("sdkgen: %s %s has no operationId", method, path)
	}
	if security := get(node, "security"); security != nil && security.Kind == yaml.SequenceNode && len(security.Content) == 0 {
		op.Public = true
	}
	all := append(append([]*yaml.Node{}, shared...), items(get(node, "parameters"))...)
	for _, raw := range all {
		p := r.resolveParameter(raw)
		if p == nil {
			return op, fmt.Errorf("sdkgen: %s: unknown parameter %s", op.ID, text(get(raw, "$ref")))
		}
		param := Param{
			Name:        text(get(p, "name")),
			In:          text(get(p, "in")),
			Required:    boolean(get(p, "required")),
			Description: text(get(p, "description")),
			Type:        r.pyType(get(p, "schema"), false),
		}
		switch param.In {
		case "path":
			param.Required = true
			op.PathParams = append(op.PathParams, param)
		case "query":
			op.Query = append(op.Query, param)
		case "header":
			op.Headers = append(op.Headers, param)
		}
	}
	if body := get(node, "requestBody"); body != nil {
		for _, content := range mapping(get(body, "content")) {
			op.Body = &Body{
				ContentType: content.Key.Value,
				SchemaName:  refName(text(get(get(content.Value, "schema"), "$ref"))),
				Required:    boolean(get(body, "required")),
			}
			break
		}
	}
	for _, response := range mapping(get(node, "responses")) {
		status := response.Key.Value
		if !strings.HasPrefix(status, "2") {
			continue
		}
		answer := &Answer{Status: status}
		for _, content := range mapping(get(response.Value, "content")) {
			answer.ContentType = content.Key.Value
			answer.SchemaName = refName(text(get(get(content.Value, "schema"), "$ref")))
			break
		}
		// The first 2xx in the document's order is the one a method decodes; a second one
		// (`signIn` answers 201 or 202) is read from the raw answer by a caller who needs it.
		op.Answer = answer
		break
	}
	return op, nil
}

func (r *reader) schema(name string, node *yaml.Node) Schema {
	out := Schema{Name: name, Description: text(get(node, "description"))}
	props, required := r.objectShape(node)
	if props == nil {
		out.Alias = r.pyType(node, true)
		return out
	}
	for _, p := range props {
		out.Properties = append(out.Properties, Property{
			Name:        p.Key.Value,
			Type:        r.pyType(p.Value, false),
			Required:    required[p.Key.Value],
			Description: text(get(p.Value, "description")),
		})
	}
	return out
}

// objectShape answers an object's properties and required set, `allOf` merged; nil for anything
// that is not an object.
func (r *reader) objectShape(node *yaml.Node) ([]struct{ Key, Value *yaml.Node }, map[string]bool) {
	if node == nil {
		return nil, nil
	}
	if ref := text(get(node, "$ref")); ref != "" {
		return r.objectShape(get(r.schemas, refName(ref)))
	}
	required := map[string]bool{}
	var props []struct{ Key, Value *yaml.Node }
	found := false
	if all := items(get(node, "allOf")); all != nil {
		for _, part := range all {
			p, req := r.objectShape(part)
			if p != nil {
				found = true
				props = append(props, p...)
				for k := range req {
					required[k] = true
				}
			}
		}
	}
	if properties := get(node, "properties"); properties != nil {
		found = true
		props = append(props, mapping(properties)...)
	}
	for _, name := range items(get(node, "required")) {
		required[name.Value] = true
	}
	if !found {
		return nil, nil
	}
	return props, required
}

// pyType spells a schema as a Python annotation. Named schemas are forward references (quoted
// at the call site by the generator), so a recursive schema is finite.
func (r *reader) pyType(node *yaml.Node, top bool) string {
	if node == nil {
		return "Any"
	}
	if ref := text(get(node, "$ref")); ref != "" {
		return `"` + refName(ref) + `"`
	}
	if union := items(get(node, "oneOf")); union == nil {
		union = items(get(node, "anyOf"))
		if union != nil {
			return r.union(union)
		}
	} else {
		return r.union(union)
	}
	if all := items(get(node, "allOf")); all != nil && !top {
		names := []string{}
		for _, part := range all {
			if ref := text(get(part, "$ref")); ref != "" {
				names = append(names, `"`+refName(ref)+`"`)
			}
		}
		if len(names) == 1 {
			return names[0]
		}
		return "dict[str, Any]"
	}
	types := []string{}
	if t := get(node, "type"); t != nil {
		if t.Kind == yaml.SequenceNode {
			for _, each := range t.Content {
				types = append(types, each.Value)
			}
		} else {
			types = append(types, t.Value)
		}
	}
	nullable := false
	own := ""
	for _, t := range types {
		if t == "null" {
			nullable = true
		} else {
			own = t
		}
	}
	if boolean(get(node, "nullable")) {
		nullable = true
	}
	var py string
	switch own {
	case "string":
		if enum := items(get(node, "enum")); enum != nil {
			values := []string{}
			for _, v := range enum {
				if v.Tag == "!!null" {
					nullable = true
					continue
				}
				values = append(values, fmt.Sprintf("%q", v.Value))
			}
			py = "Literal[" + strings.Join(values, ", ") + "]"
		} else {
			py = "str"
		}
	case "integer":
		py = "int"
	case "number":
		py = "float"
	case "boolean":
		py = "bool"
	case "array":
		py = "list[" + r.pyType(get(node, "items"), false) + "]"
	case "object":
		py = "dict[str, Any]"
	default:
		if get(node, "properties") != nil {
			py = "dict[str, Any]"
		} else {
			py = "Any"
		}
	}
	if nullable && py != "Any" {
		py += " | None"
	}
	return py
}

func (r *reader) union(parts []*yaml.Node) string {
	seen := map[string]bool{}
	names := []string{}
	for _, part := range parts {
		t := r.pyType(part, false)
		if !seen[t] {
			seen[t] = true
			names = append(names, t)
		}
	}
	sort.Strings(names)
	return strings.Join(names, " | ")
}
