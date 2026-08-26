// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	recurrenceport "github.com/Jersyfi/hubtask/core/port/recurrence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetRecurrenceName    = "SetRecurrence"
	RemoveRecurrenceName = "RemoveRecurrence"
	GetRecurrenceName    = "GetRecurrence"

	recurrenceTarget = "recurrence"
	recurrenceWrite  = "recurrence:write"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	RecurrenceSetAction     audit.Action = "recurrence.set"
	RecurrenceChangedAction audit.Action = "recurrence.changed"
	RecurrenceRemovedAction audit.Action = "recurrence.removed"
	// RecurrenceReadAction is the audit code of an attempted read, declared for the reason
	// CommentsReadAction is: an ordinary read writes no entry, a refused one does.
	RecurrenceReadAction audit.Action = "recurrence.read"
)

// RecurrenceWriter is what both directions of a series need: the same reads, the same permission
// question, the same records - and the port that decides whether the text is a rule at all.
type RecurrenceWriter struct {
	Recurrences repository.Recurrences
	Items       repository.Items
	Containers  repository.Containers
	Profiles    metarepo.CapabilityProfiles
	Authorizer  Authorizer
	// Expander is the library behind ADR-0008's decision, as a port: it is what turns "is this a
	// rule, and is it a sane one" from an opinion into an answer, and it is asked before anything
	// is stored rather than by the scheduler afterwards (T-17, R-07).
	Expander recurrenceport.Expander
	Changes  changelog.ChangeLog
	Audit    audit.Sink
	Activity ActivityJournal
	// Jobs is where the series asks to be materialised. The write that made something owed seeds
	// it, because nothing may enumerate tenants (multi-tenancy.md §2.1) - the same shape the
	// reminder's wake-up has (D-05).
	Jobs       queue.Queue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// SetRecurrence puts a series on an entry, or changes the one it has.
//
// One use case for both, which is the one place this task departs from the catalogue's wording:
// §5 names `SetRecurrence` and `UpdateRecurrence`, and they are the same call here for the reason
// the route is one PUT - a series is one document, "every Monday in Berlin, on completion, ninety
// days ahead" is a sentence rather than six settings, and a caller sending it does not know or
// care whether the entry already had one. What differs between the two is the trail: the audit
// entry and the history verb say which of the two happened, decided from what was there.
type SetRecurrence struct {
	Writer RecurrenceWriter
}

// RemoveRecurrence takes the series off an entry, leaving every occurrence it produced standing.
type RemoveRecurrence struct {
	Writer RecurrenceWriter
}

// SetRecurrenceCommand is the input, typed.
type SetRecurrenceCommand struct {
	ItemID shared.ID
	Spec   domain.RecurrenceSpec
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// RemoveRecurrenceCommand is the input, typed.
type RemoveRecurrenceCommand struct {
	ItemID          shared.ID
	ExpectedVersion int
}

// Execute stores the series and returns it.
func (h SetRecurrence) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetRecurrenceCommand,
) (domain.RecurrenceRule, error) {
	w := h.Writer
	if cmd.ItemID.IsZero() {
		return domain.RecurrenceRule{}, itemIDRequired()
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := readItemScope(
		ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return domain.RecurrenceRule{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     RecurrenceSetAction,
		TokenScope: recurrenceWrite,
		TargetType: recurrenceTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.RecurrenceRule{}, err
	}

	var stored domain.RecurrenceRule
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := w.readRecurringItem(ctx, cmd.ItemID)
		if err != nil {
			return err
		}

		current, found, err := w.findRule(ctx, item.ID)
		if err != nil {
			return err
		}
		if found {
			stored, err = w.change(ctx, actor, current, item, cmd, now)
			return err
		}
		stored, err = w.create(ctx, actor, item, cmd, now)
		return err
	})
	if err != nil {
		return domain.RecurrenceRule{}, err
	}
	return stored, nil
}

// create stores a series an entry did not have.
func (w RecurrenceWriter) create(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	cmd SetRecurrenceCommand, now time.Time,
) (domain.RecurrenceRule, error) {
	rule, err := domain.NewRecurrenceRule(domain.NewRecurrenceRuleInput{
		ID:       w.IDs.NewID(),
		TenantID: actor.TenantID,
		ItemID:   item.ID,
		Spec:     cmd.Spec,
		Due:      item.Due,
		Now:      now,
	})
	if err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.ensureExpandable(rule, item); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.Recurrences.Insert(ctx, rule); err != nil {
		return domain.RecurrenceRule{}, err
	}

	if err := w.recordWhole(ctx, actor, rule, item); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.recordAudit(
		ctx, actor, rule, RecurrenceSetAction, wholeRecurrenceAudit(rule), now,
	); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.recordActivity(ctx, actor, item, activity.ItemRecurrenceSet, now); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.scheduleMaterialisation(ctx, rule.TenantID); err != nil {
		return domain.RecurrenceRule{}, err
	}
	return rule, nil
}

// change applies a document to the series an entry already has.
func (w RecurrenceWriter) change(
	ctx context.Context, actor appshared.ActorContext, current domain.RecurrenceRule,
	item domain.WorkItem, cmd SetRecurrenceCommand, now time.Time,
) (domain.RecurrenceRule, error) {
	wanted, changes, err := current.Changed(cmd.Spec, item.Due, now)
	if err != nil {
		return domain.RecurrenceRule{}, err
	}
	if len(changes) == 0 {
		// Already the series asked for. Nothing is written, no version is spent and nothing is
		// recorded - and the If-Match is still honoured, because the state the caller was
		// reasoning about is not the state that is there.
		if err := ensureRecurrenceVersion(current, cmd.ExpectedVersion); err != nil {
			return domain.RecurrenceRule{}, err
		}
		return current, nil
	}
	if err := w.ensureExpandable(wanted, item); err != nil {
		return domain.RecurrenceRule{}, err
	}

	expected := cmd.ExpectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still what the update matches on.
		expected = current.Version
	}
	if err := w.Recurrences.Update(ctx, wanted, expected); err != nil {
		return domain.RecurrenceRule{}, err
	}
	wanted.Version = expected + 1

	if err := w.recordFields(ctx, actor, wanted, item, changes); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.recordAudit(
		ctx, actor, wanted, RecurrenceChangedAction, recurrenceFieldAudit(changes), now,
	); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.recordActivity(ctx, actor, item, activity.ItemRecurrenceChanged, now); err != nil {
		return domain.RecurrenceRule{}, err
	}
	if err := w.scheduleMaterialisation(ctx, wanted.TenantID); err != nil {
		return domain.RecurrenceRule{}, err
	}
	return wanted, nil
}

