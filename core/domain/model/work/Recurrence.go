// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RecurrenceRule is a series: what repeats, in which zone it is read, how the next occurrence
// arrives, and how far ahead the occurrences are kept (domain-model.md §3.5, arc42 §6.3).
//
// Its own entity beside the entry rather than columns on it, because it is one thing an entry
// either carries or does not, and because the occurrences it produces point back at it. The entry
// it belongs to is the one the occurrences are copied from - the series' own template.
//
// What is not decided here is what the rule *means*: expanding it is a library's job behind a port
// (ADR-0008, core/port/recurrence), because "RRULE plus time zones plus DST is error-prone" is a
// risk this project answers with a proven implementation rather than with its own (R-07). What is
// decided here is everything that can be decided without one.
type RecurrenceRule struct {
	ID       shared.ID
	TenantID shared.ID
	// ItemID is the entry the series sits on: the source the occurrences are copied from.
	ItemID shared.ID
	// RRULE is the rule text as it was given, without a DTSTART and without an end.
	RRULE string
	// TimeZone is the IANA name the rule is read in, required. DST is resolved through it and
	// never through an offset (i18n-l10n.md §4).
	TimeZone string
	Mode     RecurrenceMode
	// HorizonDays is how far ahead occurrences are materialised. A rolling window rather than the
	// whole series: a rule with no end would otherwise be an infinite list of entries.
	HorizonDays int
	// EndsAt and MaxCount are the two spellings of the end, and at most one of them is set.
	EndsAt   *time.Time
	MaxCount int
	// LastMaterializedAt is how far the materialisation has come (D-05). The server's bookkeeping:
	// nothing a client sends ever reaches it.
	LastMaterializedAt *time.Time
	CreatedAt          time.Time
	UpdatedAt          *time.Time
	Version            int
}

// RecurrenceMode is how the next occurrence arrives (arc42 §6.3).
type RecurrenceMode string

const (
	// RecurrenceOnSchedule puts the next occurrence in place at its time, whether or not the last
	// one was done. A rent payment does not wait for the previous one to be ticked off.
	RecurrenceOnSchedule RecurrenceMode = "ON_SCHEDULE"
	// RecurrenceOnCompletion creates the next occurrence only once its predecessor is completed:
	// "again, two weeks after I last did it" rather than "again on the first of every month".
	RecurrenceOnCompletion RecurrenceMode = "ON_COMPLETION"
)

// RecurrenceModes is the closed set, in the order the schema's check constraint lists them.
func RecurrenceModes() []RecurrenceMode {
	return []RecurrenceMode{RecurrenceOnSchedule, RecurrenceOnCompletion}
}

// Valid reports whether a mode is one this system knows.
func (m RecurrenceMode) Valid() bool {
	for _, known := range RecurrenceModes() {
		if m == known {
			return true
		}
	}
	return false
}

func (m RecurrenceMode) String() string { return string(m) }

// The rule's own field names, as the contract spells them. They travel into the change log, where
// a client matches them against the members it sent (offline-sync.md §4.2).
const (
	FieldRRULE       = "rrule"
	FieldTimeZone    = "time_zone"
	FieldMode        = "mode"
	FieldHorizonDays = "horizon_days"
	FieldEndsAt      = "ends_at"
	FieldMaxCount    = "max_count"
)

const (
	// MaxRRULELength is the contract's maxLength in code points, so that a refusal comes from the
	// same number the specification declares.
	MaxRRULELength = 1024
	// DefaultHorizonDays is the column's own default (0001_init) and arc42 §6.3's window.
	DefaultHorizonDays = 90
	// MinHorizonDays and MaxHorizonDays bound the window. A year is the longest that still reads
	// as a rolling window rather than as "materialise the series"; a day is the shortest that can
	// hold a daily series at all.
	MinHorizonDays = 1
	MaxHorizonDays = 365
	// MaxOccurrencesPerHorizon is the bound T-17 asks for, checked at the write by expanding the
	// rule inside its own horizon: FREQ=SECONDLY is a denial of service wearing a calendar, and
	// the place to refuse it is where somebody wrote it rather than where the scheduler meets it.
	MaxOccurrencesPerHorizon = 500
)

// NewRecurrenceRuleInput is what a series needs decided.
type NewRecurrenceRuleInput struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	Spec     RecurrenceSpec
	// Due is the entry's due date, already read. A series counts from it, so an entry without one
	// cannot carry a rule.
	Due *DueDate
	Now time.Time
}

// RecurrenceSpec is what a caller says about a series - the whole document, because the route is a
// PUT and a series is one thing.
type RecurrenceSpec struct {
	RRULE    string
	TimeZone string
	Mode     string
	// HorizonDays is zero for "the default", which is what an omitted member means.
	HorizonDays int
	EndsAt      *time.Time
	MaxCount    int
}

