// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "testing"

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