// scheduleMaterialisation asks for the tenant's series to be looked at, in the transaction that
// wrote one. The dedupe key is the tenant, so a person setting five rules leaves one job, and the
// pass decides what is actually owed (D-05).
//
// A nil queue is a build without one, which the composition root does not produce and a test may:
// nothing to schedule is better than a panic on the write path.
func (w RecurrenceWriter) scheduleMaterialisation(ctx context.Context, tenantID shared.ID) error {
	if w.Jobs == nil {
		return nil
	}
	return w.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindRecurrenceMaterialize,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
	})
}

// Execute takes the series off the entry.
func (h RemoveRecurrence) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RemoveRecurrenceCommand,
) error {
	w := h.Writer
	if cmd.ItemID.IsZero() {
		return itemIDRequired()
	}

	subject, collection, err := readItemScope(
		ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return err
	}

	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     RecurrenceRemovedAction,
		TokenScope: recurrenceWrite,
		TargetType: recurrenceTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return err
	}

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := w.readRecurringItem(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		current, found, err := w.findRule(ctx, item.ID)
		if err != nil {
			return err
		}
		if !found {
			// No series to take off. Nothing is written and nothing is announced - the entry is in
			// the state the caller asked for, which is what makes a repeat harmless.
			return nil
		}

		expected := cmd.ExpectedVersion
		if expected == 0 {
			expected = current.Version
		}
		if err := w.Recurrences.Delete(ctx, current, expected); err != nil {
			return err
		}

		// A deletion carries no payload: there is nothing left to describe, and the occurrences
		// the series produced are ordinary entries that outlive it.
		if err := w.Changes.Record(ctx, changelog.Change{
			TenantID:    current.TenantID,
			Entity:      recurrenceEntity,
			EntityID:    current.ID,
			Op:          changelog.Delete,
			ContainerID: item.CollectionID,
			ActorID:     actor.AccountID,
			HLC:         w.HLC.Next(),
		}); err != nil {
			return err
		}
		if err := w.recordAudit(
			ctx, actor, current, RecurrenceRemovedAction, wholeRecurrenceAudit(current), now,
		); err != nil {
			return err
		}
		return w.recordActivity(ctx, actor, item, activity.ItemRecurrenceRemoved, now)
	})
}

