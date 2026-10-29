// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command doors writes hubctl's generated commands from api/openapi.yaml (F-7).
//
// An operation names its command with `x-hubtask-cli: "<group> <verb>"` (a verb may be two
// words, `type form set`); a new group's one-line summary is in the document's
// `x-hubtask-cli-groups`. Everything else is read from the operation: the path parameters become
// the positional arguments in path order, the query parameters and the top-level properties of a
// JSON request body become flags, `If-Match` becomes `--expect-version`, a step-up parameter makes
// the command prove the person afresh when refused for it, and a list answer's columns are taken
// from its items. The operationId is the use case's REST operation (usecase.Descriptor), so the
// command reaches the same use case every other door does.
//
// **A mark it cannot honour fails it**: an unknown group without a summary, a command named twice,
// or a body that is not a JSON object stops `make generate` rather than going unsaid.
//
// The same operation may also say `x-hubtask-rest: generated`: the REST adapter then answers it
// by the generated mapping in presentation/rest/Doors.gen.go (rest.go) instead of a hand-written
// handler.
//
// Usage: doors <openapi.yaml> <Api.gen.go> <hubctl Doors.gen.go> <rest Doors.gen.go>
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type document struct {
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Groups     map[string]string               `yaml:"x-hubtask-cli-groups"`
	Components struct {
		Schemas    map[string]schema    `yaml:"schemas"`
		Parameters map[string]parameter `yaml:"parameters"`
	} `yaml:"components"`
}

