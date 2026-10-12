// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AiPersonOptOut is the key `ai.person_opt_out`: whether each person may keep the content they
// authored out of AI (data-protection.md §4.1).
type AiPersonOptOut string

const (
	// AiOptOutOffered lets each person keep their own content out - the product's default.
	AiOptOutOffered AiPersonOptOut = "OFFERED"
	// AiOptOutNotOffered makes AI part of everybody's work. Only a workspace can choose it, and
	// only naming a legal basis that does not rest on each person's agreement.
	AiOptOutNotOffered AiPersonOptOut = "NOT_OFFERED"
)

var aiOptOutValues = []AiPersonOptOut{AiOptOutOffered, AiOptOutNotOffered}

// AiPersonOptOutValues is the closed set, in the contract's order.
func AiPersonOptOutValues() []AiPersonOptOut { return slices.Clone(aiOptOutValues) }

// Valid reports whether the value is one of the defined ones.
func (p AiPersonOptOut) Valid() bool { return slices.Contains(aiOptOutValues, p) }

// SettableAbove checks a value the installation or a plan writes. Neither can name the legal
// basis NOT_OFFERED needs - only the controller can - so they hold a workspace at OFFERED or
// leave it alone (milestone PH, decision 4).
func (p AiPersonOptOut) SettableAbove() error {
	switch p {
	case AiOptOutOffered:
		return nil
	case AiOptOutNotOffered:
		return shared.ErrValidation.
			WithDetail("ai.opt_out_workspace_only").
			WithFields(shared.FieldError{Path: "/ai/person_opt_out", Code: "ai.opt_out_workspace_only"})
	}
	return shared.ErrValidation.
		WithDetail("auth.policy_value_invalid").
		WithParams(map[string]string{"switch": "ai." + string(AiSwitchPersonOptOut)}).
		WithFields(shared.FieldError{Path: "/ai/person_opt_out", Code: "auth.policy_value_invalid"})
}

// AiOptOutBasis is the legal basis a workspace names when it makes AI part of everybody's work.
// A closed set, the three the use case names: Hubtask does not judge whether the basis holds,
// but it has to say it in every language, which a free text would not allow.
type AiOptOutBasis string

const (
	AiBasisEmploymentContract AiOptOutBasis = "EMPLOYMENT_CONTRACT"
	AiBasisWorksAgreement     AiOptOutBasis = "WORKS_AGREEMENT"
	AiBasisLegalObligation    AiOptOutBasis = "LEGAL_OBLIGATION"
)

var aiOptOutBases = []AiOptOutBasis{AiBasisEmploymentContract, AiBasisWorksAgreement, AiBasisLegalObligation}

// AiOptOutBases is the closed set, in the contract's order.
func AiOptOutBases() []AiOptOutBasis { return slices.Clone(aiOptOutBases) }

// Valid reports whether the basis is one of the defined ones.
func (b AiOptOutBasis) Valid() bool { return slices.Contains(aiOptOutBases, b) }

// MaxOptOutContact bounds whom to ask: a name, a role or an address, not a letter.
const MaxOptOutContact = 200

// AiPosition is a workspace's position on people keeping their content out, as proposed.
type AiPosition struct {
	PersonOptOut AiPersonOptOut
	Basis        AiOptOutBasis
	Contact      string
}

// NewAiPosition validates a proposed position. OFFERED carries no basis and no contact - a basis
// kept beside a position that does not need one would be shown to nobody and believed by somebody.
func NewAiPosition(optOut AiPersonOptOut, basis AiOptOutBasis, contact string) (AiPosition, error) {
	if !optOut.Valid() {
		return AiPosition{}, fieldError("/person_opt_out", "auth.policy_value_invalid")
	}
	if optOut == AiOptOutOffered {
		return AiPosition{PersonOptOut: AiOptOutOffered}, nil
	}
	switch {
	case basis == "":
		return AiPosition{}, fieldError("/basis", "ai.opt_out_basis_required")
	case !basis.Valid():
		return AiPosition{}, fieldError("/basis", "ai.opt_out_basis_unknown")
	}
	cleaned := strings.TrimSpace(contact)
	if len([]rune(cleaned)) > MaxOptOutContact {
		return AiPosition{}, fieldError("/contact", "ai.opt_out_contact_too_long")
	}
	return AiPosition{PersonOptOut: optOut, Basis: basis, Contact: cleaned}, nil
}

// PositionChange checks a workspace's proposed position against the rule: OFFERED is always the
// workspace's own to choose, NOT_OFFERED only where no level above locked the key.
func (r AiRule) PositionChange(proposed AiPosition) error {
	if proposed.PersonOptOut == AiOptOutOffered {
		return nil
	}
	if origin := r.LockOf(AiSwitchPersonOptOut); origin != identity.LockNone {
		return shared.ErrValidation.
			WithDetail("auth.policy_locked").
			WithParams(map[string]string{"switch": "ai." + string(AiSwitchPersonOptOut), "origin": string(origin)}).
			WithFields(shared.FieldError{
				Path: "/person_opt_out", Code: "auth.policy_locked",
				Params: map[string]string{"origin": string(origin)},
			})
	}
	return nil
}

// OptOutOffered reports whether each person may keep their own content out under this rule.
func (r AiRule) OptOutOffered() bool { return r.PersonOptOut != AiOptOutNotOffered }
