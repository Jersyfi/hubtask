// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package work holds the model of what is being managed: containers, items, and the policy that
// says what each kind of item can do.
package work

import "slices"

// ItemType is the level of an item. Extensible by design: a new type is a new profile row and an
// adjustment to the permitted children, not a schema change and not an API change
// (domain-model.md §1).
type ItemType string

const (
	ItemTask        ItemType = "TASK"
	ItemWorkPackage ItemType = "WORK_PACKAGE"
	ItemActivity    ItemType = "ACTIVITY"
)

// Capability is one thing an item type may do. The values are the rows of the capability matrix
// in domain-model.md §2.
type Capability string

const (
	CapabilityCompletion   Capability = "COMPLETION"
	CapabilityDueDate      Capability = "DUE_DATE"
	CapabilityReminder     Capability = "REMINDER"
	CapabilityAssignment   Capability = "ASSIGNMENT"
	CapabilityMembers      Capability = "MEMBERS"
	CapabilityBucket       Capability = "BUCKET"
	CapabilityNotes        Capability = "NOTES"
	CapabilityLabels       Capability = "LABELS"
	CapabilityComments     Capability = "COMMENTS"
	CapabilityCover        Capability = "COVER"
	CapabilityAttachments  Capability = "ATTACHMENTS"
	CapabilityHistory      Capability = "HISTORY"
	CapabilityRecurrence   Capability = "RECURRENCE"
	CapabilityCustomFields Capability = "CUSTOM_FIELDS"
)

// CapabilityProfile is what one item type may do, and what may sit underneath it.
//
// It is a policy rather than a rule, which is why it is data: the system ships defaults and a
// tenant may narrow them. Widening past the system boundary is not permitted, and the place that
// enforces that is whatever writes an override - not this type, which describes the profile in
// force rather than deciding it (domain-model.md §2).
type CapabilityProfile struct {
	Type              ItemType
	Capabilities      []Capability
	AllowedChildTypes []ItemType
	// MaxDepth is relative to the collection: how deep a subtree of this type may go.
	MaxDepth int
}

// Allows reports whether the capability is active for this type. Setting a field whose capability
// is not active produces ErrCapabilityNotSupported rather than being ignored - silent ignoring is
// how a client comes to believe it stored something (domain-model.md §2).
func (p CapabilityProfile) Allows(capability Capability) bool {
	return slices.Contains(p.Capabilities, capability)
}

// AllowsChild reports whether an item of the given type may sit directly underneath one of this
// type.
func (p CapabilityProfile) AllowsChild(child ItemType) bool {
	return slices.Contains(p.AllowedChildTypes, child)
}
