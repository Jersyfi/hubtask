// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// tongues keeps the links beside the store's pages.
type tongues struct {
	links map[shared.ID]domain.LanguageLink
	pages *store
}

func (t *tongues) SourceOf(_ context.Context, page shared.ID) (shared.ID, string, error) {
	link := t.links[page]
	return link.SourceID, link.Language, nil
}
func (t *tongues) VariantsOf(_ context.Context, source shared.ID) ([]domain.LanguageVariant, error) {
	var out []domain.LanguageVariant
	for _, link := range t.links {
		if link.SourceID == source && t.pages.pages[link.PageID].DeletedAt == nil {
			out = append(out, domain.LanguageVariant{PageID: link.PageID, Language: link.Language})
		}
	}
	slices.SortFunc(out, func(a, b domain.LanguageVariant) int { return strings.Compare(a.Language, b.Language) })
	return out, nil
}
func (t *tongues) Link(_ context.Context, link domain.LanguageLink) error {
	t.links[link.PageID] = link
	return nil
}
func (t *tongues) Unlink(_ context.Context, page shared.ID) (bool, error) {
	_, held := t.links[page]
	delete(t.links, page)
	return held, nil
}

// spoken is the shelf whose search offers each article in the reader's language, as the
// adapter's query does.
type spoken struct {
	*shelf
	tongues *tongues
}

func (s spoken) Search(ctx context.Context, collection shared.ID, text string, languages []string, _ int) ([]repository.ArticleHit, error) {
	var out []repository.ArticleHit
	for key := range s.articles {
		source := s.pages.pages[key[0]]
		if key[1] != collection || source.DeletedAt != nil || source.Restricted {
			continue
		}
		chosen, language := source, source.ContentLanguage
		variants, _ := s.tongues.VariantsOf(ctx, source.ID)
		for _, tag := range slices.Backward(languages) {
			for _, v := range variants {
				if strings.EqualFold(v.Language, tag) {
					chosen, language = s.pages.pages[v.PageID], v.Language
				}
			}
		}
		if strings.Contains(chosen.Body, text) {
			out = append(out, repository.ArticleHit{PageID: source.ID, Title: chosen.Title, ContentLanguage: language})
		}
	}
	return out, nil
}

func in(locale string) appshared.ActorContext {
	actor := as(visitor)
	actor.Locale = locale
	return actor
}

