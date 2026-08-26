// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Template is a tree somebody wrote down once so that it can be stamped out again
// (domain-model.md §3.5).
//
// One tree rather than several: `root_type` names what it produces and an instantiation answers
// one `root_item_id`, so a document with two roots would be two templates wearing one name.
// Somebody who wants three tasks writes three templates, or one task with three work packages -
// which is also the shape the hierarchy is about.
//
// What it stores is a shape and never a schedule: a node's due date is a duration from the anchor
// of the instantiation, because a template outlives the week it was written in and an absolute
// date in it would be wrong by the second time it is used.
type Template struct {
	ID       shared.ID
	TenantID shared.ID
	// Scope is where the template is defined and therefore who can use it. ScopeID names the
	// container for a hub or a collection scope, and is empty for a workspace-wide one.
	Scope       TemplateScope
	ScopeID     shared.ID
	Name        string
	Description string
	RootType    ItemType
	// Root is the tree. One node with its children under it, which is what the contract's array
	// of one carries.
	Root      TemplateNode
	CreatedAt time.Time
	UpdatedAt *time.Time
	// DeletedAt marks a soft deletion: the trees it has already stamped out are ordinary entries
	// and outlive it, and a template defined later under the same name is a new one rather than
	// this one coming back (the C-07 lesson).
	DeletedAt *time.Time
	Version   int
}

// TemplateNode is one entry the template will produce.
type TemplateNode struct {
	Type  ItemType
	Title string
	Notes string
	// DueOffset is how far after the anchor this entry is due, and nil for a node with no due
	// date. A duration rather than a date - "three days in" is what a template can know.
	DueOffset *time.Duration
	// DueDateOnly marks a node whose due date is a day rather than a moment.
	DueDateOnly bool
	// AssigneeID is who this step always belongs to, and empty for a node the target collection
	// decides about.
	//
	// The assignment rule a template carries, and the whole of it: a fixed person. Naming the
	// collection's own policy instead would name what already happens - a create runs it (C-02) -
	// and would give a template a second way of saying nothing.
	AssigneeID shared.ID
	Children   []TemplateNode
}

// TemplateScope is where a template is defined.
type TemplateScope string

const (
	TemplateScopeTenant     TemplateScope = "TENANT"
	TemplateScopeHub        TemplateScope = "HUB"
	TemplateScopeCollection TemplateScope = "COLLECTION"
)

// TemplateScopes is the closed set, in the order the schema's check constraint lists them.
func TemplateScopes() []TemplateScope {
	return []TemplateScope{TemplateScopeTenant, TemplateScopeHub, TemplateScopeCollection}
}

// Valid reports whether a scope is one this system knows.
func (s TemplateScope) Valid() bool {
	for _, known := range TemplateScopes() {
		if s == known {
			return true
		}
	}
	return false
}

func (s TemplateScope) String() string { return string(s) }

// NeedsContainer reports whether the scope names a container. A workspace-wide template names
// none, and the two that do are refused without one.
func (s TemplateScope) NeedsContainer() bool { return s != TemplateScopeTenant }

const (
	// MaxTemplateNameLength and MaxTemplateDescriptionLength count code points, for the reason
	// every length in this codebase does (I-W7): a limit in bytes measures the alphabet.
	MaxTemplateNameLength        = 200
	MaxTemplateDescriptionLength = 2000
	// MaxTemplateNodes is the bound the backlog set instead of a /jobs resource: an instantiation
	// is one synchronous transaction, and this is how large it may be. Enforced at the definition
	// and again at the instantiation, and published in /meta/capabilities so that a client knows
	// the number rather than discovering it.
	MaxTemplateNodes = 500
	// MaxTemplateOffset is the longest a relative date may be: ten years, the same plausibility
	// bound a reminder's offset takes.
	MaxTemplateOffset = 3650 * 24 * time.Hour
)

// The template's own field names, as the contract spells them.
const (
	FieldTemplateName        = "name"
	FieldTemplateDescription = "description"
	FieldTemplateNodes       = "nodes"
)

// NewTemplateInput is what a definition needs decided.
type NewTemplateInput struct {
	ID       shared.ID
	TenantID shared.ID
	Spec     TemplateSpec
	Now      time.Time
}

