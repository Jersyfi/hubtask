// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	tenantID      = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	occurred      = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	completeEntry = Entry{
		TenantID: tenantID, OccurredAt: occurred, Action: "container.created",
		Outcome: OutcomeSuccess, Severity: SeverityInfo, ActorKind: shared.ActorUser,
	}
)

// Test AT-4 at the level where the masking is decided: no sensitive value in clear text, and no
// secret in any form.
func TestChangesMaskPerClassification(t *testing.T) {
	masked := Changes(
		Change{Field: "type", Classification: Open, To: "COLLECTION"},
		Change{Field: "name", Classification: Sensitive, From: "Groceries", To: "Shopping"},
		Change{Field: "token", Classification: Secret, To: "hbt_pat_secret"},
	)

	if _, present := masked["token"]; present {
		t.Error("a secret reached the trail")
	}
	if open := masked["type"].(map[string]any); open["to"] != "COLLECTION" {
		t.Errorf("an open value was not recorded readably: %v", open)
	}

	name, ok := masked["name"].(map[string]any)
	if !ok {
		t.Fatalf("a sensitive field is missing: %v", masked)
	}
	if name["changed"] != true || name["to"] != nil || name["from"] != nil {
		t.Errorf("a sensitive value reached the trail: %v", name)
	}
	if name["to_hash"] == name["from_hash"] {
		t.Error("two different values produced the same fingerprint")
	}

	// The point of the fingerprint: comparing two entries without reading either value.
	keyed := KeyFingerprints(Entry{TenantID: tenantID, Changes: masked}, tagging{}).Changes
	again := KeyFingerprints(Entry{TenantID: tenantID,
		Changes: Changes(Change{Field: "name", Classification: Sensitive, To: "Shopping"})}, tagging{}).Changes
	if again["name"].(map[string]any)["to_hash"] != keyed["name"].(map[string]any)["to_hash"] {
		t.Error("the same value produced two different fingerprints")
	}
}

// tagging is a fingerprinter whose answer shows what it was given, so a test can see that the
// keyed step received the workspace and the canonical value.
type tagging struct{}

func (tagging) Fingerprint(tenant shared.ID, value string) string {
	return "keyed:" + tenant.String() + ":" + value
}

// The sink keys every sensitive value with the entry's workspace, and touches nothing else - not
// an open value, and not the map the caller built.
func TestKeyFingerprintsKeysEverySensitiveValueAndNothingElse(t *testing.T) {
	built := Changes(
		Change{Field: "type", Classification: Open, From: "OPEN", To: "DONE"},
		Change{Field: "name", Classification: Sensitive, From: "Groceries", To: 42},
	)
	keyed := KeyFingerprints(Entry{TenantID: tenantID, Changes: built}, tagging{}).Changes

	name := keyed["name"].(map[string]any)
	if name["to_hash"] != "keyed:"+tenantID.String()+":42" || name["from_hash"] != "keyed:"+tenantID.String()+":Groceries" {
		t.Errorf("the sensitive values were not keyed with the workspace and the canonical value: %v", name)
	}
	if name["changed"] != true {
		t.Errorf("the keyed field lost its marker: %v", name)
	}
	if open := keyed["type"].(map[string]any); open["to"] != "DONE" || open["from"] != "OPEN" {
		t.Errorf("an open value changed on its way through: %v", open)
	}
	if _, stillPending := built["name"].(map[string]any)["to_hash"].(unkeyed); !stillPending {
		t.Error("keying changed the caller's map")
	}
	if _, err := json.Marshal(keyed); err != nil {
		t.Errorf("a keyed entry does not serialise: %v", err)
	}
}

// A sink without a fingerprinter records that the field changed, and no fingerprint at all - never
// an unkeyed one, and never the value.
func TestWithoutAFingerprinterNoFingerprintIsRecorded(t *testing.T) {
	built := Changes(Change{Field: "name", Classification: Sensitive, From: "Groceries", To: "Shopping"})
	keyed := KeyFingerprints(Entry{TenantID: tenantID, Changes: built}, nil).Changes

	name := keyed["name"].(map[string]any)
	if len(name) != 1 || name["changed"] != true {
		t.Errorf("an unkeyed sink recorded more than that the field changed: %v", name)
	}
	stored, err := json.Marshal(keyed)
	if err != nil {
		t.Fatalf("serialising: %v", err)
	}
	if strings.Contains(string(stored), "Groceries") || strings.Contains(string(stored), "Shopping") ||
		strings.Contains(string(stored), "_hash") {
		t.Errorf("an unkeyed sink stored a value or a fingerprint: %s", stored)
	}
}

// Until the sink keys it, a sensitive value cannot be read anywhere it might be printed (rule 10),
// and cannot be serialised - a path that skipped the sink fails rather than storing something.
func TestAValueAwaitingItsFingerprintCannotBeRead(t *testing.T) {
	masked := Changes(Change{Field: "name", Classification: Sensitive, To: "Lena Beispiel"})
	pending := masked["name"].(map[string]any)["to_hash"]

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T %v"} {
		for _, printed := range []string{fmt.Sprintf(verb, pending), fmt.Sprintf(verb, masked)} {
			if strings.Contains(printed, "Lena") || strings.Contains(printed, "4c656e61") ||
				strings.Contains(printed, "4C656E61") {
				t.Errorf("%s printed the value: %s", verb, printed)
			}
		}
		if printed := fmt.Sprintf(verb, pending); verb != "%T %v" && printed != Redacted {
			t.Errorf("%s printed %q, want %q", verb, printed, Redacted)
		}
	}
	if fmt.Sprint(pending) != Redacted || fmt.Sprintln(masked) == "" {
		t.Error("the value is not shown as redacted")
	}

	_, err := json.Marshal(masked)
	if err == nil || !strings.Contains(err.Error(), "audit.fingerprint_unkeyed") {
		t.Errorf("an unkeyed value serialised: %v", err)
	}
}

