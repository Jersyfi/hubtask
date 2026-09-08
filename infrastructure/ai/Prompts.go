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
		instruction := strings.TrimSpace(string(text))
		if instruction == "" {
			return Store{}, fmt.Errorf("the prompt %s is empty", entry.Name())
		}
		if held, seen := newest[id]; seen && held >= number {
			continue
		}
		newest[id] = number
		store.byID[id] = port.Prompt{ID: id, Version: version, Instruction: instruction}
	}

	store.ids = make([]string, 0, len(store.byID))
	for id := range store.byID {
		store.ids = append(store.ids, id)
	}
	sort.Strings(store.ids)
	return store, nil
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
)
