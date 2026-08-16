// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Acceptance criterion of task A-02: every error category has a test.
func TestEveryCategoryIsValidAndCarriesASentinel(t *testing.T) {
	cases := []struct {
		category Category
		sentinel *Error
		code     string
	}{
		{CategoryValidation, ErrValidation, "validation_failed"},
		{CategoryNotFound, ErrNotFound, "not_found"},
		{CategoryConflict, ErrConflict, "conflict"},
		{CategoryForbidden, ErrForbidden, "forbidden"},
		{CategoryUnauthenticated, ErrUnauthenticated, "unauthenticated"},
		{CategoryGone, ErrGone, "gone"},
		{CategoryRateLimited, ErrRateLimited, "rate_limited"},
		{CategoryUnavailable, ErrUnavailable, "dependency_unavailable"},
		{CategoryInternal, ErrInternal, "internal"},
	}

	covered := map[Category]bool{}
	for _, tc := range cases {
		covered[tc.category] = true
		t.Run(string(tc.category), func(t *testing.T) {
			if !tc.category.Valid() {
				t.Errorf("category %s is not recognised as valid", tc.category)
			}
			if tc.sentinel.Category != tc.category {
				t.Errorf("sentinel carries category %s, want %s", tc.sentinel.Category, tc.category)
			}
			if tc.sentinel.Code != tc.code {
				t.Errorf("code = %q, want %q", tc.sentinel.Code, tc.code)
			}
		})
	}

	// "Every category has a test" has to stay true when a tenth one is added, so the list above
	// is checked against the set rather than trusted. Categories is also what the observability
	// layer derives its `result` label from (observability-reliability.md §4.1) - a category
	// missing from it would be invisible in the metrics.
	for _, category := range Categories() {
		if !covered[category] {
			t.Errorf("the category %s has no case in this test", category)
		}
	}
	if len(cases) != len(Categories()) {
		t.Errorf("%d cases for %d categories", len(cases), len(Categories()))
	}
}

func TestUnknownCategoryIsNotValid(t *testing.T) {
	if Category("SOMETHING_ELSE").Valid() {
		t.Error("an invented category must not pass as valid")
	}
	if Category("").Valid() {
		t.Error("the empty category must not pass as valid")
	}
}

// Categories hands out a copy: a caller that sorts or truncates the result must not be able to
// change what Valid accepts.
func TestCategoriesCannotBeModifiedByACaller(t *testing.T) {
	first := Categories()
	first[0] = "MADE_UP"

	if got := Categories()[0]; got == "MADE_UP" {
		t.Error("a caller modified the category set")
	}
	if !CategoryValidation.Valid() {
		t.Error("a caller broke the validity check")
	}
}

func TestIsComparesByCodeNotByInstance(t *testing.T) {
	err := ErrNotFound.WithParams(map[string]string{"id": "01J9"}).WithDetail("items.not_found")

	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is does not recognise the sentinel it was derived from")
	}
	if errors.Is(err, ErrConflict) {
		t.Error("errors.Is confuses two different codes")
	}
	// conflict and version_conflict share a category but are distinct codes.
	if errors.Is(ErrVersionConflict, ErrConflict) {
		t.Error("version_conflict must not pass as conflict")
	}
}

func TestIsFindsTheErrorThroughAWrappedChain(t *testing.T) {
	wrapped := fmt.Errorf("loading the container: %w", ErrNotFound)

	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("errors.Is does not see through a wrapping chain")
	}
	var domainErr *Error
	if !errors.As(wrapped, &domainErr) || domainErr.Category != CategoryNotFound {
		t.Error("errors.As does not recover the category from a wrapping chain")
	}
}

// A sentinel is package state. A caller enriching it must not change it for everyone else.
func TestDerivingDoesNotModifyTheSentinel(t *testing.T) {
	_ = ErrValidation.
		WithParams(map[string]string{"field": "title"}).
		WithDetail("items.title_too_long").
		WithFields(FieldError{Path: "/title", Code: "too_long"}).
		WithCause(errors.New("boom"))

	if ErrValidation.Params != nil || ErrValidation.DetailCode != "" ||
		ErrValidation.Fields != nil || ErrValidation.Unwrap() != nil {
		t.Errorf("the sentinel was modified: %+v", ErrValidation)
	}
}

