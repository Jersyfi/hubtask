// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A change of a workspace's address (multi-tenancy.md §3.1, ADR-0095 §3): the new main address
// checked as provisioning checks one, and the rule that nobody is locked out by it (P-16).

// TenantAddress is a workspace's short name and the host it answers at.
type TenantAddress struct {
	Slug string
	Host string
}

// NewTenantAddressInput is what a change names and what it changes from.
type NewTenantAddressInput struct {
	// Assigned is whether the installation assigns hosts (ADR-0095 §6).
	Assigned         bool
	RawHost, RawSlug string
	InstallationHost string
	Domains          text.DomainEncoder
	Current          TenantAddress
	// Reserved is the operator's own reserved short names, beside the product's list.
	Reserved []string
}

// NewTenantAddress is the address a change moves to. With subdomains it is a short name beneath the
// installation's host, refused as invalid, reserved or - by the store - taken; with assigned hosts it
// is a host, refused as invalid or the installation's own, and the short name stays. The address the
// workspace already has is refused: a change that changes nothing would still end every session and
// passkey. The same short name is a change where the main address names another host - a workspace
// moving beneath a changed installation host.
func NewTenantAddress(in NewTenantAddressInput) (TenantAddress, error) {
	next := TenantAddress{Slug: in.Current.Slug}
	if in.Assigned {
		if strings.TrimSpace(in.RawSlug) != "" {
			return TenantAddress{}, shared.ErrValidation.WithDetail("admin.slug_not_changed_here").
				WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_not_changed_here"})
		}
		host, err := AssignedHost(in.RawHost, in.InstallationHost, in.Domains)
		if err != nil {
			return TenantAddress{}, err
		}
		next.Host = host
	} else {
		if strings.TrimSpace(in.RawHost) != "" {
			return TenantAddress{}, shared.ErrValidation.WithDetail("admin.host_not_assigned").
				WithFields(shared.FieldError{Path: "/host", Code: "admin.host_not_assigned"})
		}
		slug := strings.ToLower(strings.TrimSpace(in.RawSlug))
		if slug == "" {
			return TenantAddress{}, shared.ErrValidation.WithDetail("admin.slug_required").
				WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_required"})
		}
		if !validTenantSlug(slug) {
			return TenantAddress{}, shared.ErrValidation.WithDetail("admin.slug_invalid").
				WithParams(map[string]string{"value": slug}).
				WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_invalid"})
		}
		// A name reserved after the workspace took it stays its own (milestone CD, Decision 8).
		if slug != strings.ToLower(in.Current.Slug) {
			if err := RefuseReservedShortName(slug, in.Reserved); err != nil {
				return TenantAddress{}, err
			}
		}
		next.Slug = slug
		next.Host = CanonicalHostOf(slug, in.InstallationHost, true)
		if next.Host == "" {
			return TenantAddress{}, shared.ErrInternal.WithDetail("admin.installation_host_missing")
		}
	}
	if next.Host == in.Current.Host {
		path := "/slug"
		if in.Assigned {
			path = "/host"
		}
		return TenantAddress{}, shared.ErrValidation.WithDetail("admin.address_unchanged").
			WithFields(shared.FieldError{Path: path, Code: "admin.address_unchanged"})
	}
	return next, nil
}

// WaysHeld is what one person holds to sign in with, as a change of address would leave it.
type WaysHeld struct {
	// Password is a password the workspace lets this person use; SignInName a sign-in name, which
	// signs in with a password of its own.
	Password, SignInName bool
	// Provider is a connected provider that is switched on for the workspace.
	Provider bool
	// App is an armed authenticator app, RecoveryCodes whether any are left.
	App, RecoveryCodes bool
	// Passkeys is whether any are held. They end with the old address and count for nothing here.
	Passkeys bool
	// FactorRequired is whether the workspace's rule demands a second factor of this person.
	FactorRequired bool
}

// What a lasting way in lacks, as the lockout refusal's `reason` names it.
const (
	// LackingWayIn is neither a password (or sign-in name) nor a connected provider.
	LackingWayIn = "way_in"
	// LackingFactor is a required second factor with no app and no recovery codes to answer it.
	LackingFactor = "factor"
)

// Lacking answers what this person would be without after a change of address, empty where a way in
// survives it: a password (or a sign-in name) or a connected provider, plus an app factor or recovery
// codes where a second factor is required. Passkeys count for nothing - their relying party is the
// old host (identity.md §23.3).
func (w WaysHeld) Lacking() string {
	if !w.Password && !w.SignInName && !w.Provider {
		return LackingWayIn
	}
	if w.FactorRequired && !w.App && !w.RecoveryCodes {
		return LackingFactor
	}
	return ""
}

// RefuseLockout is the refusal of a change that would lock somebody out: the person making it
// (`admin.address_lockout_self`) or every owner of the workspace (`admin.address_lockout_owner`).
// It names what is missing, never whom: the operator sees no account inside a workspace (P-01).
func RefuseLockout(self bool, lacking string) error {
	code := "admin.address_lockout_owner"
	if self {
		code = "admin.address_lockout_self"
	}
	return shared.ErrConflict.WithDetail(code).WithParams(map[string]string{"reason": lacking})
}

// AddressChangeNotice is what a member without an email address is told at the next sign-in: the
// workspace's address before and since, and when it changed.
type AddressChangeNotice struct {
	From, To  string
	ChangedAt time.Time
}
