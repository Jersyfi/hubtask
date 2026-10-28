// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// productReservedShortNames are the labels an installation or its ingress uses for itself
// (multi-tenancy.md §3.1). In multi mode a short name is a subdomain, so a workspace called `api`
// or `mail` would answer where the installation has to. Each is written singular: an entry
// reserves itself and its plural.
var productReservedShortNames = []string{
	// The installation's own faces.
	"www", "app", "api", "admin", "instance", "installation", "operator", "setup", "console",
	"dashboard", "portal", "status", "health", "metric", "doc", "help", "support",
	"blog", "news", "about", "legal", "privacy", "term", "imprint", "security", "abuse",
	// Signing in.
	"auth", "login", "logout", "signin", "signup", "sso", "oauth", "oidc", "saml", "account",
	"identity",
	// Files and the edge.
	"media", "static", "asset", "file", "upload", "download", "image", "img", "font", "cdn",
	"edge", "origin", "proxy", "gateway",
	// Mail and DNS.
	"mail", "webmail", "smtp", "imap", "pop3", "autoconfig", "autodiscover", "postmaster",
	"hostmaster", "webmaster", "noreply", "no-reply", "dns",
	// The product's own channels.
	"mcp", "calendar", "intake", "stream", "sync", "webhook", "event", "hubtask",
	// Names nobody should be able to claim.
	"root", "system", "internal", "localhost",
}

// ProductReservedShortNames answers the product's own list, singular, in its order.
func ProductReservedShortNames() []string {
	return slices.Clone(productReservedShortNames)
}

// IsReservedShortName says whether a short name is the product's or one of the operator's
// entries, or the plural of one (`api` and `apis`, `status` and `statuses`). Case does not count.
func IsReservedShortName(slug string, operator []string) bool {
	name := strings.ToLower(strings.TrimSpace(slug))
	matches := func(entry string) bool {
		entry = strings.ToLower(strings.TrimSpace(entry))
		return entry != "" && (name == entry || name == entry+"s" || name == entry+"es")
	}
	return slices.ContainsFunc(productReservedShortNames, matches) || slices.ContainsFunc(operator, matches)
}

// RefuseReservedShortName is the refusal at the field, separate from `admin.slug_invalid` and
// `admin.slug_taken` so the person learns which of the three it was. Provisioning and the change
// of address ask it; a workspace that already carries a name reserved later keeps it, because
// nothing but those two asks.
func RefuseReservedShortName(slug string, operator []string) error {
	if !IsReservedShortName(slug, operator) {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(slug))
	return shared.ErrValidation.
		WithDetail("admin.slug_reserved").
		WithParams(map[string]string{"value": name}).
		WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_reserved"})
}

// ValidReservedShortNames brings the operator's own list to its stored form: trimmed, lower case,
// each once, sorted. An entry no short name could ever be is refused rather than kept: it reserves
// nothing, and an operator who typed it meant something else.
func ValidReservedShortNames(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := strings.ToLower(strings.TrimSpace(entry))
		if !validTenantSlug(name) {
			return nil, shared.ErrValidation.
				WithDetail("admin.reserved_short_name_invalid").
				WithParams(map[string]string{"value": name}).
				WithFields(shared.FieldError{
					Path: "/reserved_short_names", Code: "admin.reserved_short_name_invalid",
					Params: map[string]string{"value": name},
				})
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
