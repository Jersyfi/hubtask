// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Spec is one query: where to look, what to keep, in which order, how grouped, and how much.
//
// It is the whole of what `POST /items:query` means, and what a saved view will store when 0.5.0
// gets one (domain-model.md §5). Everything in it has been through the grammar - a Spec that a use
// case handed to an adapter names only fields this installation serves, only operators those
// fields permit, and only values of the kind they hold.
type Spec struct {
	Scope  Scope
	Filter *Node
	// Sort is never empty by the time an adapter sees it: Sorting fills in the manual order when
	// the caller named none, and the adapter completes every sort with the identifier
	// (api-guidelines.md §3, "sorting always ends implicitly on id ASC").
	Sort    []SortTerm
	GroupBy GroupBy
	// IncludeArchived and IncludeTrashed widen what a query sees. Both default to false, which is
	// what makes the plain query mean "the work that is live".
	IncludeArchived bool
	IncludeTrashed  bool
	Cursor          string
	Size            int
	Count           CountMode
}

// Scope is what a query is anchored to.
//
// Required, and exactly one of the two. An unanchored query is a scan of every item a tenant has,
// and worse than that it is a question authorisation cannot answer in one step: a membership is
// held on a hub or a collection (domain-model.md §3.2), so "everything I may see" would be a
// permission check per row. Naming the anchor makes the check one question with one answer, and a
// refusal a refusal rather than a short page.
type Scope struct {
	// ContainerID is a hub or a collection.
	ContainerID shared.ID
	// ItemID is an entry whose subtree is searched.
	ItemID shared.ID
	// IncludeDescendants searches the whole subtree rather than one level. True unless the caller
	// said otherwise, because the whole subtree is what a view usually means.
	IncludeDescendants bool
}

// SortTerm is one ordering, most significant first.
type SortTerm struct {
	Field      Field
	Descending bool
	// NullsFirst places the entries that have no value. Only meaningful for a nullable field, and
	// harmless on any other - a column that is never null has nowhere to put them.
	NullsFirst bool
}

// GroupBy is the board projection: one group per distinct value of one field.
type GroupBy struct {
	Field Field
	// LimitPerGroup is how many entries a group carries. A board column shows a screenful and
	// pages for the rest, which is why this is per group rather than per query.
	LimitPerGroup int
}

// IsZero reports whether the query is ungrouped.
func (g GroupBy) IsZero() bool { return g.Field.Name == "" }

// CountMode says whether the caller wants to know how large the whole result is.
type CountMode string

const (
	// CountNone is the default: paging tells a client whether there is more, which is what a list
	// needs, and counting the rest costs a second pass over them.
	CountNone CountMode = "none"
	// CountExact counts, in a second query with the same filter.
	CountExact CountMode = "exact"
	// CountEstimated is in the contract and is not served. It is refused by name rather than
	// answered with a null total: this API says what it cannot do (api-guidelines.md §6).
	CountEstimated CountMode = "estimated"
)

// The bounds of the parts of a query that are not the filter.
const (
	// MaxSortTerms bounds an ordering. Four keys is more than any view has ever needed, and each
	// one is a comparison the keyset cursor has to carry.
	MaxSortTerms = 4
	// DefaultGroupSize and MaxGroupSize bound a board column, as the contract states them.
	DefaultGroupSize = 50
	MaxGroupSize     = 200
)

// ParseScope reads the anchor and refuses a query that names none or names two.
func ParseScope(containerID, itemID shared.ID, includeDescendants bool, path string) (Scope, error) {
	switch {
	case containerID.IsZero() && itemID.IsZero():
		return Scope{}, fieldError(path, "query.scope_required", nil)
	case !containerID.IsZero() && !itemID.IsZero():
		// Not resolved in the caller's favour. A query naming both a collection and an entry has
		// two ideas of what it is looking in, and answering one of them is how a client comes to
		// believe it searched the other.
		return Scope{}, fieldError(path, "query.scope_ambiguous", nil)
	}
	return Scope{
		ContainerID: containerID, ItemID: itemID, IncludeDescendants: includeDescendants,
	}, nil
}

