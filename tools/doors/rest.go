// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// RESTDoor is one operation the REST adapter answers by the generated mapping: an operation
// marked `x-hubtask-rest: generated`, whose use case's output already carries the contract's
// names. Its method signature is the generated ServerInterface's, read from Api.gen.go, so the
// mapping cannot disagree with the router.
type RESTDoor struct {
	OperationID, UseCase, Method, Path string
	Status                             int
	Query                              []string
	PathArgs                           []pathArg
	Params                             []string // the signature after (w, r), as Go source
	ParamsArg                          string   // the params struct's argument, empty for none
	BodyType                           string   // the generated request body type, empty for none
	AnswerType                         string   // the generated answer type, empty for none
	BodyRequired, IfMatch, StepUp      bool
	// IfMatchAs is the input If-Match reaches as text; empty passes it as `expected_version`.
	IfMatchAs string
	// ETag is the answer's member the ETag is drawn from; empty for none.
	ETag string
	// BodyAs is the input the whole body reaches the use case as; empty spreads its members.
	BodyAs string
	// AnswerBy is the adapter's shared mapper that draws the answer (an entry is drawn by one
	// mapper wherever it is answered); empty passes the output through the answer's type.
	AnswerBy string
	// AnswerRows says the answer is a bare array, each of the output's rows drawn by AnswerBy;
	// AnswerPage that it is a page, `data` drawn so and `page` as every page is.
	AnswerRows, AnswerPage bool
	// Location is the created resource's path below the API, its `{member}`s filled from the
	// answer.
	Location string
	// NullClears passes a member sent as null as the empty text, which is how the use case reads
	// "clear it" (`x-hubtask-rest-null: empty`).
	NullClears bool
	// Integers are the query's parameters and the body's members the contract types as integers,
	// handed on as Go integers as a hand-written handler's typed values were.
	Integers []string
	// ShownOnce is the member that shows a secret once (`x-hubtask-rest-shown-once`): an answer
	// carrying it is marked `no-store` for a cache and shown once for the idempotency record.
	ShownOnce string
	// QueryAs renames a query parameter to the input the use case reads (`x-hubtask-rest-query`).
	QueryAs map[string]string
}

type pathArg struct{ Input, Value string }

type restOperation struct {
	OperationID string            `yaml:"operationId"`
	REST        string            `yaml:"x-hubtask-rest"`
	IfMatchAs   string            `yaml:"x-hubtask-rest-if-match"`
	PathAs      map[string]string `yaml:"x-hubtask-rest-path"`
	BodyAs      string            `yaml:"x-hubtask-rest-body"`
	AnswerBy    string            `yaml:"x-hubtask-rest-answer"`
	Location    string            `yaml:"x-hubtask-rest-location"`
	Null        string            `yaml:"x-hubtask-rest-null"`
	ETagFrom    string            `yaml:"x-hubtask-rest-etag"`
	ShownOnce   string            `yaml:"x-hubtask-rest-shown-once"`
	QueryAs     map[string]string `yaml:"x-hubtask-rest-query"`
	Parameters  []parameter       `yaml:"parameters"`
	RequestBody *struct {
		Required bool                 `yaml:"required"`
		Content  map[string]mediaType `yaml:"content"`
	} `yaml:"requestBody"`
	Responses map[string]struct {
		Content map[string]mediaType `yaml:"content"`
	} `yaml:"responses"`
}

func restTable(raw, server []byte) ([]RESTDoor, error) {
	var doc document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	signatures, bodies, texts, err := serverTypes(server)
	if err != nil {
		return nil, err
	}
	var doors []RESTDoor
	for path, item := range doc.Paths {
		var shared []parameter
		if node, ok := item["parameters"]; ok {
			if err := node.Decode(&shared); err != nil {
				return nil, err
			}
		}
		for _, method := range methods {
			node, ok := item[method]
			if !ok {
				continue
			}
			var op restOperation
			if err := node.Decode(&op); err != nil {
				return nil, err
			}
			switch op.REST {
			case "":
				continue
			case "generated":
			default:
				return nil, fmt.Errorf("%s: x-hubtask-rest %q is not \"generated\"", op.OperationID, op.REST)
			}
			door, err := restDoor(doc, path, method, shared, op, signatures, bodies, texts)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", op.OperationID, err)
			}
			doors = append(doors, door)
		}
	}
	sort.Slice(doors, func(i, j int) bool { return doors[i].UseCase < doors[j].UseCase })
	return doors, nil
}

