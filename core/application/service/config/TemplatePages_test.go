// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"testing"
	"time"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const guidePage = "0190a000-0000-7000-8000-0000000000d1"

// configuration.md §6: a hub's document reads the pages a template brought by key with their
// title and icon - never their text -, writes one through AddHubTemplatePage with its body and
// subpages, and removes one by trashing its page; every attribute is fixed once written.
func TestAHubsDocumentCarriesItsTemplatePages(t *testing.T) {
	s := hubTemplatePagesSection(shared.MustParseID(supportHub))
	key, block := s.read(map[string]any{"key": "how_we_work", "page_id": guidePage, "title": "How we work", "icon": "📘"})
	if key != "how_we_work" || block["title"] != "How we work" || block["icon"] != "📘" || block["body"] != nil {
		t.Fatalf("read %q %v", key, block)
	}
	subpages := []any{map[string]any{"title": "Escalations", "body": "Call the on-call group."}}
	ops, err := s.opsAt(current{}, domain.BlockChange{Section: sectionTemplatePages, Key: "how_we_work", Op: domain.OpAdd,
		After: domain.Block{"title": "How we work", "body": "# Hello", "subpages": subpages}}, nil)
	if err != nil || len(ops) != 1 || ops[0].UseCase != AddHubTemplatePageName || ops[0].Input["hub_id"] != supportHub ||
		ops[0].Input["key"] != "how_we_work" || ops[0].Input["body"] != "# Hello" || ops[0].Input["subpages"] == nil {
		t.Fatalf("add %+v, %v", ops, err)
	}
	ops, err = s.opsAt(current{}, domain.BlockChange{Section: sectionTemplatePages, Key: "how_we_work", Op: domain.OpRemove},
		map[string]any{"page_id": guidePage})
	if err != nil || len(ops) != 1 || ops[0].UseCase != "TrashPage" || ops[0].Input["page_id"] != guidePage {
		t.Fatalf("remove %+v, %v", ops, err)
	}
	for _, attribute := range templatePageAttributes {
		if !s.isFixed(attribute) {
			t.Errorf("%s rewrites a page the hub may have made its own", attribute)
		}
	}
}

// templatePageWorld is the catalogue and the marks the template page use cases reach.
type templatePageWorld struct {
	catalogue
	created []usecase.Input
	marks   map[string]shared.ID
}

func (w *templatePageWorld) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if name == "CreatePage" {
		w.created = append(w.created, in)
		return usecase.Output{"id": guidePage, "title": in["title"], "icon": in["icon"]}, nil
	}
	return w.catalogue.Invoke(ctx, name, actor, in)
}

func (w *templatePageWorld) Mark(_ context.Context, _, page shared.ID, key string, _ time.Time) (bool, error) {
	if _, held := w.marks[key]; held {
		return false, nil
	}
	w.marks[key] = page
	return true, nil
}

func (w *templatePageWorld) List(context.Context, shared.ID) ([]configrepo.TemplatePage, error) {
	pages := []configrepo.TemplatePage{}
	for key, page := range w.marks {
		pages = append(pages, configrepo.TemplatePage{Key: key, PageID: page, Title: "How we work"})
	}
	return pages, nil
}

