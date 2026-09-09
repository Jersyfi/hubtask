// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"sort"
	"time"
)

// Workspace is a tenant as the people inside it see it (F4-01).
//
// `Tenant` is deliberately what the installation provisions; this is that plus what the workspace
// has configured since. The two are separate types rather than one grown type because the readers
// are separate: the control plane provisions and suspends without ever asking what a workspace
// configured, and a member reads the configuration without being entitled to the listing the
// control plane holds.
type Workspace struct {
	Tenant
	Settings  WorkspaceSettings
	UpdatedAt time.Time
	Version   int
}

// WorkspaceSettings is the modelled half of the tenant's settings document.
//
// The document itself is the adapter's shape and stays there (`TenantSettings`' discipline): what
// this type carries is the keys this build knows, and the ones it does not are neither read here
// nor lost on a write.
type WorkspaceSettings struct {
	// RequireAdminTotp demands a second factor of the OWNER and ADMIN role holders
	// (security.md §5, H-02). The sign-in path has read it since 0.6.0.
	RequireAdminTotp bool
}

// WorkspaceChange is a merge-patch, typed: a nil pointer is a key the caller did not send, and
// therefore a field that does not move. There is no "clear it" for any of the four - a workspace
// always has a name, a locale, a zone and an answer to the enforcement question - which is why
// the type has no way to express one.
type WorkspaceChange struct {
	DisplayName      *string
	DefaultLocale    *string
	DefaultTimeZone  *string
	RequireAdminTotp *bool
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
func (w Workspace) With(change WorkspaceChange) (Workspace, []FieldChange, error) {
	changed := w
	moved := map[string]FieldChange{}

	if change.DisplayName != nil {
		name, err := ValidDisplayName(*change.DisplayName)
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

	if change.RequireAdminTotp != nil && *change.RequireAdminTotp != w.Settings.RequireAdminTotp {
		moved["require_admin_totp"] = FieldChange{
			Field: "require_admin_totp",
			From:  boolText(w.Settings.RequireAdminTotp),
			To:    boolText(*change.RequireAdminTotp),
		}
		changed.Settings.RequireAdminTotp = *change.RequireAdminTotp
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
