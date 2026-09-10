// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	port "github.com/Jersyfi/hubtask/core/port/ai"
)

// The prompts, compiled in (ADR-0049 decision 3).
//
// Files rather than string literals, and the version in the *filename* rather than in front
// matter, so that changing a prompt is adding a file and the old one stays readable. That is what
// makes a suggestion's recorded `prompt_version` resolvable a year later: the text that produced it
// is still in the repository, under the name the suggestion names.
//
// Embedded rather than read from disk, for the reason the message catalogue is: a container that
// lost a file at runtime would answer a caller with an internal error instead of failing to start.
//
//go:embed prompts/*.md
var promptFiles embed.FS

// Store is the prompt store.
//
// It is built once at startup and read many times, so the parsing happens in the constructor and
// every Get after that is a map lookup - a template compiled per call would put file parsing on
// the path of every suggestion.
type Store struct {
	byID map[string]port.Prompt
	ids  []string
}

var _ port.Prompts = Store{}

// NewStore reads the embedded prompts, keeping the newest version of each.
//
// A malformed name is an error rather than a skipped file. A prompt silently missing is the worst
// outcome here: the feature that wanted it would answer "AI is unavailable", which is the same
// thing an operator sees when they have configured no provider, and the two would be
// indistinguishable from outside.
func NewStore() (Store, error) {
	entries, err := fs.ReadDir(promptFiles, "prompts")
	if err != nil {
		return Store{}, fmt.Errorf("reading the prompt directory: %w", err)
	}

	newest := make(map[string]int, len(entries))
	store := Store{byID: make(map[string]port.Prompt, len(entries))}

	for _, entry := range entries {
		id, version, number, err := parsePromptName(entry.Name())
		if err != nil {
			return Store{}, err
		}
		text, err := fs.ReadFile(promptFiles, "prompts/"+entry.Name())
		if err != nil {
			return Store{}, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		prompt, err := parsePrompt(entry.Name(), string(text))
		if err != nil {
			return Store{}, err
		}
		if held, seen := newest[id]; seen && held >= number {
			continue
		}
		newest[id] = number
		prompt.ID, prompt.Version = id, version
		store.byID[id] = prompt
	}

	store.ids = make([]string, 0, len(store.byID))
	for id := range store.byID {
		store.ids = append(store.ids, id)
	}
	sort.Strings(store.ids)
	return store, nil
}

// parsePrompt reads one file: an optional header, then the instruction.
//
// The header is what makes a prompt publishable to an agent (J-12). It is delimited by `---` lines
// and hand-parsed rather than read as YAML, because a header of five keys is not worth a
// dependency - and a dependency is a supply chain decision this file is not the place to make.
//
// A file without a header is a prompt the product asks its own provider, and is the ordinary case:
// the whole file is the instruction, exactly as it was before J-12.
func parsePrompt(name, text string) (port.Prompt, error) {
	body := text
	var prompt port.Prompt

	if header, rest, found := cutHeader(text); found {
		for _, line := range strings.Split(header, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, split := strings.Cut(line, ":")
			if !split {
				return port.Prompt{}, fmt.Errorf("the prompt %s has the header line %q, want key: value", name, line)
			}
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)

			switch key {
			case "title":
				prompt.Title = value
			case "description":
				prompt.Description = value
			case "argument":
				argument, err := parsePromptArgument(name, value)
				if err != nil {
					return port.Prompt{}, err
				}
				prompt.Arguments = append(prompt.Arguments, argument)
			default:
				return port.Prompt{}, fmt.Errorf("the prompt %s declares %q, which nothing reads", name, key)
			}
		}
		body = rest
	}

	prompt.Instruction = strings.TrimSpace(body)
	if prompt.Instruction == "" {
		return port.Prompt{}, fmt.Errorf("the prompt %s is empty", name)
	}
	prompt.Answers = answerKeys(prompt.Instruction)
	// A published prompt has to say what it is and what to send it. A title without a description
	// is a prompt an agent's client lists and nobody can tell what it does.
	if prompt.Title != "" && prompt.Description == "" {
		return port.Prompt{}, fmt.Errorf("the prompt %s has a title and no description", name)
	}
	if prompt.Title == "" && len(prompt.Arguments) > 0 {
		return port.Prompt{}, fmt.Errorf("the prompt %s declares arguments and is not published", name)
	}
	return prompt, nil
}