type parameter struct {
	Ref         string `yaml:"$ref"`
	Name        string `yaml:"name"`
	In          string `yaml:"in"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
	Schema      schema `yaml:"schema"`
}

type operation struct {
	OperationID string      `yaml:"operationId"`
	Summary     string      `yaml:"summary"`
	Description string      `yaml:"description"`
	CLI         string      `yaml:"x-hubtask-cli"`
	Columns     []string    `yaml:"x-hubtask-cli-columns"`
	Parameters  []parameter `yaml:"parameters"`
	RequestBody *struct {
		Required bool                 `yaml:"required"`
		Content  map[string]mediaType `yaml:"content"`
	} `yaml:"requestBody"`
	Responses map[string]struct {
		Content map[string]mediaType `yaml:"content"`
	} `yaml:"responses"`
}

type mediaType struct {
	Schema schema `yaml:"schema"`
}

type schema struct {
	Ref         string            `yaml:"$ref"`
	Type        typeName          `yaml:"type"`
	Format      string            `yaml:"format"`
	Enum        []any             `yaml:"enum"`
	Description string            `yaml:"description"`
	Required    []string          `yaml:"required"`
	Properties  map[string]schema `yaml:"properties"`
	Items       *schema           `yaml:"items"`
	ReadOnly    bool              `yaml:"readOnly"`
}

// typeName is a schema's type; OpenAPI 3.1 also writes it as a list, `[string, "null"]`, of
// which the first that is not null counts.
type typeName string

func (t *typeName) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*t = typeName(node.Value)
		return nil
	}
	var names []string
	if err := node.Decode(&names); err != nil {
		return err
	}
	for _, name := range names {
		if name != "null" {
			*t = typeName(name)
			return nil
		}
	}
	return nil
}

// Command is one generated hubctl command, as Doors.gen.go declares it.
type Command struct {
	Group, Verb, OperationID, Method, Path, Summary string
	PathParams, Query, Body                         []Param
	HasBody, BodyRequired, IfMatch, StepUp, List    bool
	Columns                                         []string
}

// Param is one positional argument or flag.
type Param struct {
	Name, Kind, Help string
	Required         bool
}

var methods = []string{"get", "put", "post", "patch", "delete"}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: doors <openapi.yaml> <Api.gen.go> <hubctl out.go> <rest out.go>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1]) //nolint:gosec // G703: the file the Makefile names; reading it is the job
	if err != nil {
		fail(err)
	}
	commands, groups, err := table(raw)
	if err != nil {
		fail(err)
	}
	source, err := render(commands, groups)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(os.Args[3], source, 0o644); err != nil { //nolint:gosec // G306,G703: a committed source file
		fail(err)
	}
	server, err := os.ReadFile(os.Args[2]) //nolint:gosec // G703: the file the Makefile names
	if err != nil {
		fail(err)
	}
	doors, err := restTable(raw, server)
	if err != nil {
		fail(err)
	}
	if source, err = renderREST(doors); err != nil {
		fail(err)
	}
	if err := os.WriteFile(os.Args[4], source, 0o644); err != nil { //nolint:gosec // G306,G703: a committed source file
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "doors:", err)
	os.Exit(1)
}

func table(raw []byte) ([]Command, map[string]string, error) {
	var doc document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	var commands []Command
	seen := map[string]string{}
	usedGroups := map[string]string{}
	for path, item := range doc.Paths {
		var shared []parameter
		if node, ok := item["parameters"]; ok {
			if err := node.Decode(&shared); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
		}
		for _, method := range methods {
			node, ok := item[method]
			if !ok {
				continue
			}
			var op operation
			if err := node.Decode(&op); err != nil {
				return nil, nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
			if strings.TrimSpace(op.CLI) == "" {
				continue
			}
			command, err := build(doc, path, method, shared, op)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", op.OperationID, err)
			}
			name := command.Group + " " + command.Verb
			if other, twice := seen[name]; twice {
				return nil, nil, fmt.Errorf("hubctl %s is named by %s and %s", name, other, op.OperationID)
			}
			seen[name] = op.OperationID
			usedGroups[command.Group] = doc.Groups[command.Group]
			commands = append(commands, command)
		}
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Group+" "+commands[i].Verb < commands[j].Group+" "+commands[j].Verb
	})
	return commands, usedGroups, nil
}

func build(doc document, path, method string, shared []parameter, op operation) (Command, error) {
	words := strings.Fields(op.CLI)
	if len(words) < 2 || len(words) > 3 {
		return Command{}, fmt.Errorf("x-hubtask-cli %q is not \"<group> <verb>\"", op.CLI)
	}
	command := Command{
		Group: words[0], Verb: strings.Join(words[1:], " "), OperationID: op.OperationID,
		Method: strings.ToUpper(method), Path: path, Summary: firstSentence(op.Summary),
	}
	if command.Summary == "" {
		command.Summary = firstSentence(op.Description)
	}

	byName := map[string]parameter{}
	for _, raw := range append(slices.Clone(shared), op.Parameters...) {
		resolved, err := doc.parameter(raw)
		if err != nil {
			return Command{}, err
		}
		byName[resolved.In+":"+resolved.Name] = resolved
	}
	for _, name := range pathNames(path) {
		p, ok := byName["path:"+name]
		if !ok {
			return Command{}, fmt.Errorf("the path parameter %s is not declared", name)
		}
		command.PathParams = append(command.PathParams, Param{Name: name, Kind: doc.kind(p.Schema), Help: firstSentence(p.Description), Required: true})
	}
	var queries []Param
	for key, p := range byName {
		switch {
		case strings.HasPrefix(key, "query:"):
			queries = append(queries, Param{Name: p.Name, Kind: doc.kind(p.Schema), Help: firstSentence(p.Description), Required: p.Required})
		case p.In == "header" && p.Name == "If-Match":
			command.IfMatch = true
		case p.In == "header" && p.Name == "X-Hubtask-Step-Up":
			command.StepUp = true
		}
	}
	sort.Slice(queries, func(i, j int) bool { return queries[i].Name < queries[j].Name })
	command.Query = queries

	if op.RequestBody != nil {
		command.HasBody = true
		command.BodyRequired = op.RequestBody.Required
		for media, content := range op.RequestBody.Content {
			if !strings.Contains(media, "json") {
				return Command{}, fmt.Errorf("the request body %s is not JSON", media)
			}
			body := doc.resolve(content.Schema)
			if body.Type != "object" && len(body.Properties) == 0 {
				return Command{}, fmt.Errorf("the request body is not a JSON object")
			}
			names := make([]string, 0, len(body.Properties))
			for name, property := range body.Properties {
				if !property.ReadOnly {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			for _, name := range names {
				property := body.Properties[name]
				description := property.Description
				if description == "" {
					description = doc.resolve(property).Description
				}
				command.Body = append(command.Body, Param{
					Name: name, Kind: doc.kind(property), Help: firstSentence(description),
					Required: slices.Contains(body.Required, name),
				})
			}
		}
	}

	command.Columns = op.Columns
	for _, status := range []string{"200", "201", "202"} {
		response, ok := op.Responses[status]
		if !ok {
			continue
		}
		content, ok := response.Content["application/json"]
		if !ok {
			break
		}
		answer := doc.resolve(content.Schema)
		if data, ok := answer.Properties["data"]; ok && data.Type == "array" && data.Items != nil {
			command.List = true
			answer = doc.resolve(*data.Items)
		}
		if len(command.Columns) == 0 {
			command.Columns = columns(doc, answer)
		}
		break
	}
	return command, nil
}

// columns picks the scalar properties a person recognises an answer by, at most five.
func columns(doc document, answer schema) []string {
	preferred := []string{"id", "key", "name", "title", "kind", "level", "status", "state", "version"}
	var picked []string
	for _, name := range preferred {
		if property, ok := answer.Properties[name]; ok && scalar(doc.kind(property)) {
			picked = append(picked, name)
		}
	}
	if len(picked) == 0 {
		names := make([]string, 0, len(answer.Properties))
		for name, property := range answer.Properties {
			if scalar(doc.kind(property)) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		picked = names
	}
	if len(picked) > 5 {
		picked = picked[:5]
	}
	return picked
}

func scalar(kind string) bool { return kind != "json" && kind != "strings" }

func (doc document) parameter(p parameter) (parameter, error) {
	if p.Ref == "" {
		return p, nil
	}
	resolved, ok := doc.Components.Parameters[strings.TrimPrefix(p.Ref, "#/components/parameters/")]
	if !ok {
		return parameter{}, fmt.Errorf("the parameter %s is not declared", p.Ref)
	}
	return resolved, nil
}

func (doc document) resolve(s schema) schema {
	for depth := 0; s.Ref != "" && depth < 8; depth++ {
		s = doc.Components.Schemas[strings.TrimPrefix(s.Ref, "#/components/schemas/")]
	}
	return s
}

// kind is how a value is read from the command line: string, uuid, integer, number, boolean,
// strings (comma-separated) or json.
func (doc document) kind(s schema) string {
	s = doc.resolve(s)
	switch s.Type {
	case "string":
		if s.Format == "uuid" {
			return "uuid"
		}
		return "string"
	case "integer", "number", "boolean":
		return string(s.Type)
	case "array":
		if s.Items != nil && doc.resolve(*s.Items).Type == "string" {
			return "strings"
		}
	}
	return "json"
}

func pathNames(path string) []string {
	var names []string
	for rest := path; ; {
		start := strings.Index(rest, "{")
		if start < 0 {
			return names
		}
		end := strings.Index(rest[start:], "}")
		names = append(names, rest[start+1:start+end])
		rest = rest[start+end+1:]
	}
}

// firstSentence keeps a flag's help to one line.
func firstSentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if end := strings.Index(text, ". "); end >= 0 {
		text = text[:end]
	}
	text = strings.TrimSuffix(text, ".")
	if len(text) > 100 {
		cut := strings.LastIndex(text[:100], " ")
		if cut < 40 {
			cut = 100
		}
		text = text[:cut] + " …"
	}
	return text
}

func render(commands []Command, groups map[string]string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/doors from api/openapi.yaml. DO NOT EDIT.\n\npackage main\n\n")
	b.WriteString("// generatedGroups is the summary of each group only generated commands open.\n")
	b.WriteString("var generatedGroups = map[string]string{\n")
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if groups[name] != "" {
			fmt.Fprintf(&b, "\t%q: %q,\n", name, groups[name])
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// generatedCommands is every operation the contract names a command for, in a stable order.\n")
	b.WriteString("var generatedCommands = []door{\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "\t{Group: %q, Verb: %q, OperationID: %q, Method: %q, Path: %q, Summary: %q,\n", c.Group, c.Verb, c.OperationID, c.Method, c.Path, c.Summary)
		writeParams(&b, "PathParams", c.PathParams)
		writeParams(&b, "Query", c.Query)
		writeParams(&b, "Body", c.Body)
		fmt.Fprintf(&b, "\t\tHasBody: %t, BodyRequired: %t, IfMatch: %t, StepUp: %t, List: %t, Columns: %#v},\n",
			c.HasBody, c.BodyRequired, c.IfMatch, c.StepUp, c.List, c.Columns)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

func writeParams(b *bytes.Buffer, field string, params []Param) {
	if len(params) == 0 {
		return
	}
	fmt.Fprintf(b, "\t\t%s: []doorParam{\n", field)
	for _, p := range params {
		fmt.Fprintf(b, "\t\t\t{Name: %q, Kind: %q, Help: %q, Required: %t},\n", p.Name, p.Kind, p.Help, p.Required)
	}
	b.WriteString("\t\t},\n")
}
