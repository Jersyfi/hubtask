// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sync holds the offline synchronisation context's own entities (offline-sync.md §10):
// what the server keeps about the devices that synchronise, beside the change log they read.
package sync

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The bounds of what a device says about itself (api/openapi.yaml, SyncPullRequest).
const (
	PlatformMaxLength    = 40
	DisplayNameMaxLength = 120
)

// Device is one installation of a client on one machine, registered to one account
// (offline-sync.md §6, §10). The identifier is the client's - minted once, kept in its own store,
// sent with every pull and push - and it is what a push's `op_id`s and a change's `device_id` name.
type Device struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	// Platform and DisplayName are what the device said about itself, last value standing. Both
	// are personal data - a name somebody typed, a class of machine they own - and travel to the
	// device list and nowhere else (rule 10).
	Platform    string
	DisplayName string
	// LastSeq is where the device last stood in the log: the position of the last page it
	// pulled. Zero for a device that has never finished a page.
	LastSeq    int64
	LastSeenAt time.Time
	CreatedAt  time.Time
	// Blocked is a forgotten device: every contact from the identifier is refused until the
	// sweep removes the row and the client mints a new identifier. The row stays rather than
	// going at once, so that the list shows what a person ended and so that an identifier cannot
	// register afresh while somebody still holds the old one.
	Blocked bool
	// CredentialID is the credential of the request that last touched the device - a session,
	// for a client that signed in; a token's identifier for one that did not. Forgetting the
	// device revokes it where it is a session and does nothing where it is not (migration 0082).
	CredentialID shared.ID
}

// Contact is what one pull or push says about the device making it.
type Contact struct {
	DeviceID     shared.ID
	AccountID    shared.ID
	CredentialID shared.ID
	// Platform and DisplayName are optional: empty leaves what the row holds.
	Platform    string
	DisplayName string
	// LastSeq is the position after the page, zero when the contact did not finish one.
	LastSeq int64
	Now     time.Time
}

// Validate bounds what a client says about its device, and normalises it: trimmed, and refused
// when longer than the contract allows. The identifier is required and has to be one the client
// minted - a UUIDv7 - because it is what every change of the device is later attributed to
// (offline-sync.md §9, requirement 1).
func (c Contact) Validate() (Contact, error) {
	if c.DeviceID.IsZero() {
		return Contact{}, shared.ErrValidation.
			WithDetail("sync.device_required").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.device_required"})
	}
	if !c.DeviceID.IsUUIDv7() {
		return Contact{}, shared.ErrValidation.
			WithDetail("sync.id_not_uuidv7").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.id_not_uuidv7"})
	}
	c.Platform = strings.TrimSpace(c.Platform)
	if utf8.RuneCountInString(c.Platform) > PlatformMaxLength {
		return Contact{}, shared.ErrValidation.
			WithDetail("sync.device_platform_too_long").
			WithFields(shared.FieldError{Path: "/platform", Code: "sync.device_platform_too_long"})
	}
	c.DisplayName = strings.TrimSpace(c.DisplayName)
	if utf8.RuneCountInString(c.DisplayName) > DisplayNameMaxLength {
		return Contact{}, shared.ErrValidation.
			WithDetail("sync.device_name_too_long").
			WithFields(shared.FieldError{Path: "/display_name", Code: "sync.device_name_too_long"})
	}
	return c, nil
}
