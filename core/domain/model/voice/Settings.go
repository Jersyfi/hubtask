// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxAliases bounds the names a workspace sets: enough for every list a household or a team
// speaks to, few enough to read through on one page.
const MaxAliases = 100

// MaxAliasLength bounds one name.
const MaxAliasLength = 60

// MaxSkillIDLength bounds a skill identifier, which Amazon writes as amzn1.ask.skill.<uuid>.
const MaxSkillIDLength = 120

// Settings are a workspace's voice settings.
type Settings struct {
	Enabled bool
	// DefaultCollectionID takes what is said without a place: "Butter" alone.
	DefaultCollectionID shared.ID
	// SkillID is the workspace's own skill, for a self-hosted skill; empty where the installation's
	// published skill is used.
	SkillID string
	// InvocationName is what follows "Alexa, sag" / "Alexa, ask": empty for the language's
	// default (DefaultInvocation). The same name goes into the console's model and is what people
	// say, so it is the administrator's to choose - a name the Echo in their kitchen hears.
	InvocationName string
	Aliases        []Alias
}

// defaultInvocations are two words, which Amazon requires, that the language's speech
// recognition writes apart and says the same in every launch phrase. German joins two nouns into
// one ("Listen Helfer" arrives as "listenhelfer" and never matches) but never an adjective with its
// noun, and a feminine noun without an article reads the same after "öffne", "frage" and "sage":
// "digitale pinnwand", confirmed on an Echo across a room. Neither default is a word Alexa keeps
// for its own lists ("Liste"), its to-dos ("Aufgaben") or Amazon's wish list ("Merkzettel").
var defaultInvocations = map[Language]string{German: "digitale pinnwand", English: "list helper"}

// DefaultInvocation is the name a workspace that chose none is spoken to by.
func DefaultInvocation(language Language) string { return defaultInvocations[language] }

// Invocation is the name people say in this language: the workspace's own, or the default.
func (s Settings) Invocation(language Language) string {
	if name := NormalInvocation(s.InvocationName); name != "" {
		return name
	}
	return DefaultInvocation(language)
}

// NormalInvocation is a name as Amazon takes it: lower case, single spaces.
func NormalInvocation(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// reservedInvocationWords are the words Amazon refuses in an invocation name: the wake words, the
// words for what a skill is, the launch phrases and the connecting words of both languages - a
// name holding one would be cut where Alexa expects the request to begin.
var reservedInvocationWords = map[string]bool{
	"alexa": true, "amazon": true, "echo": true, "computer": true, "skill": true, "app": true,
	"run": true, "start": true, "play": true, "resume": true, "use": true, "launch": true, "ask": true,
	"open": true, "tell": true, "load": true, "begin": true, "enable": true,
	"to": true, "from": true, "using": true, "with": true, "about": true, "for": true, "that": true,
	"by": true, "if": true, "and": true, "whether": true,
	"öffne": true, "starte": true, "frage": true, "frag": true, "sage": true, "sag": true,
	"benutze": true, "dass": true, "ob": true, "nach": true, "von": true, "vom": true, "mit": true,
	"für": true, "und": true,
}

// articles may not stand as a name's second word: "a bicycle", "eine pinnwand" name nothing.
var articles = map[string]bool{
	"a": true, "an": true, "the": true, "der": true, "die": true, "das": true, "den": true, "dem": true,
	"des": true, "ein": true, "eine": true, "einen": true, "einem": true, "einer": true,
}

// validInvocation applies Amazon's rules: two words at least; letters (the language's own
// included), spaces, a dot or an apostrophe; two to fifty characters; no wake word; starting with
// a letter.
func validInvocation(name string) bool {
	if len(strings.Fields(name)) < 2 || utf8.RuneCountInString(name) > 50 {
		return false
	}
	for index, r := range name {
		letter := unicode.IsLetter(r)
		if index == 0 && !letter {
			return false
		}
		if !letter && r != ' ' && r != '.' && r != '\'' {
			return false
		}
	}
	words := strings.Fields(name)
	for _, word := range words {
		if reservedInvocationWords[strings.Trim(word, ".'")] {
			return false
		}
	}
	return !articles[words[1]]
}

// Alias is a name somebody chose for a place, as it is spoken.
type Alias struct {
	ID           shared.ID
	Phrase       string
	CollectionID shared.ID
	LabelID      shared.ID
}

// Validate refuses what cannot be stored: too many names, a name too long or empty, two names
// that fold to the same, a skill identifier of the wrong shape.
func (s Settings) Validate() error {
	var findings []shared.FieldError
	if len(s.Aliases) > MaxAliases {
		findings = append(findings, shared.FieldError{Path: "/aliases", Code: "voice.too_many_aliases"})
	}
	seen := map[string]bool{}
	for _, alias := range s.Aliases {
		phrase := strings.TrimSpace(alias.Phrase)
		switch {
		case phrase == "" || utf8.RuneCountInString(phrase) > MaxAliasLength:
			findings = append(findings, shared.FieldError{Path: "/aliases", Code: "voice.alias_invalid"})
		case alias.CollectionID.IsZero():
			findings = append(findings, shared.FieldError{Path: "/aliases", Code: "voice.alias_place_required"})
		case seen[Fold(phrase)]:
			findings = append(findings, shared.FieldError{Path: "/aliases", Code: "voice.alias_twice"})
		}
		seen[Fold(phrase)] = true
	}
	if skill := strings.TrimSpace(s.SkillID); skill != "" &&
		(len(skill) > MaxSkillIDLength || !strings.HasPrefix(skill, "amzn1.")) {
		findings = append(findings, shared.FieldError{Path: "/skill_id", Code: "voice.skill_id_invalid"})
	}
	if name := NormalInvocation(s.InvocationName); name != "" && !validInvocation(name) {
		findings = append(findings, shared.FieldError{Path: "/invocation_name", Code: "voice.invocation_invalid"})
	}
	if len(findings) > 0 {
		return shared.ErrValidation.WithDetail("voice.settings_invalid").WithFields(findings...)
	}
	return nil
}
