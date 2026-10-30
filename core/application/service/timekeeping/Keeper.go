// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package timekeeping is the time module's use cases (M7): a person records time by hand or with a
// timer (UC-TIM-01), fills in and hands in a week (UC-TIM-02), an approver approves it or sends it
// back and the workspace locks a period (UC-TIM-03), a report sums the time and an entry's estimate
// meets its actuals (UC-TIM-04), and the time goes to accounting as a file (UC-TIM-05).
//
// A person's own time is theirs to read and write. Other people's time is read where the reader
// approves it - APPROVE_TIME, or MANAGE_MEMBERS which the shipped roles hold, at the workspace or
// at the hub or collection the entries lie in - and, where the workspace chose aggregated-only
// visibility, only as sums that name nobody. A note is a person's words: never in the audit trail
// (rule 10).
package timekeeping

import (
	"context"
	"strconv"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	deskrepo "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	timerepo "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes: time is recorded beside the entries, so it is read and written as they are;
// the workspace's kinds of work and policy are its configuration.
const (
	itemsRead       = "items:read"
	itemsWrite      = "items:write"
	workspaceManage = "workspace:manage"
)

const (
	recordTarget   = "time_record"
	timerTarget    = "time_timer"
	sheetTarget    = "timesheet"
	workTypeTarget = "time_work_type"
	policyTarget   = "time_policy"
	itemTarget     = "item"
)

// The stable audit codes (audit.md §2).
const (
	RecordLoggedAction    audit.Action = "time_record.logged"
	RecordUpdatedAction   audit.Action = "time_record.updated"
	RecordDeletedAction   audit.Action = "time_record.deleted"
	RecordCorrectedAction audit.Action = "time_record.corrected"
	RecordsReadAction     audit.Action = "time_record.read"
	RecordsExportedAction audit.Action = "time_record.exported"
	WeekCopiedAction      audit.Action = "timesheet.copied"
	TimerStartedAction    audit.Action = "time_timer.started"
	TimerStoppedAction    audit.Action = "time_timer.stopped"
	TimerDiscardedAction  audit.Action = "time_timer.discarded"
	SheetSubmittedAction  audit.Action = "timesheet.submitted"
	SheetWithdrawnAction  audit.Action = "timesheet.withdrawn"
	SheetApprovedAction   audit.Action = "timesheet.approved"
	SheetRejectedAction   audit.Action = "timesheet.rejected"
	SheetReadAction       audit.Action = "timesheet.read"
	WorkTypeDefinedAction audit.Action = "time_work_type.defined"
	WorkTypesReadAction   audit.Action = "time_work_type.read"
	PolicySetAction       audit.Action = "time_policy.set"
	PolicyReadAction      audit.Action = "time_policy.read"
	ReportReadAction      audit.Action = "time_report.read"
	ActualsReadAction     audit.Action = "time_actuals.read"
	itemReadAction        audit.Action = "item.read"
	maxDaysOfARange                    = 366
	maxRecordsListed                   = 2000
	maxRecordsExported                 = 50000
	maxLinesSummed                     = 100000
	maxSheetsListed                    = 100
	maxEntriesOfAPlan                  = 5000
	maxRecordsOfAWeek                  = 500
)

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Reader answers which of many paths a reader may act on, without recording a refusal.
type Reader interface {
	Permitted(ctx context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error)
}

// RoleHolder answers the role keys a person holds along a path, the most specific first.
type RoleHolder interface {
	RolesBy(ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []identity.Scope) ([]string, error)
}

// Keeper is what every time use case shares.
type Keeper struct {
	Store     timerepo.Store
	Resources timerepo.Resources
	// Roles answers the roles a person holds where their time was spent, which a role's cost rate
	// prices - the authoriser's answer, so that a private hub counts as everywhere else (rule 2).
	Roles RoleHolder
	// Calendars names the business calendars a workload counts public holidays on; nil counts none.
	Calendars  deskrepo.Calendars
	Items      workrepo.Items
	Containers workrepo.Containers
	References workrepo.References
	Plans      planrepo.Plans
	Estimates  planrepo.Sprints
	Authorizer Authorizer
	Reader     Reader
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Changes and HLC tell a person's devices of their records and timer, and every member's of
	// the kinds of work and the policy; nil records nothing.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

func (k Keeper) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID,
	now time.Time, changes ...audit.Change,
) error {
	return k.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// person refuses a caller who is not somebody: time is always a person's.
func person(actor appshared.ActorContext) error {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	return nil
}

// entryReadable reads an entry time is recorded on and asks READ on it: a person records time on
// what they may see. It answers the entry's place.
func (k Keeper) entryReadable(ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action) error {
	var summaries []workrepo.EntrySummary
	if err := k.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = k.References.Entries(ctx, []shared.ID{id})
		return err
	}); err != nil {
		return err
	}
	if len(summaries) == 0 {
		return appshared.ItemNotFound(id)
	}
	s := summaries[0]
	return k.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: entryPath(s.HubID, s.CollectionID, s.ID),
		Action: action, TokenScope: itemsRead, TargetType: itemTarget, TargetID: s.ID,
		On: access.ItemSubject{Does: service.ItemRead, ID: s.ID},
	})
}

