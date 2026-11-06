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

// fakeRegistry knows the registered domains these tests write: the last two labels, or three
// under co.uk; a public suffix alone lies in none.
type fakeRegistry struct{}

func (fakeRegistry) RegisteredDomain(host string) (string, error) {
	labels := strings.Split(host, ".")
	keep := 2
	if strings.HasSuffix(host, ".co.uk") {
		keep = 3
	}
	if len(labels) < keep || host == "co.uk" {
		return "", errors.New("a public suffix")
	}
	return strings.Join(labels[len(labels)-keep:], "."), nil
}

// UC-INS-15 check 4: a domain beneath or at the installation's host is refused at the field with a
// code naming the reason, as is what is no domain and an apex, which takes no CNAME.
func TestAnOwnDomainIsADomainOfTheOwners(t *testing.T) {
	cases := []struct {
		name, raw, want, code string
	}{
		{name: "a subdomain of the owner's", raw: "tasks.acme.com", want: "tasks.acme.com"},
		{name: "case, spaces and the root's dot", raw: " Tasks.ACME.com. ", want: "tasks.acme.com"},
		{name: "under a two-label suffix", raw: "tasks.acme.co.uk", want: "tasks.acme.co.uk"},
		{name: "letters beyond ASCII", raw: "aufgaben.müller.example", want: "aufgaben.xn--mller-kva.example"},
		{name: "nothing", raw: " ", code: "admin.own_domain_required"},
		{name: "a URL", raw: "https://tasks acme", code: "admin.own_domain_invalid"},
		{name: "a port", raw: "tasks.acme.com:443", code: "admin.own_domain_invalid"},
		{name: "a wildcard", raw: "*.acme.com", code: "admin.own_domain_invalid"},
		{name: "a label the encoding refuses", raw: "straß-.acme.com", code: "admin.own_domain_invalid"},
		{name: "the installation's own host", raw: "tasks.example.net", code: "admin.own_domain_installation"},
		{name: "beneath the installation's host", raw: "acme.tasks.example.net", code: "admin.own_domain_installation"},
		{name: "an apex", raw: "acme.com", code: "admin.own_domain_apex"},
		{name: "an apex under a two-label suffix", raw: "acme.co.uk", code: "admin.own_domain_apex"},
		{name: "a public suffix", raw: "co.uk", code: "admin.own_domain_invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := OwnDomainOf(c.raw, "tasks.example.net", fakeDomains{}, fakeRegistry{})
			if c.code == "" {
				if err != nil || got != c.want {
					t.Fatalf("OwnDomainOf(%q) = (%q, %v), want %q", c.raw, got, err, c.want)
				}
				return
			}
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != c.code || !errors.Is(err, shared.ErrValidation) ||
				len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/domain" {
				t.Fatalf("OwnDomainOf(%q) answered (%q, %v), want %s at /domain", c.raw, got, err, c.code)
			}
		})
	}
}

// The other two refusals of check 4 are the store's; both are at the field, and neither names the
// workspace that holds the domain (P-01).
func TestATakenOrBlockedDomainIsRefusedAtTheField(t *testing.T) {
	for code, err := range map[string]error{
		"admin.own_domain_taken":   OwnDomainTaken("tasks.northwind.example"),
		"admin.own_domain_blocked": OwnDomainBlocked("acme-login.example"),
	} {
		refusal := shared.AsError(err)
		if refusal == nil || refusal.DetailCode != code || !errors.Is(err, shared.ErrConflict) ||
			len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/domain" || len(refusal.Params) != 1 {
			t.Errorf("%s: %v", code, err)
		}
	}
}

