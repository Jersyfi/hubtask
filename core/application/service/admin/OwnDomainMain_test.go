// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// UC-INS-15 checks 6, 8, 9 and 11 at the application layer: making the own domain the main address
// and switching back each ask what a change of address asks and move the main address once; a main
// domain that breaks, or that the operator blocks, hands the main address back in the same
// transaction; each move is in the workspace's trail.

const subdomain = "meier.tasks.example.net"

type mainFixture struct {
	*ownDomainFixture
	address *addressFixture
	shared  MainAddress
}

// newMainFixture is a workspace whose active own domain is not yet the main address.
func newMainFixture(t *testing.T, allowed bool) *mainFixture {
	t.Helper()
	f := &mainFixture{ownDomainFixture: newOwnDomainFixture(true), address: newAddressFixture(env.HostsSubdomain)}
	if err := f.setUp(t, workspacePerson(addressOwner), "tasks.acme.com"); err != nil {
		t.Fatal(err)
	}
	f.store.rows[lifecycleTenant].State = domain.HostActive
	f.settings.level.OwnersOwnDomains = allowed
	change := f.address.handler
	change.UnitOfWork = f.ownDomainFixture.shared.UnitOfWork
	f.shared = MainAddress{OwnDomains: f.ownDomainFixture.shared, Change: change, Authorizer: f.authorizer}
	return f
}

func (f *mainFixture) makeMain(t *testing.T, cmd MainAddressCommand) (AddressChange, error) {
	t.Helper()
	return MakeOwnDomainMain{MainAddress: f.shared}.Execute(t.Context(), workspacePerson(addressOwner), cmd)
}

func (f *mainFixture) switchBack(t *testing.T, cmd MainAddressCommand) (AddressChange, error) {
	t.Helper()
	return SwitchBackFromOwnDomain{MainAddress: f.shared}.Execute(t.Context(), workspacePerson(addressOwner), cmd)
}

func (f *mainFixture) lastTrail() audit.Entry { return f.audit.entries[len(f.audit.entries)-1] }

