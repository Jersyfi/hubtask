// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A value saved by its kind is stored as that kind (UC-INS-09 check 2): the document a screen sends,
// decoded as REST decodes it, read by the use case's own reading and written to the real table -
// and the row's JSON type read back, not the value the test wrote.
func TestAnInstanceValueIsStoredAsItsKind(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	instance := postgres.NewInstanceSettingRepository()

	read := func() identityrepo.InstanceLevel {
		var level identityrepo.InstanceLevel
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			var err error
			level, err = instance.Read(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading the level: %v", err)
		}
		return level
	}
	write := func(level identityrepo.InstanceLevel) {
		inSystem(ctx, t, uow, func(ctx context.Context) error {
			return instance.Write(ctx, level, "", time.Now().UTC())
		})
	}
	before := read()
	t.Cleanup(func() { write(before) })

	body := []byte(`{
		"sign_in": {
			"min_length": {"value": 14, "locked": true},
			"breach_check": {"value": true, "locked": false},
			"methods": {"value": ["PASSWORD"], "locked": false},
			"min_digits": {"set": false, "locked": false}
		},
		"localisation": {
			"locale": {"value": "de", "locked": false},
			"week_start": {"value": "SUNDAY", "locked": false}
		},
		"quotas": {"items": {"value": 500, "locked": true}}
	}`)
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	level, err := adminservice.InstanceLevelOf(document)
	if err != nil {
		t.Fatalf("reading the document: %v", err)
	}
	write(level)

	admin := adminPool(ctx, t)
	for key, want := range map[string]string{
		"sign_in.min_length":      "number",
		"sign_in.breach_check":    "boolean",
		"sign_in.methods":         "array",
		"localisation.locale":     "string",
		"localisation.week_start": "number",
		"quota.items":             "number",
	} {
		var kind string
		if err := admin.QueryRow(ctx,
			`SELECT jsonb_typeof(value) FROM instance_setting WHERE key = $1`, key).Scan(&kind); err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		if kind != want {
			t.Errorf("%s is stored as %s, want %s", key, kind, want)
		}
	}

	// The week start keeps its ISO number in the row, which a server one release older reads, and
	// comes back as the word every other level uses.
	var stored int
	if err := admin.QueryRow(ctx,
		`SELECT value::int FROM instance_setting WHERE key = 'localisation.week_start'`).Scan(&stored); err != nil {
		t.Fatalf("reading the week start's row: %v", err)
	}
	if stored != 7 {
		t.Errorf("the week start's row holds %d, want 7", stored)
	}
	got := read()
	if got.Localisation.WeekStart != identity.WeekStartSunday {
		t.Errorf("the week start reads %q", got.Localisation.WeekStart)
	}
	if got.Policy.Patch.MinDigits != nil {
		t.Error("an entry sent without a value was decided")
	}
}
