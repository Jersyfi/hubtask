// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names and trail codes of a page's languages for a portal (UC-DOC-13 check 4).
const (
	ListPageLanguageVariantsName  = "ListPageLanguageVariants"
	LinkPageLanguageVariantName   = "LinkPageLanguageVariant"
	UnlinkPageLanguageVariantName = "UnlinkPageLanguageVariant"

	PageLanguageLinkedAction   audit.Action = "page.language_linked"
	PageLanguageUnlinkedAction audit.Action = "page.language_unlinked"
)

// languagesOf is what a portal answers an actor in, preferred first.
func (s Space) languagesOf(actor appshared.ActorContext) []string {
	if s.Variants == nil {
		return nil
	}
	return domain.LanguagePreference(actor.Locale)
}

// inLanguageOf answers the published page in the actor's language, inside the caller's
// transaction: the first variant of it in their preference that is live and unrestricted, else
// the page itself. A variant restricted since is skipped as an article restricted since is not
// found: the portal reaches people the restriction does not name.
func (s Space) inLanguageOf(ctx context.Context, actor appshared.ActorContext, source domain.Page) (ArticleRead, error) {
	read := ArticleRead{Source: source.ID, Page: source, Language: source.ContentLanguage}
	if s.Variants == nil {
		return read, nil
	}
	preferred := s.languagesOf(actor)
	if len(preferred) == 0 {
		return read, nil
	}
	variants, err := s.Variants.VariantsOf(ctx, source.ID)
	if err != nil {
		return ArticleRead{}, err
	}
	tags := make(map[shared.ID]string, len(variants))
	for _, variant := range variants {
		tags[variant.PageID] = variant.Language
	}
	for _, id := range domain.VariantsFor(actor.Locale, variants) {
		page, err := s.find(ctx, id, false)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return ArticleRead{}, err
		}
		readers, err := s.Pages.Readers(ctx, []shared.ID{page.ID})
		if err != nil {
			return ArticleRead{}, err
		}
		if readers[page.ID].Restricted || page.IsArchived() {
			continue
		}
		return ArticleRead{Source: source.ID, Page: page, Language: tags[id]}, nil
	}
	return read, nil
}

// PageLanguages is where a page stands among the languages of its source (schema
// PageLanguageVariants).
type PageLanguages struct {
	PageID   shared.ID
	SourceID shared.ID
	Language string
	Variants []domain.LanguageVariant
}

// ListPageLanguageVariants answers the page a page is a variant of and the variants of its own
// that the caller may read.
type ListPageLanguageVariants struct{ Space Space }

// Execute lists.
func (h ListPageLanguageVariants) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) (PageLanguages, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageReadAction)
	if err != nil {
		return PageLanguages{}, err
	}
	out := PageLanguages{PageID: page.ID, Variants: []domain.LanguageVariant{}}
	if h.Space.Variants == nil {
		return out, nil
	}
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if out.SourceID, out.Language, err = h.Space.Variants.SourceOf(ctx, page.ID); err != nil {
			return err
		}
		variants, err := h.Space.Variants.VariantsOf(ctx, page.ID)
		if err != nil {
			return err
		}
		for _, variant := range variants {
			if err := h.Space.admitted(ctx, actor, variant.PageID); err == nil {
				out.Variants = append(out.Variants, variant)
			} else if !errors.Is(err, shared.ErrNotFound) {
				return err
			}
		}
		return nil
	})
	return out, err
}

func variantOutput(v domain.LanguageVariant, source shared.ID) usecase.Output {
	return usecase.Output{"page_id": v.PageID.String(), "source_page_id": source.String(), "language": v.Language}
}

// Descriptor is the catalogue entry.
func (h ListPageLanguageVariants) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPageLanguageVariantsName, Permission: service.PermissionRead,
		Summary: "Answers the page a page is a language variant of, under which tag, and the " +
			"variants of its own the caller may read - the languages a portal answers it in.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
		},
		Audit: usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			languages, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			variants := make([]usecase.Output, 0, len(languages.Variants))
			for _, v := range languages.Variants {
				variants = append(variants, variantOutput(v, languages.PageID))
			}
			return usecase.Output{
				"page_id": languages.PageID.String(), "source_page_id": idOrNil(languages.SourceID),
				"language": nilIfEmpty(languages.Language), "variants": variants,
			}, nil
		}),
	})
}

// LinkPageLanguageVariant makes a page the variant of another in one language: wherever the
// source is published, a requester of that language reads it in the source's place. Asked as
// publishing is, since the variant reaches every portal the source does.
type LinkPageLanguageVariant struct{ Space Space }

