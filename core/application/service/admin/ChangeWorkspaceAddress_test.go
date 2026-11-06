// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// UC-INS-19 check 3: the workspace's owner changes the address where the installation allows it -
// off by default, in multi mode only - and not again inside its cooldown, measured by the Clock;
// the owner's role is required, and the installation alone decides: no workspace switches it on,
// and the operator is held by neither the switch nor the cooldown.

var addressAdmin = shared.ID("018f2a1b-0000-7000-8000-0000000000c3")

type workspaceAuthorizerFake struct {
	refuse   bool
	requests []access.Request
}

func (a *workspaceAuthorizerFake) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.requests = append(a.requests, request)
	if a.refuse {
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return nil
}

// addressChangesFake is the moment of the last change; locked is what the write's locked read
// answers, as another owner's change committed in between would leave it.
type addressChangesFake struct {
	last, locked time.Time
	lockedReads  int
}

func (c *addressChangesFake) LastAddressChange(context.Context) (time.Time, error) {
	return c.last, nil
}

func (c *addressChangesFake) LockAddressChange(context.Context) (time.Time, error) {
	c.lockedReads++
	if !c.locked.IsZero() {
		return c.locked, nil
	}
	return c.last, nil
}

type ownerFixture struct {
	*addressFixture
	owner      ChangeWorkspaceAddress
	authorizer *workspaceAuthorizerFake
	changes    *addressChangesFake
	settings   *instanceStore
}

func newOwnerFixture(allowed bool) *ownerFixture {
	f := &ownerFixture{
		addressFixture: newAddressFixture(env.HostsSubdomain),
		authorizer:     &workspaceAuthorizerFake{}, changes: &addressChangesFake{},
	}
	_, f.settings, _ = newInstanceWriter(newRegister(operatorID))
	f.settings.level.OwnersChangeAddress = allowed
	f.handler.Settings, f.handler.Changes = f.settings, f.changes
	f.owner = ChangeWorkspaceAddress{Authorizer: f.authorizer}
	return f
}

func (f *ownerFixture) changeAsOwner(t *testing.T, actor appshared.ActorContext, cmd ChangeTenantAddressCommand) (AddressChange, error) {
	t.Helper()
	f.owner.Change = f.handler
	return f.owner.Execute(t.Context(), actor, cmd)
}

// workspacePerson is a person signed in to the workspace whose address changes.
func workspacePerson(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: lifecycleTenant, AccountID: account, AccountName: "Eva Meier",
		Scopes: []string{workspaceManageScope},
	}
}

func TestTheOwnerChangesTheAddressWhereTheInstallationAllowsIt(t *testing.T) {
	f := newOwnerFixture(true)

	changed, err := f.changeAsOwner(t, workspacePerson(addressOwner),
		ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof})
	if err != nil {
		t.Fatalf("the owner's change: %v", err)
	}
	if changed.From != "meier.tasks.example.net" || changed.To != "familie-meier.tasks.example.net" ||
		f.hosts.main[lifecycleTenant] != "familie-meier.tasks.example.net" {
		t.Fatalf("from %q to %q, hosts %v", changed.From, changed.To, f.hosts.main)
	}
	// The same change as the operator's: the proof, the sessions, the notices, the trail.
	if f.stepUp.consumed != 1 || f.hosts.revoked != 1 || f.notices.calls != 1 {
		t.Errorf("step-up %d, sessions ended %d, notices %d", f.stepUp.consumed, f.hosts.revoked, f.notices.calls)
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantAddressChangedAction ||
		f.audit.entries[0].ActorID != addressOwner {
		t.Errorf("trail %+v", f.audit.entries)
	}
	// Check 11: only the operator's change is in the instance journal.
	if len(f.journal.entries) != 0 {
		t.Errorf("an owner's change was journalled: %+v", f.journal.entries)
	}
	// The role matrix is asked for STRUCTURE on the workspace, with the workspace's token scope.
	if len(f.authorizer.requests) != 1 || f.authorizer.requests[0].Permission != service.PermissionStructure ||
		f.authorizer.requests[0].TokenScope != workspaceManageScope {
		t.Errorf("authorizer asked %+v", f.authorizer.requests)
	}
	if f.changes.lockedReads != 1 {
		t.Errorf("the cooldown was checked %d times inside the write", f.changes.lockedReads)
	}
}