// configuration.md §6: a template's page is a page at the top of the hub's space with the pages
// beneath it, marked under its key; a key taken is refused and nothing stays marked twice.
func TestATemplatePageIsAPageOfTheHubMarkedByItsKey(t *testing.T) {
	f := hubFixture()
	w := &templatePageWorld{catalogue: catalogue{f.world}, marks: map[string]shared.ID{}}
	f.desk.Catalogue, f.desk.TemplatePages = w, w
	add := AddHubTemplatePage{f.desk}.Descriptor()
	subpages := []any{map[string]any{"title": "Escalations"}}
	out := invoke(t, add, person, usecase.Input{
		"hub_id": supportHub, "key": "how_we_work", "title": "How we work", "body": "# Hello", "icon": "📘", "subpages": subpages,
	})
	if out["page_id"] != guidePage || out["key"] != "how_we_work" || out["icon"] != "📘" || len(w.created) != 1 {
		t.Fatalf("added %v, created %v", out, w.created)
	}
	if c := w.created[0]; c["hub_id"] != supportHub || c["parent_id"] != nil || c["body"] != "# Hello" || c["subpages"] == nil {
		t.Errorf("created %v", c)
	}
	_, err := add.Handler.Invoke(t.Context(), person, usecase.Input{"hub_id": supportHub, "key": "how_we_work", "title": "Again"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.template_page_taken" {
		t.Errorf("a key taken twice: %v", err)
	}
	_, err = add.Handler.Invoke(t.Context(), person, usecase.Input{"hub_id": supportHub, "key": "Bad Key", "title": "X"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.key_invalid" {
		t.Errorf("a key that is no key: %v", err)
	}
	listed := invoke(t, ListHubTemplatePages{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	if rows := listed["pages"].([]any); len(rows) != 1 || rows[0].(map[string]any)["icon"] != nil {
		t.Errorf("listed %v", listed)
	}
}

// configuration.md §6: a template's pages come with a hub's first take of it and never with an
// upgrade, which compares none of them; a block of a page is held to a page's shape when the
// template is published.
func TestATemplatesPagesComeWithTheFirstTakeAlone(t *testing.T) {
	f := hubFixture()
	pages := map[string]any{"how_we_work": map[string]any{"title": "How we work", "body": "# Hello"}}
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "desk", "name": "Desk",
		"document":     map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}},
		"hub_document": map[string]any{"scheme": map[string]any{"every_type": map[string]any{"workflow": "service"}}, "template_pages": pages},
	})
	take := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk"})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": take["document"], "plan_hash": take["plan_hash"], "template": take["template"],
	})
	f.world.preview = usecase.Output{"entries": 2, "moves": []usecase.Output{}, "removed": []usecase.Output{}}

	first := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk", "hub_id": supportHub})
	if document := first["document"].(map[string]any); document["template_pages"] == nil {
		t.Fatalf("a hub's first take brings no pages: %v", document)
	}
	// The hub took the template; recorded as the apply records it.
	if err := f.desk.Store.PutTemplateUse(t.Context(), domain.TemplateUse{
		HubID: shared.MustParseID(supportHub), TemplateKey: "desk", Version: 1, Document: domain.Document{},
	}); err != nil {
		t.Fatal(err)
	}
	again := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk", "hub_id": supportHub})
	if document := again["document"].(map[string]any); document["template_pages"] != nil {
		t.Fatalf("a hub that took the template is brought its pages again: %v", document)
	}

	_, err := PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "bad", "name": "Bad", "document": map[string]any{},
		"hub_document": map[string]any{"template_pages": map[string]any{"guide": map[string]any{"title": "Two\nlines", "colour": "red"}}},
	})
	codes := map[string]bool{}
	for _, r := range refusalsIn(err) {
		codes[r.Code] = true
	}
	if !codes["config.template_page_title_invalid"] || !codes["config.attribute_unknown"] {
		t.Errorf("a page block of no page's shape was published: %v %v", err, refusalsIn(err))
	}

	// A title, a sample's and the template's own name are one line without a character the
	// database cannot store: refused at publishing, not as the service being away at the apply.
	_, err = PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "zero", "name": "Zero", "document": map[string]any{},
		"hub_document": map[string]any{
			"template_pages": map[string]any{"guide": map[string]any{"title": "Guide\x00"}},
			"samples":        map[string]any{"first": map[string]any{"title": "First\tsample"}},
		},
	})
	codes = map[string]bool{}
	for _, r := range refusalsIn(err) {
		codes[r.Code] = true
	}
	if !codes["config.template_page_title_invalid"] || !codes["config.sample_title_invalid"] {
		t.Errorf("a title with a control character was published: %v %v", err, refusalsIn(err))
	}
	_, err = PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "named", "name": "Desk\x00", "document": map[string]any{},
	})
	if codes := refusalsIn(err); len(codes) != 1 || codes[0].Code != "config.name_invalid" {
		t.Errorf("a template name with a zero byte was published: %v %v", err, codes)
	}
}