// recurrenceEntity is what a change log entry about a series is called. The name the schema's own
// entity list uses (0001_init), so a client reading a pull does not have to translate.
const recurrenceEntity = "recurrence_rule"

// ensureExpandable asks the port whether the text is a rule at all, and whether it is one anybody
// meant.
//
// Both questions at once, because the answer to the second is the shape of the first: expanding
// the rule inside its own horizon either fails - so it is not a rule - or comes back with a count,
// and a count past the bound is a series that would fill the entry's collection with occurrences
// (T-17). Asked at the write rather than by the scheduler: a rule stored and discovered broken at
// two in the morning is one nobody can fix from the outside.
func (w RecurrenceWriter) ensureExpandable(
	rule domain.RecurrenceRule, item domain.WorkItem,
) error {
	if item.Due == nil {
		return nil
	}
	start, err := item.Due.Anchor()
	if err != nil {
		return err
	}

	moments, err := w.Expander.Occurrences(recurrenceport.Rule{
		RRULE:    rule.RRULE,
		TimeZone: rule.TimeZone,
		Start:    start,
		Until:    endsAt(rule),
		Count:    rule.MaxCount,
	}, start, rule.Horizon(start), domain.MaxOccurrencesPerHorizon+1)
	switch {
	case errors.Is(err, recurrenceport.ErrZoneUnknown):
		return shared.ErrValidation.
			WithDetail("recurrence.time_zone_invalid").
			WithParams(map[string]string{"value": rule.TimeZone}).
			WithFields(shared.FieldError{
				Path: "/time_zone", Code: "recurrence.time_zone_invalid",
				Params: map[string]string{"value": rule.TimeZone},
			})
	case errors.Is(err, recurrenceport.ErrRuleUnreadable):
		return shared.ErrValidation.
			WithDetail("recurrence.rrule_invalid").
			WithParams(map[string]string{"value": rule.RRULE}).
			WithFields(shared.FieldError{
				Path: "/rrule", Code: "recurrence.rrule_invalid",
				Params: map[string]string{"value": rule.RRULE},
			})
	case err != nil:
		return err
	}

	if len(moments) > domain.MaxOccurrencesPerHorizon {
		maximum := strconv.Itoa(domain.MaxOccurrencesPerHorizon)
		return shared.ErrValidation.
			WithDetail("recurrence.rrule_too_dense").
			WithParams(map[string]string{"maximum": maximum}).
			WithFields(shared.FieldError{
				Path: "/rrule", Code: "recurrence.rrule_too_dense",
				Params: map[string]string{"maximum": maximum},
			})
	}
	return nil
}

// endsAt spells the end spec for the port: the instant, or the zero time for a series that runs on.
func endsAt(rule domain.RecurrenceRule) time.Time {
	if rule.EndsAt == nil {
		return time.Time{}
	}
	return *rule.EndsAt
}

