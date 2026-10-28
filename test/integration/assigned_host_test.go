// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// assignedBase is the installation the workspaces of this file are provisioned on: its own host is
// hub.example, and it assigns each workspace a host of the operator's choosing.
const assignedBase = "https://hub.example:8443"

// assignedProvisioning wires provisioning the way the composition root does, on an installation
// that assigns hosts.
func assignedProvisioning(t *testing.T, uow persistence.UnitOfWork, signIn postgres.SignInRepository) adminservice.ProvisionTenant {
	t.Helper()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(clockadapter.System{}, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	return adminservice.ProvisionTenant{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Accounts: postgres.NewAccountRepository(), Redemption: signIn,
		Grants: postgres.NewMembershipGrantRepository(pageCursors()), Containers: containerRepo(),
		Buckets: postgres.NewBucketRepository(), Labels: postgres.NewLabelRepository(),
		Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(),
		Audit: postgres.NewAuditSink(ids), Renderer: renderer, Text: text.Composing{},
		Domains: textadapter.Domains{}, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids,
		HLC: hybrid, Entropy: clockadapter.CryptoRandom{},
		Tenancy: env.TenancyMulti, WorkspaceHosts: env.HostsAssigned,
		Hosts: postgres.NewTenantHostRepository(), InstallationHost: "hub.example",
	}
}

func assignedOperator() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: shared.ActorUser, TenantID: hostTenantB,
		AccountID:   shared.MustParseID("01936f2a-7c1e-7000-8000-0000000cd401"),
		AccountName: "Operator", Scopes: []string{"admin:tenants"},
	}
}