func restDoor(
	doc document, path, method string, shared []parameter, op restOperation,
	signatures map[string][]*ast.Field, bodies, texts map[string]bool,
) (RESTDoor, error) {
	name := upperFirst(op.OperationID)
	door := RESTDoor{
		OperationID: op.OperationID, UseCase: name, Method: strings.ToUpper(method), Path: path, IfMatchAs: op.IfMatchAs,
		BodyAs: op.BodyAs, AnswerBy: op.AnswerBy, Location: op.Location, ShownOnce: op.ShownOnce, QueryAs: op.QueryAs,
	}
	switch op.Null {
	case "":
	case "empty":
		door.NullClears = true
	default:
		return door, fmt.Errorf("x-hubtask-rest-null %q is not \"empty\"", op.Null)
	}
	signature, ok := signatures[name]
	if !ok {
		return door, fmt.Errorf("the ServerInterface has no method %s; run oapi-codegen first", name)
	}

	for _, raw := range append(append([]parameter{}, shared...), op.Parameters...) {
		p, err := doc.parameter(raw)
		if err != nil {
			return door, err
		}
		switch {
		case p.In == "query":
			door.Query = append(door.Query, p.Name)
			if doc.kind(p.Schema) == "integer" {
				door.Integers = append(door.Integers, p.Name)
			}
		case p.In == "header" && p.Name == "If-Match":
			door.IfMatch = true
		case p.In == "header" && p.Name == "X-Hubtask-Step-Up":
			// The proof travels as `step_up_token`, as every hand-written door passes it; the use
			// case decides whether it holds (rule 2).
			door.StepUp = true
		}
	}
	sort.Strings(door.Query)
	for name := range op.QueryAs {
		if !slicesContains(door.Query, name) {
			return door, fmt.Errorf("x-hubtask-rest-query renames %s, which is not a query parameter", name)
		}
	}

	for _, field := range signature[2:] {
		typ := qualified(field.Type)
		for _, ident := range field.Names {
			door.Params = append(door.Params, ident.Name+" "+typ)
			if strings.HasSuffix(typ, "Params") {
				door.ParamsArg = ident.Name
				continue
			}
			// A text or a number reaches the use case as it is; an identifier as its text, which is
			// what the descriptor's KindID reads.
			value := ident.Name
			switch {
			case typ == "string" || typ == "int":
			case texts[typ]:
				value = "string(" + value + ")"
			default:
				value += ".String()"
			}
			input := snake(ident.Name)
			if renamed, ok := op.PathAs[ident.Name]; ok {
				// Renamed to nothing: the path places the resource, and its own identifier names
				// it to the use case (a label is named by its id, under its collection).
				if renamed == "" {
					continue
				}
				input = renamed
			}
			door.PathArgs = append(door.PathArgs, pathArg{Input: input, Value: value})
		}
	}

	if op.RequestBody != nil {
		door.BodyRequired = op.RequestBody.Required
		for _, content := range op.RequestBody.Content {
			for name, property := range doc.resolve(content.Schema).Properties {
				if doc.kind(property) == "integer" {
					door.Integers = append(door.Integers, name)
				}
			}
		}
		sort.Strings(door.Integers)
		for _, suffix := range []string{"JSONRequestBody", "ApplicationMergePatchPlusJSONRequestBody"} {
			if bodies[name+suffix] {
				door.BodyType = "openapi." + name + suffix
			}
		}
		if door.BodyType == "" {
			return door, fmt.Errorf("no generated JSON request body type")
		}
	}

	for _, status := range []string{"200", "201", "202", "204"} {
		response, ok := op.Responses[status]
		if !ok {
			continue
		}
		fmt.Sscan(status, &door.Status)
		if content, ok := response.Content["application/json"]; ok {
			answer := doc.composed(content.Schema)
			door.AnswerRows = answer.Type == "array"
			_, paged := answer.Properties["page"]
			if data, listed := answer.Properties["data"]; listed && paged && data.Type == "array" && door.AnswerBy != "" {
				door.AnswerPage = true
			}
			if door.AnswerRows && door.AnswerBy == "" {
				return door, fmt.Errorf("an answer that is a bare array needs x-hubtask-rest-answer")
			}
			if version, versioned := answer.Properties["version"]; versioned && doc.kind(version) == "integer" {
				door.ETag = "version"
			}
			if op.ETagFrom != "" {
				if _, held := answer.Properties[op.ETagFrom]; !held {
					return door, fmt.Errorf("x-hubtask-rest-etag names %s, which the answer does not hold", op.ETagFrom)
				}
				door.ETag = op.ETagFrom
			}
			// Typed by the contract: the answer passes through its generated type, so it holds
			// the contract's shape whatever else the use case's output carries. An answer written
			// inline has no type of its own and is written as the output.
			if ref := strings.TrimPrefix(content.Schema.Ref, "#/components/schemas/"); ref != "" && bodies[ref] {
				door.AnswerType = "openapi." + ref
			}
		}
		break
	}
	if door.Status == 0 {
		return door, fmt.Errorf("no successful answer")
	}
	return door, nil
}

