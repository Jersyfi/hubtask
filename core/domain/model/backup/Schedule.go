// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ScheduleScope is what a schedule backs up.
//
// Four values, because `0001_init` has had four since phase 0 and the check constraint ties the
// last of them to the tenant column: an INSTANCE schedule has no tenant, and a schedule with a
// tenant is never an INSTANCE one. That pairing is the whole reason this is a kind and not a
// boolean - it is what decides whether a schedule is a tenant's own work or the operator's.
type ScheduleScope string

const (
	// ScopeInstance is the operator's: everything this installation holds, and it belongs to no
	// tenant. It is the one shape a scheduler may look for by itself.
	ScopeInstance ScheduleScope = "INSTANCE"
	// ScopeTenant is a whole tenant, and the only scope a tenant's own schedule can have today.
	ScopeTenant ScheduleScope = "TENANT"
	// ScopeHub and ScopeCollection are a container and what hangs below it. The column allows
	// them; nothing creates one yet, and a restore that met one would have to know what it means,
	// which is why they are named rather than silently absent.
	ScopeHub        ScheduleScope = "HUB"
	ScopeCollection ScheduleScope = "COLLECTION"
)

var scheduleScopes = [...]ScheduleScope{ScopeInstance, ScopeTenant, ScopeHub, ScopeCollection}

func (s ScheduleScope) String() string { return string(s) }

func (s ScheduleScope) Valid() bool { return slices.Contains(scheduleScopes[:], s) }

// ScheduleScopes is the whole enum, for a caller that has to render it.
func ScheduleScopes() []ScheduleScope { return slices.Clone(scheduleScopes[:]) }

// Mode is what a run produces.
type Mode string

const (
	ModeFull        Mode = "FULL"
	ModeIncremental Mode = "INCREMENTAL"
)

func (m Mode) Valid() bool { return m == ModeFull || m == ModeIncremental }

func (m Mode) String() string { return string(m) }

// Notification is an occasion a schedule tells somebody about.
type Notification string

const (
	NotifyFailure                  Notification = "FAILURE"
	NotifySuccess                  Notification = "SUCCESS"
	NotifyFirstSuccessAfterFailure Notification = "FIRST_SUCCESS_AFTER_FAILURE"
)

var notifications = [...]Notification{NotifyFailure, NotifySuccess, NotifyFirstSuccessAfterFailure}

func (n Notification) Valid() bool { return slices.Contains(notifications[:], n) }

// Retention is the generation plan and the floor beneath it (backup-restore.md §6, ADR-0019
// decision 4).
//
// The five generations are the ones every established backup tool has, and they are counts rather
// than periods on purpose: "keep twelve monthly backups" survives a month with no run in it, where
// "keep archives younger than a year" quietly keeps nothing at all if the schedule was off.
type Retention struct {
	KeepLast    int
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
	KeepYearly  int
	// MinKeep is the floor a rule may never breach: whatever the generations work out to, this
	// many archives stay. It is the answer to the failure mode that makes retention frightening -
	// a plan that is misread, or a clock that jumps, deleting the last copy of everything.
	MinKeep int
}

// DefaultRetention is the plan `0001_init` puts on a schedule that does not name one, and the one
// the contract documents as its defaults.
func DefaultRetention() Retention {
	return Retention{KeepLast: 7, KeepDaily: 14, KeepWeekly: 8, KeepMonthly: 12, KeepYearly: 3, MinKeep: 3}
}

