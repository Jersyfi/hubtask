// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// Person-level settings on the catalogue (F-5, model.md §4): a person's language, zone and week
// start are answered as the catalogue resolves them, each with the level it comes from - their
// own, the workspace's, the installation's or Hubtask's - so the preferences screen names it.
func TestThePersonReadsTheirSettingsWithWhereEachComesFrom(t *testing.T) {
	account := settled(t) // de, Europe/Berlin, no week start of their own
	workspace := &workspaceStore{row: domain.Workspace{Tenant: domain.Tenant{
		ID: tenant, Slug: "acme", DisplayName: "Acme", Status: domain.TenantActive,
		DefaultLocale: "en", DefaultTimeZone: "UTC",
	}}}
	instance := &instanceSettings{level: repository.InstanceLevel{
		Localisation: domain.LocalisationDefaults{WeekStart: domain.WeekStartSunday},
	}}
	handler := ReadOwnSettings{
		Accounts: newAccounts(account), UnitOfWork: &unitOfWork{},
		Resolver: SignInPolicyResolver{Workspaces: workspace, Instance: instance, UnitOfWork: &unitOfWork{}},
	}

	out, err := handler.Descriptor().Handler.Invoke(t.Context(), self(account.ID), usecase.Input{})
	if err != nil {
		t.Fatalf("reading the own settings: %v", err)
	}
	got := map[string]string{}
	for _, row := range out["data"].([]usecase.Output) {
		got[row["setting"].(string)] = row["source"].(string) + " " + toText(row["value"])
	}
	want := map[string]string{
		"localisation.locale": "PERSON de", "localisation.time_zone": "PERSON Europe/Berlin",
		"localisation.week_start": "INSTANCE SUNDAY",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
	if len(got) != len(want) {
		t.Errorf("answered %v, want only the person's own settings", got)
	}
}

// Nobody reads another's settings here: the system itself has no person to answer for.
func TestTheOwnSettingsNeedAPerson(t *testing.T) {
	handler := ReadOwnSettings{Accounts: newAccounts(), UnitOfWork: &unitOfWork{}}
	actor := self("")
	if _, err := handler.Descriptor().Handler.Invoke(t.Context(), actor, usecase.Input{}); err == nil {
		t.Fatal("a caller without an account read settings")
	}
}

func toText(v any) string {
	s, _ := v.(string)
	return s
}
