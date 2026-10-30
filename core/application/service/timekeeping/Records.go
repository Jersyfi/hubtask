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
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	LogTimeName           = "LogTime"
	UpdateTimeRecordName  = "UpdateTimeRecord"
	DeleteTimeRecordName  = "DeleteTimeRecord"
	CorrectTimeRecordName = "CorrectTimeRecord"
	ListTimeRecordsName   = "ListTimeRecords"
)

// RecordFields are what a person writes of a record.
type RecordFields struct {
	EntryID  shared.ID
	WorkType string
	Day      domain.Day
	// Minutes, or a start and an end the minutes are counted from.
	Minutes   int
	StartedAt *time.Time
	EndedAt   *time.Time
	Note      string
	// Billable is the kind of work's flag when nil.
	Billable *bool
}

// fill completes a record from what was written, inside the caller's transaction: the kind of
// work's billable flag, the minutes of a start and an end, and the day of the start in the zone.
func (k Keeper) fill(ctx context.Context, r *domain.Record, f RecordFields, zone *time.Location) error {
	kind, err := k.workType(ctx, f.WorkType)
	if err != nil {
		return err
	}
	r.EntryID, r.WorkType, r.Note = f.EntryID, f.WorkType, f.Note
	r.Billable = kind.Billable
	if f.Billable != nil {
		r.Billable = *f.Billable
	}
	r.Day, r.Minutes, r.StartedAt = f.Day, f.Minutes, f.StartedAt
	if f.StartedAt != nil && f.EndedAt != nil {
		minutes, err := domain.Span(*f.StartedAt, *f.EndedAt)
		if err != nil {
			return err
		}
		r.Minutes = minutes
		if f.Day == 0 {
			r.Day = planningDay(*f.StartedAt, zone)
		}
	}
	if r.Day == 0 {
		return invalid("/day", "time.day_required")
	}
	return nil
}

// LogTime records time a person spent, on an entry they may see or on none (UC-TIM-01).
type LogTime struct{ Keeper Keeper }

// Execute writes the record.
func (h LogTime) Execute(ctx context.Context, actor appshared.ActorContext, f RecordFields, zone *time.Location) (domain.Record, error) {
	if err := person(actor); err != nil {
		return domain.Record{}, err
	}
	if !f.EntryID.IsZero() {
		if err := h.Keeper.entryReadable(ctx, actor, f.EntryID, RecordLoggedAction); err != nil {
			return domain.Record{}, err
		}
	}
	now := h.Keeper.Clock.Now()
	record := domain.Record{
		ID: h.Keeper.IDs.NewID(), AccountID: actor.AccountID, Source: domain.SourceManual,
		CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.fill(ctx, &record, f, zone); err != nil {
			return err
		}
		if err := record.Check(); err != nil {
			return err
		}
		policy, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		if err := h.Keeper.mayWrite(ctx, policy, actor.AccountID, record.Day); err != nil {
			return err
		}
		if err := h.Keeper.Store.InsertRecord(ctx, record); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, RecordLoggedAction, recordTarget, record.ID, now, recordChanges(domain.Record{}, record)...)
	})
	return record, err
}

// recordChanges are a record's audited fields; the note is a person's words and stays out.
func recordChanges(before, after domain.Record) []audit.Change {
	text := func(r domain.Record) (string, string, string, string, string) {
		if r.ID.IsZero() {
			return "", "", "", "", ""
		}
		return r.Day.String(), strconv.Itoa(r.Minutes), r.WorkType, strconv.FormatBool(r.Billable), r.EntryID.String()
	}
	d0, m0, w0, b0, e0 := text(before)
	d1, m1, w1, b1, e1 := text(after)
	return []audit.Change{
		{Field: "day", Classification: audit.Open, From: d0, To: d1},
		{Field: "minutes", Classification: audit.Open, From: m0, To: m1},
		{Field: "work_type", Classification: audit.Open, From: w0, To: w1},
		{Field: "billable", Classification: audit.Open, From: b0, To: b1},
		{Field: "entry_id", Classification: audit.Open, From: e0, To: e1},
	}
}

