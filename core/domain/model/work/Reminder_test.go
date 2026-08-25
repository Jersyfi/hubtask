// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var remindedAt = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

// dueAt is the entry's due date as a plain instant, which is what most of these tests count from.
func dueAt(value string) *work.DueDate {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	due, err := work.NewDueDate(&parsed, false, "")
	if err != nil {
		panic(err)
	}
	return due
}

func draftReminder(spec string, due *work.DueDate) work.NewReminderInput {
	return work.NewReminderInput{
		ID: "r1", TenantID: "t1", ItemID: "i1", OffsetSpec: spec, Due: due, Now: remindedAt,
	}
}

// The grammar, in full. Every way of getting it wrong answers with its own code and the same field
// path, because a client shows the message beside the field somebody typed in.
func TestAnOffsetSpecIsOneOfTwoFormsOrNothing(t *testing.T) {
	for name, test := range map[string]struct {
		spec string

		wantRelative bool
		wantDuration time.Duration
		wantInstant  string
		wantCode     string
	}{
		"an hour before": {
			spec: "REL:-PT1H", wantRelative: true, wantDuration: -time.Hour,
		},
		"a day and a half before": {
			spec: "REL:-P1DT12H", wantRelative: true, wantDuration: -36 * time.Hour,
		},
		"two weeks before": {
			spec: "REL:-P2W", wantRelative: true, wantDuration: -14 * 24 * time.Hour,
		},
		"ten minutes after, unsigned": {
			spec: "REL:PT10M", wantRelative: true, wantDuration: 10 * time.Minute,
		},
		"the moment itself": {
			spec: "REL:PT0S", wantRelative: true,
		},
		"an instant in UTC": {
			spec: "ABS:2026-09-01T08:00:00Z", wantInstant: "2026-09-01T08:00:00Z",
		},
		"an instant with an offset, stored in UTC": {
			spec: "ABS:2026-09-01T10:00:00+02:00", wantInstant: "2026-09-01T08:00:00Z",
		},
		"nothing at all": {
			spec: "   ", wantCode: "reminders.offset_spec_required",
		},
		"a duration without its form": {
			spec: "-PT1H", wantCode: "reminders.offset_spec_invalid",
		},
		"a form without its value": {
			spec: "REL:", wantCode: "reminders.offset_spec_invalid",
		},
		"a number without its unit": {
			spec: "REL:-PT30", wantCode: "reminders.offset_spec_invalid",
		},
		"a unit without its number": {
			spec: "REL:-PTH", wantCode: "reminders.offset_spec_invalid",
		},
		"minutes written as months": {
			spec: "REL:-P30M", wantCode: "reminders.offset_calendar_unit",
		},
		"a year before": {
			spec: "REL:-P1Y", wantCode: "reminders.offset_calendar_unit",
		},
		"longer than any reminder means": {
			spec: "REL:-P4000D", wantCode: "reminders.offset_out_of_range",
		},
		"more digits than any duration has": {
			spec: "REL:-PT1234567H", wantCode: "reminders.offset_out_of_range",
		},
		"an instant that is not one": {
			spec: "ABS:tomorrow morning", wantCode: "reminders.offset_spec_invalid",
		},
		"longer than the contract declares": {
			spec:     "ABS:2026-09-01T08:00:00Z" + string(make([]byte, 64)),
			wantCode: "reminders.offset_spec_too_long",
		},
	} {
		t.Run(name, func(t *testing.T) {
			offset, err := work.ParseReminderOffset(test.spec)

			if test.wantCode != "" {
				refusal := shared.AsError(err)
				if refusal == nil || refusal.DetailCode != test.wantCode {
					t.Fatalf("refused as %v, want %s", err, test.wantCode)
				}
				if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/offset_spec" {
					t.Errorf("the refusal does not point at the offset: %v", refusal.Fields)
				}
				return
			}

			if err != nil {
				t.Fatalf("the offset was refused: %v", err)
			}
			if offset.Relative != test.wantRelative {
				t.Errorf("relative is %v rather than %v", offset.Relative, test.wantRelative)
			}
			if offset.Duration != test.wantDuration {
				t.Errorf("the duration is %v rather than %v", offset.Duration, test.wantDuration)
			}
			if test.wantInstant != "" && offset.Instant.Format(time.RFC3339) != test.wantInstant {
				t.Errorf("the instant is %v rather than %s", offset.Instant, test.wantInstant)
			}
			// The text is stored as it was given: two spellings of one hour are two strings, and
			// answering a client with words it did not choose is not this server's business.
			if offset.Spec != test.spec {
				t.Errorf("the stored spec is %q rather than %q", offset.Spec, test.spec)
			}
		})
	}
}

