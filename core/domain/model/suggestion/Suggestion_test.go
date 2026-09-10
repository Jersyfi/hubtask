// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

var (
	suggestionID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	tenantID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
	targetID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000e3")
	deciderID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000e4")
	moment       = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
)

func input() domain.NewInput {
	return domain.NewInput{
		ID: suggestionID, TenantID: tenantID,
		TargetType: domain.TargetWorkItem, TargetID: targetID,
		Kind:    domain.KindFields,
		Payload: map[string]any{"title": "Buy milk"},
		Provenance: domain.Provenance{
			Model: "a-model", PromptID: "suggest-fields", PromptVersion: "v1",
			ProducedAt: moment.Add(-time.Minute),
		},
		InputDigest: []byte("a-digest"),
		Now:         moment,
	}
}

func TestANewSuggestionIsProposedAndHasChangedNothing(t *testing.T) {
	recorded, err := domain.New(input())
	if err != nil {
		t.Fatalf("a valid suggestion was refused: %v", err)
	}

	if recorded.Status != domain.StatusProposed {
		t.Errorf("status %q, want PROPOSED", recorded.Status)
	}
	if !recorded.DecidedAt.IsZero() || !recorded.DecidedBy.IsZero() {
		t.Error("a new suggestion arrived already decided")
	}
	if recorded.Source != domain.SourceAI {
		t.Errorf("source %q, want AI", recorded.Source)
	}
	if recorded.Version != 1 || recorded.CreatedAt.IsZero() {
		t.Errorf("the record came back unstamped: %+v", recorded)
	}
}

// The producer's own defects, each with its own code, because each sends whoever reads the log to
// a different place. All internal: nobody submits a suggestion.
func TestAMalformedSuggestionIsTheProducersDefect(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		amend func(*domain.NewInput)
		code  string
	}{
		{"no identity", func(in *domain.NewInput) { in.ID = shared.ID("") }, "suggestions.incomplete"},
		{"no target", func(in *domain.NewInput) { in.TargetID = shared.ID("") }, "suggestions.incomplete"},
		{"a target type nobody defined", func(in *domain.NewInput) {
			in.TargetType = "SOMETHING"
		}, "suggestions.target_type_unknown"},
		{"a kind nobody defined", func(in *domain.NewInput) {
			in.Kind = "GUESS"
		}, "suggestions.kind_unknown"},
		{"nothing proposed", func(in *domain.NewInput) {
			in.Payload = map[string]any{}
		}, "suggestions.payload_empty"},
		{"nothing to judge staleness by", func(in *domain.NewInput) {
			in.InputDigest = nil
		}, "suggestions.input_digest_required"},
		{"no model", func(in *domain.NewInput) {
			in.Provenance.Model = ""
		}, "suggestions.provenance_incomplete"},
		{"no prompt version", func(in *domain.NewInput) {
			in.Provenance.PromptVersion = ""
		}, "suggestions.provenance_incomplete"},
		{"no moment the provider answered", func(in *domain.NewInput) {
			in.Provenance.ProducedAt = time.Time{}
		}, "suggestions.provenance_incomplete"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			in := input()
			testCase.amend(&in)

			_, err := domain.New(in)
			if !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("the refusal was %v, want an internal error - nobody submits a suggestion", err)
			}
			if got := shared.AsError(err).DetailCode; got != testCase.code {
				t.Errorf("detail code %q, want %q", got, testCase.code)
			}
		})
	}
}

// The source is written down whatever a caller says, so a producer cannot record a proposal as
// anything but what it is.
func TestTheSourceIsNotTheProducersToChoose(t *testing.T) {
	in := input()
	in.Provenance.Source = "A_PERSON"

	recorded, err := domain.New(in)
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	if recorded.Source != domain.SourceAI {
		t.Errorf("source %q, want AI", recorded.Source)
	}
}

func TestDecidingHappensOnceAndRecordsWho(t *testing.T) {
	recorded, err := domain.New(input())
	if err != nil {
		t.Fatalf("recording: %v", err)
	}

	accepted, err := recorded.Decide(domain.StatusAccepted, deciderID, moment)
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if accepted.Status != domain.StatusAccepted || accepted.DecidedBy != deciderID {
		t.Errorf("the decision is %+v", accepted)
	}
	if accepted.Version != 2 || accepted.DecidedAt.IsZero() {
		t.Errorf("the decision left the record at %+v", accepted)
	}

	for _, second := range []domain.Status{domain.StatusAccepted, domain.StatusDismissed} {
		if _, err := accepted.Decide(second, deciderID, moment); !errors.Is(err, shared.ErrConflict) {
			t.Errorf("deciding a decided suggestion %q answered %v, want a conflict", second, err)
		}
	}
}

