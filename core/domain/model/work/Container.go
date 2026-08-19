// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ContainerType is the level of a container: a hub holds collections, a collection holds work
// items. The two levels are one type rather than two entities for the same reason the item levels
// are (domain-model.md §1) - one table, one repository, one set of use cases.
type ContainerType string

const (
	ContainerHub        ContainerType = "HUB"
	ContainerCollection ContainerType = "COLLECTION"
)

// containerTypes is the closed set, in the order of the constants above.
var containerTypes = [...]ContainerType{ContainerHub, ContainerCollection}

// ContainerTypes returns every defined type. An adapter uses it to prove it handles all of them.
func ContainerTypes() []ContainerType { return containerTypes[:] }

// Valid reports whether the type is one of the defined ones.
func (t ContainerType) Valid() bool {
	for _, known := range containerTypes {
		if known == t {
			return true
		}
	}
	return false
}

// AllowsChild is invariant I-C1 read downwards: a hub takes collections, a collection takes work
// items and no further containers (domain-model.md §3.3).
func (t ContainerType) AllowsChild(child ContainerType) bool {
	return t == ContainerHub && child == ContainerCollection
}

// MaxContainerNameLength counts code points rather than bytes. A limit in bytes would accept
// "Übersicht" and reject the same name written with a combining diaeresis, which is not a
// distinction a user can see (domain-model.md §3.3, I-W7).
const MaxContainerNameLength = 200

// Container is a hub or a collection (domain-model.md §3.3).
//
// Of the `policies` column's four documented keys, one is here: the completion policy, because B-07
// reads it. The other three - the default bucket, capability overrides, automatic assignment - stay
// absent on the reasoning that kept this one absent until now: a field nothing reads and nothing
// writes is a promise nothing keeps. They arrive with the use cases that own them.
//
// UpdateContainerPolicies writes the completion policy, and a collection that has never been
// configured reads as the default - which is why the column starting as `{}` is a special case
// nowhere above the adapter.
type Container struct {
	ID       shared.ID
	TenantID shared.ID
	Type     ContainerType
	// ParentID is the hub a collection sits in, and empty for a hub (I-C1).
	ParentID shared.ID
	Name     string
	// Description, Icon and ColorToken are optional; the empty string is "not set" rather than a
	// value, which is what lets the adapter store NULL without a second flag per field.
	Description string
	Icon        string
	ColorToken  string
	// OrderKey ranks the container among its siblings. A lexicographic key rather than a number,
	// so an insertion between two neighbours renumbers nothing (offline-sync.md §4.2).
	OrderKey string
	// ArchivedAt and DeletedAt are the lifecycle, as timestamps rather than as a status column:
	// restoring then moves no data (domain-model.md §6).
	ArchivedAt *time.Time
	DeletedAt  *time.Time
	// ParentArchivedAt is the hub's own stamp, read alongside the row rather than stored on it. That
	// is invariant I-C3's second half: a collection in an archived hub is read-only without being
	// archived itself, and the two facts stay separate so that unarchiving the hub restores exactly
	// what it covered - a collection archived in its own right stays archived.
	//
	// Denormalising it onto the collection would mean archiving a hub wrote every collection under it,
	// and unarchiving could no longer tell which of them had been archived before.
	ParentArchivedAt *time.Time
	// TrashBatchID ties every container trashed in one operation together, so that restoring a
	// subtree is one decision rather than a walk (I-C2). Set by TrashContainer, which arrives
	// with its own use case.
	TrashBatchID shared.ID
	CreatedBy    shared.ID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// CompletionPolicy decides whether a child's completion propagates upwards (I-W5). Only a
	// collection carries a meaningful one - a hub holds no items - and it is read off both, because
	// the column is on both and a reader that skipped hubs would be a second rule to keep.
	CompletionPolicy CompletionPolicy
	// Version is the optimistic lock. It starts at 1, which is what the column default says, so
	// that a freshly created container has an ETag before it has been read back.
	Version int
}

