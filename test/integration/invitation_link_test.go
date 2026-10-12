// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// UC-ID-14 check 5 against the real database: on an installation that sends no mail the invitation
// answers its link, queues no mail whose delivery would mint again and kill it, and the link is the
// mailed one in every way that counts - the token the provider's start looks up and the redemption
// accepts. A renewal replaces it at once, and only inside its own workspace (gate SG-3). Its own
// workspaces and addresses, named by nobody else in this shared database.
var (
	sc13TenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-000000013a01")
	sc13TenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-000000013a02")
	sc13AdminA  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000013a11")
	sc13AdminB  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000013a12")
)

const sc13Base = "https://home.example"

func seedLinkTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name)
		VALUES ($1, 'sc13-a', 'SC-13 A'), ($2, 'sc13-b', 'SC-13 B')
		ON CONFLICT (id) DO NOTHING`, sc13TenantA.String(), sc13TenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'admin-a@sc13.example', 'Admin A', 'ACTIVE'),
		       ($3, $4, 'USER', 'admin-b@sc13.example', 'Admin B', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING`,
		sc13AdminA.String(), sc13TenantA.String(), sc13AdminB.String(), sc13TenantB.String()); err != nil {
		t.Fatalf("seeding the administrators: %v", err)
	}
}

