// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package dns is the adapter for core/port/dns: the standard library's resolver, every lookup
// under its own deadline (rule 7). Rule 6 governs HTTP; a DNS question carries nothing of a
// workspace's out and reaches only the name servers the installation's host is configured with.
package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/dns"
)

// DefaultTimeout bounds one lookup where none is configured.
const DefaultTimeout = 5 * time.Second

// Resolver asks the system's name servers.
type Resolver struct {
	// Timeout bounds each lookup; zero is DefaultTimeout.
	Timeout time.Duration
	// Net is the resolver asked; nil is the standard library's default.
	Net *net.Resolver
}

var _ port.Resolver = Resolver{}

func (r Resolver) bounded(ctx context.Context) (context.Context, context.CancelFunc, *net.Resolver) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	resolver := r.Net
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	return bounded, cancel, resolver
}

// TXT is the port's method.
func (r Resolver) TXT(ctx context.Context, name string) ([]string, error) {
	bounded, cancel, resolver := r.bounded(ctx)
	defer cancel()
	texts, err := resolver.LookupTXT(bounded, name)
	if err := classify(err); err != nil {
		return nil, err
	}
	return texts, nil
}

// CanonicalName is the port's method.
func (r Resolver) CanonicalName(ctx context.Context, name string) (string, error) {
	bounded, cancel, resolver := r.bounded(ctx)
	defer cancel()
	canonical, err := resolver.LookupCNAME(bounded, name)
	if err := classify(err); err != nil {
		return "", err
	}
	if err != nil {
		return "", nil
	}
	return strings.TrimSuffix(strings.ToLower(canonical), "."), nil
}

// classify tells an answer from no answer: a name that does not exist, or holds no record of the
// kind, is an answer (nil); anything else - a timeout, a failing server - is unavailable. The name
// asked is not in the error: it is the owner's domain.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return nil
	}
	reason := "lookup failed"
	if errors.As(err, &dnsErr) && dnsErr.IsTimeout || errors.Is(err, context.DeadlineExceeded) {
		reason = "lookup timed out"
	}
	return shared.ErrUnavailable.WithDetail("dependency.dns_unavailable").WithCause(fmt.Errorf("dns: %s", reason))
}
