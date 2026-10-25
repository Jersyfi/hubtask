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

// defaultInvocations are two plain words each: an Echo across a room hears real words far better
// than a made-up one ("hubtask" was heard as "hub task", "hab task"), and neither is a word Alexa
// keeps for its own lists ("Liste") or its own to-dos ("Aufgaben").
var defaultInvocations = map[Language]string{German: "listen helfer", English: "list helper"}

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

// reservedInvocationWords are the words Amazon refuses in an invocation name: the wake words and
// the words for what a skill is.
var reservedInvocationWords = map[string]bool{
	"alexa": true, "amazon": true, "echo": true, "computer": true, "skill": true, "app": true,
}

// validInvocation applies Amazon's rules: letters (the language's own included), spaces, a dot or
// an apostrophe; two to fifty characters; no wake word; starting with a letter.
func validInvocation(name string) bool {
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 50 {
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
	for _, word := range strings.Fields(name) {
		if reservedInvocationWords[strings.Trim(word, ".'")] {
			return false
		}
	}
	return true
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
