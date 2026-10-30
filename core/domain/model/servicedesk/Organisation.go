// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

const (
	// MaxOrganisationDomains bounds the mail domains one organisation is assigned by.
	MaxOrganisationDomains = 20
	// MaxOrganisations bounds a workspace's organisations: a list an administrator reads whole.
	MaxOrganisations = 500
	// MaxOrganisationMembers bounds the colleagues whose requests one portal list reads; a larger
	// organisation reads the first of them by account, never a scan of every account.
	MaxOrganisationMembers = 2000
	maxDomainLength        = 253
	maxLabelLength         = 63
)

// Organisation is a group of requesters (UC-SVC-11): every person of the workspace whose address
// is at one of its mail domains belongs to it, so nobody is assigned by hand and nobody stays in it
// after their address moves. Its members see each other's requests in the portal only where
// SharesRequests is on; off is the default, since a domain can be wider than a person thinks.
type Organisation struct {
	ID             shared.ID
	TenantID       shared.ID
	Key            string
	Name           string
	Domains        []string
	SharesRequests bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// NewOrganisationInput is what defining an organisation takes.
type NewOrganisationInput struct {
	ID, TenantID   shared.ID
	Key, Name      string
	Domains        []string
	SharesRequests bool
	Now            time.Time
	Text           text.Normalizer
}

// NewOrganisation checks an organisation: a key, a name on one line, and one to twenty mail
// domains, each a host name, small letters, without a duplicate.
func NewOrganisation(in NewOrganisationInput) (Organisation, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return Organisation{}, shared.ErrInternal.WithDetail("organisations.identity_incomplete")
	}
	key := strings.TrimSpace(in.Key)
	if !keyPattern.MatchString(key) {
		return Organisation{}, invalid("/key", "organisations.key_malformed")
	}
	name, err := checkName(in.Name, in.Text, "organisations")
	if err != nil {
		return Organisation{}, err
	}
	domains, err := OrganisationDomains(in.Domains)
	if err != nil {
		return Organisation{}, err
	}
	return Organisation{
		ID: in.ID, TenantID: in.TenantID, Key: key, Name: name, Domains: domains,
		SharesRequests: in.SharesRequests, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// Change answers the organisation with a new name, domains and sharing, its key kept.
func (o Organisation) Change(name string, domains []string, shares bool, now time.Time, normalizer text.Normalizer) (Organisation, error) {
	changed, err := NewOrganisation(NewOrganisationInput{
		ID: o.ID, TenantID: o.TenantID, Key: o.Key, Name: name, Domains: domains,
		SharesRequests: shares, Now: now, Text: normalizer,
	})
	if err != nil {
		return Organisation{}, err
	}
	changed.CreatedAt, changed.Version = o.CreatedAt, o.Version
	return changed, nil
}

// OrganisationDomains checks and normalises a list of mail domains: trimmed, small letters, a
// leading `@` dropped, sorted; an empty, malformed or repeated one is refused.
func OrganisationDomains(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, invalid("/domains", "organisations.domains_required")
	}
	if len(raw) > MaxOrganisationDomains {
		return nil, invalid("/domains", "organisations.domains_too_many")
	}
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(d)), "@")
		if !validDomain(domain) {
			return nil, shared.ErrValidation.WithDetail("organisations.domain_malformed").
				WithParams(map[string]string{"domain": d}).
				WithFields(shared.FieldError{Path: "/domains", Code: "organisations.domain_malformed"})
		}
		if slices.Contains(out, domain) {
			return nil, shared.ErrValidation.WithDetail("organisations.domain_repeated").
				WithParams(map[string]string{"domain": domain}).
				WithFields(shared.FieldError{Path: "/domains", Code: "organisations.domain_repeated"})
		}
		out = append(out, domain)
	}
	slices.Sort(out)
	return out, nil
}

// DomainOfAddress is the mail domain of an address in small letters, empty for none.
func DomainOfAddress(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(address[at+1:]))
}

// Holds reports whether an address is at one of the organisation's domains - exactly, never a
// subdomain: `mail.example.org` is a domain of its own.
func (o Organisation) Holds(address string) bool {
	domain := DomainOfAddress(address)
	return domain != "" && slices.Contains(o.Domains, domain)
}

// validDomain is a host name of at least two labels, each of letters, digits and inner hyphens.
func validDomain(domain string) bool {
	if domain == "" || len(domain) > maxDomainLength {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > maxLabelLength || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}
