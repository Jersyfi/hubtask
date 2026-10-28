// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The hosts a workspace answers at.
//
// In multi mode a request's host names its workspace only through a serving row, and the canonical
// row is the workspace's main address (ADR-0095 §1): a mail, a redirect and the provider's return
// name that one host rather than whichever the request arrived at.

// HostState is how far a host has got.
//
// Four, and the fourth is the one that makes the model worth landing early: a host that *was*
// serving and stopped is not a host that never worked, and the fallback - the provider host becomes
// canonical again by itself - is keyed on exactly that difference.
type HostState string

const (
	// HostPending is a host somebody claimed. It resolves nothing: a host typed is not a host owned,
	// and acting on the claim is how one workspace answers at another's domain.
	HostPending HostState = "PENDING"
	// HostVerified is a host whose zone carried the mark. It may become canonical; it is not
	// serving yet - the certificate is the operator's next step.
	HostVerified HostState = "VERIFIED"
	// HostActive is verified, its certificate in place, and answering. The canonical host is
	// always this one, and the derived host is this from the moment a workspace exists.
	HostActive HostState = "ACTIVE"
	// HostBroken was ACTIVE and stopped - the record went, the certificate lapsed, the zone moved.
	// Kept rather than deleted, because the row is the way back: when it recovers it is ACTIVE
	// again without anybody doing anything, and until then the provider host is canonical.
	HostBroken HostState = "BROKEN"
)

// MaxHostLength is a hostname's own bound (RFC 1035's 255 octets, less the length byte and the root
// label). A row longer than this is not a host anybody can look up.
const MaxHostLength = 253

// HostVerificationPrefix labels the mark a zone has to carry, so a value found in a DNS record says
// what it is without anybody having to guess (the `hbt_` prefixes of api-guidelines.md §7).
//
// It is **not** a secret and is deliberately not hashed: it is published in a TXT record an operator
// reads out and anybody can look up. What it proves is control of a zone, and only somebody who
// controls the zone can put it there.
const HostVerificationPrefix = "hbt-dv-"

// TenantHost is one host.
type TenantHost struct {
	TenantID shared.ID
	Host     string
	State    HostState
	// Verification is what the zone has to carry. Present on every row, including the canonical
	// one: a column that is sometimes absent is a column every reader has to branch on.
	Verification string
	VerifiedAt   time.Time
	Canonical    bool
	CreatedAt    time.Time
	// Kind is an address the installation gave, or the owner's own domain; empty reads as an address.
	Kind HostKind
}

// CanonicalHostOf is the host a workspace answers at today.
//
// In multi mode the slug is a label under the installation's own host; in single mode there is one
// workspace and the installation's host is its. Derived rather than stored for the rows that already
// exist, which is why a migration could not backfill this: the domain is configuration read at
// start, and a migration knows none of it.
func CanonicalHostOf(slug, installationHost string, multi bool) string {
	host := strings.ToLower(strings.TrimSpace(installationHost))
	host = strings.TrimSuffix(host, ".")
	if !multi {
		return host
	}
	label := strings.ToLower(strings.TrimSpace(slug))
	if label == "" || host == "" {
		return ""
	}
	return label + "." + host
}

// MainHostOf is the workspace's main address among its rows: the canonical one while it serves,
// empty where there is none (multi-tenancy.md §3.1).
func MainHostOf(hosts []TenantHost) string {
	for _, host := range hosts {
		if host.Canonical && host.State == HostActive {
			return host.Host
		}
	}
	return ""
}

// RequestHost is the host a request arrived at, as a `tenant_host` row would hold it: lowercase,
// without a port or the root's dot. Empty for what names no row - nothing, or an IPv6 literal.
func RequestHost(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(host, "[") {
		return ""
	}
	if at := strings.LastIndexByte(host, ':'); at >= 0 {
		host = host[:at]
	}
	return strings.TrimSuffix(host, ".")
}

// NewCanonicalHostInput is what writing the derived row needs.
type NewCanonicalHostInput struct {
	TenantID shared.ID
	Host     string
	// Verification is drawn through the entropy port like every other unguessable value (rule 4).
	// The canonical row never presents it - there is nothing to prove - and it is written all the
	// same, so that promoting a custom host to canonical later is one update rather than a column
	// that has to be filled in first.
	Verification string
	Now          time.Time
}

