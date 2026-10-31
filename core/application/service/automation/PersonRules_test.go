// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func personalCommand(owner shared.ID) CreateRuleCommand {
	cmd := validCommand()
	cmd.Name = "Assigned to me goes on my Today"
	cmd.Scope = domain.Scope{Type: domain.ScopePerson, ID: owner}
	cmd.RunAs = owner
	cmd.Trigger = domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemAssigned}
	return cmd
}

// UC-AUT-01 check 10: a member keeps a rule of their own without the automation permission; it
// acts as them, and nobody else - an administrator included - reads, lists or changes it.
func TestAPersonsOwnRuleIsTheirsAlone(t *testing.T) {
	h := newHarness()
	h.auth.refuse = true // the member holds no automation permission anywhere
	owner := writerActor()
	rule, err := CreateRule{Writer: h.writer}.Execute(context.Background(), owner, personalCommand(writerID))
	if err != nil {
		t.Fatalf("the member's own rule was refused: %v", err)
	}
	if rule.RunAs != writerID || len(h.auth.requests) != 0 {
		t.Fatalf("run_as %s, %d permission questions; want the owner and none", rule.RunAs, len(h.auth.requests))
	}
	if _, err := (GetRule{Writer: h.writer}).Execute(context.Background(), owner, rule.ID); err != nil {
		t.Errorf("the owner cannot read their rule: %v", err)
	}

	h.auth.refuse = false // an administrator, who may manage every rule of the workspace
	admin := owner
	admin.AccountID = personID
	if _, err := (GetRule{Writer: h.writer}).Execute(context.Background(), admin, rule.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another person read it: %v, want not found", err)
	}
	page, err := ListRules{Writer: h.writer}.Execute(context.Background(), admin, repository.Query{})
	if err != nil || len(page.Rules) != 0 {
		t.Errorf("another person's list holds %d rules (%v), want none", len(page.Rules), err)
	}
	if err := (DeleteRule{Writer: h.writer}).Execute(context.Background(), admin, rule.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another person deleted it: %v, want not found", err)
	}
	if _, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), admin, personalCommand(writerID)); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a rule was written into another person's own: %v, want not found", err)
	}
	agent := owner
	agent.Kind = shared.ActorAIAgent
	if _, err := (GetRule{Writer: h.writer}).Execute(context.Background(), agent, rule.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an agent read the person's rule: %v, want not found", err)
	}
}

// UC-AUT-01 check 11: a person's rule fires only on an entry its owner may read, and never on an
// event with no entry's place.
func TestAPersonsRuleFiresOnlyWhereItsOwnerReads(t *testing.T) {
	personal := ruleAt(domain.Scope{Type: domain.ScopePerson, ID: personID}, ruleID)
	personal.RunAs = personID
	for name, tc := range map[string]struct {
		reads bool
		event event.Envelope
		want  int
	}{
		"an entry they read":       {reads: true, event: itemEvent(), want: 1},
		"an entry they cannot see": {reads: false, event: itemEvent()},
		"no place at all": {reads: true, event: event.Envelope{
			ID: itemEvent().ID, Type: event.ItemUpdated, TenantID: tenant, OccurredAt: now,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			matcher, queued, _ := newMatcher([]domain.Rule{personal})
			readers := &authorizer{refuse: !tc.reads}
			matcher.Readers = readers
			if err := matcher.Deliver(context.Background(), tc.event); err != nil {
				t.Fatal(err)
			}
			if len(queued.queued) != tc.want {
				t.Fatalf("queued %d, want %d", len(queued.queued), tc.want)
			}
			if tc.want == 1 && (len(readers.requests) != 1 || readers.requests[0].Permission != "READ") {
				t.Errorf("asked %+v, want READ at the entry's place", readers.requests)
			}
		})
	}
	matcher, queued, _ := newMatcher([]domain.Rule{personal})
	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil || len(queued.queued) != 0 {
		t.Errorf("without a reader's question a person's rule fired (%v)", err)
	}
}
