// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package goal is the goals and insight module's use cases (M8): periods and goals with key
// results and check-ins (UC-GOL-01, UC-GOL-02), portfolios with their health and status updates
// (UC-GOL-03), and dashboards whose widgets are the query's own summaries (UC-GOL-04).
//
// A goal lives at the workspace or in a hub and is read with READ there and written with
// WRITE_ITEMS; one a reader may not read answers not found. Every number - a goal's progress, a
// key result fed by a view, a portfolio's roll-up, a widget - is computed for the reader from what
// the reader may see, through the same summary POST /items:aggregate answers (P-05, P-11). A
// note, a description or an update's words are a person's: never in the audit trail (rule 10).
package goal

import (
	"context"
	"errors"
	"strconv"
	"time"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes: goals and portfolios are read and written beside the entries; dashboards as
// saved views are, with the containers' scopes.
const (
	itemsRead       = "items:read"
	itemsWrite      = "items:write"
	containersRead  = "containers:read"
	containersWrite = "containers:write"
)

const (
	periodTarget    = "goal_period"
	goalTarget      = "goal"
	portfolioTarget = "portfolio"
	dashboardTarget = "dashboard"
)

// The stable audit codes (audit.md §2).
const (
	PeriodDefinedAction     audit.Action = "goal_period.defined"
	PeriodClosedAction      audit.Action = "goal_period.closed"
	PeriodsReadAction       audit.Action = "goal_period.read"
	GoalCreatedAction       audit.Action = "goal.created"
	GoalUpdatedAction       audit.Action = "goal.updated"
	GoalDeletedAction       audit.Action = "goal.deleted"
	GoalEntriesLinkedAction audit.Action = "goal.entries_linked"
	GoalEntryUnlinkedAction audit.Action = "goal.entry_unlinked"
	GoalCheckedInAction     audit.Action = "goal.checked_in"
	GoalReadAction          audit.Action = "goal.read"
	PortfolioCreatedAction  audit.Action = "portfolio.created"
	PortfolioUpdatedAction  audit.Action = "portfolio.updated"
	PortfolioDeletedAction  audit.Action = "portfolio.deleted"
	PortfolioReportedAction audit.Action = "portfolio.status_reported"
	PortfolioReadAction     audit.Action = "portfolio.read"
	DashboardCreatedAction  audit.Action = "dashboard.created"
	DashboardUpdatedAction  audit.Action = "dashboard.updated"
	DashboardDeletedAction  audit.Action = "dashboard.deleted"
	DashboardReadAction     audit.Action = "dashboard.read"
	maxPeriods                           = 200
	maxGoals                             = 2000
	maxPortfolios                        = 500
	maxDashboards                        = 1000
	maxCheckIns                          = 200
	maxUpdates                           = 200
	maxLiveResults                       = 100
)

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Reader answers which of many paths a reader may act on, without recording a refusal.
type Reader interface {
	Permitted(ctx context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error)
}

// Steward is what every use case of the module shares.
type Steward struct {
	Store      goalrepo.Store
	References workrepo.References
	Containers workrepo.Containers
	// Summaries is POST /items:aggregate's use case and ViewSummaries a saved view's: every number
	// of the module is one of theirs, run as the reader.
	Summaries     usecase.Handler
	ViewSummaries usecase.Handler
	Authorizer    Authorizer
	Reader        Reader
	Audit         audit.Sink
	UnitOfWork    persistence.UnitOfWork
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Text          text.Normalizer
}

func (s Steward) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID,
	now time.Time, changes ...audit.Change,
) error {
	return s.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// placePath is the path of the workspace, or of a hub.
func placePath(hub shared.ID) []identity.Scope {
	if hub.IsZero() {
		return []identity.Scope{identity.TenantScope()}
	}
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hub)}
}

func summaryPath(c workrepo.ContainerSummary) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if c.Kind == work.ContainerHub {
		return append(path, identity.HubScope(c.ID))
	}
	if !c.ParentID.IsZero() {
		path = append(path, identity.HubScope(c.ParentID))
	}
	return append(path, identity.CollectionScope(c.ID))
}

