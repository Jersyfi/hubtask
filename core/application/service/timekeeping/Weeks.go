// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"strconv"
	"time"

	timerepo "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadTimesheetName     = "ReadTimesheet"
	CopyTimesheetWeekName = "CopyTimesheetWeek"
	SubmitTimesheetName   = "SubmitTimesheet"
	WithdrawTimesheetName = "WithdrawTimesheet"
	ApproveTimesheetName  = "ApproveTimesheet"
	RejectTimesheetName   = "RejectTimesheet"
	ListTimesheetsName    = "ListTimesheets"
)

// Week is a person's week with its records and its totals per day.
type Week struct {
	Sheet   domain.Timesheet
	Records []timerepo.Placed
	// Days are the seven days' minutes, Monday first.
	Days    [7]int
	Minutes int
	// Locked reports, per day, whether it lies in the workspace's locked period.
	Locked [7]bool
}

func (k Keeper) week(ctx context.Context, account shared.ID, day domain.Day, policy domain.Policy) (Week, error) {
	sheet, _, err := k.Store.Sheet(ctx, account, day)
	if err != nil {
		return Week{}, err
	}
	start := sheet.WeekStart
	records, err := k.Store.Records(ctx, timerepo.Filter{From: start, To: start + 6, AccountID: account}, maxRecordsOfAWeek)
	if err != nil {
		return Week{}, err
	}
	w := Week{Sheet: sheet, Records: records}
	for _, r := range records {
		w.Days[int(r.Day-start)] += r.Minutes
		w.Minutes += r.Minutes
	}
	for i := range w.Locked {
		w.Locked[i] = policy.Locked(start + domain.Day(i))
	}
	return w, nil
}

func placesOf(records []timerepo.Placed) []place {
	seen := map[place]bool{}
	var out []place
	for _, r := range records {
		p := place{hub: r.HubID, collection: r.CollectionID}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, place{})
	}
	return out
}

// ReadTimesheet answers a person's week: the caller's own, or another person's for whoever
// approves every place its time lies in; where the workspace shows only sums, another person's week
// only while it waits for a decision (UC-TIM-02, UC-TIM-03).
type ReadTimesheet struct{ Keeper Keeper }

// Execute reads the week.
func (h ReadTimesheet) Execute(ctx context.Context, actor appshared.ActorContext, account shared.ID, day domain.Day) (Week, error) {
	if err := person(actor); err != nil {
		return Week{}, err
	}
	if account.IsZero() {
		account = actor.AccountID
	}
	var (
		w      Week
		policy domain.Policy
	)
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if policy, err = h.Keeper.Store.Policy(ctx); err != nil {
			return err
		}
		w, err = h.Keeper.week(ctx, account, day, policy)
		return err
	}); err != nil {
		return Week{}, err
	}
	if account == actor.AccountID {
		return w, nil
	}
	if policy.Visibility == domain.VisibilityAggregated && w.Sheet.State != domain.SheetSubmitted {
		return Week{}, shared.ErrForbidden.WithDetail("time.aggregated_only")
	}
	ok, err := h.Keeper.approvesAll(ctx, actor, placesOf(w.Records), SheetReadAction, itemsRead)
	if err != nil {
		return Week{}, err
	}
	if !ok {
		return Week{}, h.Keeper.refuseAsApprover(ctx, actor, SheetReadAction, itemsRead, account)
	}
	return w, nil
}

// SheetOutput is a week's head on the wire; the reason goes to the person and the approvers.
func SheetOutput(s domain.Timesheet) usecase.Output {
	out := usecase.Output{
		"account_id": s.AccountID.String(), "week_start": s.WeekStart.String(), "state": string(s.State),
		"submitted_minutes": s.Minutes, "decided_by": idText(s.DecidedBy), "reason": s.Reason, "version": s.Version,
		"submitted_at": nil, "decided_at": nil,
	}
	if s.SubmittedAt != nil {
		out["submitted_at"] = *s.SubmittedAt
	}
	if s.DecidedAt != nil {
		out["decided_at"] = *s.DecidedAt
	}
	return out
}

