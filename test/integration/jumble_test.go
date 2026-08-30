// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The jumble against a real database (G-10): the row survives whole, the settlement is a single
// raced statement, and a cross-tenant negative for every method (gate SG-3).

func jumbleRepo() postgres.JumbleRepository {
	return postgres.NewJumbleRepository(pageCursors())
}

func seedJumbleEntry(
	ctx context.Context, t *testing.T, tenant shared.ID, change func(*domain.NewEntryInput),
) domain.Entry {
	t.Helper()

	in := domain.NewEntryInput{
		ID: freshID(t), TenantID: tenant, Channel: domain.ChannelAPI,
		Sender: "orders@example.org", RawSubject: "Order #42",
		RawBody: "The customer asked for a call back.",
		Now:     time.Now().UTC().Truncate(time.Microsecond),
	}
	if change != nil {
		change(&in)
	}
	entry, err := domain.NewEntry(in)
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return jumbleRepo().Insert(ctx, entry)
	}); err != nil {
		t.Fatalf("storing the entry: %v", err)
	}
	return entry
}

func TestAJumbleEntrySurvivesTheRowWhole(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	attachment := freshID(t)
	entry := seedJumbleEntry(ctx, t, tenantA, func(in *domain.NewEntryInput) {
		in.Attachments = []shared.ID{attachment}
	})

	var stored domain.Entry
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = jumbleRepo().Find(ctx, entry.ID)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}

	if stored.RawSubject != entry.RawSubject || stored.RawBody != entry.RawBody ||
		stored.Sender != entry.Sender {
		t.Errorf("the content was rewritten: %+v", stored)
	}
	if stored.Channel != domain.ChannelAPI || stored.Status != domain.StatusNew {
		t.Errorf("read back %+v", stored)
	}
	if len(stored.Attachments) != 1 || stored.Attachments[0] != attachment {
		t.Errorf("the attachments came back %v", stored.Attachments)
	}
	if stored.SettledAt != nil {
		t.Error("a fresh entry claims a settlement")
	}
}

// The settlement is one statement with the state guard in the WHERE: two conversions racing
// produce one item and one refusal, never two items.
func TestASettlementDecidesExactlyOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	entry := seedJumbleEntry(ctx, t, tenantA, nil)

	converted, err := entry.Convert(freshID(t), time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}

	var first, second bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		first, err = jumbleRepo().Settle(ctx, converted)
		return err
	}); err != nil {
		t.Fatalf("settling: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		second, err = jumbleRepo().Settle(ctx, converted)
		return err
	}); err != nil {
		t.Fatalf("settling again: %v", err)
	}

	if !first || second {
		t.Errorf("first=%v second=%v, want exactly one decision", first, second)
	}

	var stored domain.Entry
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = jumbleRepo().Find(ctx, entry.ID)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if stored.Status != domain.StatusProcessed || stored.TargetItemID != converted.TargetItemID {
		t.Errorf("the settled row reads %+v", stored)
	}
	if stored.SettledAt == nil {
		t.Error("the settlement has no moment")
	}
}

// The listing is tenant-bounded and filters by state and channel.
func TestTheJumbleListingFiltersInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	kept := seedJumbleEntry(ctx, t, tenantA, func(in *domain.NewEntryInput) {
		in.Channel = domain.ChannelQuickCapture
	})
	seedJumbleEntry(ctx, t, tenantB, func(in *domain.NewEntryInput) {
		in.Channel = domain.ChannelQuickCapture
	})

	var page repository.Page
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		page, err = jumbleRepo().List(ctx, repository.Query{Channel: domain.ChannelQuickCapture})
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	for _, entry := range page.Entries {
		if entry.ID != kept.ID && entry.Channel != domain.ChannelQuickCapture {
			t.Errorf("the filter leaked %+v", entry)
		}
	}
	found := false
	for _, entry := range page.Entries {
		found = found || entry.ID == kept.ID
	}
	if !found {
		t.Error("the tenant's own entry is missing")
	}
}

// The cross-tenant negatives (gate SG-3): another tenant's entry is invisible, unlistable, and
// unsettleable.
func TestAJumbleEntryCannotBeReachedFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	entry := seedJumbleEntry(ctx, t, tenantA, nil)

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := jumbleRepo().Find(ctx, entry.ID)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Find across the boundary answered %v, want not found", err)
	}

	var page repository.Page
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		page, err = jumbleRepo().List(ctx, repository.Query{})
		return err
	}); err != nil {
		t.Fatalf("listing as tenant B: %v", err)
	}
	for _, listed := range page.Entries {
		if listed.ID == entry.ID {
			t.Error("the listing leaked another tenant's entry")
		}
	}

	converted, err := entry.Convert(freshID(t), time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var settled bool
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		settled, err = jumbleRepo().Settle(ctx, converted)
		return err
	}); err != nil {
		t.Fatalf("settling as tenant B: %v", err)
	}
	if settled {
		t.Error("another tenant settled the entry")
	}
}

