// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The catalogue names.
const (
	ListPortfoliosName        = "ListPortfolios"
	ReadPortfolioName         = "ReadPortfolio"
	CreatePortfolioName       = "CreatePortfolio"
	UpdatePortfolioName       = "UpdatePortfolio"
	DeletePortfolioName       = "DeletePortfolio"
	ReportPortfolioStatusName = "ReportPortfolioStatus"
	ListPortfolioUpdatesName  = "ListPortfolioUpdates"
)

// projects answers the containers among a portfolio's projects that the reader may read.
func (s Steward) projects(ctx context.Context, actor appshared.ActorContext, ids []shared.ID) (map[shared.ID]workrepo.ContainerSummary, error) {
	out := map[shared.ID]workrepo.ContainerSummary{}
	if len(ids) == 0 {
		return out, nil
	}
	var summaries []workrepo.ContainerSummary
	if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = s.References.Containers(ctx, ids)
		return err
	}); err != nil {
		return nil, err
	}
	paths := make([][]identity.Scope, len(summaries))
	for i, c := range summaries {
		paths[i] = summaryPath(c)
	}
	allowed, err := s.permitted(ctx, actor, service.PermissionRead, paths, PortfolioReadAction, itemsRead, portfolioTarget)
	if err != nil {
		return nil, err
	}
	for i, c := range summaries {
		if allowed[i] {
			out[c.ID] = c
		}
	}
	return out, nil
}

// PortfolioOutput is a portfolio on the wire with the projects the reader may read; rolled up,
// each row carries the project's entries and the share completed, as the reader counts them.
func (s Steward) PortfolioOutput(ctx context.Context, actor appshared.ActorContext, p domain.Portfolio, rollUp bool) (usecase.Output, error) {
	ids := make([]shared.ID, len(p.Items))
	for i, item := range p.Items {
		ids[i] = item.ContainerID
	}
	readable, err := s.projects(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	rows := []usecase.Output{}
	total, done := 0, 0
	costs := map[shared.ID]bool{}
	if rollUp && s.Budgets != nil {
		if costs, err = s.managesCosts(ctx, actor, readable); err != nil {
			return nil, err
		}
	}
	var sum budgetSum
	for _, item := range p.Items {
		c, ok := readable[item.ContainerID]
		if !ok {
			continue
		}
		row := usecase.Output{
			"container_id": c.ID.String(), "kind": string(c.Kind), "name": c.Name, "archived": c.Archived,
			"start_on": dayText(item.StartOn), "end_on": dayText(item.EndOn), "status": string(item.Status),
			"entries": nil, "done": nil, "progress": nil,
		}
		if rollUp && s.Summaries != nil {
			count, completed, ok := s.completion(ctx, actor, c.ID)
			if ok {
				row["entries"], row["done"] = count, completed
				total, done = total+count, done+completed
				if count > 0 {
					row["progress"] = float64(completed) / float64(count)
				}
			}
		}
		if costs[c.ID] {
			if burn, ok := s.burn(ctx, actor, c.ID); ok {
				row["budget"] = burn
				sum.add(burn)
			}
		}
		rows = append(rows, row)
	}
	out := usecase.Output{
		"id": p.ID.String(), "parent_id": idText(p.ParentID), "name": p.Name, "description": p.Description,
		"owner_id": idText(p.OwnerID), "status": string(p.Status), "items": rows, "version": p.Version,
		"progress": nil,
	}
	if rollUp && total > 0 {
		out["progress"] = float64(done) / float64(total)
	}
	if sum.projects > 0 {
		out["budget"] = sum.output()
	}
	return out, nil
}

// managesCosts answers which projects the reader manages costs at - MANAGE_COSTS, which the
// shipped roles reach through MANAGE_MEMBERS -, asked without recording a refusal: a budget left
// out is no attempt at anything.
func (s Steward) managesCosts(ctx context.Context, actor appshared.ActorContext, readable map[shared.ID]workrepo.ContainerSummary) (map[shared.ID]bool, error) {
	out := map[shared.ID]bool{}
	ids := make([]shared.ID, 0, len(readable))
	paths := make([][]identity.Scope, 0, len(readable))
	for id, c := range readable {
		ids = append(ids, id)
		paths = append(paths, summaryPath(c))
	}
	if len(paths) == 0 {
		return out, nil
	}
	allowed, err := s.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionManageCosts, Alternative: service.PermissionManageMembers,
		Action: PortfolioReadAction, TokenScope: itemsRead, TargetType: portfolioTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		out[id] = i < len(allowed) && allowed[i]
	}
	return out, nil
}

