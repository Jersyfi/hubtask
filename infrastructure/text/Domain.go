// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package text is the adapter for core/port/text: golang.org/x/net/idna and its publicsuffix for
// domains, and golang.org/x/text/unicode/norm for the Unicode normal form. Both libraries are
// confined here (ADR-0056).
package text

import (
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"

	port "github.com/Jersyfi/hubtask/core/port/text"
)

// Domains encodes domain names with the lookup profile: the one for a name that is going to be
// resolved, which maps case, checks the labels and refuses what the DNS would.
type Domains struct{}

var _ port.DomainEncoder = Domains{}

// ToASCII is the port's method.
func (Domains) ToASCII(domain string) (string, error) {
	return idna.Lookup.ToASCII(domain)
}

// Registry answers a host's registered domain by the public suffix list compiled into
// golang.org/x/net/publicsuffix, so no list is fetched at run time.
type Registry struct{}

var _ port.DomainRegistry = Registry{}

// RegisteredDomain is the port's method.
func (Registry) RegisteredDomain(host string) (string, error) {
	return publicsuffix.EffectiveTLDPlusOne(host)
}
