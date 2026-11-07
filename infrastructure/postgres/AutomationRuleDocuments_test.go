// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"reflect"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// A relative date on a business calendar is stored and read back with its calendar (UC-AUT-04
// check 9): a document that dropped it would count plain days and close earlier than promised.
func TestARelativeDateKeepsItsCalendarThroughTheStoredDocument(t *testing.T) {
	ids := []shared.ID{
		shared.MustParseID("0190f0a0-0000-7000-8000-000000000001"),
		shared.MustParseID("0190f0a0-0000-7000-8000-000000000002"),
		shared.MustParseID("0190f0a0-0000-7000-8000-000000000003"),
	}
	trigger := domain.Trigger{Kind: domain.TriggerRelativeDate, Anchor: domain.AnchorCompletedAt, Offset: "P5D", Calendar: "office"}
	documents, err := documentsOf(domain.Rule{ID: ids[0], Trigger: trigger})
	if err != nil {
		t.Fatal(err)
	}
	row := sqlc.ListAutomationRulesRow{
		Trigger: documents.trigger, Conditions: documents.conditions, Actions: documents.actions, Throttle: documents.throttle,
		ScopeType: string(domain.ScopeTenant), OnError: string(domain.OnErrorContinue),
	}
	for at, id := range ids {
		into := []*[16]byte{&row.ID.Bytes, &row.TenantID.Bytes, &row.RunAs.Bytes}[at]
		parsed, err := uuidOf(id)
		if err != nil {
			t.Fatal(err)
		}
		*into = parsed.Bytes
	}
	row.ID.Valid, row.TenantID.Valid, row.RunAs.Valid = true, true, true
	row.CreatedBy = row.RunAs
	rule, err := automationRuleFrom(row)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rule.Trigger, trigger) {
		t.Errorf("read back %+v, want %+v", rule.Trigger, trigger)
	}
}
