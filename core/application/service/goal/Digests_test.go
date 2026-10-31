// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func (m *memory) Digest(_ context.Context, dashboard, account shared.ID) (domain.Digest, error) {
	for _, d := range m.digests {
		if d.DashboardID == dashboard && d.AccountID == account {
			return d, nil
		}
	}
	return domain.Digest{}, shared.ErrNotFound.WithDetail("dashboards.digest_not_found")
}

func (m *memory) DigestByID(_ context.Context, id shared.ID) (domain.Digest, error) {
	d, ok := m.digests[id]
	if !ok {
		return domain.Digest{}, shared.ErrNotFound.WithDetail("dashboards.digest_not_found")
	}
	return d, nil
}

func (m *memory) PutDigest(ctx context.Context, d domain.Digest, _ time.Time) (shared.ID, error) {
	if old, err := m.Digest(ctx, d.DashboardID, d.AccountID); err == nil {
		d.ID, d.LastSentAt = old.ID, old.LastSentAt
	}
	m.digests[d.ID] = d
	return d.ID, nil
}

func (m *memory) DeleteDigest(ctx context.Context, dashboard, account shared.ID) (bool, error) {
	d, err := m.Digest(ctx, dashboard, account)
	if err != nil {
		return false, nil
	}
	delete(m.digests, d.ID)
	return true, nil
}

func (m *memory) DigestSent(_ context.Context, id shared.ID, sent *time.Time, next time.Time) error {
	d := m.digests[id]
	if sent != nil {
		d.LastSentAt = sent
	}
	d.NextAt = next
	m.digests[id] = d
	return nil
}

type queued struct{ requests []queue.Request }

func (q *queued) Enqueue(_ context.Context, r queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, r)
	return "", nil
}

type people map[shared.ID]identity.Account

func (p people) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	a, ok := p[id]
	if !ok {
		return identity.Account{}, shared.ErrNotFound
	}
	return a, nil
}

type titlesOff bool

func (t titlesOff) Find(_ context.Context, accountID shared.ID, _ notification.Category, _ notification.Channel) (notification.Preference, error) {
	if !t {
		return notification.Preference{}, shared.ErrNotFound
	}
	return notification.Preference{AccountID: accountID, Enabled: true, IncludeTitle: false}, nil
}

type address struct{}

func (address) Of(context.Context) (string, error) { return "https://acme.example", nil }

type outbox struct{ sent []mail.Message }

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.sent = append(o.sent, m)
	return nil
}

// echo renders a code with its parameters, so a test reads what reached the message.
type echo struct{}

func (echo) Render(_ string, code string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{code}
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, " ")
}

type later struct{ at time.Time }

func (l later) Now() time.Time { return l.at }

func digestWidgets() []any {
	return []any{
		map[string]any{"title": "Launch", "chart": "NUMBER", "source": "QUERY", "query": map[string]any{
			"scope_container_id": listA.String(), "aggregate": map[string]any{"function": "COUNT", "group_by": "is_completed"},
		}},
		map[string]any{"title": "Merger", "chart": "NUMBER", "source": "QUERY", "x": 6.0, "query": map[string]any{
			"scope_container_id": listS.String(), "aggregate": map[string]any{"function": "COUNT"},
		}},
		map[string]any{"title": "Split", "chart": "PIE", "source": "VIEW", "y": 2.0, "view_id": viewID.String()},
		map[string]any{"title": "Rows", "chart": "TABLE", "source": "QUERY", "y": 4.0, "query": map[string]any{
			"scope_container_id": listA.String(), "aggregate": map[string]any{"function": "COUNT", "group_by": "is_completed"},
		}},
	}
}

