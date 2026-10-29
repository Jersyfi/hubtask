// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// entryTypeNameCodes are the codes the type's name rule reports with.
var entryTypeNameCodes = nameCodes{
	empty:     "entry_types.name_empty",
	tooLong:   "entry_types.name_too_long",
	malformed: "entry_types.name_malformed",
}

var (
	entryTypeKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
	entryTypeIconPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
)

// EntryBehaviours is what the entries of a type do (F-1). Each is a switch of the type, not of the
// entry: two entries of one type never behave differently.
type EntryBehaviours struct {
	// Completable entries are done or open. A record is not (D-07).
	Completable bool
	// Workflow entries move through their type's statuses (FLW-02, a later step).
	Workflow bool
	// Record entries are things kept rather than work done - a configuration item, a risk, a
	// contact - and have no completion.
	Record bool
	// Schedulable entries carry a due date, reminders and a recurrence.
	Schedulable bool
	// Estimable entries carry an estimate (a later step's field).
	Estimable bool
}

// EntryType is a kind of entry a workspace names on one level - a bug, a feature, a purchase, a
// configuration item - with its icon, its colour and its behaviours (UC-FLW-01, domain-model.md §2).
//
// The level is the structure, and a type does not change it: a type sits on a level, and which
// levels a collection shows stays the collection's structure. The three system types - one per
// level, keyed after it - are declared here (SystemEntryTypes) and are what an entry with no type
// of its own is; a workspace's row for one only restates how it looks and behaves.
type EntryType struct {
	// ID is zero for a system type the workspace has never changed: it has no row.
	ID       shared.ID
	TenantID shared.ID
	// Key is the stable name every door, the configuration document and a query use.
	Key    string
	Level  ItemType
	System bool
	// Name is empty on a system type nobody named: a client then shows its own word for the level,
	// because the backend carries no display text (rule 8).
	Name string
	// Icon is the name of an icon in the design system's set, never an image.
	Icon string
	// ColorToken is one of the label tokens, never a colour value (rule 15).
	ColorToken string
	Behaviours EntryBehaviours
	// AllowedChildren are the keys of the types that may sit directly underneath; empty is every
	// type of the level below.
	AllowedChildren []string
	// Form is the type's one layout and logic, empty until somebody designs one (Form, P-18).
	Form       Form
	Position   int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// Version is zero for a system type without a row.
	Version int
}

// SystemEntryTypeKey is the key of a level's system type: the level in lower case.
func SystemEntryTypeKey(level ItemType) string { return strings.ToLower(string(level)) }

// SystemEntryType is the level's system type as Hubtask ships it.
func SystemEntryType(level ItemType) EntryType {
	icon := map[ItemType]string{
		ItemTask: "square-check", ItemWorkPackage: "file-text", ItemActivity: "check",
	}[level]
	return EntryType{
		Key: SystemEntryTypeKey(level), Level: level, System: true,
		Icon: icon, ColorToken: string(shared.LabelTokenSlate),
		Behaviours: EntryBehaviours{Completable: true, Schedulable: true},
	}
}

// SystemEntryTypes are the three system types, one per level.
func SystemEntryTypes() []EntryType {
	types := make([]EntryType, 0, len(itemTypes))
	for _, level := range itemTypes {
		types = append(types, SystemEntryType(level))
	}
	return types
}

// FieldForm is the field name a change of the form records.
const FieldForm = "form"

// WithForm gives the type a form already checked by NewForm. A system type takes one too: it is
// what an entry without a type of its own is, and its form binds those entries.
func (t EntryType) WithForm(form Form, now time.Time) (EntryType, []FieldChange) {
	next := t
	next.Form = form
	next.UpdatedAt = now
	return next, []FieldChange{{Field: FieldForm, To: strconv.Itoa(len(form.Fields))}}
}

// IsArchived reports whether the type is out of use: not offered for new entries, readable on old.
func (t EntryType) IsArchived() bool { return t.ArchivedAt != nil }

// Stored reports whether the type has a row.
func (t EntryType) Stored() bool { return !t.ID.IsZero() }