// approverRequest is the question of approving other people's time at a place.
func approverRequest(action audit.Action, scope string) access.Request {
	return access.Request{
		Permission: service.PermissionApproveTime, Alternative: service.PermissionManageMembers,
		Action: action, TokenScope: scope, TargetType: sheetTarget,
	}
}

// approvesAll reports whether the reader approves time at every place: the workspace, or each hub
// or collection a record's entry lies in. A record on no entry, or on one purged since, is the
// workspace's.
func (k Keeper) approvesAll(ctx context.Context, actor appshared.ActorContext, places []place, action audit.Action, scope string) (bool, error) {
	allowed, err := k.approves(ctx, actor, places, action, scope)
	if err != nil {
		return false, err
	}
	for _, ok := range allowed {
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// place is where a record's entry lies; the zero place is the workspace.
type place struct{ hub, collection shared.ID }

func (p place) path() []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !p.hub.IsZero() {
		path = append(path, identity.HubScope(p.hub))
	}
	if !p.collection.IsZero() {
		path = append(path, identity.CollectionScope(p.collection))
	}
	return path
}

// approves answers, per place, whether the reader approves time there.
func (k Keeper) approves(ctx context.Context, actor appshared.ActorContext, places []place, action audit.Action, scope string) ([]bool, error) {
	if len(places) == 0 {
		return nil, nil
	}
	paths := make([][]identity.Scope, len(places))
	for i, p := range places {
		paths[i] = p.path()
	}
	allowed, err := k.Reader.Permitted(ctx, actor, approverRequest(action, scope), paths)
	if err != nil {
		return nil, err
	}
	for len(allowed) < len(places) {
		allowed = append(allowed, false)
	}
	return allowed, nil
}

// refuseAsApprover records and answers the refusal of somebody who does not approve the time,
// asked at the workspace.
func (k Keeper) refuseAsApprover(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope string, target shared.ID) error {
	request := approverRequest(action, scope)
	request.Path, request.TargetID = []identity.Scope{identity.TenantScope()}, target
	if err := k.Authorizer.Authorize(ctx, actor, request); err != nil {
		return err
	}
	return nil
}

func (k Keeper) policy(ctx context.Context, actor appshared.ActorContext) (domain.Policy, error) {
	var policy domain.Policy
	err := k.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		policy, err = k.Store.Policy(ctx)
		return err
	})
	return policy, err
}

// weekState answers the state of a person's week holding a day.
func (k Keeper) weekState(ctx context.Context, account shared.ID, day domain.Day) (domain.SheetState, error) {
	sheet, _, err := k.Store.Sheet(ctx, account, day)
	if err != nil {
		return "", err
	}
	return sheet.State, nil
}

// mayWrite holds a write of a person's record on a day to the locked period and the week's state,
// inside the caller's transaction.
func (k Keeper) mayWrite(ctx context.Context, policy domain.Policy, account shared.ID, day domain.Day) error {
	state, err := k.weekState(ctx, account, day)
	if err != nil {
		return err
	}
	return policy.MayWrite(day, state)
}

