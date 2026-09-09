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
	// Title and Description are what an agent's client shows for this prompt, and they are the
	// reason a prompt is published at all (J-12): a prompt that describes itself is one somebody
	// wrote for an agent to use, and one that does not is the product's own instruction to its own
	// provider. Empty in the second case, and `Published` is what asks.
	//
	// They are protocol documentation, exactly like a tool's description and for exactly the same
	// reason: rule 8 forbids *display text* - what a person reads in their own language, rendered
	// from a message code - and this is addressed to the client that operates a model. A prompt
	// translated through the message catalogue would be a prompt whose recorded version named a
	// text that depended on who was reading it, which is the one thing versioning it exists to
	// prevent.
	Title       string
	Description string
	// Arguments are what a caller fills in. Declared, so a client can ask for them before it calls
	// and so an unknown one is refused by name rather than ignored - the same discipline a use
	// case input has (ADR-0012, `usecase.Field`).
	Arguments []PromptArgument
}

// PromptArgument is one thing a caller supplies when it asks for a prompt.
//
// Two shapes and no third, which is what keeps rendering one message sequence a rule rather than a
// per-prompt program:
//
//   - Text: the value is material, and becomes a user message. It is the shape `Ask` already has,
//     and it carries the same guardrail - the instruction is the system message, the material is
//     the user message, and nothing concatenates the two (ai-first.md §1.3).
//   - A resource: the value is an identifier, and becomes a link into the URI space J-11 published.
//     The client fetches it through `resources/read`, which is where the permission is asked - so a
//     prompt naming a collection cannot hand out a collection the caller may not open.
type PromptArgument struct {
	// Name is what the caller sends it under.
	Name string
	// Description says what to put there.
	Description string
	// Required refuses the call when it is missing.
	Required bool
	// Resource is the kind of resource an identifier addresses - "containers", "items", "views" -
	// or empty where the argument is material rather than an address.
	Resource string
}

// Published reports whether this prompt is one an agent may ask for.
//
// A prompt says so by describing itself. The four the product asks its own provider - suggest
// fields, decompose, summarise, classify - carry no title, and publishing them would offer an agent
// an instruction written for one specific call site with one specific expected answer shape.
func (p Prompt) Published() bool { return p.Title != "" }

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
