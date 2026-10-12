// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-ID-20 check 4 the way the card asks it: nobody signed in, the workspace named only by the
// host's subdomain, the real resolver, the real directory and the workspace's own rows under row
// level security. The answer turns once a managed account exists - read again, with nothing kept
// from the first read - and stays no for another workspace at the same moment.
func TestTheSignedOutCardLearnsOfAManagedAccountAtItsHost(t *testing.T) {
	ctx := context.Background()
	_, directory, uow := realWriter(ctx, t)
	tenant, _, _ := restrictedTenant(ctx, t)
	other, _, _ := restrictedTenant(ctx, t)

	card := identityservice.GetSignInRules{
		Resolver: identityservice.SignInPolicyResolver{
			Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Instance:   postgres.NewInstanceSettingRepository(), UnitOfWork: uow,
		},
		Tenants: directory, Providers: postgres.NewIdentityProviderRepository(),
		Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Managed:    postgres.NewAccountRepository(),
		Clock:      clockadapter.System{}, UnitOfWork: uow, Multi: true,
	}
	namesAt := func(slug string) bool {
		t.Helper()
		rules, err := card.Execute(ctx, identityservice.GetSignInRulesCommand{
			Host: slug + ".example.org:8443", TenantSlug: slug,
		})
		if err != nil {
			t.Fatalf("reading the card at %s: %v", slug, err)
		}
		return rules.SignInNames
	}
	slug := "restricted-" + tenant.String()[24:]
	otherSlug := "restricted-" + other.String()[24:]

	if namesAt(slug) {
		t.Fatal("a workspace without a managed account asked for a sign-in name")
	}
	managedIn(t, tenant, freshSignInName(t))
	if !namesAt(slug) {
		t.Error("the card at the workspace's host still asks for an address after a managed account was made")
	}
	if namesAt(otherSlug) {
		t.Error("another workspace's card asked for a sign-in name")
	}
}
