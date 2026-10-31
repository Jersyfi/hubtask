// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strconv"

	deskrepo "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// InstallationCalendars reads the instance file's `business_calendars` - each in the shape
// `PUT /calendars/{key}` takes: `key`, `name`, `zone`, `hours`, `holidays` - into the calendars every
// workspace falls back to. One malformed entry or a key named twice refuses the whole list, so an
// installation never starts on half of what its operator wrote.
func InstallationCalendars(entries []map[string]any, normalizer text.Normalizer) (map[string]domain.Calendar, error) {
	out := make(map[string]domain.Calendar, len(entries))
	for i, entry := range entries {
		at := "/business_calendars/" + strconv.Itoa(i)
		key, _ := entry["key"].(string)
		name, _ := entry["name"].(string)
		zone, _ := entry["zone"].(string)
		var holidays []string
		if raw, present := entry["holidays"]; present {
			list, isList := raw.([]any)
			if !isList {
				return nil, malformed(at+"/holidays", "calendar.holiday_invalid")
			}
			for _, day := range list {
				date, isText := day.(string)
				if !isText {
					return nil, malformed(at+"/holidays", "calendar.holiday_invalid")
				}
				holidays = append(holidays, date)
			}
		}
		spec, err := SpecFromInput(zone, entry["hours"], holidays)
		if err != nil {
			return nil, err
		}
		cal, err := domain.NewInstallationCalendar(key, name, spec, normalizer)
		if err != nil {
			return nil, err
		}
		if _, twice := out[cal.Key]; twice {
			return nil, malformed(at+"/key", "calendars.key_taken")
		}
		out[cal.Key] = cal
	}
	return out, nil
}

// WithInstallation is the workspace's calendars as the readers of a calendar by key see them -
// planning, capacity and the SLA clocks: the workspace's own of a key and, where it holds none, the
// installation's. Writers keep the workspace's store itself, so defining or deleting a calendar
// never touches the installation's, which only the instance file changes.
type WithInstallation struct {
	deskrepo.Calendars
	Installation map[string]domain.Calendar
}

// Find answers the workspace's calendar of a key, else the installation's.
func (w WithInstallation) Find(ctx context.Context, key string) (domain.Calendar, error) {
	found, err := w.Calendars.Find(ctx, key)
	if errors.Is(err, shared.ErrNotFound) {
		if offered, ok := w.Installation[key]; ok {
			return offered, nil
		}
	}
	return found, err
}