// NewRecurrenceRule validates and builds a series, or says why it is not one.
//
// Everything here is decidable without expanding anything. Whether the text is a rule at all, and
// whether it is a *dense* one, needs the library and is asked by the application through the port -
// which is also the only order that works: a text that is not a rule cannot be measured.
func NewRecurrenceRule(input NewRecurrenceRuleInput) (RecurrenceRule, error) {
	if input.Due == nil {
		// A series counts from the entry's due date (arc42 §6.3: the rule sits on the template
		// item, and DTSTART is that item's date). Without one there is nothing to count from, and
		// a rule stored dormant would be a series nobody could predict.
		return RecurrenceRule{}, shared.ErrValidation.
			WithDetail("recurrence.due_date_required").
			WithFields(shared.FieldError{Path: "/rrule", Code: "recurrence.due_date_required"})
	}

	spec, err := validRecurrenceSpec(input.Spec, *input.Due)
	if err != nil {
		return RecurrenceRule{}, err
	}

	return RecurrenceRule{
		ID:          input.ID,
		TenantID:    input.TenantID,
		ItemID:      input.ItemID,
		RRULE:       spec.RRULE,
		TimeZone:    spec.TimeZone,
		Mode:        RecurrenceMode(spec.Mode),
		HorizonDays: spec.HorizonDays,
		EndsAt:      spec.EndsAt,
		MaxCount:    spec.MaxCount,
		CreatedAt:   input.Now,
		Version:     1,
	}, nil
}

// Changed applies a new document to an existing series and reports which fields moved.
//
// A PUT rather than a patch, because a series is one statement: "every Monday in Berlin, on
// completion, ninety days ahead" is not five independent settings, and a caller sending half of it
// means the half it sent. The changes are still reported per field, because that is what the merge
// needs (offline-sync.md §4.2).
//
// Idempotent like every other writer here: a document that says what is already stored reports no
// changes, so the caller writes nothing, spends no version and announces nothing.
func (r RecurrenceRule) Changed(
	spec RecurrenceSpec, due *DueDate, at time.Time,
) (RecurrenceRule, []FieldChange, error) {
	if due == nil {
		return RecurrenceRule{}, nil, shared.ErrValidation.
			WithDetail("recurrence.due_date_required").
			WithFields(shared.FieldError{Path: "/rrule", Code: "recurrence.due_date_required"})
	}

	valid, err := validRecurrenceSpec(spec, *due)
	if err != nil {
		return RecurrenceRule{}, nil, err
	}

	target := r
	target.RRULE = valid.RRULE
	target.TimeZone = valid.TimeZone
	target.Mode = RecurrenceMode(valid.Mode)
	target.HorizonDays = valid.HorizonDays
	target.EndsAt = valid.EndsAt
	target.MaxCount = valid.MaxCount

	changes := recurrenceChanges(r, target)
	if len(changes) == 0 {
		return r, nil, nil
	}
	target.UpdatedAt = &at
	return target, changes, nil
}

// validRecurrenceSpec is the whole of what can be decided without a library.
func validRecurrenceSpec(spec RecurrenceSpec, due DueDate) (RecurrenceSpec, error) {
	rule := strings.TrimSpace(spec.RRULE)
	switch {
	case rule == "":
		return spec, recurrenceInvalid("recurrence.rrule_required", "/rrule", nil)
	case len([]rune(rule)) > MaxRRULELength:
		return spec, recurrenceInvalid("recurrence.rrule_too_long", "/rrule",
			map[string]string{"maximum": strconv.Itoa(MaxRRULELength)})
	}
	if err := ensureRuleCarriesOnlyTheRule(rule); err != nil {
		return spec, err
	}

	zone := strings.TrimSpace(spec.TimeZone)
	if zone == "" {
		return spec, recurrenceInvalid("recurrence.time_zone_required", "/time_zone", nil)
	}
	if _, err := time.LoadLocation(zone); err != nil {
		// The same check a due date's zone gets, and for the same reason: a name the time package
		// cannot load is a name no expansion can use, and never a fixed offset - an offset cannot
		// represent daylight saving, which is the one thing this rule exists to survive.
		return spec, recurrenceInvalid("recurrence.time_zone_invalid", "/time_zone",
			map[string]string{"value": zone})
	}

	mode := RecurrenceMode(strings.TrimSpace(spec.Mode))
	if !mode.Valid() {
		return spec, recurrenceInvalid("recurrence.mode_unknown", "/mode",
			map[string]string{"value": string(mode)})
	}

	horizon := spec.HorizonDays
	if horizon == 0 {
		horizon = DefaultHorizonDays
	}
	if horizon < MinHorizonDays || horizon > MaxHorizonDays {
		return spec, recurrenceInvalid("recurrence.horizon_out_of_range", "/horizon_days",
			map[string]string{
				"minimum": strconv.Itoa(MinHorizonDays), "maximum": strconv.Itoa(MaxHorizonDays),
			})
	}

	if spec.EndsAt != nil && spec.MaxCount > 0 {
		// Two ends are two different series. Refused rather than resolved by precedence, because
		// whichever one this code picked would be the one the caller did not mean half the time.
		return spec, recurrenceInvalid("recurrence.end_spec_ambiguous", "/ends_at", nil)
	}
	if spec.MaxCount < 0 {
		return spec, recurrenceInvalid("recurrence.max_count_invalid", "/max_count", nil)
	}
	if spec.EndsAt != nil && !spec.EndsAt.After(due.At) {
		// A series that ends before it starts produces nothing, which is not what anybody means by
		// setting one.
		return spec, recurrenceInvalid("recurrence.end_before_start", "/ends_at", nil)
	}

	spec.RRULE = rule
	spec.TimeZone = zone
	spec.Mode = string(mode)
	spec.HorizonDays = horizon
	if spec.EndsAt != nil {
		ends := spec.EndsAt.UTC()
		spec.EndsAt = &ends
	}
	return spec, nil
}

