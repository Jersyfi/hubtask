// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"strconv"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	StartTimerName   = "StartTimer"
	StopTimerName    = "StopTimer"
	DiscardTimerName = "DiscardTimer"
	ReadTimerName    = "ReadTimer"
)

func planningDay(at time.Time, zone *time.Location) domain.Day {
	if zone == nil {
		zone = time.UTC
	}
	return planning.DayOf(at, zone)
}

// TimerOutput is a running timer on the wire, with the minutes it has run.
func TimerOutput(t domain.Timer, now time.Time) usecase.Output {
	return usecase.Output{
		"running": true, "entry_id": idText(t.EntryID), "work_type": t.WorkType, "note": t.Note,
		"billable": t.Billable, "started_at": t.StartedAt, "elapsed_minutes": int(now.Sub(t.StartedAt) / time.Minute),
	}
}

// StartTimer starts the caller's clock on an entry they may see, or on none (UC-TIM-01). One runs
// per person; the same one answers on every device.
type StartTimer struct{ Keeper Keeper }

// Execute starts it now.
func (h StartTimer) Execute(ctx context.Context, actor appshared.ActorContext, entry shared.ID, workType, note string, billable *bool) (domain.Timer, error) {
	return h.ExecuteAt(ctx, actor, entry, workType, note, billable, time.Time{})
}

// ExecuteAt starts it at a device's moment - a start queued offline -, now without one.
func (h StartTimer) ExecuteAt(
	ctx context.Context, actor appshared.ActorContext, entry shared.ID, workType, note string, billable *bool, at time.Time,
) (domain.Timer, error) {
	if err := person(actor); err != nil {
		return domain.Timer{}, err
	}
	if err := domain.CheckNote(note); err != nil {
		return domain.Timer{}, err
	}
	if !entry.IsZero() {
		if err := h.Keeper.entryReadable(ctx, actor, entry, TimerStartedAction); err != nil {
			return domain.Timer{}, err
		}
	}
	now := h.Keeper.Clock.Now()
	started, err := deviceMoment(at, now)
	if err != nil {
		return domain.Timer{}, err
	}
	timer := domain.Timer{AccountID: actor.AccountID, EntryID: entry, WorkType: workType, Note: note, StartedAt: started, Version: 1}
	id := h.Keeper.IDs.NewID()
	err = h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		kind, err := h.Keeper.workType(ctx, workType)
		if err != nil {
			return err
		}
		timer.Billable = kind.Billable
		if billable != nil {
			timer.Billable = *billable
		}
		if err := h.Keeper.Store.StartTimer(ctx, id, timer); err != nil {
			return err
		}
		if err := h.Keeper.timerChanged(ctx, actor, actor.AccountID, &timer); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, TimerStartedAction, timerTarget, id, now,
			audit.Change{Field: "entry_id", Classification: audit.Open, To: entry.String()})
	})
	return timer, err
}

func timerInput() []usecase.Field {
	return []usecase.Field{
		{Name: "entry_id", Kind: usecase.KindID, Description: "The entry; none for time on no entry."},
		{Name: "work_type", Kind: usecase.KindString, Description: "The key of one of the workspace's kinds of work."},
		{Name: "note", Kind: usecase.KindString, Description: "What is being done, at most 2000 characters."},
		{Name: "billable", Kind: usecase.KindBool, Description: "Whether the time is billable; the kind of work's flag without."},
		{Name: "started_at", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "When the device started it (RFC 3339), a start queued offline; now without one."},
	}
}

// deviceMoment is the moment a device says a timer started or stopped, held to the server's clock:
// a moment ahead of it is now - a device's clock running fast -, and one more than a day before it
// is a timer run too long. Without one it is now.
func deviceMoment(at, now time.Time) (time.Time, error) {
	if at.IsZero() || at.After(now) {
		return now, nil
	}
	if now.Sub(at) > domain.MaxMinutes*time.Minute {
		return time.Time{}, shared.ErrValidation.WithDetail("timers.ran_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxMinutes)})
	}
	return at, nil
}

// momentOf reads an RFC 3339 moment, the zero time without one.
func momentOf(in usecase.Input, field string) (time.Time, error) {
	raw := in.String(field)
	if raw == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, invalid("/"+field, "time.instant_invalid")
	}
	return at, nil
}

// Descriptor is the catalogue entry.
func (h StartTimer) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: StartTimerName,
		Summary: "Starts the caller's timer on an entry they may read, or on none. One runs per person " +
			"(timers.running); every device reads the same one.",
		SideEffects: "Writes the timer and an audit entry without its note.",
		TokenScope:  itemsWrite,
		Input:       timerInput(),
		Audit:       usecase.AuditDeclaration{Action: TimerStartedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			entry, err := optionalID(in, "entry_id")
			if err != nil {
				return nil, err
			}
			at, err := momentOf(in, "started_at")
			if err != nil {
				return nil, err
			}
			t, err := h.ExecuteAt(ctx, actor, entry, in.String("work_type"), in.String("note"), optionalBool(in, "billable"), at)
			if err != nil {
				return nil, err
			}
			return TimerOutput(t, t.StartedAt), nil
		}),
	}, timerTarget)
}

