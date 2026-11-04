// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A plan's words that hold a zero byte or another control character where a line belongs are
// refused as malformed: the database stores no zero byte, and the write would fail as
// unavailable - an error a client retries for ever.
func TestAPlansWordsHoldingAZeroByteAreRefused(t *testing.T) {
	code := func(err error) string {
		var e *shared.Error
		if errors.As(err, &e) {
			return e.DetailCode
		}
		return ""
	}
	id := shared.MustParseID("0192f000-0000-7000-8000-0000000007b1")
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	sprint := func(name, goal string) error {
		_, err := planning.NewSprint(planning.NewSprintInput{
			ID: id, TenantID: id, CollectionID: id, Now: now,
			Fields: planning.SprintFields{Name: name, Goal: goal, StartOn: 1, EndOn: 2},
		})
		return err
	}
	release := func(name, description string) error {
		_, err := planning.NewRelease(planning.NewReleaseInput{
			ID: id, TenantID: id, HubID: id, Now: now,
			Fields: planning.ReleaseFields{Name: name, Description: description, ReleaseOn: 1},
		})
		return err
	}
	baseline := func(name string) error {
		_, err := planning.NewBaseline(planning.NewBaselineInput{ID: id, TenantID: id, PlanID: id, Name: name, Now: now})
		return err
	}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"a sprint's name", sprint("Sprint\x001", ""), "sprints.name_invalid"},
		{"a sprint's name with a tab", sprint("Sprint\t1", ""), "sprints.name_invalid"},
		{"a sprint's goal", sprint("Sprint 1", "Ship\x00"), "sprints.goal_malformed"},
		{"a release's name", release("2.4\x00", ""), "releases.name_invalid"},
		{"a release's description", release("2.4", "Fixes\x00"), "releases.description_malformed"},
		{"a baseline's name", baseline("Agreed\x00"), "baselines.name_invalid"},
	} {
		if got := code(tc.err); got != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, tc.err, tc.want)
		}
	}
	if err := sprint("Sprint 1", "Ship the beta.\nThen the docs."); err != nil {
		t.Errorf("a goal over two lines was refused: %v", err)
	}
}