// burn reads a project's budget and its burn through the time module's own use case, as the
// reader, so that the portfolio's row agrees with the project's budget (P-11).
func (s Steward) burn(ctx context.Context, actor appshared.ActorContext, container shared.ID) (usecase.Output, bool) {
	out, err := s.Budgets.Invoke(ctx, actor, usecase.Input{"plan_id": container.String()})
	if err != nil {
		return nil, false
	}
	budgeted, _ := out["budgeted"].(bool)
	if !budgeted {
		return nil, false
	}
	keep := usecase.Output{}
	for _, key := range []string{"budget_minutes", "budget_amount_minor", "currency", "minutes", "amount_minor", "unpriced_minutes", "over_time", "over_money"} {
		keep[key] = out[key]
	}
	return keep, true
}

// budgetSum rolls the projects' budgets up: minutes always, money only while every budget with
// money is in one currency.
type budgetSum struct {
	projects                         int
	budgetMinutes, minutes, unpriced int64
	budgetMinor, minor               int64
	currency                         string
	mixed                            bool
}

func (b *budgetSum) add(burn usecase.Output) {
	b.projects++
	n := func(key string) int64 { v, _ := intOf(burn[key]); return int64(v) }
	b.budgetMinutes += n("budget_minutes")
	b.minutes += n("minutes")
	b.unpriced += n("unpriced_minutes")
	currency, _ := burn["currency"].(string)
	if currency == "" {
		return
	}
	if b.currency != "" && b.currency != currency {
		b.mixed = true
	}
	if b.currency == "" {
		b.currency = currency
	}
	b.budgetMinor += n("budget_amount_minor")
	b.minor += n("amount_minor")
}

func (b budgetSum) output() usecase.Output {
	out := usecase.Output{
		"projects": b.projects, "budget_minutes": b.budgetMinutes, "minutes": b.minutes,
		"unpriced_minutes": b.unpriced, "currency": b.currency, "mixed_currencies": b.mixed,
		"budget_amount_minor": nil, "amount_minor": nil,
	}
	if !b.mixed && b.currency != "" {
		out["budget_amount_minor"], out["amount_minor"] = b.budgetMinor, b.minor
	}
	return out
}

// completion counts a project's entries and the completed ones through the query's own summary,
// so the row agrees with the project's list (P-11).
func (s Steward) completion(ctx context.Context, actor appshared.ActorContext, container shared.ID) (int, int, bool) {
	out, err := s.Summaries.Invoke(ctx, actor, usecase.Input{
		"scope_container_id": container.String(),
		"aggregate":          map[string]any{"function": "COUNT", "group_by": "is_completed"},
	})
	if err != nil {
		return 0, 0, false
	}
	rows, _ := out["data"].([]usecase.Output)
	total, done := 0, 0
	for _, row := range rows {
		n, _ := intOf(row["count"])
		total += n
		if row["group"] == "true" {
			done += n
		}
	}
	return total, done, true
}

func (s Steward) readPortfolio(ctx context.Context, actor appshared.ActorContext, id shared.ID, permission service.Permission, action audit.Action) (domain.Portfolio, error) {
	if err := person(actor); err != nil {
		return domain.Portfolio{}, err
	}
	scope := itemsRead
	if permission != service.PermissionRead {
		scope = itemsWrite
	}
	if err := s.authorize(ctx, actor, permission, workspace, action, scope, portfolioTarget, id); err != nil {
		return domain.Portfolio{}, err
	}
	var p domain.Portfolio
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		p, err = s.Store.Portfolio(ctx, id)
		return err
	})
	return p, err
}

