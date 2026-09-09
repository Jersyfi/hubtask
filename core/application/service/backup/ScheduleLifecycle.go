// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The lifecycle a schedule had no operations for until F4-02: it could be created, and then only
// read by the poller that ran it.
const (
	ListBackupSchedulesName  = "ListBackupSchedules"
	UpdateBackupScheduleName = "UpdateBackupSchedule"
	DeleteBackupScheduleName = "DeleteBackupSchedule"

	// ScheduleReadAction exists for the refusal's sake: a denied read is recorded against it.
	ScheduleReadAction audit.Action = "backup.schedule_read"

	// ScheduleRemovedAction is its own code rather than a `schedule_changed` with an empty after.
	// "Somebody edited the nightly backup" and "somebody removed it" are the two different things
	// a reader of the trail is looking for, and one code for both would make them one thing.
	ScheduleRemovedAction audit.Action = "backup.schedule_removed"
)

// ListBackupSchedules answers what runs on its own, and when it next will.
type ListBackupSchedules struct{ Scheduling Scheduling }

// Execute lists them.
func (h ListBackupSchedules) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.Schedule, error) {
	s := h.Scheduling
	if err := s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		// A-4, G-12: a schedule is what a workspace has decided will leave it every night, which
		// is exactly the configuration an auditor reads without being able to change it.
		Alternative: service.PermissionReadConfiguration,
		Path:        []identity.Scope{identity.TenantScope()},
		Action:      ScheduleReadAction,
		TokenScope:  backupRead,
		TargetType:  scheduleType,
	}); err != nil {
		return nil, err
	}

	var schedules []domain.Schedule
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		listed, err := s.Schedules.List(ctx)
		schedules = listed
		return err
	})
	if err != nil {
		return nil, err
	}
	return schedules, nil
}

// UpdateBackupSchedule changes a schedule, or switches it off.
type UpdateBackupSchedule struct{ Scheduling Scheduling }

// UpdateBackupScheduleCommand is the merge-patch, typed. A nil pointer is a key the caller did
// not send. The target and the scope are absent by construction: neither may move.
type UpdateBackupScheduleCommand struct {
	ID              shared.ID
	RRULE           *string
	TimeZone        *string
	Mode            *domain.Mode
	FullRRULE       *string
	IncludeMedia    *bool
	IncludeAudit    *bool
	Retention       *domain.Retention
	NotifyOn        []domain.Notification
	Enabled         *bool
	ExpectedVersion int
}

// Execute applies the patch, recomputes when the schedule is next owed, and writes both.
//
// The recomputation happens here rather than on the next read, for the reason creating one does
// it here: what is stored is the moment, and a poller that re-derived it would pay a library call
// for every schedule that is not due. A schedule switched off owes nothing at all, so its moment
// is cleared - and switching it back on computes the next one afresh rather than resuming a
// moment that has since passed.
func (h UpdateBackupSchedule) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateBackupScheduleCommand,
) (domain.Schedule, error) {
	s := h.Scheduling
	if err := s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ScheduleChangedAction,
		TokenScope: backupManage,
		TargetType: scheduleType,
		TargetID:   cmd.ID,
	}); err != nil {
		return domain.Schedule{}, err
	}

	now := s.Clock.Now()
	var answer domain.Schedule
	err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := s.Schedules.Find(ctx, cmd.ID)
		if err != nil {
			return err
		}

		after, err := cmd.applyTo(before)
		if err != nil {
			return err
		}

		// A rule this installation cannot read is refused here, as a field error on the rule,
		// rather than as a job that fails at three in the morning.
		nextRunAt := time.Time{}
		if after.Enabled {
			nextRunAt, err = s.nextOccurrence(after, now)
			if err != nil {
				return err
			}
		}
		after.NextRunAt = nextRunAt

		expected := before.Version
		if cmd.ExpectedVersion > 0 {
			expected = cmd.ExpectedVersion
		}
		written, err := s.Schedules.Update(ctx, after, nextRunAt, expected)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrConflict.
				WithDetail(domain.CodeScheduleVersionConflict).
				WithParams(map[string]string{"schedule_id": cmd.ID.String()})
		}

		after.Version = expected + 1
		answer = after

		// Seed or pull forward this tenant's poller, the way creating one does. A schedule that
		// now runs earlier than anything else has to be woken for; one switched off leaves the
		// existing wake-up alone, which costs a poll that finds nothing.
		if err := s.wake(ctx, actor.TenantID, nextRunAt); err != nil {
			return err
		}
		return s.recordChange(ctx, actor, before, after, now)
	})
	if err != nil {
		return domain.Schedule{}, err
	}
	return answer, nil
}

