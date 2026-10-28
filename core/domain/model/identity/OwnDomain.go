// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A workspace's own domain, proven by DNS (ADR-0095 §4, UC-INS-15): a host its owner sets up
// without the operator, which answers once a CNAME points it at the installation's target and a
// TXT record at `_hubtask.<domain>` carries the row's mark, and stops answering when the mark goes.

// HostKind tells a row the workspace's owner set up from one the installation gave it.
type HostKind string

const (
	// HostKindAddress is a subdomain or an assigned host: the installation's, nothing to prove.
	HostKindAddress HostKind = "ADDRESS"
	// HostKindOwnDomain is the owner's own domain, proven by DNS and re-proven.
	HostKindOwnDomain HostKind = "OWN_DOMAIN"
)

const (
	// OwnDomainTargetLabel is the label in front of the installation's host that every own
	// domain's CNAME points at, so the operator's DNS names one target for all of them.
	OwnDomainTargetLabel = "domains"
	// OwnDomainProofLabel is the label in front of the domain where the TXT record lives, so the
	// proof sits beside the domain's own records rather than among them.
	OwnDomainProofLabel = "_hubtask"
	// OwnDomainProofPrefix labels the text the TXT record carries, so it says what it is.
	OwnDomainProofPrefix = "hubtask-domain="
	// OwnDomainBlockReasonMax bounds the operator's reason, which the owner reads word for word.
	OwnDomainBlockReasonMax = 500
)

// OwnDomain is the workspace's own domain as its owner and the check see it.
type OwnDomain struct {
	Host  string
	State HostState
	// Verification is the full text the TXT record has to carry.
	Verification string
	// Since is when it entered its state.
	Since time.Time
	// CheckedAt is the last look at its records; zero before the first.
	CheckedAt time.Time
	// TargetFound and ProofFound are what that look found.
	TargetFound, ProofFound bool
	// CertificateAsked is whether the ingress asked for its certificate.
	CertificateAsked bool
	// Main is whether it is the workspace's main address.
	Main bool
	// FellBackAt is when it was main and broke, so the address row became main again by itself;
	// zero otherwise. The warning it raises stays until the owner makes it main again or removes it.
	FellBackAt time.Time
}

// MainAddressMove is a move of the main address between the own domain and the workspace's own
// address row - its subdomain or assigned host (UC-INS-15 checks 6 and 8): it answers the host the
// main address leaves and the one it goes to. Only an active own domain becomes main, and only where
// there is an address row to come back to; only a main one is switched back from.
func MainAddressMove(own OwnDomain, addressHost string, toOwn bool) (string, string, error) {
	if addressHost == "" {
		return "", "", shared.ErrConflict.WithDetail("admin.own_domain_no_way_back")
	}
	if !toOwn {
		if !own.Main {
			return "", "", shared.ErrConflict.WithDetail("admin.own_domain_not_main")
		}
		return own.Host, addressHost, nil
	}
	if own.Main {
		return "", "", shared.ErrConflict.WithDetail("admin.own_domain_already_main")
	}
	if own.State != HostActive {
		params := map[string]string{"domain": own.Host}
		return "", "", shared.ErrConflict.WithDetail("admin.own_domain_not_active").WithParams(params)
	}
	return addressHost, own.Host, nil
}

// DomainBlock is an own domain the operator blocked, as the workspace it was taken from sees it.
type DomainBlock struct {
	Host      string
	Reason    string
	BlockedAt time.Time
	WasMain   bool
}

// OwnDomainRule is what the installation decided: whether owners set up own domains. Off by
// default and always off in single mode (Decision 4 of milestone CD).
type OwnDomainRule struct {
	Allowed bool
}

// Refuse is the owner's refusal where the installation does not allow own domains.
func (r OwnDomainRule) Refuse() error {
	if !r.Allowed {
		return shared.ErrForbidden.WithDetail("access.own_domains_not_allowed")
	}
	return nil
}

// OwnDomainOf reads the domain an owner typed: a host name in the form the DNS holds it, neither
// the installation's own host nor one beneath it, and not a registered domain itself - an apex
// takes no CNAME, so the owner is told to use a name in front of it, such as `tasks.<domain>`.
// Whether another workspace answers at it, and whether the operator blocked it, is the store's.
func OwnDomainOf(
	raw, installationHost string, domains text.DomainEncoder, registry text.DomainRegistry,
) (string, error) {
	typed := strings.TrimSpace(raw)
	if typed == "" {
		return "", ownDomainRefused("admin.own_domain_required", "")
	}
	host, err := normalisedHost(typed)
	if err != nil {
		return "", ownDomainRefused("admin.own_domain_invalid", typed)
	}
	if domains != nil {
		encoded, err := domains.ToASCII(host)
		if err != nil {
			return "", ownDomainRefused("admin.own_domain_invalid", typed)
		}
		host = strings.ToLower(encoded)
	}
	own := RequestHost(installationHost)
	if own != "" && (host == own || strings.HasSuffix(host, "."+own)) {
		params := map[string]string{"domain": host, "installation_host": own}
		return "", shared.ErrValidation.WithDetail("admin.own_domain_installation").WithParams(params).
			WithFields(shared.FieldError{Path: "/domain", Code: "admin.own_domain_installation", Params: params})
	}
	if registry != nil {
		registered, err := registry.RegisteredDomain(host)
		if err != nil {
			return "", ownDomainRefused("admin.own_domain_invalid", typed)
		}
		if registered == host {
			return "", ownDomainRefused("admin.own_domain_apex", host)
		}
	}
	return host, nil
}