func recordInput() []usecase.Field {
	return []usecase.Field{
		{Name: "entry_id", Kind: usecase.KindID, Description: "The entry the time was spent on; none for time on no entry."},
		{Name: "work_type", Kind: usecase.KindString, Description: "The key of one of the workspace's kinds of work."},
		{Name: "day", Kind: usecase.KindString, Description: "The day, YYYY-MM-DD; the start's day in the zone without."},
		{Name: "minutes", Kind: usecase.KindAny, Description: "Minutes, 1 to 1440; left out with a start and an end."},
		{Name: "started_at", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "When the work began (RFC 3339)."},
		{Name: "ended_at", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "When it ended (RFC 3339); the minutes are counted from the start."},
		{Name: "note", Kind: usecase.KindString, Description: "What was done, at most 2000 characters."},
		{Name: "billable", Kind: usecase.KindBool, Description: "Whether the time is billable; the kind of work's flag without."},
		{Name: "zone", Kind: usecase.KindString, Description: "The person's time zone (IANA), for the start's day; UTC without."},
	}
}

func recordFieldsOf(in usecase.Input) (RecordFields, *time.Location, error) {
	var f RecordFields
	entry, err := optionalID(in, "entry_id")
	if err != nil {
		return f, nil, err
	}
	day, err := dayField(in, "day", false)
	if err != nil {
		return f, nil, err
	}
	zone, err := zoneOf(in)
	if err != nil {
		return f, nil, err
	}
	f = RecordFields{EntryID: entry, WorkType: in.String("work_type"), Day: day, Note: in.String("note"), Billable: optionalBool(in, "billable")}
	if _, given := in["minutes"]; given {
		minutes, ok := intField(in, "minutes")
		if !ok {
			return f, nil, invalid("/minutes", "time.minutes_out_of_range")
		}
		f.Minutes = minutes
	}
	for _, field := range []string{"started_at", "ended_at"} {
		raw := in.String(field)
		if raw == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, nil, invalid("/"+field, "time.instant_invalid")
		}
		if field == "started_at" {
			f.StartedAt = &at
		} else {
			f.EndedAt = &at
		}
	}
	if f.EndedAt != nil && f.StartedAt == nil {
		return f, nil, invalid("/started_at", "time.start_required")
	}
	return f, zone, nil
}

// Descriptor is the catalogue entry.
func (h LogTime) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: LogTimeName,
		Summary: "Records time the caller spent on one day: minutes from 1 to 1440, or a start and an end, on an " +
			"entry the caller may read or on none, of a kind of work, billable or not, with a note. Refused in the " +
			"workspace's locked period and in a week handed in or approved.",
		SideEffects: "Writes the record and an audit entry without its note.",
		TokenScope:  itemsWrite,
		Input:       recordInput(),
		Audit:       usecase.AuditDeclaration{Action: RecordLoggedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			f, zone, err := recordFieldsOf(in)
			if err != nil {
				return nil, err
			}
			r, err := h.Execute(ctx, actor, f, zone)
			if err != nil {
				return nil, err
			}
			return RecordOutput(r), nil
		}),
	}, recordTarget)
}

// own reads a record of the caller's.
func (k Keeper) own(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Record, error) {
	if err := person(actor); err != nil {
		return domain.Record{}, err
	}
	if id.IsZero() {
		return domain.Record{}, invalid("/record_id", "time.record_required")
	}
	var r domain.Record
	err := k.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		r, err = k.Store.FindRecord(ctx, id)
		return err
	})
	if err != nil {
		return domain.Record{}, err
	}
	if r.AccountID != actor.AccountID {
		// Another person's record is never written by anybody else, and is not told of (T-04).
		return domain.Record{}, shared.ErrNotFound.WithDetail("time.record_not_found")
	}
	return r, nil
}

// UpdateTimeRecord changes a record of the caller's while its week is open (UC-TIM-01, UC-TIM-02).
type UpdateTimeRecord struct{ Keeper Keeper }

