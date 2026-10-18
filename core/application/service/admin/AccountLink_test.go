// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

const providerAccount = "https://provider.example/account"

func accountEntry(t *testing.T, out usecase.Output) usecase.Output {
	t.Helper()
	legal, _ := out["legal"].(usecase.Output)
	entry, held := legal[string(identity.LinkAccount)].(usecase.Output)
	if !held {
		t.Fatalf("the legal area has no account link: %v", legal)
	}
	return entry
}

// The installation's document carries the account link as a link; in multi mode its lock says what
// plans may do, and in single mode - no plans - there is no lock to describe. A plan's own document
// describes none either: nothing below a plan sets it.
func TestTheAccountLinkIsDrawnAtEachLevel(t *testing.T) {
	level := identityrepo.InstanceLevel{Legal: identity.LegalLayer{
		Links: identity.LegalLinks{}.With(identity.LinkAccount, providerAccount),
		Locks: map[identity.LegalLink]bool{identity.LinkAccount: true},
	}}

	multi := accountEntry(t, levelOutput(level, true))
	if multi["kind"] != string(identity.KindLink) || multi["value"] != providerAccount || multi["locked"] != true ||
		multi["workspace_may"] != string(identity.MayPlanReplace) {
		t.Errorf("multi mode answers %v", multi)
	}
	single := accountEntry(t, levelOutput(level, false))
	if _, described := single["workspace_may"]; described {
		t.Errorf("single mode describes a lock that binds no plan: %v", single)
	}

	onPlan := accountEntry(t, planOutput(PlanView{Plan: plan.Plan{Name: "family", Values: plan.Values{
		Legal: identity.LegalLayer{Links: identity.LegalLinks{}.With(identity.LinkAccount, "https://shop.example/family")},
	}}}))
	if _, described := onPlan["workspace_may"]; described || onPlan["locked"] != false || onPlan["value"] != "https://shop.example/family" {
		t.Errorf("a plan's document answers %v", onPlan)
	}
}

// One reading for every door: the address is read, checked as the legal links are, and locked.
func TestTheAccountLinkIsReadFromTheDocument(t *testing.T) {
	read, err := instanceLevelFrom(usecase.Input{"legal": map[string]any{
		"account_url": map[string]any{"value": providerAccount, "locked": true},
	}})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if read.Legal.Links.AccountURL != providerAccount || !read.Legal.Locks[identity.LinkAccount] {
		t.Errorf("read %+v", read.Legal)
	}

	for raw, code := range map[string]string{
		"ftp://provider.example/account": "auth.legal_url_scheme",
		"not an address":                 "auth.legal_url_invalid",
	} {
		_, err := instanceLevelFrom(usecase.Input{"legal": map[string]any{"account_url": map[string]any{"value": raw}}})
		if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != code {
			t.Errorf("%q answered %v, want %s", raw, err, code)
		}
	}
}

// The journal names the moved key and never the address (rule 10).
func TestMovingTheAccountLinkIsNamedWithoutItsValue(t *testing.T) {
	after := identityrepo.InstanceLevel{Legal: identity.LegalLayer{Links: identity.LegalLinks{}.With(identity.LinkAccount, providerAccount)}}
	moved := movedSwitches(identityrepo.InstanceLevel{}, after)
	if !slices.Contains(moved, any(string(identity.LinkAccount))) {
		t.Errorf("the journal does not name the account link: %v", moved)
	}
	for _, name := range moved {
		if name == providerAccount {
			t.Errorf("the journal carries the address: %v", moved)
		}
	}
}

// In single mode there are no plans for the account link's lock to bind: a lock sent with it is not
// stored, so that it cannot bind plans unseen the day the installation turns multi.
func TestASingleInstallationStoresNoAccountLinkLock(t *testing.T) {
	document := usecase.Input{"legal": map[string]any{
		"account_url": map[string]any{"value": providerAccount, "locked": true},
	}}
	for mode, wantLocked := range map[env.TenancyMode]bool{env.TenancySingle: false, env.TenancyMulti: true} {
		writer, store, _ := newInstanceWriter(newRegister(operatorID))
		writer.Tenancy = mode
		registry, err := usecase.NewRegistry(nil, WriteInstanceSettings{Writer: writer}.Descriptor())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), document); err != nil {
			t.Fatalf("%s: writing: %v", mode, err)
		}
		if store.level.Legal.Links.AccountURL != providerAccount || store.level.Legal.Locks[identity.LinkAccount] != wantLocked {
			t.Errorf("%s: stored %+v", mode, store.level.Legal)
		}
	}
}
