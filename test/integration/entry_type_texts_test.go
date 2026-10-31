// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

var (
	textsTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe01")
	textsTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe02")
)

// A type's texts against the real boundary (UC-SVC-05 check 7, migration 0311, gate SG-3): its
// description and its texts per language are stored on its row and read back whole, and the
// workspace next door neither reads nor changes them.
func TestATypesTextsAreStoredWholeAndItsOwn(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + textsTenantA.String() + `', 'texts-a', 'Texts A') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + textsTenantB.String() + `', 'texts-b', 'Texts B') ON CONFLICT (id) DO NOTHING`,
		`DELETE FROM entry_type WHERE tenant_id IN ('` + textsTenantA.String() + `', '` + textsTenantB.String() + `')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	types := postgres.NewEntryTypeRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	incident, err := domain.NewEntryType(domain.NewEntryTypeInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe11"), TenantID: textsTenantA,
		Key: "incident", Level: domain.ItemTask, Name: "Incident", Icon: "alert", ColorToken: "red",
		Description:  "Something is broken",
		Translations: map[string]domain.EntryTypeText{"de": {Name: "Störung", Description: "Etwas ist kaputt"}},
		Behaviours:   domain.EntryBehaviours{Completable: true, Schedulable: true}, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	french := map[string]domain.EntryTypeText{"fr": {Name: "Panne"}}
	changed, _, err := incident.Changed(domain.EntryTypeChange{Translations: &french}, now, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}

	inTenant(t, uow, textsTenantA, func(ctx context.Context) error {
		if err := types.Insert(ctx, incident); err != nil {
			t.Fatalf("storing the type: %v", err)
		}
		read, err := types.Find(ctx, "incident")
		if err != nil || read.Description != "Something is broken" || read.TextIn("de-AT").Name != "Störung" ||
			read.TextIn("de").Description != "Etwas ist kaputt" {
			t.Errorf("the texts were not read back whole: %+v, %v", read, err)
		}
		if err := types.Update(ctx, changed, incident.Version); err != nil {
			t.Fatalf("storing the change: %v", err)
		}
		listed, err := types.List(ctx)
		if err != nil || len(listed) != 1 || listed[0].TextIn("fr").Name != "Panne" || listed[0].TextIn("de").Name != "Incident" {
			t.Errorf("the changed texts read %+v, %v", listed, err)
		}
		return nil
	})

	inTenant(t, uow, textsTenantB, func(ctx context.Context) error {
		if _, err := types.Find(ctx, "incident"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B read A's texts: %v", err)
		}
		if err := types.Update(ctx, changed, 2); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("B changed A's texts: %v", err)
		}
		return nil
	})
}
