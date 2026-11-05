// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// user-experience.md §3.6: a mark of the workspace's sample content is kept once per key, answers
// the mark a key holds, is unmarked by its key, and is invisible to the workspace next door
// (migration 0377).
func TestAWorkspaceSampleMarkIsKeptPerKeyAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	marks := postgres.SampleObjectRepository{}
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	hub, goal := freshID(t).String(), freshID(t).String()
	version := int64(3)
	key := "hub.k" + hub[len(hub)-8:]
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		held, err := marks.Mark(ctx, domain.SampleMark{Key: key, Kind: domain.SampleContainer, ObjectID: hub}, at)
		if err != nil || held.ObjectID != hub || held.Kind != domain.SampleContainer || held.Version != nil {
			t.Errorf("mark %+v, %v", held, err)
		}
		held, err = marks.Mark(ctx, domain.SampleMark{Key: key, Kind: domain.SampleGoal, ObjectID: goal, Version: &version}, at)
		if err != nil || held.ObjectID != hub || held.Kind != domain.SampleContainer {
			t.Errorf("a key marked twice answered %+v, %v", held, err)
		}
		held, err = marks.Mark(ctx, domain.SampleMark{Key: key + "g", Kind: domain.SampleGoal, ObjectID: goal, Version: &version}, at)
		if err != nil || held.Version == nil || *held.Version != 3 {
			t.Errorf("a versioned mark %+v, %v", held, err)
		}
		listed, err := marks.List(ctx)
		if err != nil || !holdsMark(listed, key) || !holdsMark(listed, key+"g") {
			t.Errorf("listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		listed, err := marks.List(ctx)
		if err != nil || holdsMark(listed, key) {
			t.Errorf("the workspace next door listed %+v, %v", listed, err)
		}
		// Unmarking another workspace's key there removes nothing of it.
		return marks.Unmark(ctx, key)
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		listed, err := marks.List(ctx)
		if err != nil || !holdsMark(listed, key) {
			t.Errorf("the workspace next door unmarked %s: %+v, %v", key, listed, err)
		}
		if err := marks.Unmark(ctx, key); err != nil {
			return err
		}
		listed, err = marks.List(ctx)
		if err != nil || holdsMark(listed, key) || !holdsMark(listed, key+"g") {
			t.Errorf("after unmarking %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func holdsMark(marks []domain.SampleMark, key string) bool {
	for _, m := range marks {
		if m.Key == key {
			return true
		}
	}
	return false
}