// NewEntryTypeInput is what a workspace's own type is made of.
type NewEntryTypeInput struct {
	ID              shared.ID
	TenantID        shared.ID
	Key             string
	Level           ItemType
	Name            string
	Icon            string
	ColorToken      string
	Behaviours      EntryBehaviours
	AllowedChildren []string
	Position        int
	Now             time.Time
	Text            text.Normalizer
}

// NewEntryType builds a workspace's own type and checks its invariants. A key already taken is the
// unique index's decision, for the reason NewLabel gives.
func NewEntryType(in NewEntryTypeInput) (EntryType, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return EntryType{}, shared.ErrInternal.WithDetail("entry_types.identity_incomplete")
	}
	if !in.Level.Valid() {
		return EntryType{}, entryTypeInvalid("/level", "entry_types.level_unknown")
	}
	key := strings.TrimSpace(in.Key)
	if !entryTypeKeyPattern.MatchString(key) {
		return EntryType{}, entryTypeInvalid("/key", "entry_types.key_malformed")
	}
	if isSystemEntryTypeKey(key) {
		return EntryType{}, entryTypeInvalid("/key", "entry_types.key_reserved")
	}
	t := EntryType{
		ID: in.ID, TenantID: in.TenantID, Key: key, Level: in.Level,
		Position: in.Position, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}
	return t.with(EntryTypeChange{
		Name: &in.Name, Icon: &in.Icon, ColorToken: &in.ColorToken, Behaviours: &in.Behaviours,
		AllowedChildren: &in.AllowedChildren,
	}, in.Text)
}

func isSystemEntryTypeKey(key string) bool {
	for _, level := range itemTypes {
		if key == SystemEntryTypeKey(level) {
			return true
		}
	}
	return false
}

// EntryTypeChange is what a change of a type may set. A pointer per field, so that "leave it" and
// "set it" stay two requests; Archived true archives, false brings the type back.
type EntryTypeChange struct {
	Name            *string
	Icon            *string
	ColorToken      *string
	Behaviours      *EntryBehaviours
	AllowedChildren *[]string
	Position        *int
	Archived        *bool
}

// IsEmpty reports whether the change asks for nothing.
func (c EntryTypeChange) IsEmpty() bool {
	return c.Name == nil && c.Icon == nil && c.ColorToken == nil && c.Behaviours == nil &&
		c.AllowedChildren == nil && c.Position == nil && c.Archived == nil
}

// Changed applies a change at now and reports which fields moved. A system type is never archived
// and never becomes a record: it is what an entry with no type of its own is.
func (t EntryType) Changed(change EntryTypeChange, now time.Time, form text.Normalizer) (EntryType, []FieldChange, error) {
	if change.IsEmpty() {
		return EntryType{}, nil, shared.ErrValidation.WithDetail("entry_types.update_empty")
	}
	next, err := t.with(change, form)
	if err != nil {
		return EntryType{}, nil, err
	}
	if change.Position != nil {
		next.Position = *change.Position
	}
	if change.Archived != nil {
		switch {
		case *change.Archived && t.System:
			return EntryType{}, nil, shared.ErrConflict.WithDetail("entry_types.system_not_archivable")
		case *change.Archived && next.ArchivedAt == nil:
			at := now
			next.ArchivedAt = &at
		case !*change.Archived:
			next.ArchivedAt = nil
		}
	}
	changes := t.diff(next)
	if len(changes) > 0 {
		next.UpdatedAt = now
	}
	return next, changes, nil
}

