// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"slices"
	"strings"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListHubTemplatePagesName = "ListHubTemplatePages"
	AddHubTemplatePageName   = "AddHubTemplatePage"

	TemplatePageAddedAction audit.Action = "config.template_page_added"

	// sectionTemplatePages is the pages a template brings to a hub's space (configuration.md §6).
	sectionTemplatePages = "template_pages"
)

// templatePageAttributes is what a block names; the document reads back title and icon only -
// a page's text is no configuration (configuration.md §1), so body and subpages are words given
// once, when the page is written.
var templatePageAttributes = []string{"title", "icon", "body", "subpages"}

// ListHubTemplatePages answers the pages a template brought to a hub.
type ListHubTemplatePages struct{ Desk Desk }

func (h ListHubTemplatePages) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListHubTemplatePagesName,
		Permission: service.PermissionStructure,
		Summary: "Lists the pages of the hub's space a template brought - a guide, a knowledge space's " +
			"first pages - marked under their key and not in the trash: the hub document's " +
			"template_pages section.",
		SideEffects: "None.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit:       usecase.AuditDeclaration{},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var pages []configrepo.TemplatePage
			err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				pages, err = h.Desk.TemplatePages.List(ctx, hub)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := make([]any, 0, len(pages))
			for _, p := range pages {
				rows = append(rows, templatePageRow(p.Key, p.PageID.String(), p.Title, p.Icon))
			}
			return usecase.Output{"pages": rows}, nil
		}),
	}
}

func templatePageRow(key, page, title, icon string) map[string]any {
	var shown any
	if icon != "" {
		shown = icon
	}
	return map[string]any{"key": key, "page_id": page, "title": title, "icon": shown}
}

// AddHubTemplatePage writes a page at the top of the hub's space, marked as a template's.
type AddHubTemplatePage struct{ Desk Desk }

func (h AddHubTemplatePage) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       AddHubTemplatePageName,
		Permission: service.PermissionStructure,
		Summary: "Writes a page at the top of the hub's space, with the pages beneath it, and marks it " +
			"as the hub's template page under its key, as a template's pages are taken. A key the hub " +
			"marks already is refused. The page is an ordinary page: writing it asks what writing any " +
			"page asks, and it is the hub's to change from then on.",
		SideEffects: "Writes the page and its subpages with everything writing a page writes, marks it, " +
			"and writes an audit entry.",
		TokenScope: scopeItemsWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The page's key in the hub, a lower-case word."},
			{Name: "title", Kind: usecase.KindString, Required: true, Description: "The page's title, one line."},
			{Name: "body", Kind: usecase.KindString, Description: "The body in Markdown."},
			{Name: "icon", Kind: usecase.KindString, Description: "An emoji or short symbol shown beside the title."},
			{Name: "subpages", Kind: usecase.KindList, Description: "Pages written beneath it, each {title, body, icon, subpages}."},
		},
		Audit:    usecase.AuditDeclaration{Action: TemplatePageAddedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "the page's writing is its own activity."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, TemplatePageAddedAction, scopeItemsWrite, false); err != nil {
				return nil, err
			}
			return h.Desk.addTemplatePage(ctx, actor, hub, in)
		}),
	}
}

func (d Desk) addTemplatePage(ctx context.Context, actor appshared.ActorContext, hub shared.ID, in usecase.Input) (usecase.Output, error) {
	key := in.String("key")
	if !sampleKeyPattern.MatchString(key) {
		return nil, refused("config.key_invalid", "/key", nil)
	}
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		create := usecase.Input{"hub_id": hub.String(), "title": in.String("title")}
		for _, name := range []string{"body", "icon"} {
			if value := in.String(name); value != "" {
				create[name] = value
			}
		}
		if subpages, ok := in["subpages"].([]any); ok && len(subpages) > 0 {
			create["subpages"] = subpages
		}
		page, err := d.Catalogue.Invoke(ctx, "CreatePage", actor, create)
		if err != nil {
			return err
		}
		id, err := shared.ParseID(text(page["id"]))
		if err != nil {
			return err
		}
		now := d.Clock.Now()
		marked, err := d.TemplatePages.Mark(ctx, hub, id, key, now)
		if err != nil {
			return err
		}
		if !marked {
			// The whole write rolls back: no unmarked page is left behind a refusal.
			return refused("config.template_page_taken", "/key", map[string]string{"key": key})
		}
		out = templatePageRow(key, id.String(), text(page["title"]), text(page["icon"]))
		return d.trail(ctx, actor, TemplatePageAddedAction, now, audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.String()})
	})
	return out, err
}

