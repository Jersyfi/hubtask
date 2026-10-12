// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// EmergencyAccessLasts is how long the workspace owner's emergency access to a private hub lasts
// (ADR-0073 §4). It is not extended: a second access while one runs is refused, and one after it
// ended is a new act, told to the members again.
const EmergencyAccessLasts = time.Hour

// MaxEmergencyReason bounds the reason in characters: a sentence or two the members read in a
// mail and on their hub, not a document.
const MaxEmergencyReason = 500

// EmergencyAccess is one opening of a private hub by the workspace's owner: the hub, who, why, and
// the hour. It grants nothing by itself; the authoriser reads a running one for its opener alone
// (identity.md §22).
type EmergencyAccess struct {
	ID       shared.ID
	TenantID shared.ID
	HubID    shared.ID
	// OpenedBy is zero once the opener's account is gone; the members' record stays.
	OpenedBy shared.ID
	// Reason is written to the hub's members. User content: never in a log, a metric or the trail
	// in clear (rule 10).
	Reason   string
	OpenedAt time.Time
	EndsAt   time.Time
}

// NewEmergencyAccessInput is what opening a hub needs decided.
type NewEmergencyAccessInput struct {
	ID       shared.ID
	TenantID shared.ID
	HubID    shared.ID
	OpenedBy shared.ID
	Reason   string
	At       time.Time
}

// NewEmergencyAccess opens an access for one hour from now, with the reason trimmed and bounded.
func NewEmergencyAccess(in NewEmergencyAccessInput) (EmergencyAccess, error) {
	reason := strings.TrimSpace(in.Reason)
	switch {
	case reason == "":
		return EmergencyAccess{}, shared.ErrValidation.
			WithDetail("containers.emergency_reason_required").
			WithFields(shared.FieldError{Path: "/reason", Code: "containers.emergency_reason_required"})
	case utf8.RuneCountInString(reason) > MaxEmergencyReason:
		return EmergencyAccess{}, shared.ErrValidation.
			WithDetail("containers.emergency_reason_too_long").
			WithFields(shared.FieldError{Path: "/reason", Code: "containers.emergency_reason_too_long"})
	}
	return EmergencyAccess{
		ID: in.ID, TenantID: in.TenantID, HubID: in.HubID, OpenedBy: in.OpenedBy,
		Reason: reason, OpenedAt: in.At, EndsAt: in.At.Add(EmergencyAccessLasts),
	}, nil
}

// RunningAt reports whether the access is still open at the moment.
func (a EmergencyAccess) RunningAt(now time.Time) bool { return now.Before(a.EndsAt) }

// ErrEmergencyNotPrivate refuses opening a shared hub: the owner reaches it already.
func ErrEmergencyNotPrivate() error {
	return shared.ErrConflict.WithDetail("containers.emergency_not_private")
}

// ErrEmergencyNotNeeded refuses opening a private hub the owner already holds a role in.
func ErrEmergencyNotNeeded() error {
	return shared.ErrConflict.WithDetail("containers.emergency_not_needed")
}

// ErrEmergencyRunning refuses a second access while the first runs, naming when it ends: the hour
// is not renewed.
func ErrEmergencyRunning(endsAt time.Time) error {
	return shared.ErrConflict.
		WithDetail("containers.emergency_running").
		WithParams(map[string]string{"ends_at": endsAt.UTC().Format(time.RFC3339)})
}

// ErrPrivateHubsOff refuses a new private hub where the workspace, or a level above it, does not
// allow private hubs (ADR-0073 §6). Every private hub already there stays private.
func ErrPrivateHubsOff() error {
	return shared.ErrForbidden.WithDetail("containers.private_hubs_off")
}