// A field that was not set before has no previous value, and recording an empty one would read as
// "it was empty" rather than "it did not exist".
func TestAnAbsentPreviousValueIsNotRecorded(t *testing.T) {
	masked := Changes(
		Change{Field: "type", Classification: Open, To: "HUB"},
		Change{Field: "name", Classification: Sensitive, To: "Private"},
	)

	if _, present := masked["type"].(map[string]any)["from"]; present {
		t.Error("an open field reports a previous value it never had")
	}
	if _, present := masked["name"].(map[string]any)["from_hash"]; present {
		t.Error("a sensitive field reports a previous value it never had")
	}
}

// An entry that cannot be written aborts the transaction it belongs to, so it is refused before
// the database gets the chance - with a code that says what is missing (test AT-5).
func TestValidateRefusesAnEntryTheTrailCouldNotStandBehind(t *testing.T) {
	cases := map[string]struct {
		mutate     func(*Entry)
		detailCode string
	}{
		"without a tenant":         {func(e *Entry) { e.TenantID = "" }, "audit.entry_incomplete"},
		"without an action":        {func(e *Entry) { e.Action = "" }, "audit.entry_incomplete"},
		"without a time":           {func(e *Entry) { e.OccurredAt = time.Time{} }, "audit.entry_incomplete"},
		"with no outcome":          {func(e *Entry) { e.Outcome = "" }, "audit.entry_incomplete"},
		"with an invented outcome": {func(e *Entry) { e.Outcome = "MAYBE" }, "audit.entry_incomplete"},
		"with no severity":         {func(e *Entry) { e.Severity = "" }, "audit.entry_incomplete"},
		"with an anonymous actor":  {func(e *Entry) { e.ActorKind = shared.ActorAnonymous }, "audit.actor_kind_invalid"},
		"with an invented actor":   {func(e *Entry) { e.ActorKind = "ROBOT" }, "audit.actor_kind_invalid"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			entry := completeEntry
			c.mutate(&entry)

			err := entry.Validate()
			if !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("error %v, want an internal one", err)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("detail code %s, want %s", got, c.detailCode)
			}
		})
	}

	if err := completeEntry.Validate(); err != nil {
		t.Errorf("a complete entry was refused: %v", err)
	}
}

// The vocabulary is the database's: an outcome or a severity outside these sets is refused by a
// CHECK constraint, and finding that out at commit time is finding it out too late.
func TestTheVocabularyMatchesTheSchema(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeSuccess, OutcomeDenied, OutcomeFailed} {
		if !outcome.Valid() || strings.ToUpper(string(outcome)) != string(outcome) {
			t.Errorf("%s is not a schema outcome", outcome)
		}
	}
	for _, severity := range []Severity{SeverityInfo, SeverityNotice, SeverityWarning, SeverityCritical} {
		if !severity.Valid() {
			t.Errorf("%s is not a schema severity", severity)
		}
	}
	if Outcome("SUCCEEDED").Valid() || Severity("FATAL").Valid() {
		t.Error("a value the schema would reject is accepted here")
	}
}

// The derivation: six classes in, three masking levels out. It is written down so that a gate can
// check a use case's decision rather than each use case deciding alone.
func TestEveryClassDerivesItsMasking(t *testing.T) {
	want := map[shared.DataClass]Classification{
		shared.ClassNonPersonal:         Open,
		shared.ClassPersonalTechnical:   Open,
		shared.ClassPersonalBasic:       Sensitive,
		shared.ClassPersonalContent:     Sensitive,
		shared.ClassSpecialCategoryRisk: Sensitive,
		shared.ClassSecret:              Secret,
	}

	for _, class := range shared.DataClasses() {
		masking, known := want[class]
		if !known {
			t.Fatalf("%s has no derivation - a class without one is a field nobody can classify", class)
		}
		if got := MaskingFor(class); got != masking {
			t.Errorf("%s masks as %s, want %s", class, got, masking)
		}
	}

	// A class this build does not know masks as the strictest thing that still records something.
	// Guessing OPEN for an unfamiliar class is the one direction that cannot be taken back.
	if MaskingFor("SOMETHING_NEW") != Sensitive {
		t.Error("an unknown class is treated as open")
	}
}

// An unclassified change is masked rather than written in clear text. PG-1 refuses one at build
// time; this is what happens if something builds a change from a variable anyway.
func TestAnUnclassifiedChangeIsNotWrittenInClearText(t *testing.T) {
	masked := Changes(Change{Field: "title", To: "Weekly shop"})

	entry, _ := masked["title"].(map[string]any)
	if entry == nil {
		t.Fatalf("the unclassified field was dropped entirely: %v", masked)
	}
	if entry["to"] != nil {
		t.Errorf("an unclassified value reached the trail: %v", entry)
	}
	if entry["unclassified"] != true {
		t.Errorf("the entry does not say the field was unclassified: %v", entry)
	}
}
