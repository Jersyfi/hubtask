// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SavedView is a stored query with the layout it is drawn in (domain-model.md §3.5, D-07).
//
// It lives beside the grammar deliberately: this package decides what a query *means*, and a
// saved view is a query somebody kept - validated by the same catalogue at write time, because a
// bookmark that fails when it is opened is one nobody can repair. What the server never does is
// interpret the layout or the visible fields: both are the client's vocabulary, stored and echoed
// (api-guidelines.md §3), which is what makes new frontend views possible without a backend
// change.
type SavedView struct {
	ID       shared.ID
	TenantID shared.ID
	// ScopeType is where the view lives, which is who it can be shared with: a container's
	// members, the workspace, or - for ACCOUNT - nobody but its owner.
	ScopeType ViewScope
	// ScopeID is the container the scope names, the owner's account for ACCOUNT, and empty for
	// TENANT.
	ScopeID shared.ID
	// OwnerID is who saved it. Ownership is the qualifier most of the authorisation rests on: a
	// PRIVATE view is visible to its owner alone, whatever its scope.
	OwnerID shared.ID
	Name    string
	// Layout is one of the declared set, stored and never consulted.
	Layout Layout
	// Query is the document of POST /items:query, exactly as sent. Validated at write against
	// the same grammar and bounds as an ad-hoc query, stored uninterpreted.
	Query map[string]any
	// Grouping is the client's grouping hint, an object the server stores whole.
	Grouping map[string]any
	// VisibleFields is the client's column choice. Shape-checked, never resolved against the
	// catalogue: a custom field's key or a field a later version serves are both legitimate
	// spellings this version cannot enumerate.
	VisibleFields []string
	Sharing       Sharing
	CreatedAt     time.Time
	Version       int
}

// Layout is how a client draws a view. A closed set the server declares and never consults - the
// declaration is what /meta/capabilities publishes as view_layouts, so a client knows the
// spellings this installation stores.
type Layout string

const (
	LayoutListCollapsed Layout = "LIST_COLLAPSED"
	LayoutListExpanded  Layout = "LIST_EXPANDED"
	LayoutKanban        Layout = "KANBAN"
	LayoutTimeline      Layout = "TIMELINE"
)

var layouts = [...]Layout{LayoutListCollapsed, LayoutListExpanded, LayoutKanban, LayoutTimeline}

// Layouts returns the declared set, in a stable order. `/meta/capabilities` answers it verbatim.
func Layouts() []Layout { return layouts[:] }

// Valid reports whether the layout is one of the declared set.
func (l Layout) Valid() bool {
	for _, known := range layouts {
		if known == l {
			return true
		}
	}
	return false
}

// ViewScope is where a view lives.
type ViewScope string

const (
	ViewScopeTenant     ViewScope = "TENANT"
	ViewScopeHub        ViewScope = "HUB"
	ViewScopeCollection ViewScope = "COLLECTION"
	ViewScopeAccount    ViewScope = "ACCOUNT"
)

var viewScopes = [...]ViewScope{ViewScopeTenant, ViewScopeHub, ViewScopeCollection, ViewScopeAccount}

// Valid reports whether the scope is one of the defined ones.
func (s ViewScope) Valid() bool {
	for _, known := range viewScopes {
		if known == s {
			return true
		}
	}
	return false
}

// Sharing is who sees a view beyond its owner.
type Sharing string

const (
	SharingPrivate Sharing = "PRIVATE"
	SharingScope   Sharing = "SCOPE"
	// SharingPublicLink is declared - the check constraint has carried it since 0001_init - and
	// refused by name: the calendar feed is this version's only public reader and carries its own
	// token, and a browsable public link is a product decision no task takes in passing (D-07).
	SharingPublicLink Sharing = "PUBLIC_LINK"
)

// NewSharing reads a sharing value and refuses what this version does not serve.
func NewSharing(raw string) (Sharing, error) {
	switch Sharing(raw) {
	case SharingPrivate, SharingScope:
		return Sharing(raw), nil
	case SharingPublicLink:
		return "", shared.ErrValidation.
			WithDetail("views.public_link_not_available").
			WithFields(shared.FieldError{Path: "/sharing", Code: "views.public_link_not_available"})
	default:
		return "", shared.ErrValidation.
			WithDetail("views.sharing_unknown").
			WithParams(map[string]string{"value": raw}).
			WithFields(shared.FieldError{
				Path: "/sharing", Code: "views.sharing_unknown",
				Params: map[string]string{"value": raw},
			})
	}
}

