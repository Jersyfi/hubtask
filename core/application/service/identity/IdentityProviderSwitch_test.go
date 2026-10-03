// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-11 check 8 and UC-ID-12 check 6 (SC-21, ADR-0076 §5): a provider is switched on or off in
// the list of ways to sign in, and nowhere else. Its own form - the collection's `PUT`, its `POST`
// and the singular route - refuses a changed `enabled` with a sentence pointing to the list, accepts
// the same value so a client that echoes the field keeps working, and creates a provider off.

func refusedAsSwitchInList(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was accepted", what)
	}
	if detail := shared.AsError(err).DetailCode; detail != "identity_provider.switch_in_list" {
		t.Errorf("%s was refused with %q, want identity_provider.switch_in_list", what, detail)
	}
}

func TestAChangedEnabledIsRefusedOnTheFormAndAnEchoedOneAccepted(t *testing.T) {
	for _, on := range []bool{true, false} {
		f := newOfferFixture(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
		id := f.own(t, on)
		f.installation(t) // another way in, so the last-way-in guard is not what is measured
		f.writer.Workspaces = f.workspaces

		command := configureCommand()
		command.ID = id
		command.DisplayName = "Renamed"

		command.Enabled = boolOf(!on)
		_, err := ConfigureIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), command)
		refusedAsSwitchInList(t, err, "switching a provider on its form")
		if row := f.store.rows[0]; row.Enabled != on || row.DisplayName == "Renamed" {
			t.Errorf("the refused form was written: %+v", row)
		}

		// The same value, and none at all, are ordinary saves that leave the switch where it is.
		for _, echoed := range []*bool{boolOf(on), nil} {
			command.Enabled = echoed
			saved, err := ConfigureIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), command)
			if err != nil {
				t.Fatalf("saving the form with enabled %v on a provider that is %v: %v", echoed, on, err)
			}
			if saved.Enabled != on || saved.DisplayName != "Renamed" {
				t.Errorf("the save answered enabled %v and %q, want %v and the new name",
					saved.Enabled, saved.DisplayName, on)
			}
		}
	}
}

// A save does not write the switch at all, so one made in the list while the form was open stands -
// the race a read-then-write would lose.
func TestASaveKeepsASwitchMadeWhileTheFormWasOpen(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	id := f.own(t, true)
	f.installation(t)
	f.writer.Workspaces = f.workspaces

	command := configureCommand()
	command.ID = id
	command.Enabled = boolOf(true) // echoed as the form read it
	f.store.afterFind = func() {
		f.store.afterFind = nil
		f.store.rows[0].Enabled = false // somebody switched it off in the list a moment later
	}
	saved, err := ConfigureIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), command)
	if err != nil {
		t.Fatalf("saving: %v", err)
	}
	if saved.Enabled || f.store.rows[0].Enabled {
		t.Error("the save wrote back the switch the list had just turned off")
	}
}

func TestANewProviderIsCreatedSwitchedOff(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))

	command := configureCommand()
	command.Enabled = boolOf(true)
	_, err := ConfigureIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), command)
	refusedAsSwitchInList(t, err, "creating a provider switched on")
	if len(f.store.rows) != 0 || len(f.relying.checked) != 0 {
		t.Errorf("the refused creation wrote %d rows and asked the issuer %d times",
			len(f.store.rows), len(f.relying.checked))
	}

	for _, given := range []*bool{nil, boolOf(false)} {
		f := newProviderFixture(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
		command.Enabled = given
		created, err := ConfigureIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), command)
		if err != nil {
			t.Fatalf("creating with enabled %v: %v", given, err)
		}
		if created.Enabled || f.store.only(t).Enabled {
			t.Errorf("a provider created with enabled %v is on - configuring is not offering", given)
		}
	}
}

// The singular route is the same form behind another door, and it holds the same rule.
func TestTheSingularRouteSwitchesNothingEither(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	first := ConfigureFirstIdentityProvider{Writer: f.writer}

	command := configureCommand()
	command.Enabled = boolOf(true)
	_, err := first.Execute(t.Context(), providerActor(), command)
	refusedAsSwitchInList(t, err, "creating the first provider switched on")

	command.Enabled = nil
	created, err := first.Execute(t.Context(), providerActor(), command)
	if err != nil || created.Enabled {
		t.Fatalf("creating the first provider answered (%+v, %v), want it created off", created, err)
	}

	command.Enabled = boolOf(true)
	_, err = first.Execute(t.Context(), providerActor(), command)
	refusedAsSwitchInList(t, err, "switching the first provider on through the singular route")
	if f.store.only(t).Enabled {
		t.Error("the singular route switched the provider on")
	}
}

// Through the registry, which is what REST, MCP and automation reach: the field is declared and
// read, and the refusal is the use case's rather than a validation error about an unknown key.
func TestTheRegistryCarriesTheRefusal(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	registry, err := usecase.NewRegistry(nil, ConfigureIdentityProvider{Writer: f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	input := usecase.Input{
		"issuer": "https://login.example.org", "client_id": "hubtask", "client_secret": "s3cr3t",
		"allowed_email_domains": []any{"example.org"},
	}
	added, err := registry.Invoke(t.Context(), ConfigureIdentityProviderName, providerActor(), input)
	if err != nil {
		t.Fatalf("adding: %v", err)
	}
	if added["enabled"] != false {
		t.Errorf("the provider was added with enabled %v", added["enabled"])
	}

	input["id"] = added["id"]
	input["enabled"] = true
	_, err = registry.Invoke(t.Context(), ConfigureIdentityProviderName, providerActor(), input)
	refusedAsSwitchInList(t, err, "switching through the registry")
}
