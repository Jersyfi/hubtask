// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package i18n renders message codes into sentences.
//
// The backend emits no display text: an answer carries a code and its parameters, and whoever has
// a person in front of them builds the sentence (ADR-0011, i18n-l10n.md §3). This package is the
// other half of that bargain for the clients that ship with the server - today the CLI, later the
// web frontend's fallback.
package i18n

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jersyfi/hubtask/locales"
)

// metadataPrefix marks a key that is not a message. The catalogue documents itself in `_comment`,
// and a renderer that offered that as a message would be offering the reader a note to the
// translators.
const metadataPrefix = "_"

// Catalogue is one locale's messages, keyed by message code.
//
// It is immutable once loaded and safe to share: nothing writes to the map after Load returns.
type Catalogue struct {
	messages map[string]string
}

// LoadEnglish parses the embedded source catalogue.
//
// English rather than a locale argument, because there is one file today. When de.json arrives,
// this grows a sibling that takes a tag and falls back to this one - the fallback chain is a
// decision for the day there is something to fall back from.
func LoadEnglish() (Catalogue, error) {
	return load(locales.English)
}

func load(raw []byte) (Catalogue, error) {
	var entries map[string]string
	if err := json.Unmarshal(raw, &entries); err != nil {
		return Catalogue{}, fmt.Errorf("reading the message catalogue: %w", err)
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

// substitute replaces `{name}` with the parameter of that name.
//
// This is the simple-argument subset of ICU MessageFormat, which is all the source catalogue uses.
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