// The backlog's rule, both halves of it: a relative reminder follows its due date, an absolute one
// does not, and both are decided against a fixed clock so the test says what it means.
func TestARelativeReminderFollowsTheDueDateAndAnAbsoluteOneDoesNot(t *testing.T) {
	due := dueAt("2026-09-01T17:00:00Z")
	moved := dueAt("2026-09-04T17:00:00Z")

	relative, err := work.NewReminder(draftReminder("REL:-PT1H", due))
	if err != nil {
		t.Fatalf("the relative reminder was refused: %v", err)
	}
	if got := relative.FireAt.Format(time.RFC3339); got != "2026-09-01T16:00:00Z" {
		t.Fatalf("it fires at %s rather than an hour before the date", got)
	}

	absolute, err := work.NewReminder(draftReminder("ABS:2026-09-01T08:00:00Z", due))
	if err != nil {
		t.Fatalf("the absolute reminder was refused: %v", err)
	}

	rescheduled, movedWithIt, err := relative.Rescheduled(moved)
	if err != nil {
		t.Fatalf("rescheduling failed: %v", err)
	}
	if !movedWithIt {
		t.Error("the relative reminder did not follow the due date")
	}
	if got := rescheduled.FireAt.Format(time.RFC3339); got != "2026-09-04T16:00:00Z" {
		t.Errorf("it fires at %s rather than an hour before the new date", got)
	}

	stayed, changed, err := absolute.Rescheduled(moved)
	if err != nil {
		t.Fatalf("rescheduling failed: %v", err)
	}
	if changed {
		t.Error("the absolute reminder followed a due date it does not count from")
	}
	if got := stayed.FireAt.Format(time.RFC3339); got != "2026-09-01T08:00:00Z" {
		t.Errorf("the absolute reminder moved to %s", got)
	}
}

// An all-day due date is a date in a place, and a reminder counts from the start of that day
// there. The two Berlin dates are on either side of the summer transition, which is what makes the
// difference visible: the same "an hour before" is a different instant in UTC, and midnight UTC
// would have been the wrong moment in both.
func TestAnAllDayDueDateIsCountedFromTheStartOfItsDayInItsOwnZone(t *testing.T) {
	for name, test := range map[string]struct {
		date string
		zone string
		want string
	}{
		"Berlin in winter": {
			date: "2026-02-10T00:00:00+01:00", zone: "Europe/Berlin",
			want: "2026-02-09T22:00:00Z",
		},
		"Berlin in summer": {
			date: "2026-08-10T00:00:00+02:00", zone: "Europe/Berlin",
			want: "2026-08-09T21:00:00Z",
		},
		"the same date, a hemisphere away": {
			date: "2026-08-10T00:00:00-03:00", zone: "America/Sao_Paulo",
			want: "2026-08-10T02:00:00Z",
		},
	} {
		t.Run(name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, test.date)
			if err != nil {
				t.Fatalf("the date does not parse: %v", err)
			}
			due, err := work.NewDueDate(&at, true, test.zone)
			if err != nil {
				t.Fatalf("the due date was refused: %v", err)
			}

			reminder, err := work.NewReminder(draftReminder("REL:-PT1H", due))
			if err != nil {
				t.Fatalf("the reminder was refused: %v", err)
			}
			if got := reminder.FireAt.UTC().Format(time.RFC3339); got != test.want {
				t.Errorf("it fires at %s rather than %s", got, test.want)
			}
		})
	}
}

