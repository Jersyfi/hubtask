// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"cmp"
	"context"
	"slices"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	ListPageLabelsName  = "ListPageLabels"
	SetPageLabelsName   = "SetPageLabels"
	ListSpaceLabelsName = "ListSpaceLabels"

	// maxSpaceLabelRows bounds the rows a facet counts: twenty labels on each of a large space's
	// pages. A space beyond it is counted over its first rows rather than refused.
	maxSpaceLabelRows = 20000
)

// ListPageLabels answers the labels a page carries (UC-DOC-01 check 9).
type ListPageLabels struct{ Space Space }

// Execute reads the labels.
func (h ListPageLabels) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) ([]string, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageReadAction)
	if err != nil {
		return nil, err
	}
	var labels map[shared.ID][]string
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		labels, err = h.Space.Pages.Labels(ctx, []shared.ID{page.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	return orEmpty(labels[page.ID]), nil
}

// Descriptor is the catalogue entry.
func (h ListPageLabels) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPageLabelsName, Permission: service.PermissionRead,
		Summary:     "Answers the labels a page carries, in alphabetical order.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			labels, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"labels": labels}, nil
		}),
	})
}

// SetPageLabels replaces the labels a page carries (UC-DOC-01 check 9). Labels are no part of a
// page's words, so the page's version and history stay as they are; the trail names the change and
// fingerprints the labels, which are the writer's words (rule 10).
type SetPageLabels struct{ Space Space }

// Execute writes the labels and answers them as they stand.
func (h SetPageLabels) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, raw []string) ([]string, error) {
	labels, err := domain.Labels(raw)
	if err != nil {
		return nil, err
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return nil, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageChangedAction, pageTarget); err != nil {
		return nil, err
	}
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		page, err := h.Space.find(ctx, pageID, true)
		if err != nil {
			return err
		}
		before, err := h.Space.Pages.Labels(ctx, []shared.ID{page.ID})
		if err != nil {
			return err
		}
		if slices.Equal(before[page.ID], labels) {
			return nil
		}
		if err := h.Space.Pages.SetLabels(ctx, page.ID, labels); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageChangedAction, pageTarget, page.ID, []audit.Change{
			{Field: "labels", Classification: audit.Sensitive, To: labels},
		}, h.Space.Clock.Now())
	})
	if err != nil {
		return nil, err
	}
	return labels, nil
}

// Descriptor is the catalogue entry.
func (h SetPageLabels) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetPageLabelsName, Permission: service.PermissionWriteItems,
		Summary: "Replaces the labels a page carries: each one word in lower case of letters, digits, " +
			"'-', '_' or '.', at most 40 characters, at most 20 a page; an empty list takes them all " +
			"away. The page's version and history stay as they are.",
		SideEffects: "Writes the page's labels and an audit entry when they change.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "labels", Kind: usecase.KindList, Required: true, Description: "Every label the page carries afterwards."},
		},
		Audit: usecase.AuditDeclaration{Action: PageChangedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			raw, err := in.StringList("labels")
			if err != nil {
				return nil, err
			}
			labels, err := h.Execute(ctx, actor, id, raw)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"labels": labels}, nil
		}),
	})
}

// LabelCount is a label of a space and how many of its pages the reader may read carry it.
type LabelCount struct {
	Label string
	Pages int
}

// ListSpaceLabels answers the labels of a space's live pages with their counts, most used first:
// the facet a search narrows by (UC-DOC-01 check 9). A page a restriction keeps from the reader is
// not counted, so a facet never tells of a page the reader cannot open.
type ListSpaceLabels struct{ Space Space }

// Execute counts.
func (h ListSpaceLabels) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) ([]LabelCount, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor,
		h.Space.request(hubID, service.PermissionRead, PageReadAction, pagesRead, pageTarget, hubID)); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, hubID); err != nil {
			return err
		}
		rows, err := h.Space.Pages.SpaceLabels(ctx, hubID, maxSpaceLabelRows)
		if err != nil || len(rows) == 0 {
			return err
		}
		ids := make([]shared.ID, 0, len(rows))
		for _, row := range rows {
			if len(ids) == 0 || ids[len(ids)-1] != row.PageID {
				ids = append(ids, row.PageID)
			}
		}
		readers, err := h.Space.Pages.Readers(ctx, ids)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if readers[row.PageID].Admits(actor.AccountID) {
				counts[row.Label]++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]LabelCount, 0, len(counts))
	for label, pages := range counts {
		out = append(out, LabelCount{Label: label, Pages: pages})
	}
	slices.SortFunc(out, func(a, b LabelCount) int {
		return cmp.Or(cmp.Compare(b.Pages, a.Pages), cmp.Compare(a.Label, b.Label))
	})
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ListSpaceLabels) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListSpaceLabelsName, Permission: service.PermissionRead,
		Summary: "Answers every label a hub's live pages carry with how many pages carry it, most used " +
			"first - only pages the reader may read are counted.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose space is counted."}},
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			counts, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(counts))
			for _, count := range counts {
				data = append(data, usecase.Output{"label": count.Label, "pages": count.Pages})
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

func orEmpty(labels []string) []string {
	if labels == nil {
		return []string{}
	}
	return labels
}
