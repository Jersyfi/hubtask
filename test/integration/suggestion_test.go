// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// What AI proposed, against the real boundary (J-05). Gate SG-3: one workspace's suggestions are
// invisible, unanswerable and unremovable next door - and the retention sweep, which runs with no
// actor behind it, is bounded by the same policy as everything else.

var (
	sugTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb01")
	sugTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb02")
	sugTargetA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb11")
	sugPersonA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb21")
	sugLabelA  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb41")
	sugBucketA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb42")
)

func seedSuggestionTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + sugTenantA.String() + `', 'sug-a', 'Suggestion A'),
		        ('` + sugTenantB.String() + `', 'sug-b', 'Suggestion B')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('` + sugPersonA.String() + `', '` + sugTenantA.String() + `', 'USER',
		         'ada@sug-a.example', 'Ada', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the suggestion tenants: %v", err)
		}
	}
}

func proposalFor(t *testing.T, id, tenantID shared.ID, at time.Time) domain.Suggestion {
	t.Helper()
	recorded, err := domain.New(domain.NewInput{
		ID: id, TenantID: tenantID,
		TargetType: domain.TargetWorkItem, TargetID: sugTargetA,
		Kind: domain.KindFields,
		// The titles a note implied travel in the payload beside the fields (K-01), so the
		// boundary is proved over the shape the product actually stores rather than over a
		// simpler one.
		Payload: map[string]any{
			"title":    "Buy oat milk",
			"subtasks": []any{"Check the fridge", "Walk to the shop"},
			// And what a classification chose from the sets it was shown (K-02): identifiers
			// rather than words, which is what makes them applicable at all.
			"label_ids": []any{sugLabelA.String()},
			"bucket_id": sugBucketA.String(),
			// And the values it proposed for the fields the collection declared (K-03), which are
			// a nested document inside the payload's own.
			"custom_fields": map[string]any{
				"priority": "high", "areas": []any{"kitchen"},
			},
		},
		Provenance: domain.Provenance{
			Model: "a-model", PromptID: "suggest-fields", PromptVersion: "v1",
			ProducedAt: at.Add(-time.Minute),
		},
		InputDigest: domain.Digest("Buy milk", ""),
		Now:         at,
	})
	if err != nil {
		t.Fatalf("building the suggestion: %v", err)
	}
	return recorded
}

func TestOneWorkspacesSuggestionsAreInvisibleNextDoor(t *testing.T) {
	ctx := context.Background()
	seedSuggestionTenants(ctx, t)

	suggestions := postgres.NewSuggestionRepository(security.NewCursorCodec(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	id := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb31")

	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		if err := suggestions.Record(ctx, proposalFor(t, id, sugTenantA, now)); err != nil {
			t.Fatalf("recording A's suggestion: %v", err)
		}
		return nil
	})

	// Gate SG-3: B sees nothing, finds nothing and cannot answer it.
	inTenant(t, uow, sugTenantB, func(ctx context.Context) error {
		if _, err := suggestions.Find(ctx, id); err == nil {
			t.Error("A's suggestion is readable next door")
		} else if !isNotFound(err) {
			t.Errorf("B's read answered %v, want not found", err)
		}

		page, err := suggestions.List(ctx, repository.Query{
			TargetType: domain.TargetWorkItem, TargetID: sugTargetA, Size: 10,
		})
		if err != nil {
			t.Fatalf("B's list: %v", err)
		}
		if len(page.Items) != 0 {
			t.Errorf("B sees %d of A's suggestions", len(page.Items))
		}

		// The decision is attempted with A's own row read from A's side, so the only thing
		// stopping it is the boundary.
		decided, err := proposalFor(t, id, sugTenantA, now).
			Decide(domain.StatusAccepted, sugPersonA, now)
		if err != nil {
			t.Fatalf("building the decision: %v", err)
		}
		written, err := suggestions.Decide(ctx, decided, 1)
		if err != nil {
			t.Fatalf("B's decide: %v", err)
		}
		if written {
			t.Error("B answered A's suggestion")
		}
		return nil
	})

	// And A's is still standing, unanswered.
	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		found, err := suggestions.Find(ctx, id)
		if err != nil {
			t.Fatalf("reading A's suggestion back: %v", err)
		}
		if found.Status != domain.StatusProposed {
			t.Errorf("status %q, want it still standing", found.Status)
		}
		if found.Model != "a-model" || found.PromptVersion != "v1" || found.ProducedAt.IsZero() {
			t.Errorf("the provenance did not survive the round trip: %+v", found.Provenance)
		}
		if !found.Fresh(domain.Digest("Buy milk", "")) {
			t.Error("the input digest did not survive the round trip, so nothing could judge staleness")
		}
		if found.Payload["title"] != "Buy oat milk" {
			t.Errorf("the payload came back as %v", found.Payload)
		}
		titles, held := found.Payload["subtasks"].([]any)
		if !held || len(titles) != 2 || titles[0] != "Check the fridge" {
			t.Errorf("the proposed titles came back as %v, and the order is what a person read",
				found.Payload["subtasks"])
		}
		chosen, held := found.Payload["label_ids"].([]any)
		if !held || len(chosen) != 1 || chosen[0] != sugLabelA.String() {
			t.Errorf("the chosen labels came back as %v", found.Payload["label_ids"])
		}
		if found.Payload["bucket_id"] != sugBucketA.String() {
			t.Errorf("the chosen column came back as %v", found.Payload["bucket_id"])
		}
		filled, held := found.Payload["custom_fields"].(map[string]any)
		if !held || filled["priority"] != "high" {
			t.Errorf("the proposed values came back as %v", found.Payload["custom_fields"])
		}
		if areas, listed := filled["areas"].([]any); !listed || len(areas) != 1 ||
			areas[0] != "kitchen" {
			t.Errorf("a multi-select value did not survive the round trip: %v", filled["areas"])
		}
		return nil
	})
}

// A suggestion about a *collection* (K-05) is the third target type, and it is bounded by the same
// policy as the other two: the row is invisible next door, and it round-trips with the target type
// the aggregate now carries.
func TestASuggestionAboutACollectionIsBoundedLikeEveryOther(t *testing.T) {
	ctx := context.Background()
	seedSuggestionTenants(ctx, t)

	suggestions := postgres.NewSuggestionRepository(security.NewCursorCodec(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	id := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb51")
	collection := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb52")

	recorded, err := domain.New(domain.NewInput{
		ID: id, TenantID: sugTenantA,
		TargetType: domain.TargetContainer, TargetID: collection,
		Kind:    domain.KindFields,
		Payload: map[string]any{"notes": "Two open, one overdue."},
		Provenance: domain.Provenance{
			Model: "a-model", PromptID: "summarize-collection", PromptVersion: "v1",
			ProducedAt: now.Add(-time.Minute),
		},
		InputDigest: domain.Digest("This quarter", ""),
		Now:         now,
	})
	if err != nil {
		t.Fatalf("building the summary: %v", err)
	}

	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		if err := suggestions.Record(ctx, recorded); err != nil {
			t.Fatalf("recording A's collection summary: %v", err)
		}
		return nil
	})

	inTenant(t, uow, sugTenantB, func(ctx context.Context) error {
		if _, err := suggestions.Find(ctx, id); err == nil {
			t.Error("A's collection summary is readable next door")
		} else if !isNotFound(err) {
			t.Errorf("B's read answered %v, want not found", err)
		}
		return nil
	})

	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		found, err := suggestions.Find(ctx, id)
		if err != nil {
			t.Fatalf("reading it back: %v", err)
		}
		if found.TargetType != domain.TargetContainer || found.TargetID != collection {
			t.Errorf("the target came back as %s / %s", found.TargetType, found.TargetID)
		}
		if found.Payload["notes"] != "Two open, one overdue." {
			t.Errorf("the payload came back as %v", found.Payload)
		}
		return nil
	})
}

// The retention sweep runs with no actor behind it, so the boundary is the only thing between one
// workspace's sweep and another's rows.
func TestTheSuggestionSweepStaysInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	seedSuggestionTenants(ctx, t)

	suggestions := postgres.NewSuggestionRepository(security.NewCursorCodec(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	old := now.Add(-90 * 24 * time.Hour)

	aOld := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb41")
	aNew := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb42")
	bOld := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb43")

	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		for id, at := range map[shared.ID]time.Time{aOld: old, aNew: now} {
			if err := suggestions.Record(ctx, proposalFor(t, id, sugTenantA, at)); err != nil {
				t.Fatalf("recording: %v", err)
			}
		}
		return nil
	})
	inTenant(t, uow, sugTenantB, func(ctx context.Context) error {
		return suggestions.Record(ctx, proposalFor(t, bOld, sugTenantB, old))
	})

	cutoff := now.Add(-30 * 24 * time.Hour)
	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		due, err := suggestions.CountExpired(ctx, cutoff, 100)
		if err != nil {
			t.Fatalf("counting: %v", err)
		}
		if due != 1 {
			t.Errorf("%d due in A, want A's one over suggestion - not B's, not the recent one", due)
		}
		removed, err := suggestions.DeleteExpired(ctx, cutoff, 100)
		if err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		if removed != 1 {
			t.Errorf("removed %d, want exactly A's over suggestion", removed)
		}
		if _, err := suggestions.Find(ctx, aNew); err != nil {
			t.Errorf("A's recent suggestion was swept: %v", err)
		}
		return nil
	})

	// B's is untouched, which is the whole claim.
	inTenant(t, uow, sugTenantB, func(ctx context.Context) error {
		if _, err := suggestions.Find(ctx, bOld); err != nil {
			t.Errorf("A's sweep took B's suggestion: %v", err)
		}
		// Clean up after itself: this package shares its database across files.
		if _, err := suggestions.DeleteExpired(ctx, now.Add(time.Hour), 100); err != nil {
			t.Fatalf("clearing up: %v", err)
		}
		return nil
	})
	inTenant(t, uow, sugTenantA, func(ctx context.Context) error {
		_, err := suggestions.DeleteExpired(ctx, now.Add(time.Hour), 100)
		return err
	})
}