// The bounds a view's own fields keep. The query inside is bounded by the grammar's caps.
const (
	MaxViewNameLength    = 200
	MaxVisibleFields     = 50
	MaxVisibleFieldChars = 100
)

// NewSavedViewInput is what a view is made of. The scope has already been resolved by the use
// case - which container the identifier names, and that it is of the type the scope claims - so
// what arrives here is checked for meaning, not for existence.
type NewSavedViewInput struct {
	ID        shared.ID
	TenantID  shared.ID
	ScopeType ViewScope
	ScopeID   shared.ID
	OwnerID   shared.ID

	Name          string
	Layout        string
	Query         map[string]any
	Grouping      map[string]any
	VisibleFields []string
	Sharing       Sharing

	Now time.Time
}

// NewSavedView builds a view and checks its invariants.
func NewSavedView(in NewSavedViewInput) (SavedView, error) {
	if !in.ScopeType.Valid() {
		return SavedView{}, shared.ErrValidation.
			WithDetail("views.scope_unknown").
			WithParams(map[string]string{"value": string(in.ScopeType)}).
			WithFields(shared.FieldError{
				Path: "/scope_type", Code: "views.scope_unknown",
				Params: map[string]string{"value": string(in.ScopeType)},
			})
	}
	// TENANT names the whole workspace and nothing narrower; every other scope names something.
	if in.ScopeType == ViewScopeTenant && !in.ScopeID.IsZero() {
		return SavedView{}, shared.ErrValidation.
			WithDetail("views.scope_id_not_allowed").
			WithFields(shared.FieldError{Path: "/scope_id", Code: "views.scope_id_not_allowed"})
	}
	if in.ScopeType != ViewScopeTenant && in.ScopeID.IsZero() {
		return SavedView{}, shared.ErrValidation.
			WithDetail("views.scope_id_required").
			WithFields(shared.FieldError{Path: "/scope_id", Code: "views.scope_id_required"})
	}

	name, err := viewName(in.Name)
	if err != nil {
		return SavedView{}, err
	}
	layout, err := viewLayout(in.Layout)
	if err != nil {
		return SavedView{}, err
	}
	query, err := ValidatedViewQuery(in.Query)
	if err != nil {
		return SavedView{}, err
	}
	grouping, err := viewGrouping(in.Grouping)
	if err != nil {
		return SavedView{}, err
	}
	fields, err := viewVisibleFields(in.VisibleFields)
	if err != nil {
		return SavedView{}, err
	}

	if in.Sharing != SharingPrivate && in.Sharing != SharingScope {
		// The constructor takes the type, not the wire value: an unknown spelling was refused by
		// NewSharing where it arrived, so this is the caller and the model disagreeing.
		return SavedView{}, shared.ErrInternal.WithDetail("views.sharing_undecided")
	}
	if in.ID.IsZero() || in.TenantID.IsZero() || in.OwnerID.IsZero() {
		return SavedView{}, shared.ErrInternal.WithDetail("views.identity_incomplete")
	}

	return SavedView{
		ID:            in.ID,
		TenantID:      in.TenantID,
		ScopeType:     in.ScopeType,
		ScopeID:       in.ScopeID,
		OwnerID:       in.OwnerID,
		Name:          name,
		Layout:        layout,
		Query:         query,
		Grouping:      grouping,
		VisibleFields: fields,
		Sharing:       in.Sharing,
		CreatedAt:     in.Now,
		Version:       1,
	}, nil
}

// ViewAttributes is what an update may change. A nil member means "leave it alone", which is what
// an absent member of a JSON Merge Patch says - the same distinction ItemAttributes keeps, and
// for the same reason. The scope and the sharing are deliberately absent: where a view lives is
// decided at creation, and sharing is its own act with its own permission.
type ViewAttributes struct {
	Name   *string
	Layout *string
	// Query replaces the stored document whole: a query is one statement, and merging two
	// half-queries would produce one nobody wrote. Nil leaves it alone.
	Query map[string]any
	// Grouping replaces the stored hint whole, for the same reason. Nil leaves it alone.
	Grouping map[string]any
	// VisibleFields replaces the stored list whole. Nil leaves it alone; an empty list is a
	// choice - "show the defaults" - and reaches here as one.
	VisibleFields []string
}