// invitationRegistry wires the invitation and its renewal the way the composition root does, with
// mail or without.
func invitationRegistry(t *testing.T, uow persistence.UnitOfWork, signIn postgres.SignInRepository, withoutMail bool) *usecase.Registry {
	t.Helper()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	jobs := postgres.NewQueue(ids, clockadapter.System{})
	var links identityservice.InvitationLinks
	if withoutMail {
		links = identityservice.InvitationLinks{
			Minter: identityservice.MintRedemptionToken{
				Accounts: signIn, UnitOfWork: uow, Clock: clockadapter.System{}, Entropy: clockadapter.CryptoRandom{},
			},
			BaseURL: sc13Base,
		}
	}
	accounts := postgres.NewAccountRepository()
	sink := postgres.NewAuditSink(generator{t})
	registry, err := usecase.NewRegistry(nil,
		identityservice.InviteAccount{
			Accounts: accounts, Authorizer: allowAll{}, Notifier: jobs, Audit: sink, UnitOfWork: uow,
			Clock: clockadapter.System{}, IDs: ids, Domains: textadapter.Domains{}, Text: text.Composing{},
			Links: links,
		}.Descriptor(),
		identityservice.RenewInvitation{
			Accounts: accounts, Authorizer: allowAll{}, Notifier: jobs, Audit: sink, UnitOfWork: uow,
			Clock: clockadapter.System{}, Links: links,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

func sc13Actor(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenant, AccountID: account, AccountName: "Admin", Kind: shared.ActorUser}
}

// tokenOf reads the credential out of a link's fragment, as the redeem screen does.
func tokenOf(t *testing.T, link any) secret.Secret {
	t.Helper()
	text, _ := link.(string)
	prefix := sc13Base + "/redeem#token="
	if !strings.HasPrefix(text, prefix) {
		t.Fatalf("link %q, want %s…", text, prefix)
	}
	return secret.New(strings.TrimPrefix(text, prefix))
}

func TestAnInvitationWithoutMailIsALinkThatWorksLikeTheMailedOne(t *testing.T) {
	ctx := context.Background()
	seedLinkTenants(ctx, t)
	sessionFixtures(ctx, t)
	writer, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	registry := invitationRegistry(t, uow, signIn, true)
	address := "family-" + freshID(t).String() + "@sc13.example"

	out, err := registry.Invoke(ctx, identityservice.InviteAccountName, sc13Actor(sc13TenantA, sc13AdminA),
		usecase.Input{"email": address})
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	invitedID := shared.MustParseID(out["id"].(string))
	first := tokenOf(t, out["invitation_link"])

	// No mail queued: its delivery would mint a token of its own and kill the link handed over.
	var queued int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE kind = 'notification.invitation' AND dedupe_key = $1`,
		invitedID.String()).Scan(&queued); err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if queued != 0 {
		t.Errorf("%d invitation mails queued beside the link", queued)
	}

	// The token the link carries is the one the provider's start looks up (OidcSignIn's
	// invitation check) - in its own workspace, and in no other.
	finds := func(tenant shared.ID, presented secret.Secret) bool {
		t.Helper()
		token, err := domain.ParseRedemptionToken(presented.Reveal())
		if err != nil {
			t.Fatalf("the link carries no redemption token: %v", err)
		}
		found := false
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			read, err := signIn.FindByRedemptionToken(ctx, token)
			switch {
			case err == nil:
				found = read.Account.ID == invitedID
			case !errors.Is(err, shared.ErrNotFound):
				return err
			}
			return nil
		})
		return found
	}
	if !finds(sc13TenantA, first) {
		t.Fatal("the link's token does not find the invited account")
	}
	if finds(sc13TenantB, first) {
		t.Error("another workspace found the invitation by its link")
	}

	// Another workspace cannot renew it: the account is not found there (SG-3).
	_, err = registry.Invoke(ctx, identityservice.RenewInvitationName, sc13Actor(sc13TenantB, sc13AdminB),
		usecase.Input{"account_id": invitedID.String()})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("renewing from another workspace answered %v, want not found", err)
	}

	// The renewal replaces the link at once: the first stops working, the new one works.
	renewed, err := registry.Invoke(ctx, identityservice.RenewInvitationName, sc13Actor(sc13TenantA, sc13AdminA),
		usecase.Input{"account_id": invitedID.String()})
	if err != nil {
		t.Fatalf("renewing: %v", err)
	}
	second := tokenOf(t, renewed["invitation_link"])
	if finds(sc13TenantA, first) {
		t.Error("the first link still works after the renewal")
	}
	if !finds(sc13TenantA, second) {
		t.Fatal("the renewed link does not find the invited account")
	}

	// Accepted with a password through the link, exactly as through the mailed one.
	if _, err := (identityservice.RedeemInvitation{Writer: writer}).Execute(ctx, identityservice.RedeemInvitationCommand{
		Token: second, Password: secret.New("seven blue lanterns over the harbour"),
	}); err != nil {
		t.Fatalf("redeeming the handed-over link: %v", err)
	}
	var status string
	if err := admin.QueryRow(ctx, `SELECT status FROM account WHERE id = $1`, invitedID.String()).Scan(&status); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if status != "ACTIVE" {
		t.Errorf("the account is %s after accepting, want ACTIVE", status)
	}

	// Once accepted there is nothing to renew.
	_, err = registry.Invoke(ctx, identityservice.RenewInvitationName, sc13Actor(sc13TenantA, sc13AdminA),
		usecase.Input{"account_id": invitedID.String()})
	if shared.AsError(err).DetailCode != "accounts.not_invited" {
		t.Errorf("renewing an accepted invitation answered %v, want accounts.not_invited", err)
	}

	// The trail says how each went out, and never carries the link.
	var trail int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND target_id = $2
		  AND action IN ('account.invited', 'account.invitation_renewed')
		  AND changes -> 'delivery' ->> 'to' = 'LINK'
		  AND changes::text NOT LIKE '%hbt_inv_%'`,
		sc13TenantA.String(), invitedID.String()).Scan(&trail); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if trail != 2 {
		t.Errorf("%d trail entries with delivery LINK and no link, want the invitation and the renewal", trail)
	}
}

// With mail nothing changes: the mail is queued, no token exists until its delivery mints one, and
// the answer carries no link.
func TestAnInvitationWithMailQueuesTheMailAndAnswersNoLink(t *testing.T) {
	ctx := context.Background()
	seedLinkTenants(ctx, t)
	sessionFixtures(ctx, t)
	_, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	registry := invitationRegistry(t, uow, signIn, false)
	address := "mailed-" + freshID(t).String() + "@sc13.example"

	out, err := registry.Invoke(ctx, identityservice.InviteAccountName, sc13Actor(sc13TenantA, sc13AdminA),
		usecase.Input{"email": address})
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	// The queue is shared by the whole package: a mail left pending here would be claimed and
	// sent by another test's worker.
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(),
			`DELETE FROM job WHERE kind = 'notification.invitation' AND dedupe_key = $1`, out["id"].(string))
	})
	if _, linked := out["invitation_link"]; linked {
		t.Error("a link answered where the mail carries the invitation")
	}
	var queued int
	var hash []byte
	if err := admin.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM job WHERE kind = 'notification.invitation' AND dedupe_key = $1),
		       (SELECT redemption_token_hash FROM account WHERE id = $1::uuid)`,
		out["id"].(string)).Scan(&queued, &hash); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if queued != 1 || hash != nil {
		t.Errorf("%d mails queued and a token hash %v, want one mail and no token yet", queued, hash)
	}
}
