// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ownDomainsFake is the workspace's own domain or its block, as the store would answer them.
type ownDomainsFake struct {
	repository.OwnDomains
	view  *domain.OwnDomain
	block *domain.DomainBlock
}

func (f ownDomainsFake) Find(context.Context) (domain.OwnDomain, error) {
	if f.view == nil {
		return domain.OwnDomain{}, shared.ErrNotFound
	}
	return *f.view, nil
}

func (f ownDomainsFake) BlockInView(context.Context) (domain.DomainBlock, error) {
	if f.block == nil {
		return domain.DomainBlock{}, shared.ErrNotFound
	}
	return *f.block, nil
}

func addressOf(t *testing.T, reader WorkspaceAddressReader) usecase.Output {
	t.Helper()
	fixture := newWorkspaceFixture(at())
	fixture.writer.Address = reader
	registry, err := usecase.NewRegistry(nil, ReadWorkspace{Writer: fixture.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	address, _ := readWorkspace(t, registry)["address"].(usecase.Output)
	if address == nil {
		t.Fatal("no address answered")
	}
	return address
}

// UC-INS-15 check 1: the address answers whether the installation lets owners set up an own
// domain - off by default and always in single mode - so the screen draws the panel only there.
func TestTheAddressSaysWhetherOwnDomainsAreAllowed(t *testing.T) {
	cases := []struct {
		name    string
		tenancy env.TenancyMode
		allowed bool
		want    bool
	}{
		{"off by default", env.TenancyMulti, false, false},
		{"on", env.TenancyMulti, true, true},
		{"single mode", env.TenancySingle, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			address := addressOf(t, WorkspaceAddressReader{
				Tenancy: tc.tenancy, InstallationHost: "tasks.example.net",
				Settings: &instanceSettings{level: repository.InstanceLevel{OwnersOwnDomains: tc.allowed}},
				Changes:  lastChangeFake{}, Owners: &ownersFake{}, Clock: clock.Fixed(at()),
				OwnDomains: ownDomainsFake{},
			})
			if address["own_domains_allowed"] != tc.want {
				t.Errorf("own_domains_allowed = %v, want %v", address["own_domains_allowed"], tc.want)
			}
			// Nobody is an owner here, so nobody may set one up.
			if address["may_manage_own_domain"] != false {
				t.Errorf("may_manage_own_domain = %v", address["may_manage_own_domain"])
			}
			if _, held := address["own_domain"]; held {
				t.Error("an own domain was answered where there is none")
			}
		})
	}
}

// UC-INS-15 checks 2, 3 and 10: the owner's view carries the state, the two records with what the
// last look found, and - for a blocked domain - the operator's reason word for word, no records.
func TestTheAddressAnswersTheOwnDomain(t *testing.T) {
	checked := at().Add(-3 * time.Minute)
	address := addressOf(t, WorkspaceAddressReader{
		Tenancy: env.TenancyMulti, InstallationHost: "tasks.example.net",
		Changes: lastChangeFake{}, Owners: &ownersFake{}, Clock: clock.Fixed(at()),
		OwnDomains: ownDomainsFake{view: &domain.OwnDomain{
			Host: "tasks.acme.com", State: domain.HostPending, Verification: "hubtask-domain=q7n4kx2p8zram3vd",
			Since: at().Add(-time.Hour), CheckedAt: checked, ProofFound: true,
		}},
	})
	own, _ := address["own_domain"].(usecase.Output)
	records, _ := own["records"].([]usecase.Output)
	if own["state"] != "PENDING" || own["checked_at"] != checked || len(records) != 2 {
		t.Fatalf("own domain %v", own)
	}
	if records[0]["type"] != "CNAME" || records[0]["value"] != "domains.tasks.example.net" || records[0]["found"] != false ||
		records[1]["name"] != "_hubtask.tasks.acme.com" || records[1]["value"] != "hubtask-domain=q7n4kx2p8zram3vd" ||
		records[1]["found"] != true {
		t.Errorf("records %v", records)
	}

	reason := "Reported as a look-alike of another company's sign-in page."
	address = addressOf(t, WorkspaceAddressReader{
		Tenancy: env.TenancyMulti, InstallationHost: "tasks.example.net",
		Changes: lastChangeFake{}, Owners: &ownersFake{}, Clock: clock.Fixed(at()),
		OwnDomains: ownDomainsFake{block: &domain.DomainBlock{Host: "tasks.acme.com", Reason: reason, BlockedAt: checked}},
	})
	own, _ = address["own_domain"].(usecase.Output)
	if own["state"] != "BLOCKED" || own["blocked_reason"] != reason || len(own["records"].([]usecase.Output)) != 0 {
		t.Errorf("blocked %v", own)
	}
}

// certificateDoors answers yes for the hosts it serves and records each question.
type certificateDoors struct {
	repository.OwnDomainDoors
	serving map[string]bool
	asked   []string
}

func (d *certificateDoors) Certificate(_ context.Context, host string, _ time.Time) (bool, error) {
	d.asked = append(d.asked, host)
	return d.serving[host], nil
}

// UC-INS-15 check 5: the ingress asks the operations port; the answer is the store's, for the
// host as a request would carry it, and single mode answers no without asking.
func TestTheIngressIsAnsweredWhetherAHostMayHaveACertificate(t *testing.T) {
	doors := &certificateDoors{serving: map[string]bool{"tasks.acme.com": true}}
	question := CertificateQuestion{Doors: doors, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(at()), Tenancy: env.TenancyMulti}
	for host, want := range map[string]bool{"Tasks.Acme.com.": true, "tasks.acme.com:443": true, "elsewhere.example": false, "": false} {
		if got, err := question.Allowed(t.Context(), host); err != nil || got != want {
			t.Errorf("Allowed(%q) = (%v, %v), want %v", host, got, err, want)
		}
	}
	question.Tenancy = env.TenancySingle
	asked := len(doors.asked)
	if got, _ := question.Allowed(t.Context(), "tasks.acme.com"); got || len(doors.asked) != asked {
		t.Error("single mode answered or asked")
	}
}

// servingDirectory is a directory whose one host is a verified own domain the ingress asked about.
type servingDirectory struct {
	tenantDirectory
	host   string
	awaits bool
	served int
}

func (d *servingDirectory) ResolveServing(_ context.Context, host string) (shared.ID, bool, error) {
	if host != d.host {
		return "", false, shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
	}
	return d.single, d.awaits, nil
}

func (d *servingDirectory) Served(context.Context, string) (bool, error) {
	d.served++
	d.awaits = false
	return true, nil
}

// UC-INS-15 check 3, "active": a verified own domain the ingress asked about becomes active on the
// first request that reaches its workspace through it - written then, and never again.
func TestTheFirstRequestMakesAVerifiedDomainActive(t *testing.T) {
	tenant := shared.ID("01900000-0000-7000-8000-0000000000aa")
	directory := &servingDirectory{tenantDirectory: tenantDirectory{single: tenant}, host: "tasks.acme.com", awaits: true}
	uow := &unitOfWork{}
	for range 2 {
		got, err := requestTenant(t.Context(), uow, directory, true, "https://tasks.example.net", "tasks.acme.com", "")
		if err != nil || got != tenant {
			t.Fatalf("requestTenant = (%v, %v)", got, err)
		}
	}
	if directory.served != 1 {
		t.Errorf("served %d times, want once", directory.served)
	}
	writes := 0
	for _, scope := range uow.scopes {
		if scope == persistence.SystemScope() {
			writes++
		}
	}
	if writes != 1 {
		t.Errorf("%d writes for two requests", writes)
	}
}

// The owner may set one up and remove it; the switch for changing the address has no say in it.
func TestTheOwnerManagesTheOwnDomain(t *testing.T) {
	address := addressOf(t, WorkspaceAddressReader{
		Tenancy: env.TenancyMulti, InstallationHost: "tasks.example.net",
		Settings: &instanceSettings{level: repository.InstanceLevel{OwnersOwnDomains: true}},
		Changes:  lastChangeFake{}, Clock: clock.Fixed(at()), OwnDomains: ownDomainsFake{},
		Owners: &ownersFake{paths: []repository.OwnerPath{{AccountID: workspaceActor().AccountID}}},
	})
	if address["may_manage_own_domain"] != true || address["may_change"] != false {
		t.Errorf("address %v", address)
	}
}
