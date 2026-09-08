// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTheLogLineCarriesNoMeasurement(t *testing.T) {
	lag, wait := 12.5, 3.25
	ev := evidence{
		RunID:      "20260907-100405",
		Result:     resultFail,
		FinishedAt: time.Date(2026, 9, 7, 10, 20, 0, 0, time.UTC),
		RPO:        rpoSample{ArchiveLagAtDrillSeconds: &lag, WALArchiveWaitSeconds: &wait},
		RTO:        rtoSample{RestoreSeconds: 187.5, VerificationSeconds: 4.75},
		Checks: []check{
			{Name: "recovery_target_between_the_markers", OK: true},
			{Name: "tenant_sees_only_itself", OK: false, Detail: "foreign rows 3"},
		},
		Error: "a check failed",
	}

	var rendered []string
	for _, attr := range ev.redacted("s3:prod/restore-drill/x.json") {
		rendered = append(rendered, attr.Key+"="+attr.Value.String())
	}
	line := strings.Join(rendered, " ")

	for _, forbidden := range []string{"12.5", "3.25", "187", "4.75"} {
		if strings.Contains(line, forbidden) {
			t.Errorf("the log line carries a measurement: %s", line)
		}
	}
	for _, wanted := range []string{"result=fail", "checks=1/2", "failed_check=tenant_sees_only_itself", "error_code=restore_drill.failed"} {
		if !strings.Contains(line, wanted) {
			t.Errorf("the log line lacks %s: %s", wanted, line)
		}
	}
	if strings.Contains(line, "foreign rows") {
		t.Error("a check's detail reached the log line")
	}
}

func TestPassedNeedsEveryCheckAndAtLeastOne(t *testing.T) {
	if (evidence{}).passed() {
		t.Error("no checks at all passed")
	}
	if !(evidence{Checks: []check{{OK: true}, {OK: true}}}).passed() {
		t.Error("two green checks did not pass")
	}
	if (evidence{Checks: []check{{OK: true}, {OK: false}}}).passed() {
		t.Error("a red check passed")
	}
}

func TestTheSummaryIsARecordWithoutNumbers(t *testing.T) {
	ev := evidence{RunID: "r", Result: resultPass, Release: "0.6.0",
		FinishedAt: time.Date(2026, 9, 7, 10, 20, 0, 0, time.UTC),
		RTO:        rtoSample{RestoreSeconds: 99}}
	summary := ev.summary()
	if !strings.Contains(summary, `"finished_at":"2026-09-07T10:20:00Z"`) || !strings.Contains(summary, `"result":"pass"`) {
		t.Errorf("summary %s", summary)
	}
	if strings.Contains(summary, "99") {
		t.Errorf("the summary carries a measurement: %s", summary)
	}
	if lvl := resultLevel(nil); lvl != slog.LevelInfo {
		t.Errorf("a pass logs at %v", lvl)
	}
}
