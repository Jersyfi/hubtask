// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// Prompt is one versioned instruction, as it was written down (ADR-0049 decision 3).
//
// It carries no user content and cannot: the whole reason a prompt is a stored artefact rather
// than a string built at the call site is that the instruction and the content have to stay two
// different things, and the way to keep them apart is to make the instruction something nobody
// concatenates to.
type Prompt struct {
	// ID names the prompt, e.g. "suggest-fields". Stable across versions.
	ID string
	// Version is the version this text is, e.g. "v1". It travels into a suggestion's provenance,
	// so a suggestion made a year ago can still be traced to the words that produced it.
	Version string
	// Instruction is the system message. Written in English and never localised through the
	// message catalogue: it is addressed to a model, not to a person reading the product, which
	// is the same reason a tool description is not display text (rule 8, ADR-0011).
	Instruction string
}

// Ask builds the request that puts this instruction and that content in front of a model.
//
// It exists so that there is exactly one place where the two are combined, and so that the
// combining cannot go wrong: the instruction is RoleSystem, the content is RoleUser, and there is
// no third path. A caller assembling a CompletionRequest by hand could put a title into the system
// message; a caller using this cannot, which is what turns ai-first.md §1.3's "content is data, not
// instructions" from a rule somebody remembers into a shape of the code.
func (p Prompt) Ask(content string) CompletionRequest {
	return CompletionRequest{
		PromptID:      p.ID,
		PromptVersion: p.Version,
		Messages: []Message{
			{Role: RoleSystem, Content: p.Instruction},
			{Role: RoleUser, Content: content},
		},
	}
}

// ErrPromptUnknown is a prompt this build does not carry. Internal rather than validation: a
// prompt id is written in the source beside the call that uses it, so an unknown one is a defect
// rather than somebody's input.
var ErrPromptUnknown = shared.ErrInternal.WithDetail("ai.prompt_unknown")

// Prompts is where the instructions live.
//
// One store, read by whatever asks a model something and published by the MCP prompts endpoint
// (J-12): two stores would be how one prompt comes to exist in two versions, and a suggestion's
// recorded prompt version would then name a text that depends on who is reading it.
type Prompts interface {
	// Get answers the newest version of one prompt.
	Get(id string) (Prompt, error)
	// IDs is every prompt this build carries, in a stable order. What the MCP endpoint lists and
	// what a test walks to check that each one parses.
	IDs() []string
}
