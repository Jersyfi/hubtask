// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateBackupScheduleName = "CreateBackupSchedule"

	scheduleType = "backup_schedule"

	// ScheduleChangedAction is a recurring egress channel being established or altered. A warning
	// for the reason creating a target is one, and more so: a target is a place data *may* go,
	// and a schedule is the decision that it *will*, every night, without anybody asking again.
	ScheduleChangedAction audit.Action = "backup.schedule_changed"
)

// scheduleHorizon is how far ahead the first occurrence of a rule is looked for.
//
// A year, because a rule that produces nothing in a year produces nothing anybody is waiting for -
// `FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=29` is the shape that gets close, and it still lands inside four.
// A schedule with no next moment is stored with none rather than refused: the rule may be perfectly
// good and simply exhausted, and a schedule an operator can see and edit is better than an error
// that loses what they typed.
const scheduleHorizon = 366 * 24 * time.Hour

// Scheduling is what the schedule use cases share.
type Scheduling struct {
	Schedules  repository.Schedules
	Targets    repository.Targets
	Jobs       queue.Queue
	Expander   recurrence.Expander
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// CreateBackupSchedule writes down that this should happen on its own from now on.
type CreateBackupSchedule struct{ Scheduling Scheduling }

// CreateBackupScheduleCommand is the input, typed.
type CreateBackupScheduleCommand struct {
	TargetID     shared.ID
	Scope        domain.ScheduleScope
	ScopeID      shared.ID
	RRULE        string
	TimeZone     string
	Mode         domain.Mode
	FullRRULE    string
	IncludeMedia bool
	IncludeAudit bool
	Retention    domain.Retention
	NotifyOn     []domain.Notification
}

// Execute writes the schedule and decides when it first runs.
func (h CreateBackupSchedule) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateBackupScheduleCommand,
) (domain.Schedule, error) {
	// The owner's right, the same line creating a target needs. A target is a place the data
	// *may* go; a schedule is the decision that it *will*, every night, without anybody being
	// asked again - which is more of a decision rather than less.
	if err := h.Scheduling.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ScheduleChangedAction,
		TokenScope: backupManage,
		TargetType: scheduleType,
		TargetID:   cmd.TargetID,
	}); err != nil {
		return domain.Schedule{}, err
	}

	scope := cmd.Scope
	if scope == "" {
		scope = domain.ScopeTenant
	}
	now := h.Scheduling.Clock.Now()
	retention := cmd.Retention
	if retention == (domain.Retention{}) {
		retention = domain.DefaultRetention()
	}

	schedule, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: h.Scheduling.IDs.NewID(), TargetID: cmd.TargetID, TenantID: actor.TenantID,
		Scope: scope, ScopeID: cmd.ScopeID, RRULE: cmd.RRULE, TimeZone: cmd.TimeZone,
		Mode: cmd.Mode, FullRRULE: cmd.FullRRULE,
		IncludeMedia: cmd.IncludeMedia, IncludeAudit: cmd.IncludeAudit,
		Retention: retention, NotifyOn: cmd.NotifyOn, Now: now,
	})
	if err != nil {
		return domain.Schedule{}, err
	}

	// The rule is expanded once, here, rather than on every read: what is stored is the moment,
	// and a poller that re-derived it would pay a library call for every schedule that is not due.
	// It is also the first place a rule this installation cannot read is refused - as a field
	// error on the rule rather than as a job that fails at three in the morning.
	nextRunAt, err := h.Scheduling.nextOccurrence(schedule, now)
	if err != nil {
		return domain.Schedule{}, err
	}
	schedule.NextRunAt = nextRunAt

	err = h.Scheduling.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Scheduling.Targets.Find(ctx, cmd.TargetID); err != nil {
			return err
		}
		if err := h.Scheduling.Schedules.Insert(ctx, schedule, nextRunAt); err != nil {
			return err
		}
		// The write that creates the work seeds the job for its own tenant, which is how every
		// per-tenant job in this system starts: nothing may enumerate tenants, so a scheduler
		// cannot create one job per tenant even if it wanted to (multi-tenancy.md §2.1).
		if err := h.Scheduling.wake(ctx, actor.TenantID, nextRunAt); err != nil {
			return err
		}
		return h.Scheduling.record(ctx, actor, schedule, now)
	})
	if err != nil {
		return domain.Schedule{}, err
	}
	return schedule, nil
}

