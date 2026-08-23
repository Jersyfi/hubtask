// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package query turns a validated query into the statement that answers it.
//
// The one place in this system where SQL is not written in advance, and the reason ADR-0026 exists.
// The rule it works under is exact: every character of the statement is a constant of this package,
// chosen by a switch over a type the domain validated, and every value is bound as a parameter. No
// byte that arrived in a request reaches the text.
//
// Which is why this is a package of its own rather than a file in the adapter beside it. The
// boundary is enforced rather than remembered - the depguard rule in .golangci.yml refuses `fmt`
// and the driver here, so the two ways of accidentally putting a value into a statement are not
// available to write in the first place - and FuzzCompile proves the result.
package query

import (
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// Statement is a compiled query: text, and the values it is missing.
type Statement struct {
	SQL  string
	Args []any
}

// Boundary is a decoded cursor: one key per sort term, and the identifier that breaks the tie.
//
// The keys are text because that is what a cursor carries; the compiler turns each one back into
// the type its sort field holds, because a boundary compared as text against a timestamp column is
// a comparison the database refuses at run time.
type Boundary struct {
	Keys []string
	ID   shared.ID
}

// IsZero reports the first page.
func (b Boundary) IsZero() bool { return b.ID.IsZero() }

// The two projections. Written out rather than assembled, and in the order of sqlc's
// FindWorkItemRow, which is what the adapter scans into: a column added to one and not the other is
// a mismatch the scan reports on the first row rather than a wrong answer.
const (
	itemColumns = `wi.id, wi.tenant_id, wi.collection_id, wi.type, wi.parent_id, wi.path, ` +
		`wi.depth, wi.title, wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, ` +
		`wi.bucket_id, wi.order_key, wi.assignee_id, wi.archived_at, wi.deleted_at, ` +
		`wi.trash_batch_id, ` +
		`wi.created_by, wi.created_at, wi.updated_at, wi.version`

	groupedColumns = `id, tenant_id, collection_id, type, parent_id, path, depth, title, notes, ` +
		`is_completed, completed_at, completed_by, bucket_id, order_key, assignee_id, ` +
		`archived_at, deleted_at, trash_batch_id, created_by, created_at, updated_at, version`
)

// The two prefixes a column expression can carry: the table alias inside the query, and nothing at
// all in the outer select of a grouped one, where the subquery is the only relation in scope.
const (
	itemPrefix  = "wi."
	groupPrefix = ""
)

// Rows compiles an ungrouped query: one page of entries in one order.
func Rows(search repository.ItemSearch, boundary Boundary, probe int) (Statement, error) {
	b := &builder{}

	b.write(`SELECT `, itemColumns, ` FROM work_item wi WHERE `)
	b.predicates(search)
	if !boundary.IsZero() {
		b.write(` AND (`)
		b.keyset(search.Spec.Sort, boundary)
		b.write(`)`)
	}
	b.write(` ORDER BY `)
	b.ordering(search.Spec.Sort, itemPrefix)
	b.write(`, wi.id LIMIT `)
	b.param(int64(probe))

	return b.statement()
}

// Groups compiles the board projection: the first entries of every distinct value of one field.
//
// A window function rather than a query per group, because the number of groups is data - a board
// has as many columns as somebody made - and a query per column is a round trip per column. The
// row number is computed in the same order the entries are returned in, so a group's page is the
// beginning of that group in that order, and its cursor continues it.
func Groups(search repository.ItemSearch, probe int) (Statement, error) {
	group, prefixed := column(search.Spec.GroupBy.Field, itemPrefix)
	if !prefixed {
		return Statement{}, errFieldNotCompilable(search.Spec.GroupBy.Field)
	}

	b := &builder{}
	b.write(`SELECT `, groupedColumns, ` FROM (SELECT `, itemColumns, `, row_number() OVER (`,
		`PARTITION BY `, group, ` ORDER BY `)
	b.ordering(search.Spec.Sort, itemPrefix)
	b.write(`, wi.id) AS rn FROM work_item wi WHERE `)
	b.predicates(search)
	b.write(`) g WHERE g.rn <= `)
	b.param(int64(probe))
	b.write(` ORDER BY `)

	// The groups come in the grouping field's own order, and the entries that have no value at all
	// come last: a board draws that column at the end, and a client that reads the groups in order
	// gets the same layout every time.
	outer, _ := column(search.Spec.GroupBy.Field, groupPrefix)
	b.write(outer, ` ASC NULLS LAST, `)
	b.ordering(search.Spec.Sort, groupPrefix)
	b.write(`, id`)

	return b.statement()
}

// Count compiles the second pass an exact count costs: the same filter, without order, page or
// projection.
func Count(search repository.ItemSearch) (Statement, error) {
	b := &builder{}

	if group, ok := column(search.Spec.GroupBy.Field, itemPrefix); ok {
		// The key comes back as text whatever the column holds, because a group key is text
		// everywhere above this line: a uuid, an item type and a boolean all become the string the
		// rows themselves are keyed by, and the caller matches the two against each other.
		b.write(`SELECT `, group, `::text, count(*)::bigint FROM work_item wi WHERE `)
		b.predicates(search)
		b.write(` GROUP BY 1`)
		return b.statement()
	}

	b.write(`SELECT count(*)::bigint FROM work_item wi WHERE `)
	b.predicates(search)
	return b.statement()
}

// predicates writes what every shape of the query shares: the scope, the lifecycle, the filter.
func (b *builder) predicates(search repository.ItemSearch) {
	b.scope(search.Anchor)
	b.restriction(search.RestrictTo)
	b.lifecycle(search.Spec)
	if search.Spec.Filter != nil {
		b.write(` AND (`)
		b.node(*search.Spec.Filter)
		b.write(`)`)
	}
}

// restriction narrows the answer to the entries the caller may see, when that is fewer than the
// anchor holds (C-04).
//
// A bound array rather than a list written into the statement: no byte of it becomes SQL text
// (rule 9, T-06). Empty means no restriction, which is what every caller holding a role on the
// anchor passes - the predicate is not written at all then, so the ordinary query is unchanged.
func (b *builder) restriction(ids []shared.ID) {
	if len(ids) == 0 {
		return
	}

	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	b.write(` AND wi.id = ANY(`)
	b.param(values)
	b.write(`::uuid[])`)
}

// scope is the anchor the use case resolved, as a predicate.
func (b *builder) scope(anchor repository.Anchor) {
	switch anchor.Kind {
	case repository.AnchorCollection:
		b.write(`wi.collection_id = `)
		b.uuid(anchor.CollectionID)
		if !anchor.IncludeDescendants {
			b.write(` AND wi.parent_id IS NULL`)
		}

	case repository.AnchorHub:
		// The hub's collections, as a subquery rather than a list the use case read first: the list
		// would be a second round trip and a second thing to keep in step with what a hub contains.
		// Trashed collections are left out - their items are in the trash with them (I-C2).
		b.write(`wi.collection_id IN (SELECT c.id FROM container c WHERE c.parent_id = `)
		b.uuid(anchor.HubID)
		b.write(` AND c.deleted_at IS NULL)`)

	case repository.AnchorItem:
		// The collection as well as the path: it is the leading column of the index the scope reads
		// through, and a prefix scan without it spans every path in the tenant.
		b.write(`wi.collection_id = `)
		b.uuid(anchor.CollectionID)
		if !anchor.IncludeDescendants {
			b.write(` AND wi.parent_id = `)
			b.uuid(anchor.ItemID)
			return
		}
		// LIKE with a prefix, which is what wi_path_idx (text_pattern_ops) serves. The pattern is a
		// parameter and the prefix is a chain of identifiers and separators, so it carries no
		// wildcard of its own. The anchor is not in its own subtree here: "what is inside this
		// entry" does not include the entry, exactly as one level below it does not.
		b.write(` AND wi.path LIKE `)
		b.param(anchor.PathPrefix + `%`)
		b.write(` AND wi.id <> `)
		b.uuid(anchor.ItemID)

	default:
		b.fail(shared.ErrInternal.WithDetail("query.anchor_unknown"))
	}
}

// lifecycle is the pair of stamps that decide whether an entry is live.
func (b *builder) lifecycle(spec view.Spec) {
	if !spec.IncludeTrashed {
		b.write(` AND wi.deleted_at IS NULL`)
	}
	if !spec.IncludeArchived {
		b.write(` AND wi.archived_at IS NULL`)
	}
}

// node writes one node of the filter, and recurses. The tree it walks has been through the grammar,
// so the depth is bounded and the recursion terminates (view.MaxFilterDepth).
func (b *builder) node(node view.Node) {
	if !node.IsLeaf() {
		b.combination(node)
		return
	}
	b.leaf(node)
}

func (b *builder) combination(node view.Node) {
	if node.Op == view.OpNot {
		b.write(`NOT (`)
		b.node(node.Nodes[0])
		b.write(`)`)
		return
	}

	separator := ` AND `
	if node.Op == view.OpOr {
		separator = ` OR `
	}
	for index, child := range node.Nodes {
		if index > 0 {
			b.write(separator)
		}
		b.write(`(`)
		b.node(child)
		b.write(`)`)
	}
}

// leaf writes one comparison. The fields with no column of their own are answered first, then the
// ordinary ones by operator.
func (b *builder) leaf(node view.Node) {
	switch node.Field.Name {
	case view.FieldLabels:
		b.labels(node)
		return
	case view.FieldMembers:
		b.members(node)
		return
	case view.FieldText:
		// The same text search configuration the generated column was built with. A query parsed
		// under a different one would match nothing and look like an empty result.
		b.write(`wi.search_vector @@ websearch_to_tsquery('simple', `)
		b.text(node.Values[0])
		b.write(`)`)
		return
	}

	name, ok := column(node.Field, itemPrefix)
	if !ok {
		b.fail(errFieldNotCompilable(node.Field))
		return
	}

	switch node.Op {
	case view.OpIsNull:
		b.write(name, ` IS NULL`)
	case view.OpEq:
		b.write(name, ` = `)
		b.value(node.Field, node.Values[0])
	case view.OpNeq:
		// IS DISTINCT FROM rather than <>, so that an entry with no value at all counts as "not
		// that one". `bucket_id NEQ x` asked of an entry on no board is a question somebody expects
		// "yes" to, and three-valued logic would drop the row instead.
		b.write(name, ` IS DISTINCT FROM `)
		b.value(node.Field, node.Values[0])
	case view.OpLt:
		b.write(name, ` < `)
		b.value(node.Field, node.Values[0])
	case view.OpLte:
		b.write(name, ` <= `)
		b.value(node.Field, node.Values[0])
	case view.OpGt:
		b.write(name, ` > `)
		b.value(node.Field, node.Values[0])
	case view.OpGte:
		b.write(name, ` >= `)
		b.value(node.Field, node.Values[0])
	case view.OpBetween:
		b.write(name, ` BETWEEN `)
		b.value(node.Field, node.Values[0])
		b.write(` AND `)
		b.value(node.Field, node.Values[1])
	case view.OpIn:
		b.write(name, ` = ANY(`)
		b.array(node.Field, node.Values)
		b.write(`)`)
	case view.OpNotIn:
		b.write(`(`, name, ` IS NULL OR `, name, ` <> ALL(`)
		b.array(node.Field, node.Values)
		b.write(`))`)
	case view.OpContains:
		// position() rather than LIKE: a substring search through LIKE would have to escape the
		// wildcards in the value, and an escape that is forgotten turns a search for "50%" into a
		// search for everything. Case-insensitive, because a person typing into a search box means
		// the word rather than the spelling.
		b.write(`position(lower(`)
		b.text(node.Values[0])
		b.write(`) IN lower(coalesce(`, name, `, ''))) > 0`)
	case view.OpStartsWith:
		b.write(`starts_with(lower(`, name, `), lower(`)
		b.text(node.Values[0])
		b.write(`))`)
	default:
		b.fail(errOperatorNotCompilable(node.Op))
	}
}

// labels is the one relation a filter reaches into: the labels an entry carries.
//
// An EXISTS against the join table rather than an array on the item, which is the shape
// domain-model.md §6 chose - "filterability takes precedence". The label's own deletion stamp is
// checked, so a deleted label stops filtering the moment it is deleted and starts again if it is
// brought back, without anything having to rewrite the memberships.
func (b *builder) labels(node view.Node) {
	switch node.Op {
	case view.OpContains, view.OpContainsAny:
		b.write(`EXISTS (SELECT 1 FROM item_label il JOIN label l ON l.id = il.label_id `,
			`WHERE il.item_id = wi.id AND l.deleted_at IS NULL AND il.label_id = ANY(`)
		b.array(node.Field, node.Values)
		b.write(`))`)

	case view.OpContainsAll:
		// Counted rather than one EXISTS per label: the number of labels is data, and a predicate
		// whose size follows the request is one a client can make expensive.
		b.write(`(SELECT count(DISTINCT il.label_id) FROM item_label il `,
			`JOIN label l ON l.id = il.label_id `,
			`WHERE il.item_id = wi.id AND l.deleted_at IS NULL AND il.label_id = ANY(`)
		b.array(node.Field, node.Values)
		b.write(`)) = `)
		b.param(int64(len(node.Values)))

	default:
		b.fail(errOperatorNotCompilable(node.Op))
	}
}

// members is the second relation a filter reaches into: the accounts an entry carries.
//
// The labels' shape without the join. A label has a deletion stamp and stops filtering the moment
// it is deleted; an account has none - it is disabled or it is gone, and a gone one takes its rows
// with it through the tenant-scoped foreign key - so there is no second table whose state could
// hide a row here.
func (b *builder) members(node view.Node) {
	switch node.Op {
	case view.OpContains, view.OpContainsAny:
		b.write(`EXISTS (SELECT 1 FROM item_member im `,
			`WHERE im.item_id = wi.id AND im.account_id = ANY(`)
		b.array(node.Field, node.Values)
		b.write(`))`)

	case view.OpContainsAll:
		// Counted rather than one EXISTS per account, for the reason the labels are counted: the
		// number of members is data, and a predicate whose size follows the request is one a client
		// can make expensive.
		b.write(`(SELECT count(DISTINCT im.account_id) FROM item_member im `,
			`WHERE im.item_id = wi.id AND im.account_id = ANY(`)
		b.array(node.Field, node.Values)
		b.write(`)) = `)
		b.param(int64(len(node.Values)))

	default:
		b.fail(errOperatorNotCompilable(node.Op))
	}
}

// ordering writes the ORDER BY terms, without the identifier the caller appends.
func (b *builder) ordering(sort []view.SortTerm, prefix string) {
	for index, term := range sort {
		if index > 0 {
			b.write(`, `)
		}
		name, ok := column(term.Field, prefix)
		if !ok {
			b.fail(errFieldNotCompilable(term.Field))
			return
		}
		b.write(name, collationOf(term.Field))
		if term.Descending {
			b.write(` DESC`)
		} else {
			b.write(` ASC`)
		}
		if term.Field.Nullable {
			if term.NullsFirst {
				b.write(` NULLS FIRST`)
			} else {
				b.write(` NULLS LAST`)
			}
		}
	}
}

// keyset writes "strictly after the boundary" in the sort's own order.
//
// Two shapes, and the simple one is not an optimisation for its own sake. A sort that is entirely
// ascending over columns that are never null is a row comparison, which is the one form an index
// can seek with - and it is the default sort of this endpoint, the manual order every board and
// list is drawn in. Everything else expands into the nested form below, which is correct for mixed
// directions and for nulls and costs a scan of the range.
func (b *builder) keyset(sort []view.SortTerm, boundary Boundary) {
	if len(boundary.Keys) != len(sort) {
		b.fail(errCursorInvalid)
		return
	}

	if seekable(sort) {
		b.write(`(`)
		for _, term := range sort {
			name, _ := column(term.Field, itemPrefix)
			b.write(name, collationOf(term.Field), `, `)
		}
		b.write(`wi.id) > (`)
		for index, term := range sort {
			b.boundaryValue(term, boundary.Keys[index])
			b.write(collationOf(term.Field), `, `)
		}
		b.uuid(boundary.ID)
		b.write(`)`)
		return
	}

	b.expandedKeyset(sort, boundary, 0)
}

// expandedKeyset is the general form: after in this term, or equal in it and after in the rest.
func (b *builder) expandedKeyset(sort []view.SortTerm, boundary Boundary, index int) {
	if index == len(sort) {
		b.write(`wi.id > `)
		b.uuid(boundary.ID)
		return
	}

	term := sort[index]
	name, ok := column(term.Field, itemPrefix)
	if !ok {
		b.fail(errFieldNotCompilable(term.Field))
		return
	}

	b.write(`(`)
	b.after(name, term, boundary.Keys[index])
	b.write(` OR (`, name, ` IS NOT DISTINCT FROM `)
	b.boundaryValue(term, boundary.Keys[index])
	b.write(` AND `)
	b.expandedKeyset(sort, boundary, index+1)
	b.write(`))`)
}

// after is "strictly later than the boundary in this term's order", nulls included.
//
// The CASE is what a plain comparison cannot express: where the nulls sit is part of the ordering,
// so whether a null row is after the boundary depends on which side of it the nulls were placed and
// on whether the boundary is itself a null.
func (b *builder) after(name string, term view.SortTerm, key string) {
	comparison := ` > `
	if term.Descending {
		comparison = ` < `
	}
	if !term.Field.Nullable {
		b.write(name, comparison)
		b.boundaryValue(term, key)
		return
	}

	b.write(`(CASE `)
	if term.NullsFirst {
		b.write(`WHEN `, name, ` IS NULL THEN false WHEN `)
		b.boundaryValue(term, key)
		b.write(` IS NULL THEN true `)
	} else {
		b.write(`WHEN `)
		b.boundaryValue(term, key)
		b.write(` IS NULL THEN false WHEN `, name, ` IS NULL THEN true `)
	}
	b.write(`ELSE `, name, comparison)
	b.boundaryValue(term, key)
	b.write(` END)`)
}

// seekable reports whether the sort can be compared as a row: every term ascending, and no term on
// a column that can be null.
func seekable(sort []view.SortTerm) bool {
	for _, term := range sort {
		if term.Descending || term.Field.Nullable {
			return false
		}
	}
	return true
}