// WeekOutput is a week with its records and totals.
func WeekOutput(w Week) usecase.Output {
	out := SheetOutput(w.Sheet)
	records := make([]usecase.Output, 0, len(w.Records))
	for _, r := range w.Records {
		records = append(records, PlacedOutput(r))
	}
	days := make([]usecase.Output, 0, 7)
	for i, m := range w.Days {
		days = append(days, usecase.Output{"day": (w.Sheet.WeekStart + domain.Day(i)).String(), "minutes": m, "locked": w.Locked[i]})
	}
	out["records"], out["days"], out["minutes"] = records, days, w.Minutes
	return out
}

func weekInput() []usecase.Field {
	return []usecase.Field{
		{Name: "week", Kind: usecase.KindString, Required: true, Description: "Any day of the week, YYYY-MM-DD; the week runs from Monday."},
	}
}

// Descriptor is the catalogue entry.
func (h ReadTimesheet) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadTimesheetName,
		Summary: "Answers a week from Monday: its state, its records, each day's minutes and whether the day is " +
			"locked. The caller's own, or another person's for whoever approves time at every place the week's " +
			"entries lie in; where the workspace shows only sums, another person's week only while it is handed in.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: append(weekInput(), usecase.Field{Name: "account_id", Kind: usecase.KindID, Description: "The person; the caller without."}),
		Audit: usecase.AuditDeclaration{Action: SheetReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			day, err := dayField(in, "week", true)
			if err != nil {
				return nil, err
			}
			account, err := optionalID(in, "account_id")
			if err != nil {
				return nil, err
			}
			w, err := h.Execute(ctx, actor, account, day)
			if err != nil {
				return nil, err
			}
			return WeekOutput(w), nil
		}),
	}, sheetTarget)
}

// CopyTimesheetWeek copies the caller's records of the week before into a week, day by day (UC-TIM-02):
// the same entries, kinds, minutes and notes, as new records of the caller's.
type CopyTimesheetWeek struct{ Keeper Keeper }

// Execute copies; a locked day of the week is left out, a week not open is refused.
func (h CopyTimesheetWeek) Execute(ctx context.Context, actor appshared.ActorContext, day domain.Day) (int, error) {
	if err := person(actor); err != nil {
		return 0, err
	}
	start := domain.WeekOf(day)
	now := h.Keeper.Clock.Now()
	copied := 0
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		policy, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		state, err := h.Keeper.weekState(ctx, actor.AccountID, start)
		if err != nil {
			return err
		}
		if !state.Writable() {
			return shared.ErrConflict.WithDetail("time.week_not_open").WithParams(map[string]string{"state": string(state)})
		}
		before, err := h.Keeper.Store.Records(ctx, timerepo.Filter{From: start - 7, To: start - 1, AccountID: actor.AccountID}, maxRecordsOfAWeek)
		if err != nil {
			return err
		}
		for _, r := range before {
			if r.Correction() || policy.Locked(r.Day+7) {
				continue
			}
			c := domain.Record{
				ID: h.Keeper.IDs.NewID(), AccountID: actor.AccountID, EntryID: r.EntryID, WorkType: r.WorkType,
				Day: r.Day + 7, Minutes: r.Minutes, Note: r.Note, Billable: r.Billable, Source: domain.SourceCopy,
				CreatedAt: now, UpdatedAt: now, Version: 1,
			}
			if err := h.Keeper.Store.InsertRecord(ctx, c); err != nil {
				return err
			}
			if err := h.Keeper.recordChanged(ctx, actor, c); err != nil {
				return err
			}
			copied++
		}
		return h.Keeper.record(ctx, actor, WeekCopiedAction, sheetTarget, actor.AccountID, now,
			audit.Change{Field: "week_start", Classification: audit.Open, To: start.String()},
			audit.Change{Field: "records", Classification: audit.Open, To: strconv.Itoa(copied)})
	})
	return copied, err
}

