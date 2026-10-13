// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func TestAnEmergencyAccessTakesAReasonAndLastsAnHour(t *testing.T) {
	at := time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		reason string
		want   string
		code   string
	}{
		{name: "a sentence", reason: "Maria is in hospital", want: "Maria is in hospital"},
		{name: "trimmed", reason: "  the insurance papers \n", want: "the insurance papers"},
		{name: "exactly the bound", reason: strings.Repeat("ä", work.MaxEmergencyReason), want: strings.Repeat("ä", work.MaxEmergencyReason)},
		{name: "empty", reason: "", code: "containers.emergency_reason_required"},
		{name: "blank", reason: " \t\n", code: "containers.emergency_reason_required"},
		{name: "one over, counted in characters", reason: strings.Repeat("ä", work.MaxEmergencyReason+1), code: "containers.emergency_reason_too_long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			access, err := work.NewEmergencyAccess(work.NewEmergencyAccessInput{
				ID: "a", TenantID: "t", HubID: "h", OpenedBy: "o", Reason: tc.reason, At: at,
			})
			if tc.code != "" {
				if !errors.Is(err, shared.ErrValidation) || shared.AsError(err).DetailCode != tc.code {
					t.Fatalf("error %v, want %s", err, tc.code)
				}
				if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Path != "/reason" {
					t.Errorf("fields %+v, want one at /reason", fields)
				}
				return
			}
			if err != nil {
				t.Fatalf("opening: %v", err)
			}
			if access.Reason != tc.want {
				t.Errorf("reason %q, want %q", access.Reason, tc.want)
			}
			if !access.EndsAt.Equal(at.Add(time.Hour)) {
				t.Errorf("ends at %v, want an hour after %v", access.EndsAt, at)
			}
			if !access.RunningAt(at.Add(59*time.Minute)) || access.RunningAt(at.Add(time.Hour)) {
				t.Error("the access does not run for exactly the hour")
			}
		})
	}
}

func TestARunningAccessNamesItsEnd(t *testing.T) {
	end := time.Date(2026, 10, 12, 15, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	err := work.ErrEmergencyRunning(end)
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).Params["ends_at"] != "2026-10-12T13:30:00Z" {
		t.Errorf("refusal %v %+v, want a conflict naming the end in UTC", err, shared.AsError(err).Params)
	}
	if !errors.Is(work.ErrPrivateHubsOff(), shared.ErrForbidden) {
		t.Error("the switched-off refusal is not a 403")
	}
}
