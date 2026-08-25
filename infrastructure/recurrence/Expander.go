// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package recurrence expands RFC 5545 rules through rrule-go (ADR-0008).
//
// The dependency lives here and nowhere else. ADR-0008 chose it over a home-grown implementation -
// "DST and edge cases are a well-known source of bugs" - and arc42 §11 R-07 names the same risk;
// what this package does is keep that choice reversible: the port is ours, the library is behind
// it, and a second implementation would be a second file rather than a change to the core.
package recurrence

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"

	port "github.com/Jersyfi/hubtask/core/port/recurrence"
)

// Expander is the adapter. It holds nothing: a rule is expanded from its text every time, because
// the text is what is stored and a cache would be a second copy of it that can disagree.
type Expander struct{}

func New() Expander { return Expander{} }

var _ port.Expander = Expander{}

// Occurrences reads the rule in its own zone and answers the moments it produces.
//
// The start is put into the rule rather than taken from the text: DTSTART belongs to the entry's
// due date (the application refuses a rule that carries one), and a library reading it in UTC
// would resolve every transition against the wrong clock. The zone is loaded and the start is
// moved into it before the expansion, which is what makes 09:00 stay 09:00 across a change.
func (e Expander) Occurrences(
	rule port.Rule, after, before time.Time, limit int,
) ([]time.Time, error) {
	location, err := time.LoadLocation(strings.TrimSpace(rule.TimeZone))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", port.ErrZoneUnknown, rule.TimeZone)
	}

	option, err := rrule.StrToROption(strings.TrimSpace(rule.RRULE))
	if err != nil {
		return nil, errors.Join(port.ErrRuleUnreadable, err)
	}
	option.Dtstart = rule.Start.In(location)
	if !rule.Until.IsZero() {
		option.Until = rule.Until.In(location)
	}
	if rule.Count > 0 {
		option.Count = rule.Count
	}

	expanded, err := rrule.NewRRule(*option)
	if err != nil {
		// The library's own bounds check: a BYHOUR of 25, a BYMONTH of 13. Text that parsed and
		// still cannot be a rule is the same answer as text that did not parse, because it is the
		// same thing to whoever wrote it.
		return nil, errors.Join(port.ErrRuleUnreadable, err)
	}

	// Inclusive on both ends: a caller asking from the entry's due date expects that date to be
	// the first occurrence, and one asking to a horizon expects the horizon's own day to count.
	moments := expanded.Between(after, before, true)
	if limit > 0 && len(moments) > limit {
		moments = moments[:limit]
	}

	// Answered in UTC, which is how every instant in this system travels (i18n-l10n.md §7). The
	// zone did its work during the expansion; carrying it further would invite a caller to compare
	// two moments in two zones.
	utc := make([]time.Time, 0, len(moments))
	for _, moment := range moments {
		utc = append(utc, moment.UTC())
	}
	return utc, nil
}
