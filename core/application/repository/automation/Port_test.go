// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in place.
// The double proves the interface can still be implemented by a fake, which is what the use case
// tests depend on.
type double struct{}

func (double) Insert(context.Context, domain.Rule) error { return nil }
func (double) Find(context.Context, shared.ID) (domain.Rule, error) {
	return domain.Rule{}, shared.ErrNotFound
}
func (double) List(context.Context, Query) (Page, error) { return Page{}, nil }
func (double) Update(context.Context, domain.Rule, int) error {
	return shared.ErrConflict
}

func (double) SetEnabled(context.Context, shared.ID, bool, int, time.Time) error {
	return shared.ErrConflict
}
func (double) Delete(context.Context, shared.ID, time.Time) (bool, error) { return false, nil }

var _ Rules = double{}

// Four answers the use cases tell apart, so all four have to be expressible: a rule that is not
// there, a version somebody got to first, a page with nothing on it, and a deletion that changed
// nothing because it had already happened.
func TestTheEmptyAnswersAreDistinguishable(t *testing.T) {
	if _, err := (double{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing rule answered %v", err)
	}
	if err := (double{}).Update(t.Context(), domain.Rule{}, 1); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a stale version answered %v", err)
	}

	page, err := (double{}).List(t.Context(), Query{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Rules) != 0 || page.HasMore || page.NextCursor != "" {
		t.Errorf("an empty page is %+v", page)
	}

	changed, err := (double{}).Delete(t.Context(), "", time.Time{})
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if changed {
		t.Error("a deletion that changed nothing reported that it had")
	}
}

// Nil is "either", which is what an absent query parameter means. A bool would make "not asked" and
// "asked for the disabled ones" the same request.
func TestTheEnabledFilterHasThreeStates(t *testing.T) {
	on, off := true, false

	for name, query := range map[string]Query{
		"either": {},
		"on":     {Enabled: &on},
		"off":    {Enabled: &off},
	} {
		t.Run(name, func(t *testing.T) {
			switch {
			case name == "either" && query.Enabled != nil:
				t.Error("an unasked filter carries a value")
			case name == "on" && (query.Enabled == nil || !*query.Enabled):
				t.Error("the filter for the ones that are on does not say so")
			case name == "off" && (query.Enabled == nil || *query.Enabled):
				t.Error("the filter for the ones that are off does not say so")
			}
		})
	}
}
