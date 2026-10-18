// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// UC-INS-14 checks 4 and 8 (multi-tenancy.md §4.3): the use per workspace, metric and day, only
// while metering is on; a day without a row carries the closing count forward for the counts kept as
// the day's highest and is 0 for the sums; every metric says its day; the answer says up to which
// day it is final; the range and the metric are bounded; pages of workspaces.

type ledgerFake struct {
	rows  []adminrepo.UsageRow
	asked adminrepo.UsageQuery
}

func (l *ledgerFake) CursorAfter(last shared.ID) string { return "after:" + string(last) }

func (l *ledgerFake) Read(_ context.Context, query adminrepo.UsageQuery) ([]adminrepo.UsageRow, error) {
	l.asked = query
	return l.rows, nil
}

var (
	usageA = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	usageB = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
)

func day(text string) time.Time {
	parsed, _ := time.Parse(time.DateOnly, text)
	return parsed
}

func closing(value int64) *int64 { return &value }

func usageReader(rows ...adminrepo.UsageRow) (ReadUsage, *instanceStore, *ledgerFake) {
	writer, settings, _ := newInstanceWriter(newRegister(operatorID))
	settings.level.Metering = true
	ledger := &ledgerFake{rows: rows}
	return ReadUsage{Writer: writer, Ledger: ledger, Tenancy: env.TenancyMulti}, settings, ledger
}

func TestTheUseIsReadOnlyWhileMeteringIsOn(t *testing.T) {
	reader, settings, _ := usageReader()
	settings.level.Metering = false
	_, err := reader.Execute(t.Context(), operator(), UsageQuery{From: day("2026-10-01"), To: day("2026-10-02")})
	if got := shared.AsError(err); got == nil || got.DetailCode != "admin.metering_off" {
		t.Fatalf("a read with metering off answered %v, want admin.metering_off", err)
	}
	reader.Tenancy = env.TenancySingle
	settings.level.Metering = true
	if _, err := reader.Execute(t.Context(), operator(), UsageQuery{From: day("2026-10-01"), To: day("2026-10-02")}); err == nil {
		t.Error("single mode answered the use")
	}
}

func TestADayWithoutARowCarriesTheClosingCountOrIsNothing(t *testing.T) {
	reader, _, _ := usageReader(
		adminrepo.UsageRow{Tenant: usageA, Day: day("2026-09-28"), Metric: "accounts", Value: 9, Closing: closing(6)},
		adminrepo.UsageRow{Tenant: usageA, Day: day("2026-10-02"), Metric: "accounts", Value: 8, Closing: closing(7)},
		adminrepo.UsageRow{Tenant: usageA, Day: day("2026-10-02"), Metric: "ai_tokens_per_day", Value: 500},
	)
	page, err := reader.Execute(t.Context(), operator(), UsageQuery{From: day("2026-10-01"), To: day("2026-10-03")})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Workspaces) != 1 {
		t.Fatalf("answered %d workspaces", len(page.Workspaces))
	}
	people := page.Workspaces[0].Metrics["accounts"]
	if len(people) != 3 || people[0].Value != 6 || people[1].Value != 8 || people[2].Value != 7 {
		t.Errorf("people read %+v, want 6 carried, the day's highest 8, then 7 carried", people)
	}
	tokens := page.Workspaces[0].Metrics["ai_tokens_per_day"]
	if tokens[0].Value != 0 || tokens[1].Value != 500 || tokens[2].Value != 0 {
		t.Errorf("tokens read %+v, want 0, 500, 0", tokens)
	}
	for _, metric := range page.Metrics {
		if metric.Name == "ai_tokens_per_day" && metric.Day != "WORKSPACE" || metric.Name == "items" && metric.Day != "UTC" {
			t.Errorf("%s counts by %s", metric.Name, metric.Day)
		}
	}
}

func TestTheReadSaysUpToWhichDayItIsFinal(t *testing.T) {
	for _, testCase := range []struct{ now, want string }{
		{"2026-10-18T13:00:00Z", "2026-10-17"},
		{"2026-10-18T11:00:00Z", "2026-10-16"},
	} {
		now, _ := time.Parse(time.RFC3339, testCase.now)
		if got := finalThrough(now).Format(time.DateOnly); got != testCase.want {
			t.Errorf("at %s final through %s, want %s", testCase.now, got, testCase.want)
		}
	}
}

func TestTheRangeAndTheMetricAreBounded(t *testing.T) {
	reader, _, _ := usageReader()
	for _, testCase := range []struct {
		name  string
		query UsageQuery
		code  string
	}{
		{"backwards", UsageQuery{From: day("2026-10-02"), To: day("2026-10-01")}, "admin.usage_range_invalid"},
		{"too long", UsageQuery{From: day("2025-01-01"), To: day("2026-10-01")}, "admin.usage_range_invalid"},
		{"an unknown metric", UsageQuery{From: day("2026-10-01"), To: day("2026-10-01"), Metric: "invoices"}, "admin.usage_metric_unknown"},
	} {
		_, err := reader.Execute(t.Context(), operator(), testCase.query)
		if got := shared.AsError(err); got == nil || got.DetailCode != testCase.code {
			t.Errorf("%s answered %v, want %s", testCase.name, err, testCase.code)
		}
	}
}

func TestWorkspacesComeAPageAtATime(t *testing.T) {
	reader, _, ledger := usageReader(
		adminrepo.UsageRow{Tenant: usageA, Day: day("2026-10-01"), Metric: "items", Value: 3, Closing: closing(3)},
		adminrepo.UsageRow{Tenant: usageB, Day: day("2026-10-01"), Metric: "items", Value: 4, Closing: closing(4), Purged: true},
	)
	page, err := reader.Execute(t.Context(), operator(), UsageQuery{From: day("2026-10-01"), To: day("2026-10-01"), Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ledger.asked.Size != 2 || len(page.Workspaces) != 1 || !page.More || page.Next == "" {
		t.Fatalf("a page of one answered %d workspaces, more %v, next %q (asked %d)",
			len(page.Workspaces), page.More, page.Next, ledger.asked.Size)
	}
	if page.Next != "after:"+string(usageA) {
		t.Errorf("the cursor reads %q, want the last workspace answered", page.Next)
	}
}

// A purged workspace bills nothing after its purge day: the purge closed its counts at 0.
func TestAPurgedWorkspaceCountsNothingAfterItsPurge(t *testing.T) {
	reader, _, _ := usageReader(
		adminrepo.UsageRow{Tenant: usageB, Day: day("2026-09-30"), Metric: "items", Value: 5, Closing: closing(5), Purged: true},
		adminrepo.UsageRow{Tenant: usageB, Day: day("2026-10-02"), Metric: "items", Value: 5, Closing: closing(0), Purged: true},
	)
	page, err := reader.Execute(t.Context(), operator(), UsageQuery{From: day("2026-10-01"), To: day("2026-10-04"), Metric: "items"})
	if err != nil {
		t.Fatal(err)
	}
	items := page.Workspaces[0].Metrics["items"]
	if !page.Workspaces[0].Purged || items[0].Value != 5 || items[1].Value != 5 || items[2].Value != 0 || items[3].Value != 0 {
		t.Errorf("the purged workspace reads %+v, want 5, 5 on the purge day, then 0", items)
	}
}