// serverTypes reads the generated router's method signatures, every generated type's name, and
// the types that are text (`type Module string`).
func serverTypes(source []byte) (map[string][]*ast.Field, map[string]bool, map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "Api.gen.go", source, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, nil, err
	}
	signatures := map[string][]*ast.Field{}
	bodies := map[string]bool{}
	texts := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			bodies[typeSpec.Name.Name] = true
			if ident, ok := typeSpec.Type.(*ast.Ident); ok && ident.Name == "string" {
				texts["openapi."+typeSpec.Name.Name] = true
			}
			iface, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok || typeSpec.Name.Name != "ServerInterface" {
				continue
			}
			for _, method := range iface.Methods.List {
				fn, ok := method.Type.(*ast.FuncType)
				if !ok || len(method.Names) != 1 {
					continue
				}
				// w and r are one field each in the generated source, so [2:] is what follows them.
				signatures[method.Names[0].Name] = fn.Params.List
			}
		}
	}
	return signatures, bodies, texts, nil
}

// qualified names a generated type from the rest package: `ItemId` is `openapi.ItemId`.
func qualified(expr ast.Expr) string {
	text := types.ExprString(expr)
	if ident, ok := expr.(*ast.Ident); ok && types.Universe.Lookup(ident.Name) == nil {
		return "openapi." + text
	}
	return text
}

func slicesContains(list []string, s string) bool {
	for _, each := range list {
		if each == s {
			return true
		}
	}
	return false
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func renderREST(doors []RESTDoor) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/doors from api/openapi.yaml. DO NOT EDIT.\n\npackage rest\n\n")
	b.WriteString("import (\n\t\"net/http\"\n\n")
	if strings.Contains(fmt.Sprint(doors), "openapi_types.") {
		b.WriteString("\topenapi_types \"github.com/oapi-codegen/runtime/types\"\n\n")
	}
	b.WriteString("\t\"github.com/Jersyfi/hubtask/core/application/usecase\"\n")
	b.WriteString("\t\"github.com/Jersyfi/hubtask/presentation/openapi\"\n)\n\n")
	b.WriteString("// restDoors is every operation answered by the generated mapping, by operationId.\n")
	b.WriteString("var restDoors = map[string]restDoor{\n")
	for _, d := range doors {
		extra := ""
		if d.ShownOnce != "" {
			extra += fmt.Sprintf(", ShownOnce: %q", d.ShownOnce)
		}
		if len(d.QueryAs) > 0 {
			keys := make([]string, 0, len(d.QueryAs))
			for key := range d.QueryAs {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			pairs := make([]string, 0, len(keys))
			for _, key := range keys {
				pairs = append(pairs, fmt.Sprintf("%q: %q", key, d.QueryAs[key]))
			}
			extra += ", QueryAs: map[string]string{" + strings.Join(pairs, ", ") + "}"
		}
		fmt.Fprintf(&b, "\t%q: {UseCase: %q, Status: %d, Query: %#v, IfMatch: %t, IfMatchAs: %q, StepUp: %t, BodyRequired: %t, ETag: %q, BodyAs: %q, Location: %q, NullClears: %t, Integers: %#v%s},\n",
			d.OperationID, d.UseCase, d.Status, d.Query, d.IfMatch, d.IfMatchAs, d.StepUp, d.BodyRequired, d.ETag, d.BodyAs, d.Location, d.NullClears, d.Integers, extra)
	}
	b.WriteString("}\n")
	for _, d := range doors {
		fmt.Fprintf(&b, "\n// %s answers %s %s by the generated mapping.\n", d.UseCase, d.Method, d.Path)
		fmt.Fprintf(&b, "func (c *RestController) %s(%s) {\n", d.UseCase,
			strings.Join(append([]string{"w http.ResponseWriter", "r *http.Request"}, d.Params...), ", "))
		in := make([]string, 0, len(d.PathArgs))
		for _, arg := range d.PathArgs {
			in = append(in, fmt.Sprintf("%q: %s", arg.Input, arg.Value))
		}
		params := "nil"
		if d.ParamsArg != "" {
			params = d.ParamsArg
		}
		body, answer := "nil", "nil"
		if d.BodyType != "" {
			fmt.Fprintf(&b, "\tvar body %s\n", d.BodyType)
			body = "&body"
		}
		switch {
		case d.AnswerBy != "" && d.AnswerRows:
			answer = "answerRows(" + d.AnswerBy + ")"
		case d.AnswerBy != "" && d.AnswerPage:
			answer = "answerPage(" + d.AnswerBy + ")"
		case d.AnswerBy != "":
			answer = "answerBy(" + d.AnswerBy + ")"
		case d.AnswerType != "":
			fmt.Fprintf(&b, "\tvar answer %s\n", d.AnswerType)
			answer = "&answer"
		}
		fmt.Fprintf(&b, "\tc.door(w, r, restDoors[%q], usecase.Input{%s}, %s, %s, %s)\n}\n",
			d.OperationID, strings.Join(in, ", "), params, body, answer)
	}
	return format.Source(b.Bytes())
}