// authorize asks a permission at a place, recording a refusal.
func (s Steward) authorize(
	ctx context.Context, actor appshared.ActorContext, permission service.Permission, path []identity.Scope,
	action audit.Action, scope, target string, id shared.ID,
) error {
	return s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: path, Action: action, TokenScope: scope, TargetType: target, TargetID: id,
	})
}

// permitted answers, per path, whether the reader holds a permission there; nothing is recorded.
func (s Steward) permitted(
	ctx context.Context, actor appshared.ActorContext, permission service.Permission, paths [][]identity.Scope,
	action audit.Action, scope, target string,
) ([]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	allowed, err := s.Reader.Permitted(ctx, actor, access.Request{
		Permission: permission, Action: action, TokenScope: scope, TargetType: target,
	}, paths)
	if err != nil {
		return nil, err
	}
	for len(allowed) < len(paths) {
		allowed = append(allowed, false)
	}
	return allowed, nil
}

// readsPlaces answers which of the places (the workspace as the zero hub) the reader reads.
func (s Steward) readsPlaces(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID, scope, target string) (map[shared.ID]bool, error) {
	distinct := []shared.ID{}
	seen := map[shared.ID]bool{}
	for _, h := range hubs {
		if !seen[h] {
			seen[h] = true
			distinct = append(distinct, h)
		}
	}
	paths := make([][]identity.Scope, len(distinct))
	for i, h := range distinct {
		paths[i] = placePath(h)
	}
	allowed, err := s.permitted(ctx, actor, service.PermissionRead, paths, GoalReadAction, scope, target)
	if err != nil {
		return nil, err
	}
	out := make(map[shared.ID]bool, len(distinct))
	for i, h := range distinct {
		out[h] = allowed[i]
	}
	return out, nil
}

// hub checks that an identifier names a hub of the workspace.
func (s Steward) hub(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	if id.IsZero() {
		return nil
	}
	var c work.Container
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		c, err = s.Containers.Find(ctx, id)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && c.Type != work.ContainerHub) {
		return shared.ErrNotFound.WithDetail("containers.not_found").WithFields(shared.FieldError{Path: "/hub_id", Code: "containers.not_found"})
	}
	return err
}

func person(actor appshared.ActorContext) error {
	if !actor.IsAuthenticated() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	return nil
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

func notFound(code string) error { return shared.ErrNotFound.WithDetail(code) }

func versionOf(in usecase.Input, required bool) (int, error) {
	switch v := in["expected_version"].(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v == float64(int(v)) {
			return int(v), nil
		}
	case nil:
		if !required {
			return 0, nil
		}
	}
	return 0, invalid("/expected_version", "goals.version_required")
}

func optionalID(in usecase.Input, field string) (shared.ID, error) {
	if in.String(field) == "" {
		return "", nil
	}
	return in.ID(field)
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func intOf(v any) (int, bool) {
	n, ok := number(v)
	if !ok || n != float64(int(n)) {
		return 0, false
	}
	return int(n), true
}

func dayOf(in map[string]any, field, path string) (*domain.Day, error) {
	raw, _ := in[field].(string)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, invalid(path, "goals.day_invalid")
	}
	d := domain.Day(t.Unix() / 86400)
	return &d, nil
}

func idText(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

func dayText(d *domain.Day) any {
	if d == nil {
		return nil
	}
	return d.String()
}

// descriptor is the shape every use case of the module shares.
func descriptor(d usecase.Descriptor, target string) usecase.Descriptor {
	d.Module = module.Goals
	if d.Audit.Action != "" {
		d.Audit.TargetType, d.Audit.Severity, d.Audit.Required = target, audit.SeverityInfo, !d.ReadOnly
	}
	if d.Activity.Exempt == "" {
		if d.ReadOnly {
			d.Activity.Exempt = "a read changes nothing."
		} else {
			d.Activity.Exempt = "goals, portfolios and dashboards are not entries; their changes are in the audit trail."
		}
	}
	return d
}

func itoa(i int) string { return strconv.Itoa(i) }