// nextOccurrence answers when the rule next fires after a moment, and the zero time when it never
// does again.
func (s Scheduling) nextOccurrence(
	schedule domain.Schedule, after time.Time,
) (time.Time, error) {
	moments, err := s.Expander.Occurrences(recurrence.Rule{
		RRULE: schedule.RRULE, TimeZone: schedule.TimeZone, Start: schedule.Anchor(),
	}, after, after.Add(scheduleHorizon), 1)
	if err != nil {
		return time.Time{}, ruleRefusal(err)
	}
	if len(moments) == 0 {
		return time.Time{}, nil
	}
	return moments[0].UTC(), nil
}

// ruleRefusal turns the expander's two sentinels into the field errors a client can act on.
func ruleRefusal(err error) error {
	switch {
	case errors.Is(err, recurrence.ErrRuleUnreadable):
		return shared.ErrValidation.WithDetail(domain.CodeScheduleRuleUnreadable).
			WithFields(shared.FieldError{Path: "/rrule", Code: domain.CodeScheduleRuleUnreadable})
	case errors.Is(err, recurrence.ErrZoneUnknown):
		return shared.ErrValidation.WithDetail(domain.CodeScheduleZoneUnknown).
			WithFields(shared.FieldError{Path: "/timezone", Code: domain.CodeScheduleZoneUnknown})
	}
	return shared.Internalf("backup: expanding a schedule: %w", err)
}

// wake seeds or pulls forward this tenant's backup poller.
//
// One job per tenant, rescheduling itself, seeded by the write that made something owed - the shape
// the reminders and the recurrence materialisation already use, and for the same reason: nothing in
// this system may enumerate tenants. `Enqueue`'s conflict clause pulls an existing wake-up forward
// rather than adding a row, so a tenant with forty schedules still has one job.
func (s Scheduling) wake(ctx context.Context, tenantID shared.ID, at time.Time) error {
	if at.IsZero() {
		return nil
	}
	_, err := s.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindBackupSchedule,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
		RunAt:     at.UTC(),
	})
	return err
}

// record writes the entry an auditor is looking for: a recurring channel the tenant's data will
// leave by, from now on, without anybody being asked again.
func (s Scheduling) record(
	ctx context.Context, actor appshared.ActorContext, schedule domain.Schedule, now time.Time,
) error {
	return s.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now,
		Action: ScheduleChangedAction, Outcome: audit.OutcomeSuccess,
		Severity:  audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: scheduleType, TargetID: schedule.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "target_id", Classification: audit.Open, To: schedule.TargetID.String()},
			audit.Change{Field: "scope", Classification: audit.Open, To: schedule.Scope.String()},
			audit.Change{Field: "rrule", Classification: audit.Open, To: schedule.RRULE},
			audit.Change{Field: "timezone", Classification: audit.Open, To: schedule.TimeZone},
			audit.Change{Field: "mode", Classification: audit.Open, To: schedule.Mode.String()},
		),
	})
}

