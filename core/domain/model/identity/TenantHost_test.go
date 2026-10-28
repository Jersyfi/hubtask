// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
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

// A request's host is read as a row holds it, so that `Anna.Hub.Example:8443` and the row
// `anna.hub.example` are one host (ADR-0095 §1).
func TestARequestsHostIsReadAsARowHoldsIt(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"anna.hub.example", "anna.hub.example"},
		{"Anna.Hub.Example:8443", "anna.hub.example"},
		{"tasks.meier.example.", "tasks.meier.example"},
		{" localhost:8080 ", "localhost"},
		{"[::1]:8080", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := RequestHost(c.raw); got != c.want {
			t.Errorf("RequestHost(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// The main address is the canonical row while it serves, and no other row however it is ordered.
func TestTheMainHostIsTheCanonicalRowThatServes(t *testing.T) {
	cases := []struct {
		name  string
		hosts []TenantHost
		want  string
	}{
		{"none", nil, ""},
		{"the canonical one, wherever it stands", []TenantHost{
			{Host: "meier.hub.example", State: HostActive},
			{Host: "tasks.meier.example", State: HostActive, Canonical: true},
		}, "tasks.meier.example"},
		{"a canonical row that does not serve is no address", []TenantHost{
			{Host: "tasks.meier.example", State: HostBroken, Canonical: true},
			{Host: "meier.hub.example", State: HostActive},
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MainHostOf(c.hosts); got != c.want {
				t.Errorf("MainHostOf = %q, want %q", got, c.want)
			}
		})
	}
}

// fakeDomains encodes the one name these tests write with letters beyond ASCII, and refuses a
// label the encoding profile would.
type fakeDomains struct{}

func (fakeDomains) ToASCII(domain string) (string, error) {
	if domain == "aufgaben.müller.example" {
		return "aufgaben.xn--mller-kva.example", nil
	}
	if strings.Contains(domain, "ß-") {
		return "", errors.New("refused by the profile")
	}
	return domain, nil
}

// UC-INS-20 check 3: an assigned host is a host name, never the installation's own, and each
// refusal is at the field with a code naming its reason; nothing about DNS is asked.
func TestAnAssignedHostIsAHostNameOfItsOwn(t *testing.T) {
	cases := []struct {
		name, raw, want, code string
	}{
		{name: "a host as typed", raw: "tasks-hr.corp.example", want: "tasks-hr.corp.example"},
		{name: "case and the root's dot are not part of it", raw: " Tasks-HR.Corp.Example. ", want: "tasks-hr.corp.example"},
		{name: "letters beyond ASCII in the form a browser sends", raw: "aufgaben.müller.example", want: "aufgaben.xn--mller-kva.example"},
		{name: "nothing", raw: "  ", code: "admin.host_required"},
		{name: "a URL", raw: "https://tasks hr/", code: "admin.host_invalid"},
		{name: "a port", raw: "tasks-hr.corp.example:8443", code: "admin.host_invalid"},
		{name: "a single label", raw: "tasks", code: "admin.host_invalid"},
		{name: "an empty label", raw: "tasks..corp.example", code: "admin.host_invalid"},
		{name: "a hyphen at a label's edge", raw: "-tasks.corp.example", code: "admin.host_invalid"},
		{name: "a wildcard", raw: "*.corp.example", code: "admin.host_invalid"},
		{name: "punctuation", raw: "tasks!hr.corp.example", code: "admin.host_invalid"},
		{name: "a label the encoding refuses", raw: "straß-.corp.example", code: "admin.host_invalid"},
		{name: "the installation's own host", raw: "Tasks.Corp.Example", code: "admin.host_installation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := AssignedHost(c.raw, "tasks.corp.example.", fakeDomains{})
			if c.code == "" {
				if err != nil || got != c.want {
					t.Errorf("AssignedHost(%q) = (%q, %v), want %q", c.raw, got, err, c.want)
				}
				return
			}
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != c.code || len(refusal.Fields) != 1 ||
				refusal.Fields[0].Path != "/host" || refusal.Fields[0].Code != c.code {
				t.Errorf("AssignedHost(%q) answered (%q, %v), want %s at /host", c.raw, got, err, c.code)
			}
		})
	}
}

// The short name of a workspace on an assigned host, where the operator gave none: the host's first
// label, and the next ones while that is too short to be a short name.
func TestTheShortNameIsMadeFromTheHost(t *testing.T) {
	for host, want := range map[string]string{
		"tasks-hr.corp.example":          "tasks-hr",
		"hr.corp.example":                "hr-corp",
		"a.b.example":                    "a-b",
		"aufgaben.xn--mller-kva.example": "aufgaben",
		"xn--mller-kva.example":          "xn-mller-kva",
	} {
		if got := ShortNameOfHost(host); got != want {
			t.Errorf("ShortNameOfHost(%q) = %q, want %q", host, got, want)
		}
	}
}