// The parameter map handed in must not stay connected to the error afterwards.
func TestParamsAreCopied(t *testing.T) {
	params := map[string]string{"item_type": "ACTIVITY"}
	err := ErrValidation.WithParams(params)
	params["item_type"] = "TASK"

	if err.Params["item_type"] != "ACTIVITY" {
		t.Errorf("params = %q, want ACTIVITY - the map was not copied", err.Params["item_type"])
	}
}

func TestErrorTextIsCodesNotProse(t *testing.T) {
	err := ErrValidation.
		WithDetail("items.cover_not_supported_for_type").
		WithParams(map[string]string{"item_type": "ACTIVITY", "capability": "COVER"})

	got := err.Error()
	want := "VALIDATION: validation_failed (items.cover_not_supported_for_type) " +
		"[capability=COVER item_type=ACTIVITY]"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// The text must be stable across runs, otherwise it is useless in a log and in a test. Go
// randomises map iteration, so unsorted parameters would show up here.
func TestErrorTextIsStableAcrossRuns(t *testing.T) {
	err := ErrValidation.WithParams(map[string]string{
		"d": "4", "a": "1", "c": "3", "b": "2", "e": "5",
	})

	first := err.Error()
	for range 50 {
		if got := err.Error(); got != first {
			t.Fatalf("Error() varies between calls: %q then %q", first, got)
		}
	}
	if !strings.Contains(first, "[a=1 b=2 c=3 d=4 e=5]") {
		t.Errorf("parameters are not sorted: %q", first)
	}
}

func TestCauseIsReachableForTheLogAndKeptOutOfTheContract(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.5:5432: connection refused")
	err := ErrUnavailable.WithCause(cause)

	if !errors.Is(err, cause) {
		t.Error("the technical cause is not reachable through errors.Is")
	}
	// Code and category are what an adapter renders - and neither carries the cause.
	if err.Code != "dependency_unavailable" || err.Category != CategoryUnavailable {
		t.Errorf("the cause changed the classification: %s/%s", err.Category, err.Code)
	}
}

func TestAsErrorClassifiesAForeignErrorAsInternal(t *testing.T) {
	foreign := errors.New("postgres://hubtask:hunter2@db:5432 is unreachable")

	got := AsError(foreign)

	if got.Category != CategoryInternal || got.Code != "internal" {
		t.Errorf("classification = %s/%s, want INTERNAL/internal", got.Category, got.Code)
	}
	// The text of a foreign error may contain anything, a connection string included. It
	// belongs in the log, and nowhere near a response (security.md §9).
	if got.Code != "internal" || got.DetailCode != "" || len(got.Params) != 0 {
		t.Error("a foreign error contributed contract fields")
	}
	if !errors.Is(got, foreign) {
		t.Error("the original error is not reachable for the log")
	}
}

func TestAsErrorPassesADomainErrorThrough(t *testing.T) {
	original := ErrForbidden.WithDetail("containers.not_a_member")

	got := AsError(fmt.Errorf("checking the permission: %w", original))

	if got.Category != CategoryForbidden || got.DetailCode != "containers.not_a_member" {
		t.Errorf("got %s/%s, want FORBIDDEN/containers.not_a_member", got.Category, got.DetailCode)
	}
}

// An error carrying a category nobody defined is a defect, not a new category.
func TestAsErrorTreatsAnInventedCategoryAsInternal(t *testing.T) {
	invented := &Error{Category: "MADE_UP", Code: "surprise"}

	if got := AsError(invented); got.Category != CategoryInternal {
		t.Errorf("category = %s, want INTERNAL", got.Category)
	}
}

func TestAsErrorOfNilIsNil(t *testing.T) {
	if got := AsError(nil); got != nil {
		t.Errorf("AsError(nil) = %v, want nil", got)
	}
}

func TestValidationCarriesTheFieldPath(t *testing.T) {
	err := Validation("/title", "too_long", map[string]string{"max": "200"})

	if err.Category != CategoryValidation {
		t.Errorf("category = %s, want VALIDATION", err.Category)
	}
	if len(err.Fields) != 1 || err.Fields[0].Path != "/title" || err.Fields[0].Code != "too_long" {
		t.Errorf("field error = %+v", err.Fields)
	}
	if err.Fields[0].Params["max"] != "200" {
		t.Errorf("field parameters = %v", err.Fields[0].Params)
	}
}

func TestInternalfKeepsTheDetailInTheCause(t *testing.T) {
	err := Internalf("writing the outbox entry: %w", errors.New("disk full"))

	if err.Category != CategoryInternal {
		t.Errorf("category = %s, want INTERNAL", err.Category)
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("the cause is missing from the log text: %q", err.Error())
	}
}