// UC-GOL-04 check 9: a person subscribes to a dashboard they see, at an hour of their zone; the
// write schedules its mail, and a dashboard they do not see takes no digest.
func TestADigestIsSetForADashboardTheSubscriberSees(t *testing.T) {
	s, store, _, _ := newSteward(t)
	jobs := &queued{}
	s.Jobs = jobs
	board := ok(t, CreateDashboard{s}.Descriptor(), admin, usecase.Input{"name": "Team", "sharing": "SCOPE", "widgets": digestWidgets()})
	private := ok(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Mine"})
	out := ok(t, SetDashboardDigest{s}.Descriptor(), ben, usecase.Input{
		"dashboard_id": board.String("id"), "cadence": "WEEKLY", "weekday": 1, "hour": 8, "time_zone": "Europe/Berlin",
	})
	// Monday 9 November 2026, 08:00 in Berlin is 07:00 UTC: noon on Monday the 2nd is past the hour.
	if want := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC); !out["next_at"].(time.Time).Equal(want) {
		t.Fatalf("next_at = %v, want %v", out["next_at"], want)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindDashboardDigest ||
		!jobs.requests[0].RunAt.Equal(out["next_at"].(time.Time)) {
		t.Fatalf("jobs = %+v", jobs.requests)
	}
	ok(t, SetDashboardDigest{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id"), "cadence": "DAILY", "hour": 7})
	if len(store.digests) != 1 {
		t.Fatalf("a second write kept %d digests", len(store.digests))
	}
	read := ok(t, ReadDashboardDigest{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id")})
	if read["cadence"] != "DAILY" || read["weekday"] != nil {
		t.Fatalf("read = %v", read)
	}
	if _, err := call(t, SetDashboardDigest{s}.Descriptor(), ben, usecase.Input{
		"dashboard_id": private.String("id"), "cadence": "DAILY", "hour": 7,
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Ben subscribed to Ana's private dashboard: %v", err)
	}
	ok(t, DeleteDashboardDigest{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id")})
	if _, err := call(t, ReadDashboardDigest{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id")}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the digest outlived its deletion: %v", err)
	}
}

// UC-GOL-04 check 10: the mail carries a link and the totals of the NUMBER widgets, computed as
// the subscriber - a widget they cannot compute says so -, names nothing where no title may
// travel, and waits until it is owed.
func TestADigestMailsTheSubscribersOwnTotals(t *testing.T) {
	s, store, _, _ := newSteward(t)
	board := ok(t, CreateDashboard{s}.Descriptor(), admin, usecase.Input{"name": "Team", "sharing": "SCOPE", "widgets": digestWidgets()})
	set := ok(t, SetDashboardDigest{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id"), "cadence": "DAILY", "hour": 7})
	var id shared.ID
	for k := range store.digests {
		id = k
	}
	box := &outbox{}
	runner := DashboardDigests{
		Steward: s, Accounts: people{ben: {ID: ben, Email: "ben@example.com", DisplayName: "Ben", Status: identity.AccountActive}},
		Preferences: titlesOff(false), Addresses: address{}, Mail: box, Renderer: echo{}, Background: inline{},
		FallbackLocale: "en",
	}
	next, err := runner.RunScheduled(context.Background(), tenantID, id)
	if err != nil || len(box.sent) != 0 || !next.Equal(set["next_at"].(time.Time)) {
		t.Fatalf("a digest not yet owed: %v %v %v", next, box.sent, err)
	}
	runner.Steward.Clock = later{set["next_at"].(time.Time)}
	next, err = runner.RunScheduled(context.Background(), tenantID, id)
	if err != nil || len(box.sent) != 1 {
		t.Fatalf("sent = %v %v", box.sent, err)
	}
	m := box.sent[0]
	if m.To != "ben@example.com" || !strings.Contains(m.Subject, "name=Team") {
		t.Fatalf("message = %+v", m)
	}
	for _, want := range []string{
		"email.dashboard_digest.line position=1 title=Launch total=4",
		"email.dashboard_digest.line.unavailable position=2 title=Merger",
		"link=https://acme.example/dashboards/" + board.String("id"),
	} {
		if !strings.Contains(m.Body, want) {
			t.Fatalf("body %q lacks %q", m.Body, want)
		}
	}
	if strings.Contains(m.Body, "Split") {
		t.Fatalf("a chart that is not a number reached the mail: %q", m.Body)
	}
	// UC-GOL-04 check 13: each TABLE widget travels as a CSV file of the subscriber's rows.
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "table-1.csv" || m.Attachments[0].ContentType != "text/csv" ||
		!strings.HasPrefix(string(m.Attachments[0].Content), "group,period,value\r\n") {
		t.Fatalf("attachments = %+v", m.Attachments)
	}
	if !next.After(set["next_at"].(time.Time)) || store.digests[id].LastSentAt == nil {
		t.Fatalf("next = %v, digest = %+v", next, store.digests[id])
	}

	runner.Preferences = titlesOff(true)
	runner.Steward.Clock = later{next}
	if _, err := runner.RunScheduled(context.Background(), tenantID, id); err != nil || len(box.sent) != 2 {
		t.Fatalf("second = %v %v", box.sent, err)
	}
	withheld := box.sent[1]
	if strings.Contains(withheld.Subject+withheld.Body, "Team") || strings.Contains(withheld.Body, "Launch") ||
		!strings.Contains(withheld.Body, "email.dashboard_digest.line.withheld position=1") {
		t.Fatalf("a title travelled: %+v", withheld)
	}
	if len(withheld.Attachments) != 0 {
		t.Fatalf("a table travelled where no title may: %+v", withheld.Attachments)
	}

	delete(store.dashboards, shared.MustParseID(board.String("id")))
	runner.Steward.Clock = later{store.digests[id].NextAt}
	if _, err := runner.RunScheduled(context.Background(), tenantID, id); err != nil || len(box.sent) != 2 {
		t.Fatalf("a digest of a dashboard gone was sent: %v %v", box.sent, err)
	}
	runner.Accounts = people{}
	if next, err := runner.RunScheduled(context.Background(), tenantID, id); err != nil || !next.IsZero() {
		t.Fatalf("the schedule outlived its subscriber: %v %v", next, err)
	}
}

// UC-GOL-04 check 12: a total's fraction is written after the subscriber's separator, exact and
// ungrouped.
func TestADigestTotalReadsInTheSubscribersLocale(t *testing.T) {
	widget := shared.MustParseID("018f0000-0000-7000-8000-0000000000d1")
	board := domain.Dashboard{Widgets: []domain.Widget{{ID: widget, Title: "Hours", Chart: domain.ChartNumber}}}
	figures := usecase.Output{"widgets": []usecase.Output{{"id": widget.String(), "data": []usecase.Output{
		{"value": "1234.50"}, {"value": "0.25"},
	}}}}
	render := func(code string, params map[string]string) string { return echo{}.Render("", code, params) }
	for separator, want := range map[string]string{",": "total=1234,75", ".": "total=1234.75"} {
		if got := digestLines(board, figures, true, separator, render); !strings.Contains(got, want) {
			t.Errorf("with %q the line is %q, want %s", separator, got, want)
		}
	}
}

// UC-GOL-04 check 13: a group a spreadsheet would read as a formula is quoted, a negative figure is
// not, and a widget the subscriber cannot compute has no file.
func TestADigestTableIsANeutralCsv(t *testing.T) {
	first, second := shared.MustParseID("018f0000-0000-7000-8000-0000000000e1"), shared.MustParseID("018f0000-0000-7000-8000-0000000000e2")
	board := domain.Dashboard{Widgets: []domain.Widget{
		{ID: first, Chart: domain.ChartTable, Source: domain.WidgetQuery},
		{ID: second, Chart: domain.ChartTable, Source: domain.WidgetQuery, Y: 1},
	}}
	figures := usecase.Output{"widgets": []usecase.Output{
		{"id": first.String(), "data": []usecase.Output{{"group": "=HYPERLINK(1)", "value": "-2.5"}, {"group": "Done", "count": 3}}},
		{"id": second.String(), "error": "dashboards.widget_forbidden"},
	}}
	files := digestTables(board, figures)
	if len(files) != 1 {
		t.Fatalf("files = %+v", files)
	}
	if got, want := string(files[0].Content), "group,period,value\r\n'=HYPERLINK(1),,-2.5\r\nDone,,3\r\n"; got != want {
		t.Errorf("csv = %q, want %q", got, want)
	}
}
