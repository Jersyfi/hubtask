// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

var (
	authorA = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a1")
	authorB = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a2")
	created = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

	// The whole package shares one database, so identifiers and names are handed out rather than
	// written down: two tests that both wrote "Private" into tenant A would fail on each other's
	// rows rather than on what they are testing.
	fixtureCounter atomic.Uint64
)

func freshID(t *testing.T) shared.ID {
	t.Helper()
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012x", fixtureCounter.Add(1)))
}

func freshName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("Fixture %d", fixtureCounter.Add(1))
}

// seedContainerTenants gives both tenants an account, so that created_by points at something
// plausible. The containers themselves are written through the repository under test.
func seedContainerTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'tenant-a', 'A'), ($2, 'tenant-b', 'B')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $3, 'Anna'), ($2, $4, 'Bert')
		ON CONFLICT (id) DO NOTHING`,
		authorA.String(), authorB.String(), tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding accounts: %v", err)
	}
}

func containerIn(tenant, author shared.ID, id shared.ID, name, orderKey string) work.Container {
	return work.Container{
		ID: id, TenantID: tenant, Type: work.ContainerHub, Name: name,
		OrderKey: orderKey, CreatedBy: author, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
}

// write runs fn in a read-write transaction for the tenant, which is the only way a repository
// method can be called at all.
func write(ctx context.Context, t *testing.T, tenant shared.ID, fn func(context.Context) error) error {
	t.Helper()
	return postgres.NewUnitOfWork(appPool(ctx, t)).
		Within(ctx, persistence.Scope{TenantID: tenant}, fn)
}

func read(ctx context.Context, t *testing.T, tenant shared.ID, fn func(context.Context) error) error {
	t.Helper()
	return postgres.NewUnitOfWork(appPool(ctx, t)).
		WithinReadOnly(ctx, persistence.Scope{TenantID: tenant}, fn)
}

func TestAContainerIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()

	id, name := freshID(t), freshName(t)
	hub := containerIn(tenantA, authorA, id, name, "a0")
	hub.Description, hub.Icon, hub.ColorToken = "Everything personal", "home", "blue"

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, hub)
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}

	var stored work.Container
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = repo.Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}

	if stored.Name != name || stored.Type != work.ContainerHub || stored.Version != 1 {
		t.Errorf("unexpected container: %+v", stored)
	}
	if !stored.ParentID.IsZero() {
		t.Errorf("a hub came back with a parent: %s", stored.ParentID)
	}
	if stored.TenantID != tenantA {
		t.Errorf("the row was written into tenant %s", stored.TenantID)
	}
	if stored.Description != "Everything personal" || stored.ColorToken != "blue" {
		t.Errorf("the optional fields did not survive: %+v", stored)
	}
	if stored.IsArchived() || stored.IsTrashed() {
		t.Errorf("a new container is archived or trashed: %+v", stored)
	}
	if !stored.CreatedAt.Equal(created) {
		t.Errorf("created at %v, want %v", stored.CreatedAt, created)
	}
}

// The cross-tenant negative test for Find (gate SG-3): a container is invisible from another
// tenant, and invisible is the same answer as absent.
func TestAContainerIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()

	id := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, id, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}

	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := repo.Find(ctx, id)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's hub: %v", err)
	}
}

// The cross-tenant negative test for Insert: the tenant comes from the transaction, not from the
// object, so a container built for another tenant lands in the caller's own - it cannot be
// smuggled across the boundary.
func TestInsertCannotWriteIntoAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()
	smuggled := freshID(t)

	// The object claims tenant A, the transaction belongs to tenant B.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorB, smuggled, freshName(t), "a1"))
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := repo.Find(ctx, smuggled)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the row landed in tenant A although the transaction was tenant B's: %v", err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := repo.Find(ctx, smuggled)
		return err
	}); err != nil {
		t.Fatalf("the row is not in the tenant that wrote it: %v", err)
	}
}

// The cross-tenant negative test for LastOrderKey: ranks are counted per tenant, so a busy
// neighbour cannot push a container to the end of somebody else's list.
func TestTheLastOrderKeyIsPerTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantB, authorB, freshID(t), freshName(t), "zz"))
	}); err != nil {
		t.Fatalf("writing tenant B's hub: %v", err)
	}

	var key string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		key, err = repo.LastOrderKey(ctx, "")
		return err
	}); err != nil {
		t.Fatalf("reading the last order key: %v", err)
	}
	if key == "zz" {
		t.Error("tenant A ranks its hubs after tenant B's")
	}
}

// An empty level answers with nothing rather than with an error - the first container at a level
// has nothing to sort after.
func TestAnEmptyLevelHasNoLastOrderKey(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()
	emptyHub := freshID(t)

	var key string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		key, err = repo.LastOrderKey(ctx, emptyHub)
		return err
	}); err != nil {
		t.Fatalf("reading the last order key: %v", err)
	}
	if key != "" {
		t.Errorf("an empty level answered %q", key)
	}
}

// The ranks the domain produces have to sort in the database the way they sort in Go: the whole
// point of a lexicographic key is that the index does the ordering.
func TestTheRanksSortInTheDatabaseAsTheyDoInTheDomain(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()
	parent := freshID(t)

	// The hub has to exist: a collection references it, and the database says so.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, parent, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}

	previous := ""
	for i := range 5 {
		key, err := service.OrderKeyAfter(previous)
		if err != nil {
			t.Fatalf("ranking: %v", err)
		}
		container := containerIn(tenantA, authorA, freshID(t), freshName(t), key)
		container.Type, container.ParentID = work.ContainerCollection, parent

		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return repo.Insert(ctx, container)
		}); err != nil {
			t.Fatalf("writing collection %d: %v", i, err)
		}
		previous = key
	}

	var last string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		last, err = repo.LastOrderKey(ctx, parent)
		return err
	}); err != nil {
		t.Fatalf("reading the last order key: %v", err)
	}
	if last != previous {
		t.Errorf("the database ranks %q last, the domain ranks %q last", last, previous)
	}
}

// The unique index is the name check, and it answers as a conflict rather than as a driver error.
func TestASecondContainerWithTheSameNameIsAConflict(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()
	first, second, name := freshID(t), freshID(t), freshName(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, first, name, "a0"))
	}); err != nil {
		t.Fatalf("writing the first hub: %v", err)
	}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, second, name, "a1"))
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
	if code := shared.AsError(err).DetailCode; code != "containers.name_taken" {
		t.Errorf("detail code %s, want containers.name_taken", code)
	}

	// The same name in another tenant is not a duplicate: uniqueness is per tenant and level.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantB, authorB, freshID(t), name, "a0"))
	}); err != nil {
		t.Fatalf("the name is taken across tenants: %v", err)
	}
}

// The name is stored NFC normalised, which is what makes the unique index see one name where a
// person sees one name - the composed and the decomposed spelling of "Übersicht".
func TestNamesAreNormalisedBeforeTheyAreCompared(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewContainerRepository()

	composed := "\u00dcbersicht"    // "Übersicht" with Ü as one code point
	decomposed := "U\u0308bersicht" // the same word with a combining diaeresis
	first := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f1")
	second := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f2")

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, first, decomposed, "a0"))
	}); err != nil {
		t.Fatalf("writing the first hub: %v", err)
	}

	var stored work.Container
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = repo.Find(ctx, first)
		return err
	}); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	if stored.Name != composed {
		t.Errorf("the stored name is %q, want the composed form", stored.Name)
	}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, second, composed, "a1"))
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("two spellings of one name were accepted as two names: %v", err)
	}
}