// Schedule is a target, a rule, and what to do with what it produces.
type Schedule struct {
	ID       shared.ID
	TargetID shared.ID
	// TenantID is whose schedule it is, and empty exactly when the scope is INSTANCE.
	TenantID shared.ID
	Scope    ScheduleScope
	// ScopeID names the container for a HUB or COLLECTION scope. Empty otherwise: an INSTANCE
	// schedule has nothing to name, and a TENANT one is named by TenantID already.
	ScopeID shared.ID
	// RRULE is when a run happens, in RFC 5545, without a DTSTART - see Anchor for what stands in
	// for one.
	RRULE string
	// TimeZone is the IANA name the rule is read in. It is what makes "three in the morning" mean
	// three in the morning through a daylight saving transition rather than an hour either side.
	TimeZone string
	// Mode is what an occurrence produces unless FullRRULE promotes it.
	Mode Mode
	// FullRRULE marks which of RRULE's occurrences are full ones. It does not produce occurrences
	// of its own - see IsFullOn.
	FullRRULE    string
	IncludeMedia bool
	IncludeAudit bool
	Retention    Retention
	NotifyOn     []Notification
	Enabled      bool
	NextRunAt    time.Time
	CreatedAt    time.Time
	Version      int
}

// NewScheduleInput is what a caller supplies; everything else is decided here.
type NewScheduleInput struct {
	ID           shared.ID
	TargetID     shared.ID
	TenantID     shared.ID
	Scope        ScheduleScope
	ScopeID      shared.ID
	RRULE        string
	TimeZone     string
	Mode         Mode
	FullRRULE    string
	IncludeMedia bool
	IncludeAudit bool
	Retention    Retention
	NotifyOn     []Notification
	Now          time.Time
}

// NewSchedule builds a schedule, or says which field is wrong.
//
// The rule text itself is not parsed here: expanding RFC 5545 is a library's job and the domain
// has no libraries (ADR-0001, ADR-0008). What is checked here is everything a rule's meaning does
// not depend on - that there is one, that the scope and the tenant agree, and that the plan cannot
// delete the last copy of anything.
func NewSchedule(in NewScheduleInput) (Schedule, error) {
	var fields []shared.FieldError
	add := func(path, code string) { fields = append(fields, field(path, code)) }

	if in.TargetID.IsZero() {
		add("/target_id", CodeScheduleTargetRequired)
	}
	if !in.Scope.Valid() {
		add("/scope/kind", CodeScheduleScopeInvalid)
	}
	// The pairing `0001_init`'s check constraint makes, refused here so that a person configuring
	// a backup gets a field error rather than a database error.
	if in.Scope == ScopeInstance && !in.TenantID.IsZero() {
		add("/scope/kind", CodeScheduleInstanceHasNoTenant)
	}
	if in.Scope != ScopeInstance && in.TenantID.IsZero() {
		add("/scope/kind", CodeScheduleTenantRequired)
	}
	if (in.Scope == ScopeHub || in.Scope == ScopeCollection) && in.ScopeID.IsZero() {
		add("/scope/id", CodeScheduleScopeIDRequired)
	}
	if strings.TrimSpace(in.RRULE) == "" {
		add("/rrule", CodeScheduleRuleRequired)
	}
	if strings.TrimSpace(in.TimeZone) == "" {
		add("/timezone", CodeScheduleTimeZoneRequired)
	}
	mode := in.Mode
	if mode == "" {
		mode = ModeIncremental
	}
	if !mode.Valid() {
		add("/mode", CodeScheduleModeInvalid)
	}
	for _, occasion := range in.NotifyOn {
		if !occasion.Valid() {
			add("/notify_on", CodeScheduleNotificationInvalid)
		}
	}
	fields = append(fields, in.Retention.problems()...)

	if len(fields) > 0 {
		return Schedule{}, shared.ErrValidation.WithDetail(CodeScheduleInvalid).WithFields(fields...)
	}

	notify := slices.Clone(in.NotifyOn)
	if len(notify) == 0 {
		notify = []Notification{NotifyFailure}
	}
	return Schedule{
		ID: in.ID, TargetID: in.TargetID, TenantID: in.TenantID,
		Scope: in.Scope, ScopeID: in.ScopeID,
		RRULE: strings.TrimSpace(in.RRULE), TimeZone: strings.TrimSpace(in.TimeZone),
		Mode: mode, FullRRULE: strings.TrimSpace(in.FullRRULE),
		IncludeMedia: in.IncludeMedia, IncludeAudit: in.IncludeAudit,
		Retention: in.Retention, NotifyOn: notify,
		Enabled: true, CreatedAt: in.Now, Version: 1,
	}, nil
}