// IsEmpty reports whether the update asks for nothing at all.
func (a ViewAttributes) IsEmpty() bool {
	return a.Name == nil && a.Layout == nil && a.Query == nil && a.Grouping == nil &&
		a.VisibleFields == nil
}

// Updated applies an update and reports whether anything moved. The caller writes nothing, spends
// no version and records nothing for an update that changes nothing - the contract every writer
// here keeps.
func (v SavedView) Updated(attributes ViewAttributes) (SavedView, bool, error) {
	changed := false

	if attributes.Name != nil {
		name, err := viewName(*attributes.Name)
		if err != nil {
			return SavedView{}, false, err
		}
		if name != v.Name {
			v.Name, changed = name, true
		}
	}
	if attributes.Layout != nil {
		layout, err := viewLayout(*attributes.Layout)
		if err != nil {
			return SavedView{}, false, err
		}
		if layout != v.Layout {
			v.Layout, changed = layout, true
		}
	}
	if attributes.Query != nil {
		query, err := ValidatedViewQuery(attributes.Query)
		if err != nil {
			return SavedView{}, false, err
		}
		// Replaced whole rather than diffed: two documents are equal so rarely that comparing
		// them buys nothing a write would not.
		v.Query, changed = query, true
	}
	if attributes.Grouping != nil {
		grouping, err := viewGrouping(attributes.Grouping)
		if err != nil {
			return SavedView{}, false, err
		}
		v.Grouping, changed = grouping, true
	}
	if attributes.VisibleFields != nil {
		fields, err := viewVisibleFields(attributes.VisibleFields)
		if err != nil {
			return SavedView{}, false, err
		}
		v.VisibleFields, changed = fields, true
	}

	return v, changed, nil
}

// Shared returns the view with its sharing decided, and reports whether it moved.
//
// An ACCOUNT-scoped view cannot be shared: its scope names no audience. Refused on the way in
// rather than stored meaninglessly - a stored SCOPE on an account view would be a promise the
// read side never keeps.
func (v SavedView) Shared(sharing Sharing) (SavedView, bool, error) {
	if sharing == SharingScope && v.ScopeType == ViewScopeAccount {
		return SavedView{}, false, shared.ErrValidation.
			WithDetail("views.account_scope_not_shareable").
			WithFields(shared.FieldError{
				Path: "/sharing", Code: "views.account_scope_not_shareable",
			})
	}
	if sharing != SharingPrivate && sharing != SharingScope {
		return SavedView{}, false, shared.ErrInternal.WithDetail("views.sharing_undecided")
	}
	if sharing == v.Sharing {
		return v, false, nil
	}
	v.Sharing = sharing
	return v, true, nil
}