// NewCanonicalHost builds the row a workspace gets when it is provisioned.
//
// ACTIVE on arrival, and that is not a shortcut: the host is the installation's own domain with
// the workspace's slug in front of it, so the installation already answers at it, certificate and
// all, and there is nobody to prove anything to.
func NewCanonicalHost(in NewCanonicalHostInput) (TenantHost, error) {
	host, err := normalisedHost(in.Host)
	if err != nil {
		return TenantHost{}, err
	}
	if in.TenantID.IsZero() || in.Now.IsZero() || in.Verification == "" {
		return TenantHost{}, shared.ErrInternal.WithDetail("tenant_host.incomplete")
	}
	return TenantHost{
		TenantID: in.TenantID, Host: host, State: HostActive,
		Verification: in.Verification, VerifiedAt: in.Now.UTC(),
		Canonical: true, CreatedAt: in.Now.UTC(),
	}, nil
}

// normalisedHost holds a host to what one is: lowercase, dotted, without a scheme, a port, a path or
// a wildcard.
//
// Refused rather than interpreted, for `normalisedDomains`' reason: "anything ending in
// example.com" is how `evil-example.com` becomes a way in, and a host somebody has to write out is a
// host somebody has to think about.
func normalisedHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > MaxHostLength || !strings.Contains(host, ".") ||
		strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return "", hostRefused("admin.host_invalid", strings.TrimSpace(raw))
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 ||
			strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") ||
			strings.IndexFunc(label, notHostRune) >= 0 {
			return "", hostRefused("admin.host_invalid", strings.TrimSpace(raw))
		}
	}
	return host, nil
}

// notHostRune is what no label carries: anything but a letter, a digit and the hyphen. Letters
// beyond ASCII stay - they are a host's before it is encoded.
func notHostRune(r rune) bool {
	return r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// hostRefused is a host's refusal at the field it was typed into.
func hostRefused(code, host string) error {
	params := map[string]string{"host": host}
	return shared.ErrValidation.
		WithDetail(code).
		WithParams(params).
		WithFields(shared.FieldError{Path: "/host", Code: code, Params: params})
}

// AssignedHost is the host an operator names for a workspace in an installation that assigns hosts
// (ADR-0095 §6): a host name in the form a browser sends it - lower case, letters beyond ASCII
// encoded - and never the installation's own. No DNS is asked: the record and the certificate are
// the operator's, as for any host of theirs. Whether another workspace answers at it is the store's
// to say, installation-wide.
func AssignedHost(raw, installationHost string, domains text.DomainEncoder) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", hostRefused("admin.host_required", "")
	}
	host, err := normalisedHost(raw)
	if err != nil {
		return "", err
	}
	if domains != nil {
		encoded, err := domains.ToASCII(host)
		if err != nil {
			return "", hostRefused("admin.host_invalid", strings.TrimSpace(raw))
		}
		host = strings.ToLower(encoded)
	}
	own := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(installationHost)), ".")
	if host == own {
		return "", hostRefused("admin.host_installation", host)
	}
	return host, nil
}

// ShortNameOfHost is the short name a workspace on an assigned host gets where the operator gave
// none: its host's first label, and the next ones while that is shorter than a short name may be -
// `tasks-hr.corp.example` is `tasks-hr`, `hr.corp.example` is `hr-corp`.
func ShortNameOfHost(host string) string {
	name := ""
	for _, label := range strings.Split(host, ".") {
		if name != "" {
			name += "-"
		}
		name += label
		// DeriveTenantSlug stands in "workspace" for what makes no short name, so a name is only
		// taken as it comes when it already is one.
		if derived := DeriveTenantSlug(name); derived != "workspace" || name == "workspace" {
			return derived
		}
	}
	return DeriveTenantSlug(name)
}

// AddressAtCreation reads what creating a workspace in multi mode names of its address, by the
// installation's way of giving one (ADR-0095 §6): with assigned hosts the host, checked, and the
// short name - made from the host where none is given; with subdomains the short name alone, a host
// refused. It answers the host to write, empty for the subdomain the short name makes, and the
// short name. Provisioning and the first start's setup ask it alike, so every door refuses the same.
func AddressAtCreation(
	assigned bool, rawHost, rawSlug, installationHost string, domains text.DomainEncoder,
) (string, string, error) {
	slug := strings.TrimSpace(rawSlug)
	if !assigned {
		if strings.TrimSpace(rawHost) != "" {
			return "", "", shared.ErrValidation.WithDetail("admin.host_not_assigned").
				WithFields(shared.FieldError{Path: "/host", Code: "admin.host_not_assigned"})
		}
		if slug == "" {
			return "", "", shared.ErrValidation.WithDetail("admin.slug_required").
				WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_required"})
		}
		return "", slug, nil
	}
	host, err := AssignedHost(rawHost, installationHost, domains)
	if err != nil {
		return "", "", err
	}
	if slug == "" {
		slug = ShortNameOfHost(host)
	}
	return host, slug, nil
}
