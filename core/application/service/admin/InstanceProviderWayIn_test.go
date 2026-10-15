// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"testing"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domainidentity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0078 §6, UC-ID-04 check 9: removing the installation's provider disconnects people in every
// workspace that used it, and each is told - read in each workspace's own scope.

// holdersInTurn answers, call by call, who holds an identity at the provider: one answer per
// workspace, in the order the workspaces are listed.
type holdersInTurn struct {
	identityrepo.ExternalAccounts
	answers [][]shared.ID
}

func (h *holdersInTurn) HoldersOf(context.Context, shared.ID) ([]shared.ID, error) {
	if len(h.answers) == 0 {
		return nil, nil
	}
	next := h.answers[0]
	h.answers = h.answers[1:]
	return next, nil
}

type toldNotices struct {
	notices []struct {
		tenant, account shared.ID
		change          domainidentity.WayInChange
	}
}

func (t *toldNotices) WayInChanged(
	_ context.Context, tenantID, accountID shared.ID, change domainidentity.WayInChange,
) error {
	t.notices = append(t.notices, struct {
		tenant, account shared.ID
		change          domainidentity.WayInChange
	}{tenantID, accountID, change})
	return nil
}

func TestRemovingTheInstallationsProviderTellsItsPeopleInEveryWorkspace(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	stored, err := (ConfigureInstanceIdentityProvider{Writer: writer}).Execute(t.Context(), operator(),
		identityservice.ConfigureIdentityProviderCommand{
			Issuer: "https://accounts.google.com", ClientID: "hubtask", Kind: "GOOGLE",
			ClientSecret: secret.New("s3cr3t"),
		})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	first := shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	second := shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
	anna := shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
	bert := shared.ID("01936f2a-7c1e-7000-8000-0000000000e2")
	told := &toldNotices{}
	writer.Tenants = &tenantsStore{listed: []adminrepo.TenantRecord{{ID: first}, {ID: second}}}
	writer.Configure.WayIn = told
	writer.Configure.External = &holdersInTurn{answers: [][]shared.ID{{anna}, {bert}}}

	if err := (RemoveInstanceIdentityProvider{Writer: writer}).
		Execute(t.Context(), operator(), stored.ID, ""); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("the provider is still there: %v", store.rows)
	}
	if len(told.notices) != 2 {
		t.Fatalf("%d notices, want one per connected person", len(told.notices))
	}
	for i, want := range []struct{ tenant, account shared.ID }{{first, anna}, {second, bert}} {
		got := told.notices[i]
		if got.tenant != want.tenant || got.account != want.account ||
			got.change.Event != domainidentity.WayInProviderDisconnected || got.change.ProviderID != stored.ID {
			t.Errorf("notice %d is %+v, want %v", i, got, want)
		}
	}
	// Each workspace was read in its own scope.
	work, _ := writer.Instance.UnitOfWork.(*unitOfWork)
	scoped := map[shared.ID]bool{}
	for _, scope := range work.scopes {
		scoped[scope.TenantID] = true
	}
	if !scoped[first] || !scoped[second] {
		t.Errorf("the workspaces were read under %v, want each in its own", work.scopes)
	}
}