// ValidatedViewQuery checks the stored query document against the grammar an ad-hoc query passes,
// with the same codes - which is the whole point: a saved view refused at write is a correction
// somebody can make, and one refused at read is a broken bookmark (D-07).
//
// Validated, not normalised: the document is stored exactly as sent, placeholders included -
// `@me` in a saved view is the reader at read time, which is what makes one view mean the right
// thing to each of its readers. Members the grammar has no opinion on - `expand`, `page`,
// `include_archived` - are stored as they are: they are bounded again at execution, and a write
// that guessed at their future shapes would refuse documents a later version serves.
func ValidatedViewQuery(raw map[string]any) (map[string]any, error) {
	if raw == nil {
		return nil, shared.ErrValidation.
			WithDetail("views.query_required").
			WithFields(shared.FieldError{Path: "/query", Code: "views.query_required"})
	}

	if rawScope, present := raw["scope"]; present {
		if err := validatedQueryScope(rawScope); err != nil {
			return nil, err
		}
	}
	if rawFilter, present := raw["filter"]; present {
		if _, err := ParseFilter(rawFilter, "/query/filter"); err != nil {
			return nil, err
		}
	}
	if rawSort, present := raw["sort"]; present {
		if _, err := ParseSort(rawSort, "/query/sort"); err != nil {
			return nil, err
		}
	}
	if rawGroup, present := raw["group_by"]; present && rawGroup != nil {
		if _, err := ParseGroupBy(rawGroup, "/query/group_by"); err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// validatedQueryScope checks the document's own anchor: the shape of api-guidelines.md §3, parsed
// by the same rule the query endpoint applies. Existence is not asked here - the anchor is
// resolved by whoever executes the view, under their own authorisation.
func validatedQueryScope(raw any) error {
	document, isObject := raw.(map[string]any)
	if !isObject {
		return shared.ErrValidation.
			WithDetail("query.node_malformed").
			WithFields(shared.FieldError{Path: "/query/scope", Code: "query.node_malformed"})
	}

	containerID, err := scopeIdentifier(document, "container_id")
	if err != nil {
		return err
	}
	itemID, err := scopeIdentifier(document, "item_id")
	if err != nil {
		return err
	}
	include, _ := document["include_descendants"].(bool)

	_, err = ParseScope(containerID, itemID, include, "/query/scope")
	return err
}

func scopeIdentifier(document map[string]any, key string) (shared.ID, error) {
	raw, present := document[key]
	if !present || raw == nil {
		return shared.ID(""), nil
	}
	text, isText := raw.(string)
	if !isText {
		return shared.ID(""), scopeIdentifierError(key)
	}
	id, err := shared.ParseID(text)
	if err != nil {
		return shared.ID(""), scopeIdentifierError(key)
	}
	return id, nil
}

func scopeIdentifierError(key string) error {
	return shared.ErrValidation.
		WithDetail("query.value_type_invalid").
		WithFields(shared.FieldError{
			Path: "/query/scope/" + key, Code: "query.value_type_invalid",
		})
}

// viewName trims and checks the name. One line, like a container's name and for the same reason:
// it survives every layer and then breaks the one that renders it.
func viewName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", shared.ErrValidation.
			WithDetail("views.name_empty").
			WithFields(shared.FieldError{Path: "/name", Code: "views.name_empty"})
	case utf8.RuneCountInString(name) > MaxViewNameLength:
		return "", shared.ErrValidation.
			WithDetail("views.name_too_long").
			WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/name", Code: "views.name_too_long"})
	case strings.ContainsAny(name, "\r\n") || hasControlCharacter(name):
		return "", shared.ErrValidation.
			WithDetail("views.name_malformed").
			WithFields(shared.FieldError{Path: "/name", Code: "views.name_malformed"})
	}
	return name, nil
}

// viewLayout validates against the declared set - and nothing more. Which pixels a KANBAN is
// worth is a question this server has never answered (api-guidelines.md §3).
func viewLayout(raw string) (Layout, error) {
	layout := Layout(strings.TrimSpace(raw))
	if !layout.Valid() {
		return "", shared.ErrValidation.
			WithDetail("views.layout_unknown").
			WithParams(map[string]string{"value": string(layout)}).
			WithFields(shared.FieldError{
				Path: "/layout", Code: "views.layout_unknown",
				Params: map[string]string{"value": string(layout)},
			})
	}
	return layout, nil
}

// viewGrouping stores the client's hint as an object, nil read as the empty one. Its members are
// deliberately not enumerated - the server does not interpret grouping any more than layout -
// but a member the grammar does know, the grouped field, is held to the catalogue: a grouping
// naming a field nothing serves is the same broken bookmark a filter naming one would be.
func viewGrouping(raw map[string]any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	if field, present := raw["field"]; present {
		if _, err := ParseGroupBy(map[string]any{"field": field}, "/grouping"); err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// viewVisibleFields checks the shape and nothing else: a custom field's key and a field a later
// version serves are both spellings this catalogue cannot enumerate, so the list is the client's
// - bounded, one line each, and otherwise its own.
func viewVisibleFields(raw []string) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	if len(raw) > MaxVisibleFields {
		return nil, shared.ErrValidation.
			WithDetail("views.visible_fields_too_many").
			WithParams(map[string]string{"maximum": "50"}).
			WithFields(shared.FieldError{
				Path: "/visible_fields", Code: "views.visible_fields_too_many",
			})
	}
	for _, field := range raw {
		if field == "" || utf8.RuneCountInString(field) > MaxVisibleFieldChars ||
			hasControlCharacter(field) {
			return nil, shared.ErrValidation.
				WithDetail("views.visible_field_malformed").
				WithFields(shared.FieldError{
					Path: "/visible_fields", Code: "views.visible_field_malformed",
				})
		}
	}
	return raw, nil
}

// hasControlCharacter mirrors the rule every stored name keeps (I-W7's spirit): nothing below
// space, nothing that steers a terminal or a log line.
func hasControlCharacter(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7F {
			return true
		}
	}
	return false
}
