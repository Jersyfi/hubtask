// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

func settled(t *testing.T) domain.Account {
	t.Helper()
	account, err := domain.Invite(adminID, tenant, "anna@example.org", "Anna")
	if err != nil {
		t.Fatalf("preparing the account: %v", err)
	}
	account, err = account.WithPreferences(domain.Preferences{Locale: "de", TimeZone: "Europe/Berlin"})
	if err != nil {
		t.Fatalf("preparing the preferences: %v", err)
	}
	return account
}

func preferencesHandler(accounts *accountStore, auth *authorizer, sink *auditSink) UpdateAccountPreferences {
	return UpdateAccountPreferences{
		Accounts: accounts, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

// The distinction the whole command exists for: absent leaves, empty clears. A client that changes
// its locale must not have to send its time zone back, and one that forgets must not reset it.
func TestAnAbsentPreferenceIsLeftAndAnEmptyOneIsCleared(t *testing.T) {
	empty := ""
	dutch := "nl"

	cases := []struct {
		name         string
		command      UpdateAccountPreferencesCommand
		wantLocale   string
		wantTimeZone string
	}{
		{
			name:         "changing one leaves the other",
			command:      UpdateAccountPreferencesCommand{Locale: &dutch},
			wantLocale:   "nl",
			wantTimeZone: "Europe/Berlin",
		},
		{
			name:         "an empty value clears that one only",
			command:      UpdateAccountPreferencesCommand{TimeZone: &empty},
			wantLocale:   "de",
			wantTimeZone: "",
		},
		{
			name:         "sending nothing changes nothing",
			command:      UpdateAccountPreferencesCommand{},
			wantLocale:   "de",
			wantTimeZone: "Europe/Berlin",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			accounts := newAccounts(settled(t))

			updated, err := preferencesHandler(accounts, &authorizer{}, &auditSink{}).
				Execute(t.Context(), admin(), c.command)
			if err != nil {
				t.Fatalf("updating: %v", err)
			}

			if updated.Locale != c.wantLocale {
				t.Errorf("locale %q, want %q", updated.Locale, c.wantLocale)
			}
			if updated.TimeZone != c.wantTimeZone {
				t.Errorf("time zone %q, want %q", updated.TimeZone, c.wantTimeZone)
			}
		})
	}
}

// Changing one's own preferences is not administering anybody. Requiring the member permission for
// it would mean a viewer could not pick their own time zone.
func TestChangingOnesOwnPreferencesNeedsNoPermission(t *testing.T) {
	auth := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
	dutch := "nl"

	_, err := preferencesHandler(newAccounts(settled(t)), auth, &auditSink{}).
		Execute(t.Context(), admin(), UpdateAccountPreferencesCommand{Locale: &dutch})
	if err != nil {
		t.Fatalf("the caller was refused their own preferences: %v", err)
	}
	if len(auth.requests) != 0 {
		t.Error("a permission was asked for somebody changing their own settings")
	}
}

// Changing somebody else's is administering them, and needs the permission that says so.
func TestChangingSomebodyElsesNeedsThePermission(t *testing.T) {
	other, err := domain.Invite(invitedID, tenant, "bert@example.org", "Bert")
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}
	accounts := newAccounts(settled(t), other)
	auth := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
	dutch := "nl"

	_, err = preferencesHandler(accounts, auth, &auditSink{}).Execute(t.Context(), admin(),
		UpdateAccountPreferencesCommand{AccountID: invitedID, Locale: &dutch})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want the refusal", err)
	}
	if len(accounts.preference) != 0 {
		t.Error("a refused change was written")
	}
	if len(auth.requests) != 1 || auth.requests[0].TargetID != invitedID {
		t.Errorf("asked for %v, want the permission over the other account", auth.requests)
	}
}

// A value the domain refuses never reaches the database, and the trail records the change openly:
// a locale is a choice from a closed set, and an auditor has to see who changed whose settings.
func TestAnInvalidPreferenceIsRefusedAndAValidOneIsRecordedOpenly(t *testing.T) {
	accounts, sink := newAccounts(settled(t)), &auditSink{}
	atlantis := "Europe/Atlantis"

	_, err := preferencesHandler(accounts, &authorizer{}, sink).
		Execute(t.Context(), admin(), UpdateAccountPreferencesCommand{TimeZone: &atlantis})
	if err == nil || shared.AsError(err).DetailCode != "accounts.time_zone_invalid" {
		t.Fatalf("error %v, want the zone refused", err)
	}
	if len(accounts.preference) != 0 {
		t.Fatal("the refused zone was written")
	}

	dutch := "nl"
	if _, err := preferencesHandler(accounts, &authorizer{}, sink).
		Execute(t.Context(), admin(), UpdateAccountPreferencesCommand{Locale: &dutch}); err != nil {
		t.Fatalf("updating: %v", err)
	}

	if len(sink.entries) != 1 {
		t.Fatalf("%d audit entries, want one for the change that happened", len(sink.entries))
	}
	locale, recorded := sink.entries[0].Changes["locale"].(map[string]any)
	if !recorded || locale["to"] != "nl" || locale["from"] != "de" {
		t.Errorf("the trail records %v, want the locale openly with both sides", sink.entries[0].Changes)
	}
}

// The catalogue's untyped input has to preserve the same distinction, or the REST and MCP channels
// silently lose it.
func TestTheChannelInputKeepsAbsentAndEmptyApart(t *testing.T) {
	accounts := newAccounts(settled(t))
	handler := preferencesHandler(accounts, &authorizer{}, &auditSink{})

	if _, err := handler.invoke(t.Context(), admin(), usecase.Input{"time_zone": ""}); err != nil {
		t.Fatalf("clearing through the channel: %v", err)
	}
	if account := accounts.byID[adminID]; account.TimeZone != "" || account.Locale != "de" {
		t.Errorf("after clearing: locale %q, zone %q - want only the zone cleared",
			account.Locale, account.TimeZone)
	}

	if _, err := handler.invoke(t.Context(), admin(), usecase.Input{}); err != nil {
		t.Fatalf("sending nothing through the channel: %v", err)
	}
	if account := accounts.byID[adminID]; account.Locale != "de" {
		t.Errorf("an empty input changed the locale to %q", account.Locale)
	}
}
