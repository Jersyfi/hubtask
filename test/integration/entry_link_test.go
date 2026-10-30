// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Links and references against the real boundary (UC-PLN-01 checks 1, 2, 5 and 7, gate SG-3): a
// workspace draws links between its entries and walks them for a loop, a type in use is held, the
// references its texts hold are kept by the database - and the workspace next door neither reads
// nor removes nor walks any of it.
func TestAWorkspacesLinksAreItsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	one := seedTask(ctx, t, tenantA, authorA, collection)
	two := seedTask(ctx, t, tenantA, authorA, collection)
	three := seedTask(ctx, t, tenantA, authorA, collection)

	for _, statement := range []string{
		`DELETE FROM entry_link WHERE id::text LIKE '01936f2a-7c1e-7000-8000-00000000fd%'`,
		`DELETE FROM link_type WHERE tenant_id = '` + tenantA.String() + `' AND key = 'verifies'`,
		`DELETE FROM comment WHERE id = '01936f2a-7c1e-7000-8000-00000000fd21'`,
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement); err != nil {
			t.Fatalf("clearing an earlier run: %v", err)
		}
	}

	types := postgres.NewLinkTypeRepository()
	links := postgres.NewEntryLinkRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	registry := domain.NewLinkTypes(nil)
	dependsOn, _ := registry.Find(domain.LinkTypeDependsOn)

	tests, err := domain.NewLinkType(domain.NewLinkTypeInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd01"), TenantID: tenantA,
		Key: "verifies", Name: "verifies", InverseName: "is verified by", Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	draw := func(id string, linkType domain.LinkType, source, target shared.ID) domain.EntryLink {
		link, err := domain.NewEntryLink(domain.NewEntryLinkInput{
			ID: shared.MustParseID(id), TenantID: tenantA, Type: linkType,
			SourceID: source, TargetID: target, CreatedBy: authorA, Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return link
	}
	first := draw("01936f2a-7c1e-7000-8000-00000000fd11", dependsOn, one, two)
	second := draw("01936f2a-7c1e-7000-8000-00000000fd12", dependsOn, two, three)
	own := draw("01936f2a-7c1e-7000-8000-00000000fd13", tests, three, one)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := types.Insert(ctx, tests); err != nil {
			t.Fatalf("storing A's link type: %v", err)
		}
		if err := links.Lock(ctx); err != nil {
			t.Fatalf("locking: %v", err)
		}
		for _, link := range []domain.EntryLink{first, second, own} {
			if err := links.Insert(ctx, link); err != nil {
				t.Fatalf("drawing %s: %v", link.Type, err)
			}
		}
		return nil
	})
	// A refused write aborts its transaction, so each is a transaction of its own and its answer.
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		again := first
		again.ID = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd14")
		return links.Insert(ctx, again)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a link was drawn twice: %v", err)
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return types.Delete(ctx, "verifies")
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a link type in use was deleted: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// three -> one would close one -> two -> three.
		sourceWaits, targetWaits := registry.Waiting()
		if loops, err := links.PathExists(ctx, one, three, sourceWaits, targetWaits); err != nil || !loops {
			t.Errorf("the walk did not reach: %v, %v", loops, err)
		}
		if loops, err := links.PathExists(ctx, three, one, sourceWaits, targetWaits); err != nil || loops {
			t.Errorf("the walk reached against the links: %v, %v", loops, err)
		}
		if of, err := links.Of(ctx, one, 50); err != nil || len(of) != 2 {
			t.Errorf("one's links from both ends: %v, %v", of, err)
		}
		if count, err := links.CountFrom(ctx, one, domain.LinkTypeDependsOn); err != nil || count != 1 {
			t.Errorf("one starts %d depends_on links, %v", count, err)
		}
		if summaries, err := links.Entries(ctx, []shared.ID{one, two}); err != nil || len(summaries) != 2 || summaries[0].CollectionID != collection {
			t.Errorf("the summaries: %+v, %v", summaries, err)
		}
		return nil
	})

	// The texts that point: the notes of one mention two as a card, a comment on three mentions two.
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`UPDATE work_item SET notes = 'see ![](hubtask:entry/` + two.String() + `) and [me](hubtask:entry/` + one.String() + `)' WHERE id = '` + one.String() + `'`,
		`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ('01936f2a-7c1e-7000-8000-00000000fd21', '` +
			tenantA.String() + `', '` + three.String() + `', '` + authorA.String() + `', 'waits for [x](hubtask:entry/` + two.String() + `#plan)')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the texts: %v", err)
		}
	}
	target := reference.Reference{Kind: reference.KindEntry, ID: two}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		backlinks, err := links.Backlinks(ctx, target, 50)
		if err != nil || len(backlinks) != 2 {
			t.Fatalf("two's backlinks: %+v, %v", backlinks, err)
		}
		for _, backlink := range backlinks {
			switch backlink.SourceKind {
			case "ENTRY":
				if backlink.EntryID != one || backlink.Strength != reference.Card {
					t.Errorf("the notes' backlink: %+v", backlink)
				}
			case "COMMENT":
				if backlink.EntryID != three || backlink.Anchor != "plan" || backlink.Strength != reference.Mention {
					t.Errorf("the comment's backlink: %+v", backlink)
				}
			}
		}
		// An entry's notes naming itself are no backlink of it.
		if self, err := links.Backlinks(ctx, reference.Reference{Kind: reference.KindEntry, ID: one}, 50); err != nil || len(self) != 0 {
			t.Errorf("one's notes are a backlink of one: %+v, %v", self, err)
		}
		return nil
	})
	if _, err := admin.Exec(ctx, `UPDATE comment SET deleted_at = now(), body = '' WHERE id = '01936f2a-7c1e-7000-8000-00000000fd21'`); err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if backlinks, err := links.Backlinks(ctx, target, 50); err != nil || len(backlinks) != 1 {
			t.Errorf("a deleted comment still points: %+v, %v", backlinks, err)
		}
		return nil
	})

	// The workspace next door.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if of, err := links.Of(ctx, one, 50); err != nil || len(of) != 0 {
			t.Errorf("B listed A's links: %v, %v", of, err)
		}
		if _, err := links.Find(ctx, first.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B found A's link: %v", err)
		}
		if err := links.Delete(ctx, first.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B removed A's link: %v", err)
		}
		if loops, err := links.PathExists(ctx, one, three, []string{domain.LinkTypeDependsOn}, nil); err != nil || loops {
			t.Errorf("B walked A's links: %v, %v", loops, err)
		}
		if summaries, err := links.Entries(ctx, []shared.ID{one}); err != nil || len(summaries) != 0 {
			t.Errorf("B read A's entry: %v, %v", summaries, err)
		}
		if backlinks, err := links.Backlinks(ctx, target, 50); err != nil || len(backlinks) != 0 {
			t.Errorf("B read A's references: %v, %v", backlinks, err)
		}
		if rows, err := types.List(ctx); err != nil || len(rows) != 0 {
			t.Errorf("B listed A's link types: %v, %v", rows, err)
		}
		return nil
	})
	// A link from B to A's entries finds no entry to point at; the refusal aborts its transaction.
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		foreign := first
		foreign.ID, foreign.TenantID = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd15"), tenantB
		return links.Insert(ctx, foreign)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("B linked A's entries: %v", err)
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := links.Delete(ctx, own.ID); err != nil {
			t.Errorf("A could not remove its link: %v", err)
		}
		if err := types.Delete(ctx, "verifies"); err != nil {
			t.Errorf("A could not delete a link type nobody uses: %v", err)
		}
		return nil
	})
}
