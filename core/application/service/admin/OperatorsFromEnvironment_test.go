// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

const (
	adaAccount = shared.ID("01936f2a-7c1e-7000-8000-00000000ada1")
	evaAccount = shared.ID("01936f2a-7c1e-7000-8000-00000000e7a1")
)

type singleResolver map[string]shared.ID

func (s singleResolver) ResolveSingle(_ context.Context, email string) (shared.ID, error) {
	return s[email], nil
}

type registerState struct {
	workspaces, operators int64
	err                   error
}

func (s registerState) State(context.Context) (int64, int64, error) {
	return s.workspaces, s.operators, s.err
}

func environmentFixture(entries ...env.OperatorEntry) (OperatorsFromEnvironment, *registerStore, *journalStore) {
	register := newRegister()
	register.addresses = map[string]shared.ID{"acme\x00ada@acme.example": adaAccount}
	journal := &journalStore{}
	return OperatorsFromEnvironment{
		Entries: entries, Operators: register, Journal: journal,
		Single:     singleResolver{"eva@family.example": evaAccount},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(fixed), IDs: &ids{},
	}, register, journal
}

// UC-INS-02 check 4: an entry whose account exists is put into the register and recorded; one whose
// account does not exist yet waits, and lands on a later pass once it does.
func TestTheEnvironmentPutsItsOperatorsIntoTheRegister(t *testing.T) {
	ada := env.OperatorEntry{Workspace: "acme", Email: "ada@acme.example"}
	bob := env.OperatorEntry{Workspace: "acme", Email: "bob@acme.example"}
	eva := env.OperatorEntry{Email: "eva@family.example"}
	applying, register, journal := environmentFixture(ada, bob, eva)

	waiting, err := applying.Apply(t.Context())
	if err != nil {
		t.Fatalf("applying: %v", err)
	}
	if !register.accounts[adaAccount] || !register.accounts[evaAccount] {
		t.Errorf("register = %v, want Ada and Eva", register.accounts)
	}
	if len(waiting) != 1 || waiting[0] != bob {
		t.Errorf("waiting = %v, want Bob", waiting)
	}
	if len(journal.entries) != 2 || journal.entries[0].Action != "instance.operator_added" ||
		journal.entries[0].ActorLabel != "HUBTASK_OPERATORS" {
		t.Errorf("journal = %+v", journal.entries)
	}

	// A second pass changes nothing that is already there, and records nothing twice.
	if _, err := applying.Apply(t.Context()); err != nil {
		t.Fatalf("applying again: %v", err)
	}
	if len(journal.entries) != 2 {
		t.Errorf("a second pass recorded %d entries", len(journal.entries)-2)
	}

	// Bob's account comes to exist: the next pass adds him.
	register.addresses["acme\x00bob@acme.example"] = secondOperator
	if waiting, _ := applying.Apply(t.Context()); len(waiting) != 0 || !register.accounts[secondOperator] {
		t.Errorf("Bob was not added once his account existed: waiting %v", waiting)
	}
}

// An entry whose account exists and is not active - invited, disabled - waits like one whose
// account does not exist yet, and lands once the account is active (UC-INS-05 check 4).
func TestTheEnvironmentWaitsForAnAccountThatIsNotActive(t *testing.T) {
	ada := env.OperatorEntry{Workspace: "acme", Email: "ada@acme.example"}
	applying, register, journal := environmentFixture(ada)
	register.inactive[adaAccount] = true

	waiting, err := applying.Apply(t.Context())
	if err != nil {
		t.Fatalf("applying: %v", err)
	}
	if register.accounts[adaAccount] || len(journal.entries) != 0 || len(waiting) != 1 {
		t.Fatalf("an invited account was added (waiting %v, journal %d)", waiting, len(journal.entries))
	}

	delete(register.inactive, adaAccount)
	if waiting, _ := applying.Apply(t.Context()); len(waiting) != 0 || !register.accounts[adaAccount] {
		t.Errorf("the account was not added once active: waiting %v", waiting)
	}
}

// UC-INS-02 check 5: the environment adds, it does not own the register. An operator the variable
// no longer names stays.
func TestTheEnvironmentNeverRemovesAnOperator(t *testing.T) {
	applying, register, _ := environmentFixture()
	register.accounts[secondOperator] = true
	if _, err := applying.Apply(t.Context()); err != nil {
		t.Fatalf("applying an empty list: %v", err)
	}
	if !register.accounts[secondOperator] {
		t.Error("an operator the variable does not name was removed")
	}
}

// UC-INS-03 check 4 and UC-INS-02 check 4, in the health report.
func TestTheHealthReportNamesAnEmptyRegisterAndTheWaitingEntries(t *testing.T) {
	bob := env.OperatorEntry{Workspace: "acme", Email: "bob@acme.example"}
	applying, _, _ := environmentFixture(bob)

	codes := func(state registerState) map[string]map[string]string {
		got := map[string]map[string]string{}
		for _, warning := range (RegisterWarnings{State: state, Environment: applying, UnitOfWork: &unitOfWork{}}).Warnings(t.Context()) {
			got[warning.Code] = warning.Params
		}
		return got
	}

	several := codes(registerState{workspaces: 3, operators: 0})
	if several["config.operator_register_empty"]["command"] != RestoreCommand {
		t.Errorf("several workspaces and an empty register: %v", several)
	}
	if several["config.operator_pending"]["entry"] != "acme:bob@acme.example" {
		t.Errorf("the waiting entry is not named: %v", several)
	}

	// One workspace with an empty register is the private installation, whose owner is the operator.
	if _, named := codes(registerState{workspaces: 1})["config.operator_register_empty"]; named {
		t.Error("a single workspace's empty register was reported")
	}
	if _, named := codes(registerState{workspaces: 3, operators: 1})["config.operator_register_empty"]; named {
		t.Error("a filled register was reported empty")
	}
	// A register that cannot be read is said to be unchecked, never left out (P-11).
	if _, named := codes(registerState{err: errors.New("down")})["config.operator_register_unchecked"]; !named {
		t.Error("an unreadable register reads as fine")
	}
}