// Execute writes the change, held to the version read.
func (h UpdateTimeRecord) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, expected int, f RecordFields, zone *time.Location,
) (domain.Record, error) {
	before, err := h.Keeper.own(ctx, actor, id)
	if err != nil {
		return domain.Record{}, err
	}
	if before.Correction() {
		f.Minutes = before.Minutes
	}
	if !f.EntryID.IsZero() && f.EntryID != before.EntryID {
		if err := h.Keeper.entryReadable(ctx, actor, f.EntryID, RecordUpdatedAction); err != nil {
			return domain.Record{}, err
		}
	}
	now := h.Keeper.Clock.Now()
	after := before
	after.UpdatedAt, after.Version = now, before.Version+1
	err = h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.fill(ctx, &after, f, zone); err != nil {
			return err
		}
		if err := after.Check(); err != nil {
			return err
		}
		policy, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		for _, d := range []domain.Day{before.Day, after.Day} {
			if err := h.Keeper.mayWrite(ctx, policy, actor.AccountID, d); err != nil {
				return err
			}
		}
		if err := h.Keeper.Store.UpdateRecord(ctx, after, expected); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, RecordUpdatedAction, recordTarget, after.ID, now, recordChanges(before, after)...)
	})
	return after, err
}

// Descriptor is the catalogue entry.
func (h UpdateTimeRecord) Descriptor() usecase.Descriptor {
	input := append([]usecase.Field{
		{Name: "record_id", Kind: usecase.KindID, Required: true, Description: "The record."},
		{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
	}, recordInput()...)
	return descriptor(usecase.Descriptor{
		Name: UpdateTimeRecordName,
		Summary: "Changes one of the caller's records while its old and its new day lie outside the locked period " +
			"and in an open or a sent-back week; every field is written as given. A correction keeps its minutes.",
		SideEffects: "Writes the record and an audit entry without its note.",
		TokenScope:  itemsWrite,
		Input:       input,
		Audit:       usecase.AuditDeclaration{Action: RecordUpdatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("record_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			f, zone, err := recordFieldsOf(in)
			if err != nil {
				return nil, err
			}
			r, err := h.Execute(ctx, actor, id, expected, f, zone)
			if err != nil {
				return nil, err
			}
			return RecordOutput(r), nil
		}),
	}, recordTarget)
}

// DeleteTimeRecord removes a record of the caller's while its week is open.
type DeleteTimeRecord struct{ Keeper Keeper }

// Execute removes it, held to the version read.
func (h DeleteTimeRecord) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, expected int) error {
	r, err := h.Keeper.own(ctx, actor, id)
	if err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		policy, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		if err := h.Keeper.mayWrite(ctx, policy, actor.AccountID, r.Day); err != nil {
			return err
		}
		if err := h.Keeper.Store.DeleteRecord(ctx, id, expected); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, RecordDeletedAction, recordTarget, id, now, recordChanges(r, domain.Record{})...)
	})
}

// Descriptor is the catalogue entry.
func (h DeleteTimeRecord) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        DeleteTimeRecordName,
		Summary:     "Removes one of the caller's records while its day lies outside the locked period and in an open or a sent-back week.",
		SideEffects: "Deletes the record and writes an audit entry.",
		TokenScope:  itemsWrite, Destructive: true,
		Input: []usecase.Field{
			{Name: "record_id", Kind: usecase.KindID, Required: true, Description: "The record."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: RecordDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("record_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			return usecase.Output{}, h.Execute(ctx, actor, id, expected)
		}),
	}, recordTarget)
}

// CorrectTimeRecord corrects an approved record of the caller's - or one in the locked period - by
// a new, signed record in an open week; the approved record itself never changes (UC-TIM-03).
type CorrectTimeRecord struct{ Keeper Keeper }