// problems reports what makes a retention plan one nobody should be given.
func (r Retention) problems() []shared.FieldError {
	var fields []shared.FieldError
	for path, value := range map[string]int{
		"/retention/keep_last":    r.KeepLast,
		"/retention/keep_daily":   r.KeepDaily,
		"/retention/keep_weekly":  r.KeepWeekly,
		"/retention/keep_monthly": r.KeepMonthly,
		"/retention/keep_yearly":  r.KeepYearly,
	} {
		if value < 0 {
			fields = append(fields, field(path, CodeRetentionNegative))
		}
	}
	// The floor is the one number that may not be zero. A plan with no floor is a plan that can
	// arrive at "keep nothing", and the whole point of §6's second level is that it cannot.
	if r.MinKeep < 1 {
		fields = append(fields, field("/retention/min_keep", CodeRetentionFloorRequired))
	}
	// Sorted, so that two runs of the same input produce the same error. A map's order is
	// deliberately not an order.
	slices.SortFunc(fields, func(a, b shared.FieldError) int { return strings.Compare(a.Path, b.Path) })
	return fields
}

// Anchor is what stands in for the DTSTART a backup schedule does not have.
//
// A recurring task counts from its due date (domain-model.md §3.5); a backup schedule has no due
// date and no natural first moment, so **it counts from when it was created**, read in its own time
// zone. Two alternatives were available and both are worse. Counting from "now" at every expansion
// makes a rule without a BYDAY drift to whatever weekday the process happened to ask on - a weekly
// backup that moves because a pod restarted on a Tuesday. Counting from midnight of the creation
// day is a value nobody chose and cannot explain afterwards.
//
// What the creation instant means in practice: a rule that pins its own time - `FREQ=DAILY;BYHOUR=3`
// - is unaffected, because BYHOUR decides. A rule that does not pin one fires at the time of day it
// was created at, which is at least a moment somebody was present for and can be read back off the
// schedule.
func (s Schedule) Anchor() time.Time { return s.CreatedAt }

// IsFullOn reports whether an occurrence at that instant produces a full archive.
//
// **`full_rrule` selects among `rrule`'s occurrences rather than producing occurrences of its
// own**, and that is the decision this method exists to state. §5's example is
// `FREQ=DAILY;BYHOUR=3` with `full_every: FREQ=WEEKLY;BYDAY=SU`, and the second rule names no hour
// at all: expanded on its own it would fire at whatever time of day the anchor happens to carry,
// producing a full backup at a moment nobody scheduled and, on the Sunday, a second run beside the
// three o'clock one. Read as a filter it means what "every" means - the daily run on a Sunday is
// the full one.
//
// It also disposes of the question of what happens when both rules fall on the same instant: they
// cannot, because only one of them produces instants. The other decides what an instant is for.
//
// The comparison is by calendar day in the schedule's own zone, not by instant. A rule that names
// a day names a day, and requiring the two expansions to agree to the second would make
// `full_every` work only when it repeated `rrule`'s BYHOUR - which is the trap of writing it twice.
func (s Schedule) IsFullOn(occurrence time.Time, fullDays []time.Time, zone *time.Location) bool {
	if s.Mode == ModeFull {
		return true
	}
	if s.FullRRULE == "" || zone == nil {
		return false
	}
	wanted := occurrence.In(zone).Format(dayLayout)
	for _, day := range fullDays {
		if day.In(zone).Format(dayLayout) == wanted {
			return true
		}
	}
	return false
}

// dayLayout is a calendar day, which is what `full_every` names.
const dayLayout = "2006-01-02"

// IsInstanceWide reports the one shape a scheduler may look for by itself: a schedule that belongs
// to no tenant.
//
// Everything else is a tenant's own work, and a tenant's work is seeded by a write in that tenant -
// nothing in this system enumerates tenants (multi-tenancy.md §2.1). An instance-wide schedule is
// not a tenant's work at all, which is why reading them by `next_run_at` is a leader's duty rather
// than a hole in that rule.
func (s Schedule) IsInstanceWide() bool { return s.Scope == ScopeInstance }

