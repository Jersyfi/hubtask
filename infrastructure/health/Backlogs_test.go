// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
)

// The report's backlogs are a count somebody took, never a zero standing in for one nobody did
// (P-11): absent until counted, and a dead letter then shows in the figures and as a warning.
func TestADeadLetteredJobAppearsInTheReport(t *testing.T) {
	registry := startedRegistry()
	registry.SetBacklogs(func(context.Context) (port.Backlogs, error) {
		return port.Backlogs{DeadLetterTotal: 1, JobQueueDepth: 4, OutboxPending: 2, OutboxLagSeconds: 3.5}, nil
	})

	if got := registry.Report(context.Background()).Backlogs; !got.SampledAt.IsZero() {
		t.Errorf("before any count the report serves backlogs %+v", got)
	}

	registry.SampleBacklogs(context.Background())
	report := registry.Report(context.Background())
	if report.Backlogs.SampledAt.IsZero() || report.Backlogs.DeadLetterTotal != 1 || report.Backlogs.JobQueueDepth != 4 {
		t.Fatalf("backlogs = %+v, want the count", report.Backlogs)
	}
	var warned *port.Warning
	for i, w := range report.Warnings {
		if w.Code == DeadLettersCode {
			warned = &report.Warnings[i]
		}
	}
	if warned == nil || warned.Severity != "warn" || warned.Params["count"] != "1" {
		t.Errorf("warnings = %+v, want %s at warn with count 1", report.Warnings, DeadLettersCode)
	}
}

// A count that fails keeps the last one rather than replacing it with zeros; no dead letter, no
// warning.
func TestAFailedCountKeepsTheLastOne(t *testing.T) {
	registry := startedRegistry()
	fail := false
	registry.SetBacklogs(func(context.Context) (port.Backlogs, error) {
		if fail {
			return port.Backlogs{}, errors.New("postgres.query_failed")
		}
		return port.Backlogs{JobQueueDepth: 7}, nil
	})
	registry.now = func() time.Time { return time.Unix(100, 0) }
	registry.SampleBacklogs(context.Background())
	fail = true
	registry.SampleBacklogs(context.Background())

	report := registry.Report(context.Background())
	if report.Backlogs.JobQueueDepth != 7 || !report.Backlogs.SampledAt.Equal(time.Unix(100, 0)) {
		t.Errorf("backlogs = %+v, want the last good count", report.Backlogs)
	}
	for _, w := range report.Warnings {
		if w.Code == DeadLettersCode {
			t.Errorf("a warning about dead letters without any: %+v", w)
		}
	}
}

// Held uploads are said on the report, so an installation without Prometheus learns of them from
// the overview rather than from its users (A-23's notice); with scanning switched off they are
// said as critical, since nothing will ever release them.
func TestHeldUploadsAppearInTheReport(t *testing.T) {
	for _, tc := range []struct {
		name     string
		off      bool
		code     string
		severity string
	}{
		{"a scanner that cannot answer", false, UploadsHeldCode, "warn"},
		{"scanning switched off", true, UploadsHeldUnscannedCode, "critical"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := startedRegistry()
			if tc.off {
				registry.ScanningSwitchedOff()
			}
			registry.SetBacklogs(func(context.Context) (port.Backlogs, error) {
				return port.Backlogs{HeldUploadWorkspaces: 2, HeldUploadOldestSeconds: 7260}, nil
			})
			registry.SampleBacklogs(context.Background())

			var warned *port.Warning
			for i, w := range registry.Report(context.Background()).Warnings {
				if w.Code == UploadsHeldCode || w.Code == UploadsHeldUnscannedCode {
					warned = &registry.Report(context.Background()).Warnings[i]
				}
			}
			if warned == nil || warned.Code != tc.code || warned.Severity != tc.severity ||
				warned.Params["workspaces"] != "2" || warned.Params["oldest_minutes"] != "121" {
				t.Errorf("warning = %+v, want %s at %s for 2 workspaces, oldest 121 minutes", warned, tc.code, tc.severity)
			}
		})
	}
	registry := startedRegistry()
	registry.SetBacklogs(func(context.Context) (port.Backlogs, error) { return port.Backlogs{}, nil })
	registry.SampleBacklogs(context.Background())
	for _, w := range registry.Report(context.Background()).Warnings {
		if w.Code == UploadsHeldCode {
			t.Errorf("nothing held, and the report warns %+v", w)
		}
	}
}

func TestAWorkspaceWithoutABackupIsWarnedAbout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		counts port.Backlogs
		warned bool
	}{
		{"every workspace backed up this week", port.Backlogs{}, false},
		{"a workspace with no target", port.Backlogs{BackupWithoutTarget: 1, BackupWithoutRecent: 1}, true},
		{"a target but no backup in seven days", port.Backlogs{BackupWithoutRecent: 3}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := startedRegistry()
			registry.SetBacklogs(func(context.Context) (port.Backlogs, error) { return tc.counts, nil })
			registry.SampleBacklogs(context.Background())
			var warned *port.Warning
			for _, w := range registry.Report(context.Background()).Warnings {
				if w.Code == BackupMissingCode {
					warned = &w
				}
			}
			if (warned != nil) != tc.warned {
				t.Fatalf("warned %+v, want %v", warned, tc.warned)
			}
			if warned != nil && (warned.Severity != "warn" || warned.Params["days"] != "7" ||
				warned.Params["without_recent"] != strconv.FormatInt(tc.counts.BackupWithoutRecent, 10)) {
				t.Errorf("warning = %+v", warned)
			}
		})
	}
}
