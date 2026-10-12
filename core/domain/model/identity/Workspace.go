// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"sort"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// Workspace is a tenant as the people inside it see it.
//
// `Tenant` is deliberately what the installation provisions; this is that plus what the workspace
// has configured since. The two are separate types rather than one grown type because the readers
// are separate: the control plane provisions and suspends without ever asking what a workspace
// configured, and a member reads the configuration without being entitled to the listing the
// control plane holds.
type Workspace struct {
	Tenant
	Settings WorkspaceSettings
	// PasswordOpening is an operator's opening of the password, where one was made (ADR-0078 §3).
	// Read with the row and written by the control plane alone: it is not in the settings document,
	// so nothing the workspace sends through its own form reaches it.
	PasswordOpening PasswordOpening
	UpdatedAt       time.Time
	Version         int
}

// WorkspaceSettings is the modelled half of the tenant's settings document.
//
// The document itself is the adapter's shape and stays there (`TenantSettings`' discipline): what
// this type carries is the keys this build knows, and the ones it does not are neither read here
// nor lost on a write.
type WorkspaceSettings struct {
	// RequireAdminTotp demands a second factor of the OWNER and ADMIN role holders (security.md §5,
	// identity.md §8).
	RequireAdminTotp bool
	// AuditAnchorTargetID names the workspace's backup target the audit chain's end is exported to
	// once a day (audit.md §3). Zero is anchoring switched off.
	AuditAnchorTargetID shared.ID
	// SignIn is what this workspace decided about signing in (ADR-0068 §2): a patch, because a
	// switch it never touched is the level above's rather than a zero of its own.
	SignIn PolicyPatch
	// Legal is what this workspace set for its own sign-in footer. Empty where the instance's
	// stand, which is what a B2C installation locks and a B2B one leaves open (identity.md §18).
	Legal LegalLinks
	// OfferedProviders are the installation's providers this workspace switched **on** (identity.md
	// §10.2).
	//
	// A list of the ones taken rather than of the ones refused, and that is the whole decision: an
	// installation's provider is "offered to every workspace, switched on in none" - offering it is
	// the installation's, taking it is the workspace's. A list of refusals would mean a provider
	// the installation adds tomorrow is on everywhere tonight.
	OfferedProviders []shared.ID
	// AiSources is this workspace's own narrowing of which AI sources it may use (ADR-0072 §2),
	// one of `ai.sources`' values, or empty where it narrowed nothing. A string rather than the
	// integration package's type, which already depends on this one; the value is checked where
	// it is written.
	//
	// Here, in the archived settings, and not beside the choice of source: a narrowing is an
	// administrator's restriction, and a restore must never widen what the workspace allows.
	AiSources string
	// WeekStart is the day this workspace's weeks start on for a member who chose none, or empty
	// where it decided nothing and the installation's day, then the language's, applies
	// (i18n-l10n.md §4). In the settings document rather than a column of its own, so the archive,
	// the export and every restore carry it as they carry the rest of the document.
	WeekStart WeekStart
	// AiPersonOptOut is this workspace's position on people keeping their own content out of AI
	// (data-protection.md §4.1): `NOT_OFFERED` where it made AI part of everybody's work, empty or
	// `OFFERED` otherwise. Strings for AiSources' reason; checked where written.
	//
	// In the archived settings, beside the narrowing: it is the controller's documented decision
	// with its legal basis, and a restore puts back what was decided.
	AiPersonOptOut string
	// AiOptOutBasis is the legal basis named for `NOT_OFFERED`; AiOptOutContact whom to ask about
	// it, empty for "an administrator of this workspace"; AiOptOutSince when the position was
	// taken - what tells a person whether they have been told yet.
	AiOptOutBasis   string
	AiOptOutContact string
	AiOptOutSince   time.Time
}

// Offers reports whether this workspace switched on a provider the installation offers.
func (w WorkspaceSettings) Offers(providerID shared.ID) bool {
	for _, offered := range w.OfferedProviders {
		if offered == providerID {
			return true
		}
	}
	return false
}

// WithOffer answers the settings with one installation provider switched on or off. A value, not a
// mutation: a settings document two callers could edit in place is one they would edit at once.
func (w WorkspaceSettings) WithOffer(providerID shared.ID, offered bool) WorkspaceSettings {
	kept := make([]shared.ID, 0, len(w.OfferedProviders)+1)
	for _, each := range w.OfferedProviders {
		if each != providerID {
			kept = append(kept, each)
		}
	}
	if offered {
		kept = append(kept, providerID)
	}
	w.OfferedProviders = kept
	return w
}

