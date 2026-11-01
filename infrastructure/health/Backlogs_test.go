// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"errors"
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
