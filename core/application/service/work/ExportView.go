// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ExportViewName = "ExportView"

	// ViewExportedAction is the audit code. An export is a bulk read of somebody's work, which is
	// exactly the act an auditor asks about after the fact (audit.md §2).
	ViewExportedAction audit.Action = "view.exported"
)

// ExportView renders a saved view's result whole (D-08).
//
// The rows are selected here and rendered by whichever channel asked: a wire format is an
// adapter's business (project-structure.md §3), and the backend composes no document text of its
// own (rule 8). What this use case owes is the selection, the cap, and the honest statement that
// the cap was reached.
//
// The query executes under the **caller's** authorisation, exactly as reading the view does: two
// people exporting one shared view export two different files, and neither gets a row their role
// hides (T-04, T-05).
type ExportView struct {
	Views      viewrepo.SavedViews
	Containers repository.Containers
	Permits    Permitting
	// Query is the same use case an ad-hoc query goes through. Held rather than reimplemented:
	// an export that selected rows its own way would be a second answer to "what does this view
	// contain", and the two would drift.
	Query      QueryItems
	ItemLabels repository.ItemLabels
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ExportedView is what an export answers.
type ExportedView struct {
	View  view.SavedView
	Items []domain.WorkItem
	// Labels is the label identifiers per entry, so that a rendering that shows them does not
	// have to ask again.
	Labels map[shared.ID][]shared.ID
	// Truncated says the result reached view.MaxExportRows and there was more behind it.
	Truncated   bool
	GeneratedAt time.Time
}

// Execute selects the rows.
//
// It walks the query's own pages rather than asking for one large one: the page size the whole
// product shares is 200, the statements are written and indexed for that, and an export is the
// one caller that wants all of them - so it asks repeatedly and stops at the cap. One page beyond
// the cap is what tells it there was more, which is why the walk stops on the first page that
// crosses it rather than on an exact count nobody asked the database for.
func (h ExportView) Execute(
	ctx context.Context, actor appshared.ActorContext, viewID shared.ID,
) (ExportedView, error) {
	if err := actor.RequireScope(viewsRead); err != nil {
		return ExportedView{}, err
	}
	if viewID.IsZero() {
		return ExportedView{}, shared.ErrValidation.
			WithDetail("views.view_id_required").
			WithFields(shared.FieldError{Path: "/view_id", Code: "views.view_id_required"})
	}

	var saved view.SavedView
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Views.Find(ctx, viewID)
		saved = found
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return ExportedView{}, viewNotFound(viewID)
		}
		return ExportedView{}, err
	}

	visible, err := viewVisibleTo(ctx, h.UnitOfWork, h.Containers, h.Permits, actor, saved)
	if err != nil {
		return ExportedView{}, err
	}
	if !visible {
		return ExportedView{}, viewNotFound(viewID)
	}

	items, truncated, err := h.walk(ctx, actor, saved)
	if err != nil {
		return ExportedView{}, err
	}

	// The labels every rendering may show, read once for the whole export rather than per page.
	carried, err := labelsOf(ctx, h.UnitOfWork, h.ItemLabels, actor, true, items)
	if err != nil {
		return ExportedView{}, err
	}

	now := h.Clock.Now()
	if err := h.record(ctx, actor, saved, len(items), truncated, now); err != nil {
		return ExportedView{}, err
	}
	return ExportedView{
		View: saved, Items: items, Labels: carried, Truncated: truncated, GeneratedAt: now,
	}, nil
}

// walk runs the stored query page by page until the cap or the end.
func (h ExportView) walk(
	ctx context.Context, actor appshared.ActorContext, saved view.SavedView,
) ([]domain.WorkItem, bool, error) {
	spec, err := specOf(usecase.Input(saved.Query))
	if err != nil {
		// A stored query that no longer parses is a broken bookmark. It answers the grammar's own
		// code, which is what a client needs in order to say which field is the problem (D-07).
		return nil, false, err
	}
	// The grouping a client draws with is not an export's business: a file is rows, and a query
	// asked to group answers groups instead of a page.
	spec.GroupBy = view.GroupBy{}
	spec.Size = MaxPageSize
	spec.Count = view.CountNone

	var items []domain.WorkItem
	for {
		result, err := h.Query.Execute(ctx, actor, spec)
		if err != nil {
			return nil, false, err
		}
		items = append(items, result.Items...)

		if len(items) >= view.MaxExportRows {
			// The cap. Anything past it is dropped, and the caller is told - a truncation
			// somebody knows about is honest, and a silent one reads as "that is everything".
			return items[:view.MaxExportRows], true, nil
		}
		if !result.Info.HasMore {
			return items, false, nil
		}
		spec.Cursor = result.Info.NextCursor
	}
}

// record writes the audit entry. An export is a bulk read, so what it records is how much was
// read rather than what was in it: the rows are content and the count is not (rule 10).
func (h ExportView) record(
	ctx context.Context, actor appshared.ActorContext, saved view.SavedView,
	rows int, truncated bool, at time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   saved.TenantID,
		OccurredAt: at,
		Action:     ViewExportedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: viewTarget,
		TargetID:   saved.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "rows", Classification: audit.Open, To: strconv.Itoa(rows)},
			audit.Change{Field: "truncated", Classification: audit.Open, To: boolText(truncated)},
		),
	})
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// Descriptor is the catalogue entry.
func (h ExportView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ExportViewName,
		Summary: "A saved view's result, whole, under the caller's own authorisation - two " +
			"people exporting one shared view export two different files. Synchronous and " +
			"bounded: at most max_export_rows entries, and a result that reached the cap says " +
			"so rather than pretending to be everything. The rows come back as entries; which " +
			"file they become is the channel's business, and REST offers CSV, JSON and ICS.",
		SideEffects: "Writes an audit entry, because an export is a bulk read of somebody's " +
			"work. Changes nothing.",
		TokenScope: viewsRead,
		ReadOnly:   true,
		Input: []usecase.Field{
			{
				Name: "view_id", Kind: usecase.KindID, Required: true,
				Description: "The saved view to export. One the caller may read - a view they " +
					"cannot see answers not found, exactly as reading it would.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ViewExportedAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Reading is not a change to an entry, and the item history is what changed.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ExportView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	viewID, err := in.ID("view_id")
	if err != nil {
		return nil, err
	}

	exported, err := h.Execute(ctx, actor, viewID)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"view_id": exported.View.ID.String(),
		// The view's own name, so that a calendar client can label the subscription and a
		// spreadsheet can be called something. Content, and answered only to somebody who may
		// already read the view.
		"view_name":    exported.View.Name,
		"generated_at": exported.GeneratedAt.UTC(),
		"count":        len(exported.Items),
		"truncated":    exported.Truncated,
		"rows":         rowsOf(exported.Items, exported.Labels),
	}, nil
}
