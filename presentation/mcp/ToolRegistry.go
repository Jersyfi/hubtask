// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mcp exposes the use case catalogue to agents (ai-first.md §1.1, ADR-0012).
//
// The tools are generated from core/application/usecase.Registry rather than written out here.
// That is the whole design: a hand-maintained second list is a list that falls behind, and an
// agent interface that can do less than the API is the failure this project set out to avoid. A
// new use case is a new tool the moment it is registered - no code in this package changes.
package mcp

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// ProtocolVersion is the revision of the Model Context Protocol this server speaks. It is
// answered on initialize, so a client can decide whether it understands us.
const ProtocolVersion = "2025-06-18"

// Tool is one entry of tools/list.
type Tool struct {
	Name string `json:"name"`
	// Description carries the preconditions and side effects an agent needs in order to decide
	// whether to call the tool (ai-first.md §1.1). It is protocol documentation like an OpenAPI
	// description, not display text - what a user sees is rendered from a message code.
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations Annotations    `json:"annotations"`
}

// Annotations are the hints a client uses to decide whether to ask for confirmation before it
// calls (ai-first.md §1.1). Destructive operations are the ones an agent token may not even hold
// by default (§1.3).
type Annotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
}

// ToolsOf renders the catalogue as MCP tools, in the catalogue's own order.
func ToolsOf(descriptors []usecase.Descriptor) []Tool {
	tools := make([]Tool, 0, len(descriptors))

	for _, descriptor := range descriptors {
		description := descriptor.Summary
		if descriptor.SideEffects != "" {
			description += " " + descriptor.SideEffects
		}
		tools = append(tools, Tool{
			Name:        descriptor.MCPTool(),
			Description: description,
			InputSchema: InputSchema(descriptor.Input),
			Annotations: Annotations{
				ReadOnlyHint:    descriptor.ReadOnly,
				DestructiveHint: descriptor.Destructive,
			},
		})
	}
	return tools
}

// InputSchema turns the declared fields into JSON Schema.
//
// `additionalProperties: false` mirrors what the catalogue does with an input: an unknown field is
// refused, not ignored. An agent that sees the schema can then correct itself before the call
// rather than after a silent misplacement (ai-first.md §1.2).
func InputSchema(fields []usecase.Field) map[string]any {
	properties := make(map[string]any, len(fields))
	required := make([]string, 0, len(fields))

	for _, field := range fields {
		property := map[string]any{"type": jsonType(field.Kind)}
		if field.Kind == usecase.KindID {
			property["format"] = "uuid"
		}
		if len(field.Enum) > 0 {
			property["enum"] = field.Enum
		}
		if field.Description != "" {
			property["description"] = field.Description
		}
		properties[field.Name] = property

		if field.Required {
			required = append(required, field.Name)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// jsonType maps a declared kind onto the JSON Schema type. An identifier is a string with a
// format rather than a type of its own - JSON Schema has no uuid type, and an agent that only
// reads `type` still gets something it can satisfy.
func jsonType(kind usecase.Kind) string {
	switch kind {
	case usecase.KindBool:
		return "boolean"
	case usecase.KindInt:
		return "integer"
	default:
		return "string"
	}
}