// Execute links.
func (h LinkPageLanguageVariant) Execute(ctx context.Context, actor appshared.ActorContext, pageID, sourceID shared.ID, language string) (domain.LanguageLink, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return domain.LanguageLink{}, err
	}
	if h.Space.Variants == nil {
		return domain.LanguageLink{}, shared.ErrUnavailable.WithDetail("pages.articles_unavailable")
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return domain.LanguageLink{}, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PageLanguageLinkedAction, pagesWrite, pageTarget, place.ID)); err != nil {
		return domain.LanguageLink{}, err
	}
	if sourceID.IsZero() {
		return domain.LanguageLink{}, fieldInvalid("/source_page_id", "pages.not_found")
	}
	if _, err := h.Space.placeOf(ctx, actor, sourceID); err != nil {
		return domain.LanguageLink{}, err
	}
	now := h.Space.Clock.Now()
	var link domain.LanguageLink
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		page, source, err := h.lockBoth(ctx, pageID, sourceID)
		if err != nil {
			return err
		}
		above, _, err := h.Space.Variants.SourceOf(ctx, source.ID)
		if err != nil {
			return err
		}
		own, err := h.Space.Variants.VariantsOf(ctx, page.ID)
		if err != nil {
			return err
		}
		taken, err := h.Space.Variants.VariantsOf(ctx, source.ID)
		if err != nil {
			return err
		}
		if link, err = domain.NewLanguageLink(page, source, !above.IsZero(), len(own), taken, language); err != nil {
			return err
		}
		if err := h.Space.Variants.Link(ctx, link); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageLanguageLinkedAction, pageTarget, page.ID, []audit.Change{
			{Field: "source_page_id", Classification: audit.Open, To: source.ID.String()},
			{Field: "language", Classification: audit.Open, To: link.Language},
		}, now)
	})
	return link, err
}

// lockBoth locks the two pages in identifier order, so that two links crossing each other wait
// rather than deadlock - and neither can make a page the variant of its own variant.
func (h LinkPageLanguageVariant) lockBoth(ctx context.Context, pageID, sourceID shared.ID) (domain.Page, domain.Page, error) {
	first, second := pageID, sourceID
	if second.String() < first.String() {
		first, second = second, first
	}
	locked := map[shared.ID]domain.Page{}
	for _, id := range []shared.ID{first, second} {
		if _, held := locked[id]; held {
			continue
		}
		page, err := h.Space.find(ctx, id, true)
		if err != nil {
			return domain.Page{}, domain.Page{}, err
		}
		locked[id] = page
	}
	return locked[pageID], locked[sourceID], nil
}

// Descriptor is the catalogue entry.
func (h LinkPageLanguageVariant) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: LinkPageLanguageVariantName, Permission: service.PermissionManageMembers,
		Summary: "Makes a page the variant of another of its space in one language: wherever the " +
			"source is published to a service's portal, a requester of that language reads this " +
			"page in its place. One level only, one page per tag, at most 20; linked again, the " +
			"link is replaced.",
		SideEffects: "Writes the link and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page written in another language."},
			{Name: "source_page_id", Kind: usecase.KindID, Required: true, Description: "The page it is a variant of."},
			{Name: "language", Kind: usecase.KindString, Required: true, Description: "The BCP 47 tag it is written in."},
		},
		Audit:    usecase.AuditDeclaration{Action: PageLanguageLinkedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: pagesExempt},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids, err := twoIDs(in, "page_id", "source_page_id")
			if err != nil {
				return nil, err
			}
			link, err := h.Execute(ctx, actor, ids[0], ids[1], in.String("language"))
			if err != nil {
				return nil, err
			}
			return variantOutput(domain.LanguageVariant{PageID: link.PageID, Language: link.Language}, link.SourceID), nil
		}),
	})
}

// UnlinkPageLanguageVariant makes a language variant a page of its own again.
type UnlinkPageLanguageVariant struct{ Space Space }

// Execute unlinks.
func (h UnlinkPageLanguageVariant) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) error {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return err
	}
	if h.Space.Variants == nil {
		return shared.ErrNotFound.WithDetail("pages.variant_not_found")
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PageLanguageUnlinkedAction, pagesWrite, pageTarget, place.ID)); err != nil {
		return err
	}
	now := h.Space.Clock.Now()
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Space.Variants.Unlink(ctx, place.ID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("pages.variant_not_found")
		}
		return h.Space.record(ctx, actor, PageLanguageUnlinkedAction, pageTarget, place.ID, nil, now)
	})
}

// Descriptor is the catalogue entry.
func (h UnlinkPageLanguageVariant) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UnlinkPageLanguageVariantName, Permission: service.PermissionManageMembers,
		Summary:     "Makes a language variant a page of its own again: a portal answers its source where it stood.",
		SideEffects: "Removes the link and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The variant."},
		},
		Audit:    usecase.AuditDeclaration{Action: PageLanguageUnlinkedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: pagesExempt},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, id)
		}),
	})
}
