// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"strings"
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
	Aliases []Alias
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
	if len(findings) > 0 {
		return shared.ErrValidation.WithDetail("voice.settings_invalid").WithFields(findings...)
	}
	return nil
}
