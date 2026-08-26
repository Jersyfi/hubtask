// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Placing and lifting a legal hold against a real database (E-08): the lifting that happens once,
// and the boundary each method may not cross (gate SG-3). A hold read across the boundary would not
// be a wrong answer but somebody else's obligation ignored — or, worse, lifted.

func holdRepo() postgres.LegalHoldRepository { return postgres.NewLegalHoldRepository() }

func holdIn(t *testing.T, tenant shared.ID, scope domain.HoldScope, scopeID shared.ID) domain.LegalHold {
	t.Helper()
	hold, err := domain.NewLegalHold(domain.NewHoldInput{
		ID: freshID(t), Scope: scope, ScopeID: scopeID,
		Reason: "Pending litigation, ref. 4 O 128/26", PlacedBy: authorFor(tenant),
		Now: created,
	})
	if err != nil {
		t.Fatalf("building the hold: %v", err)
	}
	return hold
}

func authorFor(tenant shared.ID) shared.ID {
	if tenant == tenantB {
		return authorB
	}
	return authorA
}

func TestAHoldIsPlacedAndReadBackWithBothEnds(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	hold := holdIn(t, tenantA, domain.HoldContainer, collection)

	var found domain.LegalHold
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := holdRepo().Place(ctx, hold); err != nil {
			return err
		}
		var err error
		found, err = holdRepo().Find(ctx, hold.ID)
		return err
	}); err != nil {
		t.Fatalf("placing: %v", err)
	}

	switch {
	case found.Scope != domain.HoldContainer || found.ScopeID != collection:
		t.Errorf("the hold covers %s %s", found.Scope, found.ScopeID)
	case found.PlacedBy != authorA:
		t.Errorf("it was placed by %s", found.PlacedBy)
	case found.Released():
		t.Error("a new hold came back released")
	}

	// And it is in force, which is what every deletion path reads.
	var active domain.Holds
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		active, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the active holds: %v", err)
	}
	if !containsHold(active, hold.ID) {
		t.Fatal("a placed hold is not among the ones in force")
	}
}

func containsHold(holds domain.Holds, id shared.ID) bool {
	for _, hold := range holds {
		if hold.ID == id {
			return true
		}
	}
	return false
}

// The guard is the statement: two requests lifting one hold cannot both succeed, or the second
// would overwrite who lifted it and when.
func TestAHoldIsLiftedOnce(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	hold := holdIn(t, tenantA, domain.HoldContainer, collection)

	var first, second bool
	var found domain.LegalHold
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := holdRepo().Place(ctx, hold); err != nil {
			return err
		}
		released, err := hold.Release(authorA, "The proceedings ended", created.Add(time.Hour))
		if err != nil {
			return err
		}
		if first, err = holdRepo().Release(ctx, released); err != nil {
			return err
		}

		// A second lifting, by somebody else, with a different reason: the statement refuses it
		// rather than overwriting the record.
		again := released
		again.ReleasedBy, again.ReleasedReason = authorB, "Something else"
		if second, err = holdRepo().Release(ctx, again); err != nil {
			return err
		}
		found, err = holdRepo().Find(ctx, hold.ID)
		return err
	}); err != nil {
		t.Fatalf("lifting: %v", err)
	}

	if !first {
		t.Error("the first lifting did nothing")
	}
	if second {
		t.Error("a hold was lifted twice")
	}
	if found.ReleasedBy != authorA || found.ReleasedReason != "The proceedings ended" {
		t.Fatalf("the record says %s / %q", found.ReleasedBy, found.ReleasedReason)
	}
	// And a lifted hold is no longer in force, which is what makes lifting mean anything.
	var active domain.Holds
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		active, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the active holds: %v", err)
	}
	if containsHold(active, hold.ID) {
		t.Error("a lifted hold is still in force")
	}
}

// The listing answers what is frozen now, and the lifted ones when asked - because a hold that has
// been lifted is what shows an auditor that it was.
func TestTheListingShowsLiftedHoldsOnlyWhenAskedFor(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	lifted := holdIn(t, tenantA, domain.HoldContainer, collection)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := holdRepo().Place(ctx, lifted); err != nil {
			return err
		}
		released, err := lifted.Release(authorA, "Done", created.Add(time.Hour))
		if err != nil {
			return err
		}
		_, err = holdRepo().Release(ctx, released)
		return err
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	var inForce, all []domain.LegalHold
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if inForce, err = holdRepo().List(ctx, false); err != nil {
			return err
		}
		all, err = holdRepo().List(ctx, true)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	if containsHold(inForce, lifted.ID) {
		t.Error("a lifted hold is in the default listing")
	}
	if !containsHold(all, lifted.ID) {
		t.Error("a lifted hold is missing from the full listing")
	}
}

// Gate SG-3, and the sharpest case for it: a hold read across the boundary would not be a wrong
// answer but somebody else's obligation ignored, and one *lifted* across it would be somebody
// else's obligation removed.
func TestAHoldIsInvisibleAndInertFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantB, authorB)
	theirs := holdIn(t, tenantB, domain.HoldContainer, collection)

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return holdRepo().Place(ctx, theirs)
	}); err != nil {
		t.Fatalf("seeding B's hold: %v", err)
	}

	var find error
	var listed []domain.LegalHold
	var lifted bool
	var active domain.Holds
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, find = holdRepo().Find(ctx, theirs.ID)
		var err error
		if listed, err = holdRepo().List(ctx, true); err != nil {
			return err
		}
		released, err := theirs.Release(authorA, "Not mine to lift", created.Add(time.Hour))
		if err != nil {
			return err
		}
		if lifted, err = holdRepo().Release(ctx, released); err != nil {
			return err
		}
		active, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading from A: %v", err)
	}

	if !errors.Is(find, shared.ErrNotFound) {
		t.Errorf("tenant A found tenant B's hold: %v", find)
	}
	if containsHold(listed, theirs.ID) {
		t.Error("tenant A listed tenant B's hold")
	}
	if lifted {
		t.Error("tenant A lifted tenant B's hold")
	}
	if containsHold(active, theirs.ID) {
		t.Error("tenant B's hold is in force for tenant A")
	}

	// And it is still in force where it belongs.
	var stillHeld domain.Holds
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		stillHeld, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading from B: %v", err)
	}
	if !containsHold(stillHeld, theirs.ID) {
		t.Fatal("tenant A's attempt lifted tenant B's hold after all")
	}
}