// ParseSort reads the ordering, or answers the manual order when the caller named none.
//
// The default is `order_key ASC` - the order somebody dragged the entries into, which is what
// every board and every list is drawn in. A query with no sort at all would have the database's
// order, which is no order and would make the cursor meaningless.
func ParseSort(raw any, path string) ([]SortTerm, error) {
	if raw == nil {
		return defaultSort(), nil
	}
	terms, ok := raw.([]any)
	if !ok {
		return nil, fieldError(path, "query.sort_malformed", nil)
	}
	if len(terms) == 0 {
		return defaultSort(), nil
	}
	if len(terms) > MaxSortTerms {
		return nil, fieldError(path, "query.sort_too_long", map[string]string{
			"maximum": strconv.Itoa(MaxSortTerms),
		})
	}

	sort := make([]SortTerm, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for index, raw := range terms {
		term, err := parseSortTerm(raw, path+"/"+strconv.Itoa(index))
		if err != nil {
			return nil, err
		}
		if seen[term.Field.Name] {
			// The second mention of a field can never change the order the first one established,
			// so it is a client that meant something else - most often a second field it forgot
			// to name.
			return nil, fieldError(path+"/"+strconv.Itoa(index), "query.sort_duplicated",
				map[string]string{"field": term.Field.Name})
		}
		seen[term.Field.Name] = true
		sort = append(sort, term)
	}
	return sort, nil
}

func defaultSort() []SortTerm {
	manual, _ := FieldByName(FieldOrderKey)
	return []SortTerm{{Field: manual}}
}

func parseSortTerm(raw any, path string) (SortTerm, error) {
	document, ok := raw.(map[string]any)
	if !ok {
		return SortTerm{}, fieldError(path, "query.sort_malformed", nil)
	}

	name, _ := document["field"].(string)
	target, err := field(name, path+"/field")
	if err != nil {
		return SortTerm{}, err
	}
	if !target.Sortable {
		return SortTerm{}, fieldError(path+"/field", "query.field_not_sortable", map[string]string{
			"field": target.Name,
		})
	}

	term := SortTerm{Field: target}
	switch direction := strings.TrimSpace(stringOf(document["dir"])); direction {
	case "", "ASC":
	case "DESC":
		term.Descending = true
	default:
		return SortTerm{}, fieldError(path+"/dir", "query.sort_direction_unknown", map[string]string{
			"value": direction,
		})
	}
	switch nulls := strings.TrimSpace(stringOf(document["nulls"])); nulls {
	case "", "LAST":
	case "FIRST":
		term.NullsFirst = true
	default:
		return SortTerm{}, fieldError(path+"/nulls", "query.sort_nulls_unknown", map[string]string{
			"value": nulls,
		})
	}
	return term, nil
}

// ParseGroupBy reads the board projection. An absent group_by is an ungrouped query.
func ParseGroupBy(raw any, path string) (GroupBy, error) {
	if raw == nil {
		return GroupBy{}, nil
	}
	document, ok := raw.(map[string]any)
	if !ok {
		return GroupBy{}, fieldError(path, "query.group_by_malformed", nil)
	}

	name, _ := document["field"].(string)
	target, err := field(name, path+"/field")
	if err != nil {
		return GroupBy{}, err
	}
	if !target.Groupable {
		// Grouping by a timestamp or a title is a question about buckets nobody has defined -
		// by day, by week, by first letter - and inventing one of them here would be a projection
		// the client never asked for.
		return GroupBy{}, fieldError(path+"/field", "query.field_not_groupable", map[string]string{
			"field": target.Name,
		})
	}

	limit := DefaultGroupSize
	if raw, present := document["limit_per_group"]; present && raw != nil {
		number, ok := integerOf(raw)
		if !ok {
			return GroupBy{}, fieldError(path+"/limit_per_group", "query.value_type_invalid", nil)
		}
		limit = GroupSize(int(number))
	}
	return GroupBy{Field: target, LimitPerGroup: limit}, nil
}

// GroupSize clamps a column's size into the contract's range, for the reason the page size is
// clamped rather than refused: a client asking for more than it can have wants as many as it can.
func GroupSize(requested int) int {
	switch {
	case requested < 1:
		return DefaultGroupSize
	case requested > MaxGroupSize:
		return MaxGroupSize
	default:
		return requested
	}
}

// ParseCount reads what the caller wants to know about the size of the whole result.
func ParseCount(raw string, path string) (CountMode, error) {
	switch mode := CountMode(strings.TrimSpace(raw)); mode {
	case "", CountNone:
		return CountNone, nil
	case CountExact:
		return CountExact, nil
	case CountEstimated:
		return "", fieldError(path, "query.count_not_supported", map[string]string{
			"value": string(CountEstimated),
		})
	default:
		return "", fieldError(path, "query.count_unknown", map[string]string{"value": string(mode)})
	}
}

// Validate is the last question, the one that needs the whole query rather than one part of it.
func (s Spec) Validate(path string) error {
	if s.Cursor != "" && !s.GroupBy.IsZero() {
		// A grouped query has no single walk to continue: each group carries its own cursor, and
		// one boundary applied to all of them would continue every column at the position of
		// whichever column produced it. A client pages a column by asking for that column - the
		// group's key as a filter, the group's cursor as the cursor.
		return fieldError(path+"/page/cursor", "query.cursor_not_grouped", nil)
	}
	if len(s.Sort) == 0 {
		return fieldError(path+"/sort", "query.sort_malformed", nil)
	}
	return nil
}

func stringOf(raw any) string {
	value, _ := raw.(string)
	return value
}