// ListPortfolios answers the workspace's portfolios to its readers, each with the projects the
// reader may read; the roll-up is ReadPortfolio's.
type ListPortfolios struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListPortfolios) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPortfoliosName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "The workspace's portfolios by name, nested by parent_id, each with its projects the caller " +
			"may read, their dates and health. Needs READ at the workspace.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Audit: usecase.AuditDeclaration{Action: PortfolioReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			if err := s.authorize(ctx, actor, service.PermissionRead, workspace, PortfolioReadAction, itemsRead, portfolioTarget, ""); err != nil {
				return nil, err
			}
			var portfolios []domain.Portfolio
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				portfolios, err = s.Store.Portfolios(ctx, maxPortfolios)
				return err
			}); err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(portfolios))
			for _, p := range portfolios {
				out, err := s.PortfolioOutput(ctx, actor, p, false)
				if err != nil {
					return nil, err
				}
				data = append(data, out)
			}
			return usecase.Output{"data": data}, nil
		}),
	}, portfolioTarget)
}

// ReadPortfolio answers one portfolio with its projects rolled up for the reader, and its latest
// status update.
type ReadPortfolio struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadPortfolio) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPortfolioName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "One portfolio as a table: each project the caller may read with its dates, health, how many " +
			"entries it holds and how many are completed, as the caller's own query counts them; the " +
			"portfolio's progress over them; its latest status update; and, for a caller who manages costs " +
			"at a project, its budget and burn and their roll-up. Needs READ at the workspace.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."}},
		Audit: usecase.AuditDeclaration{Action: PortfolioReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			p, err := s.readPortfolio(ctx, actor, id, service.PermissionRead, PortfolioReadAction)
			if err != nil {
				return nil, err
			}
			out, err := s.PortfolioOutput(ctx, actor, p, true)
			if err != nil {
				return nil, err
			}
			var updates []domain.StatusUpdate
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				updates, err = s.Store.StatusUpdates(ctx, id, 1)
				return err
			}); err != nil {
				return nil, err
			}
			out["latest_update"] = nil
			if len(updates) == 1 {
				out["latest_update"] = updateOutput(updates[0])
			}
			return out, nil
		}),
	}, portfolioTarget)
}

// portfolioFields reads a portfolio's fields and projects.
func (s Steward) portfolioFields(in usecase.Input) (domain.Portfolio, error) {
	p := domain.Portfolio{Name: in.String("name"), Description: in.String("description"), Status: domain.Status(in.String("status"))}
	var err error
	if p.ParentID, err = optionalID(in, "parent_id"); err != nil {
		return domain.Portfolio{}, err
	}
	if p.OwnerID, err = optionalID(in, "owner_id"); err != nil {
		return domain.Portfolio{}, err
	}
	raw, _ := in["items"].([]any)
	for i, item := range raw {
		at := "/items/" + itoa(i)
		doc, ok := item.(map[string]any)
		if !ok {
			return domain.Portfolio{}, invalid(at, "portfolios.item_malformed")
		}
		container, err := shared.ParseID(stringOf(doc["container_id"]))
		if err != nil {
			return domain.Portfolio{}, invalid(at+"/container_id", "portfolios.item_malformed")
		}
		start, err := dayOf(doc, "start_on", at+"/start_on")
		if err != nil {
			return domain.Portfolio{}, err
		}
		end, err := dayOf(doc, "end_on", at+"/end_on")
		if err != nil {
			return domain.Portfolio{}, err
		}
		p.Items = append(p.Items, domain.PortfolioItem{
			ContainerID: container, StartOn: start, EndOn: end, Status: domain.Status(stringOf(doc["status"])),
		})
	}
	return domain.CheckPortfolio(p, s.Text)
}

// placeable checks that the writer reads every project and that the parent may hold the
// portfolio.
func (s Steward) placeable(ctx context.Context, actor appshared.ActorContext, p domain.Portfolio) error {
	ids := make([]shared.ID, len(p.Items))
	for i, item := range p.Items {
		ids[i] = item.ContainerID
	}
	readable, err := s.projects(ctx, actor, ids)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, ok := readable[id]; !ok {
			return shared.ErrNotFound.WithDetail("containers.not_found").
				WithFields(shared.FieldError{Path: "/items/" + itoa(i) + "/container_id", Code: "containers.not_found"})
		}
	}
	if p.ParentID.IsZero() {
		return nil
	}
	var chain []shared.ID
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for next := p.ParentID; !next.IsZero() && len(chain) <= domain.MaxPortfolioNested; {
			chain = append(chain, next)
			parent, err := s.Store.Portfolio(ctx, next)
			if err != nil {
				return shared.ErrNotFound.WithDetail("portfolios.parent_not_found").
					WithFields(shared.FieldError{Path: "/parent_id", Code: "portfolios.parent_not_found"})
			}
			next = parent.ParentID
		}
		return nil
	})
	if err != nil {
		return err
	}
	return domain.PortfolioNesting(p.ID, chain)
}

