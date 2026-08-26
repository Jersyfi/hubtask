// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Consent is one person's agreement to one optional processing purpose (data-protection.md §4,
// Art. 21).
//
// It is a record rather than a flag, and the difference is what an operator has to be able to
// show: not "is this allowed now" but "was it allowed then". A withdrawal therefore ends the
// record rather than deleting it - a consent that vanished when it was withdrawn would leave the
// processing that happened while it stood unexplained.
type Consent struct {
	ID        shared.ID
	AccountID shared.ID
	// Purpose is what was agreed to: `ai_processing`, `metering`, `email_content`. Open text
	// rather than an enum, because a purpose is what an installation offers rather than what this
	// model fixes - and a purpose nobody offers is a record nothing reads.
	Purpose   string
	Granted   bool
	GrantedAt time.Time
	RevokedAt time.Time
	// Source is who recorded it: the person, an administrator on their behalf, or the
	// configuration. It matters to an operator showing that a consent was actually given.
	Source string
}

// The sources the schema allows.
const (
	SourceUser        = "user"
	SourceTenantAdmin = "tenant_admin"
	SourceConfig      = "config"
)

// Withdrawn reports a consent that no longer stands.
func (c Consent) Withdrawn() bool { return !c.RevokedAt.IsZero() }

// InForce reports a consent that was granted and not taken back.
func (c Consent) InForce() bool { return c.Granted && !c.Withdrawn() }

// maxPurpose bounds the purpose. It is an identifier an installation chose, not a sentence.
const maxPurpose = 100

// NewWithdrawal records a consent being taken back.
//
// It answers a *record* rather than a change to one, because withdrawing a consent that was never
// granted is a legitimate act with a legitimate answer: the person says no, and the record says
// they said no. An operator reading it afterwards sees a decision rather than a gap.
func NewWithdrawal(id, accountID shared.ID, purpose, source string, now time.Time) (Consent, error) {
	cleaned := strings.TrimSpace(purpose)
	if cleaned == "" || len([]rune(cleaned)) > maxPurpose {
		return Consent{}, invalid(CodePurposeRequired, "/purpose")
	}
	if source == "" {
		source = SourceUser
	}

	return Consent{
		ID: id, AccountID: accountID, Purpose: cleaned,
		Granted: false, GrantedAt: now, RevokedAt: now, Source: source,
	}, nil
}

// Withdraw ends a standing consent.
func (c Consent) Withdraw(at time.Time) Consent {
	if c.Withdrawn() {
		// Already taken back. Keeping the first moment is the point: when the processing stopped
		// is what an operator has to be able to show, and a second withdrawal moves nothing.
		return c
	}
	withdrawn := c
	withdrawn.Granted = false
	withdrawn.RevokedAt = at
	return withdrawn
}
