// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The matrix of domain-model.md §2, as far as the profile type is concerned: what a type allows
// and what may sit under it.
func TestAProfileAnswersItsMatrixRow(t *testing.T) {
	task := CapabilityProfile{
		Type: ItemTask,
		Capabilities: []Capability{
			CapabilityCompletion, CapabilityBucket, CapabilityCover, CapabilityRecurrence,
		},
		AllowedChildTypes: []ItemType{ItemWorkPackage},
		MaxDepth:          3,
	}

	for _, capability := range task.Capabilities {
		if !task.Allows(capability) {
			t.Errorf("%s is in the profile and not allowed", capability)
		}
	}
	if task.Allows(CapabilityComments) {
		t.Error("a capability outside the profile is allowed")
	}
	if !task.AllowsChild(ItemWorkPackage) {
		t.Error("a work package may not sit under a task")
	}
	if task.AllowsChild(ItemActivity) {
		t.Error("an activity may sit directly under a task")
	}
}

// The reduced level: an activity ends the hierarchy, so nothing may sit under it.
func TestAProfileWithoutChildrenEndsTheHierarchy(t *testing.T) {
	activity := CapabilityProfile{
		Type:         ItemActivity,
		Capabilities: []Capability{CapabilityCompletion},
		MaxDepth:     1,
	}

	for _, child := range []ItemType{ItemTask, ItemWorkPackage, ItemActivity} {
		if activity.AllowsChild(child) {
			t.Errorf("%s may sit under an activity", child)
		}
	}
}

// The zero profile allows nothing. It is what a missing row would produce, and "allows nothing"
// is the only safe reading of that.
func TestTheZeroProfileAllowsNothing(t *testing.T) {
	var empty CapabilityProfile

	if empty.Allows(CapabilityCompletion) || empty.AllowsChild(ItemTask) {
		t.Error("the zero profile allows something")
	}
}

// The rule of ADR-0006: a field whose capability is not in the profile is refused, not ignored.
// The refusal carries its own code, because the client's fix is to change the type it writes to
// rather than the value it sent.
func TestACapabilityOutsideTheProfileIsRefusedRatherThanIgnored(t *testing.T) {
	activity := CapabilityProfile{
		Type:         ItemActivity,
		Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate},
		MaxDepth:     1,
	}

	if err := activity.Require(CapabilityDueDate, "/due_at"); err != nil {
		t.Errorf("a capability in the profile was refused: %v", err)
	}

	err := activity.Require(CapabilityNotes, "/notes")
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error = %v, want capability_not_supported", err)
	}

	refusal := shared.AsError(err)
	if refusal.Params["item_type"] != string(ItemActivity) {
		t.Errorf("item_type = %q", refusal.Params["item_type"])
	}
	if refusal.Params["capability"] != string(CapabilityNotes) {
		t.Errorf("capability = %q", refusal.Params["capability"])
	}
	// Without the pointer the client knows a field was wrong and not which one - and a create
	// request carries several capability-dependent fields at once.
	if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/notes" {
		t.Errorf("field errors = %v", refusal.Fields)
	}
}
