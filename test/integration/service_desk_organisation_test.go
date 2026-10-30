// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An organisation is stored, replaced by its version and found by a domain; a domain in another
// organisation of the workspace is refused by the database, and the workspace next door neither
// reads, finds nor deletes it, nor counts its people (migration 0287, UC-SVC-11).
func TestOrganisationsStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE account SET email = 'anna@acme.example' WHERE id = $1`, authorA.String()); err != nil {
		t.Fatal(err)
	}
	organisations := postgres.NewServiceDeskRepository().Organisations()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	acme, err := servicedesk.NewOrganisation(servicedesk.NewOrganisationInput{
		ID: freshID(t), TenantID: tenantA, Key: "acme", Name: "Acme",
		Domains: []string{"acme.example"}, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return organisations.Insert(ctx, acme) }); err != nil {
		t.Fatalf("insert: %v", err)
	}
	other := acme
	other.ID, other.Key, other.Domains = freshID(t), "other", []string{"other.example", "acme.example"}
	err = write(ctx, t, tenantA, func(ctx context.Context) error { return organisations.Insert(ctx, other) })
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "organisations.domain_taken" {
		t.Errorf("a domain of another organisation: %v, want organisations.domain_taken", err)
	}
	changed, err := acme.Change("Acme Ltd", []string{"acme.example", "acme.test"}, true, now, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return organisations.Update(ctx, changed, 1) }); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return organisations.Update(ctx, changed, 1) }); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("an update of an older version: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := organisations.OfDomain(ctx, "acme.test")
		if err != nil {
			return err
		}
		if found.Name != "Acme Ltd" || !found.SharesRequests || found.Version != 2 {
			t.Errorf("read back %+v", found)
		}
		members, err := organisations.Members(ctx, found.Domains, 10)
		if err == nil && !slices.Contains(members, authorA) {
			t.Errorf("the members are %v", members)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if listed, err := organisations.List(ctx); err != nil || len(listed) != 0 {
			t.Errorf("the other workspace lists %d organisations: %v", len(listed), err)
		}
		if _, err := organisations.OfDomain(ctx, "acme.example"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the organisation: %v", err)
		}
		if members, err := organisations.Members(ctx, []string{"acme.example"}, 10); err != nil || len(members) != 0 {
			t.Errorf("the other workspace counts %v: %v", members, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error { return organisations.Delete(ctx, "acme") }); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace deletes the organisation: %v", err)
	}
	theirs := acme
	theirs.ID, theirs.TenantID = freshID(t), tenantB
	if err := write(ctx, t, tenantB, func(ctx context.Context) error { return organisations.Insert(ctx, theirs) }); err != nil {
		t.Errorf("the same domain in another workspace: %v", err)
	}
}