// StopTimer stops the caller's clock into a record of the day it started in their zone.
type StopTimer struct{ Keeper Keeper }

// Execute stops it and records the time; a week no longer open, or a timer run longer than a
// day, keeps it running and answers why.
func (h StopTimer) Execute(ctx context.Context, actor appshared.ActorContext, zone *time.Location) (domain.Record, error) {
	return h.ExecuteAt(ctx, actor, zone, time.Time{})
}

// ExecuteAt stops it at a device's moment - a stop queued offline -, now without one.
func (h StopTimer) ExecuteAt(ctx context.Context, actor appshared.ActorContext, zone *time.Location, at time.Time) (domain.Record, error) {
	if err := person(actor); err != nil {
		return domain.Record{}, err
	}
	now := h.Keeper.Clock.Now()
	stopped, err := deviceMoment(at, now)
	if err != nil {
		return domain.Record{}, err
	}
	var record domain.Record
	err = h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		timer, running, err := h.Keeper.Store.Timer(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if !running {
			return shared.ErrNotFound.WithDetail("timers.not_running")
		}
		if record, err = timer.Stop(h.Keeper.IDs.NewID(), stopped, zone); err != nil {
			return err
		}
		record.CreatedAt, record.UpdatedAt = now, now
		policy, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		if err := h.Keeper.mayWrite(ctx, policy, actor.AccountID, record.Day); err != nil {
			return err
		}
		if _, err := h.Keeper.Store.RemoveTimer(ctx, actor.AccountID); err != nil {
			return err
		}
		if err := h.Keeper.Store.InsertRecord(ctx, record); err != nil {
			return err
		}
		if err := h.Keeper.timerChanged(ctx, actor, actor.AccountID, nil); err != nil {
			return err
		}
		if err := h.Keeper.recordChanged(ctx, actor, record); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, TimerStoppedAction, recordTarget, record.ID, now, recordChanges(domain.Record{}, record)...)
	})
	return record, err
}

// Descriptor is the catalogue entry.
func (h StopTimer) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: StopTimerName,
		Summary: "Stops the caller's timer and records its time, rounded up to the started minute, on the day it " +
			"started in the zone. A timer run longer than a day is refused (timers.ran_too_long) and keeps running " +
			"until it is discarded.",
		SideEffects: "Deletes the timer, writes the record and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "zone", Kind: usecase.KindString, Description: "The person's time zone (IANA); UTC without."},
			{Name: "stopped_at", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "When the device stopped it (RFC 3339), a stop queued offline; now without one."},
		},
		Audit: usecase.AuditDeclaration{Action: TimerStoppedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			zone, err := zoneOf(in)
			if err != nil {
				return nil, err
			}
			at, err := momentOf(in, "stopped_at")
			if err != nil {
				return nil, err
			}
			r, err := h.ExecuteAt(ctx, actor, zone, at)
			if err != nil {
				return nil, err
			}
			return RecordOutput(r), nil
		}),
	}, recordTarget)
}

// DiscardTimer stops the caller's clock without recording anything.
type DiscardTimer struct{ Keeper Keeper }

// Execute discards it.
func (h DiscardTimer) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if err := person(actor); err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Keeper.Store.RemoveTimer(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("timers.not_running")
		}
		if err := h.Keeper.timerChanged(ctx, actor, actor.AccountID, nil); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, TimerDiscardedAction, timerTarget, actor.AccountID, now)
	})
}

// Descriptor is the catalogue entry.
func (h DiscardTimer) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DiscardTimerName, Summary: "Stops the caller's timer without recording its time.",
		SideEffects: "Deletes the timer and writes an audit entry.",
		TokenScope:  itemsWrite, Destructive: true,
		Audit: usecase.AuditDeclaration{Action: TimerDiscardedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, h.Execute(ctx, actor)
		}),
	}, timerTarget)
}

// ReadTimer answers the caller's running timer.
type ReadTimer struct{ Keeper Keeper }

// Execute reads it.
func (h ReadTimer) Execute(ctx context.Context, actor appshared.ActorContext) (domain.Timer, bool, error) {
	if err := person(actor); err != nil {
		return domain.Timer{}, false, err
	}
	var (
		timer   domain.Timer
		running bool
	)
	err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		timer, running, err = h.Keeper.Store.Timer(ctx, actor.AccountID)
		return err
	})
	return timer, running, err
}

// Descriptor is the catalogue entry.
func (h ReadTimer) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadTimerName, Summary: "Answers the caller's running timer, or `running: false`.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			t, running, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			if !running {
				return usecase.Output{"running": false}, nil
			}
			return TimerOutput(t, h.Keeper.Clock.Now()), nil
		}),
	}, timerTarget)
}