// Execute writes the correction.
func (h CorrectTimeRecord) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, minutes int, day domain.Day, note string,
) (domain.Record, error) {
	target, err := h.Keeper.own(ctx, actor, id)
	if err != nil {
		return domain.Record{}, err
	}
	if target.Correction() {
		return domain.Record{}, shared.ErrConflict.WithDetail("time.correction_of_correction")
	}
	now := h.Keeper.Clock.Now()
	correction := domain.Record{
		ID: h.Keeper.IDs.NewID(), AccountID: actor.AccountID, EntryID: target.EntryID, WorkType: target.WorkType,
		Day: day, Minutes: minutes, Note: note, Billable: target.Billable, Source: domain.SourceCorrection,
		Corrects: target.ID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := correction.Check(); err != nil {
		return domain.Record{}, err
	}
	err = h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		policy, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		state, err := h.Keeper.weekState(ctx, actor.AccountID, target.Day)
		if err != nil {
			return err
		}
		if state != domain.SheetApproved && !policy.Locked(target.Day) {
			return shared.ErrConflict.WithDetail("time.correction_not_needed")
		}
		if err := h.Keeper.mayWrite(ctx, policy, actor.AccountID, day); err != nil {
			return err
		}
		if err := h.Keeper.Store.InsertRecord(ctx, correction); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, RecordCorrectedAction, recordTarget, correction.ID, now,
			append(recordChanges(domain.Record{}, correction),
				audit.Change{Field: "corrects", Classification: audit.Open, To: target.ID.String()})...)
	})
	return correction, err
}

// Descriptor is the catalogue entry.
func (h CorrectTimeRecord) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CorrectTimeRecordName,
		Summary: "Corrects one of the caller's approved records, or one in the locked period, by a new record of " +
			"signed minutes (-1440 to 1440, not 0) on a day of an open week; the approved record never changes.",
		SideEffects: "Writes the correction and an audit entry without its note.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "record_id", Kind: usecase.KindID, Required: true, Description: "The approved record."},
			{Name: "minutes", Kind: usecase.KindAny, Required: true, Description: "The minutes to add, negative to take away."},
			{Name: "day", Kind: usecase.KindString, Required: true, Description: "The open day the correction is recorded on, YYYY-MM-DD."},
			{Name: "note", Kind: usecase.KindString, Description: "Why, at most 2000 characters."},
		},
		Audit: usecase.AuditDeclaration{Action: RecordCorrectedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("record_id")
			if err != nil {
				return nil, err
			}
			minutes, ok := intField(in, "minutes")
			if !ok {
				return nil, invalid("/minutes", "time.minutes_out_of_range")
			}
			day, err := dayField(in, "day", true)
			if err != nil {
				return nil, err
			}
			r, err := h.Execute(ctx, actor, id, minutes, day, in.String("note"))
			if err != nil {
				return nil, err
			}
			return RecordOutput(r), nil
		}),
	}, recordTarget)
}

// ListTimeRecords lists the caller's records in a range of days, or - for an approver where the
// workspace shows each person's time - another person's or everybody's records where the entries
// lie in the places they approve (UC-TIM-01, UC-TIM-03).
type ListTimeRecords struct{ Keeper Keeper }

// Execute reads the records; everybody's when account is zero and mine is false.
func (h ListTimeRecords) Execute(
	ctx context.Context, actor appshared.ActorContext, f timerepo.Filter, mine bool,
) ([]timerepo.Placed, bool, error) {
	if err := person(actor); err != nil {
		return nil, false, err
	}
	if mine {
		f.AccountID = actor.AccountID
	}
	others := f.AccountID != actor.AccountID
	if others {
		policy, err := h.Keeper.policy(ctx, actor)
		if err != nil {
			return nil, false, err
		}
		if policy.Visibility == domain.VisibilityAggregated {
			return nil, false, shared.ErrForbidden.WithDetail("time.aggregated_only")
		}
	}
	var placed []timerepo.Placed
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		placed, err = h.Keeper.Store.Records(ctx, f, maxRecordsListed+1)
		return err
	}); err != nil {
		return nil, false, err
	}
	truncated := len(placed) > maxRecordsListed
	if truncated {
		placed = placed[:maxRecordsListed]
	}
	if !others {
		return placed, truncated, nil
	}
	kept, err := h.Keeper.approvedRecords(ctx, actor, placed, RecordsReadAction, itemsRead)
	return kept, truncated, err
}

