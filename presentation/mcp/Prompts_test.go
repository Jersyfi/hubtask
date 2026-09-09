// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"strings"
	"testing"

	port "github.com/Jersyfi/hubtask/core/port/ai"
)

// The prompts half of the server (J-12). What is asked here is that the published prompts reach a
// client with their versions and their declared arguments, that an argument nobody declared is
// refused by name, and - the one that matters most - that no path exists which would put material
// into the instruction.

func weeklyReview() port.Prompt {
	return port.Prompt{
		ID: "weekly-review", Version: "v1",
		Title:       "Weekly review of a collection",
		Description: "Reviews one collection's week.",
		Instruction: "You are helping somebody look back at a week of their own work.",
		Arguments: []port.PromptArgument{
			{Name: "collection", Description: "The collection to review.", Required: true, Resource: "containers"},
			{Name: "focus", Description: "What the person wants out of it."},
		},
	}
}

// promptStore is the slice of the store this server needs.
type promptStore struct {
	prompts []port.Prompt
}

// Published filters, exactly as the real store does: a fake that published everything it held
// would let a test pass that the store would fail.
func (s promptStore) Published() []port.Prompt {
	published := make([]port.Prompt, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		if prompt.Published() {
			published = append(published, prompt)
		}
	}
	return published
}

func (s promptStore) Get(id string) (port.Prompt, error) {
	for _, prompt := range s.prompts {
		if prompt.ID == id {
			return prompt, nil
		}
	}
	return port.Prompt{}, port.ErrPromptUnknown
}

func serverWithPrompts() Server {
	server := serverWith(&catalogue{})
	server.Prompts = promptStore{prompts: []port.Prompt{weeklyReview()}}
	return server
}

// The list carries what a client needs in order to call: the name with its version, what the prompt
// is for, and what to send it.
func TestThePromptListCarriesTheVersionAndTheArguments(t *testing.T) {
	answer := rpc(t, serverWithPrompts(), `{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`, true)

	result, _ := answer["result"].(map[string]any)
	prompts, _ := result["prompts"].([]any)
	if len(prompts) != 1 {
		t.Fatalf("%d prompts, want the one that is published", len(prompts))
	}

	prompt, _ := prompts[0].(map[string]any)
	if prompt["name"] != "weekly-review@v1" {
		t.Errorf("the prompt is called %v, want its name and its version", prompt["name"])
	}
	if prompt["description"] == "" {
		t.Error("the prompt carries no description for a client to show")
	}

	arguments, _ := prompt["arguments"].([]any)
	if len(arguments) != 2 {
		t.Fatalf("%d declared arguments", len(arguments))
	}
	first, _ := arguments[0].(map[string]any)
	if first["name"] != "collection" || first["required"] != true {
		t.Errorf("the first argument is %v, want a required collection", first)
	}
	second, _ := arguments[1].(map[string]any)
	if second["required"] != false {
		t.Errorf("the focus is required: %v", second)
	}
}

// The rule that renders every prompt, and the guardrail inside it: the instruction is the system
// message and every filled-in argument is a user message after it. There is no code path that would
// put material into the instruction, which is what turns "content is data, not instructions" from a
// rule somebody remembers into a shape of the code (ai-first.md §1.3).
func TestAPromptRendersTheInstructionAndTheMaterialAsSeparateMessages(t *testing.T) {
	answer := rpc(t, serverWithPrompts(),
		`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"weekly-review",`+
			`"arguments":{"collection":"0192f000-0000-7000-8000-00000000000c","focus":"what did I drop"}}}`,
		true)

	result, _ := answer["result"].(map[string]any)
	messages, _ := result["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("the prompt rendered %d messages, want the instruction and both arguments", len(messages))
	}

	system, _ := messages[0].(map[string]any)
	if system["role"] != "system" {
		t.Errorf("the instruction is a %v message", system["role"])
	}
	instruction, _ := system["content"].(map[string]any)
	if text, _ := instruction["text"].(string); !strings.Contains(text, "look back at a week") {
		t.Errorf("the system message is not the instruction: %q", text)
	}
	if strings.Contains(instruction["text"].(string), "what did I drop") {
		t.Error("the material reached the instruction")
	}

	// The resource argument is a link rather than content: the client resolves it through
	// resources/read, which is where the permission is asked.
	link, _ := messages[1].(map[string]any)
	content, _ := link["content"].(map[string]any)
	if content["type"] != "resource_link" {
		t.Fatalf("the collection was rendered as %v", content)
	}
	if content["uri"] != "hubtask://containers/0192f000-0000-7000-8000-00000000000c" {
		t.Errorf("the link points at %v", content["uri"])
	}
	if link["role"] != "user" {
		t.Errorf("the material is a %v message, want a user message", link["role"])
	}

	focus, _ := messages[2].(map[string]any)
	material, _ := focus["content"].(map[string]any)
	if material["text"] != "what did I drop" {
		t.Errorf("the focus was rendered as %v", material)
	}
}