// A suggestion is decided by a person. No rule and no job writes here, and the schema says the
// same thing from the other side.
func TestADecisionNeedsAPersonAndAMoment(t *testing.T) {
	recorded, _ := domain.New(input())

	for _, testCase := range []struct {
		name string
		by   shared.ID
		at   time.Time
	}{
		{"nobody decided it", shared.ID(""), moment},
		{"it was decided at no time", deciderID, time.Time{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := recorded.Decide(domain.StatusAccepted, testCase.by, testCase.at); !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("the refusal was %v", err)
			}
		})
	}
}

// PROPOSED is not a decision, and a status outside the set is a defect rather than a third state.
func TestOnlyADecisionCanBeDecided(t *testing.T) {
	recorded, _ := domain.New(input())

	for _, status := range []domain.Status{domain.StatusProposed, "SOMETHING"} {
		if _, err := recorded.Decide(status, deciderID, moment); !errors.Is(err, shared.ErrInternal) {
			t.Errorf("deciding %q answered %v", status, err)
		}
	}
}

// The fingerprint is what makes a stale suggestion recognisable instead of silently applied.
func TestFreshnessIsJudgedAgainstWhatTheSuggestionWasMadeFrom(t *testing.T) {
	recorded, _ := domain.New(input())

	if !recorded.Fresh([]byte("a-digest")) {
		t.Error("a suggestion is stale against the very state it was made from")
	}
	if recorded.Fresh([]byte("the entry was rewritten")) {
		t.Error("a suggestion is fresh against a state it was not made from")
	}
	if recorded.Fresh(nil) {
		t.Error("a suggestion is fresh against no state at all; an unknown state is not a match")
	}
}

// A caller that mutates the digest it handed over must not be able to make a stale suggestion look
// fresh afterwards.
func TestTheRecordKeepsItsOwnCopyOfTheDigest(t *testing.T) {
	digest := []byte("a-digest")
	in := input()
	in.InputDigest = digest

	recorded, _ := domain.New(in)
	digest[0] = 'X'

	if !recorded.Fresh([]byte("a-digest")) {
		t.Error("changing the caller's slice changed the record's")
	}
}

// A proposal no prompt produced carries no prompt, and one that carries half of the pair is a
// record that resolves to nothing (K-04).
func TestAProposalCarriesAPromptAndItsVersionOrNeither(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		id, version string
		recorded    bool
	}{
		{"a prompt and its version", "classify", "v2", true},
		{"neither, which is what a nearest-neighbour query has", "", "", true},
		{"a prompt whose version nobody wrote down", "classify", "", false},
		{"a version belonging to no prompt", "", "v2", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			in := input()
			in.Kind = domain.KindDuplicates
			in.Provenance.PromptID, in.Provenance.PromptVersion = testCase.id, testCase.version

			recorded, err := domain.New(in)
			if testCase.recorded {
				if err != nil {
					t.Fatalf("recording: %v", err)
				}
				if recorded.PromptID != testCase.id {
					t.Errorf("the prompt came back as %q", recorded.PromptID)
				}
				return
			}
			if err == nil {
				t.Fatal("half a provenance was recorded")
			}
			if got := shared.AsError(err).DetailCode; got != "suggestions.provenance_incomplete" {
				t.Errorf("detail code %q", got)
			}
		})
	}
}

// The model is required whatever produced the proposal: for a completion it is what answered, and
// for a nearest-neighbour query it is the embedding model whose vectors were compared - a
// similarity means nothing outside one model's space.
func TestAProposalWithoutAModelIsNotRecorded(t *testing.T) {
	in := input()
	in.Provenance.Model = ""

	if _, err := domain.New(in); err == nil {
		t.Fatal("a proposal that names no model was recorded")
	}
}

func TestTheClosedSetsAreClosed(t *testing.T) {
	if domain.Kind("SUMMARY").Valid() {
		t.Error("SUMMARY reports itself a kind; it is a FIELDS suggestion whose payload is notes")
	}
	if domain.TargetType("COMMENT").Valid() || domain.Status("PENDING").Valid() {
		t.Error("an invented value reports itself valid")
	}
	if len(domain.Kinds()) != 3 || len(domain.TargetTypes()) != 3 || len(domain.Statuses()) != 3 {
		t.Error("a value was added to a closed set without this test being told")
	}
	if domain.StatusProposed.Decided() {
		t.Error("a proposal reports itself decided")
	}
}