// SignInLayer is this workspace as a level of the resolution.
//
// `require_admin_totp` is folded in here rather than anywhere else: it is what `mfa_required_for`
// derives from, so a workspace that set the old boolean and never saw the new switch resolves to
// ADMINS - no stored row has to move, and no client that only knows the boolean breaks
// (ADR-0068 §1). An explicit `mfa_required_for` wins, because it is the newer statement of the
// same thing.
func (w WorkspaceSettings) SignInLayer() PolicyLayer {
	patch := w.SignIn
	if patch.MfaRequiredFor == nil && w.RequireAdminTotp {
		admins := MfaForAdmins
		patch.MfaRequiredFor = &admins
	}
	// A workspace locks nothing: there is nobody below it.
	return PolicyLayer{Patch: patch}
}

// LegalLayer is this workspace as a level of the four links' resolution.
func (w WorkspaceSettings) LegalLayer() LegalLayer { return LegalLayer{Links: w.Legal} }

// WorkspaceChange is a merge-patch, typed: a nil pointer is a key the caller did not send, and
// therefore a field that does not move. A workspace always has a name, a locale and a zone, so
// those three have no "clear it"; the week start does, and the empty string says it.
//
// `require_admin_totp` is not here any more. It was a second, unguarded way to set who needs a
// second factor, and it came apart from the rule (UC-ID-12 checks 1 and 2); a write of it is now
// translated into the rule's own switch, with the rule's step-up, lock and direction.
type WorkspaceChange struct {
	DisplayName     *string
	DefaultLocale   *string
	DefaultTimeZone *string
	// DefaultWeekStart is the one field with an absent state: the empty string clears it.
	DefaultWeekStart *string
}

// FieldChange is one field that moved, with what it moved from and to. The strings are the
// audit's, not a reader's: they are field names and values, never sentences (ADR-0011).
type FieldChange struct {
	Field string
	From  string
	To    string
}

// With applies the patch and answers the workspace as it would then stand, together with the
// fields that actually moved.
//
// A field set to the value it already holds is **not** a change: a client that sends the whole
// form back would otherwise write an audit entry saying nothing happened, every time. An empty
// change list is what the caller uses to skip the write altogether.
//
// The normaliser is for the display name, which is stored in normal form C (i18n-l10n.md §5).
func (w Workspace) With(change WorkspaceChange, form text.Normalizer) (Workspace, []FieldChange, error) {
	changed := w
	moved := map[string]FieldChange{}

	if change.DisplayName != nil {
		name, err := ValidDisplayName(*change.DisplayName, form)
		if err != nil {
			return Workspace{}, nil, err
		}
		if name != w.DisplayName {
			moved["display_name"] = FieldChange{Field: "display_name", From: w.DisplayName, To: name}
			changed.DisplayName = name
		}
	}

	if change.DefaultLocale != nil {
		locale, err := ValidDefaultLocale(*change.DefaultLocale)
		if err != nil {
			return Workspace{}, nil, err
		}
		if locale != w.DefaultLocale {
			moved["default_locale"] = FieldChange{
				Field: "default_locale", From: w.DefaultLocale, To: locale}
			changed.DefaultLocale = locale
		}
	}

	if change.DefaultTimeZone != nil {
		zone, err := ValidDefaultTimeZone(*change.DefaultTimeZone)
		if err != nil {
			return Workspace{}, nil, err
		}
		if zone != w.DefaultTimeZone {
			moved["default_time_zone"] = FieldChange{
				Field: "default_time_zone", From: w.DefaultTimeZone, To: zone}
			changed.DefaultTimeZone = zone
		}
	}

	if change.DefaultWeekStart != nil {
		day := WeekStart(strings.TrimSpace(*change.DefaultWeekStart))
		if day != "" && !day.Valid() {
			return Workspace{}, nil, shared.ErrValidation.
				WithDetail("workspace.week_start_invalid").
				WithParams(map[string]string{"value": string(day)}).
				WithFields(shared.FieldError{Path: "/default_week_start", Code: "workspace.week_start_invalid"})
		}
		if day != w.Settings.WeekStart {
			moved["default_week_start"] = FieldChange{
				Field: "default_week_start", From: string(w.Settings.WeekStart), To: string(day)}
			changed.Settings.WeekStart = day
		}
	}

	return changed, sortedChanges(moved), nil
}

// sortedChanges puts the fields in a stable order, because an audit entry whose change list
// depends on map iteration is an audit entry two readers describe differently.
func sortedChanges(moved map[string]FieldChange) []FieldChange {
	names := make([]string, 0, len(moved))
	for name := range moved {
		names = append(names, name)
	}
	sort.Strings(names)
	changes := make([]FieldChange, 0, len(names))
	for _, name := range names {
		changes = append(changes, moved[name])
	}
	return changes
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
