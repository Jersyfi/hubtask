// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// noEntryEdits stands for the catalogue: the pages below hold the only lines.
type noEntryEdits struct{}

func (noEntryEdits) Invoke(context.Context, string, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
	return usecase.Output{}, nil
}

// UC-PRV-01 check 13 against the real statements: a page's intake line is found by its body, its
// draft or a version, removed from each text that opens with it, the body written as the page's
// next version, and its live document erased - in this workspace alone.
func TestAPagesIntakeLineIsRemovedInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	pool := adminPool(ctx, t)
	address := "page-" + freshID(t).String()[28:] + "@shop.test"
	pages := map[shared.ID]shared.ID{}
	for tenant, author := range map[shared.ID]shared.ID{tenantA: authorA, tenantB: authorB} {
		collection := collectionFor(ctx, t, tenant, author)
		var hub string
		if err := pool.QueryRow(ctx, `SELECT parent_id::text FROM container WHERE id = $1`, collection.String()).Scan(&hub); err != nil {
			t.Fatalf("reading the hub: %v", err)
		}
		page := freshID(t)
		pages[tenant] = page
		at := time.Now().UTC()
		if _, err := pool.Exec(ctx, `INSERT INTO page (id, tenant_id, hub_id, title, body, order_key, created_by,
			created_at, updated_by, updated_at) VALUES ($1, $2, $3, 'P', $4, 'a', $5, $6, $5, $6)`,
			page.String(), tenant.String(), hub, "Our own words", author.String(), at); err != nil {
			t.Fatalf("writing a page: %v", err)
		}
		// The body no longer holds the line; the draft and the first version do.
		if _, err := pool.Exec(ctx, `INSERT INTO page_draft (tenant_id, page_id, title, body, updated_by, updated_at)
			VALUES ($1, $2, 'P', $3, $4, $5)`, tenant.String(), page.String(),
			"From: Erika <"+address+">\n\nDraft words", author.String(), at); err != nil {
			t.Fatalf("writing the draft: %v", err)
		}
		for seq, body := range map[int]string{1: "From: " + address + "\n\nFirst", 2: "Our own words"} {
			if _, err := pool.Exec(ctx, `INSERT INTO page_version (tenant_id, page_id, seq, title, body, author_id, created_at)
				VALUES ($1, $2, $3, 'P', $4, $5, $6)`, tenant.String(), page.String(), seq, body, author.String(), at); err != nil {
				t.Fatalf("writing a version: %v", err)
			}
		}
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			_, err := postgres.NewRealtimeRepository().Append(ctx, realtime.Ref{Kind: "page", ID: page}, []byte{1}, author, at)
			return err
		}); err != nil {
			t.Fatalf("writing the live document: %v", err)
		}
	}

	strip := privacyservice.StripIntakeLines{
		Lines: postgres.RequesterRepository{IDs: freshIDs{t}}, Entries: noEntryEdits{},
		Pages: postgres.IntakePageRepository{}, Documents: postgres.NewRealtimeRepository(),
	}
	t.Run("another workspace", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			if _, found, err := (postgres.IntakePageRepository{}).PageTexts(ctx, pages[tenantA]); err != nil || found {
				t.Errorf("another workspace read the page's texts (found %v, %v)", found, err)
			}
			if err := (postgres.IntakePageRepository{}).WritePageTexts(ctx, pages[tenantA], privacyrepo.IntakePageTexts{Body: "x"}); err == nil {
				t.Error("another workspace wrote the page's body")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	var lines []privacyrepo.IntakeLine
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if lines, err = (postgres.RequesterRepository{}).IntakeLinesFrom(ctx, []string{address}, privacyrepo.IntakeCursor{}, 500); err != nil {
			return err
		}
		changed, err := strip.Strip(ctx, tenantA, []string{address}, appshared.ActorContext{})
		if changed != 1 {
			t.Errorf("%d texts changed, want the one page", changed)
		}
		return err
	}); err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if len(lines) != 1 || !lines[0].Page || lines[0].ID != pages[tenantA] {
		t.Fatalf("the lines found are %+v, want this workspace's page", lines)
	}

	texts := func(page shared.ID) (body, draft, first string, version int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT p.body, d.body, v.body, p.version FROM page p
			JOIN page_draft d ON d.page_id = p.id JOIN page_version v ON v.page_id = p.id AND v.seq = 1
			WHERE p.id = $1`, page.String()).Scan(&body, &draft, &first, &version); err != nil {
			t.Fatalf("reading the page: %v", err)
		}
		return body, draft, first, version
	}
	if body, draft, first, version := texts(pages[tenantA]); body != "Our own words" || draft != "Draft words" ||
		first != "First" || version != 2 {
		t.Errorf("the page reads %q, draft %q, first version %q at version %d", body, draft, first, version)
	}
	if _, draft, first, version := texts(pages[tenantB]); draft == "Draft words" || first == "First" || version != 1 {
		t.Error("the other workspace's page was stripped")
	}
	heads := map[shared.ID]realtime.Head{}
	for tenant, page := range pages {
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			heads[tenant], err = postgres.NewRealtimeRepository().Head(ctx, realtime.Ref{Kind: "page", ID: page})
			return err
		}); err != nil {
			t.Fatalf("reading the live document: %v", err)
		}
	}
	if heads[tenantA].Seq != 0 || heads[tenantB].Seq == 0 {
		t.Errorf("the live documents stand at %+v, want this workspace's erased alone", heads)
	}
}