// UC-INS-20 checks 1, 2, 3 and 5 against the real stores: provisioning on an installation that
// assigns hosts writes the host as the workspace's main row, which the request's host then names;
// a host another workspace answers at is refused at the field by the table's own uniqueness. The
// workspace there signs in, shows that host on its sign-in card, hands out an invitation whose link
// names it, and a provider's return lands on it.
func TestAWorkspaceOnAnAssignedHostAnswersThere(t *testing.T) {
	ctx := context.Background()
	seedHostTenants(ctx, t)
	passwords, directory := passwordWriterOver(ctx, t)
	writer := passwords.Session
	writer.InstallationHost = "hub.example"
	uow := writer.UnitOfWork
	_, signIn, _ := realWriter(ctx, t)
	provisioning := assignedProvisioning(t, uow, signIn)
	registry, err := usecase.NewRegistry(nil, provisioning.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	suffix := strings.ReplaceAll(freshID(t).String()[24:], "-", "")
	host := "tasks-hr-" + suffix + ".corp.example"
	out, err := registry.Invoke(ctx, adminservice.ProvisionTenantName, assignedOperator(), usecase.Input{
		"host": strings.ToUpper(host), "display_name": "Human Resources",
		"owner_email": "jana.koch@corp.example",
	})
	if err != nil {
		t.Fatalf("provisioning on an assigned host: %v", err)
	}
	tenant := shared.MustParseID(out["id"].(string))
	owner := shared.MustParseID(out["owner_account_id"].(string))
	if out["host"] != host || out["slug"] != "tasks-hr-"+suffix {
		t.Errorf("answered slug %v at %v, want the short name made from %s", out["slug"], out["host"], host)
	}

	t.Run("the host names the workspace", func(t *testing.T) {
		var named shared.ID
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			var err error
			named, err = directory.ResolveHost(ctx, domain.RequestHost(host+":8443"))
			return err
		}); err != nil || named != tenant {
			t.Errorf("the assigned host named (%q, %v), want the new workspace", named, err)
		}
	})

	// The control plane's reads carry the main address: the list *New workspace* checks a typed host
	// against, and the one workspace read inside its own scope - which sees its own row only.
	t.Run("the control plane reads the host", func(t *testing.T) {
		tenants := postgres.NewAdminTenantRepository()
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			listed, err := tenants.List(ctx)
			if err != nil {
				return err
			}
			for _, record := range listed {
				if record.ID == tenant && record.Host != host {
					t.Errorf("the list names %q, want %s", record.Host, host)
				}
				if record.ID != tenant && record.Host == host {
					t.Errorf("the list names %s for another workspace", host)
				}
			}
			found, _, err := postgres.NewTenantQueryRepository(pageCursors()).Query(ctx,
				adminrepo.TenantQuery{Slug: "tasks-hr-" + suffix, Size: 10})
			if err != nil {
				return err
			}
			if len(found) != 1 || found[0].Host != host {
				t.Errorf("the query found %+v, want the workspace at %s", found, host)
			}
			return nil
		}); err != nil {
			t.Fatalf("listing: %v", err)
		}
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
			record, err := tenants.Find(ctx)
			if err != nil || record.Host != host {
				t.Errorf("the workspace's own read answered (%q, %v), want %s", record.Host, err, host)
			}
			return nil
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: hostTenantB}, func(ctx context.Context) error {
			record, err := tenants.Find(ctx)
			if err != nil || record.Host != hostOfB {
				t.Errorf("another workspace's read answered (%q, %v), want its own %s", record.Host, err, hostOfB)
			}
			return nil
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
	})

	t.Run("a host another workspace answers at is refused at the field", func(t *testing.T) {
		_, err := registry.Invoke(ctx, adminservice.ProvisionTenantName, assignedOperator(), usecase.Input{
			"host": host, "slug": "other-" + suffix, "display_name": "Other",
			"owner_email": "eva@corp.example",
		})
		refusal := shared.AsError(err)
		if refusal == nil || refusal.DetailCode != "admin.host_taken" || len(refusal.Fields) != 1 ||
			refusal.Fields[0].Path != "/host" {
			t.Fatalf("a taken host answered %v, want admin.host_taken at /host", err)
		}
		var written int
		if err := adminPool(ctx, t).QueryRow(ctx, `SELECT count(*) FROM tenant WHERE slug = $1`,
			"other-"+suffix).Scan(&written); err != nil || written != 0 {
			t.Errorf("the refused workspace was written (%d, %v)", written, err)
		}
	})

	const password = "a lantern above the assigned host"
	hash, err := writer.Passwords.Hash(secret.New(password))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET password_hash = $2, status = 'ACTIVE' WHERE id = $1`,
		owner.String(), hash); err != nil {
		t.Fatalf("giving the owner a password: %v", err)
	}

	t.Run("sign-in and the sign-in card", func(t *testing.T) {
		signedIn, err := identityservice.SignIn{Writer: writer}.Execute(ctx, identityservice.SignInCommand{
			Email: "jana.koch@corp.example", Password: secret.New(password), Host: host + ":8443",
		})
		if err != nil || signedIn.Pair == nil || signedIn.Pair.Session.TenantID != tenant {
			t.Fatalf("signing in at the assigned host answered (%+v, %v)", signedIn, err)
		}
		card := identityservice.GetSignInRules{
			Resolver: identityservice.SignInPolicyResolver{
				Workspaces: postgres.NewWorkspaceSettingsRepository(),
				Instance:   postgres.NewInstanceSettingRepository(), UnitOfWork: uow,
			},
			Tenants: directory, Providers: postgres.NewIdentityProviderRepository(),
			Workspaces: postgres.NewWorkspaceSettingsRepository(), Managed: postgres.NewAccountRepository(),
			Clock: clockadapter.System{}, UnitOfWork: uow, Multi: true, InstallationHost: "hub.example",
		}
		rules, err := card.Execute(ctx, identityservice.GetSignInRulesCommand{Host: host})
		if err != nil || rules.WorkspaceHost != host || len(rules.Methods) == 0 {
			t.Errorf("the sign-in card at the assigned host answered (%+v, %v)", rules, err)
		}
	})

	address := identityservice.WorkspaceAddress{
		Workspaces: postgres.NewWorkspaceSettingsRepository(), Hosts: postgres.NewTenantHostRepository(),
		Domains: textadapter.Domains{}, BaseURL: assignedBase, Multi: true,
	}

	t.Run("the invitation names the assigned host", func(t *testing.T) {
		ids := clockadapter.NewUUIDv7(clockadapter.System{})
		invite, err := usecase.NewRegistry(nil, identityservice.InviteAccount{
			Accounts: postgres.NewAccountRepository(), Authorizer: allowAll{},
			Notifier: postgres.NewQueue(ids, clockadapter.System{}), Audit: postgres.NewAuditSink(generator{t}),
			UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids, Domains: textadapter.Domains{},
			Text: text.Composing{},
			Links: identityservice.InvitationLinks{
				Minter: identityservice.MintRedemptionToken{
					Accounts: signIn, UnitOfWork: uow, Clock: clockadapter.System{}, Entropy: clockadapter.CryptoRandom{},
				},
				Address: address,
			},
		}.Descriptor())
		if err != nil {
			t.Fatalf("registering: %v", err)
		}
		invited, err := invite.Invoke(ctx, identityservice.InviteAccountName,
			appshared.ActorContext{TenantID: tenant, AccountID: owner, AccountName: "Jana", Kind: shared.ActorUser},
			usecase.Input{"email": "colleague-" + suffix + "@corp.example"})
		if err != nil {
			t.Fatalf("inviting: %v", err)
		}
		link, _ := invited["invitation_link"].(string)
		if !strings.HasPrefix(link, "https://"+host+":8443/redeem#token=") {
			t.Errorf("the invitation's link is %q, want one at the assigned host", link)
		}
	})

	t.Run("the provider's return lands on the assigned host", func(t *testing.T) {
		flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
		material := make([]byte, domain.TokenSecretBytes)
		for i := range material {
			material[i] = 0x40 + byte(i)
		}
		state, err := domain.NewOidcFlowState(tenant, material)
		if err != nil {
			t.Fatalf("minting a state: %v", err)
		}
		flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
			ID: freshID(t), TenantID: tenant, ProviderID: hostProvider, Nonce: "the-nonce",
			Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("building a flow: %v", err)
		}
		if err := uow.Within(ctx, persistence.Scope{TenantID: tenant},
			func(ctx context.Context) error { return flows.Insert(ctx, flow, state) }); err != nil {
			t.Fatalf("writing the flow: %v", err)
		}
		base, err := identityservice.LocateOidcFlow{
			Flows: flows, UnitOfWork: uow, Clock: clockadapter.System{}, Address: address,
		}.Execute(ctx, state.Secret())
		if err != nil || base != "https://"+host+":8443" {
			t.Errorf("the return was located at (%q, %v), want the assigned host", base, err)
		}
	})
}
