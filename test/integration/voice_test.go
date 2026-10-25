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
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Voice settings against a real database (ADR-0093): saved whole and read back, a name twice
// refused, and bounded by the workspace (gate SG-3).
func TestVoiceSettingsAreKeptPerWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewVoiceRepository()
	collection := freshID(t)
	settings := domain.Settings{
		Enabled: true, DefaultCollectionID: collection, SkillID: "amzn1.ask.skill.test",
		Aliases: []domain.Alias{{ID: freshID(t), Phrase: "Kühlschrank", CollectionID: collection}},
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Save(ctx, settings, authorA, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	var stored domain.Settings
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = repo.Find(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !stored.Enabled || stored.DefaultCollectionID != collection || stored.SkillID != settings.SkillID ||
		len(stored.Aliases) != 1 || stored.Aliases[0].Phrase != "Kühlschrank" {
		t.Errorf("read back %+v", stored)
	}

	var other domain.Settings
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		other, err = repo.Find(ctx)
		return err
	}); err != nil || other.Enabled || len(other.Aliases) != 0 {
		t.Errorf("another workspace read %+v, %v", other, err)
	}

	twice := settings
	twice.Aliases = []domain.Alias{
		{ID: freshID(t), Phrase: "Büro", CollectionID: collection},
		{ID: freshID(t), Phrase: "buero", CollectionID: collection},
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Save(ctx, twice, authorA, time.Now())
	}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a name twice answered %v", err)
	}
}