// NewContainerInput is what a container is made of. A struct rather than eleven parameters,
// because eleven parameters of which four are identifiers is an invitation to swap two of them.
type NewContainerInput struct {
	ID       shared.ID
	TenantID shared.ID
	Type     ContainerType
	ParentID shared.ID
	Name     string

	Description string
	Icon        string
	ColorToken  string

	OrderKey  string
	CreatedBy shared.ID
	Now       time.Time
}

// NewContainer builds a container and checks its invariants (project-structure.md §3:
// constructors check, callers do not).
//
// The name is trimmed here rather than in the adapter, because the trimmed form is the one the
// uniqueness check must see: " Team" and "Team" are the same name to a person, and a check that
// disagrees with a person is a bug report waiting.
func NewContainer(in NewContainerInput) (Container, error) {
	if !in.Type.Valid() {
		return Container{}, shared.ErrValidation.
			WithDetail("containers.type_unknown").
			WithParams(map[string]string{"value": string(in.Type)}).
			WithFields(shared.FieldError{Path: "/type", Code: "containers.type_unknown"})
	}

	name, err := containerName(in.Name)
	if err != nil {
		return Container{}, err
	}

	if err := checkParent(in.Type, in.ParentID); err != nil {
		return Container{}, err
	}

	// The identifiers and the rank come from ports, not from a client. Missing means the use case
	// was wired wrong, which is a defect rather than something the caller can fix (security.md §9).
	if in.ID.IsZero() || in.TenantID.IsZero() || in.CreatedBy.IsZero() || in.OrderKey == "" {
		return Container{}, shared.ErrInternal.WithDetail("containers.identity_incomplete")
	}

	return Container{
		ID:          in.ID,
		TenantID:    in.TenantID,
		Type:        in.Type,
		ParentID:    in.ParentID,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Icon:        strings.TrimSpace(in.Icon),
		ColorToken:  strings.TrimSpace(in.ColorToken),
		OrderKey:    in.OrderKey,
		// Not a parameter: a new collection has no policy configured, and the policy it behaves as is
		// the default until UpdateContainerPolicies (B-06) says otherwise. Setting it here rather than
		// leaving the zero value means nothing above this constructor has to know that "" and MANUAL
		// are the same thing.
		CompletionPolicy: DefaultCompletionPolicy,
		CreatedBy:        in.CreatedBy,
		CreatedAt:        in.Now,
		UpdatedAt:        in.Now,
		Version:          1,
	}, nil
}

// checkParent is invariant I-C1 read upwards: a hub has no container above it, a collection has
// exactly one.
func checkParent(containerType ContainerType, parentID shared.ID) error {
	switch {
	case containerType == ContainerHub && !parentID.IsZero():
		return shared.ErrValidation.
			WithDetail("containers.hub_has_no_parent").
			WithFields(shared.FieldError{Path: "/parent_id", Code: "containers.hub_has_no_parent"})
	case containerType == ContainerCollection && parentID.IsZero():
		return shared.ErrValidation.
			WithDetail("containers.collection_needs_parent").
			WithFields(shared.FieldError{Path: "/parent_id", Code: "containers.collection_needs_parent"})
	}
	return nil
}

func containerName(raw string) (string, error) {
	name := strings.TrimSpace(raw)

	switch {
	case name == "":
		return "", shared.ErrValidation.
			WithDetail("containers.name_empty").
			WithFields(shared.FieldError{Path: "/name", Code: "containers.name_empty"})

	case utf8.RuneCountInString(name) > MaxContainerNameLength:
		return "", shared.ErrValidation.
			WithDetail("containers.name_too_long").
			WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/name", Code: "containers.name_too_long"})

	case hasControlCharacter(name):
		// A name is one line. A newline or a tab in it survives every layer and then breaks the
		// one that renders it - a CSV export, a log line, a calendar summary.
		return "", shared.ErrValidation.
			WithDetail("containers.name_malformed").
			WithFields(shared.FieldError{Path: "/name", Code: "containers.name_malformed"})
	}
	return name, nil
}