// The refusals, as codes rather than as prose.
const (
	CodeScheduleInvalid             = "backup.schedule_invalid"
	CodeScheduleTargetRequired      = "backup.schedule_target_required"
	CodeScheduleScopeInvalid        = "backup.schedule_scope_invalid"
	CodeScheduleScopeIDRequired     = "backup.schedule_scope_id_required"
	CodeScheduleInstanceHasNoTenant = "backup.schedule_instance_has_no_tenant"
	CodeScheduleTenantRequired      = "backup.schedule_tenant_required"
	CodeScheduleRuleRequired        = "backup.schedule_rule_required"
	CodeScheduleTimeZoneRequired    = "backup.schedule_time_zone_required"
	CodeScheduleModeInvalid         = "backup.schedule_mode_invalid"
	CodeScheduleNotificationInvalid = "backup.schedule_notification_invalid"
	CodeRetentionNegative           = "backup.retention_negative"
	CodeRetentionFloorRequired      = "backup.retention_floor_required"
	CodeScheduleNotFound            = "backup.schedule_not_found"
	CodeRunNotFound                 = "backup.run_not_found"
	CodeRunNotRunning               = "backup.run_not_running"
	CodeNoParentArchive             = "backup.no_parent_archive"
)

// Trigger is why a run happened.
type Trigger string

const (
	TriggerSchedule   Trigger = "SCHEDULE"
	TriggerManual     Trigger = "MANUAL"
	TriggerPreRestore Trigger = "PRE_RESTORE"
	TriggerAPI        Trigger = "API"
)

var triggers = [...]Trigger{TriggerSchedule, TriggerManual, TriggerPreRestore, TriggerAPI}

func (t Trigger) Valid() bool { return slices.Contains(triggers[:], t) }

// RunStatus is where a run stands.
type RunStatus string

const (
	RunRunning   RunStatus = "RUNNING"
	RunSucceeded RunStatus = "SUCCEEDED"
	RunFailed    RunStatus = "FAILED"
	RunCancelled RunStatus = "CANCELLED"
	// RunExpired is a run whose archive the generation plan has since deleted. The row stays, so
	// that the history of what was backed up survives the archives themselves.
	RunExpired RunStatus = "EXPIRED"
)

func (s RunStatus) Valid() bool {
	return slices.Contains([]RunStatus{RunRunning, RunSucceeded, RunFailed, RunCancelled, RunExpired}, s)
}

// Run is one backup, from the moment it claimed its target to whatever it left behind.
//
// Its ID is the archive's ID in the manifest at the target. One identifier in two places rather
// than a mapping between them: a caller who has a run has an archive, and `:verify` and a restore
// name the thing the caller already holds.
type Run struct {
	ID          shared.ID
	ScheduleID  shared.ID
	TargetID    shared.ID
	TenantID    shared.ID
	ParentRunID shared.ID
	Trigger     Trigger
	Mode        Mode
	Status      RunStatus
	ArchivePath string
	SizeBytes   int64
	ItemCount   int
	MediaCount  int
	Checksum    string
	SnapshotAt  time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	ErrorCode   string
	ExpiresAt   time.Time
	VerifiedAt  time.Time
	VerifyOK    *bool
}

// Outcome is how a run ended, as the one statement that closes it takes it.
type Outcome struct {
	ID          shared.ID
	Status      RunStatus
	ArchivePath string
	Manifest    []byte
	SizeBytes   int64
	ItemCount   int
	MediaCount  int
	Checksum    string
	SnapshotAt  time.Time
	FinishedAt  time.Time
	// ErrorCode is the message code of the failure, never a message and never anything the run
	// was working on (rules 8 and 10).
	ErrorCode string
}

// Succeeded reports a run that left an archive behind.
func (r Run) Succeeded() bool { return r.Status == RunSucceeded }
