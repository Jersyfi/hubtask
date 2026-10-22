// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Kudos against PostgreSQL as the application role (UC-WRK-29 checks 5 and 6): read by the person
// they were given to and by nobody else - the workspace's owner and an auditor included - written
// to the trail as an act naming nobody it went to, announced by no event; and another workspace
// reads and writes none. Tenant B; the catalogue is milestones_test.go's.

// seedPersonWithRole is an account in tenant B holding a role on the whole workspace.
func seedPersonWithRole(ctx context.Context, t *testing.T, role string) shared.ID {
	t.Helper()
	account := seedAccount(ctx, t, tenantB)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', $4)`,
		freshID(t).String(), tenantB.String(), account.String(), role); err != nil {
		t.Fatalf("granting %s: %v", role, err)
	}
	return account
}

func TestKudosAreSeenOnlyByWhomTheyWereGivenTo(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := celebrationCatalogue(ctx, t)
	anna := seedPersonWithRole(ctx, t, "MEMBER")
	ben := seedPersonWithRole(ctx, t, "MEMBER")
	auditor := seedPersonWithRole(ctx, t, "AUDITOR")
	_, collection := seedHubWithCollection(ctx, t, tenantB, authorB)
	entry := seedEntry(ctx, t, tenantB, collection.ID, entryRow{})

	// An open entry has no completion to acknowledge.
	if _, err := invokeAs(ctx, t, registry, "GiveKudos", celebrant(ben), entry.id); shared.AsError(err).DetailCode != domain.CodeKudosNotCompleted {
		t.Fatalf("kudos on an open entry: %v", err)
	}
	if _, err := invokeAs(ctx, t, registry, "CompleteWorkItem", celebrant(anna), entry.id); err != nil {
		t.Fatalf("Anna completing: %v", err)
	}
	if _, err := invokeAs(ctx, t, registry, "GiveKudos", celebrant(anna), entry.id); shared.AsError(err).DetailCode != domain.CodeKudosOwnCompletion {
		t.Fatalf("Anna's kudos to herself: %v", err)
	}

	for range 2 { // idempotent
		out, err := invokeAs(ctx, t, registry, "GiveKudos", celebrant(ben), entry.id)
		if err != nil || out["given"] != true {
			t.Fatalf("Ben giving kudos: %v %v", out, err)
		}
	}

	// Anna sees who; through the REST door, the contract's shape.
	status, body := restAs(ctx, t, registry, celebrant(anna), http.MethodGet, "/items/"+entry.id.String()+"/kudos")
	from, _ := body["from"].([]any)
	if status != http.StatusOK || len(from) != 1 || from[0].(map[string]any)["account_id"] != ben.String() {
		t.Fatalf("Anna reads: %d %v", status, body)
	}

	// Everybody else learns nothing of Anna's: the workspace's owner, and Ben beyond his own.
	for name, reader := range map[string]shared.ID{"the owner": authorB, "Ben": ben} {
		out, err := invokeAs(ctx, t, registry, "ReadKudos", celebrant(reader), entry.id)
		if err != nil {
			t.Fatalf("%s reading: %v", name, err)
		}
		if received, _ := out["from"].([]any); len(received) != 0 {
			t.Errorf("%s reads kudos given to Anna: %v", name, received)
		}
	}
	// An auditor reads no entry, and so no kudos (audit.md §5).
	if _, err := invokeAs(ctx, t, registry, "ReadKudos", celebrant(auditor), entry.id); err == nil {
		t.Error("an auditor read an entry's kudos")
	}

	// The trail holds the act once - the repeat changed nothing - with no changes naming anybody;
	// no event announces it.
	var acts int
	var changes string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*), coalesce(max(changes::text), '') FROM audit_log
		WHERE tenant_id = $1 AND action = 'kudos.given' AND target_id = $2`,
		tenantB.String(), entry.id.String()).Scan(&acts, &changes); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if acts != 1 || changes != "{}" {
		t.Errorf("the trail holds %d kudos acts with changes %s, want one with none", acts, changes)
	}
	var announced int
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*) FROM outbox_event WHERE tenant_id = $1 AND event_type LIKE '%kudos%'`,
		tenantB.String()).Scan(&announced); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	if announced != 0 {
		t.Errorf("%d events announce kudos", announced)
	}

	// Taken back, Anna's view is empty again.
	if out, err := invokeAs(ctx, t, registry, "TakeBackKudos", celebrant(ben), entry.id); err != nil || out["given"] != false {
		t.Fatalf("Ben taking back: %v %v", out, err)
	}
	out, err := invokeAs(ctx, t, registry, "ReadKudos", celebrant(anna), entry.id)
	if received, _ := out["from"].([]any); err != nil || len(received) != 0 {
		t.Fatalf("Anna reads after the take-back: %v %v", out, err)
	}
}

