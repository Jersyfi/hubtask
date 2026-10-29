// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// UC-INS-15 checks 3, 4, 5 and 10 against the database (migration 0211): the operations port's
// answer for a verified, a pending, a blocked and an unknown host; a verified domain answering and
// turning active on its first request after the ingress asked; a block that stops it answering at
// once and keeps it from every workspace; and a workspace that reaches no other's own domain.
var (
	domainTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-000000211a01")
	domainOther  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000211a02")
)

func seedDomainTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`DELETE FROM blocked_domain WHERE host LIKE '%.own-domain.example'`,
		`DELETE FROM tenant_host WHERE tenant_id IN ('` + domainTenant.String() + `', '` + domainOther.String() + `')`,
		`DELETE FROM tenant WHERE id IN ('` + domainTenant.String() + `', '` + domainOther.String() + `')`,
		`INSERT INTO tenant (id, slug, display_name) VALUES
		 ('` + domainTenant.String() + `', 'domain-acme', 'Acme Ltd'),
		 ('` + domainOther.String() + `', 'domain-north', 'Northwind Studio')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	servedAt(ctx, t, domainTenant, "domain-acme.hub.example")
	servedAt(ctx, t, domainOther, "domain-north.hub.example")
}

func ownDomainRow(t *testing.T, tenant shared.ID, host string) domain.TenantHost {
	t.Helper()
	row, err := domain.NewOwnDomain(domain.NewOwnDomainInput{
		TenantID: tenant, Host: host, Mark: "q7n4kx2p8zram3vd", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestAnOwnDomainAnswersOnceProvenAndStopsWhenBlocked(t *testing.T) {
	ctx := context.Background()
	seedDomainTenants(ctx, t)
	_, directory, uow := realWriter(ctx, t)
	domains := postgres.NewOwnDomainRepository()
	const host = "tasks.own-domain.example"

	certificate := func(asked string) bool {
		t.Helper()
		var allowed bool
		err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			answer, err := domains.Certificate(ctx, asked, time.Now())
			allowed = answer
			return err
		})
		if err != nil {
			t.Fatalf("the ingress's question: %v", err)
		}
		return allowed
	}
	resolve := func(asked string) (shared.ID, bool) {
		t.Helper()
		var (
			named  shared.ID
			awaits bool
		)
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			id, waiting, err := directory.ResolveServing(ctx, asked)
			named, awaits = id, waiting
			return err
		})
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			t.Fatal(err)
		}
		return named, awaits
	}

	// Pending: no certificate, nothing answers.
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		return domains.Insert(ctx, ownDomainRow(t, domainTenant, host))
	})
	if certificate(host) {
		t.Error("a pending domain may have a certificate")
	}
	if named, _ := resolve(host); !named.IsZero() {
		t.Errorf("a pending domain names %s", named)
	}
	// The unknown host, and a workspace's subdomain.
	if certificate("nobody.own-domain.example") {
		t.Error("an unknown host may have a certificate")
	}
	if !certificate("domain-acme.hub.example") {
		t.Error("a workspace's subdomain may not have a certificate")
	}

	// Check 4: another workspace cannot take it.
	inTenantRolledBack(t, uow, domainOther, func(ctx context.Context) {
		err := domains.Insert(ctx, ownDomainRow(t, domainOther, host))
		if shared.AsError(err).DetailCode != "admin.own_domain_taken" {
			t.Errorf("another workspace took it: %v", err)
		}
	})

	// The check finds both records: verified. It answers, and the ingress may have its certificate.
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		written, err := domains.RecordLook(ctx, host, domain.HostPending, domain.HostVerified,
			domain.DomainLookup{Known: true, TargetFound: true, ProofFound: true}, time.Now())
		if !written {
			t.Error("the look was not written")
		}
		return err
	})
	if named, awaits := resolve(host); named != domainTenant || awaits {
		t.Errorf("verified, before the ingress asked: names %q, awaits %v", named, awaits)
	}
	if !certificate("Tasks.Own-Domain.Example.") {
		t.Error("a verified domain may not have a certificate")
	}
	// The ingress asked: the first request makes it active, and only the first.
	if named, awaits := resolve(host); named != domainTenant || !awaits {
		t.Errorf("after the ingress asked: names %q, awaits %v", named, awaits)
	}
	served := func() bool {
		var moved bool
		err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			answer, err := directory.Served(ctx, host)
			moved = answer
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return moved
	}
	if !served() || served() {
		t.Error("the first request did not make it active, or a second did again")
	}
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		view, err := domains.Find(ctx)
		if view.State != domain.HostActive || !view.CertificateAsked || !view.TargetFound || !view.ProofFound {
			t.Errorf("the owner sees %+v", view)
		}
		return err
	})

	// Cross-tenant: another workspace's transaction sees no own domain and removes none.
	inTenantRolledBack(t, uow, domainOther, func(ctx context.Context) {
		if _, err := domains.Find(ctx); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another workspace read the own domain: %v", err)
		}
		if _, err := domains.Remove(ctx); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another workspace removed it: %v", err)
		}
		if written, _ := domains.RecordLook(ctx, host, domain.HostActive, domain.HostBroken, domain.DomainLookup{Known: true}, time.Now()); written {
			t.Error("another workspace moved it")
		}
	})

	// The operator's list names it with its workspace.
	err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		all, err := domains.All(ctx)
		for _, row := range all {
			if row.Host == host && (row.TenantID != domainTenant || row.TenantName != "Acme Ltd" || row.State != domain.HostActive) {
				t.Errorf("listed %+v", row)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check 10: the block. The row goes and the block stays, in one transaction.
	reason := "Reported as a look-alike of another company's sign-in page."
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		if _, err := domains.Remove(ctx); err != nil {
			return err
		}
		return domains.Block(ctx, domain.DomainBlock{Host: host, Reason: reason, BlockedAt: time.Now()})
	})
	if named, _ := resolve(host); !named.IsZero() {
		t.Errorf("a blocked domain still names %s", named)
	}
	if certificate(host) {
		t.Error("a blocked domain may have a certificate")
	}
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		block, err := domains.BlockInView(ctx)
		if block.Reason != reason {
			t.Errorf("the owner sees the block %+v", block)
		}
		return err
	})
	inTenantRolledBack(t, uow, domainOther, func(ctx context.Context) {
		if _, err := domains.BlockInView(ctx); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another workspace sees the block: %v", err)
		}
		if blocked, err := domains.Blocked(ctx, host); err != nil || !blocked {
			t.Errorf("another workspace may set it up: %v, %v", blocked, err)
		}
		if err := domains.Block(ctx, domain.DomainBlock{Host: host, Reason: "again", BlockedAt: time.Now()}); shared.AsError(err).DetailCode != "admin.own_domain_already_blocked" {
			t.Errorf("blocked twice: %v", err)
		}
	})
	// The owner removes it from view; the block stays.
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		if n, err := domains.DismissBlocks(ctx, time.Now()); err != nil || n != 1 {
			t.Errorf("dismissed %d, %v", n, err)
		}
		blocked, err := domains.Blocked(ctx, host)
		if !blocked {
			t.Error("the block went with the owner's view")
		}
		return err
	})
}

// The check's due rows: a pending domain not looked at since the pass before, a serving one an hour
// ago; never a workspace's subdomain.
func TestTheCheckIsOwedTheDomainsDue(t *testing.T) {
	ctx := context.Background()
	seedDomainTenants(ctx, t)
	_, _, uow := realWriter(ctx, t)
	domains := postgres.NewOwnDomainRepository()
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		return domains.Insert(ctx, ownDomainRow(t, domainTenant, "due.own-domain.example"))
	})
	due := func(pendingBefore time.Time) []string {
		var hosts []string
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			rows, err := domains.Due(ctx, pendingBefore, time.Now().Add(-time.Hour), 500)
			for _, row := range rows {
				if row.TenantID == domainTenant || row.TenantID == domainOther {
					hosts = append(hosts, row.Host)
				}
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return hosts
	}
	if got := due(time.Now()); len(got) != 1 || got[0] != "due.own-domain.example" {
		t.Fatalf("due %v", got)
	}
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		_, err := domains.RecordLook(ctx, "due.own-domain.example", domain.HostPending, domain.HostPending,
			domain.DomainLookup{Known: true}, time.Now())
		return err
	})
	if got := due(time.Now().Add(-time.Minute)); len(got) != 0 {
		t.Errorf("looked at just now and due again: %v", got)
	}
}

// UC-INS-15 checks 7, 8 and 9 against the database (migration 0212): the main address moves to an
// active own domain and back, each move once; a browser on the host beside the main address is sent
// on to it - through the server's own redirect - while the API resolves the workspace on both; the
// fall-back keeps its moment; another workspace moves nothing.
func TestTheSubdomainSendsABrowserOnAndAnswersTheAPI(t *testing.T) {
	ctx := context.Background()
	seedDomainTenants(ctx, t)
	_, directory, uow := realWriter(ctx, t)
	domains := postgres.NewOwnDomainRepository()
	const own, sub = "main.own-domain.example", "domain-acme.hub.example"
	admin := adminPool(ctx, t)

	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		return domains.Insert(ctx, ownDomainRow(t, domainTenant, own))
	})
	if _, err := admin.Exec(ctx, `UPDATE tenant_host SET state = 'ACTIVE' WHERE host = $1`, own); err != nil {
		t.Fatal(err)
	}

	// Cross-tenant: another workspace's transaction makes nothing main.
	inTenantRolledBack(t, uow, domainOther, func(ctx context.Context) {
		if _, err := domains.MainToOwnDomain(ctx); shared.AsError(err).DetailCode != "admin.own_domain_not_active" {
			t.Errorf("another workspace made it main: %v", err)
		}
	})

	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		replaced, err := domains.MainToOwnDomain(ctx)
		if replaced != sub {
			t.Errorf("replaced %q", replaced)
		}
		return err
	})
	mains := func() []string {
		t.Helper()
		rows, err := admin.Query(ctx, `SELECT host FROM tenant_host WHERE tenant_id = $1 AND is_canonical`, domainTenant.String())
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var hosts []string
		for rows.Next() {
			var host string
			if err := rows.Scan(&host); err != nil {
				t.Fatal(err)
			}
			hosts = append(hosts, host)
		}
		return hosts
	}
	if got := mains(); len(got) != 1 || got[0] != own {
		t.Fatalf("main rows %v", got)
	}

	// The browser on the subdomain is sent on; the API resolves the workspace on both.
	browser := identity.BrowserAddress{
		Hosts: domains, UnitOfWork: uow, BaseURL: "https://hub.example", InstallationHost: "hub.example",
		Tenancy: env.TenancyMulti,
	}
	fallback := rest.Fallback{
		API: stubAPI{}, Reserved: []string{rest.APIBasePath + "/"}, Serve: stubAPI{}, Browsers: browser,
		UI: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	}
	ask := func(host, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		request.Host, request.Header["Sec-Fetch-Mode"] = host, []string{"navigate"}
		recorder := httptest.NewRecorder()
		fallback.ServeHTTP(recorder, request)
		return recorder
	}
	if page := ask(sub, "/hubs?x=1"); page.Code != http.StatusFound || page.Header().Get("Location") != "https://"+own+"/hubs?x=1" {
		t.Errorf("a browser on the subdomain: %d to %q", page.Code, page.Header().Get("Location"))
	}
	if api := ask(sub, rest.APIBasePath+"/items"); api.Code != http.StatusTeapot {
		t.Errorf("the API on the subdomain answered %d", api.Code)
	}
	if page := ask(own, "/"); page.Code != http.StatusOK {
		t.Errorf("the main address answered %d", page.Code)
	}
	for _, host := range []string{sub, own} {
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			named, _, err := directory.ResolveServing(ctx, host)
			if named != domainTenant {
				t.Errorf("%s names %s for the API", host, named)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Back by a fall-back: the subdomain is main, the moment kept, and the own domain sends on.
	fell := time.Now().UTC().Truncate(time.Second)
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		to, err := domains.MainToAddress(ctx, fell)
		if to != sub {
			t.Errorf("fell back to %q", to)
		}
		return err
	})
	if got := mains(); len(got) != 1 || got[0] != sub {
		t.Fatalf("main rows %v", got)
	}
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		view, err := domains.Find(ctx)
		if view.Main || !view.FellBackAt.Equal(fell) {
			t.Errorf("the owner sees %+v", view)
		}
		return err
	})
	if page := ask(own, "/"); page.Header().Get("Location") != "https://"+sub+"/" {
		t.Errorf("the own domain after switching back sends on to %q", page.Header().Get("Location"))
	}
	err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		all, err := domains.All(ctx)
		for _, row := range all {
			if row.Host == own && (row.AddressHost != sub || !row.FellBackAt.Equal(fell)) {
				t.Errorf("the operator sees %+v", row)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// stubAPI stands for the API's chain: it answers every request it is handed.
type stubAPI struct{}

func (stubAPI) Handler(*http.Request) (http.Handler, string) { return stubAPI{}, "GET /api" }

func (stubAPI) ServeHTTP(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }

// A change of address while the own domain is main replaces the subdomain beside it and leaves the
// own domain set up, no longer main; the host it answers is the one people knew, the own domain.
func TestAChangeOfAddressBesideAMainOwnDomainKeepsTheDomain(t *testing.T) {
	ctx := context.Background()
	seedDomainTenants(ctx, t)
	_, _, uow := realWriter(ctx, t)
	domains := postgres.NewOwnDomainRepository()
	const own = "kept.own-domain.example"
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		return domains.Insert(ctx, ownDomainRow(t, domainTenant, own))
	})
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE tenant_host SET state = 'ACTIVE' WHERE host = $1`, own); err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		_, err := domains.MainToOwnDomain(ctx)
		return err
	})
	row, err := domain.NewCanonicalHost(domain.NewCanonicalHostInput{
		TenantID: domainTenant, Host: "domain-acme-new.hub.example", Verification: "hbt-dv-integration", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, domainTenant, func(ctx context.Context) error {
		from, err := postgres.NewWorkspaceAddressRepository().ChangeAddress(ctx, "domain-acme-new", row, time.Now())
		if from != own {
			t.Errorf("replaced %q", from)
		}
		return err
	})
	var hosts []string
	rows, err := adminPool(ctx, t).Query(ctx,
		`SELECT host || ':' || is_canonical::text FROM tenant_host WHERE tenant_id = $1 ORDER BY host`, domainTenant.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, host)
	}
	want := []string{"domain-acme-new.hub.example:true", own + ":false"}
	if len(hosts) != 2 || hosts[0] != want[0] || hosts[1] != want[1] {
		t.Errorf("rows %v, want %v", hosts, want)
	}
}