// OwnDomainTarget is where every own domain's CNAME points: `domains.<installation host>`.
func OwnDomainTarget(installationHost string) string {
	own := RequestHost(installationHost)
	if own == "" {
		return ""
	}
	return OwnDomainTargetLabel + "." + own
}

// OwnDomainProofName is where the TXT record lives: `_hubtask.<domain>`.
func OwnDomainProofName(host string) string {
	return OwnDomainProofLabel + "." + host
}

// OwnDomainProof is the text the TXT record carries, from the row's drawn mark.
func OwnDomainProof(mark string) string {
	return OwnDomainProofPrefix + mark
}

// NewOwnDomainInput is what setting one up needs.
type NewOwnDomainInput struct {
	TenantID shared.ID
	Host     string
	// Mark is drawn through the entropy port (rule 4); published, so not a secret.
	Mark string
	Now  time.Time
}

// NewOwnDomain is the row an owner's set-up writes: PENDING, never the main address.
func NewOwnDomain(in NewOwnDomainInput) (TenantHost, error) {
	host, err := normalisedHost(in.Host)
	if err != nil {
		return TenantHost{}, err
	}
	if in.TenantID.IsZero() || in.Now.IsZero() || len(in.Mark) < 8 {
		return TenantHost{}, shared.ErrInternal.WithDetail("tenant_host.incomplete")
	}
	return TenantHost{
		TenantID: in.TenantID, Host: host, State: HostPending, Kind: HostKindOwnDomain,
		Verification: OwnDomainProof(in.Mark), CreatedAt: in.Now.UTC(),
	}, nil
}

// DomainLookup is what one look at an own domain's records found. Known is false where the DNS did
// not answer - a timeout, a failing server - and then nothing moves: a domain breaks on a missing
// record, never on a resolver's bad minute.
type DomainLookup struct {
	Known       bool
	TargetFound bool
	ProofFound  bool
}

// NextOwnDomainState is where a look moves a domain (ADR-0095 §4). A pending one is verified once
// both records are found; a serving one turns broken once its proof is gone - the pointing record
// is the owner's to keep, and without it nothing reaches the installation anyway; a broken one
// comes back once both are found again, active where its certificate was already asked for.
func NextOwnDomainState(current HostState, look DomainLookup, certificateAsked bool) HostState {
	if !look.Known {
		return current
	}
	switch current {
	case HostPending:
		if look.TargetFound && look.ProofFound {
			return HostVerified
		}
	case HostVerified, HostActive:
		if !look.ProofFound {
			return HostBroken
		}
	case HostBroken:
		if look.TargetFound && look.ProofFound {
			if certificateAsked {
				return HostActive
			}
			return HostVerified
		}
	}
	return current
}

// ProofCarried is whether one of the texts at the proof's name is the row's mark. Exact, after
// the quotes and spaces a zone editor adds: a record that merely contains the mark proves nothing.
func ProofCarried(texts []string, verification string) bool {
	for _, held := range texts {
		if strings.TrimSpace(strings.Trim(strings.TrimSpace(held), `"`)) == verification {
			return true
		}
	}
	return false
}

// TargetReached is whether a domain's CNAME chain arrives at the installation's target: its
// canonical name is the target, or the name the target's own chain ends at.
func TargetReached(domainCanonical, target, targetCanonical string) bool {
	name := canonicalName(domainCanonical)
	if name == "" {
		return false
	}
	return name == canonicalName(target) || (canonicalName(targetCanonical) != "" && name == canonicalName(targetCanonical))
}

func canonicalName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

// BlockReason reads the operator's reason: trimmed, present and bounded.
func BlockReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	if reason == "" {
		return "", shared.ErrValidation.WithDetail("admin.own_domain_reason_required").
			WithFields(shared.FieldError{Path: "/reason", Code: "admin.own_domain_reason_required"})
	}
	if len([]rune(reason)) > OwnDomainBlockReasonMax {
		return "", shared.ErrValidation.WithDetail("admin.own_domain_reason_too_long").
			WithFields(shared.FieldError{Path: "/reason", Code: "admin.own_domain_reason_too_long"})
	}
	return reason, nil
}

// ownDomainRefused is a refusal at the field the domain was typed into.
func ownDomainRefused(code, domain string) error {
	params := map[string]string{"domain": domain}
	return shared.ErrValidation.WithDetail(code).WithParams(params).
		WithFields(shared.FieldError{Path: "/domain", Code: code, Params: params})
}

// OwnDomainTaken is the store's refusal of a domain another workspace answers at, never saying which.
func OwnDomainTaken(host string) error {
	params := map[string]string{"domain": host}
	return shared.ErrConflict.WithDetail("admin.own_domain_taken").WithParams(params).
		WithFields(shared.FieldError{Path: "/domain", Code: "admin.own_domain_taken", Params: params})
}

// OwnDomainBlocked is the refusal of a domain the operator blocked.
func OwnDomainBlocked(host string) error {
	params := map[string]string{"domain": host}
	return shared.ErrConflict.WithDetail("admin.own_domain_blocked").WithParams(params).
		WithFields(shared.FieldError{Path: "/domain", Code: "admin.own_domain_blocked", Params: params})
}