func (t EntryType) with(change EntryTypeChange, form text.Normalizer) (EntryType, error) {
	next := t
	next.AllowedChildren = slices.Clone(t.AllowedChildren)
	if change.Name != nil {
		name := strings.TrimSpace(*change.Name)
		// A system type may go back to no name of its own: the client's word for the level.
		if name == "" && t.System {
			next.Name = ""
		} else {
			normalised, err := structureName(name, entryTypeNameCodes, form)
			if err != nil {
				return EntryType{}, err
			}
			next.Name = normalised
		}
	}
	if change.Icon != nil {
		icon := strings.TrimSpace(*change.Icon)
		if !entryTypeIconPattern.MatchString(icon) {
			return EntryType{}, entryTypeInvalid("/icon", "entry_types.icon_malformed")
		}
		next.Icon = icon
	}
	if change.ColorToken != nil {
		token := strings.TrimSpace(*change.ColorToken)
		if token == "" {
			return EntryType{}, entryTypeInvalid("/color_token", "entry_types.color_token_empty")
		}
		if err := knownColorToken(token, "entry_types.color_token_unknown"); err != nil {
			return EntryType{}, err
		}
		next.ColorToken = token
	}
	if change.Behaviours != nil {
		next.Behaviours = *change.Behaviours
	}
	if change.AllowedChildren != nil {
		children := make([]string, 0, len(*change.AllowedChildren))
		for _, key := range *change.AllowedChildren {
			key = strings.TrimSpace(key)
			if !entryTypeKeyPattern.MatchString(key) {
				return EntryType{}, entryTypeInvalid("/allowed_children", "entry_types.key_malformed")
			}
			if !slices.Contains(children, key) {
				children = append(children, key)
			}
		}
		if len(children) > 0 && t.Level.childLevel() == "" {
			return EntryType{}, entryTypeInvalid("/allowed_children", "entry_types.no_level_below")
		}
		next.AllowedChildren = children
	}
	if next.Behaviours.Record && next.Behaviours.Completable {
		return EntryType{}, entryTypeInvalid("/behaviours", "entry_types.record_not_completable")
	}
	if next.System && next.Behaviours.Record {
		return EntryType{}, entryTypeInvalid("/behaviours", "entry_types.system_not_record")
	}
	return next, nil
}

func (t EntryType) diff(next EntryType) []FieldChange {
	var changes []FieldChange
	add := func(field, from, to string) {
		if from != to {
			changes = append(changes, FieldChange{Field: field, From: from, To: to})
		}
	}
	add(FieldName, t.Name, next.Name)
	add("icon", t.Icon, next.Icon)
	add(FieldColorToken, t.ColorToken, next.ColorToken)
	add("behaviours", t.Behaviours.code(), next.Behaviours.code())
	add("allowed_children", strings.Join(t.AllowedChildren, ","), strings.Join(next.AllowedChildren, ","))
	add("position", strconv.Itoa(t.Position), strconv.Itoa(next.Position))
	add("archived", boolCode(t.IsArchived()), boolCode(next.IsArchived()))
	return changes
}

// code is the behaviours as one comparable word, for a change record.
func (b EntryBehaviours) code() string {
	var on []string
	for _, pair := range []struct {
		name string
		on   bool
	}{
		{"completable", b.Completable}, {"workflow", b.Workflow}, {"record", b.Record},
		{"schedulable", b.Schedulable}, {"estimable", b.Estimable},
	} {
		if pair.on {
			on = append(on, pair.name)
		}
	}
	return strings.Join(on, ",")
}