// The refusal the backlog names, and its counterpart: an absolute reminder needs no due date at
// all, because it counts from nothing.
func TestARelativeReminderNeedsADueDateAndAnAbsoluteOneDoesNot(t *testing.T) {
	_, err := work.NewReminder(draftReminder("REL:-PT1H", nil))
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "reminders.due_date_required" {
		t.Fatalf("refused as %v, want reminders.due_date_required", err)
	}

	absolute, err := work.NewReminder(draftReminder("ABS:2026-09-01T08:00:00Z", nil))
	if err != nil {
		t.Fatalf("an absolute reminder was refused for want of a due date: %v", err)
	}
	if absolute.FireAt == nil {
		t.Error("the absolute reminder has no moment to fire at")
	}
}

// A due date that goes leaves the reminder standing with no moment: the date may come back, and
// nobody asked for the reminder to go.
func TestClearingTheDueDateLeavesARelativeReminderWithoutAMoment(t *testing.T) {
	reminder, err := work.NewReminder(draftReminder("REL:-PT1H", dueAt("2026-09-01T17:00:00Z")))
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}

	orphaned, changed, err := reminder.Rescheduled(nil)
	if err != nil {
		t.Fatalf("rescheduling failed: %v", err)
	}
	if !changed {
		t.Fatal("losing the due date changed nothing")
	}
	if orphaned.FireAt != nil {
		t.Errorf("it still fires at %v", orphaned.FireAt)
	}
	if orphaned.State != work.ReminderPending {
		t.Errorf("the state is %s rather than pending", orphaned.State)
	}
}

// What a channel list may be. The default is the column's own, an unknown channel is refused by
// name, and naming one twice is one channel rather than two sends.
func TestTheChannelListIsCheckedAndDeduplicated(t *testing.T) {
	draft := draftReminder("ABS:2026-09-01T08:00:00Z", nil)

	defaulted, err := work.NewReminder(draft)
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}
	if got := work.ChannelList(defaulted.Channels); got != "EMAIL" {
		t.Errorf("an omitted channel list became %q", got)
	}

	draft.Channels = []string{"EMAIL", "EMAIL"}
	deduplicated, err := work.NewReminder(draft)
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}
	if got := work.ChannelList(deduplicated.Channels); got != "EMAIL" {
		t.Errorf("naming email twice became %q", got)
	}

	draft.Channels = []string{"CARRIER_PIGEON"}
	_, err = work.NewReminder(draft)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "reminders.channel_unknown" {
		t.Fatalf("refused as %v, want reminders.channel_unknown", err)
	}
	if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/channels" {
		t.Errorf("the refusal does not point at the channels: %v", refusal.Fields)
	}
}

// What a recipient list may be. Empty stays empty - it is the list that means "the assignee and
// the members", resolved when the reminder fires - and the bounds hold.
func TestTheRecipientListIsBoundedAndDeduplicated(t *testing.T) {
	draft := draftReminder("ABS:2026-09-01T08:00:00Z", nil)

	empty, err := work.NewReminder(draft)
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}
	if len(empty.Recipients) != 0 {
		t.Errorf("an empty recipient list became %v", empty.Recipients)
	}

	draft.Recipients = []shared.ID{"a1", "a2", "a1"}
	deduplicated, err := work.NewReminder(draft)
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}
	if got := work.RecipientList(deduplicated.Recipients); got != "a1,a2" {
		t.Errorf("the recipients are %q rather than a1,a2", got)
	}

	draft.Recipients = make([]shared.ID, work.MaxReminderRecipients+1)
	for i := range draft.Recipients {
		draft.Recipients[i] = shared.ID(string(rune('a' + i%26)))
	}
	_, err = work.NewReminder(draft)
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "reminders.too_many_recipients" {
		t.Fatalf("refused as %v, want reminders.too_many_recipients", err)
	}
}

// The bound that makes the list unpaged. It is asked before the write, so the refusal names the
// entry rather than the reminder that would have been the twenty-sixth.
func TestAnEntryCarriesABoundedNumberOfReminders(t *testing.T) {
	if err := work.EnsureReminderCapacity(work.MaxRemindersPerItem - 1); err != nil {
		t.Fatalf("a reminder below the bound was refused: %v", err)
	}

	err := work.EnsureReminderCapacity(work.MaxRemindersPerItem)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "reminders.too_many" {
		t.Fatalf("refused as %v, want reminders.too_many", err)
	}
	if refusal.Params["maximum"] == "" {
		t.Error("the refusal does not say what the maximum is")
	}
}

