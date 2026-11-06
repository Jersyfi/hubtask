// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// entrySignerFake signs with a key of its own; an entry signed with another key is another
// installation's.
type entrySignerFake struct{ key string }

func (s entrySignerFake) Sign(entry domain.DeviceListEntry) string {
	return entry.Host + "|" + entry.Name + "|" + s.key
}

func (s entrySignerFake) Read(signed string) (domain.DeviceListEntry, error) {
	parts := strings.Split(signed, "|")
	if len(parts) != 3 || parts[2] != s.key {
		return domain.DeviceListEntry{}, shared.ErrValidation.WithDetail("auth.device_list_entry_invalid")
	}
	return domain.DeviceListEntry{Host: parts[0], Name: parts[1]}, nil
}

// mainAddressesFake answers yes for the hosts it holds, and records what it was asked.
type mainAddressesFake struct {
	main  map[string]bool
	asked []string
}

func (m *mainAddressesFake) IsMainHost(_ context.Context, host string) (bool, error) {
	m.asked = append(m.asked, host)
	return m.main[host], nil
}

// UC-ID-25 checks 5 and 6: the list takes only an entry this installation signed whose host is
// still a main address. A forged entry is left out of the answer and its host never asked about;
// an entry whose workspace moved answers no.
func TestTheCheckAcceptsOnlyTheInstallationsOwnSignatureAndACurrentMainAddress(t *testing.T) {
	own, foreign := entrySignerFake{key: "ours"}, entrySignerFake{key: "theirs"}
	hosts := &mainAddressesFake{main: map[string]bool{"acme.hub.example": true}}
	uow := &unitOfWork{}
	check := CheckDeviceList{Signer: own, Hosts: hosts, UnitOfWork: uow, Multi: true}

	current := own.Sign(domain.DeviceListEntry{Host: "acme.hub.example", Name: "Acme Ltd"})
	moved := own.Sign(domain.DeviceListEntry{Host: "meier.hub.example", Name: "Meier family"})
	forged := foreign.Sign(domain.DeviceListEntry{Host: "acme-login.example", Name: "Acme Ltd"})

	checked, err := check.Execute(t.Context(), []string{forged, current, "plain text", moved})
	if err != nil {
		t.Fatal(err)
	}
	if len(checked) != 2 {
		t.Fatalf("answered %d entries, want the two this installation signed: %+v", len(checked), checked)
	}
	if checked[0].Signed != current || checked[0].Host != "acme.hub.example" || checked[0].Name != "Acme Ltd" || !checked[0].Answers {
		t.Errorf("the current entry: %+v", checked[0])
	}
	if checked[1].Signed != moved || checked[1].Answers {
		t.Errorf("the moved entry answers: %+v", checked[1])
	}
	for _, host := range hosts.asked {
		if host == "acme-login.example" {
			t.Error("the forged entry's host was asked about")
		}
	}
	if len(uow.scopes) != 1 || uow.scopes[0] != persistence.InstallationScope() {
		t.Errorf("scopes %v, want the installation's", uow.scopes)
	}

	// Single mode keeps no list: nothing is answered.
	single := CheckDeviceList{Signer: own, Hosts: hosts, UnitOfWork: uow}
	if none, err := single.Execute(t.Context(), []string{current}); err != nil || len(none) != 0 {
		t.Errorf("single mode answered %+v, %v", none, err)
	}

	// No more than a person's list.
	if _, err := check.Execute(t.Context(), make([]string, DeviceListCheckMax+1)); shared.AsError(err).Category != shared.CategoryValidation {
		t.Errorf("an overlong check: %v", err)
	}
}

// UC-ID-25 check 1: the entry a sign-in hands over is the workspace's main address and display
// name, signed - for a person, in multi mode only.
func TestTheEntryIsTheMainAddressAndTheNameSigned(t *testing.T) {
	tenant := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000250a1")
	account := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000250a2")
	person := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, Scopes: []string{accountsRead}}
	acme := &workspaceStore{row: domain.Workspace{Tenant: domain.Tenant{Slug: "acme", DisplayName: "Acme Ltd"}}}
	hosts := tenantHostsFake{hosts: []domain.TenantHost{{Host: "tasks.acme.com", State: domain.HostActive, Canonical: true}}}
	issue := IssueDeviceListEntry{
		Signer:     entrySignerFake{key: "ours"},
		Address:    WorkspaceAddress{Workspaces: acme, Hosts: hosts, BaseURL: "https://hub.example:8443", Multi: true},
		Workspaces: acme, UnitOfWork: &unitOfWork{},
	}

	entry, err := issue.Execute(t.Context(), person)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Host != "tasks.acme.com" || entry.Name != "Acme Ltd" || entry.Signed != "tasks.acme.com|Acme Ltd|ours" {
		t.Errorf("issued %+v", entry)
	}

	single := issue
	single.Address.Multi = false
	if _, err := single.Execute(t.Context(), person); shared.AsError(err).DetailCode != "access.device_list_not_offered" {
		t.Errorf("single mode: %v", err)
	}
	machine := person
	machine.Kind = appshared.ActorServiceAccount
	if _, err := issue.Execute(t.Context(), machine); shared.AsError(err).Category != shared.CategoryForbidden {
		t.Errorf("a service account: %v", err)
	}
	if _, err := issue.Execute(t.Context(), appshared.ActorContext{}); shared.AsError(err).Category != shared.CategoryUnauthenticated {
		t.Errorf("nobody: %v", err)
	}
}