// UC-DOC-13 check 4: a page written in another language is linked to its source under a tag by
// who may publish it; a requester reads the source's article in their language - the exact tag,
// then the language alone, then the source - while the opening and the rating stay the article's,
// and a variant trashed or unlinked since gives way to the next.
func TestAnArticleIsReadInTheRequestersLanguage(t *testing.T) {
	f := newFixture()
	links := &tongues{links: map[shared.ID]domain.LanguageLink{}, pages: f.store}
	articles := spoken{shelf: &shelf{articles: map[[2]shared.ID]*domain.Article{}, ratings: map[[3]shared.ID]bool{}, pages: f.store}, tongues: links}
	f.space.Articles, f.space.Variants, f.space.Containers, f.space.Authorizer, f.space.Text = articles, links, services{}, requesterGate{}, plain{}
	ctx := context.Background()
	source := f.create(t, anna, hub, "", "Reset your password", "how to reset the password")
	german := f.create(t, anna, hub, "", "Passwort zurücksetzen", "so setzt du das Passwort zurück")
	austrian := f.create(t, anna, hub, "", "Passwort zurücksetzen (AT)", "so setzt man das Passwort in Wien zurück")
	other := f.create(t, anna, hub, "", "Mot de passe", "le mot de passe")
	link := LinkPageLanguageVariant{Space: f.space}

	if _, err := link.Execute(ctx, as(visitor), german.ID, source.ID, "de"); err == nil {
		t.Fatal("somebody who may not publish linked a variant")
	}
	for _, tc := range []struct {
		page shared.ID
		tag  string
	}{{german.ID, "de"}, {austrian.ID, " de-AT "}} {
		if got, err := link.Execute(ctx, as(anna), tc.page, source.ID, tc.tag); err != nil || got.SourceID != source.ID {
			t.Fatalf("LinkPageLanguageVariant(%s): %+v %v", tc.tag, got, err)
		}
	}
	for _, tc := range []struct {
		page, source shared.ID
		tag, want    string
	}{
		{other.ID, source.ID, "DE", "pages.variant_language_taken"},
		{other.ID, german.ID, "fr", "pages.variant_of_a_variant"},
		{source.ID, other.ID, "en", "pages.variant_has_variants"},
		{other.ID, source.ID, "français", "pages.language_malformed"},
	} {
		if _, err := link.Execute(ctx, as(anna), tc.page, tc.source, tc.tag); code(err) != tc.want {
			t.Fatalf("linking %s under %q answered %v, want %s", tc.page, tc.tag, err, tc.want)
		}
	}
	if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != PageLanguageLinkedAction || last.TargetID != austrian.ID {
		t.Fatalf("the link reached the trail as %+v", last)
	}
	listed, err := (ListPageLanguageVariants{Space: f.space}).Execute(ctx, as(anna), source.ID)
	if err != nil || !listed.SourceID.IsZero() || len(listed.Variants) != 2 {
		t.Fatalf("ListPageLanguageVariants(source): %+v %v", listed, err)
	}
	if listed, err := (ListPageLanguageVariants{Space: f.space}).Execute(ctx, as(anna), austrian.ID); err != nil || listed.SourceID != source.ID || listed.Language != "de-AT" {
		t.Fatalf("ListPageLanguageVariants(variant): %+v %v", listed, err)
	}

	if _, err := (PublishPageToPortal{Space: f.space}).Execute(ctx, as(anna), source.ID, desk); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		locale   string
		want     shared.ID
		language string
	}{
		{"de-AT", austrian.ID, "de-AT"},
		{"de-CH", german.ID, "de"},
		{"de", german.ID, "de"},
		{"fr", source.ID, ""},
		{"", source.ID, ""},
	} {
		read, err := (ReadArticle{Space: f.space}).Execute(ctx, in(tc.locale), desk, source.ID)
		if err != nil || read.Page.ID != tc.want || read.Language != tc.language || read.Source != source.ID {
			t.Fatalf("ReadArticle in %q: %+v %v, want %s", tc.locale, read, err, tc.want)
		}
	}
	hits, err := (SuggestArticles{Space: f.space}).Execute(ctx, in("de-CH"), desk, "Passwort")
	if err != nil || len(hits) != 1 || hits[0]["page_id"] != source.ID.String() || hits[0]["title"] != german.Title || hits[0]["content_language"] != "de" {
		t.Fatalf("SuggestArticles in de-CH: %v %v", hits, err)
	}
	if err := (RateArticle{Space: f.space}).Execute(ctx, in("de-AT"), desk, source.ID, true); err != nil {
		t.Fatal(err)
	}
	access, err := (ReadPageAccess{Space: f.space}).Execute(ctx, as(anna), source.ID)
	if err != nil || len(access.Articles) != 1 || access.Articles[0].Views != 5 || access.Articles[0].Helpful != 1 {
		t.Fatalf("the article's counts: %+v %v", access.Articles, err)
	}

	if _, err := (TrashPage{Space: f.space}).Execute(ctx, as(anna), austrian.ID); err != nil {
		t.Fatal(err)
	}
	if read, err := (ReadArticle{Space: f.space}).Execute(ctx, in("de-AT"), desk, source.ID); err != nil || read.Page.ID != german.ID {
		t.Fatalf("a trashed variant was read: %+v %v", read, err)
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: german.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	if read, err := (ReadArticle{Space: f.space}).Execute(ctx, in("de"), desk, source.ID); err != nil || read.Page.ID != source.ID {
		t.Fatalf("a restricted variant was read: %+v %v", read, err)
	}
	unlink := UnlinkPageLanguageVariant{Space: f.space}
	if err := unlink.Execute(ctx, as(anna), german.ID); err != nil {
		t.Fatal(err)
	}
	if err := unlink.Execute(ctx, as(anna), german.ID); code(err) != "pages.variant_not_found" {
		t.Fatalf("a second unlink answered %v", err)
	}
}