// Descriptor is the catalogue entry.
func (h CopyTimesheetWeek) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CopyTimesheetWeekName,
		Summary: "Copies the caller's records of the week before into an open week, each to the same weekday, " +
			"leaving out corrections and days in the locked period. Answers how many were copied.",
		SideEffects: "Writes the copies and an audit entry.",
		TokenScope:  itemsWrite,
		Input:       weekInput(),
		Audit:       usecase.AuditDeclaration{Action: WeekCopiedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			day, err := dayField(in, "week", true)
			if err != nil {
				return nil, err
			}
			n, err := h.Execute(ctx, actor, day)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"week_start": domain.WeekOf(day).String(), "copied": n}, nil
		}),
	}, sheetTarget)
}

// putSheet writes a week inside the caller's transaction, a new row for a week never handed in,
// and tells its person's devices.
func (k Keeper) putSheet(ctx context.Context, actor appshared.ActorContext, s *domain.Timesheet, held bool, before int, now time.Time) error {
	expected := 0
	if held {
		expected = before
	} else {
		s.ID = k.IDs.NewID()
	}
	if err := k.Store.PutSheet(ctx, s.ID, *s, expected, now); err != nil {
		return err
	}
	s.Version = before + 1
	return k.sheetChanged(ctx, actor, *s)
}

// SubmitTimesheet hands the caller's week in; its records no longer change until it is sent back.
type SubmitTimesheet struct{ Keeper Keeper }

// Execute hands it in.
func (h SubmitTimesheet) Execute(ctx context.Context, actor appshared.ActorContext, day domain.Day) (domain.Timesheet, error) {
	if err := person(actor); err != nil {
		return domain.Timesheet{}, err
	}
	now := h.Keeper.Clock.Now()
	var out domain.Timesheet
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		sheet, held, err := h.Keeper.Store.Sheet(ctx, actor.AccountID, day)
		if err != nil {
			return err
		}
		minutes, err := h.Keeper.Store.WeekMinutes(ctx, actor.AccountID, sheet.WeekStart, sheet.WeekStart+6)
		if err != nil {
			return err
		}
		if out, err = sheet.Submit(minutes, now); err != nil {
			return err
		}
		if err := h.Keeper.putSheet(ctx, actor, &out, held, sheet.Version, now); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, SheetSubmittedAction, sheetTarget, actor.AccountID, now,
			audit.Change{Field: "week_start", Classification: audit.Open, To: out.WeekStart.String()},
			audit.Change{Field: "minutes", Classification: audit.Open, To: strconv.Itoa(minutes)})
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h SubmitTimesheet) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SubmitTimesheetName,
		Summary: "Hands the caller's week in for approval with its total; an empty week is refused " +
			"(timesheets.empty). Its records then no longer change until it is sent back or taken back.",
		SideEffects: "Writes the week's state and an audit entry.",
		TokenScope:  itemsWrite,
		Input:       weekInput(),
		Audit:       usecase.AuditDeclaration{Action: SheetSubmittedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			day, err := dayField(in, "week", true)
			if err != nil {
				return nil, err
			}
			s, err := h.Execute(ctx, actor, day)
			if err != nil {
				return nil, err
			}
			return SheetOutput(s), nil
		}),
	}, sheetTarget)
}

// WithdrawTimesheet takes the caller's handed-in week back before anybody decided.
type WithdrawTimesheet struct{ Keeper Keeper }

// Execute takes it back.
func (h WithdrawTimesheet) Execute(ctx context.Context, actor appshared.ActorContext, day domain.Day) (domain.Timesheet, error) {
	if err := person(actor); err != nil {
		return domain.Timesheet{}, err
	}
	now := h.Keeper.Clock.Now()
	var out domain.Timesheet
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		sheet, held, err := h.Keeper.Store.Sheet(ctx, actor.AccountID, day)
		if err != nil {
			return err
		}
		if out, err = sheet.Withdraw(); err != nil {
			return err
		}
		if err := h.Keeper.putSheet(ctx, actor, &out, held, sheet.Version, now); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, SheetWithdrawnAction, sheetTarget, actor.AccountID, now,
			audit.Change{Field: "week_start", Classification: audit.Open, To: out.WeekStart.String()})
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h WithdrawTimesheet) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        WithdrawTimesheetName,
		Summary:     "Takes the caller's handed-in week back before anybody decided; it is open again.",
		SideEffects: "Writes the week's state and an audit entry.",
		TokenScope:  itemsWrite,
		Input:       weekInput(),
		Audit:       usecase.AuditDeclaration{Action: SheetWithdrawnAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			day, err := dayField(in, "week", true)
			if err != nil {
				return nil, err
			}
			s, err := h.Execute(ctx, actor, day)
			if err != nil {
				return nil, err
			}
			return SheetOutput(s), nil
		}),
	}, sheetTarget)
}