func TestAnotherWorkspaceReadsAndWritesNoKudos(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := celebrationCatalogue(ctx, t)
	ben := seedPersonWithRole(ctx, t, "MEMBER")
	_, collection := seedHubWithCollection(ctx, t, tenantB, authorB)
	entry := seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	if _, err := invokeAs(ctx, t, registry, "CompleteWorkItem", celebrant(authorB), entry.id); err != nil {
		t.Fatalf("completing: %v", err)
	}
	if _, err := invokeAs(ctx, t, registry, "GiveKudos", celebrant(ben), entry.id); err != nil {
		t.Fatalf("giving kudos: %v", err)
	}

	kudos := postgres.NewKudosRepository()
	// Tenant A's writes find nothing of tenant B's to remove; a foreign key refuses a row that would
	// name tenant B's entry under tenant A.
	_ = write(ctx, t, tenantA, func(ctx context.Context) error {
		taken, err := kudos.TakeBack(ctx, entry.id, ben, authorB)
		if err != nil || taken {
			t.Errorf("tenant A took back tenant B's kudos: %v %v", taken, err)
		}
		return nil
	})
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := kudos.Give(ctx, domain.Kudos{ItemID: entry.id, GivenBy: authorA, GivenTo: authorB, GivenAt: figureMoment})
		return err
	}); err == nil {
		t.Error("tenant A gave kudos on tenant B's entry")
	}
	_ = read(ctx, t, tenantA, func(ctx context.Context) error {
		if given, err := kudos.Given(ctx, entry.id, ben, authorB); err != nil || given {
			t.Errorf("tenant A reads tenant B's kudos given: %v %v", given, err)
		}
		if received, err := kudos.Received(ctx, entry.id, authorB); err != nil || len(received) != 0 {
			t.Errorf("tenant A reads tenant B's kudos received: %v %v", received, err)
		}
		return nil
	})

	// And tenant B's are where they were.
	out, err := invokeAs(ctx, t, registry, "ReadKudos", celebrant(authorB), entry.id)
	if received, _ := out["from"].([]any); err != nil || len(received) != 1 {
		t.Errorf("tenant B's kudos moved: %v %v", out, err)
	}
}

// An erasure that keeps the account (ANONYMIZE) takes the person's kudos on both sides, and leaves
// the kudos between two other people; a full deletion takes them with the account (PG-2).
func TestAnErasureTakesThePersonsKudosOnBothSides(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	subject, colleague, other := seedAccount(ctx, t, tenantB), seedAccount(ctx, t, tenantB), seedAccount(ctx, t, tenantB)
	_, collection := seedHubWithCollection(ctx, t, tenantB, authorB)
	entry := seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	kudos := postgres.NewKudosRepository()
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		for _, pair := range [][2]shared.ID{{subject, colleague}, {colleague, subject}, {colleague, other}} {
			if _, err := kudos.Give(ctx, domain.Kudos{
				ItemID: entry.id, GivenBy: pair[0], GivenTo: pair[1], GivenAt: figureMoment,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding kudos: %v", err)
	}

	var removed int
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		removed, err = postgres.NewPrivacyRepository(pageCursors()).DiscardKudos(ctx, subject)
		return err
	}); err != nil {
		t.Fatalf("discarding: %v", err)
	}
	if removed != 2 {
		t.Errorf("%d kudos removed, want the one given and the one received", removed)
	}
	_ = read(ctx, t, tenantB, func(ctx context.Context) error {
		if received, err := kudos.Received(ctx, entry.id, other); err != nil || len(received) != 1 {
			t.Errorf("kudos between two other people went too: %v %v", received, err)
		}
		if received, err := kudos.Received(ctx, entry.id, subject); err != nil || len(received) != 0 {
			t.Errorf("the person's kudos are still there: %v %v", received, err)
		}
		return nil
	})
}
