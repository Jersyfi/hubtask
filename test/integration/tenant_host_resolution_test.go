// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	queueport "github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// A request's host names its workspace through `tenant_host` (ADR-0095 §1, migration 0207). Its own
// workspaces, named by nobody else in this shared database: A answers at a host that is not its
// subdomain, B at its subdomain, and a deleted workspace still holds a serving row.
var (
	hostTenantA    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000207a01")
	hostTenantB    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000207a02")
	hostTenantGone = shared.MustParseID("01936f2a-7c1e-7000-8000-000000207a03")
	hostAccountA   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000207a11")
	hostProvider   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000207a21")
)

const (
	hostOfA      = "tasks.host-a.example"
	hostOfB      = "host-b.hub.example"
	hostOfGone   = "host-gone.hub.example"
	hostPendingA = "pending.host-a.example"
)

// servedAt writes a serving main row for a workspace, as provisioning or `ensure_main_hosts` would.
func servedAt(ctx context.Context, t *testing.T, tenant shared.ID, host string) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO tenant_host (tenant_id, host, state, verification, verified_at, is_canonical, created_at)
		VALUES ($1, $2, 'ACTIVE', 'hbt-dv-integration', now(), true, now())
		ON CONFLICT DO NOTHING`, tenant.String(), host); err != nil {
		t.Fatalf("seeding the host %s: %v", host, err)
	}
}

func seedHostTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES
		 ('` + hostTenantA.String() + `', 'host-a', 'Host A'),
		 ('` + hostTenantB.String() + `', 'host-b', 'Host B'),
		 ('` + hostTenantGone.String() + `', 'host-gone', 'Host Gone')
		 ON CONFLICT (id) DO NOTHING`,
		`UPDATE tenant SET deleted_at = now() WHERE id = '` + hostTenantGone.String() + `'`,
		`DELETE FROM tenant_host WHERE tenant_id IN ('` + hostTenantA.String() + `', '` +
			hostTenantB.String() + `', '` + hostTenantGone.String() + `')`,
		`INSERT INTO tenant_host (tenant_id, host, state, verification, created_at)
		 VALUES ('` + hostTenantA.String() + `', '` + hostPendingA + `', 'PENDING', 'hbt-dv-pending', now())`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	servedAt(ctx, t, hostTenantA, hostOfA)
	servedAt(ctx, t, hostTenantB, hostOfB)
	servedAt(ctx, t, hostTenantGone, hostOfGone)
}

// The lookup answers one workspace or none: a serving row of a live workspace, and nothing for the
// installation's own host, a row nobody serves yet, or a deleted workspace's host.
func TestAHostNamesOneWorkspaceOrNone(t *testing.T) {
	ctx := context.Background()
	seedHostTenants(ctx, t)
	_, directory, uow := realWriter(ctx, t)

	resolve := func(host string) (shared.ID, error) {
		var named shared.ID
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			id, err := directory.ResolveHost(ctx, host)
			named = id
			return err
		})
		return named, err
	}
	for _, c := range []struct {
		name, host string
		want       shared.ID
	}{
		{"a host that is not the workspace's subdomain", hostOfA, hostTenantA},
		{"the same host, as a browser may spell it", "Tasks.Host-A.Example.", hostTenantA},
		{"another workspace's subdomain", hostOfB, hostTenantB},
		{"the installation's own host", "hub.example", ""},
		{"a row nobody serves yet", hostPendingA, ""},
		{"a deleted workspace's host", hostOfGone, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			named, err := resolve(domain.RequestHost(c.host))
			if c.want.IsZero() {
				if !errors.Is(err, shared.ErrNotFound) {
					t.Errorf("answered (%q, %v), want none", named, err)
				}
				return
			}
			if err != nil || named != c.want {
				t.Errorf("answered (%q, %v), want %s", named, err, c.want)
			}
		})
	}
}

// Gate SG-3 for the new lookup: inside A's transaction B's rows stay invisible, and B's host names B
// - an identifier and nothing else - never the workspace that asks.
func TestAHostLookupNeverAnswersTheWorkspaceThatAsks(t *testing.T) {
	ctx := context.Background()
	seedHostTenants(ctx, t)
	_, directory, uow := realWriter(ctx, t)
	hosts := postgres.NewTenantHostRepository()

	inTenant(t, uow, hostTenantA, func(ctx context.Context) error {
		listed, err := hosts.List(ctx)
		if err != nil {
			return err
		}
		for _, row := range listed {
			if row.Host == hostOfB {
				t.Error("A lists B's host")
			}
		}
		named, err := directory.ResolveHost(ctx, hostOfB)
		if err != nil || named != hostTenantB {
			t.Errorf("B's host answered (%q, %v) inside A's transaction, want B", named, err)
		}
		return nil
	})
}