// decide approves or sends back another person's handed-in week, for whoever approves time at
// every place its entries lie in.
func (k Keeper) decide(
	ctx context.Context, actor appshared.ActorContext, account shared.ID, day domain.Day, action audit.Action,
	apply func(domain.Timesheet, time.Time) (domain.Timesheet, error),
) (domain.Timesheet, error) {
	if err := person(actor); err != nil {
		return domain.Timesheet{}, err
	}
	if account.IsZero() {
		return domain.Timesheet{}, invalid("/account_id", "timesheets.person_required")
	}
	var w Week
	if err := k.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		w, err = k.week(ctx, account, day, domain.DefaultPolicy())
		return err
	}); err != nil {
		return domain.Timesheet{}, err
	}
	if account != actor.AccountID {
		ok, err := k.approvesAll(ctx, actor, placesOf(w.Records), action, itemsWrite)
		if err != nil {
			return domain.Timesheet{}, err
		}
		if !ok {
			return domain.Timesheet{}, k.refuseAsApprover(ctx, actor, action, itemsWrite, account)
		}
	}
	now := k.Clock.Now()
	var out domain.Timesheet
	err := k.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		sheet, held, err := k.Store.Sheet(ctx, account, day)
		if err != nil {
			return err
		}
		if !held {
			return shared.ErrConflict.WithDetail("timesheets.not_submitted")
		}
		if out, err = apply(sheet, now); err != nil {
			return err
		}
		if err := k.Store.PutSheet(ctx, out.ID, out, sheet.Version, now); err != nil {
			return err
		}
		out.Version = sheet.Version + 1
		if err := k.sheetChanged(ctx, actor, out); err != nil {
			return err
		}
		return k.record(ctx, actor, action, sheetTarget, account, now,
			audit.Change{Field: "week_start", Classification: audit.Open, To: out.WeekStart.String()},
			audit.Change{Field: "state", Classification: audit.Open, From: string(sheet.State), To: string(out.State)})
	})
	return out, err
}

func decisionInput(withReason bool) []usecase.Field {
	fields := append(weekInput(), usecase.Field{Name: "account_id", Kind: usecase.KindID, Required: true, Description: "The person whose week it is."})
	if withReason {
		fields = append(fields, usecase.Field{Name: "reason", Kind: usecase.KindString, Required: true, Description: "Why, at most 500 characters; the person reads it."})
	}
	return fields
}

// ApproveTimesheet approves another person's handed-in week; its records never change again.
type ApproveTimesheet struct{ Keeper Keeper }

// Execute approves it.
func (h ApproveTimesheet) Execute(ctx context.Context, actor appshared.ActorContext, account shared.ID, day domain.Day) (domain.Timesheet, error) {
	return h.Keeper.decide(ctx, actor, account, day, SheetApprovedAction, func(s domain.Timesheet, now time.Time) (domain.Timesheet, error) {
		return s.Approve(actor.AccountID, now)
	})
}

// Descriptor is the catalogue entry.
func (h ApproveTimesheet) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ApproveTimesheetName, Permission: service.PermissionApproveTime,
		Summary: "Approves another person's handed-in week. Needs APPROVE_TIME or MANAGE_MEMBERS at the workspace " +
			"or at every hub or collection the week's entries lie in; nobody approves their own (timesheets.own_week). " +
			"Its records never change again; a correction goes into an open week.",
		SideEffects: "Writes the week's state and an audit entry.",
		TokenScope:  itemsWrite,
		Input:       decisionInput(false),
		Audit:       usecase.AuditDeclaration{Action: SheetApprovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			day, err := dayField(in, "week", true)
			if err != nil {
				return nil, err
			}
			account, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			s, err := h.Execute(ctx, actor, account, day)
			if err != nil {
				return nil, err
			}
			return SheetOutput(s), nil
		}),
	}, sheetTarget)
}