// workType checks a kind of work names one the workspace has and has not archived, and answers it.
func (k Keeper) workType(ctx context.Context, key string) (domain.WorkType, error) {
	if key == "" {
		return domain.WorkType{}, nil
	}
	kinds, err := k.Store.WorkTypes(ctx)
	if err != nil {
		return domain.WorkType{}, err
	}
	for _, w := range kinds {
		if w.Key == key && !w.Archived {
			return w, nil
		}
	}
	return domain.WorkType{}, invalid("/work_type", "work_types.not_found")
}

func entryPath(hub, collection, entry shared.ID) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !hub.IsZero() {
		path = append(path, identity.HubScope(hub))
	}
	return append(path, identity.CollectionScope(collection), identity.ItemScope(entry))
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// dayRange reads a range of days, at most maxDaysOfARange long.
func dayRange(in usecase.Input) (domain.Day, domain.Day, error) {
	from, err := dayField(in, "from", true)
	if err != nil {
		return 0, 0, err
	}
	to, err := dayField(in, "to", true)
	if err != nil {
		return 0, 0, err
	}
	if to < from {
		return 0, 0, invalid("/to", "time.range_reversed")
	}
	if int(to-from) >= maxDaysOfARange {
		return 0, 0, shared.ErrValidation.WithDetail("time.range_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(maxDaysOfARange)}).
			WithFields(shared.FieldError{Path: "/to", Code: "time.range_too_long"})
	}
	return from, to, nil
}

func dayField(in usecase.Input, field string, required bool) (domain.Day, error) {
	raw := in.String(field)
	if raw == "" {
		if required {
			return 0, invalid("/"+field, "time.day_required")
		}
		return 0, nil
	}
	d, ok := domain.ParseDay(raw)
	if !ok {
		return 0, invalid("/"+field, "time.day_invalid")
	}
	return d, nil
}

func optionalID(in usecase.Input, field string) (shared.ID, error) {
	if in.String(field) == "" {
		return "", nil
	}
	return in.ID(field)
}

func optionalBool(in usecase.Input, field string) *bool {
	if v, ok := in[field].(bool); ok {
		return &v
	}
	return nil
}

func zoneOf(in usecase.Input) (*time.Location, error) {
	name := in.String("zone")
	if name == "" {
		return time.UTC, nil
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return nil, invalid("/zone", "time.zone_unknown")
	}
	return zone, nil
}

func intField(in usecase.Input, field string) (int, bool) {
	switch v := in[field].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		if v != float64(int(v)) {
			return 0, false
		}
		return int(v), true
	}
	return 0, false
}

// descriptor is the shape every time use case shares.
func descriptor(d usecase.Descriptor, target string) usecase.Descriptor {
	d.Module = module.Time
	if d.Audit.Action != "" {
		d.Audit.TargetType, d.Audit.Severity, d.Audit.Required = target, audit.SeverityInfo, !d.ReadOnly
	}
	if d.Activity.Exempt == "" {
		if d.ReadOnly {
			d.Activity.Exempt = "a read changes nothing."
		} else {
			d.Activity.Exempt = "time is recorded beside the entry, in the audit trail, not in its activity."
		}
	}
	return d
}

func idText(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

// RecordOutput is a record on the wire; the note goes to whoever may read the record.
func RecordOutput(r domain.Record) usecase.Output {
	out := usecase.Output{
		"id": r.ID.String(), "account_id": r.AccountID.String(), "entry_id": idText(r.EntryID),
		"work_type": r.WorkType, "day": r.Day.String(), "minutes": r.Minutes, "note": r.Note,
		"billable": r.Billable, "source": string(r.Source), "corrects": idText(r.Corrects),
		"created_at": r.CreatedAt, "updated_at": r.UpdatedAt, "version": r.Version,
	}
	if r.StartedAt != nil {
		out["started_at"] = *r.StartedAt
	} else {
		out["started_at"] = nil
	}
	return out
}