// approvedRecords keeps the caller's own records and the others' where the caller approves time;
// none kept of another person's is refused as such, so that a plain member learns nothing.
func (k Keeper) approvedRecords(
	ctx context.Context, actor appshared.ActorContext, placed []timerepo.Placed, action audit.Action, scope string,
) ([]timerepo.Placed, error) {
	index := map[place]int{}
	var places []place
	for _, p := range placed {
		at := place{hub: p.HubID, collection: p.CollectionID}
		if _, ok := index[at]; !ok && p.AccountID != actor.AccountID {
			index[at] = len(places)
			places = append(places, at)
		}
	}
	allowed, err := k.approves(ctx, actor, places, action, scope)
	if err != nil {
		return nil, err
	}
	anyAllowed := false
	for _, ok := range allowed {
		anyAllowed = anyAllowed || ok
	}
	if len(places) > 0 && !anyAllowed {
		if err := k.refuseAsApprover(ctx, actor, action, scope, ""); err != nil {
			return nil, err
		}
	}
	kept := placed[:0]
	for _, p := range placed {
		if p.AccountID == actor.AccountID || allowed[index[place{hub: p.HubID, collection: p.CollectionID}]] {
			kept = append(kept, p)
		}
	}
	return kept, nil
}

func filterOf(in usecase.Input) (timerepo.Filter, error) {
	from, to, err := dayRange(in)
	if err != nil {
		return timerepo.Filter{}, err
	}
	f := timerepo.Filter{From: from, To: to, Billable: optionalBool(in, "billable")}
	for field, into := range map[string]*shared.ID{
		"account_id": &f.AccountID, "entry_id": &f.EntryID, "collection_id": &f.CollectionID, "hub_id": &f.HubID,
	} {
		if *into, err = optionalID(in, field); err != nil {
			return timerepo.Filter{}, err
		}
	}
	return f, nil
}

func filterInput(withEntry bool) []usecase.Field {
	fields := []usecase.Field{
		{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day, YYYY-MM-DD."},
		{Name: "to", Kind: usecase.KindString, Required: true, Description: "The last day, YYYY-MM-DD; at most 366 days after the first."},
		{Name: "mine", Kind: usecase.KindBool, Description: "Only the caller's own time."},
		{Name: "account_id", Kind: usecase.KindID, Description: "Only this person's time."},
		{Name: "collection_id", Kind: usecase.KindID, Description: "Only time on entries of this collection."},
		{Name: "hub_id", Kind: usecase.KindID, Description: "Only time on entries of this hub's collections."},
		{Name: "billable", Kind: usecase.KindBool, Description: "Only billable, or only non-billable, time."},
	}
	if withEntry {
		fields = append(fields, usecase.Field{Name: "entry_id", Kind: usecase.KindID, Description: "Only time on this entry."})
	}
	return fields
}

// PlacedOutput is a record with its entry's place and title, the title only where the record's
// reader recorded it or approves it.
func PlacedOutput(p timerepo.Placed) usecase.Output {
	out := RecordOutput(p.Record)
	out["collection_id"], out["hub_id"], out["entry_title"] = idText(p.CollectionID), idText(p.HubID), p.EntryTitle
	return out
}

// Descriptor is the catalogue entry.
func (h ListTimeRecords) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListTimeRecordsName,
		Summary: "Lists time records of a range of days, at most 2000, the earliest first: the caller's own with " +
			"`mine`, else also other people's where the caller approves time (APPROVE_TIME or MANAGE_MEMBERS at " +
			"the workspace, or at the hub or collection of the entry). Refused with time.aggregated_only for " +
			"other people's records where the workspace shows only sums.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: filterInput(true),
		Audit: usecase.AuditDeclaration{Action: RecordsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			f, err := filterOf(in)
			if err != nil {
				return nil, err
			}
			mine, _ := in["mine"].(bool)
			placed, truncated, err := h.Execute(ctx, actor, f, mine)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(placed))
			for _, p := range placed {
				data = append(data, PlacedOutput(p))
			}
			return usecase.Output{"data": data, "truncated": truncated}, nil
		}),
	}, recordTarget)
}