// RejectTimesheet sends another person's handed-in week back with a reason.
type RejectTimesheet struct{ Keeper Keeper }

// Execute sends it back.
func (h RejectTimesheet) Execute(ctx context.Context, actor appshared.ActorContext, account shared.ID, day domain.Day, reason string) (domain.Timesheet, error) {
	return h.Keeper.decide(ctx, actor, account, day, SheetRejectedAction, func(s domain.Timesheet, now time.Time) (domain.Timesheet, error) {
		return s.Reject(actor.AccountID, reason, now)
	})
}

// Descriptor is the catalogue entry.
func (h RejectTimesheet) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RejectTimesheetName, Permission: service.PermissionApproveTime,
		Summary: "Sends another person's handed-in week back with a reason they read; they change it and hand it in " +
			"again. Needs what approving needs.",
		SideEffects: "Writes the week's state and an audit entry without the reason.",
		TokenScope:  itemsWrite,
		Input:       decisionInput(true),
		Audit:       usecase.AuditDeclaration{Action: SheetRejectedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			day, err := dayField(in, "week", true)
			if err != nil {
				return nil, err
			}
			account, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			s, err := h.Execute(ctx, actor, account, day, in.String("reason"))
			if err != nil {
				return nil, err
			}
			return SheetOutput(s), nil
		}),
	}, sheetTarget)
}

// ListTimesheets lists weeks: the caller's own with `mine`, else the other people's handed-in
// weeks the caller may decide - the approval queue.
type ListTimesheets struct{ Keeper Keeper }

// Execute lists them.
func (h ListTimesheets) Execute(
	ctx context.Context, actor appshared.ActorContext, state domain.SheetState, from, to domain.Day, mine bool,
) ([]domain.Timesheet, error) {
	if err := person(actor); err != nil {
		return nil, err
	}
	if state != "" && !state.Valid() {
		return nil, invalid("/state", "timesheets.state_unknown")
	}
	account := shared.ID("")
	if mine {
		account = actor.AccountID
	} else if state == "" {
		state = domain.SheetSubmitted
	}
	var sheets []domain.Timesheet
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		sheets, err = h.Keeper.Store.Sheets(ctx, account, state, domain.WeekOf(from), domain.WeekOf(to), maxSheetsListed)
		return err
	}); err != nil {
		return nil, err
	}
	if mine {
		return sheets, nil
	}
	kept := sheets[:0]
	for _, s := range sheets {
		if s.AccountID == actor.AccountID {
			continue
		}
		var w Week
		if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			w, err = h.Keeper.week(ctx, s.AccountID, s.WeekStart, domain.DefaultPolicy())
			return err
		}); err != nil {
			return nil, err
		}
		ok, err := h.Keeper.approvesAll(ctx, actor, placesOf(w.Records), SheetReadAction, itemsRead)
		if err != nil {
			return nil, err
		}
		if ok {
			kept = append(kept, s)
		}
	}
	return kept, nil
}

// Descriptor is the catalogue entry.
func (h ListTimesheets) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListTimesheetsName,
		Summary: "Lists weeks from a first to a last day, at most 100, the oldest first: the caller's own in any " +
			"state with `mine`, else other people's weeks in a state - SUBMITTED without - that the caller may decide.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day, YYYY-MM-DD."},
			{Name: "to", Kind: usecase.KindString, Required: true, Description: "The last day, YYYY-MM-DD; at most 366 days after the first."},
			{Name: "state", Kind: usecase.KindString, Enum: []string{"OPEN", "SUBMITTED", "APPROVED", "REJECTED"}, Description: "Only weeks in this state."},
			{Name: "mine", Kind: usecase.KindBool, Description: "The caller's own weeks."},
		},
		Audit: usecase.AuditDeclaration{Action: SheetReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			from, to, err := dayRange(in)
			if err != nil {
				return nil, err
			}
			mine, _ := in["mine"].(bool)
			sheets, err := h.Execute(ctx, actor, domain.SheetState(in.String("state")), from, to, mine)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(sheets))
			for _, s := range sheets {
				data = append(data, SheetOutput(s))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, sheetTarget)
}
