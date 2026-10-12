// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// fingerprintSecret is this suite's installation secret: long enough for the configuration's
// floor, and plainly not a credential.
const fingerprintSecret = "integration-suite-installation-secret-for-audit-fingerprints"

// keyedSink is the sink as cmd/server builds it.
func keyedSink(t *testing.T) postgres.AuditSink {
	return postgres.NewAuditSink(generator{t}).Keyed(security.NewAuditFingerprinter(secret.New(fingerprintSecret)))
}

func appendWith(ctx context.Context, t *testing.T, sink postgres.AuditSink, tenant shared.ID, entries ...port.Entry) {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	for _, entry := range entries {
		if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant},
			func(ctx context.Context) error { return sink.Append(ctx, entry) }); err != nil {
			t.Fatalf("appending an audit entry: %v", err)
		}
	}
}

// storedChange reads one field of each stored entry's changes, in sequence order.
func storedChange(ctx context.Context, t *testing.T, tenant shared.ID, field string) []map[string]any {
	t.Helper()
	var found []map[string]any
	for _, entry := range storedEntries(ctx, t, tenant) {
		var changes map[string]map[string]any
		if err := json.Unmarshal(entry.changes, &changes); err != nil {
			t.Fatalf("reading stored changes %s: %v", entry.changes, err)
		}
		found = append(found, changes[field])
	}
	return found
}

func sensitiveEntry(tenant shared.ID, at time.Time, name string) port.Entry {
	entry := auditEntry(tenant, "container.created", name)
	entry.OccurredAt = at
	return entry
}

// audit.md §4: a sensitive value is stored as a keyed fingerprint that names its scheme - the same
// for one value in one workspace, different in another workspace, and never the value.
func TestTheTrailKeysItsFingerprintsPerWorkspace(t *testing.T) {
	ctx := context.Background()
	one, other := auditTenant(ctx, t), auditTenant(ctx, t)
	name := freshName(t)

	sink := keyedSink(t)
	appendWith(ctx, t, sink, one, sensitiveEntry(one, created, name), sensitiveEntry(one, created.Add(time.Second), name))
	appendWith(ctx, t, sink, other, sensitiveEntry(other, created, name))

	ones, others := storedChange(ctx, t, one, "name"), storedChange(ctx, t, other, "name")
	for _, change := range append(ones, others...) {
		fingerprint, _ := change["to_hash"].(string)
		if !strings.HasPrefix(fingerprint, security.AuditFingerprintScheme) {
			t.Errorf("a stored fingerprint is not keyed: %v", change)
		}
		if strings.Contains(fingerprint, name) || change["to"] != nil {
			t.Errorf("the value reached the trail: %v", change)
		}
	}
	if ones[0]["to_hash"] != ones[1]["to_hash"] {
		t.Error("one value in one workspace was stored with two fingerprints")
	}
	if ones[0]["to_hash"] == others[0]["to_hash"] {
		t.Error("two workspaces stored the same fingerprint for one value")
	}
}

// audit.md §3: a stored entry is never rewritten, and the change of how a fingerprint is computed
// changes nothing about the chain. An entry in the shape an earlier version stored (an unkeyed
// fingerprint, bare hex) followed by a keyed one is one chain, and it verifies across the boundary.
func TestTheChainVerifiesAcrossTheKeyedBoundary(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	legacyFingerprint := strings.Repeat("9f", 32)

	legacy := sensitiveEntry(tenant, created, "")
	legacy.Changes = map[string]any{"name": map[string]any{"changed": true, "to_hash": legacyFingerprint}}
	appendWith(ctx, t, postgres.NewAuditSink(generator{t}), tenant, legacy)
	appendWith(ctx, t, keyedSink(t), tenant, sensitiveEntry(tenant, created.Add(time.Second), freshName(t)))

	stored := storedChange(ctx, t, tenant, "name")
	if len(stored) != 2 || stored[0]["to_hash"] != legacyFingerprint {
		t.Fatalf("the earlier entry was not kept as stored: %v", stored)
	}
	if fingerprint, _ := stored[1]["to_hash"].(string); !strings.HasPrefix(fingerprint, security.AuditFingerprintScheme) {
		t.Fatalf("the later entry is not keyed: %v", stored[1])
	}

	found, err := verifierFor(t).Execute(ctx, auditActor(tenant), repository.Period{
		From: created.Add(-time.Hour), To: created.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !found.Valid || found.Checked != 2 || found.GapCount != 0 {
		t.Errorf("the chain does not verify across the boundary: %+v", found)
	}
}

// A sink built without a fingerprinter records that the field changed and nothing else: no
// fingerprint, and above all no unkeyed one.
func TestASinkWithoutAFingerprinterStoresNoFingerprint(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	name := freshName(t)

	appendWith(ctx, t, postgres.NewAuditSink(generator{t}), tenant, sensitiveEntry(tenant, created, name))

	entries := storedEntries(ctx, t, tenant)
	stored := string(entries[len(entries)-1].changes)
	if strings.Contains(stored, "_hash") || strings.Contains(stored, name) {
		t.Errorf("an unkeyed sink stored a fingerprint or the value: %s", stored)
	}
	if !strings.Contains(stored, `"changed": true`) {
		t.Errorf("the change itself is gone: %s", stored)
	}
}