// An argument nobody declared is refused by name rather than ignored, the way a use case input is:
// a caller that misspelled one has to learn it here rather than from a review that quietly left out
// what it asked about.
func TestAnUndeclaredPromptArgumentIsRefusedByName(t *testing.T) {
	answer := rpc(t, serverWithPrompts(),
		`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"weekly-review",`+
			`"arguments":{"collection":"0192f000-0000-7000-8000-00000000000c","focuss":"typo"}}}`,
		true)

	failure, _ := answer["error"].(map[string]any)
	if failure["code"] != float64(codeInvalidParams) {
		t.Fatalf("an undeclared argument answered %v", answer)
	}
	if message, _ := failure["message"].(string); !strings.Contains(message, "focuss") {
		t.Errorf("the refusal does not name the argument: %q", message)
	}
}

// Everything else a client can get wrong, and all of it correctable from the answer.
func TestWhatAPromptCallCanGetWrong(t *testing.T) {
	for name, c := range map[string]struct{ params, says string }{
		"a prompt nobody publishes": {
			`{"name":"delete-everything"}`, "unknown prompt",
		},
		"a version this build no longer carries": {
			`{"name":"weekly-review@v9","arguments":{"collection":"0192f000-0000-7000-8000-00000000000c"}}`,
			"unknown prompt version",
		},
		"a required argument left out": {
			`{"name":"weekly-review","arguments":{"focus":"what did I drop"}}`, "required",
		},
		"a resource argument that is not an identifier": {
			`{"name":"weekly-review","arguments":{"collection":"the-shopping-one"}}`, "identifier",
		},
	} {
		t.Run(name, func(t *testing.T) {
			answer := rpc(t, serverWithPrompts(),
				`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":`+c.params+`}`, true)

			failure, _ := answer["error"].(map[string]any)
			if failure["code"] != float64(codeInvalidParams) {
				t.Fatalf("answered %v", answer)
			}
			if message, _ := failure["message"].(string); !strings.Contains(message, c.says) {
				t.Errorf("the refusal says %q, want it to mention %q", message, c.says)
			}
		})
	}
}

// A version a client pins is a version it gets, or a refusal. Handing it the newest text instead
// would be the one failure pinning exists to prevent.
func TestAPinnedPromptVersionIsHonoured(t *testing.T) {
	answer := rpc(t, serverWithPrompts(),
		`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"weekly-review@v1",`+
			`"arguments":{"collection":"0192f000-0000-7000-8000-00000000000c"}}}`, true)

	if failure, refused := answer["error"]; refused {
		t.Fatalf("the pinned version was refused: %v", failure)
	}
}

// An installation running without the store declares no prompts capability and serves no prompt
// method - the rule this server has always stated about itself.
func TestWithoutAStoreThePromptsAreNotClaimed(t *testing.T) {
	server := serverWith(&catalogue{})

	answer := rpc(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, true)
	result, _ := answer["result"].(map[string]any)
	capabilities, _ := result["capabilities"].(map[string]any)
	if _, claimed := capabilities["prompts"]; claimed {
		t.Error("a server with no prompt store announces prompts")
	}

	for _, method := range []string{"prompts/list", "prompts/get"} {
		refused := rpc(t, server, `{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`, true)
		failure, _ := refused["error"].(map[string]any)
		if failure["code"] != float64(codeMethodNotFound) {
			t.Errorf("%s answered %v, want method not found", method, refused)
		}
	}
}

// And with one, the capability is declared - because the methods exist.
func TestWithAStoreThePromptsAreClaimed(t *testing.T) {
	answer := rpc(t, serverWithPrompts(), `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, true)

	result, _ := answer["result"].(map[string]any)
	capabilities, _ := result["capabilities"].(map[string]any)
	prompts, claimed := capabilities["prompts"].(map[string]any)
	if !claimed {
		t.Fatalf("the server serves prompts and does not announce them: %v", capabilities)
	}
	if prompts["listChanged"] != false {
		t.Errorf("the server promises notifications it cannot send: %v", prompts)
	}
}

// A prompt the store holds but nobody published is not reachable by name either. The four the
// product asks its own provider are written for one call site with one expected answer shape, and
// an agent asking for one would get an instruction to answer JSON nobody asked for.
func TestAnUnpublishedPromptCannotBeAskedForByName(t *testing.T) {
	// The store answers it - the adapters need it - and the server still refuses to hand it out.
	internal := port.Prompt{ID: "summarize", Version: "v1", Instruction: "You summarise."}
	server := serverWith(&catalogue{})
	server.Prompts = promptStore{prompts: []port.Prompt{weeklyReview(), internal}}

	listed := rpc(t, server, `{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`, true)
	result, _ := listed["result"].(map[string]any)
	if prompts, _ := result["prompts"].([]any); len(prompts) != 1 {
		t.Errorf("the list carries %d prompts, want only the published one", len(prompts))
	}

	answer := rpc(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"summarize"}}`, true)

	failure, _ := answer["error"].(map[string]any)
	if failure["code"] != float64(codeInvalidParams) {
		t.Errorf("an internal prompt was handed out: %v", answer)
	}
}
