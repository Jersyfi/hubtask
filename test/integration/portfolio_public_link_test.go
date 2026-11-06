// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	tokens "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-GOL-03 check 15 against PostgreSQL (migration 0329): a roadmap's public link is found by its
// token, a new one replaces it, and the workspace next door neither finds nor removes it (SG-3).
func TestARoadmapsPublicLinkIsFoundByItsTokenInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewGoalRepository()
	links := postgres.NewPortfolioPublicLinkRepository(security.NewRoadmapLinkHasher(secret.New(feedSecret)))
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	portfolio := goal.Portfolio{ID: freshID(t), Name: "Shared roadmap", OwnerID: authorA, Status: goal.StatusOnTrack}
	mint := func(fill byte) tokens.RoadmapLinkToken {
		token, err := tokens.NewRoadmapLinkToken(tenantA, bytes.Repeat([]byte{fill}, tokens.RoadmapLinkTokenSecretBytes))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	first, second := mint(1), mint(2)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertPortfolio(ctx, portfolio, authorA, at); err != nil {
			return err
		}
		link := goal.RoadmapLink{PortfolioID: portfolio.ID, CreatedBy: authorA, CreatedAt: at}
		if err := links.Put(ctx, link, first); err != nil {
			return err
		}
		if found, err := links.FindByToken(ctx, first); err != nil || found.PortfolioID != portfolio.ID || found.CreatedBy != authorA {
			t.Errorf("by token %+v, %v", found, err)
		}
		if err := links.Put(ctx, link, second); err != nil {
			return err
		}
		if _, err := links.FindByToken(ctx, first); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the replaced token still answers: %v", err)
		}
		if _, found, err := links.Find(ctx, portfolio.ID); err != nil || !found {
			t.Errorf("find %v, %v", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := links.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door found the link: %v", err)
		}
		if _, found, _ := links.Find(ctx, portfolio.ID); found {
			t.Error("the workspace next door read the link")
		}
		if removed, _ := links.Delete(ctx, portfolio.ID); removed {
			t.Error("the workspace next door removed the link")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if removed, err := links.Delete(ctx, portfolio.ID); err != nil || !removed {
			t.Errorf("delete %v, %v", removed, err)
		}
		if _, err := links.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a removed link answers: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
