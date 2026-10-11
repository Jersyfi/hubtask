// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// privateHub writes a private hub through the repository, as the application role.
func privateHub(ctx context.Context, t *testing.T, tenant, author shared.ID) shared.ID {
	t.Helper()
	seedContainerTenants(ctx, t)

	id := freshID(t)
	hub := containerIn(tenant, author, id, freshName(t), "a0")
	hub.Private = true
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, hub)
	}); err != nil {
		t.Fatalf("writing the private hub: %v", err)
	}
	return id
}

func storedContainer(ctx context.Context, t *testing.T, tenant, id shared.ID) work.Container {
	t.Helper()

	var container work.Container
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		container, err = containerRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading %s back: %v", id, err)
	}
	return container
}

// The flag is written and read back by the statements the application uses.
func TestAPrivateHubIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	id := privateHub(ctx, t, tenantA, authorA)

	if !storedContainer(ctx, t, tenantA, id).Private {
		t.Error("the hub came back shared")
	}

	sharedHub, _ := hubWithCollection(ctx, t, tenantA, authorA)
	if storedContainer(ctx, t, tenantA, sharedHub).Private {
		t.Error("a hub written without the flag came back private")
	}
}

// The column's CHECK holds for the application role, not only for the superuser that migrates: a
// collection cannot be private, whoever writes it.
func TestACollectionCannotBePrivateInStorage(t *testing.T) {
	ctx := context.Background()
	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)

	// Past the constructor, which refuses it first: the row's own guarantee is what is under test.
	collection := containerIn(tenantA, authorA, freshID(t), freshName(t), "a1")
	collection.Type = work.ContainerCollection
	collection.ParentID = hub
	collection.Private = true
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, collection)
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "container_private_hub_check" {
		t.Fatalf("a private collection was stored, or refused for another reason: %v", err)
	}
}

// Export and restore carry the mark (ADR-0073 §5), and an archive taken before the flag existed
// restores its hubs as they were: shared.
func TestTheArchiveCarriesTheMark(t *testing.T) {
	ctx := context.Background()
	id := privateHub(ctx, t, tenantA, authorA)

	row := exportedRowOf(ctx, t, tenantA, "container", id)
	if row["private"] != true {
		t.Fatalf("the export carries private = %#v, want true", row["private"])
	}

	restore := func(row map[string]any) {
		t.Helper()
		if _, err := adminPool(ctx, t).Exec(ctx,
			`DELETE FROM container WHERE tenant_id = $1 AND id = $2`,
			tenantA.String(), id.String()); err != nil {
			t.Fatalf("removing the hub: %v", err)
		}
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			_, err := importRepo().Write(ctx, "container", row, false)
			return err
		}); err != nil {
			t.Fatalf("restoring the hub: %v", err)
		}
	}

	restore(row)
	if !storedContainer(ctx, t, tenantA, id).Private {
		t.Error("the restore lost the mark")
	}

	older := make(map[string]any, len(row))
	for key, value := range row {
		if key != "private" {
			older[key] = value
		}
	}
	restore(older)
	if storedContainer(ctx, t, tenantA, id).Private {
		t.Error("a hub from an archive without the flag came back private")
	}
}