// TemplateSpec is what a caller says about a template.
type TemplateSpec struct {
	Scope       string
	ScopeID     shared.ID
	Name        string
	Description string
	RootType    string
	Root        TemplateNode
}

// NewTemplate validates and builds a template.
//
// What it does not check is whether the tree is *possible*: which type may sit under which is the
// hierarchy's answer and depends on this installation's profiles, so the application asks it (the
// same division D-04 makes with the recurrence library). Everything that is decidable from the
// document alone is decided here.
func NewTemplate(input NewTemplateInput) (Template, error) {
	spec, err := validTemplateSpec(input.Spec)
	if err != nil {
		return Template{}, err
	}

	return Template{
		ID:          input.ID,
		TenantID:    input.TenantID,
		Scope:       TemplateScope(spec.Scope),
		ScopeID:     spec.ScopeID,
		Name:        spec.Name,
		Description: spec.Description,
		RootType:    ItemType(spec.RootType),
		Root:        spec.Root,
		CreatedAt:   input.Now,
		Version:     1,
	}, nil
}

// TemplatePatch is a merge patch's touch on a template: nil means "not sent". The tree travels
// whole, because a tree is a shape rather than a list of settings and half of one is a different
// shape.
type TemplatePatch struct {
	Name        *string
	Description *string
	Root        *TemplateNode
}

// IsEmpty reports whether the patch asks for nothing at all.
func (p TemplatePatch) IsEmpty() bool {
	return p.Name == nil && p.Description == nil && p.Root == nil
}

// Changed applies a patch and reports which fields moved.
//
// The scope and the root type are not patchable, and that is the decision rather than an omission:
// a template that changed scope would move out from under the people who could use it, and one
// whose root type changed would produce a different kind of thing under the same name. Both are a
// new template - which costs nothing, since defining one is a single call.
func (t Template) Changed(
	patch TemplatePatch, at time.Time,
) (Template, []FieldChange, error) {
	if t.DeletedAt != nil {
		return Template{}, nil, shared.ErrConflict.
			WithDetail("templates.deleted").
			WithFields(shared.FieldError{Path: "/", Code: "templates.deleted"})
	}

	target := t
	if patch.Name != nil {
		name, err := validTemplateName(*patch.Name)
		if err != nil {
			return Template{}, nil, err
		}
		target.Name = name
	}
	if patch.Description != nil {
		description, err := validTemplateDescription(*patch.Description)
		if err != nil {
			return Template{}, nil, err
		}
		target.Description = description
	}
	if patch.Root != nil {
		root, err := validTemplateTree(*patch.Root, ItemType(t.RootType))
		if err != nil {
			return Template{}, nil, err
		}
		target.Root = root
	}

	changes := templateChanges(t, target)
	if len(changes) == 0 {
		return t, nil, nil
	}
	target.UpdatedAt = &at
	return target, changes, nil
}

// Removed returns the template as its tombstone. Idempotent: one already deleted comes back
// untouched, so nothing is written and nothing is announced.
func (t Template) Removed(at time.Time) Template {
	if t.DeletedAt != nil {
		return t
	}
	t.DeletedAt = &at
	return t
}

// Nodes walks the tree in the order an instantiation writes it: parents before children, so a
// child always finds the parent it hangs from.
func (t Template) Nodes() []TemplateNode { return flatten(t.Root) }

// NodeCount is how many entries this template would produce.
func (t Template) NodeCount() int { return len(t.Nodes()) }

// flatten is the walk, breadth first so that one level is written before the next.
func flatten(root TemplateNode) []TemplateNode {
	flat := []TemplateNode{root}
	for index := 0; index < len(flat); index++ {
		flat = append(flat, flat[index].Children...)
	}
	return flat
}

// DueAt resolves a node's relative date against an anchor, or answers nil for a node with none.
//
// The anchor is a moment in the reader's own zone and the offset is added to it, so a template
// used in Berlin and in São Paulo produces the same *day* in each - which is what "+3 days" means
// to the person who wrote it (i18n-l10n.md §4).
func (n TemplateNode) DueAt(anchor time.Time, zone string) (*DueDate, error) {
	if n.DueOffset == nil {
		return nil, nil
	}

	at := anchor.Add(*n.DueOffset)
	return NewDueDate(&at, n.DueDateOnly, zone)
}

