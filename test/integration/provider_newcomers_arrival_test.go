// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The admission ladder, end to end against PostgreSQL as the application role (SC-02's acceptance:
// a test per E1 mode proving who is admitted, who gets an account and what access it has). The
// provider is the one thing the database cannot stand in for, so the relying party is a stub that
// answers the identity a provider would have verified; everything else is the real service over the
// real stores, in the arrival's own transaction.
func TestEachModeAdmitsCreatesAndGivesWhatItSays(t *testing.T) {
	ctx := context.Background()
	seedNewcomerTenants(ctx, t)
	admin := adminPool(ctx, t)
	writer, _, uow := realWriter(ctx, t)
	writer.Encryptor = ring(t, crypto.KeyMaterial{
		ID: "k132newcomers", Material: secret.New("sc02-newcomers-key-not-a-real-secret"),
	})
	providers := postgres.NewIdentityProviderRepository()
	cursors := security.NewCursorCodec(secret.New(installationSecret))
	run := strconv.FormatInt(time.Now().UnixNano(), 36)

	const (
		refused  = "refused, no account"
		nothing  = "an account with no access"
		asMember = "an account holding MEMBER on the workspace"
		inGroup  = "an account in the group"
	)
	cases := []struct {
		name    string
		mode    domain.Provisioning
		role    string
		group   shared.ID
		address string
		want    string
	}{
		{"INVITED_ONLY, somebody new", domain.ProvisionInvitedOnly, "", "", "listed", refused},
		{"DOMAINS, an address outside the list", domain.ProvisionDomains, "MEMBER", "", "elsewhere", refused},
		{"DOMAINS, inside the list, nothing named", domain.ProvisionDomains, "", "", "listed", nothing},
		{"DOMAINS, inside the list, a role", domain.ProvisionDomains, "MEMBER", "", "listed", asMember},
		{"DOMAINS, inside the list, a group", domain.ProvisionDomains, "", ncGroupA, "listed", inGroup},
		{"ANY, nothing named", domain.ProvisionAny, "", "", "elsewhere", nothing},
		{"ANY, a role", domain.ProvisionAny, "MEMBER", "", "elsewhere", asMember},
		{"ANY, a group", domain.ProvisionAny, "", ncGroupA, "elsewhere", inGroup},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			providerID := shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-000000132b%02x", i))
			if _, err := admin.Exec(ctx, `DELETE FROM identity_provider WHERE id = $1`, providerID.String()); err != nil {
				t.Fatalf("clearing an earlier run: %v", err)
			}
			configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
				ID: providerID, TenantID: ncTenantA, Issuer: fmt.Sprintf("https://mode-%d.newcomers.example", i),
				ClientID: "hubtask", Provisioning: string(tc.mode), Enabled: true, Now: time.Now(),
				AllowedEmailDomains: []string{"listed.newcomers.example"},
				NewcomerRole:        tc.role, NewcomerGroupID: tc.group,
			})
			if err != nil {
				t.Fatalf("configuring: %v", err)
			}
			if err := uow.Within(ctx, persistence.Scope{TenantID: ncTenantA}, func(ctx context.Context) error {
				sealed, err := writer.Encryptor.Seal(ctx, secret.New("s3cr3t"),
					identityservice.ClientSecretPurpose(ncTenantA))
				if err != nil {
					return err
				}
				_, err = providers.Insert(ctx, configured, sealed)
				return err
			}); err != nil {
				t.Fatalf("storing the provider: %v", err)
			}

			email := fmt.Sprintf("newcomer-%d-%s@%s.newcomers.example", i, run, tc.address)
			oidc := identityservice.OidcWriter{
				Session:   writer,
				Providers: providers,
				Flows:     postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret))),
				External:  postgres.NewExternalAccountRepository(),
				Accounts:  postgres.NewAccountRepository(),
				Grants:    postgres.NewMembershipGrantRepository(cursors),
				Groups:    postgres.NewGroupRepository(cursors),
				Relying: stubRelying{identity: identityprovider.Identity{
					Subject: "newcomer-" + strconv.Itoa(i) + "-" + run, Email: email,
					EmailVerified: true, AddressAuthoritative: true, DisplayName: "Newcomer",
				}},
				RedirectURL: "https://hubtask.example/auth/callback",
			}
			started, err := identityservice.StartOidcSignIn{Writer: oidc}.Execute(ctx,
				identityservice.StartOidcSignInCommand{ProviderID: providerID, TenantHeader: ncTenantA.String()})
			if err != nil {
				t.Fatalf("starting: %v", err)
			}
			result, err := identityservice.CompleteOidcSignIn{Writer: oidc}.Execute(ctx,
				identityservice.CompleteOidcSignInCommand{Code: "the-code", State: started.State})

			var accounts, tenantRoles, groupLinks int
			var role string
			if err := admin.QueryRow(ctx, `
				SELECT count(*),
				  coalesce(sum((SELECT count(*) FROM membership m
				                WHERE m.account_id = a.id AND m.scope_type = 'TENANT')), 0),
				  coalesce(max((SELECT m.role::text FROM membership m
				                WHERE m.account_id = a.id AND m.scope_type = 'TENANT' LIMIT 1)), ''),
				  coalesce(sum((SELECT count(*) FROM account_group_member g
				                WHERE g.account_id = a.id AND g.group_id = $3)), 0)
				FROM account a WHERE a.tenant_id = $1 AND a.email = $2`,
				ncTenantA.String(), email, ncGroupA.String()).Scan(&accounts, &tenantRoles, &role, &groupLinks); err != nil {
				t.Fatalf("reading what the arrival left: %v", err)
			}

			switch tc.want {
			case refused:
				if err == nil || accounts != 0 {
					t.Fatalf("answered %v and left %d accounts, want a refusal and none", err, accounts)
				}
				return
			default:
				if err != nil || result.Pair == nil {
					t.Fatalf("the arrival answered (%+v, %v), want a session", result, err)
				}
			}
			if accounts != 1 {
				t.Fatalf("%d accounts for the newcomer, want 1", accounts)
			}
			switch tc.want {
			case nothing:
				if tenantRoles != 0 || groupLinks != 0 {
					t.Errorf("a newcomer with nothing named holds %d roles and %d groups", tenantRoles, groupLinks)
				}
			case asMember:
				if tenantRoles != 1 || role != "MEMBER" || groupLinks != 0 {
					t.Errorf("holds %d workspace roles (%q) and %d groups, want MEMBER only", tenantRoles, role, groupLinks)
				}
			case inGroup:
				if tenantRoles != 0 || groupLinks != 1 {
					t.Errorf("holds %d workspace roles and %d groups, want the group only", tenantRoles, groupLinks)
				}
			}
		})
	}
}
