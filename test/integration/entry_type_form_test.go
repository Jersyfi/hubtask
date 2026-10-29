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
	formTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000ff01")
	formTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000ff02")
)

// A type's form against the real boundary (UC-FLW-01 check 2, migration 0214, gate SG-3): it is
// stored on the type's row and read back whole, and the workspace next door neither reads nor
// changes it.
func TestATypesFormIsStoredWholeAndItsOwn(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + formTenantA.String() + `', 'form-a', 'Forms A') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + formTenantB.String() + `', 'form-b', 'Forms B') ON CONFLICT (id) DO NOTHING`,
		`DELETE FROM entry_type WHERE tenant_id IN ('` + formTenantA.String() + `', '` + formTenantB.String() + `')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	types := postgres.NewEntryTypeRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	bug, err := domain.NewEntryType(domain.NewEntryTypeInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000ff11"), TenantID: formTenantA,
		Key: "bug", Level: domain.ItemTask, Name: "Bug", Icon: "bug", ColorToken: "red",
		Behaviours: domain.EntryBehaviours{Completable: true, Schedulable: true}, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	form, err := domain.NewForm(domain.Form{
		Sections: []domain.FormSection{{Key: "triage", Name: "Triage"}},
		Fields: []domain.FormField{
			{Key: "custom_fields.severity", Section: "triage", Required: true, Default: "low"},
			{Key: "due_date", RequiredWhen: `entry.custom_fields.severity == "high"`},
		},
		Validations: []domain.FormValidation{{Key: "named", Field: "custom_fields.severity", Holds: "true", Message: "Name it"}},
		Channels: []domain.FormChannelRule{{Channel: domain.ChannelMail, Closed: true,
			Hidden: []string{"custom_fields.severity"}, Presets: map[string]any{"custom_fields.severity": "low"}}},
	}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	formed, _ := bug.WithForm(form, now)

	inTenant(t, uow, formTenantA, func(ctx context.Context) error {
		if err := types.Insert(ctx, bug); err != nil {
			t.Fatalf("storing the type: %v", err)
		}
		if read, err := types.Find(ctx, "bug"); err != nil || !read.Form.IsEmpty() {
			t.Errorf("a type stored without a form has one: %+v, %v", read.Form, err)
		}
		if err := types.Update(ctx, formed, bug.Version); err != nil {
			t.Fatalf("storing the form: %v", err)
		}
		read, err := types.Find(ctx, "bug")
		if err != nil {
			t.Fatal(err)
		}
		if len(read.Form.Fields) != 2 || read.Form.Fields[1].RequiredWhen != form.Fields[1].RequiredWhen ||
			read.Form.Fields[0].Default != "low" || !read.Form.Channel(domain.ChannelMail).Closed ||
			read.Form.Channel(domain.ChannelMail).Presets["custom_fields.severity"] != "low" ||
			read.Form.Validations[0].Message != "Name it" || read.Form.Sections[0].Name != "Triage" {
			t.Errorf("the form was not read back whole: %+v", read.Form)
		}
		return nil
	})

	inTenant(t, uow, formTenantB, func(ctx context.Context) error {
		if _, err := types.Find(ctx, "bug"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B read A's form: %v", err)
		}
		if err := types.Update(ctx, formed, 2); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("B changed A's form: %v", err)
		}
		return nil
	})
}
