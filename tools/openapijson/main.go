// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command openapijson writes api/openapi.yaml as api/openapi.json (P-01).
//
// The specification is YAML because people write it; what renders it is JavaScript that ships no
// YAML parser. `make generate` runs this beside oapi-codegen and sqlc, the output is committed
// like every other generated file (project-structure.md §6), and CI's no-diff check keeps the two
// in step - so the JSON is not a second description of the contract but the same one, in the
// encoding the website and the SDK generators read.
//
// Key order is preserved. encoding/json's map sorts keys, which would put `responses` before
// `parameters` and `404` before `200`; a reference rendered in that order reads wrongly, and the
// specification's order is the author's. So the YAML is decoded as a node tree and written by
// hand, with encoding/json used only for the scalars it is right about.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: openapijson <openapi.yaml> <openapi.json>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1]) //nolint:gosec // G703: the file the Makefile names; reading it is the job
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := Convert(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// 0o644: a generated source file of the repository, read by everything and written by this.
	if err := os.WriteFile(os.Args[2], out, 0o644); err != nil { //nolint:gosec // G306, G703: the output the Makefile names
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Convert turns a YAML document into indented JSON with the document's own key order.
func Convert(raw []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("openapijson: parse: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, fmt.Errorf("openapijson: expected one document")
	}
	var buf bytes.Buffer
	if err := write(&buf, root.Content[0], 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func write(buf *bytes.Buffer, node *yaml.Node, depth int) error {
	switch node.Kind {
	case yaml.AliasNode:
		return write(buf, node.Alias, depth)
	case yaml.MappingNode:
		if len(node.Content) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind == yaml.ScalarNode && key.Tag == "!!merge" {
				return fmt.Errorf("openapijson: merge keys are not supported (line %d)", key.Line)
			}
			indent(buf, depth+1)
			k, err := json.Marshal(scalarString(key))
			if err != nil {
				return err
			}
			buf.Write(k)
			buf.WriteString(": ")
			if err := write(buf, value, depth+1); err != nil {
				return err
			}
			if i+2 < len(node.Content) {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		indent(buf, depth)
		buf.WriteByte('}')
		return nil
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		for i, item := range node.Content {
			indent(buf, depth+1)
			if err := write(buf, item, depth+1); err != nil {
				return err
			}
			if i+1 < len(node.Content) {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		indent(buf, depth)
		buf.WriteByte(']')
		return nil
	case yaml.ScalarNode:
		return writeScalar(buf, node)
	}
	return fmt.Errorf("openapijson: unsupported node kind %d at line %d", node.Kind, node.Line)
}

// scalarString is a mapping key: always a string in JSON, whatever YAML resolved it to. A status
// code written as `200:` is the string "200" in the specification's own terms.
func scalarString(node *yaml.Node) string {
	return node.Value
}

// writeScalar writes a scalar as the JSON value YAML's core schema resolves it to. A quoted
// value is a string even when it looks like a number, which is how `version: "1"` stays a string.
func writeScalar(buf *bytes.Buffer, node *yaml.Node) error {
	if node.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return writeJSON(buf, node.Value)
	}
	switch node.Tag {
	case "!!null":
		buf.WriteString("null")
		return nil
	case "!!bool":
		b, err := strconv.ParseBool(strings.ToLower(node.Value))
		if err != nil {
			return fmt.Errorf("openapijson: bool at line %d: %w", node.Line, err)
		}
		return writeJSON(buf, b)
	case "!!int":
		i, err := strconv.ParseInt(node.Value, 0, 64)
		if err != nil {
			return fmt.Errorf("openapijson: int at line %d: %w", node.Line, err)
		}
		return writeJSON(buf, i)
	case "!!float":
		f, err := strconv.ParseFloat(node.Value, 64)
		if err != nil {
			return fmt.Errorf("openapijson: float at line %d: %w", node.Line, err)
		}
		return writeJSON(buf, f)
	}
	return writeJSON(buf, node.Value)
}

// writeJSON encodes one scalar. Through an encoder rather than json.Marshal so that `<` and `>`
// in a description stay themselves: the output is read by a renderer, not embedded in HTML.
func writeJSON(buf *bytes.Buffer, value any) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return err
	}
	buf.Write(bytes.TrimRight(tmp.Bytes(), "\n"))
	return nil
}

func indent(buf *bytes.Buffer, depth int) {
	for range depth {
		buf.WriteString("  ")
	}
}