// validTemplateSpec is everything decidable without the hierarchy.
func validTemplateSpec(spec TemplateSpec) (TemplateSpec, error) {
	scope := TemplateScope(strings.TrimSpace(spec.Scope))
	if !scope.Valid() {
		return spec, templateInvalid("templates.scope_unknown", "/scope_type",
			map[string]string{"value": string(scope)})
	}
	switch {
	case scope.NeedsContainer() && spec.ScopeID.IsZero():
		return spec, templateInvalid("templates.scope_id_required", "/scope_id", nil)
	case !scope.NeedsContainer() && !spec.ScopeID.IsZero():
		return spec, templateInvalid("templates.scope_id_not_allowed", "/scope_id", nil)
	}

	name, err := validTemplateName(spec.Name)
	if err != nil {
		return spec, err
	}
	description, err := validTemplateDescription(spec.Description)
	if err != nil {
		return spec, err
	}

	rootType := ItemType(strings.TrimSpace(spec.RootType))
	if !rootType.Valid() {
		return spec, templateInvalid("items.type_unknown", "/root_type",
			map[string]string{"value": string(rootType)})
	}
	root, err := validTemplateTree(spec.Root, rootType)
	if err != nil {
		return spec, err
	}

	spec.Scope = string(scope)
	spec.Name = name
	spec.Description = description
	spec.RootType = string(rootType)
	spec.Root = root
	return spec, nil
}

// validTemplateTree checks the shape: the root is the type the template declares, every title is
// one somebody could give an entry, every offset is a duration, and the whole thing stays inside
// the bound.
func validTemplateTree(root TemplateNode, rootType ItemType) (TemplateNode, error) {
	if root.Type == "" {
		root.Type = rootType
	}
	if root.Type != rootType {
		return root, templateInvalid("templates.root_type_mismatch", "/nodes/0/type",
			map[string]string{"value": string(root.Type)})
	}

	count := len(flatten(root))
	if count > MaxTemplateNodes {
		return root, templateInvalid("templates.too_many_nodes", "/nodes",
			map[string]string{
				"maximum": strconv.Itoa(MaxTemplateNodes), "count": strconv.Itoa(count),
			})
	}
	return validTemplateNode(root, "/nodes/0")
}

// validTemplateNode checks one node and everything under it, carrying the path so that a refusal
// points at the node that caused it rather than at the document.
func validTemplateNode(node TemplateNode, path string) (TemplateNode, error) {
	if !node.Type.Valid() {
		return node, templateInvalid("items.type_unknown", path+"/type",
			map[string]string{"value": string(node.Type)})
	}

	title := strings.TrimSpace(node.Title)
	switch {
	case title == "":
		return node, templateInvalid("items.title_empty", path+"/title", nil)
	case utf8.RuneCountInString(title) > MaxItemTitleLength:
		return node, templateInvalid("items.title_too_long", path+"/title",
			map[string]string{"maximum": strconv.Itoa(MaxItemTitleLength)})
	}
	node.Title = title

	if node.DueOffset != nil {
		if *node.DueOffset > MaxTemplateOffset || *node.DueOffset < -MaxTemplateOffset {
			return node, templateInvalid("templates.due_offset_out_of_range", path+"/due_offset",
				nil)
		}
	}
	if node.DueOffset == nil && node.DueDateOnly {
		// The same rule a due date keeps: a flag that qualifies a date which is not there is a
		// value whose meaning depends on a field it does not have (i18n-l10n.md §4).
		return node, templateInvalid("templates.due_date_only_without_offset",
			path+"/due_date_only", nil)
	}

	for index, child := range node.Children {
		checked, err := validTemplateNode(child, path+"/children/"+strconv.Itoa(index))
		if err != nil {
			return node, err
		}
		node.Children[index] = checked
	}
	return node, nil
}

