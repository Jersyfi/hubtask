// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"errors"
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
	for _, field := range []string{"to_hash", "from_hash"} {
		if digest, _ := name[field].(string); len(digest) != 64 {
			t.Errorf("%s is not a SHA-256 fingerprint: %v", field, name[field])
		}
	}

	// The point of the fingerprint: comparing two entries without reading either value.
	again := Changes(Change{Field: "name", Classification: Sensitive, To: "Shopping"})
	if again["name"].(map[string]any)["to_hash"] != name["to_hash"] {
		t.Error("the same value produced two different fingerprints")
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
