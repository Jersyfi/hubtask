// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The installation at a glance, and its own record (SI-17, ADR-0070 §5).
//
// **Counts, states and limits - never rows.** The tenant boundary is a database policy rather than a
// role, and the dashboard does not go around it: the census reaches through a function that can
// answer five integers and nothing else, and the journal holds identifiers, slugs, counts and
// moments by construction. Neither of them can be asked for the contents of a workspace, which is
// what makes a screen for the operator honest rather than a way in.
//
// Both are behind `admin:tenants` **and** the operator register, both checked, as everything else at
// this level is.

const (
	ReadInstanceOverviewName = "ReadInstanceOverview"
	ListInstanceJournalName  = "ListInstanceJournal"

	// defaultJournalPage is what a caller that names no size gets (api-guidelines.md §4).
	defaultJournalPage = 50
	// maxJournalPage is the ceiling. A clamp rather than a refusal: a client asking for 500 rows
	// wants as many as it can have, and 200 of them is a better answer than an error.
	maxJournalPage = 200
)

// The audit codes. A read that was *refused* is recorded against the action it was refused for, and
// that action has to have a name (audit.md §4).
const (
	instanceOverviewReadAction audit.Action = "instance.overview_read"
	instanceJournalReadAction  audit.Action = "instance.journal_read"
)

// ReadInstanceOverview answers how big this installation is and how its workspaces stand.
type ReadInstanceOverview struct {
	Writer InstanceWriter
	// Installation is the census, through the one function that answers it.
	Installation adminrepo.Installation
}

// Execute reads it.
func (h ReadInstanceOverview) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (adminrepo.Census, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return adminrepo.Census{}, err
	}

	var census adminrepo.Census
	// The installation's own scope: no tenant, and every table with a tenant column invisible under
	// it. What the census reads is a function, which is the only thing that could answer at all.
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := h.Installation.Census(ctx)
			census = read
			return err
		})
	if err != nil {
		return adminrepo.Census{}, err
	}
	return census, nil
}

// ListInstanceJournal answers the installation's own record, newest first.
type ListInstanceJournal struct{ Writer InstanceWriter }

// JournalPage is one page of it.
type JournalPage struct {
	Entries []adminrepo.InstanceEvent
	Page    adminrepo.PageInfo
}

// Execute reads one page.
func (h ListInstanceJournal) Execute(
	ctx context.Context, actor appshared.ActorContext, cursor string, size int,
) (JournalPage, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return JournalPage{}, err
	}

	var page JournalPage
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			entries, info, err := w.Journal.Page(ctx, cursor, journalPageSize(size))
			page = JournalPage{Entries: entries, Page: info}
			return err
		})
	if err != nil {
		return JournalPage{}, err
	}
	return page, nil
}

func journalPageSize(requested int) int {
	switch {
	case requested < 1:
		return defaultJournalPage
	case requested > maxJournalPage:
		return maxJournalPage
	default:
		return requested
	}
}

func (h ReadInstanceOverview) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadInstanceOverviewName,
		Summary: "How big this installation is and how its workspaces stand: the workspaces by " +
			"state, and how many accounts they hold between them. Counts and states only - the " +
			"contents of a workspace are behind a database policy this answer does not reach " +
			"through.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: instanceOverviewReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadInstanceOverview) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	census, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"workspaces_active":           census.WorkspacesActive,
		"workspaces_suspended":        census.WorkspacesSuspended,
		"workspaces_pending_deletion": census.WorkspacesPendingDeletion,
		"accounts_active":             census.AccountsActive,
		"accounts_total":              census.AccountsTotal,
	}, nil
}

func (h ListInstanceJournal) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListInstanceJournalName,
		Summary: "The installation's own record, newest first: what was provisioned, suspended, " +
			"resumed, deleted, and what its operator changed. Identifiers, slugs, counts and " +
			"moments - never anybody's content, which is what makes it readable at all after the " +
			"workspace it names is gone.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "cursor", Kind: usecase.KindString,
				Description: "Where the last page ended. Absent starts at the newest entry."},
			{Name: "limit", Kind: usecase.KindInt,
				Description: "How many entries, clamped to 200."},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceJournalReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListInstanceJournal) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	page, err := h.Execute(ctx, actor, in.String("cursor"), in.Int("limit"))
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Entries))
	for _, entry := range page.Entries {
		row := usecase.Output{
			"id":          entry.ID.String(),
			"occurred_at": entry.OccurredAt,
			"action":      entry.Action,
		}
		if !entry.TenantID.IsZero() {
			row["tenant_id"] = entry.TenantID.String()
		}
		if entry.TenantSlug != "" {
			row["tenant_slug"] = entry.TenantSlug
		}
		if entry.ActorLabel != "" {
			row["actor_label"] = entry.ActorLabel
		}
		if len(entry.Details) > 0 {
			row["details"] = entry.Details
		}
		rows = append(rows, row)
	}

	info := map[string]any{"next_cursor": nil, "has_more": page.Page.HasMore}
	if page.Page.NextCursor != "" {
		info["next_cursor"] = page.Page.NextCursor
	}
	return usecase.Output{"data": rows, "page": info}, nil
}