// UC-INS-15 check 2: the two records name the installation's target and carry the row's mark.
func TestTheTwoRecords(t *testing.T) {
	if got := OwnDomainTarget("Tasks.Example.Net."); got != "domains.tasks.example.net" {
		t.Errorf("target = %q", got)
	}
	if got := OwnDomainProofName("tasks.acme.com"); got != "_hubtask.tasks.acme.com" {
		t.Errorf("proof name = %q", got)
	}
	row, err := NewOwnDomain(NewOwnDomainInput{
		TenantID: shared.ID("01900000-0000-7000-8000-000000000001"), Host: "tasks.acme.com",
		Mark: "q7n4kx2p8zram3vd", Now: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Verification != "hubtask-domain=q7n4kx2p8zram3vd" || row.State != HostPending ||
		row.Kind != HostKindOwnDomain || row.Canonical {
		t.Errorf("row = %+v", row)
	}
	if _, err := NewOwnDomain(NewOwnDomainInput{Host: "tasks.acme.com", Mark: "short"}); err == nil {
		t.Error("an incomplete row was built")
	}
}

// UC-INS-15 check 3: verification moves by itself - pending to verified once both records are
// found, verified or active to broken once the proof is gone, broken back once both are there -
// and a lookup that got no answer moves nothing.
func TestALookMovesAnOwnDomain(t *testing.T) {
	both := DomainLookup{Known: true, TargetFound: true, ProofFound: true}
	proofOnly := DomainLookup{Known: true, ProofFound: true}
	targetOnly := DomainLookup{Known: true, TargetFound: true}
	none := DomainLookup{Known: true}
	unknown := DomainLookup{}
	cases := []struct {
		name    string
		current HostState
		look    DomainLookup
		asked   bool
		want    HostState
	}{
		{"pending, both found", HostPending, both, false, HostVerified},
		{"pending, only the proof", HostPending, proofOnly, false, HostPending},
		{"pending, only the target", HostPending, targetOnly, false, HostPending},
		{"pending, nothing", HostPending, none, false, HostPending},
		{"pending, no answer", HostPending, unknown, false, HostPending},
		{"verified, both still there", HostVerified, both, false, HostVerified},
		{"verified, the proof gone", HostVerified, targetOnly, false, HostBroken},
		{"verified, the target gone but the proof kept", HostVerified, proofOnly, false, HostVerified},
		{"active, the proof gone", HostActive, none, true, HostBroken},
		{"active, no answer", HostActive, unknown, true, HostActive},
		{"broken, back and asked before", HostBroken, both, true, HostActive},
		{"broken, back and never asked", HostBroken, both, false, HostVerified},
		{"broken, still gone", HostBroken, targetOnly, true, HostBroken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextOwnDomainState(c.current, c.look, c.asked); got != c.want {
				t.Errorf("NextOwnDomainState = %s, want %s", got, c.want)
			}
		})
	}
}

func TestTheProofIsTheMarkExactly(t *testing.T) {
	mark := "hubtask-domain=q7n4kx2p8zram3vd"
	cases := []struct {
		texts []string
		want  bool
	}{
		{[]string{mark}, true},
		{[]string{"v=spf1 -all", ` "` + mark + `" `}, true},
		{[]string{"x" + mark}, false},
		{[]string{mark + " and more"}, false},
		{[]string{"hubtask-domain=another"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := ProofCarried(c.texts, mark); got != c.want {
			t.Errorf("ProofCarried(%q) = %v", c.texts, got)
		}
	}
}

func TestThePointingRecordReachesTheTarget(t *testing.T) {
	cases := []struct {
		name                            string
		domain, target, targetCanonical string
		want                            bool
	}{
		{"straight at the target", "domains.tasks.example.net.", "domains.tasks.example.net", "domains.tasks.example.net", true},
		{"through the target to its end", "lb.provider.example.", "domains.tasks.example.net", "lb.provider.example.", true},
		{"elsewhere", "pages.elsewhere.example.", "domains.tasks.example.net", "lb.provider.example.", false},
		{"no such name", "", "domains.tasks.example.net", "", false},
	}
	for _, c := range cases {
		if got := TargetReached(c.domain, c.target, c.targetCanonical); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// UC-INS-15 check 10: the operator's block carries a reason, which the owner reads word for word.
func TestABlockNeedsItsReason(t *testing.T) {
	if got, err := BlockReason("  Reported as a look-alike.  "); err != nil || got != "Reported as a look-alike." {
		t.Errorf("BlockReason = (%q, %v)", got, err)
	}
	for raw, code := range map[string]string{
		"   ":                    "admin.own_domain_reason_required",
		strings.Repeat("x", 501): "admin.own_domain_reason_too_long",
	} {
		if _, err := BlockReason(raw); shared.AsError(err) == nil || shared.AsError(err).DetailCode != code {
			t.Errorf("BlockReason(%d runes) = %v, want %s", len(raw), err, code)
		}
	}
}

func TestOwnDomainsWaitForTheInstallation(t *testing.T) {
	if err := (OwnDomainRule{}).Refuse(); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "access.own_domains_not_allowed" || !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("Refuse = %v", err)
	}
	if err := (OwnDomainRule{Allowed: true}).Refuse(); err != nil {
		t.Errorf("Refuse = %v", err)
	}
}

// UC-INS-15 checks 6 and 8: the main address moves to an active own domain and back, each time
// naming where it leaves and where it goes, and never where there is no way back.
func TestTheMainAddressMovesBetweenTheOwnDomainAndTheSubdomain(t *testing.T) {
	const subdomain = "acme.tasks.example.net"
	own := func(state HostState, main bool) OwnDomain {
		return OwnDomain{Host: "tasks.acme.com", State: state, Main: main}
	}
	cases := []struct {
		name     string
		own      OwnDomain
		address  string
		toOwn    bool
		from, to string
		refused  string
	}{
		{"an active domain becomes main", own(HostActive, false), subdomain, true, subdomain, "tasks.acme.com", ""},
		{"and the subdomain comes back", own(HostActive, true), subdomain, false, "tasks.acme.com", subdomain, ""},
		{"a verified domain waits for its first request", own(HostVerified, false), subdomain, true, "", "", "admin.own_domain_not_active"},
		{"a pending domain does not", own(HostPending, false), subdomain, true, "", "", "admin.own_domain_not_active"},
		{"a broken domain does not", own(HostBroken, false), subdomain, true, "", "", "admin.own_domain_not_active"},
		{"a main domain is main already", own(HostActive, true), subdomain, true, "", "", "admin.own_domain_already_main"},
		{"there is nothing to switch back from", own(HostActive, false), subdomain, false, "", "", "admin.own_domain_not_main"},
		{"no way back, no move", own(HostActive, false), "", true, "", "", "admin.own_domain_no_way_back"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			from, to, err := MainAddressMove(c.own, c.address, c.toOwn)
			if c.refused != "" {
				if shared.AsError(err).DetailCode != c.refused {
					t.Fatalf("refused with %v, want %s", err, c.refused)
				}
				return
			}
			if err != nil || from != c.from || to != c.to {
				t.Fatalf("moved %q to %q (%v), want %q to %q", from, to, err, c.from, c.to)
			}
		})
	}
}
