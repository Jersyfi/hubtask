// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The host a workspace answers at today, derived rather than stored: the slug under the
// installation's own domain in multi mode, the domain itself where there is one workspace.
func TestTheCanonicalHostIsDerived(t *testing.T) {
	cases := []struct {
		name         string
		slug         string
		installation string
		multi        bool
		want         string
	}{
		{name: "the slug is a label in front", slug: "acme", installation: "hubtask.example", multi: true,
			want: "acme.hubtask.example"},
		{name: "case is not part of a host", slug: "ACME", installation: "Hubtask.Example", multi: true,
			want: "acme.hubtask.example"},
		{name: "a trailing root label is not part of it", slug: "acme", installation: "hubtask.example.", multi: true,
			want: "acme.hubtask.example"},
		{name: "one workspace answers at the installation's own host", slug: "acme",
			installation: "tasks.example.org", want: "tasks.example.org"},
		{name: "no slug in multi mode is no host", installation: "hubtask.example", multi: true},
		{name: "and no domain is no host either", slug: "acme", multi: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanonicalHostOf(c.slug, c.installation, c.multi); got != c.want {
				t.Errorf("CanonicalHostOf(%q, %q, %v) = %q, want %q",
					c.slug, c.installation, c.multi, got, c.want)
			}
		})
	}
}

// The canonical row arrives ACTIVE, and that is not a shortcut: the installation already answers
// at its own domain, certificate and all, so there is nobody to prove anything to and nothing to
// wait for.
func TestTheCanonicalRowIsActiveOnArrival(t *testing.T) {
	at := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	host, err := NewCanonicalHost(NewCanonicalHostInput{
		TenantID: sessionTenant, Host: "acme.hubtask.example",
		Verification: HostVerificationPrefix + "abcdefgh", Now: at,
	})
	if err != nil {
		t.Fatalf("building the canonical host: %v", err)
	}
	if host.State != HostActive || host.VerifiedAt.IsZero() {
		t.Errorf("the canonical host is %q, verified at %v", host.State, host.VerifiedAt)
	}
	if !host.Canonical {
		t.Error("the canonical host does not say it is canonical")
	}
	// The mark is written all the same, so promoting a custom host later is one update rather than
	// a column that has to be filled in first.
	if host.Verification == "" {
		t.Error("the canonical host carries no mark")
	}
}

// What may be a host, and the refusals are the point: a wildcard, a scheme, a port or a path is not
// one, and interpreting any of them is how one workspace answers at another's domain.
func TestWhatMayBeAHost(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		want    string
		refused bool
	}{
		{name: "a plain host", host: "tasks.example.org", want: "tasks.example.org"},
		{name: "case and space are typing", host: "  Tasks.Example.ORG ", want: "tasks.example.org"},
		{name: "the root label is not part of it", host: "tasks.example.org.", want: "tasks.example.org"},
		{name: "a wildcard is refused rather than interpreted", host: "*.example.org", refused: true},
		{name: "a scheme is not a host", host: "https://tasks.example.org", refused: true},
		{name: "nor is a host with a port", host: "tasks.example.org:8443", refused: true},
		{name: "nor one with a path", host: "tasks.example.org/app", refused: true},
		{name: "an address is not a host", host: "ada@example.org", refused: true},
		{name: "a bare label is not a host", host: "localhost", refused: true},
		{name: "an empty label", host: "tasks..example.org", refused: true},
		{name: "a label that starts with a hyphen", host: "-tasks.example.org", refused: true},
		{name: "an underscore is not a hostname character", host: "my_tasks.example.org", refused: true},
		{name: "empty", host: "   ", refused: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host, err := NewCanonicalHost(NewCanonicalHostInput{
				TenantID: sessionTenant, Host: c.host, Verification: "hbt-dv-abcdefgh",
				Now: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC),
			})
			if c.refused {
				if err == nil {
					t.Fatalf("%q was accepted as a host", c.host)
				}
				if shared.AsError(err).Category != shared.CategoryValidation {
					t.Errorf("the refusal is %v, want a validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q was refused: %v", c.host, err)
			}
			if host.Host != c.want {
				t.Errorf("%q normalised to %q, want %q", c.host, host.Host, c.want)
			}
		})
	}
}

// A row with no workspace, no moment or no mark is a row nothing wrote on purpose.
func TestACanonicalHostNeedsItsWorkspaceItsMomentAndItsMark(t *testing.T) {
	at := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	complete := NewCanonicalHostInput{
		TenantID: sessionTenant, Host: "acme.hubtask.example",
		Verification: "hbt-dv-abcdefgh", Now: at,
	}
	for name, spoil := range map[string]func(*NewCanonicalHostInput){
		"with no workspace": func(in *NewCanonicalHostInput) { in.TenantID = shared.ID("") },
		"with no moment":    func(in *NewCanonicalHostInput) { in.Now = time.Time{} },
		"with no mark":      func(in *NewCanonicalHostInput) { in.Verification = "" },
	} {
		t.Run(name, func(t *testing.T) {
			in := complete
			spoil(&in)
			if _, err := NewCanonicalHost(in); err == nil {
				t.Errorf("a host %s was accepted", name)
			}
		})
	}
}