// answerKeys reads the keys of the answer shape a prompt documents.
//
// The convention is one fenced block holding the object the model is to answer with, and it is a
// convention with a gate behind it: `test/architecture` compares these keys against the allow list
// that decides which of them the code keeps, and a prompt asking for a key nobody kept is what
// made 0.7.5 necessary (K-01). Prose bullets are not read - a prompt that documents a field only
// in a sentence documents it to a model and to nobody else.
//
// The block is not parsed as JSON, deliberately. An example written for a model has ellipses in it
// where the values would be, which no JSON parser accepts and every model understands; what is
// wanted here is the *shape*, and the shape is the quoted names at the object's own level.
//
// A prompt with no fenced object asks for prose, which is every published prompt (J-12) and no
// completion prompt this product asks its own provider.
func answerKeys(instruction string) []string {
	block, found := fencedObject(instruction)
	if !found {
		return nil
	}

	var keys []string
	depth := 0
	for index := 0; index < len(block); index++ {
		switch block[index] {
		case '{':
			depth++
		case '}':
			depth--
		case '"':
			key, end, closed := quoted(block, index)
			if !closed {
				return keys
			}
			index = end
			// Only the object's own level, and only a name a colon follows: the strings inside
			// `{"labels": ["…"]}` are values, and the names inside a nested object belong to the
			// node rather than to the answer.
			if depth == 1 && labels(block, end+1) {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// fencedObject answers the first fenced block containing an object.
func fencedObject(instruction string) (string, bool) {
	rest := instruction
	for {
		opened := strings.Index(rest, "```")
		if opened < 0 {
			return "", false
		}
		rest = rest[opened+3:]
		if line := strings.IndexByte(rest, '\n'); line >= 0 {
			rest = rest[line+1:]
		}
		closed := strings.Index(rest, "```")
		if closed < 0 {
			return "", false
		}
		block := rest[:closed]
		rest = rest[closed+3:]
		if strings.Contains(block, "{") {
			return block, true
		}
	}
}

// quoted reads the JSON string beginning at `start`, answering it and the index of its closing
// quote.
func quoted(text string, start int) (value string, end int, closed bool) {
	var read strings.Builder
	for index := start + 1; index < len(text); index++ {
		switch text[index] {
		case '\\':
			index++
			if index < len(text) {
				read.WriteByte(text[index])
			}
		case '"':
			return read.String(), index, true
		default:
			read.WriteByte(text[index])
		}
	}
	return "", len(text), false
}

// labels reports whether what follows is a colon, which is what makes a string a key.
func labels(text string, from int) bool {
	for index := from; index < len(text); index++ {
		switch text[index] {
		case ' ', '\t', '\r', '\n':
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}

// cutHeader takes the `---`-delimited block off the front of a file, where there is one.
func cutHeader(text string) (header, body string, found bool) {
	rest, opened := strings.CutPrefix(strings.TrimSpace(text), "---\n")
	if !opened {
		return "", text, false
	}
	header, body, closed := strings.Cut(rest, "\n---")
	if !closed {
		return "", text, false
	}
	return header, body, true
}

// parsePromptArgument reads `<name> | <kind> | <required> | <description>`.
//
// Pipes rather than prose, because every other shape invites a description containing whatever
// character was chosen as the separator. The kind is `text` or `resource:<segment>`, naming one of
// the URI shapes J-11 published.
func parsePromptArgument(file, line string) (port.PromptArgument, error) {
	parts := strings.Split(line, "|")
	if len(parts) != 4 {
		return port.PromptArgument{}, fmt.Errorf(
			"the prompt %s declares the argument %q, want `name | kind | required | description`",
			file, line)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	argument := port.PromptArgument{Name: parts[0], Description: parts[3]}
	if argument.Name == "" || argument.Description == "" {
		return port.PromptArgument{}, fmt.Errorf(
			"the prompt %s declares an argument without a name or a description: %q", file, line)
	}

	switch kind := parts[1]; {
	case kind == "text":
	case strings.HasPrefix(kind, "resource:"):
		argument.Resource = strings.TrimPrefix(kind, "resource:")
		if argument.Resource == "" {
			return port.PromptArgument{}, fmt.Errorf(
				"the prompt %s declares a resource argument naming no kind: %q", file, line)
		}
	default:
		return port.PromptArgument{}, fmt.Errorf(
			"the prompt %s declares the argument kind %q, want text or resource:<kind>", file, kind)
	}

	switch parts[2] {
	case "required":
		argument.Required = true
	case "optional":
	default:
		return port.PromptArgument{}, fmt.Errorf(
			"the prompt %s says the argument %s is %q, want required or optional",
			file, argument.Name, parts[2])
	}
	return argument, nil
}

// parsePromptName splits `<id>.v<n>.md`. The number is returned beside the version string so that
// v10 sorts after v9, which a string comparison would get wrong the first time it mattered.
func parsePromptName(name string) (id, version string, number int, err error) {
	base, found := strings.CutSuffix(name, ".md")
	if !found {
		return "", "", 0, fmt.Errorf("the prompt %s is not a .md file", name)
	}
	id, version, found = strings.Cut(base, ".")
	if !found || id == "" {
		return "", "", 0, fmt.Errorf("the prompt %s has no <id>.<version> name", name)
	}
	digits, found := strings.CutPrefix(version, "v")
	if !found {
		return "", "", 0, fmt.Errorf("the prompt %s has the version %q, want v<n>", name, version)
	}
	number, err = strconv.Atoi(digits)
	if err != nil || number < 1 {
		return "", "", 0, fmt.Errorf("the prompt %s has the version %q, want v<n>", name, version)
	}
	return id, version, number, nil
}

// Get answers the newest version of one prompt.
func (s Store) Get(id string) (port.Prompt, error) {
	prompt, held := s.byID[id]
	if !held {
		return port.Prompt{}, port.ErrPromptUnknown.WithParams(map[string]string{"prompt": id})
	}
	return prompt, nil
}

// IDs is every prompt this build carries, sorted.
func (s Store) IDs() []string { return append([]string(nil), s.ids...) }

// The prompt identifiers, named here so a caller cites a constant rather than a string that can be
// misspelled into an internal error at run time.
const (
	// PromptSuggestFields turns a rough note into the fields of a task (J-06, J-08).
	PromptSuggestFields = "suggest-fields"
	// PromptWeeklyReview is the one prompt written for an agent rather than for this product's own
	// provider (J-12). It is published over MCP and never asked by anything in `core`.
	PromptWeeklyReview = "weekly-review"
)

// Published answers the prompts an agent may ask for, in the store's own order.
//
// The store holds both kinds - what the product asks its provider, and what an agent may ask for -
// because two stores is how one prompt comes to exist in two versions, and a suggestion's recorded
// prompt version would then name a text that depends on who is reading it (ADR-0049 decision 3).
// This is the filter, not a second store.
func (s Store) Published() []port.Prompt {
	published := make([]port.Prompt, 0, len(s.ids))
	for _, id := range s.ids {
		if prompt := s.byID[id]; prompt.Published() {
			published = append(published, prompt)
		}
	}
	return published
}
