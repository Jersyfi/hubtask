// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type clearedMarks struct{ cleared []shared.ID }

func (c *clearedMarks) Clear(_ context.Context, account shared.ID) error {
	c.cleared = append(c.cleared, account)
	return nil
}

func TestAPersonClearsTheirOwnUndeliverableMark(t *testing.T) {
	marks := &clearedMarks{}
	sink := &auditSink{}
	use := ClearOwnMailUndeliverable{Marks: marks, Audit: sink, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now)}
	actor := appshared.ActorContext{
		TenantID:  shared.ID("01936f2a-7c1e-7000-8000-000000000001"),
		AccountID: shared.ID("01936f2a-7c1e-7000-8000-000000000002"),
		Kind:      appshared.ActorUser,
		Scopes:    []string{accountsWrite},
	}
	if err := use.Execute(t.Context(), actor); err != nil {
		t.Fatal(err)
	}
	if len(marks.cleared) != 1 || marks.cleared[0] != actor.AccountID {
		t.Errorf("cleared %v, want the caller's own", marks.cleared)
	}
	if len(sink.entries) != 1 || sink.entries[0].Action != MailUndeliverableClearedAction {
		t.Errorf("audit %+v, want one clear recorded", sink.entries)
	}
	if err := use.Execute(t.Context(), appshared.ActorContext{TenantID: actor.TenantID, Kind: appshared.ActorSystem, Scopes: actor.Scopes}); err == nil {
		t.Error("an actor without an account cleared a mark")
	}
}
