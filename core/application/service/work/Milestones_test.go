// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// milestones answers whether the collection and the hub are finished as the test arranges, and
// records what the writer asked, in order.
type milestones struct {
	collectionDone, hubDone bool
	asked                   []string
	reached                 map[shared.ID]time.Time
}

func (m *milestones) LockHub(_ context.Context, hub shared.ID) error {
	m.asked = append(m.asked, "lock "+hub.String())
	return nil
}

func (m *milestones) RecordCollection(_ context.Context, collection shared.ID, _ time.Time) (bool, error) {
	m.asked = append(m.asked, "collection "+collection.String())
	return m.collectionDone, nil
}

func (m *milestones) RecordHub(_ context.Context, hub shared.ID, _ time.Time) (bool, error) {
	m.asked = append(m.asked, "hub "+hub.String())
	return m.hubDone, nil
}

func (m *milestones) Of(_ context.Context, ids []shared.ID) (map[shared.ID]time.Time, error) {
	found := map[shared.ID]time.Time{}
	for _, id := range ids {
		if at, held := m.reached[id]; held {
			found[id] = at
		}
	}
	return found, nil
}

func TestACompletionAsksAfterTheMilestoneOfItsCollectionAndThenItsHub(t *testing.T) {
	for name, tc := range map[string]struct {
		collectionDone bool
		want           []string
	}{
		// The hub is asked only once its collection is finished: a hub with an open entry in this
		// collection cannot be.
		"something still open": {want: []string{"lock " + hubID.String(), "collection " + collectionID.String()}},
		"the collection finished": {collectionDone: true, want: []string{
			"lock " + hubID.String(), "collection " + collectionID.String(), "hub " + hubID.String(),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newCompletionHarness(domain.CompletionManual)
			recorded := &milestones{collectionDone: tc.collectionDone}
			h.writer.Milestones = recorded

			if _, err := (CompleteWorkItem{Completion: h.writer}).
				Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
				t.Fatalf("completing: %v", err)
			}
			if len(recorded.asked) != len(tc.want) {
				t.Fatalf("asked %v, want %v", recorded.asked, tc.want)
			}
			for i := range tc.want {
				if recorded.asked[i] != tc.want[i] {
					t.Fatalf("asked %v, want %v", recorded.asked, tc.want)
				}
			}
		})
	}
}

func TestOnlyACompletionThatMovedSomethingAsksAfterAMilestone(t *testing.T) {
	// Reopening leaves something open, and a repeat completes nothing: neither can finish anything.
	h := newCompletionHarness(domain.CompletionManual)
	recorded := &milestones{collectionDone: true, hubDone: true}
	h.writer.Milestones = recorded
	h.alreadyDone(taskID)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
		t.Fatalf("completing again: %v", err)
	}
	if _, err := (ReopenWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if len(recorded.asked) != 0 {
		t.Errorf("asked %v after a repeat and a reopening", recorded.asked)
	}
}

func TestAContainerReadAnswersItsMilestone(t *testing.T) {
	reached := time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)
	store := &containers{stored: map[shared.ID]domain.Container{
		hubID: {ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Renovation", OrderKey: "a0", Version: 1},
		collectionID: {
			ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
			Name: "Kitchen", OrderKey: "a0", Version: 1,
		},
	}}
	recorded := &milestones{reached: map[shared.ID]time.Time{collectionID: reached}}
	get := GetContainer{Containers: store, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{}, Milestones: recorded}

	for id, want := range map[shared.ID]bool{collectionID: true, hubID: false} {
		out, err := get.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"container_id": id.String()})
		if err != nil {
			t.Fatalf("reading %s: %v", id, err)
		}
		milestone, held := out["milestone"].(map[string]any)
		if held != want {
			t.Fatalf("%s answered a milestone: %v, want %v", id, held, want)
		}
		if held && milestone["reached_at"] != reached {
			t.Errorf("reached at %v, want %v", milestone["reached_at"], reached)
		}
	}

	// Without the repository the read answers none, as a build that did not wire it.
	get.Milestones = nil
	out, err := get.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"container_id": collectionID.String()})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if _, held := out["milestone"]; held {
		t.Error("a read without the repository answered a milestone")
	}
}
