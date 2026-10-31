// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func installationEntry(key string) map[string]any {
	return map[string]any{
		"key": key, "name": "Federal", "zone": "Europe/Berlin",
		"hours":    []any{map[string]any{"day": "MONDAY", "from": "08:00", "to": "16:00"}},
		"holidays": []any{"2026-12-25"},
	}
}

// UC-SVC-01 check 6: the instance file's calendars are read in the API's shape and checked as a
// workspace's are; one malformed entry or a key named twice refuses the whole list.
func TestTheInstallationsCalendarsAreReadWholeOrNotAtAll(t *testing.T) {
	read, err := InstallationCalendars([]map[string]any{installationEntry("federal")}, text.Composing{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	federal, ok := read["federal"]
	if !ok || federal.Name != "Federal" || federal.Calendar.Spec().Zone != "Europe/Berlin" ||
		len(federal.Calendar.Spec().Holidays) != 1 {
		t.Fatalf("read %+v", read)
	}
	for name, entries := range map[string][]map[string]any{
		"a key twice":     {installationEntry("federal"), installationEntry("federal")},
		"a bad key":       {installationEntry("Federal Days")},
		"no working time": {{"key": "x", "name": "X", "zone": "UTC"}},
		"an unknown zone": {{"key": "x", "name": "X", "zone": "Mars/Base", "hours": installationEntry("x")["hours"]}},
		"a bad holiday":   {{"key": "x", "name": "X", "zone": "UTC", "hours": installationEntry("x")["hours"], "holidays": []any{"soon"}}},
	} {
		if _, err := InstallationCalendars(entries, text.Composing{}); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s answered %v", name, err)
		}
	}
}

// UC-SVC-01 check 6: a reader of a calendar by key meets the workspace's own of that key first and
// the installation's where the workspace holds none; a key neither holds is not found.
func TestAWorkspaceCalendarFallsBackToTheInstallations(t *testing.T) {
	var week calendar.Week
	week[1] = []calendar.Span{{From: 9 * 60, To: 17 * 60}}
	own, err := domain.NewInstallationCalendar("federal", "Ours", calendar.Spec{Zone: "UTC", Week: week}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	offered, err := InstallationCalendars([]map[string]any{installationEntry("federal"), installationEntry("bavaria")}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	reader := WithInstallation{Calendars: calendarStore{office: own}, Installation: offered}

	if found, err := reader.Find(context.Background(), "federal"); err != nil || found.Name != "Ours" {
		t.Fatalf("the workspace's own: %+v, %v", found, err)
	}
	if found, err := reader.Find(context.Background(), "bavaria"); err != nil || found.Name != "Federal" {
		t.Fatalf("the installation's: %+v, %v", found, err)
	}
	if _, err := reader.Find(context.Background(), "nowhere"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a key nobody holds answered %v", err)
	}
}
