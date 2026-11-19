// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// keptStore answers fixed rows and remembers whom and which window it was asked about.
type keptStore struct {
	rows     []domain.KeptRow
	account  shared.ID
	from, to time.Time
}

func (k *keptStore) Kept(_ context.Context, account shared.ID, from, to time.Time) ([]domain.KeptRow, error) {
	k.account, k.from, k.to = account, from, to
	return k.rows, nil
}

// UC-SVC-02 check 15: a handler reads, per kind of deadline, how many of those on their own
// entries ended in the window and how many they kept - always their own, never another person's,
// over the last 30 days unless they ask for another span - without the report's permission.
func TestAHandlerReadsTheDeadlinesTheyKept(t *testing.T) {
	network := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007d1")
	store := &keptStore{rows: []domain.KeptRow{
		{Goal: domain.GoalFirstResponse, Ended: 4, Kept: 3},
		{OLA: true, Group: network, Goal: domain.GoalResolution, Ended: 2, Kept: 2},
	}}
	authorizer := &gate{allow: true}
	use := ReadMyDeadlinesKept{Desk: Desk{Authorizer: authorizer, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday)}, Kept: store}
	out, err := use.Descriptor().Handler.Invoke(context.Background(), reader(), usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if store.account != reader().AccountID || !store.to.Equal(monday) || !store.from.Equal(monday.AddDate(0, 0, -30)) {
		t.Fatalf("asked about %s over %s - %s", store.account, store.from, store.to)
	}
	if len(authorizer.asked) != 1 || !authorizer.asked[0].AnyMember || authorizer.asked[0].Permission != "" {
		t.Errorf("asked %+v, want a member's question and no permission", authorizer.asked)
	}
	data := out["data"].([]usecase.Output)
	if len(data) != 2 || data[0]["kind"] != "SLA" || data[0]["group_id"] != nil || data[0]["kept"] != 3 ||
		data[1]["kind"] != "OLA" || data[1]["group_id"] != network.String() || data[1]["goal"] != "RESOLUTION" {
		t.Errorf("answered %v", data)
	}

	if _, err := use.Descriptor().Handler.Invoke(context.Background(), reader(), usecase.Input{"days": 7}); err != nil {
		t.Fatal(err)
	}
	if !store.from.Equal(monday.AddDate(0, 0, -7)) {
		t.Errorf("a week asked reads from %s", store.from)
	}
	for _, days := range []int{0, 366} {
		if _, err := use.Execute(context.Background(), reader(), days); err == nil {
			t.Errorf("a span of %d days was read", days)
		}
	}
	refused := ReadMyDeadlinesKept{Desk: Desk{Authorizer: &gate{}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday)}, Kept: store}
	if _, err := refused.Execute(context.Background(), reader(), 30); err == nil {
		t.Error("somebody who is no member read their deadlines")
	}
}