// readRecurringItem reads the entry inside the writing transaction and applies the guards every
// series write shares: a collection that accepts no writes, an entry that is trashed or archived,
// and a type that carries no series at all - which is everything but a task (I-C3, I-W4).
func (w RecurrenceWriter) readRecurringItem(
	ctx context.Context, itemID shared.ID,
) (domain.WorkItem, error) {
	item, err := findItem(ctx, w.Items, itemID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	collection, err := findCollection(ctx, w.Containers, item.CollectionID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := collection.EnsureAcceptsItems(); err != nil {
		return domain.WorkItem{}, err
	}
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := item.EnsureRecurring(profile); err != nil {
		return domain.WorkItem{}, err
	}
	return item, nil
}

// findRule reads the entry's series, and reports "none" as a fact rather than as an error: an
// entry without one is the ordinary case, and both writers branch on it.
func (w RecurrenceWriter) findRule(
	ctx context.Context, itemID shared.ID,
) (domain.RecurrenceRule, bool, error) {
	rule, err := w.Recurrences.FindForItem(ctx, itemID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		return domain.RecurrenceRule{}, false, nil
	case err != nil:
		return domain.RecurrenceRule{}, false, err
	}
	return rule, true, nil
}

// recordWhole writes what an offline client has to be told about a series that arrived: one entry
// carrying the whole rule, which is how an entity that is created whole travels
// (offline-sync.md §4.2).
func (w RecurrenceWriter) recordWhole(
	ctx context.Context, actor appshared.ActorContext, rule domain.RecurrenceRule,
	item domain.WorkItem,
) error {
	return w.Changes.Record(ctx, changelog.Change{
		TenantID:    rule.TenantID,
		Entity:      recurrenceEntity,
		EntityID:    rule.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     recurrenceOutput(rule),
	})
}

// recordFields writes one entry per field that moved, each with its own HLC - the scalar rule of
// offline-sync.md §4.2, so two devices changing the rule and the horizon converge to both.
func (w RecurrenceWriter) recordFields(
	ctx context.Context, actor appshared.ActorContext, rule domain.RecurrenceRule,
	item domain.WorkItem, changes []domain.FieldChange,
) error {
	for _, change := range changes {
		err := w.Changes.Record(ctx, changelog.Change{
			TenantID:    rule.TenantID,
			Entity:      recurrenceEntity,
			EntityID:    rule.ID,
			Op:          changelog.Upsert,
			ContainerID: item.CollectionID,
			ActorID:     actor.AccountID,
			HLC:         w.HLC.Next(),
			Payload:     map[string]any{change.Field: change.To},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// recordAudit writes the evidence, values included: a series is a schedule rather than user
// content - no title, no notes travel here (rule 10, audit.md §4) - and "what did this entry
// repeat by, before somebody changed it" is not answerable without them.
func (w RecurrenceWriter) recordAudit(
	ctx context.Context, actor appshared.ActorContext, rule domain.RecurrenceRule,
	action audit.Action, changes []audit.Change, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   rule.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: recurrenceTarget,
		TargetID:   rule.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// recordActivity writes the step of the entry's own history. The series verbs are the entry's,
// because that is where a person reads them: "this repeats now" is something that happened to the
// entry (domain-model.md §3.5).
func (w RecurrenceWriter) recordActivity(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	verb activity.Verb, now time.Time,
) error {
	return w.Activity.record(ctx, actor, item, verb, activity.ChangeSet(activity.Compact), now)
}

// wholeRecurrenceAudit records the series as a whole, which is what a set and a removal are about.
func wholeRecurrenceAudit(rule domain.RecurrenceRule) []audit.Change {
	return []audit.Change{
		{Field: "item_id", Classification: audit.Open, To: rule.ItemID.String()},
		{Field: domain.FieldRRULE, Classification: audit.Open, To: rule.RRULE},
		{Field: domain.FieldTimeZone, Classification: audit.Open, To: rule.TimeZone},
		{Field: domain.FieldMode, Classification: audit.Open, To: rule.Mode.String()},
		{
			Field: domain.FieldHorizonDays, Classification: audit.Open,
			To: strconv.Itoa(rule.HorizonDays),
		},
	}
}

// recurrenceFieldAudit records both sides of every field that moved.
func recurrenceFieldAudit(changes []domain.FieldChange) []audit.Change {
	recorded := make([]audit.Change, 0, len(changes))
	for _, change := range changes {
		recorded = append(recorded, audit.Change{
			Field: change.Field, Classification: audit.Open,
			From: change.From, To: change.To,
		})
	}
	return recorded
}

// ensureRecurrenceVersion holds a no-op change to the version the caller read.
func ensureRecurrenceVersion(rule domain.RecurrenceRule, expected int) error {
	if expected == 0 || expected == rule.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("recurrence.version_conflict").
		WithParams(map[string]string{
			"recurrence_rule_id": rule.ID.String(), "expected_version": strconv.Itoa(expected),
		})
}

// recurrenceOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Recurrence).
func recurrenceOutput(rule domain.RecurrenceRule) usecase.Output {
	out := usecase.Output{
		"id":                   rule.ID.String(),
		"item_id":              rule.ItemID.String(),
		"rrule":                rule.RRULE,
		"time_zone":            rule.TimeZone,
		"mode":                 rule.Mode.String(),
		"horizon_days":         rule.HorizonDays,
		"ends_at":              timeOrNil(rule.EndsAt),
		"max_count":            nil,
		"last_materialized_at": timeOrNil(rule.LastMaterializedAt),
		"created_at":           rule.CreatedAt,
		"updated_at":           timeOrNil(rule.UpdatedAt),
		"version":              rule.Version,
	}
	if rule.MaxCount > 0 {
		out["max_count"] = rule.MaxCount
	}
	return out
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h SetRecurrence) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetRecurrenceName,
		Summary: "Puts a series on an entry, or changes the one it has - one call for both, " +
			"because a series is one document. Only a TASK may carry one: a series applies to the " +
			"whole subtree. The rule is RFC 5545 without a DTSTART and without an end of its own; " +
			"the entry's due date is where it starts and ends_at or max_count is where it stops. " +
			"A rule that cannot be read, a zone that is not an IANA name, and a rule that would " +
			"produce more occurrences inside its own horizon than the bound allows are each " +
			"refused by name. Nothing is materialised here.",
		SideEffects: "Writes the series, points the entry at it, records the change for offline " +
			"clients, writes an audit entry and a step of the entry's history.",
		TokenScope: recurrenceWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry that repeats.",
			},
			{
				Name: "rrule", Kind: usecase.KindString, Required: true,
				Description: "An RFC 5545 rule, such as FREQ=WEEKLY;BYDAY=MO.",
			},
			{
				Name: "time_zone", Kind: usecase.KindString, Required: true,
				Description: "The IANA zone the rule is read in, such as Europe/Berlin.",
			},
			{
				Name: "mode", Kind: usecase.KindString, Required: true,
				Description: "ON_SCHEDULE for a series that does not wait, ON_COMPLETION for one " +
					"whose next occurrence follows the last completion.",
			},
			{
				Name: "horizon_days", Kind: usecase.KindInt,
				Description: "How far ahead occurrences are kept, in days. Omitted means 90.",
			},
			{
				Name: "ends_at", Kind: usecase.KindString,
				Description: "When the series stops, RFC 3339. At most one of ends_at and max_count.",
			},
			{
				Name: "max_count", Kind: usecase.KindInt,
				Description: "How many occurrences the series produces at most.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read (If-Match). Omitted accepts what is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RecurrenceSetAction, TargetType: recurrenceTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemRecurrenceSet},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h SetRecurrence) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	var endsAt *time.Time
	if raw := in.String("ends_at"); raw != "" {
		// The catalogue's vocabulary has no timestamp kind, so every channel hands an instant over
		// as a string and an unparseable one is refused with the field a client sent - the same
		// shape the due date's parse has.
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, shared.ErrValidation.
				WithDetail("recurrence.ends_at_malformed").
				WithParams(map[string]string{"value": raw}).
				WithFields(shared.FieldError{Path: "/ends_at", Code: "recurrence.ends_at_malformed"})
		}
		endsAt = &at
	}

	rule, err := h.Execute(ctx, actor, SetRecurrenceCommand{
		ItemID: itemID,
		Spec: domain.RecurrenceSpec{
			RRULE:       in.String("rrule"),
			TimeZone:    in.String("time_zone"),
			Mode:        in.String("mode"),
			HorizonDays: in.Int("horizon_days"),
			EndsAt:      endsAt,
			MaxCount:    in.Int("max_count"),
		},
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return recurrenceOutput(rule), nil
}

// Descriptor is the catalogue entry.
func (h RemoveRecurrence) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveRecurrenceName,
		Summary: "Takes the series off an entry. Occurrences that were already materialised stay " +
			"exactly where they are - they are ordinary entries the moment they exist. " +
			"Idempotent: an entry with no series succeeds and announces nothing.",
		SideEffects: "Deletes the series, clears the entry's pointer, records the deletion for " +
			"offline clients, writes an audit entry and a step of the entry's history.",
		TokenScope: recurrenceWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry that should stop repeating.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read (If-Match). Omitted accepts what is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RecurrenceRemovedAction, TargetType: recurrenceTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemRecurrenceRemoved},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveRecurrence) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, RemoveRecurrenceCommand{
		ItemID: itemID, ExpectedVersion: in.Int("expected_version"),
	}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// GetRecurrence reads an entry's series.
//
// The right to read the entry is the right to read what it repeats by, for the reason the history
// draws the same line: a separate right would be one more thing to get wrong for no protection
// gained (domain-model.md §3.2).
type GetRecurrence struct {
	Recurrences repository.Recurrences
	Items       repository.Items
	Containers  repository.Containers
	Authorizer  Authorizer
	UnitOfWork  persistence.UnitOfWork
}

// Execute returns the entry's series, or reports that it has none.
func (h GetRecurrence) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.RecurrenceRule, error) {
	if itemID.IsZero() {
		return domain.RecurrenceRule{}, itemIDRequired()
	}

	subject, collection, err := readItemScope(
		ctx, h.UnitOfWork, h.Items, h.Containers, actor, itemID)
	if err != nil {
		return domain.RecurrenceRule{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     RecurrenceReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
		TargetID:   itemID,
		On:         reading(subject),
	}); err != nil {
		return domain.RecurrenceRule{}, err
	}

	var rule domain.RecurrenceRule
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rule, err = h.Recurrences.FindForItem(ctx, itemID)
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.WithDetail("recurrence.not_found")
		}
		return err
	})
	if err != nil {
		return domain.RecurrenceRule{}, err
	}
	return rule, nil
}

// Descriptor is the catalogue entry.
func (h GetRecurrence) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetRecurrenceName,
		Summary: "Reads an entry's series, or answers that it has none. Readable by whoever may " +
			"read the entry.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose series is wanted.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RecurrenceReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetRecurrence) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	rule, err := h.Execute(ctx, actor, itemID)
	if err != nil {
		return nil, err
	}
	return recurrenceOutput(rule), nil
}

