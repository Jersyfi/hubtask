// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
)

// The prompts an agent may ask for (J-12, ai-first.md §1.1).
//
// **Why this is not display text, and why rule 8 is not being bent.** Rule 8 forbids the backend
// producing what a person reads: a sentence in the product's interface is a message code plus
// parameters, rendered in that person's language by their own client (ADR-0011). A prompt is not
// that. It is addressed to a *model*, and to the client that operates one, exactly as a tool's
// description is - `ToolRegistry.go` has relied on the same distinction since the tools half
// existed. The test is not "does a human ever see these words" (a developer sees an OpenAPI
// description too); it is "is this the product speaking to a person in their language". It is not.
//
// And the versioning is the reason it must stay that way. A prompt carries a version, and a
// suggestion records the version that produced it, so that a suggestion made a year ago can be
// traced to the words behind it (ADR-0049 decision 3). A prompt translated through the message
// catalogue would be a prompt whose recorded version named a text that depended on who was reading
// it - which is precisely the thing versioning it exists to prevent.
//
// One store, shared with the outbound adapters (`core/port/ai.Prompts`). Two stores is how one
// prompt comes to exist in two versions.

// PromptArgument is one entry of a prompt's declared arguments in `prompts/list`.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// PromptSummary is one entry of `prompts/list`.
type PromptSummary struct {
	Name        string           `json:"name"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments"`
}

// PromptsOf renders the published prompts, in the store's own order.
//
// The version is not a field of its own, because MCP has none: it is carried in the name, as
// `weekly-review@v1`. A client that pins a prompt then pins the text, which is the whole point of
// versioning it - and `prompts/get` accepts the bare name too, so a client that does not care about
// versions asks for the newest and gets it.
func PromptsOf(prompts []port.Prompt) []PromptSummary {
	summaries := make([]PromptSummary, 0, len(prompts))
	for _, prompt := range prompts {
		arguments := make([]PromptArgument, 0, len(prompt.Arguments))
		for _, argument := range prompt.Arguments {
			arguments = append(arguments, PromptArgument{
				Name: argument.Name, Description: argument.Description, Required: argument.Required,
			})
		}
		summaries = append(summaries, PromptSummary{
			Name:        PromptName(prompt),
			Title:       prompt.Title,
			Description: prompt.Description,
			Arguments:   arguments,
		})
	}
	return summaries
}

// PromptName is how a prompt is addressed: `<id>@<version>`.
func PromptName(prompt port.Prompt) string { return prompt.ID + "@" + prompt.Version }

// ErrPromptArguments is a call this server cannot render into a message sequence.
//
// A validation failure rather than a protocol one, and reported as `codeInvalidParams` for the
// reason an unknown tool is: the caller can correct it, and the correction is in the message.
type ErrPromptArguments struct{ Reason string }

func (e ErrPromptArguments) Error() string { return e.Reason }

// PromptMessages renders one prompt and its arguments into the sequence a client sends to a model.
//
// One rule, and no per-prompt program behind it: **the instruction is the system message, and every
// declared argument the caller filled in becomes one user message after it**, in the order the
// prompt declared them. That is `Prompt.Ask` generalised from one argument to several, and it keeps
// the same guardrail - the instruction and the material are two different messages, and nothing in
// this package concatenates them (ai-first.md §1.3). A prompt cannot be written whose material ends
// up in the system message, because there is no code path that would put it there.
//
// A resource argument becomes a link rather than content. The client resolves it through
// `resources/read`, which is where the permission is asked (ADR-0051) - so a prompt naming a
// collection cannot hand out a collection the caller may not open, and this server reads nothing
// while rendering a prompt.
func PromptMessages(prompt port.Prompt, arguments map[string]string) ([]map[string]any, error) {
	declared := make(map[string]bool, len(prompt.Arguments))
	for _, argument := range prompt.Arguments {
		declared[argument.Name] = true
	}
	// Unknown arguments are refused by name rather than ignored, the way a use case input is: a
	// caller that misspelled one has to learn that here rather than from a review that silently
	// left out what it asked about.
	unknown := make([]string, 0, len(arguments))
	for name := range arguments {
		if !declared[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, ErrPromptArguments{
			Reason: "unknown prompt arguments: " + strings.Join(unknown, ", "),
		}
	}

	messages := []map[string]any{textMessage("system", prompt.Instruction)}

	for _, argument := range prompt.Arguments {
		value := strings.TrimSpace(arguments[argument.Name])
		if value == "" {
			if argument.Required {
				return nil, ErrPromptArguments{
					Reason: fmt.Sprintf("the prompt argument %s is required", argument.Name),
				}
			}
			continue
		}

		if argument.Resource == "" {
			messages = append(messages, textMessage("user", value))
			continue
		}

		id, err := shared.ParseID(value)
		if err != nil {
			return nil, ErrPromptArguments{
				Reason: fmt.Sprintf("the prompt argument %s does not name an identifier", argument.Name),
			}
		}
		messages = append(messages, map[string]any{
			"role": "user",
			"content": map[string]any{
				"type":     "resource_link",
				"uri":      URIOf(argument.Resource, id),
				"name":     argument.Name,
				"mimeType": MimeType,
			},
		})
	}
	return messages, nil
}

func textMessage(role, text string) map[string]any {
	return map[string]any{
		"role":    role,
		"content": map[string]any{"type": "text", "text": text},
	}
}
