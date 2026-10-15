// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"bytes"
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
)

// UC-ID-22: a person's passkeys and security keys, through the registry as a client reaches them.

// passkeyStore keeps credentials in memory, per account, refusing an identifier held twice.
type passkeyStore struct {
	held   []domain.PasskeyCredential
	locked []shared.ID
}

var _ repository.PasskeyCredentials = (*passkeyStore)(nil)

func (s *passkeyStore) ListOf(_ context.Context, accountID shared.ID) ([]domain.PasskeyCredential, error) {
	var out []domain.PasskeyCredential
	for _, credential := range s.held {
		if credential.AccountID == accountID {
			out = append(out, credential)
		}
	}
	return out, nil
}

func (s *passkeyStore) CountOf(ctx context.Context, accountID shared.ID) (int, error) {
	held, _ := s.ListOf(ctx, accountID)
	return len(held), nil
}

func (s *passkeyStore) LockAccount(_ context.Context, accountID shared.ID) error {
	s.locked = append(s.locked, accountID)
	return nil
}

func (s *passkeyStore) Insert(_ context.Context, credential domain.PasskeyCredential) (bool, error) {
	for _, held := range s.held {
		if bytes.Equal(held.CredentialID, credential.CredentialID) {
			return false, nil
		}
	}
	s.held = append(s.held, credential)
	return true, nil
}

// tenantHostsFake answers the workspace's hosts.
type tenantHostsFake struct{ hosts []domain.TenantHost }

func (h tenantHostsFake) Insert(context.Context, domain.TenantHost) error { return nil }

func (h tenantHostsFake) List(context.Context) ([]domain.TenantHost, error) { return h.hosts, nil }

// noMethodStepUp is an account that holds nothing to prove itself afresh with.
type noMethodStepUp struct{ stepUpFake }

func (noMethodStepUp) Methods(context.Context, shared.ID, shared.ID) ([]stepupport.Method, error) {
	return nil, nil
}

type passkeyFixture struct {
	session  *sessionFixture
	store    *passkeyStore
	stepUp   *stepUpFake
	writer   PasskeyWriter
	registry *usecase.Registry
}

// newPasskeyFixture is Bert, a person in the workspace Acme of a multi-mode installation at
// hubtask.example, signed in on a live session, holding no passkey yet.
func newPasskeyFixture(t *testing.T) *passkeyFixture {
	t.Helper()
	session := mfaFixture(now)
	credential, _ := refreshCredential(now)
	session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	f := &passkeyFixture{session: session, store: &passkeyStore{}, stepUp: &stepUpFake{satisfied: true}}
	f.writer = PasskeyWriter{
		Session: session.writer, Passkeys: f.store,
		Workspaces: &workspaceStore{row: domain.Workspace{
			Tenant: domain.Tenant{ID: tenant, Slug: "acme", DisplayName: "Acme"}, Version: 1,
		}},
		Installation: PasskeyInstallation{Host: "hubtask.example", Scheme: "https", Multi: true},
		StepUp:       f.stepUp,
	}
	f.register(t)
	return f
}

func (f *passkeyFixture) register(t *testing.T) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, ListOwnPasskeys{Writer: f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
}

func (f *passkeyFixture) holding(t *testing.T, count int) {
	t.Helper()
	for i := range count {
		f.store.held = append(f.store.held, domain.PasskeyCredential{
			ID:       shared.ID("01936f2a-7c1e-7000-8000-0000000000" + string(rune('a'+i/10)) + string(rune('0'+i%10))),
			TenantID: tenant, AccountID: account, CredentialID: []byte{byte(i + 1)},
			Name: "Key", CreatedAt: now.Add(time.Duration(i) * time.Minute),
		})
	}
}

func (f *passkeyFixture) list(t *testing.T, actor appshared.ActorContext) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), ListOwnPasskeysName, actor, usecase.Input{})
}

func TestAPersonsPasskeysAreListedByNameAndDateAndNothingElse(t *testing.T) {
	f := newPasskeyFixture(t)
	used := now.Add(time.Hour)
	f.store.held = []domain.PasskeyCredential{
		{ID: "01936f2a-7c1e-7000-8000-000000000001", TenantID: tenant, AccountID: account,
			CredentialID: []byte{1, 2, 3}, PublicKey: []byte{9}, UserHandle: []byte{7},
			Name: "iPhone", CreatedAt: now, LastUsedAt: used},
		{ID: "01936f2a-7c1e-7000-8000-000000000002", TenantID: tenant, AccountID: account,
			CredentialID: []byte{4}, Name: "Key in the drawer", CreatedAt: now.Add(time.Minute)},
		{ID: "01936f2a-7c1e-7000-8000-000000000003", TenantID: tenant, AccountID: "someone-else",
			CredentialID: []byte{5}, Name: "Not Bert's", CreatedAt: now},
	}
	out, err := f.list(t, signedInActor())
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 2 || rows[0].String("name") != "iPhone" || rows[1].String("name") != "Key in the drawer" {
		t.Fatalf("listed %v", rows)
	}
	if at, _ := rows[0]["last_used_at"].(time.Time); !at.Equal(used) {
		t.Errorf("last used %v", rows[0]["last_used_at"])
	}
	if _, held := rows[1]["last_used_at"]; held {
		t.Error("a passkey never used has a last use")
	}
	for _, row := range rows {
		for key := range row {
			if key != "id" && key != "name" && key != "created_at" && key != "last_used_at" {
				t.Errorf("a listed passkey carries %q", key)
			}
		}
	}
	if out["can_add"] != true || out["limit"] != domain.MaxPasskeys || out.String("rp_id") != "acme.hubtask.example" {
		t.Errorf("can_add %v, limit %v, rp_id %q", out["can_add"], out["limit"], out.String("rp_id"))
	}
	if _, held := out["cannot_add_reason"]; held {
		t.Error("a reason was given where adding is possible")
	}
}

