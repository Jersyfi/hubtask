// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"math/big"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Cadence is how often a dashboard's digest is mailed.
type Cadence string

const (
	CadenceDaily  Cadence = "DAILY"
	CadenceWeekly Cadence = "WEEKLY"
)

// MaxDigestLines bounds the totals one digest carries: a dashboard has at most MaxWidgets tiles.
const MaxDigestLines = MaxWidgets

// Digest is one person's subscription to a dashboard by mail (UC-GOL-04): at an hour of their own
// zone, every day or on one weekday. The mail carries a link and the totals of the dashboard's
// NUMBER widgets, computed as the subscriber when it is sent - never anybody else's numbers, and
// never an attachment (the mail port has none by design).
type Digest struct {
	ID          shared.ID
	DashboardID shared.ID
	AccountID   shared.ID
	Cadence     Cadence
	// Weekday is ISO: 1 Monday … 7 Sunday; WEEKLY only, 0 for DAILY.
	Weekday  int
	Hour     int
	TimeZone string
	// NextAt is the next moment the digest is owed.
	NextAt     time.Time
	LastSentAt *time.Time
}

// CheckDigest checks a digest's cadence, weekday, hour and zone.
func CheckDigest(d Digest) (Digest, error) {
	switch d.Cadence {
	case CadenceDaily:
		d.Weekday = 0
	case CadenceWeekly:
		if d.Weekday < 1 || d.Weekday > 7 {
			return Digest{}, invalid("/weekday", "dashboards.digest_weekday_invalid")
		}
	default:
		return Digest{}, invalid("/cadence", "dashboards.digest_cadence_invalid")
	}
	if d.Hour < 0 || d.Hour > 23 {
		return Digest{}, invalid("/hour", "dashboards.digest_hour_invalid")
	}
	if d.TimeZone == "" {
		d.TimeZone = "UTC"
	}
	if _, err := time.LoadLocation(d.TimeZone); err != nil {
		return Digest{}, invalid("/time_zone", "dashboards.digest_zone_invalid")
	}
	return d, nil
}

// Next answers the first moment strictly after `after` the digest is owed: the hour on the hour in
// its zone, on the next day or the next of its weekday. A wall time a daylight-saving change skips
// is the moment the zone's rules make of it.
func (d Digest) Next(after time.Time) time.Time {
	zone, err := time.LoadLocation(d.TimeZone)
	if err != nil {
		zone = time.UTC
	}
	local := after.In(zone)
	for i := 0; i <= 8; i++ {
		day := local.AddDate(0, 0, i)
		at := time.Date(day.Year(), day.Month(), day.Day(), d.Hour, 0, 0, 0, zone)
		if !at.After(after) {
			continue
		}
		if d.Cadence == CadenceWeekly && isoWeekday(at) != d.Weekday {
			continue
		}
		return at.UTC()
	}
	return after.Add(24 * time.Hour).UTC()
}

// Due answers whether the digest is owed at now.
func (d Digest) Due(now time.Time) bool { return !d.NextAt.After(now) }

func isoWeekday(t time.Time) int {
	if t.Weekday() == time.Sunday {
		return 7
	}
	return int(t.Weekday())
}

// Total is what a NUMBER widget shows: the sum of its rows' values, a row's `value` where the
// summary has one and its `count` otherwise - the web app's total, in exact decimals so a sum of
// money never gains a stray digit. False when no row carries a number.
func Total(rows []map[string]any) (string, bool) {
	sum := new(big.Rat)
	places, found := 0, false
	for _, row := range rows {
		if raw, ok := row["value"].(string); ok && raw != "" {
			if r, ok := new(big.Rat).SetString(raw); ok {
				sum.Add(sum, r)
				places = max(places, decimals(raw))
				found = true
				continue
			}
		}
		if n, ok := countOf(row["count"]); ok {
			sum.Add(sum, new(big.Rat).SetInt64(n))
			found = true
		}
	}
	if !found {
		return "", false
	}
	return sum.FloatString(places), true
}

func decimals(raw string) int {
	for i := 0; i < len(raw); i++ {
		if raw[i] == '.' {
			return len(raw) - i - 1
		}
	}
	return 0
}

func countOf(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case float64:
		if n == float64(int64(n)) {
			return int64(n), true
		}
	case string:
		if parsed, err := strconv.ParseInt(n, 10, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
