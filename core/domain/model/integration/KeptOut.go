// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Whose content stays out of AI, and how (data-protection.md §4.1, UC-PRV-05 check 7).
//
// **One exclusion, many reasons.** A person is kept out because they objected where the workspace
// lets them, or - a second reason - because their processing is restricted (Art. 18). Every path
// that hands content to a model asks the same KeptOut, read in the same transaction as the
// material, so one prompt sees one state: what a kept-out person created is not material, their
// comments are not material, and their name is replaced in whatever remains.
//
// **Nothing names who was left out.** A prompt with somebody's content dropped reads as a prompt
// about less content; material with nothing left is refused in the same words as material that
// was empty to begin with (ai.nothing_to_work_with), so that a colleague cannot tell, entry by
// entry, who objected (UC-PRV-05 check 12).

// KeptOutReason is why a person's content stays out.
type KeptOutReason string

// KeptOutObjected is a person's own objection: their newest ai_processing record is a withdrawal,
// the workspace offers keeping one's content out, and more than one person works here.
const KeptOutObjected KeptOutReason = "OBJECTED"

// KeptPerson is one person kept out, with every spelling of them a text might carry.
type KeptPerson struct {
	AccountID shared.ID
	Reason    KeptOutReason
	// Names are the display name, the sign-in name and the address. Scrub also replaces each part
	// of a name of three letters or more: "Kim" is how a colleague writes about Kim Keeper.
	Names []string
}

// KeptOut is the people whose content stays out of one prompt. The zero value keeps nobody out.
type KeptOut struct {
	people    map[shared.ID]KeptPerson
	addresses map[string]bool
	pattern   *regexp.Regexp
}

// NewKeptOut builds the exclusion. A person named twice - by two reasons - keeps the first.
func NewKeptOut(people []KeptPerson) KeptOut {
	kept := KeptOut{people: map[shared.ID]KeptPerson{}, addresses: map[string]bool{}}
	for _, person := range people {
		if person.AccountID.IsZero() {
			continue
		}
		if _, already := kept.people[person.AccountID]; already {
			continue
		}
		kept.people[person.AccountID] = person
		for _, name := range person.Names {
			if strings.Contains(name, "@") {
				kept.addresses[strings.ToLower(strings.TrimSpace(name))] = true
			}
		}
	}
	kept.pattern = namePattern(kept.people)
	return kept
}

// Has reports whether an account's content stays out. The zero account - a former user, the
// system - is never kept out by this: there is nobody behind it to have objected.
func (k KeptOut) Has(accountID shared.ID) bool {
	if accountID.IsZero() {
		return false
	}
	_, held := k.people[accountID]
	return held
}

// HasAddress reports whether a sender's address is a kept-out person's: what they mailed into the
// inbox is theirs.
func (k KeptOut) HasAddress(address string) bool {
	return k.addresses[strings.ToLower(strings.TrimSpace(address))]
}

// ReasonOf answers why an account is kept out, empty where it is not.
func (k KeptOut) ReasonOf(accountID shared.ID) KeptOutReason {
	return k.people[accountID].Reason
}

// Empty reports that nobody is kept out.
func (k KeptOut) Empty() bool { return len(k.people) == 0 }

// Placeholder is what a kept-out person's name becomes in a prompt: a fixed token rather than a
// sentence (rule 8), and the same for everybody, so that the prompt says nothing about who it was.
const Placeholder = "[person]"

// Scrub replaces every spelling of a kept-out person in a text with the placeholder - whole words
// only, ignoring case, so that "Kimberley" is not "Kim".
func (k KeptOut) Scrub(text string) string {
	if k.pattern == nil || text == "" {
		return text
	}
	matches := k.pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		if !boundaryBefore(text, start) || !boundaryAfter(text, end) {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString(Placeholder)
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

func boundaryBefore(text string, at int) bool {
	if at == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:at])
	return !wordRune(r)
}

func boundaryAfter(text string, at int) bool {
	if at >= len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[at:])
	return !wordRune(r)
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// minNamePart is the shortest spelling replaced on its own. Shorter parts - "Li", "de" - are
// words in too many sentences to be read as a name.
const minNamePart = 3

// namePattern is one alternation over every spelling, longest first, so that "Kim Keeper" is
// replaced whole rather than as "Kim" and "Keeper".
func namePattern(people map[shared.ID]KeptPerson) *regexp.Regexp {
	seen := map[string]bool{}
	var spellings []string
	add := func(spelling string) {
		spelling = strings.TrimSpace(spelling)
		if utf8.RuneCountInString(spelling) < minNamePart || seen[strings.ToLower(spelling)] {
			return
		}
		seen[strings.ToLower(spelling)] = true
		spellings = append(spellings, spelling)
	}
	for _, person := range people {
		for _, name := range person.Names {
			add(name)
			if strings.Contains(name, "@") {
				continue
			}
			for _, part := range strings.FieldsFunc(name, func(r rune) bool { return !wordRune(r) && r != '-' && r != '\'' }) {
				add(part)
			}
		}
	}
	if len(spellings) == 0 {
		return nil
	}
	sort.SliceStable(spellings, func(i, j int) bool {
		if len(spellings[i]) != len(spellings[j]) {
			return len(spellings[i]) > len(spellings[j])
		}
		return spellings[i] < spellings[j]
	})
	quoted := make([]string, len(spellings))
	for i, spelling := range spellings {
		quoted[i] = regexp.QuoteMeta(spelling)
	}
	return regexp.MustCompile(`(?i)(?:` + strings.Join(quoted, "|") + `)`)
}

// NothingToWorkWith is the one answer for material with nothing left - empty to begin with, or
// all of it kept out. One answer for both, word for word, so that it says nothing about who wrote
// what (UC-PRV-05 check 12).
func NothingToWorkWith() error {
	return shared.ErrValidation.WithDetail("ai.nothing_to_work_with")
}

// KeptOutOfAi is the answer to a person who keeps their own content out and asks for an AI
// action: no AI action is offered to them (UC-PRV-05 check 7). They know their own choice.
func KeptOutOfAi() error {
	return shared.ErrForbidden.WithDetail("ai.kept_out")
}
