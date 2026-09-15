// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"strings"

	"golang.org/x/text/language"

	port "github.com/Jersyfi/hubtask/core/port/i18n"
)

// maxAcceptLanguageLength bounds what is parsed. The header comes from outside, it ends up in a
// context and in a response, and a client with an opinion about 400 languages is not one worth
// answering carefully.
const maxAcceptLanguageLength = 256

var _ port.Negotiator = Renderer{}

// Negotiate is the port's method: the header's tags in the client's order of preference, each
// asked of the matcher, and the first one the installation serves with any confidence wins. None
// served answers the client's first preference as it stands.
//
// The client's tag rather than the catalogue's, on purpose: what travels on the actor is a
// preference, and a preference keeps its region. The renderer matches again when it renders.
func (r Renderer) Negotiate(acceptLanguage string) string {
	header := strings.TrimSpace(acceptLanguage)
	if header == "" || len(header) > maxAcceptLanguageLength {
		return ""
	}
	// An unusable header falls back rather than failing: a wrong language is a nuisance, a
	// refused request is an outage. ParseAcceptLanguage refuses the whole header on one bad
	// entry, which is the honest reading of a header nobody can act on.
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return ""
	}
	stated := tags[:0]
	for _, tag := range tags {
		// `*` parses as "mul" - every language - which names none. Dropped: it says the client
		// has no preference, and no preference is what an absent header says too.
		if tag == language.Und || tag.String() == "mul" {
			continue
		}
		stated = append(stated, tag)
	}
	if len(stated) == 0 {
		return ""
	}
	for _, tag := range stated {
		if _, confidence := r.match(tag); confidence != language.No {
			return tag.String()
		}
	}
	return stated[0].String()
}

// match asks the matcher which of the catalogues present a tag lands on. The index is into
// Locales(); the confidence is No when nothing but the default fits.
func (r Renderer) match(tag language.Tag) (int, language.Confidence) {
	_, index, confidence := r.matcher.Match(tag)
	return index, confidence
}
