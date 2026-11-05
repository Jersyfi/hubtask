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

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An organisation is stored, replaced by its version and found by a domain; a domain in another
// organisation of the workspace is refused by the database, and the workspace next door neither
// reads, finds nor deletes it (migration 0287, UC-SVC-11).
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
	changed, err := acme.Change("Acme Ltd", []string{"acme.example", "acme.test"}, true, now, text.Composing{}, nil)
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
		return nil
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

// UC-SVC-11 check 3: an organisation's list reads the requests of everybody at its domains, however
// many they are - the domains are joined in the query, not read as a bounded list of accounts - and
// not those of somebody elsewhere, nor anything in the workspace next door.
func TestAnOrganisationsListReadsEveryColleague(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	domain := "big-" + freshID(t).String()[24:] + ".example"

	// More people than the 2,000 the list once read; the one who raised the request sorts last.
	const people = 2_100
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		SELECT gen_random_uuid(), $1, 'USER', 'p' || n || '@' || $2, 'P' || n, 'ACTIVE'
		FROM generate_series(1, $3::int) AS n`, tenantA.String(), domain, people); err != nil {
		t.Fatalf("seeding the organisation's people: %v", err)
	}
	var last, outsider string
	if err := admin.QueryRow(ctx, `SELECT max(id::text) FROM account WHERE email LIKE '%@' || $1`, domain).Scan(&last); err != nil {
		t.Fatal(err)
	}
	outsider = freshID(t).String()
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'someone@elsewhere-' || $3, 'Elsewhere', 'ACTIVE')`,
		outsider, tenantA.String(), domain); err != nil {
		t.Fatal(err)
	}

	collection := collectionFor(ctx, t, tenantA, authorA)
	colleagues := seedTask(ctx, t, tenantA, shared.MustParseID(last), collection)
	own := seedTask(ctx, t, tenantA, authorA, collection)
	elsewhere := seedTask(ctx, t, tenantA, shared.MustParseID(outsider), collection)

	search := searchIn(t, collection, nil, view.Spec{})
	search.Spec.Filter = nil
	search.RaisedFrom = &repository.Raisers{Account: authorA, Domains: []string{domain}}
	got := idsOfItems(queried(ctx, t, tenantA, search).Items)
	if !slices.Contains(got, colleagues) || !slices.Contains(got, own) {
		t.Errorf("the list missed a colleague's or the reader's own request: %v", got)
	}
	if slices.Contains(got, elsewhere) {
		t.Errorf("the list read the request of somebody at another domain")
	}

	theirs := searchIn(t, collection, nil, view.Spec{})
	theirs.Spec.Filter = nil
	theirs.RaisedFrom = &repository.Raisers{Account: authorB, Domains: []string{domain}}
	if listed := queried(ctx, t, tenantB, theirs).Items; len(listed) != 0 {
		t.Errorf("the workspace next door read %d requests", len(listed))
	}
}