// A patch reports one change per field that moved, which is what makes the merge per field: two
// devices editing the offset and the channels converge to both (offline-sync.md §4.2).
func TestAPatchReportsOneChangePerFieldThatMoved(t *testing.T) {
	due := dueAt("2026-09-01T17:00:00Z")
	reminder, err := work.NewReminder(draftReminder("REL:-PT1H", due))
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}

	spec := "REL:-PT2H"
	recipients := []shared.ID{"a1"}
	changed, changes, err := reminder.Patched(
		work.ReminderPatch{OffsetSpec: &spec, Recipients: &recipients}, due, remindedAt,
	)
	if err != nil {
		t.Fatalf("the patch was refused: %v", err)
	}

	moved := map[string][2]string{}
	for _, change := range changes {
		moved[change.Field] = [2]string{change.From, change.To}
	}
	if len(moved) != 2 {
		t.Fatalf("the changes are %v", changes)
	}
	if got := moved["offset_spec"]; got != [2]string{"REL:-PT1H", "REL:-PT2H"} {
		t.Errorf("the offset change is %v", got)
	}
	if got := moved["recipients"]; got != [2]string{"", "a1"} {
		t.Errorf("the recipient change is %v", got)
	}
	if got := changed.FireAt.Format(time.RFC3339); got != "2026-09-01T15:00:00Z" {
		t.Errorf("the moment is %s rather than two hours before the date", got)
	}
	if changed.UpdatedAt == nil {
		t.Error("the change carries no stamp")
	}

	// A patch that says what is already stored moves nothing, so the caller writes nothing, spends
	// no version and announces nothing.
	same := "REL:-PT2H"
	untouched, none, err := changed.Patched(work.ReminderPatch{OffsetSpec: &same}, due, remindedAt)
	if err != nil {
		t.Fatalf("the patch was refused: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("a patch changing nothing reported %v", none)
	}
	if untouched.Version != changed.Version {
		t.Error("a patch changing nothing spent a version")
	}
}

// A reminder that has fired is a record of something that happened. It is not given a new future.
func TestOnlyAPendingReminderIsChanged(t *testing.T) {
	reminder, err := work.NewReminder(draftReminder("ABS:2026-09-01T08:00:00Z", nil))
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}
	reminder.State = work.ReminderSent

	spec := "ABS:2026-09-02T08:00:00Z"
	_, _, err = reminder.Patched(work.ReminderPatch{OffsetSpec: &spec}, nil, remindedAt)
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "reminders.not_pending" {
		t.Fatalf("refused as %v, want reminders.not_pending", err)
	}

	sent, changed, err := reminder.Rescheduled(dueAt("2026-09-04T17:00:00Z"))
	if err != nil {
		t.Fatalf("rescheduling failed: %v", err)
	}
	if changed || !sent.FireAt.Equal(*reminder.FireAt) {
		t.Error("a sent reminder was rescheduled")
	}
}

// The capability first and the lifecycle second, and both refused: a type without REMINDER carries
// none whatever state it is in, and a trashed entry carries no new one (I-W4).
func TestOnlyARemindableTypeInAWritableStateTakesAReminder(t *testing.T) {
	item := work.WorkItem{ID: "i1"}
	remindable := work.CapabilityProfile{Capabilities: []work.Capability{work.CapabilityReminder}}

	if err := item.EnsureRemindable(remindable); err != nil {
		t.Fatalf("a remindable type was refused: %v", err)
	}

	unremindable := work.CapabilityProfile{Type: work.ItemWorkPackage}
	if got := shared.AsError(item.EnsureRemindable(unremindable)).DetailCode; got !=
		"items.capability_not_supported" {
		t.Fatalf("refused as %q, want items.capability_not_supported", got)
	}

	trashed := work.WorkItem{ID: "i1", DeletedAt: &remindedAt}
	if err := trashed.EnsureRemindable(remindable); err == nil {
		t.Fatal("a trashed entry took a reminder")
	}
}