// SkipOccurrenceName is the catalogue's name for the one part of the materialisation a person asks
// for.
const SkipOccurrenceName = "SkipOccurrence"

// RecurrenceSkippedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
// matches on it (audit.md §2).
const RecurrenceSkippedAction audit.Action = "recurrence.occurrence_skipped"

// SkipOccurrence moves a series past its next unmade occurrence (D-05).
//
// The one user-facing half of the materialisation, and the shape of it is what the bookkeeping
// already is: the watermark says how far the series has been dealt with, so skipping is moving it
// past one moment without creating anything. Nothing that already exists is touched - a
// materialised occurrence is an ordinary entry, and skipping it would be deleting somebody's work.
type SkipOccurrence struct {
	Writer RecurrenceWriter
}

// SkipOccurrenceCommand is the input, typed.
type SkipOccurrenceCommand struct {
	ItemID shared.ID
}

// Execute skips the next occurrence and answers the rule.
func (h SkipOccurrence) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SkipOccurrenceCommand,
) (domain.RecurrenceRule, error) {
	w := h.Writer
	if cmd.ItemID.IsZero() {
		return domain.RecurrenceRule{}, itemIDRequired()
	}

	subject, collection, err := readItemScope(
		ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return domain.RecurrenceRule{}, err
	}

	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     RecurrenceSkippedAction,
		TokenScope: recurrenceWrite,
		TargetType: recurrenceTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.RecurrenceRule{}, err
	}

	var skipped domain.RecurrenceRule
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := w.readRecurringItem(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		rule, found, err := w.findRule(ctx, item.ID)
		if err != nil {
			return err
		}
		if !found {
			return shared.ErrNotFound.WithDetail("recurrence.not_found")
		}
		if item.Due == nil {
			// The series counts from the entry's date, and without one there is no next occurrence
			// to skip. The same refusal the write gives, at the moment it matters.
			return shared.ErrValidation.
				WithDetail("recurrence.due_date_required").
				WithFields(shared.FieldError{Path: "/item_id", Code: "recurrence.due_date_required"})
		}

		moment, err := w.nextUnmade(rule, item)
		if err != nil {
			return err
		}
		if moment == nil {
			// A series with nothing left to produce. Skipping the occurrence that is not coming is
			// the state the caller asked for, so nothing is written and nothing is announced.
			skipped = rule
			return nil
		}

		moved, err := w.Recurrences.Advance(ctx, rule, *moment)
		if err != nil {
			return err
		}
		if !moved {
			// The materialisation moved the bookkeeping between the read and here, which means the
			// occurrence being skipped may already exist. Answered as a conflict rather than
			// applied to whatever is there now: "skip the next one" is about a specific next one.
			return shared.ErrConflict.
				WithDetail("recurrence.materialization_raced").
				WithParams(map[string]string{"recurrence_rule_id": rule.ID.String()})
		}
		rule.LastMaterializedAt = moment

		if err := w.recordFields(ctx, actor, rule, item, []domain.FieldChange{{
			Field: "last_materialized_at", To: moment.UTC().Format(time.RFC3339Nano),
		}}); err != nil {
			return err
		}
		if err := w.recordAudit(ctx, actor, rule, RecurrenceSkippedAction, []audit.Change{
			{Field: "item_id", Classification: audit.Open, To: rule.ItemID.String()},
			{
				Field: "skipped_occurrence_at", Classification: audit.Open,
				To: moment.UTC().Format(time.RFC3339Nano),
			},
		}, now); err != nil {
			return err
		}
		if err := w.recordActivity(ctx, actor, item, activity.ItemRecurrenceSkipped, now); err != nil {
			return err
		}

		skipped = rule
		return nil
	})
	if err != nil {
		return domain.RecurrenceRule{}, err
	}
	return skipped, nil
}

