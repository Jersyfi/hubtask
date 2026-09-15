// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// The acceptance of M-07 against a real database: a title that arrives decomposed is stored
// composed and found by a composed search - and, beside it, the consequence i18n-l10n.md §5
// names for the rows this task does not rewrite. A row holding the decomposed bytes - written
// here straight into the column, past the constructor and past the row's own normalize() -
// is *not* found by the same search, because PostgreSQL's parser compares code points and
// `a` + U+0308 is not `ä` to it. That is the row its next edit brings into line, and nothing
// else does.
func TestADecomposedTitleIsStoredComposedAndFoundByAComposedSearch(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	items := itemRepo()

	// A word nothing else in the shared database uses, spelt with combining marks and, in the
	// query, with the composed letters a keyboard produces.
	suffix := shortSuffix(t)
	decomposed := "Kra\u0308uterbeet gie\u00dfen " + suffix
	composed := "Kr\u00e4uterbeet gie\u00dfen " + suffix
	if decomposed == composed {
		t.Fatal("the fixture is not decomposed, so it proves nothing")
	}

	id := freshID(t)
	normalised, err := work.NewWorkItem(work.NewWorkItemInput{
		ID: id, TenantID: tenantA, CollectionID: collection, Type: work.ItemTask,
		Title: decomposed, ContentLanguage: "de", Profile: writableProfile(),
		Path: work.RootPath(id), Depth: 1, OrderKey: "a0", CreatedBy: authorA,
		Now: created, Text: textadapter.Forms{},
	})
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	if normalised.Title != composed {
		t.Fatalf("the constructor stored %q, want the composed form", normalised.Title)
	}

	// A row as it stands from before any normalisation: the repository's insert would compose the
	// title itself (I-W7), so the decomposed bytes are put into the column behind its back.
	legacy := taskIn(tenantA, authorA, collection, freshID(t), "placeholder", "a1")
	legacy.ContentLanguage = "de"

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := items.Insert(ctx, normalised); err != nil {
			return err
		}
		return items.Insert(ctx, legacy)
	}); err != nil {
		t.Fatalf("writing the two entries: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE work_item SET title = $2 WHERE id = $1`, legacy.ID.String(), decomposed); err != nil {
		t.Fatalf("writing the decomposed bytes: %v", err)
	}

	// The words are handed to the repository as typed - decomposed - because the compiler brings
	// them to the form itself, as it does every text a query compares (query/Builder.go, words).
	titles := found(ctx, t, tenantA, searchWithin(collection, decomposed, "de"))
	if len(titles) != 1 || titles[0] != composed {
		t.Errorf("the search found %q, want exactly the entry the constructor normalised", titles)
	}

	// The legacy row is indexed under its own bytes - a query that skips the normalisation reaches
	// it - so what keeps it out above is the form and nothing else.
	var reachable bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT search_document @@ websearch_to_tsquery('german', $2) FROM work_item WHERE id = $1`,
		legacy.ID.String(), decomposed).Scan(&reachable); err != nil {
		t.Fatalf("asking the legacy row: %v", err)
	}
	if !reachable {
		t.Error("the legacy row is not indexed under its decomposed bytes, so the miss above proves nothing")
	}
}

// The query language compares in the form the columns hold (query/Builder.go, text): a filter
// value typed with combining marks meets a title stored composed. Held by the compiler since the
// query language landed, and pinned here against the database rather than against the SQL text.
func TestAFilterValueMeetsATitleInTheFormItIsStored(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	suffix := shortSuffix(t)
	composed := "Gr\u00fc\u00dfe " + suffix
	decomposed := "Gru\u0308\u00dfe " + suffix
	if decomposed == composed {
		t.Fatal("the fixture is not decomposed, so it proves nothing")
	}

	id := freshID(t)
	item, err := work.NewWorkItem(work.NewWorkItemInput{
		ID: id, TenantID: tenantA, CollectionID: collection, Type: work.ItemTask,
		Title: composed, Profile: writableProfile(), Path: work.RootPath(id), Depth: 1,
		OrderKey: "a0", CreatedBy: authorA, Now: created, Text: textadapter.Forms{},
	})
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, item)
	}); err != nil {
		t.Fatalf("writing the entry: %v", err)
	}

	result := queried(ctx, t, tenantA, searchIn(t, collection,
		map[string]any{"field": "title", "op": "EQ", "value": decomposed}, view.Spec{}))
	if titles := titlesOf(result.Items); len(titles) != 1 || titles[0] != composed {
		t.Errorf("EQ with a decomposed value found %q, want the composed entry", titles)
	}
}
