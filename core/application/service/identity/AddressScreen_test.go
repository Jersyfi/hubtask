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
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

type lastChangeFake struct{ at time.Time }

func (c lastChangeFake) LastAddressChange(context.Context) (time.Time, error) { return c.at, nil }
func (c lastChangeFake) LockAddressChange(context.Context) (time.Time, error) { return c.at, nil }

// UC-INS-19 check 3: the workspace's read answers its main address and whether the reader may change
// it - off by default and in single mode, the owner's alone where it is on, and inside the cooldown
// the moment a change is possible again - so that the *Address* screen draws the control only where
// it would be accepted and names the day where it would not.
func TestTheWorkspaceAnswersWhoMayChangeItsAddress(t *testing.T) {
	read := at()
	changed := read.Add(-10 * 24 * time.Hour)
	cases := []struct {
		name          string
		tenancy       env.TenancyMode
		allowed       bool
		owner         bool
		last          time.Time
		wantMay       bool
		wantOwnersMay bool
		wantFrom      time.Time
	}{
		{name: "off by default", tenancy: env.TenancyMulti, owner: true},
		{name: "single mode", tenancy: env.TenancySingle, allowed: true, owner: true},
		{name: "on, the owner", tenancy: env.TenancyMulti, allowed: true, owner: true, wantMay: true, wantOwnersMay: true},
		{name: "on, not an owner", tenancy: env.TenancyMulti, allowed: true, wantOwnersMay: true},
		{name: "on, inside the cooldown", tenancy: env.TenancyMulti, allowed: true, owner: true, last: changed,
			wantOwnersMay: true, wantFrom: changed.Add(30 * 24 * time.Hour)},
		{name: "on, past the cooldown", tenancy: env.TenancyMulti, allowed: true, owner: true,
			last: read.Add(-31 * 24 * time.Hour), wantMay: true, wantOwnersMay: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newWorkspaceFixture(read)
			owners := &ownersFake{}
			if tc.owner {
				owners.paths = []repository.OwnerPath{{AccountID: workspaceActor().AccountID}}
			}
			fixture.writer.Address = WorkspaceAddressReader{
				Tenancy: tc.tenancy, InstallationHost: "tasks.example.net",
				Settings: &instanceSettings{level: repository.InstanceLevel{OwnersChangeAddress: tc.allowed}},
				Changes:  lastChangeFake{at: tc.last}, Owners: owners, Clock: clock.Fixed(read),
			}
			registry, err := usecase.NewRegistry(nil, ReadWorkspace{Writer: fixture.writer}.Descriptor())
			if err != nil {
				t.Fatalf("registering: %v", err)
			}
			address, _ := readWorkspace(t, registry)["address"].(usecase.Output)
			if address == nil {
				t.Fatal("no address answered")
			}
			wantHost := domain.CanonicalHostOf("acme", "tasks.example.net", tc.tenancy == env.TenancyMulti)
			if address["host"] != wantHost || address["may_change"] != tc.wantMay || address["owners_may_change"] != tc.wantOwnersMay {
				t.Errorf("address %v, want host %s, may %v, owners may %v", address, wantHost, tc.wantMay, tc.wantOwnersMay)
			}
			from, _ := address["changeable_from"].(time.Time)
			if !from.Equal(tc.wantFrom) {
				t.Errorf("changeable_from %v, want %v", from, tc.wantFrom)
			}
			if changedAt, held := address["changed_at"].(time.Time); held != !tc.last.IsZero() || (held && !changedAt.Equal(tc.last)) {
				t.Errorf("changed_at %v, want %v", address["changed_at"], tc.last)
			}
		})
	}
}

// The main row names the address where there is one: the host every link is written with.
func TestTheWorkspacesAddressIsItsMainRow(t *testing.T) {
	fixture := newWorkspaceFixture(at())
	fixture.writer.Hosts = tenantHostsFake{hosts: []domain.TenantHost{{Host: "familie-meier.tasks.example.net", Canonical: true, State: domain.HostActive}}}
	fixture.writer.Address = WorkspaceAddressReader{
		Tenancy: env.TenancyMulti, InstallationHost: "tasks.example.net",
		Changes: lastChangeFake{}, Owners: &ownersFake{}, Clock: clock.Fixed(at()),
	}
	registry, err := usecase.NewRegistry(nil, ReadWorkspace{Writer: fixture.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	address, _ := readWorkspace(t, registry)["address"].(usecase.Output)
	if address["host"] != "familie-meier.tasks.example.net" {
		t.Errorf("host %v", address["host"])
	}
}