func boolCode(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func entryTypeInvalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// childLevel is the level directly underneath, empty for the lowest.
func (t ItemType) childLevel() ItemType {
	switch t {
	case ItemTask:
		return ItemWorkPackage
	case ItemWorkPackage:
		return ItemActivity
	}
	return ""
}

// EntryTypes is a workspace's registry: the system types, as the workspace has shaped them, and its
// own, in the order a picker shows them - by level, then position, then key.
type EntryTypes []EntryType

// NewEntryTypes merges the workspace's rows over the system types.
func NewEntryTypes(rows []EntryType) EntryTypes {
	registry := make(EntryTypes, 0, len(rows)+len(itemTypes))
	for _, system := range SystemEntryTypes() {
		if row, found := findEntryType(rows, system.Key); found && row.System {
			registry = append(registry, row)
			continue
		}
		registry = append(registry, system)
	}
	for _, row := range rows {
		if !row.System {
			registry = append(registry, row)
		}
	}
	levelRank := func(level ItemType) int { return slices.Index(itemTypes[:], level) }
	slices.SortStableFunc(registry, func(a, b EntryType) int {
		switch {
		case levelRank(a.Level) != levelRank(b.Level):
			return levelRank(a.Level) - levelRank(b.Level)
		case a.System != b.System:
			// The level's default leads its level.
			if a.System {
				return -1
			}
			return 1
		case a.Position != b.Position:
			return a.Position - b.Position
		}
		return strings.Compare(a.Key, b.Key)
	})
	return registry
}

func findEntryType(types []EntryType, key string) (EntryType, bool) {
	for _, t := range types {
		if t.Key == key {
			return t, true
		}
	}
	return EntryType{}, false
}

// Find answers the type of a key.
func (r EntryTypes) Find(key string) (EntryType, bool) { return findEntryType(r, key) }

// Of answers an entry's type: its own key, or its level's system type for none.
func (r EntryTypes) Of(level ItemType, key string) EntryType {
	if key != "" {
		if t, found := r.Find(key); found {
			return t
		}
	}
	if t, found := r.Find(SystemEntryTypeKey(level)); found {
		return t
	}
	return SystemEntryType(level)
}

// Offered are the types of a level a new entry may take: not archived, the system type first.
func (r EntryTypes) Offered(level ItemType) []EntryType {
	var offered []EntryType
	for _, t := range r {
		if t.Level == level && !t.IsArchived() {
			offered = append(offered, t)
		}
	}
	return offered
}

// Choose answers the type key an entry of the level is written with when key is asked for, and
// refuses one that does not exist, sits on another level, or is archived (UC-FLW-01 checks 3, 5).
// The answer is empty for the level's system type, which an entry names by having none. A parent
// whose type names its allowed children narrows the choice further.
func (r EntryTypes) Choose(level ItemType, key string, parent *EntryType) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = SystemEntryTypeKey(level)
	}
	t, found := r.Find(key)
	if !found {
		return "", shared.ErrValidation.WithDetail("entry_types.unknown").
			WithFields(shared.FieldError{Path: "/entry_type", Code: "entry_types.unknown"})
	}
	if t.Level != level {
		return "", shared.ErrValidation.WithDetail("entry_types.other_level").
			WithFields(shared.FieldError{Path: "/entry_type", Code: "entry_types.other_level"})
	}
	if t.IsArchived() {
		return "", shared.ErrValidation.WithDetail("entry_types.archived").
			WithFields(shared.FieldError{Path: "/entry_type", Code: "entry_types.archived"})
	}
	if parent != nil && len(parent.AllowedChildren) > 0 && !slices.Contains(parent.AllowedChildren, t.Key) {
		return "", shared.ErrValidation.WithDetail("entry_types.not_allowed_here").
			WithFields(shared.FieldError{Path: "/entry_type", Code: "entry_types.not_allowed_here"})
	}
	if t.System {
		return "", nil
	}
	return t.Key, nil
}

// Narrow takes from a level's capability profile what the type's behaviours switch off: a type that
// is not completable - a record - has no completion, and one that is not scheduled has no due date,
// reminder or recurrence. A type never widens a profile.
func (t EntryType) Narrow(profile CapabilityProfile) CapabilityProfile {
	var off []Capability
	if !t.Behaviours.Completable {
		off = append(off, CapabilityCompletion)
	}
	if !t.Behaviours.Schedulable {
		off = append(off, CapabilityDueDate, CapabilityReminder, CapabilityRecurrence)
	}
	if len(off) == 0 {
		return profile
	}
	narrowed := profile
	narrowed.Capabilities = slices.DeleteFunc(slices.Clone(profile.Capabilities), func(c Capability) bool {
		return slices.Contains(off, c)
	})
	return narrowed
}

// DroppedBy answers the fields of the entry's values the profile would not carry - what changing
// the entry to a type with that profile would lose (UC-FLW-01 check 4), by field.
func (i WorkItem) DroppedBy(profile CapabilityProfile) []string {
	var dropped []string
	if i.Completion.IsCompleted && !profile.Allows(CapabilityCompletion) {
		dropped = append(dropped, "completion")
	}
	if i.Due != nil && !profile.Allows(CapabilityDueDate) {
		dropped = append(dropped, "due_at")
	}
	if !i.RecurrenceRuleID.IsZero() && !profile.Allows(CapabilityRecurrence) {
		dropped = append(dropped, "recurrence")
	}
	return dropped
}