// applyTo folds the patch into the stored schedule and revalidates the whole of it.
//
// Revalidated rather than patched in place: `NewSchedule` is where a rule, a zone and a
// generation plan are judged, and a change that skipped it could store a schedule creating one
// would have refused.
func (cmd UpdateBackupScheduleCommand) applyTo(before domain.Schedule) (domain.Schedule, error) {
	in := domain.NewScheduleInput{
		ID: before.ID, TargetID: before.TargetID, TenantID: before.TenantID,
		Scope: before.Scope, ScopeID: before.ScopeID,
		RRULE: before.RRULE, TimeZone: before.TimeZone, Mode: before.Mode,
		FullRRULE: before.FullRRULE, IncludeMedia: before.IncludeMedia,
		IncludeAudit: before.IncludeAudit, Retention: before.Retention,
		NotifyOn: before.NotifyOn, Now: before.CreatedAt,
	}
	if cmd.RRULE != nil {
		in.RRULE = *cmd.RRULE
	}
	if cmd.TimeZone != nil {
		in.TimeZone = *cmd.TimeZone
	}
	if cmd.Mode != nil {
		in.Mode = *cmd.Mode
	}
	if cmd.FullRRULE != nil {
		in.FullRRULE = *cmd.FullRRULE
	}
	if cmd.IncludeMedia != nil {
		in.IncludeMedia = *cmd.IncludeMedia
	}
	if cmd.IncludeAudit != nil {
		in.IncludeAudit = *cmd.IncludeAudit
	}
	if cmd.Retention != nil {
		in.Retention = *cmd.Retention
	}
	if cmd.NotifyOn != nil {
		in.NotifyOn = cmd.NotifyOn
	}

	after, err := domain.NewSchedule(in)
	if err != nil {
		return domain.Schedule{}, err
	}
	// `NewSchedule` decides a fresh schedule, which is enabled; what was stored decides otherwise.
	after.Enabled = before.Enabled
	if cmd.Enabled != nil {
		after.Enabled = *cmd.Enabled
	}
	after.CreatedAt = before.CreatedAt
	after.Version = before.Version
	return after, nil
}

// DeleteBackupSchedule removes one.
type DeleteBackupSchedule struct{ Scheduling Scheduling }

// Execute removes it. What it produced stays: the archives at the target, and the runs in the log.
func (h DeleteBackupSchedule) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) error {
	s := h.Scheduling
	if err := s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ScheduleRemovedAction,
		TokenScope: backupManage,
		TargetType: scheduleType,
		TargetID:   id,
	}); err != nil {
		return err
	}

	now := s.Clock.Now()
	return s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := s.Schedules.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			// Nothing to remove is not a failure: the caller asked for it to be gone and it is.
			return nil
		}
		return s.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: ScheduleRemovedAction, Outcome: audit.OutcomeSuccess,
			Severity:  audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: scheduleType, TargetID: id,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
}

// recordChange writes the entry with the before and after of every field that moved.
//
// Every field is `audit.Open`: a schedule carries a rule, a zone and a generation plan, and none
// of it is anybody's content. What an auditor needs from this entry is exactly the pair - "it ran
// at three and now it runs at eleven" is the finding, and an entry with only the new value cannot
// produce it.
func (s Scheduling) recordChange(
	ctx context.Context, actor appshared.ActorContext, before, after domain.Schedule, now time.Time,
) error {
	changes := []audit.Change{}
	add := func(field, from, to string) {
		if from != to {
			changes = append(changes, audit.Change{
				Field: field, Classification: audit.Open, From: from, To: to,
			})
		}
	}
	add("rrule", before.RRULE, after.RRULE)
	add("timezone", before.TimeZone, after.TimeZone)
	add("mode", before.Mode.String(), after.Mode.String())
	add("full_rrule", before.FullRRULE, after.FullRRULE)
	add("enabled", boolText(before.Enabled), boolText(after.Enabled))
	add("include_media", boolText(before.IncludeMedia), boolText(after.IncludeMedia))
	add("include_audit", boolText(before.IncludeAudit), boolText(after.IncludeAudit))

	return s.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now,
		Action: ScheduleChangedAction, Outcome: audit.OutcomeSuccess,
		Severity:  audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: scheduleType, TargetID: after.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (h ListBackupSchedules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListBackupSchedulesName,
		Summary: "What the workspace has decided will happen on its own: the rule, the zone it " +
			"is read in, what each occurrence produces, the generation plan, and when it is next " +
			"owed. A schedule that is switched off, or whose recurrence is spent, owes no next " +
			"moment and says so rather than disappearing.",
		SideEffects: "None. Reads only.",
		TokenScope:  backupRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: ScheduleReadAction, TargetType: scheduleType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListBackupSchedules) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	schedules, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(schedules))
	for _, schedule := range schedules {
		rows = append(rows, scheduleOutput(schedule))
	}
	return usecase.Output{"data": rows}, nil
}