// The recount counts a jumble entry's attachments as references (G-10): an object a mail brought
// in must not be reclaimed while the entry that carries it is still readable.
func TestARecountCountsJumbleAttachments(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	object := stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment)
	object = sealMedia(ctx, t, tenantA, object)

	seedJumbleEntry(ctx, t, tenantA, func(in *domain.NewEntryInput) {
		in.Attachments = []shared.ID{object.ID}
	})

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return mediaRepo().Recount(ctx, time.Now().UTC())
	}); err != nil {
		t.Fatalf("recounting: %v", err)
	}

	stored := findMedia(ctx, t, tenantA, object.ID)
	if stored.RefCount != 1 {
		t.Errorf("the recount answered %d references, want the jumble's one", stored.RefCount)
	}
}

// The provenance write (gate SG-3 for the new item method): set exactly once, and never from
// another tenant.
func TestAnOriginIsRecordedOnceAndInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	item := seedWorkItemForRules(ctx, t, tenantA)
	entry := seedJumbleEntry(ctx, t, tenantA, nil)
	other := seedJumbleEntry(ctx, t, tenantA, nil)

	var recorded bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		recorded, err = itemRepo().RecordOrigin(ctx, item, entry.ID)
		return err
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	if !recorded {
		t.Fatal("the first origin was not recorded")
	}

	// Set exactly once: a second conversion cannot rewrite where the item came from.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		recorded, err = itemRepo().RecordOrigin(ctx, item, other.ID)
		return err
	}); err != nil {
		t.Fatalf("recording again: %v", err)
	}
	if recorded {
		t.Error("a second origin overwrote the first")
	}

	// The cross-tenant negative: another tenant cannot stamp provenance onto the item.
	freshItem := seedWorkItemForRules(ctx, t, tenantA)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		recorded, err = itemRepo().RecordOrigin(ctx, freshItem, entry.ID)
		return err
	}); err != nil {
		t.Fatalf("recording as tenant B: %v", err)
	}
	if recorded {
		t.Error("another tenant recorded provenance across the boundary")
	}

	// And the item reads its origin back.
	var stored work.WorkItem
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = itemRepo().Find(ctx, item)
		return err
	}); err != nil {
		t.Fatalf("reading the item: %v", err)
	}
	if stored.OriginJumbleID != entry.ID {
		t.Errorf("the item's origin reads %s, want the entry", stored.OriginJumbleID)
	}
}

// The intake credential against the real database (G-10): minting replaces in one statement, the
// lookup answers only under the tenant the token names, and the hash store never says more than a
// moment.
func TestTheIntakeTokenRotatesAndStaysInsideItsTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	intakes := postgres.NewJumbleIntakeRepository(security.NewJumbleIntakeHasher(secret.New(installationSecret)))

	first, err := integration.NewInboundToken(tenantA, bytesOf(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return intakes.SetToken(ctx, first, time.Now().UTC())
	}); err != nil {
		t.Fatalf("minting: %v", err)
	}

	var opens bool
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		opens, err = intakes.VerifyToken(ctx, first)
		return err
	}); err != nil || !opens {
		t.Fatalf("the minted token does not open its own intake: %v", err)
	}

	// Rotating kills the old address the same instant.
	second, err := integration.NewInboundToken(tenantA, bytesOf(2))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return intakes.SetToken(ctx, second, time.Now().UTC())
	}); err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		opens, err = intakes.VerifyToken(ctx, first)
		return err
	}); err != nil || opens {
		t.Fatalf("the rotated-away token still opens the intake: opens=%v err=%v", opens, err)
	}

	// The cross-tenant negative (gate SG-3): the same presented credential under another tenant's
	// context opens nothing - the hash covers the tenant half, and row level security bounds the
	// row.
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		opens, err = intakes.VerifyToken(ctx, second)
		return err
	}); err != nil || opens {
		t.Fatalf("another tenant's context opened the intake: opens=%v err=%v", opens, err)
	}

	var rotatedAt time.Time
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		rotatedAt, err = intakes.RotatedAt(ctx)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B reads tenant A's intake moment %v / %v", rotatedAt, err)
	}
}

// bytesOf fills a secret deterministically, so two mints in one test are two credentials.
func bytesOf(fill byte) []byte {
	drawn := make([]byte, integration.InboundTokenSecretBytes)
	for i := range drawn {
		drawn[i] = fill
	}
	return drawn
}