// P-05: the list says another cannot be added exactly where starting would refuse, and why.
func TestTheListSaysWhyAnotherCannotBeAdded(t *testing.T) {
	cases := map[string]struct {
		arrange func(f *passkeyFixture)
		reason  string
		rpID    string
	}{
		"ten held": {func(f *passkeyFixture) { f.holding(t, domain.MaxPasskeys) }, codeLimitReached, "acme.hubtask.example"},
		"no public address": {func(f *passkeyFixture) {
			f.writer.Installation = PasskeyInstallation{Multi: true}
		}, codeNoRelyingParty, ""},
		"no fresh proof to give": {func(f *passkeyFixture) {
			f.writer.StepUp = &noMethodStepUp{}
		}, codeStepUpNoMethod, "acme.hubtask.example"},
		"no step-up wired": {func(f *passkeyFixture) { f.writer.StepUp = nil }, codeStepUpNoMethod, "acme.hubtask.example"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPasskeyFixture(t)
			c.arrange(f)
			f.register(t)
			out, err := f.list(t, signedInActor())
			if err != nil {
				t.Fatal(err)
			}
			if out["can_add"] != false || out.String("cannot_add_reason") != c.reason || out.String("rp_id") != c.rpID {
				t.Errorf("can_add %v, reason %q, rp_id %q", out["can_add"], out.String("cannot_add_reason"), out.String("rp_id"))
			}
		})
	}
}

// The RP ID is the workspace's canonical host: in single mode the installation's, and where the
// workspace holds an active canonical row, that row's (identity.md §23.3).
func TestTheRelyingPartyIsTheWorkspacesCanonicalHost(t *testing.T) {
	f := newPasskeyFixture(t)
	f.writer.Installation = PasskeyInstallation{Host: "tasks.example.org", Scheme: "https", Port: "8443"}
	party, ok := relyingPartyOf(t, f)
	if !ok || party.ID != "tasks.example.org" || party.Origin != "https://tasks.example.org:8443" || party.Name != "Acme" {
		t.Errorf("single mode: %+v %v", party, ok)
	}

	f.writer.Installation = PasskeyInstallation{Host: "hubtask.example", Scheme: "https", Multi: true}
	f.writer.Hosts = tenantHostsFake{hosts: []domain.TenantHost{
		{Host: "old.acme.example", State: domain.HostBroken, Canonical: true},
		{Host: "acme.hubtask.example", State: domain.HostActive, Canonical: true},
	}}
	if party, ok := relyingPartyOf(t, f); !ok || party.ID != "acme.hubtask.example" {
		t.Errorf("an active canonical row: %+v %v", party, ok)
	}
	f.writer.Hosts = tenantHostsFake{}
	if party, ok := relyingPartyOf(t, f); !ok || party.ID != "acme.hubtask.example" {
		t.Errorf("no row: %+v %v", party, ok)
	}
}

func relyingPartyOf(t *testing.T, f *passkeyFixture) (relyingParty, bool) {
	t.Helper()
	party, ok, err := f.writer.relyingParty(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return party, ok
}

func TestOnlyAPersonHoldsPasskeys(t *testing.T) {
	f := newPasskeyFixture(t)
	for _, kind := range []appshared.ActorKind{
		appshared.ActorServiceAccount, appshared.ActorAutomation, appshared.ActorAIAgent, appshared.ActorSystem,
	} {
		actor := signedInActor()
		actor.Kind = kind
		_, err := f.list(t, actor)
		refusedWith(t, err, codePersonOnly)
	}
	// A service account's own row, whatever kind its credential claims.
	f.session.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountServiceAccount, DisplayName: "Robot", Status: domain.AccountActive,
	})
	f.writer.Session = f.session.writer
	f.register(t)
	_, err := f.list(t, signedInActor())
	refusedWith(t, err, codePersonOnly)

	_, err = f.list(t, appshared.ActorContext{})
	refusedWith(t, err, "access.credential_required")
}