// Checks 6, 8 and 11, and the acceptance's "each replace the main row once": the dry run names both
// addresses and who is told and moves nothing; the move asks for a password or provider proof; then
// it moves the main address once, ends the sessions, tells the members and writes the trail - and
// the way back does the same in the other direction, the own domain staying set up.
func TestMakingTheOwnDomainMainAndSwitchingBackEachMoveTheMainAddressOnce(t *testing.T) {
	f := newMainFixture(t, true)

	dry, err := f.makeMain(t, MainAddressCommand{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.From != subdomain || dry.To != "tasks.acme.com" || dry.Mailed != 2 || dry.ToldAtSignIn != 1 || !dry.DryRun {
		t.Errorf("dry run answered %+v", dry)
	}
	if f.store.toOwn != 0 || f.address.hosts.revoked != 0 || f.address.notices.calls != 0 {
		t.Fatal("the dry run moved something")
	}

	var refused *shared.Error
	if _, err := f.makeMain(t, MainAddressCommand{}); !errors.As(err, &refused) ||
		refused.DetailCode != stepup.CodeRequired || refused.Params["purpose"] != "change_address" {
		t.Fatalf("without a proof: %v", err)
	}
	f.address.stepUp.method = stepup.MethodPasskey
	if _, err := f.makeMain(t, MainAddressCommand{StepUpToken: addressProof}); detailOf(err) != stepup.CodeMethodRefused {
		t.Fatalf("a passkey's proof: %v", err)
	}
	f.address.stepUp.method = stepup.MethodPassword
	if f.store.toOwn != 0 {
		t.Fatal("the main address moved without a proof")
	}

	made, err := f.makeMain(t, MainAddressCommand{StepUpToken: addressProof})
	if err != nil {
		t.Fatalf("make main: %v", err)
	}
	if made.From != subdomain || made.To != "tasks.acme.com" {
		t.Errorf("answered %+v", made)
	}
	if f.store.toOwn != 1 || !f.store.rows[lifecycleTenant].Main {
		t.Errorf("moved %d times, main %v", f.store.toOwn, f.store.rows[lifecycleTenant].Main)
	}
	if f.address.hosts.revoked != 1 {
		t.Errorf("sessions ended %d times", f.address.hosts.revoked)
	}
	if f.address.notices.calls != 1 || f.address.notices.from != subdomain || f.address.notices.to != "tasks.acme.com" {
		t.Errorf("notices %+v", f.address.notices)
	}
	if entry := f.lastTrail(); entry.Action != OwnDomainMadeMainAction || entry.ActorID != addressOwner ||
		entry.Severity != audit.SeverityWarning {
		t.Errorf("trail %+v", entry)
	}
	if _, err := f.makeMain(t, MainAddressCommand{DryRun: true}); detailOf(err) != "admin.own_domain_already_main" {
		t.Errorf("made main twice: %v", err)
	}

	// Check 8: the way back, at the same cost, and the own domain stays.
	back, err := f.switchBack(t, MainAddressCommand{StepUpToken: addressProof})
	if err != nil {
		t.Fatalf("switch back: %v", err)
	}
	if back.From != "tasks.acme.com" || back.To != subdomain {
		t.Errorf("answered %+v", back)
	}
	if f.store.toAddress != 1 || f.store.rows[lifecycleTenant] == nil || f.store.rows[lifecycleTenant].Main {
		t.Errorf("moved back %d times, row %+v", f.store.toAddress, f.store.rows[lifecycleTenant])
	}
	if f.address.hosts.revoked != 2 || f.address.notices.calls != 2 || f.address.notices.to != subdomain {
		t.Errorf("sessions %d, notices %+v", f.address.hosts.revoked, f.address.notices)
	}
	if entry := f.lastTrail(); entry.Action != OwnDomainSwitchedBackAction {
		t.Errorf("trail %+v", entry)
	}
	if _, err := f.switchBack(t, MainAddressCommand{DryRun: true}); detailOf(err) != "admin.own_domain_not_main" {
		t.Errorf("switched back twice: %v", err)
	}
}

func TestTheMainAddressMovesOnlyWhereItMay(t *testing.T) {
	t.Run("only an active domain becomes main", func(t *testing.T) {
		f := newMainFixture(t, true)
		f.store.rows[lifecycleTenant].State = domain.HostVerified
		if _, err := f.makeMain(t, MainAddressCommand{DryRun: true}); detailOf(err) != "admin.own_domain_not_active" {
			t.Errorf("a verified domain: %v", err)
		}
	})
	t.Run("only an owner moves it", func(t *testing.T) {
		f := newMainFixture(t, true)
		_, err := MakeOwnDomainMain{MainAddress: f.shared}.Execute(t.Context(), workspacePerson(mailedMember), MainAddressCommand{DryRun: true})
		if detailOf(err) != "access.owner_required" {
			t.Errorf("a member: %v", err)
		}
	})
	t.Run("the switch holds making main, never the way back", func(t *testing.T) {
		f := newMainFixture(t, false)
		if _, err := f.makeMain(t, MainAddressCommand{DryRun: true}); detailOf(err) != "access.own_domains_not_allowed" {
			t.Errorf("switch off: %v", err)
		}
		f.store.rows[lifecycleTenant].Main = true
		if _, err := f.switchBack(t, MainAddressCommand{DryRun: true}); err != nil {
			t.Errorf("the way back with the switch off: %v", err)
		}
	})
	t.Run("no domain, nothing to move", func(t *testing.T) {
		f := newMainFixture(t, true)
		delete(f.store.rows, lifecycleTenant)
		if _, err := f.makeMain(t, MainAddressCommand{DryRun: true}); detailOf(err) != "admin.own_domain_none" {
			t.Errorf("no domain: %v", err)
		}
	})
	t.Run("the lockout is refused before the proof is spent", func(t *testing.T) {
		f := newMainFixture(t, true)
		f.address.ways[addressOwner] = domain.WaysHeld{Passkeys: true}
		_, err := f.makeMain(t, MainAddressCommand{StepUpToken: addressProof})
		if detailOf(err) != "admin.address_lockout_self" {
			t.Fatalf("a passkey-only owner: %v", err)
		}
		if f.address.stepUp.consumed != 0 || f.store.toOwn != 0 {
			t.Error("a refused move burned the proof or moved the address")
		}
	})
}

// Check 9 and the acceptance's "a broken main domain falls back in the same transaction": the
// check that finds the proof gone turns the domain broken and makes the subdomain main in the one
// transaction it writes the look in, keeps the moment for the warning, and writes both to the trail.
func TestABrokenMainDomainFallsBackInTheSameTransaction(t *testing.T) {
	f := newMainFixture(t, true)
	row := f.store.rows[lifecycleTenant]
	row.Main = true
	f.dns.cname["tasks.acme.com"] = "domains.tasks.example.net."

	if moved := f.check(t); moved != 1 || row.State != domain.HostBroken {
		t.Fatalf("moved %d, state %s", moved, row.State)
	}
	if row.Main || f.store.toAddress != 1 || !row.FellBackAt.Equal(now) {
		t.Errorf("main %v, moved back %d times, fell back at %v", row.Main, f.store.toAddress, row.FellBackAt)
	}
	if f.store.movedIn != f.store.wroteIn {
		t.Errorf("the fall-back ran in transaction %d, the look in %d", f.store.movedIn, f.store.wroteIn)
	}
	entry := f.lastTrail()
	if entry.Action != OwnDomainBrokenAction || entry.Changes["host"] == nil {
		t.Errorf("trail %+v", entry)
	}

	// Back by itself, it stays not main: the owner makes it main again.
	f.dns.txt["_hubtask.tasks.acme.com"] = []string{row.Verification}
	row.CertificateAsked = true
	if f.check(t); row.State != domain.HostActive || row.Main || row.FellBackAt.IsZero() {
		t.Errorf("recovered %+v", row)
	}
	if _, err := f.makeMain(t, MainAddressCommand{StepUpToken: addressProof}); err != nil || !row.FellBackAt.IsZero() {
		t.Errorf("made main again: %v, fell back at %v", err, row.FellBackAt)
	}

	// A domain that was not main breaks without moving anything.
	g := newMainFixture(t, true)
	g.dns.cname["tasks.acme.com"] = "domains.tasks.example.net."
	if g.check(t); g.store.toAddress != 0 || !g.store.rows[lifecycleTenant].FellBackAt.IsZero() {
		t.Errorf("a domain that was not main fell back")
	}
}

// Check 9, the operator's half: blocking the main domain makes the subdomain main in the same step,
// and the block keeps that it was main for the owner's warning.
func TestABlockedMainDomainFallsBackInTheSameTransaction(t *testing.T) {
	f := newMainFixture(t, true)
	f.store.rows[lifecycleTenant].Main = true
	blocked, err := BlockOwnDomain{OwnDomains: f.ownDomainFixture.shared}.Execute(t.Context(), operator(), "tasks.acme.com", "A look-alike.")
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if !blocked.Main || f.store.toAddress != 1 || f.store.movedIn != f.store.wroteIn {
		t.Errorf("answered %+v, moved back %d times in %d, removed in %d", blocked, f.store.toAddress, f.store.movedIn, f.store.wroteIn)
	}
	if block := f.store.blocks["tasks.acme.com"]; block == nil || !block.block.WasMain {
		t.Errorf("block %+v", block)
	}
	if entry := f.lastTrail(); entry.Action != OwnDomainBlockedAction || entry.Changes["host"] == nil {
		t.Errorf("trail %+v", entry)
	}
}