// hasControlCharacter reports C0 controls, DEL, and the C1 range. Not a general sanitiser: the
// name is stored and returned as it arrived, and the client escapes it for whatever it renders
// into (security.md §7).
func hasControlCharacter(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

// IsArchived reports whether this container carries the archive stamp itself. The question
// ArchiveContainer and UnarchiveContainer ask - not the one a write asks, which is the next method.
func (c Container) IsArchived() bool { return c.ArchivedAt != nil }

// IsEffectivelyArchived reports invariant I-C3 as a write sees it: archived itself, or sitting in an
// archived hub. Every refusal of a write reads this rather than IsArchived, because "the hub above
// this is archived" and "this is archived" are the same answer to a client that wanted to write.
func (c Container) IsEffectivelyArchived() bool {
	return c.ArchivedAt != nil || c.ParentArchivedAt != nil
}

// EffectiveArchivedAt is when the read-only state began: this container's own stamp, or the hub's.
// The container's own wins when both carry one - it is the more specific decision, and the one
// unarchiving this container would lift.
func (c Container) EffectiveArchivedAt() *time.Time {
	if c.ArchivedAt != nil {
		return c.ArchivedAt
	}
	return c.ParentArchivedAt
}

// ArchivedRootID names the container a client would have to unarchive: this one, or the hub above
// it. Carried in the refusal, because "it is archived" is unhelpful when the archived thing is not
// the object the client named.
func (c Container) ArchivedRootID() shared.ID {
	switch {
	case c.ArchivedAt != nil:
		return c.ID
	case c.ParentArchivedAt != nil:
		return c.ParentID
	default:
		return ""
	}
}

// IsTrashed reports whether the container is in the trash. Distinct from archived: the trash is
// a deletion waiting out its retention period, archiving is a decision to keep something quietly.
func (c Container) IsTrashed() bool { return c.DeletedAt != nil }

// EnsureAcceptsChildren refuses what would be created underneath a container that cannot take it.
//
// Both refusals are conflicts rather than validation errors: the request is well formed and the
// state is what makes it impossible, which is the distinction a client needs in order to know
// whether retrying after a change of its own would help (api-guidelines.md §6).
func (c Container) EnsureAcceptsChildren(child ContainerType) error {
	if c.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("containers.parent_trashed").
			WithParams(map[string]string{"parent_id": c.ID.String()})
	}
	if c.IsEffectivelyArchived() {
		return shared.ErrConflict.
			WithDetail("containers.parent_archived").
			WithParams(map[string]string{
				"parent_id": c.ID.String(), "archived_id": c.ArchivedRootID().String(),
			})
	}
	if !c.Type.AllowsChild(child) {
		return shared.ErrValidation.
			WithDetail("containers.parent_type_invalid").
			WithParams(map[string]string{"parent_type": string(c.Type), "type": string(child)}).
			WithFields(shared.FieldError{Path: "/parent_id", Code: "containers.parent_type_invalid"})
	}
	return nil
}

// EnsureAcceptsItems refuses a work item under a container that cannot hold one.
//
// Only a collection holds items: a hub holds collections, and that is the whole of the container
// hierarchy (domain-model.md §1). The codes are the item's rather than the container's, because
// the field the caller has to change is `collection_id` on the item it was creating - the
// container itself is not at fault and needs no fixing.
func (c Container) EnsureAcceptsItems() error {
	if c.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("items.collection_trashed").
			WithParams(map[string]string{"collection_id": c.ID.String()})
	}
	if c.IsEffectivelyArchived() {
		// The hub's stamp counts here as much as the collection's own: I-C3 makes an archived subtree
		// read-only, and an item under a collection whose hub was archived is inside one.
		return shared.ErrConflict.
			WithDetail("items.collection_archived").
			WithParams(map[string]string{
				"collection_id": c.ID.String(), "archived_id": c.ArchivedRootID().String(),
			})
	}
	if c.Type != ContainerCollection {
		return shared.ErrValidation.
			WithDetail("items.collection_required").
			WithParams(map[string]string{"container_type": string(c.Type)}).
			WithFields(shared.FieldError{Path: "/collection_id", Code: "items.collection_required"})
	}
	return nil
}