func (h UpdateBackupSchedule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateBackupScheduleName,
		Summary: "Changes a schedule, or switches it off. A field the caller does not send does " +
			"not move. `enabled: false` keeps the rule and owes nothing, which is what somebody " +
			"reaches for while a target is being replaced; switching it back on computes the " +
			"next moment afresh. The target and the scope do not move at all - a schedule that " +
			"changed either would be a different schedule under an old identifier.",
		SideEffects: "Writes the schedule, recomputes when it is next owed, wakes this tenant's " +
			"backup poller, and writes an audit entry naming every field that moved.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{Name: "schedule_id", Kind: usecase.KindID, Required: true},
			{Name: "rrule", Kind: usecase.KindString, Description: "RFC 5545, without a DTSTART."},
			{Name: "timezone", Kind: usecase.KindString, Description: "The IANA zone the rule is read in."},
			{Name: "mode", Kind: usecase.KindString, Enum: []string{"FULL", "INCREMENTAL"},
				Description: "What an occurrence produces."},
			{Name: "full_rrule", Kind: usecase.KindString,
				Description: "Which of the rule's occurrences are full ones. It adds no run of its own."},
			{Name: "include_media", Kind: usecase.KindBool},
			{Name: "include_audit", Kind: usecase.KindBool},
			{Name: "retention", Kind: usecase.KindObject,
				Description: "The generation plan, whole. What it leaves out keeps the stored value."},
			{Name: "notify_on", Kind: usecase.KindList,
				Description: "FAILURE, SUCCESS, FIRST_SUCCESS_AFTER_FAILURE. Replaces the set."},
			{Name: "enabled", Kind: usecase.KindBool,
				Description: "Off keeps the rule and owes nothing."},
			{Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller named none."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ScheduleChangedAction, TargetType: scheduleType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateBackupSchedule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("schedule_id")
	if err != nil {
		return nil, err
	}
	cmd := UpdateBackupScheduleCommand{ID: id, ExpectedVersion: in.Int("expected_version")}
	cmd.RRULE = in.OptionalString("rrule")
	cmd.TimeZone = in.OptionalString("timezone")
	cmd.FullRRULE = in.OptionalString("full_rrule")
	if mode := in.OptionalString("mode"); mode != nil {
		wanted := domain.Mode(*mode)
		cmd.Mode = &wanted
	}
	if in.Present("include_media") {
		wanted := in.Bool("include_media")
		cmd.IncludeMedia = &wanted
	}
	if in.Present("include_audit") {
		wanted := in.Bool("include_audit")
		cmd.IncludeAudit = &wanted
	}
	if in.Present("enabled") {
		wanted := in.Bool("enabled")
		cmd.Enabled = &wanted
	}
	if plan, present := in["retention"].(map[string]any); present {
		wanted := retentionFrom(plan, domain.DefaultRetention())
		cmd.Retention = &wanted
	}
	if in.Present("notify_on") {
		occasions, err := in.StringList("notify_on")
		if err != nil {
			return nil, err
		}
		cmd.NotifyOn = make([]domain.Notification, 0, len(occasions))
		for _, occasion := range occasions {
			cmd.NotifyOn = append(cmd.NotifyOn, domain.Notification(occasion))
		}
	}

	schedule, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return scheduleOutput(schedule), nil
}

func (h DeleteBackupSchedule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteBackupScheduleName,
		Summary: "Removes a schedule. The archives it produced stay at the target and the runs " +
			"it started stay in the log; what stops is the recurrence. Switching one off is " +
			"usually the right operation and this one is not - it keeps the rule for the night " +
			"somebody wants it back.",
		SideEffects: "Removes the schedule and writes an audit entry.",
		TokenScope:  backupManage,
		Input: []usecase.Field{
			{Name: "schedule_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: ScheduleRemovedAction, TargetType: scheduleType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteBackupSchedule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("schedule_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
