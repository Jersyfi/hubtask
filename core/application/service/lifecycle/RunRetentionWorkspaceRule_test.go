// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
)

// askedRows records the cutoff a sweep asked for and removes everything it is asked to.
type askedRows struct {
	asked time.Time
	due   int
}

func (r *askedRows) CountExpired(_ context.Context, cutoff time.Time, ceiling int) (int, error) {
	r.asked = cutoff
	return min(r.due, ceiling), nil
}

func (r *askedRows) DeleteExpired(_ context.Context, cutoff time.Time, batch int) (int, error) {
	r.asked = cutoff
	n := min(r.due, batch)
	r.due -= n
	return n, nil
}

// Third answers 10 (UC-LIF-04): a kind with a default period is swept at the workspace's
// TENANT-scope rule, raised to the kind's lower bound; a rule that only announces or is switched
// off removes nothing; without a rule the shipped default holds.
func TestADefaultPeriodKindReadsTheWorkspacesRule(t *testing.T) {
	type wire func(*RunRetention, ExpiringRows)
	kinds := []struct {
		kind domain.DataKind
		wire wire
	}{
		{domain.KindDocumentHistory, func(r *RunRetention, rows ExpiringRows) { r.DocumentHistory = rows }},
		{domain.KindAgentLedger, func(r *RunRetention, rows ExpiringRows) { r.AgentLedger = rows }},
		{domain.KindAgentRequest, func(r *RunRetention, rows ExpiringRows) { r.AgentRequests = rows }},
		{domain.KindAgentJob, func(r *RunRetention, rows ExpiringRows) { r.AgentJobs = rows }},
		{domain.KindRequester, func(r *RunRetention, rows ExpiringRows) { r.Requesters = rows }},
		{domain.KindMailReceipt, func(r *RunRetention, rows ExpiringRows) { r.MailReceipts = rows }},
	}
	for _, each := range kinds {
		floor := domain.FloorFor(each.kind)
		var fallback int
		for _, policy := range domain.DefaultPolicies() {
			if policy.DataKind == each.kind {
				fallback = policy.RetainDays
			}
		}
		rule := func(days int, action domain.Action, enabled bool) *ruleStore {
			return &ruleStore{stored: []domain.Rule{{
				ID: holdID, Scope: domain.Scope{Kind: domain.ScopeTenant}, DataKind: each.kind,
				RetainDays: days, Action: action, Enabled: enabled,
			}}}
		}
		for _, tc := range []struct {
			name  string
			rules *ruleStore
			want  int // days back from now; -1 is "nothing swept"
		}{
			{"a longer rule", rule(fallback+400, domain.ActionHardDelete, true), fallback + 400},
			{"a rule below the bound", rule(floor/2, domain.ActionHardDelete, true), max(floor, floor/2)},
			{"a rule that only announces", rule(30, domain.ActionNotifyOnly, true), -1},
			{"a rule switched off", rule(30, domain.ActionHardDelete, false), -1},
			{"no rule", &ruleStore{}, fallback},
		} {
			t.Run(string(each.kind)+"/"+tc.name, func(t *testing.T) {
				h := newRunHarness()
				rows := &askedRows{due: 2}
				each.wire(&h.run, rows)
				h.run.Documents = DocumentSweep{Rules: tc.rules}
				if _, err := h.run.Execute(t.Context(), actor()); err != nil {
					t.Fatalf("the run failed: %v", err)
				}
				if tc.want < 0 {
					if rows.due != 2 || !rows.asked.IsZero() {
						t.Fatalf("%d of 2 left, asked for %v", rows.due, rows.asked)
					}
					return
				}
				if want := now.AddDate(0, 0, -tc.want); !rows.asked.Equal(want) {
					t.Errorf("the cutoff was %v, want %v", rows.asked, want)
				}
				if rows.due != 0 {
					t.Errorf("%d of 2 left", rows.due)
				}
			})
		}
	}
}