// Field names of a container, as the API spells them. They travel into the event's change set and
// into the change log, where a client matches them against the members it sent - so they are
// written once here rather than as a literal at each place that has to agree.
const (
	FieldName             = "name"
	FieldDescription      = "description"
	FieldIcon             = "icon"
	FieldColorToken       = "color_token"
	FieldCompletionPolicy = "completion_policy"
	FieldParentID         = "parent_id"
	FieldOrderKey         = "order_key"
	FieldArchivedAt       = "archived_at"
)

// ContainerAttributes is what a rename may change: the container's own descriptive fields.
//
// A pointer per field, so that "set it to nothing" and "do not touch it" stay two different
// requests all the way down from the merge patch that expressed them. A struct of plain strings
// could not tell a caller that sent no icon from one that sent an empty one, and would clear the
// icon of every client that only meant to rename something.
//
// The policies are not here. They are a different decision with a different endpoint and a
// different audit entry - what a collection is called and how it works are not one field set.
type ContainerAttributes struct {
	Name        *string
	Description *string
	Icon        *string
	ColorToken  *string
}

// IsEmpty reports whether the update asks for nothing at all.
func (a ContainerAttributes) IsEmpty() bool {
	return a.Name == nil && a.Description == nil && a.Icon == nil && a.ColorToken == nil
}

// Renamed applies a change to the container's own fields and reports which of them moved.
//
// Nothing that did not move is in the result, and a request that changes nothing returns the
// container untouched with no changes at all. That is what makes a repeat harmless rather than
// merely accepted: the caller writes nothing, spends no version and announces nothing - the same
// contract WorkItem.Updated keeps, for the same reason.
func (c Container) Renamed(attributes ContainerAttributes, at time.Time) (Container, []FieldChange, error) {
	if err := c.EnsureEditable(); err != nil {
		return Container{}, nil, err
	}

	var changes []FieldChange

	if attributes.Name != nil {
		name, err := containerName(*attributes.Name)
		if err != nil {
			return Container{}, nil, err
		}
		if name != c.Name {
			changes = append(changes, FieldChange{Field: FieldName, From: c.Name, To: name})
			c.Name = name
		}
	}

	// The three optional fields, each trimmed and each free to become empty. Empty is "not set"
	// rather than a value here, which is what lets `null` and `""` in a merge patch mean the one
	// thing a person would expect them to mean: the field is gone.
	for _, field := range []struct {
		name  string
		want  *string
		value *string
	}{
		{FieldDescription, attributes.Description, &c.Description},
		{FieldIcon, attributes.Icon, &c.Icon},
		{FieldColorToken, attributes.ColorToken, &c.ColorToken},
	} {
		if field.want == nil {
			continue
		}
		wanted := strings.TrimSpace(*field.want)
		if wanted == *field.value {
			continue
		}
		if hasControlCharacter(wanted) {
			// The same rule the name keeps, for the same reason: these are single values that end up
			// in a CSV export or a calendar summary, and a newline in one breaks whatever renders it.
			return Container{}, nil, shared.ErrValidation.
				WithDetail("containers.field_malformed").
				WithParams(map[string]string{"field": field.name}).
				WithFields(shared.FieldError{Path: "/" + field.name, Code: "containers.field_malformed"})
		}
		changes = append(changes, FieldChange{Field: field.name, From: *field.value, To: wanted})
		*field.value = wanted
	}

	if len(changes) == 0 {
		return c, nil, nil
	}
	c.UpdatedAt = at
	return c, changes, nil
}