var portfolioInput = []usecase.Field{
	{Name: "name", Kind: usecase.KindString, Required: true, Description: "The portfolio's name."},
	{Name: "description", Kind: usecase.KindString, Description: "What it holds together."},
	{Name: "owner_id", Kind: usecase.KindID, Description: "Who owns it; the caller without."},
	{Name: "parent_id", Kind: usecase.KindID, Description: "The portfolio it sits in, at most six deep."},
	{Name: "status", Kind: usecase.KindString, Enum: []string{"NONE", "ON_TRACK", "AT_RISK", "OFF_TRACK"}, Description: "Its health; a status update sets it too."},
	{Name: "items", Kind: usecase.KindList, Description: "At most 200 projects, in order: container_id (a hub or a collection the caller may read), start_on, end_on, status."},
}

// CreatePortfolio keeps hubs and collections together, with WRITE_ITEMS at the workspace.
type CreatePortfolio struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h CreatePortfolio) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePortfolioName, Permission: service.PermissionWriteItems,
		Summary: "Creates a portfolio of hubs and collections the caller may read, with dates and health per " +
			"project and an optional parent portfolio. Needs WRITE_ITEMS at the workspace.",
		SideEffects: "Writes the portfolio and an audit entry.", TokenScope: itemsWrite, Input: portfolioInput,
		Audit: usecase.AuditDeclaration{Action: PortfolioCreatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			p, err := s.portfolioFields(in)
			if err != nil {
				return nil, err
			}
			if err := s.authorize(ctx, actor, service.PermissionWriteItems, workspace, PortfolioCreatedAction, itemsWrite, portfolioTarget, ""); err != nil {
				return nil, err
			}
			p.ID = s.IDs.NewID()
			if p.OwnerID.IsZero() {
				p.OwnerID = actor.AccountID
			}
			if err := s.placeable(ctx, actor, p); err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.InsertPortfolio(ctx, p, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, PortfolioCreatedAction, portfolioTarget, p.ID, now)
			}); err != nil {
				return nil, err
			}
			p.Version = 1
			return s.PortfolioOutput(ctx, actor, p, false)
		}),
	}, portfolioTarget)
}

// UpdatePortfolio replaces a portfolio's fields and projects, held to the version read. A project
// the writer may not read stays as it was: nobody drops what they cannot see.
type UpdatePortfolio struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h UpdatePortfolio) Descriptor() usecase.Descriptor {
	input := append([]usecase.Field{
		{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."},
		{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
	}, portfolioInput...)
	return descriptor(usecase.Descriptor{
		Name: UpdatePortfolioName, Permission: service.PermissionWriteItems,
		Summary: "Replaces a portfolio's name, description, owner, parent, health and projects, held to the " +
			"version read; a project the caller may not read is kept as it was. Needs WRITE_ITEMS at the workspace.",
		SideEffects: "Writes the portfolio and an audit entry.", TokenScope: itemsWrite, Input: input,
		Audit: usecase.AuditDeclaration{Action: PortfolioUpdatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			next, err := s.portfolioFields(in)
			if err != nil {
				return nil, err
			}
			current, err := s.readPortfolio(ctx, actor, id, service.PermissionWriteItems, PortfolioUpdatedAction)
			if err != nil {
				return nil, err
			}
			next.ID = id
			if next.OwnerID.IsZero() {
				next.OwnerID = current.OwnerID
			}
			if err := s.placeable(ctx, actor, next); err != nil {
				return nil, err
			}
			hidden, err := s.hiddenProjects(ctx, actor, current, next)
			if err != nil {
				return nil, err
			}
			next.Items = append(next.Items, hidden...)
			if _, err := domain.CheckPortfolio(next, s.Text); err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.UpdatePortfolio(ctx, next, expected, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, PortfolioUpdatedAction, portfolioTarget, id, now)
			}); err != nil {
				return nil, err
			}
			next.Version = expected + 1
			return s.PortfolioOutput(ctx, actor, next, false)
		}),
	}, portfolioTarget)
}