// Off by default, and off in single mode whatever the level says: refused with its code before
// anything is checked or written, a dry run included.
func TestTheSwitchOffRefusesTheOwner(t *testing.T) {
	for name, f := range map[string]*ownerFixture{
		"off by default": newOwnerFixture(false),
		"single mode": func() *ownerFixture {
			f := newOwnerFixture(true)
			f.handler.Tenancy = env.TenancySingle
			return f
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, cmd := range []ChangeTenantAddressCommand{
				{Slug: "familie-meier", StepUpToken: addressProof}, {Slug: "familie-meier", DryRun: true},
			} {
				_, err := f.changeAsOwner(t, workspacePerson(addressOwner), cmd)
				if detailOf(err) != "access.address_change_not_allowed" || !errors.Is(err, shared.ErrForbidden) {
					t.Fatalf("answered %v, want access.address_change_not_allowed", err)
				}
			}
			if f.stepUp.consumed != 0 || f.hosts.main[lifecycleTenant] != "meier.tasks.example.net" || len(f.audit.entries) != 0 {
				t.Fatal("a refused change changed something")
			}
		})
	}
}

// The owner's role is required: an administrator holding STRUCTURE is refused, and so is anybody
// the role matrix refuses.
func TestTheOwnersRoleIsRequired(t *testing.T) {
	f := newOwnerFixture(true)
	_, err := f.changeAsOwner(t, workspacePerson(addressAdmin), ChangeTenantAddressCommand{Slug: "familie-meier", DryRun: true})
	if detailOf(err) != "access.owner_required" || !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("an administrator answered %v, want access.owner_required", err)
	}

	f.authorizer.refuse = true
	_, err = f.changeAsOwner(t, workspacePerson(addressOwner), ChangeTenantAddressCommand{Slug: "familie-meier", DryRun: true})
	if detailOf(err) != "access.not_permitted" {
		t.Fatalf("a refused role answered %v", err)
	}
}

// The cooldown runs from the last change - by anybody - for 30 days, or the installation's own
// number, and the refusal names the moment a change is possible again.
func TestTheCooldownHoldsTheOwnerUntilTheDayItNames(t *testing.T) {
	cases := []struct {
		name     string
		days     int
		since    time.Duration
		wantFrom string
	}{
		{"ten days after", 0, 10 * 24 * time.Hour, now.Add(20 * 24 * time.Hour).Format(time.RFC3339)},
		{"a second short of 30 days", 0, 30*24*time.Hour - time.Second, now.Add(time.Second).Format(time.RFC3339)},
		{"30 days after", 0, 30 * 24 * time.Hour, ""},
		{"the installation's 7 days, after 6", 7, 6 * 24 * time.Hour, now.Add(24 * time.Hour).Format(time.RFC3339)},
		{"the installation's 7 days, after 7", 7, 7 * 24 * time.Hour, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerFixture(true)
			f.settings.level.AddressCooldownDays = tc.days
			f.changes.last = now.Add(-tc.since)

			_, err := f.changeAsOwner(t, workspacePerson(addressOwner),
				ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof})
			if tc.wantFrom == "" {
				if err != nil {
					t.Fatalf("past the cooldown: %v", err)
				}
				return
			}
			var refused *shared.Error
			if !errors.As(err, &refused) || refused.DetailCode != "access.address_change_cooldown" ||
				refused.Params["possible_at"] != tc.wantFrom {
				t.Fatalf("answered %v, want access.address_change_cooldown from %s", err, tc.wantFrom)
			}
			if f.stepUp.consumed != 0 {
				t.Error("a change inside the cooldown burned the step-up")
			}
		})
	}
}

