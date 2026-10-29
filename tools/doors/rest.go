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
}

type pathArg struct{ Input, Value string }

type restOperation struct {
	OperationID string      `yaml:"operationId"`
	REST        string      `yaml:"x-hubtask-rest"`
	IfMatchAs   string      `yaml:"x-hubtask-rest-if-match"`
	ETagFrom    string      `yaml:"x-hubtask-rest-etag"`
	Parameters  []parameter `yaml:"parameters"`
	RequestBody *struct {
		Required bool `yaml:"required"`
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
	signatures, bodies, err := serverInterface(server)
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
			door, err := restDoor(doc, path, method, shared, op, signatures, bodies)
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
	signatures map[string][]*ast.Field, bodies map[string]bool,
) (RESTDoor, error) {
	name := upperFirst(op.OperationID)
	door := RESTDoor{OperationID: op.OperationID, UseCase: name, Method: strings.ToUpper(method), Path: path, IfMatchAs: op.IfMatchAs}
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
		case p.In == "header" && p.Name == "If-Match":
			door.IfMatch = true
		case p.In == "header" && p.Name == "X-Hubtask-Step-Up":
			// The proof travels as `step_up_token`, as every hand-written door passes it; the use
			// case decides whether it holds (rule 2).
			door.StepUp = true
		}
	}
	sort.Strings(door.Query)

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
			if typ != "string" && typ != "int" {
				value += ".String()"
			}
			door.PathArgs = append(door.PathArgs, pathArg{Input: snake(ident.Name), Value: value})
		}
	}

	if op.RequestBody != nil {
		door.BodyRequired = op.RequestBody.Required
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
			answer := doc.resolve(content.Schema)
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

// serverInterface reads the generated router's method signatures and every generated type's name.
func serverInterface(source []byte) (map[string][]*ast.Field, map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "Api.gen.go", source, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	signatures := map[string][]*ast.Field{}
	bodies := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			bodies[typeSpec.Name.Name] = true
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
	return signatures, bodies, nil
}

// qualified names a generated type from the rest package: `ItemId` is `openapi.ItemId`.
func qualified(expr ast.Expr) string {
	text := types.ExprString(expr)
	if ident, ok := expr.(*ast.Ident); ok && types.Universe.Lookup(ident.Name) == nil {
		return "openapi." + text
	}
	return text
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
		fmt.Fprintf(&b, "\t%q: {UseCase: %q, Status: %d, Query: %#v, IfMatch: %t, IfMatchAs: %q, StepUp: %t, BodyRequired: %t, ETag: %q},\n",
			d.OperationID, d.UseCase, d.Status, d.Query, d.IfMatch, d.IfMatchAs, d.StepUp, d.BodyRequired, d.ETag)
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
		if d.AnswerType != "" {
			fmt.Fprintf(&b, "\tvar answer %s\n", d.AnswerType)
			answer = "&answer"
		}
		fmt.Fprintf(&b, "\tc.door(w, r, restDoors[%q], usecase.Input{%s}, %s, %s, %s)\n}\n",
			d.OperationID, strings.Join(in, ", "), params, body, answer)
	}
	return format.Source(b.Bytes())
}