// ensureRuleCarriesOnlyTheRule refuses a rule text that says what something else already says.
//
// DTSTART is the entry's due date and the end is the end spec beside it. A rule carrying either
// would be a second answer to a question already answered - and the one this system would use is
// not the one a client would expect, since the stored fields are what the expansion is given.
// Refused by name rather than ignored: silently dropping half of what somebody sent is how a
// series comes to repeat forever.
func ensureRuleCarriesOnlyTheRule(rule string) error {
	upper := strings.ToUpper(rule)
	if strings.Contains(upper, "DTSTART") {
		return recurrenceInvalid("recurrence.rrule_carries_start", "/rrule", nil)
	}
	if strings.Contains(upper, "UNTIL=") || strings.Contains(upper, "COUNT=") {
		return recurrenceInvalid("recurrence.rrule_carries_end", "/rrule", nil)
	}
	if strings.ContainsAny(rule, "\n\r") {
		// One line, one rule. A multi-line body is an RRULE set - EXDATE, RDATE, several rules -
		// and this milestone stores one series per entry (domain-model.md §3.5).
		return recurrenceInvalid("recurrence.rrule_not_single", "/rrule", nil)
	}
	return nil
}

// recurrenceInvalid is the one refusal shape, so every way of getting a series wrong answers with
// a stable code and the field path a client shows it beside.
func recurrenceInvalid(code, path string, params map[string]string) error {
	refusal := shared.ErrValidation.WithDetail(code)
	if params != nil {
		refusal = refusal.WithParams(params)
	}
	return refusal.WithFields(shared.FieldError{Path: path, Code: code, Params: params})
}

// recurrenceChanges diffs two rules field by field, which is what the merge needs: two devices
// changing the rule and the horizon converge to both (offline-sync.md §4.2).
func recurrenceChanges(from, to RecurrenceRule) []FieldChange {
	var changes []FieldChange
	appendChange := func(field, before, after string) {
		if before != after {
			changes = append(changes, FieldChange{Field: field, From: before, To: after})
		}
	}
	appendChange(FieldRRULE, from.RRULE, to.RRULE)
	appendChange(FieldTimeZone, from.TimeZone, to.TimeZone)
	appendChange(FieldMode, from.Mode.String(), to.Mode.String())
	appendChange(FieldHorizonDays, strconv.Itoa(from.HorizonDays), strconv.Itoa(to.HorizonDays))
	appendChange(FieldEndsAt, instantValue(from.EndsAt), instantValue(to.EndsAt))
	appendChange(FieldMaxCount, countValue(from.MaxCount), countValue(to.MaxCount))
	return changes
}

// countValue spells an optional count the way a change carries it, and the empty string for none -
// an omitted entry would read as "not touched" (offline-sync.md §4.2).
func countValue(count int) string {
	if count <= 0 {
		return ""
	}
	return strconv.Itoa(count)
}

// Horizon is how far ahead this rule owes occurrences, counted from a moment.
func (r RecurrenceRule) Horizon(from time.Time) time.Time {
	return from.AddDate(0, 0, r.HorizonDays)
}

// EnsureRecurring refuses what cannot carry a series: a type whose profile does not carry
// RECURRENCE - which is everything but a task, because "a series applies to the whole subtree"
// (domain-model.md §2) - and a trashed or archived entry, in that order for the reason
// EnsureCommentable asks the capability first.
func (i WorkItem) EnsureRecurring(profile CapabilityProfile) error {
	if err := profile.Require(CapabilityRecurrence, "/item_id"); err != nil {
		return err
	}
	return i.EnsureEditable()
}