// nextUnmade is the moment a skip moves past: the first the rule produces after everything already
// dealt with - the watermark where there is one, the entry's own date where there is not, since
// that date is the series' first occurrence and the entry itself is it.
func (w RecurrenceWriter) nextUnmade(
	rule domain.RecurrenceRule, item domain.WorkItem,
) (*time.Time, error) {
	anchor, err := item.Due.Anchor()
	if err != nil {
		return nil, err
	}
	after := anchor
	if rule.LastMaterializedAt != nil && rule.LastMaterializedAt.After(after) {
		after = *rule.LastMaterializedAt
	}

	moments, err := w.Expander.Occurrences(recurrenceport.Rule{
		RRULE:    rule.RRULE,
		TimeZone: rule.TimeZone,
		Start:    anchor,
		Until:    endsAt(rule),
		Count:    rule.MaxCount,
		// The window is the series' own horizon rather than a fixed one: a monthly series skipped
		// in January is skipping February, and a horizon of days would answer nothing at all.
	}, after, after.AddDate(0, 0, 2*rule.HorizonDays), 2)
	if err != nil {
		return nil, err
	}

	for _, moment := range moments {
		if moment.After(after) {
			next := moment
			return &next, nil
		}
	}
	return nil, nil
}

// Descriptor is the catalogue entry.
func (h SkipOccurrence) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SkipOccurrenceName,
		Summary: "Skips the next occurrence a series has not produced yet, leaving everything that " +
			"already exists alone - a materialised occurrence is an ordinary entry, and skipping " +
			"it would be deleting somebody's work. Called twice it skips two, which is what " +
			"'skip the next one' means said twice; the Idempotency-Key is what makes a retry safe.",
		SideEffects: "Moves the series' bookkeeping past one occurrence, records the change for " +
			"offline clients, writes an audit entry and a step of the entry's history.",
		TokenScope: recurrenceWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose series should skip its next occurrence.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RecurrenceSkippedAction, TargetType: recurrenceTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemRecurrenceSkipped},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h SkipOccurrence) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	rule, err := h.Execute(ctx, actor, SkipOccurrenceCommand{ItemID: itemID})
	if err != nil {
		return nil, err
	}
	return recurrenceOutput(rule), nil
}