// ContainerPolicies is how a collection works, as the policies document carries it.
//
// A value type rather than pointers: this is a PUT, and a key that is not sent falls back to its
// default rather than keeping what was stored. A configuration document that partly remembers what
// it replaced is one nobody can reason about - and with one key today, it would be one key nobody
// noticed getting that wrong.
type ContainerPolicies struct {
	CompletionPolicy CompletionPolicy
}

// WithPolicies replaces the container's policies and reports what moved.
//
// A hub is refused rather than accepted and ignored. A hub holds collections and no items, so a
// completion policy on one decides nothing; storing it would be a setting a person could change
// and never see take effect (domain-model.md §3.3).
func (c Container) WithPolicies(policies ContainerPolicies, at time.Time) (Container, []FieldChange, error) {
	if c.Type != ContainerCollection {
		return Container{}, nil, shared.ErrValidation.
			WithDetail("containers.policies_not_supported").
			WithParams(map[string]string{"container_type": string(c.Type)}).
			WithFields(shared.FieldError{Path: "/", Code: "containers.policies_not_supported"})
	}
	if err := c.EnsureEditable(); err != nil {
		return Container{}, nil, err
	}

	policy := policies.CompletionPolicy
	if policy == "" {
		// The key was not sent. It falls back to the default, which is what a PUT means - not to
		// whatever happens to be stored.
		policy = DefaultCompletionPolicy
	}
	if !policy.Valid() {
		return Container{}, nil, shared.ErrValidation.
			WithDetail("containers.completion_policy_unknown").
			WithParams(map[string]string{"value": string(policy)}).
			WithFields(shared.FieldError{
				Path: "/completion_policy", Code: "containers.completion_policy_unknown",
			})
	}
	if policy == c.CompletionPolicy {
		return c, nil, nil
	}

	changes := []FieldChange{{
		Field: FieldCompletionPolicy, From: string(c.CompletionPolicy), To: string(policy),
	}}
	c.CompletionPolicy = policy
	c.UpdatedAt = at
	return c, changes, nil
}

// Archived stamps the container read-only, and reports the change as the other verbs do.
//
// Idempotent: archiving an archived container succeeds with no changes, which is what makes a retry
// after a lost response harmless. The inherited state is a gate rather than a no-op - a collection
// whose hub is archived is inside an archived subtree, and archiving it would be a write into one
// (I-C3). Unarchive the hub first; the answer names it.
func (c Container) Archived(at time.Time) (Container, []FieldChange, error) {
	if err := c.ensureLifecycleChangeable(); err != nil {
		return Container{}, nil, err
	}
	if c.IsArchived() {
		return c, nil, nil
	}

	changes := []FieldChange{{Field: FieldArchivedAt, From: "", To: instant(at)}}
	c.ArchivedAt = &at
	c.UpdatedAt = at
	return c, changes, nil
}

// Unarchived lifts this container's own stamp.
//
// Only its own. A collection archived in its own right stays archived when its hub is unarchived,
// because unarchiving restores what was archived and not what was merely covered by it - which is
// the whole reason the inherited stamp is never written onto the row.
func (c Container) Unarchived(at time.Time) (Container, []FieldChange, error) {
	if err := c.ensureLifecycleChangeable(); err != nil {
		return Container{}, nil, err
	}
	if !c.IsArchived() {
		return c, nil, nil
	}

	changes := []FieldChange{{Field: FieldArchivedAt, From: instant(*c.ArchivedAt), To: ""}}
	c.ArchivedAt = nil
	c.UpdatedAt = at
	return c, changes, nil
}

