// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The audit tests use tenants of their own. The boundary tests in tenant_boundary_test.go assert
// exact row counts in audit_log for tenants A and B, and a trail that other tests keep appending
// to is not a trail those assertions can be written against.
var (
	tenantC = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000000c")
	tenantD = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000000d")
	authorC = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a3")
)

func seedAuditTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'tenant-c', 'C'), ($2, 'tenant-d', 'D')
		ON CONFLICT (id) DO NOTHING`, tenantC.String(), tenantD.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Cleo')
		ON CONFLICT (id) DO NOTHING`, authorC.String(), tenantC.String()); err != nil {
		t.Fatalf("seeding accounts: %v", err)
	}
}

// generator hands out identifiers a test can predict. The real one is UUIDv7 from the clock
// adapter; here the only requirement is that two entries do not collide.
type generator struct{ t *testing.T }

func (g generator) NewID() shared.ID { return freshID(g.t) }

func auditEntry(tenant shared.ID, action port.Action, name string) port.Entry {
	return port.Entry{
		TenantID: tenant, OccurredAt: created, Action: action,
		Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
		ActorKind: shared.ActorUser, ActorID: authorC, ActorLabel: "Cleo Beispiel",
		TargetType: "container",
		Changes: port.Changes(
			port.Change{Field: "name", Classification: port.Sensitive, To: name},
		),
	}
}

func appendEntries(ctx context.Context, t *testing.T, tenant shared.ID, entries ...port.Entry) {
	t.Helper()
	sink := postgres.NewAuditSink(generator{t})

	for _, entry := range entries {
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			return sink.Append(ctx, entry)
		}); err != nil {
			t.Fatalf("appending an audit entry: %v", err)
		}
	}
}

type storedEntry struct {
	seq      int64
	hash     []byte
	prevHash []byte
	action   string
	changes  []byte
}

func storedEntries(ctx context.Context, t *testing.T, tenant shared.ID) []storedEntry {
	t.Helper()
	admin := adminPool(ctx, t)

	rows, err := admin.Query(ctx, `
		SELECT seq, hash, prev_hash, action, changes::text
		FROM audit_log WHERE tenant_id = $1 ORDER BY seq`, tenant.String())
	if err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	defer rows.Close()

	var entries []storedEntry
	for rows.Next() {
		var entry storedEntry
		var changes string
		if err := rows.Scan(&entry.seq, &entry.hash, &entry.prevHash, &entry.action, &changes); err != nil {
			t.Fatalf("scanning the trail: %v", err)
		}
		entry.changes = []byte(changes)
		entries = append(entries, entry)
	}
	return entries
}

// The chain: gapless sequence numbers, and every entry pointing at the one before it (audit.md
// §3, test AT-2 in its first form - the full thousand-entry check arrives with `:verify`).
func TestTheTrailFormsAChainPerTenant(t *testing.T) {
	ctx := context.Background()
	seedAuditTenants(ctx, t)

	before := storedEntries(ctx, t, tenantC)
	appendEntries(ctx, t, tenantC,
		auditEntry(tenantC, "container.created", freshName(t)),
		auditEntry(tenantC, "container.renamed", freshName(t)),
		auditEntry(tenantC, "container.archived", freshName(t)))

	entries := storedEntries(ctx, t, tenantC)
	if len(entries) != len(before)+3 {
		t.Fatalf("%d entries, want %d", len(entries), len(before)+3)
	}

	for i, entry := range entries {
		if entry.seq != int64(i+1) {
			t.Errorf("entry %d carries sequence number %d - the chain has a gap", i, entry.seq)
		}
		if i == 0 {
			if entry.prevHash != nil {
				t.Error("the first entry of a chain points at a predecessor")
			}
			continue
		}
		if !bytes.Equal(entry.prevHash, entries[i-1].hash) {
			t.Errorf("entry %d does not point at its predecessor", i)
		}
	}
}

// Two tenants, two chains. A shared sequence would leak how busy a neighbour is, and a gap in
// one tenant's numbering is exactly what verification reports as tampering.
func TestTheChainsOfTwoTenantsAreIndependent(t *testing.T) {
	ctx := context.Background()
	seedAuditTenants(ctx, t)

	appendEntries(ctx, t, tenantC, auditEntry(tenantC, "container.created", freshName(t)))
	appendEntries(ctx, t, tenantD, auditEntry(tenantD, "container.created", freshName(t)))
	appendEntries(ctx, t, tenantD, auditEntry(tenantD, "container.renamed", freshName(t)))

	inC, inD := storedEntries(ctx, t, tenantC), storedEntries(ctx, t, tenantD)
	if len(inC) == 0 || len(inD) == 0 {
		t.Fatal("one of the two tenants has no entries at all")
	}
	if inD[0].seq != 1 {
		t.Errorf("the second tenant's chain starts at %d - it is counting the first one's entries", inD[0].seq)
	}
	if inD[0].prevHash != nil {
		t.Error("the second tenant's first entry chains onto the first one's")
	}
}

// The cross-tenant negative test for Append: an entry that names another tenant is refused rather
// than written into the caller's chain under a foreign identity.
func TestAnEntryForAnotherTenantIsRefused(t *testing.T) {
	ctx := context.Background()
	seedAuditTenants(ctx, t)
	sink := postgres.NewAuditSink(generator{t})

	err := write(ctx, t, tenantD, func(ctx context.Context) error {
		return sink.Append(ctx, auditEntry(tenantC, "container.created", freshName(t)))
	})
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("error %v, want the entry to be refused", err)
	}
	if code := shared.AsError(err).DetailCode; code != "audit.tenant_mismatch" {
		t.Errorf("detail code %s, want audit.tenant_mismatch", code)
	}
}

// Test AT-1: the application role may add to the trail and may not touch what is in it.
func TestTheApplicationRoleCannotChangeTheTrail(t *testing.T) {
	ctx := context.Background()
	seedAuditTenants(ctx, t)
	appendEntries(ctx, t, tenantC, auditEntry(tenantC, "container.created", freshName(t)))

	app := appPool(ctx, t)
	if _, err := app.Exec(ctx, `SELECT set_config('app.tenant_id', $1, false)`, tenantC.String()); err != nil {
		t.Fatalf("setting the tenant context: %v", err)
	}

	for _, statement := range []string{
		`UPDATE audit_log SET action = 'tampered' WHERE tenant_id = $1`,
		`DELETE FROM audit_log WHERE tenant_id = $1`,
	} {
		if _, err := app.Exec(ctx, statement, tenantC.String()); err == nil {
			t.Errorf("the application role could run: %s", statement)
		}
	}
}

// Test AT-4 at the boundary: what is written carries no readable user content.
func TestTheStoredChangesCarryNoContent(t *testing.T) {
	ctx := context.Background()
	seedAuditTenants(ctx, t)
	name := freshName(t)

	appendEntries(ctx, t, tenantC, auditEntry(tenantC, "container.created", name))

	entries := storedEntries(ctx, t, tenantC)
	last := entries[len(entries)-1]
	if bytes.Contains(last.changes, []byte(name)) {
		t.Errorf("the trail stored the name in clear text: %s", last.changes)
	}
	if !bytes.Contains(last.changes, []byte("to_hash")) {
		t.Errorf("the trail stored no fingerprint either: %s", last.changes)
	}
}
