// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The hosts a workspace answers at (SI-12).
//
// **The model a custom domain needs, and none of the feature.** Today a workspace is reached at the
// host its slug makes under the installation's own domain, and the resolution still reads the slug.
// What this type is for is that the *shape* exists before the feature does: a host of its own, a
// state, something to verify it with, and the one that is canonical. A mail, a redirect and an
// invitation link have to name one host rather than whichever the request arrived at, and that is
// what `Canonical` answers.

// HostState is how far a host has got (the concept's §6.5).
//
// Four, and the fourth is the one that makes the model worth landing early: a host that *was*
// serving and stopped is not a host that never worked, and §6.6's fallback - the provider host
// becomes canonical again by itself - is keyed on exactly that difference.
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
// what it is without anybody having to guess (D-08's prefix catalogue).
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
	refused := func() (string, error) {
		return "", shared.ErrValidation.
			WithDetail("tenant_host.host_invalid").
			WithParams(map[string]string{"host": strings.TrimSpace(raw)})
	}
	if host == "" || len(host) > MaxHostLength || !strings.Contains(host, ".") ||
		strings.ContainsAny(host, "/@ *:?#_") || strings.HasPrefix(host, ".") ||
		strings.Contains(host, "..") {
		return refused()
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 ||
			strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return refused()
		}
	}
	return host, nil
}
