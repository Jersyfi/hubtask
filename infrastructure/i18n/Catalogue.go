// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package i18n renders message codes into sentences.
//
// The backend emits no display text: an answer carries a code and its parameters, and whoever has
// a person in front of them builds the sentence (ADR-0011, i18n-l10n.md §3). This package is the
// other half of that bargain for the clients that ship with the server - the CLI, and the mail an
// account receives.
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/locales"
)

// SourceLocale is the language the catalogue is written in and the end of every fallback chain
// (i18n-l10n.md §2, §3).
const SourceLocale = "en"

// metadataPrefix marks a key that is not a message. The catalogue documents itself in `_comment`,
// and a renderer that offered that as a message would be offering the reader a note to the
// translators.
const metadataPrefix = "_"

// catalogueSuffix is what a catalogue file is called: the tag, then this.
const catalogueSuffix = ".json"

// Catalogue is one locale's messages, keyed by message code.
//
// It is immutable once loaded and safe to share: nothing writes to the map after Load returns.
type Catalogue struct {
	messages map[string]string
}

// LoadEnglish parses the embedded source catalogue.
func LoadEnglish() (Catalogue, error) {
	catalogues, err := LoadEmbedded()
	if err != nil {
		return Catalogue{}, err
	}
	return catalogues[SourceLocale], nil
}

// LoadEmbedded parses every catalogue compiled into the binary, keyed by its lower-cased tag.
//
// The source language must be among them: it is the end of every fallback chain, and a build
// without it has nothing to fall back to.
func LoadEmbedded() (map[string]Catalogue, error) {
	catalogues, err := LoadDirectory(locales.Files)
	if err != nil {
		return nil, err
	}
	if _, present := catalogues[SourceLocale]; !present {
		return nil, fmt.Errorf("reading the message catalogues: the source language %s is not embedded", SourceLocale)
	}
	return catalogues, nil
}

// LoadDirectory parses every `<tag>.json` at the root of a file system, keyed by the lower-cased
// tag. It is what the embedded catalogues and an operator's override directory (i18n-l10n.md §1)
// have in common, so the two are one reader.
//
// A file whose name is not a well-formed BCP 47 tag is refused rather than skipped: a catalogue
// nobody can ask for is a typo, and a typo that is silently ignored is how a translation goes
// missing without a trace. A subdirectory is ignored, so that a directory an operator mounts may
// hold whatever else it holds.
func LoadDirectory(files fs.FS) (map[string]Catalogue, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("reading the message catalogues: %w", err)
	}

	catalogues := make(map[string]Catalogue, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, catalogueSuffix) {
			continue
		}
		tag, ok := shared.LanguageTag(strings.TrimSuffix(name, catalogueSuffix))
		if !ok || tag == "" {
			return nil, fmt.Errorf("reading the message catalogue %s: the file name is not a language tag", name)
		}
		raw, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("reading the message catalogue %s: %w", name, err)
		}
		catalogue, err := load(raw)
		if err != nil {
			return nil, fmt.Errorf("reading the message catalogue %s: %w", name, err)
		}
		catalogues[strings.ToLower(tag)] = catalogue
	}
	return catalogues, nil
}

func load(raw []byte) (Catalogue, error) {
	var entries map[string]string
	if err := json.Unmarshal(raw, &entries); err != nil {
		return Catalogue{}, fmt.Errorf("not a flat map of codes to messages: %w", err)
	}

	messages := make(map[string]string, len(entries))
	for code, message := range entries {
		if strings.HasPrefix(code, metadataPrefix) {
			continue
		}
		messages[code] = message
	}
	return Catalogue{messages: messages}, nil
}

// Message renders the message for a code. The second return value says whether the code was
// known; an unknown code renders as itself, which is the fallback i18n-l10n.md §3 prescribes -
// a code on the screen is ugly, an empty screen is a bug report nobody can act on.
//
// Parameters are substituted by name. A placeholder with no parameter is left standing rather
// than blanked: `{limit_bytes}` on the screen says a value went missing, an empty gap says
// nothing at all.
func (c Catalogue) Message(code string, params map[string]string) (string, bool) {
	message, known := c.messages[code]
	if !known {
		return code, false
	}
	return substitute(message, params), true
}

// Has reports whether the catalogue knows a code, without rendering it. What a caller with two
// codes in hand - the contract code and the more specific detail code - needs in order to choose
// between them.
func (c Catalogue) Has(code string) bool {
	_, known := c.messages[code]
	return known
}

// Len is how many messages the catalogue holds. What a coverage report divides by.
func (c Catalogue) Len() int {
	return len(c.messages)
}

// Codes answers every code the catalogue knows, sorted. For the gates that compare a translation
// against its source; not for rendering.
func (c Catalogue) Codes() []string {
	codes := make([]string, 0, len(c.messages))
	for code := range c.messages {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// Pattern answers the raw message for a code, unrendered. For the gates.
func (c Catalogue) Pattern(code string) (string, bool) {
	message, known := c.messages[code]
	return message, known
}

// overlaid answers a catalogue with every message of `over` on top of this one: a key in both is
// the overlay's, a key in one is that one's. What an operator's file does to the embedded
// catalogue of the same tag - key by key, never as a whole (i18n-l10n.md §1).
func (c Catalogue) overlaid(over Catalogue) Catalogue {
	merged := make(map[string]string, len(c.messages)+len(over.messages))
	for code, message := range c.messages {
		merged[code] = message
	}
	for code, message := range over.messages {
		merged[code] = message
	}
	return Catalogue{messages: merged}
}

// substitute replaces `{name}` with the parameter of that name.
//
// This is the simple-argument subset of ICU MessageFormat, which is all the catalogues use.
// That is not an assumption but a checked property: Catalogue_test.go refuses a message with a
// plural, a select or a format style, so that adding one turns a build red here rather than
// printing braces at a user.
func substitute(message string, params map[string]string) string {
	if len(params) == 0 || !strings.ContainsRune(message, '{') {
		return message
	}

	var out strings.Builder
	out.Grow(len(message))
	rest := message
	for {
		before, after, found := strings.Cut(rest, "{")
		out.WriteString(before)
		if !found {
			return out.String()
		}

		name, tail, closed := strings.Cut(after, "}")
		if !closed {
			// An unterminated brace is not a placeholder. Written out as it stands.
			out.WriteString("{")
			out.WriteString(after)
			return out.String()
		}
		if value, ok := params[name]; ok {
			out.WriteString(value)
		} else {
			out.WriteString("{")
			out.WriteString(name)
			out.WriteString("}")
		}
		rest = tail
	}
}