// hiddenProjects answers the current projects the writer may not read and did not name.
func (s Steward) hiddenProjects(ctx context.Context, actor appshared.ActorContext, current, next domain.Portfolio) ([]domain.PortfolioItem, error) {
	ids := make([]shared.ID, len(current.Items))
	for i, item := range current.Items {
		ids[i] = item.ContainerID
	}
	readable, err := s.projects(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	named := map[shared.ID]bool{}
	for _, item := range next.Items {
		named[item.ContainerID] = true
	}
	var hidden []domain.PortfolioItem
	for _, item := range current.Items {
		if _, ok := readable[item.ContainerID]; !ok && !named[item.ContainerID] {
			hidden = append(hidden, item)
		}
	}
	return hidden, nil
}

// DeletePortfolio removes a portfolio and its updates; its portfolios move to its parent and its
// hubs and collections stay as they are.
type DeletePortfolio struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h DeletePortfolio) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeletePortfolioName, Permission: service.PermissionWriteItems, Destructive: true,
		Summary: "Deletes a portfolio and its status updates, held to the version read; the portfolios in it " +
			"move up to its parent, and its hubs and collections stay. Needs WRITE_ITEMS at the workspace.",
		SideEffects: "Deletes the portfolio and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: PortfolioDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			p, err := s.readPortfolio(ctx, actor, id, service.PermissionWriteItems, PortfolioDeletedAction)
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.DeletePortfolio(ctx, p, expected); err != nil {
					return err
				}
				return s.record(ctx, actor, PortfolioDeletedAction, portfolioTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return usecase.Output{"deleted": true}, nil
		}),
	}, portfolioTarget)
}

func updateOutput(u domain.StatusUpdate) usecase.Output {
	return usecase.Output{
		"id": u.ID.String(), "portfolio_id": u.PortfolioID.String(), "author_id": idText(u.AuthorID),
		"status": string(u.Status), "summary": u.Summary, "created_at": u.At,
	}
}

// ReportPortfolioStatus writes a status update: the health it gives the portfolio and what its
// writer says about it.
type ReportPortfolioStatus struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReportPortfolioStatus) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReportPortfolioStatusName, Permission: service.PermissionWriteItems,
		Summary: "Writes a portfolio's status update - ON_TRACK, AT_RISK or OFF_TRACK and a summary of at most " +
			"10000 characters -, which becomes the portfolio's health. Kept as history. Needs WRITE_ITEMS at the workspace.",
		SideEffects: "Writes the update, sets the portfolio's health and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."},
			{Name: "status", Kind: usecase.KindString, Required: true, Enum: []string{"ON_TRACK", "AT_RISK", "OFF_TRACK"}, Description: "Its health."},
			{Name: "summary", Kind: usecase.KindString, Required: true, Description: "What stands, what changed, what is needed."},
		},
		Audit: usecase.AuditDeclaration{Action: PortfolioReportedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			u, err := domain.CheckStatusUpdate(domain.StatusUpdate{
				PortfolioID: id, AuthorID: actor.AccountID, Status: domain.Status(in.String("status")), Summary: in.String("summary"),
			}, s.Text)
			if err != nil {
				return nil, err
			}
			p, err := s.readPortfolio(ctx, actor, id, service.PermissionWriteItems, PortfolioReportedAction)
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			u.ID, u.At = s.IDs.NewID(), now
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.AddStatusUpdate(ctx, u); err != nil {
					return err
				}
				expected := p.Version
				p.Status = u.Status
				if err := s.Store.UpdatePortfolio(ctx, p, expected, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, PortfolioReportedAction, portfolioTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return updateOutput(u), nil
		}),
	}, portfolioTarget)
}

// ListPortfolioUpdates answers a portfolio's status updates, the latest first.
type ListPortfolioUpdates struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListPortfolioUpdates) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPortfolioUpdatesName, Permission: service.PermissionRead, ReadOnly: true,
		Summary:     "A portfolio's status updates, the latest first, at most 200. Needs READ at the workspace.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."}},
		Audit: usecase.AuditDeclaration{Action: PortfolioReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			if _, err := s.readPortfolio(ctx, actor, id, service.PermissionRead, PortfolioReadAction); err != nil {
				return nil, err
			}
			var updates []domain.StatusUpdate
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				updates, err = s.Store.StatusUpdates(ctx, id, maxUpdates)
				return err
			}); err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(updates))
			for _, u := range updates {
				data = append(data, updateOutput(u))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, portfolioTarget)
}