// hubTemplatePagesSection reads the pages a template brought to the hub and writes or trashes
// them. Every attribute is fixed: once written, a page is the hub's to change like any other.
func hubTemplatePagesSection(hub shared.ID) section {
	id := hub.String()
	return section{
		name: sectionTemplatePages, list: ListHubTemplatePagesName, listInput: usecase.Input{"hub_id": id},
		rows: "pages", key: "key", create: AddHubTemplatePageName,
		attributes: templatePageAttributes, fixed: templatePageAttributes,
		block: func(row map[string]any) (string, domain.Block) {
			block := domain.Block{"title": row["title"]}
			if icon := text(row["icon"]); icon != "" {
				block["icon"] = icon
			}
			return text(row["key"]), block
		},
		opsAt: func(_ current, change domain.BlockChange, row map[string]any) ([]operation, error) {
			path := change.Path()
			if change.Op == domain.OpRemove {
				return []operation{{UseCase: "TrashPage", Path: path, Input: usecase.Input{"page_id": row["page_id"]}}}, nil
			}
			in := usecase.Input{"hub_id": id, "key": change.Key, "title": change.After["title"]}
			for _, name := range []string{"body", "icon", "subpages"} {
				if value, held := change.After[name]; held && value != nil {
					in[name] = value
				}
			}
			return []operation{{UseCase: AddHubTemplatePageName, Path: path, Input: in}}, nil
		},
		hides: removing,
	}
}

// withoutTemplatePages is a document without the pages it brings, the original untouched: an
// upgrade compares none of them, so no newer version rewrites or trashes a guide a hub made its own.
func withoutTemplatePages(d domain.Document) domain.Document {
	if _, held := d[sectionTemplatePages]; !held {
		return d
	}
	out := make(domain.Document, len(d))
	for name, s := range d {
		if name != sectionTemplatePages {
			out[name] = s
		}
	}
	return out
}

// templatePageRefusals checks a template's page blocks as far as no hub is needed: a key, a title
// on one line, only a page's attributes, and at most 100 pages in one write.
func templatePageRefusals(pages domain.Section) []shared.FieldError {
	var out []shared.FieldError
	for key, block := range pages {
		path := sectionTemplatePages + "." + key
		if !sampleKeyPattern.MatchString(key) {
			out = append(out, domain.Refusal("config.key_invalid", path, nil))
		}
		if block == nil {
			continue
		}
		if title := text(block["title"]); title == "" || len([]rune(title)) > 200 || strings.ContainsAny(title, "\r\n") {
			out = append(out, domain.Refusal("config.template_page_title_invalid", path+".title", nil))
		}
		for attribute := range block {
			if !slices.Contains(templatePageAttributes, attribute) {
				out = append(out, domain.Refusal("config.attribute_unknown", path+"."+attribute, nil))
			}
		}
		if 1+countPages(block["subpages"]) > 100 {
			out = append(out, domain.Refusal("pages.too_many_at_once", path+".subpages", nil))
		}
	}
	return out
}

func countPages(v any) int {
	list, _ := v.([]any)
	n := len(list)
	for _, p := range list {
		if page, ok := p.(map[string]any); ok {
			n += countPages(page["subpages"])
		}
	}
	return n
}
