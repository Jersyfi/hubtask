// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TenantStatus is the workspace's standing in the lifecycle multi-tenancy.md §5 draws. It rides
// with every credential read (H-06): a suspension has to flip authentication itself, not each
// use case separately.
type TenantStatus string

const (
	TenantActive          TenantStatus = "ACTIVE"
	TenantSuspended       TenantStatus = "SUSPENDED"
	TenantPendingDeletion TenantStatus = "PENDING_DELETION"
)

// Verify decides whether requests of this workspace may proceed. Forbidden rather than
// unauthenticated: the credential is real and verified, the workspace is what stands still -
// and the holder is entitled to be told which (multi-tenancy.md §5, "the API responds
// 403 tenant_suspended").
func (s TenantStatus) Verify() error {
	switch s {
	case TenantActive:
		return nil
	case TenantSuspended:
		return shared.ErrForbidden.WithDetail("access.tenant_suspended")
	case TenantPendingDeletion:
		return shared.ErrForbidden.WithDetail("access.tenant_pending_deletion")
	default:
		// Fail closed: a standing this build does not know is not one it may wave through.
		return shared.ErrForbidden.WithDetail("access.tenant_suspended")
	}
}

// Tenant slug bounds, the database constraint spelled as rules: lower-case letters, digits and
// hyphens, starting with a letter or digit, 3 to 40 characters. It becomes a subdomain label, so
// the grammar is DNS's, not the product's.
const (
	MinTenantSlugLength = 3
	MaxTenantSlugLength = 40
)

// Tenant is a workspace as the control plane sees it (H-06, multi-tenancy.md §5). Deliberately
// not the settings document: what a tenant configures is the tenant's; this is what the
// installation provisions.
type Tenant struct {
	ID              shared.ID
	Slug            string
	DisplayName     string
	Status          TenantStatus
	DefaultLocale   string
	DefaultTimeZone string
	CreatedAt       time.Time
}

// NewTenantInput is what provisioning states. Locale and time zone may be empty and default to
// the installation's own pair - a workspace always has both, because they are the last two links
// of every person's resolution chain (i18n-l10n.md §2).
type NewTenantInput struct {
	ID              shared.ID
	Slug            string
	DisplayName     string
	DefaultLocale   string
	DefaultTimeZone string
	Now             time.Time
}

// NewTenant validates what the database would otherwise refuse later, plus what it cannot: the
// locale grammar and a loadable time zone.
func NewTenant(in NewTenantInput) (Tenant, error) {
	if in.ID.IsZero() {
		return Tenant{}, shared.ErrInternal.WithDetail("admin.tenant_incomplete")
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if !validTenantSlug(slug) {
		return Tenant{}, shared.ErrValidation.
			WithDetail("admin.slug_invalid").
			WithParams(map[string]string{"value": slug}).
			WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_invalid"})
	}

	name := strings.TrimSpace(in.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > 200 || strings.ContainsFunc(name, unicode.IsControl) {
		return Tenant{}, shared.ErrValidation.
			WithDetail("admin.display_name_invalid").
			WithFields(shared.FieldError{Path: "/display_name", Code: "admin.display_name_invalid"})
	}

	locale := strings.TrimSpace(in.DefaultLocale)
	if locale == "" {
		locale = "en"
	}
	tag, ok := shared.LanguageTag(locale)
	if !ok {
		return Tenant{}, shared.ErrValidation.
			WithDetail("admin.locale_invalid").
			WithParams(map[string]string{"value": locale}).
			WithFields(shared.FieldError{Path: "/default_locale", Code: "admin.locale_invalid"})
	}

	zone := strings.TrimSpace(in.DefaultTimeZone)
	if zone == "" {
		zone = "UTC"
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return Tenant{}, shared.ErrValidation.
			WithDetail("admin.time_zone_invalid").
			WithParams(map[string]string{"value": zone}).
			WithFields(shared.FieldError{Path: "/default_time_zone", Code: "admin.time_zone_invalid"})
	}

	return Tenant{
		ID: in.ID, Slug: slug, DisplayName: name, Status: TenantActive,
		DefaultLocale: tag, DefaultTimeZone: zone, CreatedAt: in.Now.UTC(),
	}, nil
}

// validTenantSlug is the database's check constraint, decided here first so the refusal is a
// field error rather than a constraint surfacing.
func validTenantSlug(slug string) bool {
	if len(slug) < MinTenantSlugLength || len(slug) > MaxTenantSlugLength {
		return false
	}
	for index, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && index > 0:
		default:
			return false
		}
	}
	return true
}
