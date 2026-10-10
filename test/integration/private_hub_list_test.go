// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func privateHubList(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	registry, err := usecase.NewRegistry(nil, work.ListPrivateHubs{
		Hubs: postgres.NewPrivateHubRepository(), Policies: postgres.NewLifecycleRepository(),
		Authorizer: access.Service{
			Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork,
			Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)), Clock: fixed,
		},
		UnitOfWork: unitOfWork,
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// UC-ID-16 check 3: the workspace's owner sees the private hub - whose, how much - and never its
// name; a member is refused; another workspace sees none of it.
func TestAdministratorsSeeWhoseAndHowMuchWithoutTheName(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`, freshID(t).String(), w.tenant.String(), w.member.String(), w.hub.String()); err != nil {
		t.Fatal(err)
	}
	registry := privateHubList(ctx, t)
	owner := reader(w.tenant, w.administrator)
	owner.Scopes = []string{"members:read"}

	out, err := registry.Invoke(ctx, work.ListPrivateHubsName, owner, usecase.Input{})
	if err != nil {
		t.Fatalf("the owner was refused: %v", err)
	}
	var row usecase.Output
	for _, each := range out["data"].([]usecase.Output) {
		if each.String("id") == w.hub.String() {
			row = each
		}
	}
	if row == nil {
		t.Fatalf("the private hub is not in the list: %v", out)
	}
	// Written by hand from the seed: one collection, one entry, no attachment.
	if row["collections"] != 1 || row["entries"] != 1 || row["attachment_bytes"] != int64(0) {
		t.Errorf("the sizes are %v / %v / %v, want 1 / 1 / 0", row["collections"], row["entries"], row["attachment_bytes"])
	}
	owners, _ := row["owners"].([]any)
	found := false
	for _, each := range owners {
		found = found || each == w.member.String()
	}
	if !found {
		t.Errorf("the owners %v do not name the member who owns it", owners)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), w.title) {
		t.Error("the list carries what is inside the hub")
	}
	name := ""
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT name FROM container WHERE id = $1`, w.hub.String()).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), name) {
		t.Error("the list carries the hub's name")
	}

	member := reader(w.tenant, w.member)
	member.Scopes = []string{"members:read"}
	if _, err := registry.Invoke(ctx, work.ListPrivateHubsName, member, usecase.Input{}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member: %v, want forbidden", err)
	}

	stranger := reader(tenantB, authorB)
	stranger.Scopes = []string{"members:read"}
	seedMemberships(ctx, t) // tenant B's owner
	out, err = registry.Invoke(ctx, work.ListPrivateHubsName, stranger, usecase.Input{})
	if err != nil {
		t.Fatalf("another workspace's owner: %v", err)
	}
	encoded, _ = json.Marshal(out)
	if strings.Contains(string(encoded), w.hub.String()) {
		t.Error("another workspace's owner sees the private hub")
	}
}

// UC-ID-16 check 8: private hubs are offered where more than one person can act.
func TestPrivateHubsAreOfferedWhereThereIsMoreThanOnePerson(t *testing.T) {
	ctx := context.Background()
	tenant := freshID(t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'One person')`,
		tenant.String(), "one-"+strings.ReplaceAll(tenant.String(), "-", "")[20:]); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	count := func() int {
		var people int
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			people, err = postgres.NewPrivateHubRepository().CountPeople(ctx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return people
	}
	person := func(status string) shared.ID {
		id := freshID(t)
		if _, err := admin.Exec(ctx, `INSERT INTO account (id, tenant_id, display_name, status) VALUES ($1, $2, 'P', $3)`,
			id.String(), tenant.String(), status); err != nil {
			t.Fatalf("seeding a person: %v", err)
		}
		return id
	}

	person("ACTIVE")
	if count() != 1 {
		t.Fatalf("one person counts %d", count())
	}
	person("ANONYMIZED")
	if count() != 1 {
		t.Errorf("an anonymised account counted: %d", count())
	}
	person("ACTIVE")
	if count() != 2 {
		t.Errorf("two people count %d", count())
	}
}