// instant is how a timestamp travels in a change set: RFC 3339 in UTC, which is the one spelling
// every channel of this system already uses. The empty string is the field being cleared, exactly
// as it is for the text fields - a recipient reads "not set" from it and never a zero time.
func instant(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

// ensureLifecycleChangeable is what archiving and unarchiving both need: not in the trash, and not
// inside somebody else's archived subtree.
//
// It reads the inherited stamp rather than EnsureEditable's effective one, because these two verbs
// own the container's own stamp: a check that refused an archived container would make unarchiving
// impossible.
func (c Container) ensureLifecycleChangeable() error {
	if c.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("containers.trashed").
			WithParams(map[string]string{"container_id": c.ID.String()})
	}
	if c.ParentArchivedAt != nil {
		return shared.ErrConflict.
			WithDetail("containers.archived").
			WithParams(map[string]string{
				"container_id": c.ID.String(), "archived_id": c.ParentID.String(),
			})
	}
	return nil
}

// EnsureEditable refuses a container whose state makes it read-only (I-C2, I-C3).
//
// A conflict rather than a validation failure: the request is well formed and the state is what
// makes it impossible, which is the distinction that tells a client whether unarchiving something
// first would help (api-guidelines.md §6). `archived_id` says what that something is - this
// container, or the hub above it.
func (c Container) EnsureEditable() error {
	if c.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("containers.trashed").
			WithParams(map[string]string{"container_id": c.ID.String()})
	}
	if c.IsEffectivelyArchived() {
		return shared.ErrConflict.
			WithDetail("containers.archived").
			WithParams(map[string]string{
				"container_id": c.ID.String(), "archived_id": c.ArchivedRootID().String(),
			})
	}
	return nil
}

// MovedInto returns the container as it sits under a new parent, and reports what moved with it.
//
// Everything a create refuses a move refuses too: the type under the new parent, a trashed or
// archived destination (Container.EnsureAcceptsChildren). Re-checking rather than assuming is what
// makes a narrowing visible - the destination may have been archived since the client read it.
//
// A hub is refused outright. It sits in the tenant and in nothing else (I-C1), so there is no
// destination to name and a move would have to invent one.
func (c Container) MovedInto(parent Container, orderKey string, at time.Time) (Container, []FieldChange, error) {
	if c.Type != ContainerCollection {
		return Container{}, nil, shared.ErrValidation.
			WithDetail("containers.hub_not_movable").
			WithParams(map[string]string{"container_id": c.ID.String()}).
			WithFields(shared.FieldError{Path: "/target_parent_id", Code: "containers.hub_not_movable"})
	}
	if err := c.EnsureEditable(); err != nil {
		return Container{}, nil, err
	}
	if parent.ID == c.ID {
		// A container inside itself has no level and no path. Cheap to check and unrecoverable to
		// store, which is the same reasoning the item hierarchy's cycle check rests on (I-W2).
		return Container{}, nil, shared.ErrValidation.
			WithDetail("containers.parent_is_self").
			WithParams(map[string]string{"container_id": c.ID.String()}).
			WithFields(shared.FieldError{Path: "/target_parent_id", Code: "containers.parent_is_self"})
	}
	if err := parent.EnsureAcceptsChildren(c.Type); err != nil {
		return Container{}, nil, err
	}
	if orderKey == "" {
		return Container{}, nil, shared.ErrInternal.WithDetail("containers.identity_incomplete")
	}

	if parent.ID == c.ParentID && orderKey == c.OrderKey {
		return c, nil, nil
	}

	var changes []FieldChange
	if parent.ID != c.ParentID {
		changes = append(changes, FieldChange{
			Field: FieldParentID, From: c.ParentID.String(), To: parent.ID.String(),
		})
	}
	if orderKey != c.OrderKey {
		changes = append(changes, FieldChange{Field: FieldOrderKey, From: c.OrderKey, To: orderKey})
	}

	c.ParentID = parent.ID
	c.OrderKey = orderKey
	// The destination decides what the collection inherits. It has just been checked as accepting
	// children, so it is not archived - which is what makes this a clear rather than a copy.
	c.ParentArchivedAt = nil
	c.UpdatedAt = at
	return c, changes, nil
}