// SpellTemplateOffset writes a duration back as the ISO-8601 form a template stores.
//
// Canonical rather than as-given: unlike a reminder's offset, which is a string somebody typed and
// is kept as typed, a node's offset lives inside a document this code writes - so there is no
// original spelling to preserve, and one spelling means one string in the change log.
func SpellTemplateOffset(offset time.Duration) string {
	sign := ""
	if offset < 0 {
		sign, offset = "-", -offset
	}

	days := int64(offset / (24 * time.Hour))
	rest := offset % (24 * time.Hour)
	spelled := sign + "P"
	if days > 0 {
		spelled += strconv.FormatInt(days, 10) + "D"
	}
	if rest == 0 {
		if days == 0 {
			return sign + "PT0S"
		}
		return spelled
	}

	spelled += "T"
	hours := int64(rest / time.Hour)
	minutes := int64((rest % time.Hour) / time.Minute)
	seconds := int64((rest % time.Minute) / time.Second)
	if hours > 0 {
		spelled += strconv.FormatInt(hours, 10) + "H"
	}
	if minutes > 0 {
		spelled += strconv.FormatInt(minutes, 10) + "M"
	}
	if seconds > 0 {
		spelled += strconv.FormatInt(seconds, 10) + "S"
	}
	return spelled
}

// ParseTemplateOffset reads a node's relative date: an ISO-8601 duration, signed, of weeks down to
// seconds.
//
// The same grammar a reminder's REL offset takes, and refused for the same reasons - years and
// months are calendar arithmetic rather than a length of time, and "+1 month" would mean two
// different things in two different months.
func ParseTemplateOffset(spec string) (time.Duration, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, templateInvalid("templates.due_offset_invalid", "/due_offset",
			map[string]string{"value": spec})
	}

	offset, err := parseISODuration(spec)
	switch {
	case errors.Is(err, errOffsetCalendar):
		return 0, templateInvalid("templates.due_offset_calendar_unit", "/due_offset",
			map[string]string{"value": spec})
	case errors.Is(err, errOffsetOutOfRange):
		return 0, templateInvalid("templates.due_offset_out_of_range", "/due_offset",
			map[string]string{"value": spec})
	case err != nil:
		return 0, templateInvalid("templates.due_offset_invalid", "/due_offset",
			map[string]string{"value": spec})
	}
	return offset, nil
}

func validTemplateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", templateInvalid("templates.name_required", "/name", nil)
	case utf8.RuneCountInString(name) > MaxTemplateNameLength:
		return "", templateInvalid("templates.name_too_long", "/name",
			map[string]string{"maximum": strconv.Itoa(MaxTemplateNameLength)})
	}
	return name, nil
}

func validTemplateDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > MaxTemplateDescriptionLength {
		return "", templateInvalid("templates.description_too_long", "/description",
			map[string]string{"maximum": strconv.Itoa(MaxTemplateDescriptionLength)})
	}
	return description, nil
}

// templateInvalid is the one refusal shape, so that every way of getting a template wrong answers
// with a stable code and the path of the node that caused it.
func templateInvalid(code, path string, params map[string]string) error {
	refusal := shared.ErrValidation.WithDetail(code)
	if params != nil {
		refusal = refusal.WithParams(params)
	}
	return refusal.WithFields(shared.FieldError{Path: path, Code: code, Params: params})
}

// templateChanges diffs two templates field by field. The tree is one field: two devices editing
// two branches of one tree are editing one shape, and merging them per branch would produce a
// shape neither of them wrote (offline-sync.md §4.2).
func templateChanges(from, to Template) []FieldChange {
	var changes []FieldChange
	appendChange := func(field, before, after string) {
		if before != after {
			changes = append(changes, FieldChange{Field: field, From: before, To: after})
		}
	}
	appendChange(FieldTemplateName, from.Name, to.Name)
	appendChange(FieldTemplateDescription, from.Description, to.Description)
	appendChange(FieldTemplateNodes, treeFingerprint(from.Root), treeFingerprint(to.Root))
	return changes
}

// treeFingerprint spells a tree for the change log: the shape and the titles, in the order they
// are written. Enough for a client to see *that* the tree moved, which is what a change log entry
// about a whole document can say - the document itself travels in the payload.
func treeFingerprint(root TemplateNode) string {
	var parts []string
	for _, node := range flatten(root) {
		parts = append(parts, string(node.Type)+":"+node.Title)
	}
	return strings.Join(parts, "|")
}