// Descriptor registers the schedule in all three channels.
func (h CreateBackupSchedule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateBackupScheduleName,
		Summary: "Writes down that a backup should happen on its own from now on: an RFC 5545 " +
			"rule, a time zone so that its hour survives the clocks changing, and a generation " +
			"plan for what to keep. The rule counts from the moment the schedule was created, " +
			"because a backup schedule has no due date to count from. full_rrule does not add " +
			"runs - it marks which of the rule's own occurrences are full ones. Needs the " +
			"owner's right: a schedule is the decision that the data will leave every night " +
			"without anybody being asked again.",
		SideEffects: "Writes the schedule, decides when it first runs, wakes this tenant's " +
			"backup poller, and writes an audit entry.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "Where the archives go.",
			},
			{
				Name: "rrule", Kind: usecase.KindString, Required: true,
				Description: "RFC 5545, without a DTSTART: FREQ=DAILY;BYHOUR=3;BYMINUTE=0.",
			},
			{
				Name: "timezone", Kind: usecase.KindString,
				Description: "The IANA zone the rule is read in. UTC unless said otherwise, and " +
					"worth saying otherwise: three in the morning is three in the morning only " +
					"where somebody is.",
			},
			{
				Name: "scope", Kind: usecase.KindString,
				Enum:        []string{"TENANT", "HUB", "COLLECTION"},
				Description: "What is backed up. The whole tenant unless said otherwise.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "Which hub or collection, for those scopes.",
			},
			{
				Name: "mode", Kind: usecase.KindString,
				Enum:        []string{"FULL", "INCREMENTAL"},
				Description: "What an occurrence produces. INCREMENTAL unless said otherwise.",
			},
			{
				Name: "full_rrule", Kind: usecase.KindString,
				Description: "Which of the rule's occurrences are full ones, as a rule over days: " +
					"FREQ=WEEKLY;BYDAY=SU makes the Sunday run the full one. It does not add a " +
					"run of its own.",
			},
			{Name: "include_media", Kind: usecase.KindBool, Description: "Whether attachments travel with it."},
			{Name: "include_audit", Kind: usecase.KindBool, Description: "Whether the audit trail travels with it."},
			{
				Name: "retention", Kind: usecase.KindObject,
				Description: "The generation plan: keep_last, keep_daily, keep_weekly, " +
					"keep_monthly, keep_yearly, and min_keep as a floor no rule may breach.",
			},
			{
				Name: "notify_on", Kind: usecase.KindList,
				Description: "FAILURE, SUCCESS, FIRST_SUCCESS_AFTER_FAILURE. FAILURE unless said otherwise.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ScheduleChangedAction, TargetType: scheduleType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateBackupSchedule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	cmd := CreateBackupScheduleCommand{
		TargetID: targetID,
		RRULE:    in.String("rrule"),
		TimeZone: "UTC",
		// Both default to true, because a backup that quietly left the attachments behind is not
		// the backup anybody meant.
		IncludeMedia: !in.Present("include_media") || in.Bool("include_media"),
		IncludeAudit: !in.Present("include_audit") || in.Bool("include_audit"),
		Retention:    domain.DefaultRetention(),
	}
	if zone := in.OptionalString("timezone"); zone != nil && *zone != "" {
		cmd.TimeZone = *zone
	}
	if scope := in.OptionalString("scope"); scope != nil && *scope != "" {
		cmd.Scope = domain.ScheduleScope(*scope)
	}
	if in.Present("scope_id") {
		scopeID, err := in.ID("scope_id")
		if err != nil {
			return nil, err
		}
		cmd.ScopeID = scopeID
	}
	if mode := in.OptionalString("mode"); mode != nil && *mode != "" {
		cmd.Mode = domain.Mode(*mode)
	}
	if rule := in.OptionalString("full_rrule"); rule != nil {
		cmd.FullRRULE = *rule
	}
	occasions, err := in.StringList("notify_on")
	if err != nil {
		return nil, err
	}
	for _, occasion := range occasions {
		cmd.NotifyOn = append(cmd.NotifyOn, domain.Notification(occasion))
	}
	if plan, present := in["retention"].(map[string]any); present {
		cmd.Retention = retentionFrom(plan, cmd.Retention)
	}

	schedule, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return scheduleOutput(schedule), nil
}

// retentionFrom reads the plan a caller sent, keeping the defaults for whatever it left out. A
// caller that names only `min_keep` means "the usual plan, with a higher floor" rather than "keep
// nothing but the floor".
func retentionFrom(plan map[string]any, defaults domain.Retention) domain.Retention {
	read := func(name string, into *int) {
		if value, present := plan[name].(float64); present {
			*into = int(value)
		}
	}
	read("keep_last", &defaults.KeepLast)
	read("keep_daily", &defaults.KeepDaily)
	read("keep_weekly", &defaults.KeepWeekly)
	read("keep_monthly", &defaults.KeepMonthly)
	read("keep_yearly", &defaults.KeepYearly)
	read("min_keep", &defaults.MinKeep)
	return defaults
}

// scheduleOutput is a schedule as the three channels answer it.
func scheduleOutput(schedule domain.Schedule) usecase.Output {
	occasions := make([]string, 0, len(schedule.NotifyOn))
	for _, occasion := range schedule.NotifyOn {
		occasions = append(occasions, string(occasion))
	}

	out := usecase.Output{
		"id":        schedule.ID.String(),
		"target_id": schedule.TargetID.String(),
		"scope": map[string]any{
			"kind": schedule.Scope.String(),
			"id":   optionalIdentifier(schedule.ScopeID),
		},
		"rrule":         schedule.RRULE,
		"timezone":      schedule.TimeZone,
		"mode":          schedule.Mode.String(),
		"include_media": schedule.IncludeMedia,
		"include_audit": schedule.IncludeAudit,
		"retention": map[string]any{
			"keep_last": schedule.Retention.KeepLast, "keep_daily": schedule.Retention.KeepDaily,
			"keep_weekly": schedule.Retention.KeepWeekly, "keep_monthly": schedule.Retention.KeepMonthly,
			"keep_yearly": schedule.Retention.KeepYearly, "min_keep": schedule.Retention.MinKeep,
		},
		"notify_on": occasions,
		"enabled":   schedule.Enabled,
	}
	if schedule.FullRRULE != "" {
		out["full_rrule"] = schedule.FullRRULE
	}
	if !schedule.NextRunAt.IsZero() {
		out["next_run_at"] = schedule.NextRunAt
	}
	return out
}

func optionalIdentifier(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}