// Two owners at once: the second's write finds the first's change under the row's lock and is
// refused there.
func TestTheCooldownIsCheckedAgainInsideTheWrite(t *testing.T) {
	f := newOwnerFixture(true)
	f.changes.locked = now.Add(-time.Minute)

	_, err := f.changeAsOwner(t, workspacePerson(addressOwner),
		ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof})
	if detailOf(err) != "access.address_change_cooldown" {
		t.Fatalf("answered %v, want access.address_change_cooldown", err)
	}
	if len(f.audit.entries) != 0 || f.notices.calls != 0 {
		t.Error("the refused write recorded or told somebody")
	}
}

// The installation's lock: the switch and the cooldown are the installation's, held by no
// workspace and binding no operator.
func TestTheOperatorIsHeldByNeitherTheSwitchNorTheCooldown(t *testing.T) {
	f := newOwnerFixture(false)
	f.changes.last = now.Add(-time.Hour)
	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof}); err != nil {
		t.Fatalf("the operator's change: %v", err)
	}
	if f.changes.lockedReads != 0 || len(f.journal.entries) != 1 {
		t.Errorf("cooldown read %d times, journal %+v", f.changes.lockedReads, f.journal.entries)
	}
}

// The switch and the cooldown travel through the instance level's doors - the registry REST and
// hubctl reach, and the instance file - the same way, and an out-of-range cooldown is refused.
func TestTheSwitchAndTheCooldownAreTheInstallationsSettings(t *testing.T) {
	for _, door := range []string{"registry", "file"} {
		t.Run(door, func(t *testing.T) {
			writer, store, _ := newInstanceWriter(newRegister(operatorID))
			write := WriteInstanceSettings{Writer: writer}
			registry, err := usecase.NewRegistry(nil, write.Descriptor(), ReadInstanceSettings{Writer: writer}.Descriptor())
			if err != nil {
				t.Fatalf("registering: %v", err)
			}
			save := func(document map[string]any) error {
				var decoded map[string]any
				encoded, _ := json.Marshal(document)
				if err := json.Unmarshal(encoded, &decoded); err != nil {
					t.Fatalf("decoding: %v", err)
				}
				if door == "registry" {
					_, err := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), usecase.Input(decoded))
					return err
				}
				level, err := InstanceLevelOf(decoded)
				if err != nil {
					return err
				}
				_, err = write.ExecuteAsFile(t.Context(), level, "/etc/hubtask/instance.json")
				return err
			}

			if store.level.OwnersChangeAddress {
				t.Fatal("owners may change the address before anybody decided it")
			}
			if err := save(map[string]any{"owners_change_address": true, "address_change_cooldown_days": 14}); err != nil {
				t.Fatalf("saving: %v", err)
			}
			if !store.level.OwnersChangeAddress || store.level.AddressCooldownDays != 14 {
				t.Fatalf("stored %+v", store.level)
			}
			read, err := registry.Invoke(t.Context(), ReadInstanceSettingsName, operator(), usecase.Input{})
			if err != nil || read["owners_change_address"] != true || read["address_change_cooldown_days"] != 14 {
				t.Fatalf("read back %v, %v", read, err)
			}

			// The default sent back as it was read stays undecided.
			if err := save(map[string]any{"owners_change_address": true, "address_change_cooldown_days": 30}); err != nil {
				t.Fatalf("saving the default: %v", err)
			}
			if store.level.AddressCooldownDays != 0 {
				t.Errorf("the default was stored as %d", store.level.AddressCooldownDays)
			}

			if err := save(map[string]any{"address_change_cooldown_days": 400}); detailOf(err) != "admin.address_cooldown_invalid" {
				t.Errorf("a cooldown of 400 days answered %v", err)
			}
			if !store.level.OwnersChangeAddress {
				t.Error("a refused save was written")
			}
		})
	}
}