// UC-INS-19 check 10 and ADR-0095 §2: sign-in, the reset request and the provider's return find the
// workspace by a row whose host is not its subdomain.
func TestTheDoorsFindAWorkspaceByItsRow(t *testing.T) {
	ctx := context.Background()
	seedHostTenants(ctx, t)
	passwords, directory := passwordWriterOver(ctx, t)
	writer := passwords.Session
	writer.InstallationHost = "hub.example"
	const address = "host-a@example.org"
	hash, err := writer.Passwords.Hash(secret.New("a lantern on the far shore"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', $3, 'Host A', $4, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		hostAccountA.String(), hostTenantA.String(), address, hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	t.Run("sign-in", func(t *testing.T) {
		signedIn, err := identityservice.SignIn{Writer: writer}.Execute(ctx, identityservice.SignInCommand{
			Email: address, Password: secret.New("a lantern on the far shore"), Host: hostOfA + ":443",
		})
		if err != nil || signedIn.Pair == nil || signedIn.Pair.Session.TenantID != hostTenantA {
			t.Fatalf("signing in at A's host answered (%+v, %v)", signedIn, err)
		}
		// At the installation's own host the header names the workspace; at a host no row serves
		// it names nothing (T-28).
		atBase, err := identityservice.SignIn{Writer: writer}.Execute(ctx, identityservice.SignInCommand{
			Email: address, Password: secret.New("a lantern on the far shore"), Host: "hub.example",
			TenantHeader: hostTenantA.String(),
		})
		if err != nil || atBase.Pair == nil || atBase.Pair.Session.TenantID != hostTenantA {
			t.Errorf("signing in at the installation's host with the header answered (%+v, %v)", atBase, err)
		}
		if _, err := (identityservice.SignIn{Writer: writer}).Execute(ctx, identityservice.SignInCommand{
			Email: address, Password: secret.New("a lantern on the far shore"), Host: "gone.hub.example",
			TenantHeader: hostTenantA.String(),
		}); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("signing in at an unknown host with the header answered %v, want tenant_unresolved", err)
		}
		// The same person at B's host is a stranger there: B's row names B.
		if _, err := (identityservice.SignIn{Writer: writer}).Execute(ctx, identityservice.SignInCommand{
			Email: address, Password: secret.New("a lantern on the far shore"), Host: hostOfB,
		}); err == nil {
			t.Error("A's account signed in at B's host")
		}
	})

	t.Run("reset request", func(t *testing.T) {
		admin := adminPool(ctx, t)
		forget := func() {
			if _, err := admin.Exec(ctx, `DELETE FROM job WHERE tenant_id = $1 AND payload->>'account_id' = $2`,
				hostTenantA.String(), hostAccountA.String()); err != nil {
				t.Fatalf("clearing the jobs: %v", err)
			}
		}
		forget()
		t.Cleanup(forget)
		if err := (identityservice.ForgetPassword{
			Writer: passwords, Notifier: postgres.NewQueue(passwords.IDs, clockadapter.System{}),
			Tenants: directory, Multi: true,
		}).Execute(ctx, identityservice.ForgetPasswordCommand{Email: address, Host: hostOfA}); err != nil {
			t.Fatalf("asking for a link: %v", err)
		}
		var queued int
		if err := admin.QueryRow(ctx, `
			SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = $2 AND payload->>'account_id' = $3`,
			hostTenantA.String(), string(queueport.KindPasswordResetRequest), hostAccountA.String()).Scan(&queued); err != nil {
			t.Fatalf("reading the request job: %v", err)
		}
		if queued != 1 {
			t.Errorf("%d request jobs for A's account, want one", queued)
		}
	})

	t.Run("the provider's return", func(t *testing.T) {
		flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
		if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM oidc_flow WHERE tenant_id = $1`,
			hostTenantA.String()); err != nil {
			t.Fatalf("clearing an earlier run's flows: %v", err)
		}
		material := make([]byte, domain.TokenSecretBytes)
		for i := range material {
			material[i] = 0x70 + byte(i)
		}
		state, err := domain.NewOidcFlowState(hostTenantA, material)
		if err != nil {
			t.Fatalf("minting a state: %v", err)
		}
		flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
			ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000207a31"), TenantID: hostTenantA,
			ProviderID: hostProvider, Nonce: "the-nonce",
			Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("building a flow: %v", err)
		}
		if err := writer.UnitOfWork.Within(ctx, persistence.Scope{TenantID: hostTenantA},
			func(ctx context.Context) error { return flows.Insert(ctx, flow, state) }); err != nil {
			t.Fatalf("writing the flow: %v", err)
		}

		base, err := identityservice.LocateOidcFlow{
			Flows: flows, Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Hosts: postgres.NewTenantHostRepository(), UnitOfWork: writer.UnitOfWork,
			Clock: clockadapter.System{}, BaseURL: "https://hub.example:8443", Multi: true,
		}.Execute(ctx, state.Secret())
		if err != nil || base != "https://"+hostOfA+":8443" {
			t.Errorf("located (%q, %v), want A's main row", base, err)
		}
	})
}

// beforeTheHostResolver is the last version without migration 0207.
const beforeTheHostResolver = 206

// Migration 0207 on a database whose workspaces have no rows, run as a managed service runs it:
// `ensure_main_hosts` writes each live workspace's main row once, and the lookup then answers it.
// Seeded as the superuser, because the seed is the state an earlier release left.
func TestEveryWorkspaceGetsItsMainRowAfterTheMigration(t *testing.T) {
	ctx := context.Background()
	const name = "tenant_host_main_rows"
	ownerDSN, appDSN := managedDatabase(ctx, t, name)
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeTheHostResolver); err != nil {
		t.Fatalf("migrating to %d: %v", beforeTheHostResolver, err)
	}
	adminDSN := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(dbtest.Start(t).AdminDSN, "/"+name+"$1")
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	oldA, oldB, kept, gone := freshID(t).String(), freshID(t).String(), freshID(t).String(), freshID(t).String()
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES
		 ('` + oldA + `', 'old-a', 'Old A'), ('` + oldB + `', 'old-b', 'Old B'),
		 ('` + kept + `', 'kept', 'Kept'), ('` + gone + `', 'gone', 'Gone')`,
		`UPDATE tenant SET deleted_at = now() WHERE id = '` + gone + `'`,
		`INSERT INTO tenant_host (tenant_id, host, state, verification, verified_at, is_canonical, created_at)
		 VALUES ('` + kept + `', 'tasks.kept.example', 'ACTIVE', 'hbt-dv-kept', now(), true, now())`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("application pool: %v", err)
	}
	defer app.Close()
	ensure := func(base string) int {
		t.Helper()
		var written int
		if err := app.QueryRow(ctx, `SELECT ensure_main_hosts($1)`, base).Scan(&written); err != nil {
			t.Fatalf("ensure_main_hosts: %v", err)
		}
		return written
	}

	if written := ensure(""); written != 0 {
		t.Errorf("an empty base wrote %d rows", written)
	}
	if written := ensure("Hub.Example."); written != 2 {
		t.Errorf("the first start wrote %d rows, want the two workspaces without one", written)
	}
	if written := ensure("hub.example"); written != 0 {
		t.Errorf("the second start wrote %d rows, want none", written)
	}

	rows := map[string]string{}
	listed, err := admin.Query(ctx, `
		SELECT tenant_id::text, host FROM tenant_host
		WHERE is_canonical AND state = 'ACTIVE' AND verification LIKE 'hbt-dv-%'`)
	if err != nil {
		t.Fatalf("reading the rows: %v", err)
	}
	for listed.Next() {
		var tenant, host string
		if err := listed.Scan(&tenant, &host); err != nil {
			t.Fatalf("reading a row: %v", err)
		}
		rows[tenant] = host
	}
	listed.Close()
	want := map[string]string{oldA: "old-a.hub.example", oldB: "old-b.hub.example", kept: "tasks.kept.example"}
	for tenant, host := range want {
		if rows[tenant] != host {
			t.Errorf("workspace %s answers at %q, want %q", tenant, rows[tenant], host)
		}
	}
	if host, found := rows[gone]; found {
		t.Errorf("the deleted workspace was given %q", host)
	}

	resolve := func(host string) *string {
		t.Helper()
		var named *string
		if err := app.QueryRow(ctx, `SELECT resolve_tenant_host($1)::text`, host).Scan(&named); err != nil {
			t.Fatalf("resolve_tenant_host: %v", err)
		}
		return named
	}
	if named := resolve("old-a.hub.example"); named == nil || *named != oldA {
		t.Errorf("old-a's subdomain answered %v", named)
	}
	if named := resolve("hub.example"); named != nil {
		t.Errorf("the installation's own host answered %v", *named)
	}
}
