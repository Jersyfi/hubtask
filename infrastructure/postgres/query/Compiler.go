// SPDX-License-Identifier: Apache-2.0
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
	// visibleCustomFields is the projection of `custom_fields` every read answers with: only the
	// keys a live definition stands behind. The same expression the sqlc reads use (Work.sql,
	// FindWorkItem), so the query endpoint and the plain reads hide a deleted definition's values
	// identically. The stored document is untouched - the hiding is in the answer.
	// jsonb_build_object() with no arguments is the empty document. The literal spelling '{}'
	// would be shorter, and it is unavailable on purpose: this package's alphabet has no braces,
	// so that the fuzz gate can prove no fragment of a request ever reaches the text (FuzzCompile).
	visibleCustomFields = `(SELECT coalesce(jsonb_object_agg(kv.key, kv.value), jsonb_build_object()) ` +
		`FROM jsonb_each(wi.custom_fields) AS kv WHERE EXISTS (` +
		`SELECT 1 FROM custom_field_definition cfd WHERE cfd.deleted_at IS NULL ` +
		`AND cfd.id = jsonb_extract_path_text(wi.custom_field_refs, kv.key)::uuid ` +
		`AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL)))::jsonb ` +
		`AS custom_fields`

	itemColumns = `wi.id, wi.tenant_id, wi.collection_id, wi.type, wi.parent_id, wi.path, ` +
		`wi.depth, wi.title, wi.amount, wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, ` +
		`wi.completion_count, ` +
		`wi.bucket_id, wi.order_key, wi.assignee_id, ` +
		`wi.start_at, wi.due_at, wi.due_date_only, wi.due_time_zone, ` +
		`wi.cover_kind, wi.cover_color_token, wi.cover_media_id, ` + visibleCustomFields + `, ` +
		`wi.content_language, wi.calendar_uid, ` +
		`rt.pending_until AS retention_pending_until, rt.rule_id AS retention_rule_id, ` +
		`rt.action AS retention_action, rt.blocked_by AS retention_blocked_by, ` +
		`wi.archived_at, wi.deleted_at, ` +
		`wi.trash_batch_id, ` +
		`wi.created_by, wi.created_at, wi.updated_at, wi.version, wi.number, wi.entry_type, wi.status`

	// What a retention run marked is read from its side table (F-10, migration 0227); every
	// statement that selects itemColumns joins it.
	retentionJoin = ` LEFT JOIN work_item_retention rt ON rt.tenant_id = wi.tenant_id AND rt.item_id = wi.id`

	groupedColumns = `id, tenant_id, collection_id, type, parent_id, path, depth, title, amount, notes, ` +
		`is_completed, completed_at, completed_by, completion_count, bucket_id, order_key, assignee_id, ` +
		`start_at, due_at, due_date_only, due_time_zone, ` +
		`cover_kind, cover_color_token, cover_media_id, custom_fields, content_language, calendar_uid, ` +
		`retention_pending_until, retention_rule_id, retention_action, retention_blocked_by, ` +
		`archived_at, deleted_at, trash_batch_id, created_by, created_at, updated_at, version, ` +
		`number, entry_type, status`
)

// searchText is the expression the trigram index is built over, written here exactly as migration
// 0020 writes it.
//
// The planner matches an expression index by comparing expression trees, so the two spellings have
// to be identical: a coalesce written differently here would build the index and never use it, and
// nothing would fail - the search would simply scan the whole table. Which is why the migration
// names this constant and this constant names the migration.
const searchText = `(coalesce(wi.title, '') || ' ' || coalesce(wi.notes, ''))`

// The two prefixes a column expression can carry: the table alias inside the query, and nothing at
// all in the outer select of a grouped one, where the subquery is the only relation in scope.
const (
	itemPrefix  = "wi."
	groupPrefix = ""
)

// Rows compiles an ungrouped query: one page of entries in one order.
func Rows(search repository.ItemSearch, boundary Boundary, probe int) (Statement, error) {
	b := newBuilder(search)

	b.write(`SELECT `, itemColumns, ` FROM work_item wi`, retentionJoin, ` WHERE `)
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
	groupField := search.Spec.GroupBy.Field
	_, customGroup := view.CustomField(groupField.Name)
	group, prefixed := column(groupField, itemPrefix)
	if !prefixed && !customGroup {
		return Statement{}, errFieldNotCompilable(groupField)
	}

	b := newBuilder(search)
	b.write(`SELECT `, groupedColumns, ` FROM (SELECT `, itemColumns)
	// A custom field has no column the outer select could name, so the inner one carries its
	// value under an alias (sortAlias, groupAlias).
	for index, term := range search.Spec.Sort {
		if _, custom := view.CustomField(term.Field.Name); custom {
			b.write(`, `)
			b.customValue(term.Field)
			b.write(` AS `, sortAlias[index])
		}
		if Derived(term.Field) {
			name, _ := column(term.Field, itemPrefix)
			b.write(`, `, name, ` AS `, sortAlias[index])
		}
	}
	if customGroup {
		b.write(`, `)
		b.customValue(groupField)
		b.write(` AS `, groupAlias)
	}
	// The row numbers are drawn over the identifiers alone and only the chosen rows are read
	// whole: numbering every row of a large collection with all its columns sorted them all,
	// spilling to disk at tens of thousands of entries (observability-reliability.md §13.2).
	b.write(` FROM work_item wi`, retentionJoin, ` WHERE wi.id = ANY (ARRAY(SELECT r.id FROM (SELECT wi.id, row_number() OVER (PARTITION BY `)
	if customGroup {
		b.customValue(groupField)
	} else {
		b.write(group)
	}
	b.write(` ORDER BY `)
	b.ordering(search.Spec.Sort, itemPrefix)
	b.write(`, wi.id) AS rn FROM work_item wi WHERE `)
	b.predicates(search)
	b.write(`) r WHERE r.rn <= `)
	b.param(int64(probe))
	b.write(`))) g ORDER BY `)

	// The groups come in the grouping field's own order, and the entries that have no value at all
	// come last: a board draws that column at the end, and a client that reads the groups in order
	// gets the same layout every time.
	outer, _ := column(groupField, groupPrefix)
	if customGroup {
		outer = groupAlias
	}
	b.write(outer, ` ASC NULLS LAST, `)
	b.ordering(search.Spec.Sort, groupPrefix)
	b.write(`, id`)

	return b.statement()
}

// Count compiles the second pass an exact count costs: the same filter, without order, page or
// projection.
func Count(search repository.ItemSearch) (Statement, error) {
	b := newBuilder(search)

	if _, custom := view.CustomField(search.Spec.GroupBy.Field.Name); custom {
		b.write(`SELECT `)
		b.customGroupKey(search.Spec.GroupBy.Field)
		b.write(`, count(*)::bigint FROM work_item wi WHERE `)
		b.predicates(search)
		b.write(` GROUP BY 1`)
		return b.statement()
	}
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

// Search compiles the full text search: one page of entries in the order the database ranked them
// (ADR-0034).
//
// Three things are here that no other statement in this package has.
//
// The join to `container`, so that each row carries the hub it sits under. The search spans hubs,
// so the permission is asked about each row's place in the tree, and reading the collections back
// afterwards would be a query per collection in the page.
//
// The rank, which is the ordering. `ts_rank_cd` over the document, taking whichever of the two
// tsqueries answered better - the searcher's configuration or `simple` - because a row found by
// one of them and not the other would otherwise rank last however well it matched. The weights the
// document carries do the rest: a hit in a title outranks one buried in a note (migration 0019).
//
// And the keyset over that rank. Both keys descend, which is what lets the boundary be one row
// comparison rather than the three-clause form a mixed ordering needs; the identifier is a UUIDv7,
// so descending on it means equally-ranked entries come newest first.
func Search(
	search repository.TextSearch, meaning string, boundary SearchBoundary, probe int,
) (Statement, error) {
	b := newBuilder(repository.ItemSearch{Language: search.Request.Language})
	b.model = search.MeaningModel

	b.write(`SELECT `, itemColumns, `, c.parent_id, `)
	b.rank(search, meaning)
	b.write(` AS rank, `)
	b.byMeaning(search, meaning)
	b.write(` AS by_meaning FROM work_item wi JOIN container c ON c.id = wi.collection_id`, retentionJoin)
	b.meaningJoin(meaning)
	b.write(` WHERE `)
	b.searchPredicates(search, meaning)

	if !boundary.IsZero() {
		b.write(` AND (`)
		b.rank(search, meaning)
		b.write(`, wi.id) < (`)
		b.param(boundary.Rank)
		b.write(`::real, `)
		b.uuid(boundary.ID)
		b.write(`)`)
	}

	b.write(` ORDER BY rank DESC, wi.id DESC LIMIT `)
	b.param(int64(probe))
	return b.statement()
}

// SearchRows compiles a search that has **no words**: a filtered read of the whole workspace, or of
// one container, ordered rather than ranked (ADR-0064).
//
// The same statement `Rows` writes, with the join to `container` the search needs so that each row
// carries the hub the narrowing is asked about - and a constant rank, because the answer's shape is
// the same and nothing ranked it. Everything that decides *which* rows is the query's own:
// `node` writes the filter, `ordering` and `keyset` the order and the cursor. There is no tsquery,
// no trigram branch and no neighbourhood, because there are no words for any of them to read.
func SearchRows(search repository.TextSearch, boundary Boundary, probe int) (Statement, error) {
	b := newBuilder(repository.ItemSearch{Language: search.Request.Language})

	b.write(`SELECT `, itemColumns, `, c.parent_id, 0::real AS rank, false AS by_meaning`)
	b.write(` FROM work_item wi JOIN container c ON c.id = wi.collection_id`, retentionJoin, ` WHERE `)
	b.scope(search.Anchor)
	b.restriction(search.RestrictTo)
	b.lifecycle(view.Spec{
		IncludeArchived: search.Request.IncludeArchived,
		IncludeTrashed:  search.Request.IncludeTrashed,
	})
	if search.Request.Filter != nil {
		b.write(` AND (`)
		b.node(*search.Request.Filter)
		b.write(`)`)
	}
	if !boundary.IsZero() {
		b.write(` AND (`)
		b.keyset(search.Request.Sort, boundary)
		b.write(`)`)
	}

	b.write(` ORDER BY `)
	b.ordering(search.Request.Sort, itemPrefix)
	b.write(`, wi.id LIMIT `)
	b.param(int64(probe))
	return b.statement()
}

// SearchBoundary is a decoded search cursor: the rank of the last row of the previous page, and the
// identifier that breaks a tie between equal ranks.
type SearchBoundary struct {
	Rank float32
	ID   shared.ID
}

// IsZero reports the first page.
func (b SearchBoundary) IsZero() bool { return b.ID.IsZero() }

// rank writes the relevance expression.
//
// Lexically: the better of the two configurations' answers. With a query vector as well: that,
// plus the cosine similarity of the entry's own vector, weighted.
//
// **One number and not two orderings**, which is what keeps the keyset cursor working. A fusion of
// two ranked lists - reciprocal rank fusion and its relatives - needs both lists materialised, and
// a page boundary over a materialised list is an offset by another name; over one expression it
// stays the single row comparison every other list in this schema uses. The cost is that the
// weight is a constant rather than something the data tunes, which is the honest trade for a
// search that pages correctly.
//
// The weight is deliberately below 1: a lexical hit is evidence somebody's words are in the entry,
// and a semantic hit is evidence something is about the same subject. When they disagree the words
// win, which is what somebody typing an identifier expects.
func (b *builder) rank(search repository.TextSearch, meaning string) {
	b.write(`greatest(ts_rank_cd(wi.search_document, `)
	b.languageQuery(search.Request.Words)
	b.write(`), ts_rank_cd(wi.search_document, `)
	b.simpleQuery(search.Request.Words)
	b.write(`))`)
	b.attachedRank(search.Request.Words)

	if meaning == "" {
		return
	}
	// `<=>` is cosine distance in [0, 2]; 1 - distance is the similarity. `coalesce` covers the
	// entry that has no vector yet, which is an ordinary state rather than an absence: the pass
	// has not reached it, and it is found lexically in the meantime.
	b.write(` + `, semanticWeight, ` * coalesce(1 - (e.embedding <=> `)
	b.param(meaning)
	b.write(`::vector), 0)`)
}

// semanticWeight is how much a perfect semantic match is worth beside a lexical rank.
//
// `ts_rank_cd` answers small numbers - a good hit in a title is around 0.1 - so a similarity worth
// up to 0.05 puts a strong semantic match ahead of a weak lexical one and behind a strong one.
// That is the ordering the acceptance asks for in one sentence: an exact identifier is still found
// first, and an entry that shares no word with the query is found at all.
const semanticWeight = `0.05`

// meaningJoin brings the entry's own vector into reach, and only when there is a query vector to
// compare it with. The literal itself is rendered by the adapter and bound here, so this package
// keeps writing constants and binding values and nothing else (ADR-0026). A LEFT JOIN, because an entry the embedding pass has not reached yet is found
// lexically rather than not at all.
func (b *builder) meaningJoin(meaning string) {
	if meaning == "" {
		return
	}
	// And only the rows of the model the query vector came from: two models' vectors are not
	// comparable, and between a reconfiguration and the end of the re-embedding pass the table
	// holds both. Bound, never written - a model name is configuration somebody typed.
	b.write(` LEFT JOIN item_embedding e ON e.tenant_id = wi.tenant_id AND e.item_id = wi.id AND e.model = `)
	b.param(b.model)
}

// searchPredicates writes what the search matches: the scope, the lifecycle, the narrowing, and the
// match itself.
func (b *builder) searchPredicates(search repository.TextSearch, meaning string) {
	b.scope(search.Anchor)
	b.restriction(search.RestrictTo)
	b.lifecycle(view.Spec{
		IncludeArchived: search.Request.IncludeArchived,
		IncludeTrashed:  search.Request.IncludeTrashed,
	})

	b.write(` AND (`)
	b.wordMatch(search)
	b.neighbourhood(meaning)
	b.write(`)`)

	// And the filter, where one came with the words: the same tree the query compiles, written by
	// the same code (ADR-0064). It narrows what the words found rather than replacing it.
	if search.Request.Filter != nil {
		b.write(` AND (`)
		b.node(*search.Request.Filter)
		b.write(`)`)
	}
}

// byMeaning writes whether a row is a hit only because of its meaning: none of the word branches
// matched it, so the neighbourhood did. A constant where there is no query vector, since every row
// was then found by the words.
func (b *builder) byMeaning(search repository.TextSearch, meaning string) {
	if meaning == "" {
		b.write(`false`)
		return
	}
	b.write(`NOT (`)
	b.wordMatch(search)
	b.write(`)`)
}

// wordMatch writes the word branches of the match: the two configurations, the last word as a
// beginning, and the substring supplement - everything that finds a row by what was typed.
func (b *builder) wordMatch(search repository.TextSearch) {
	b.write(`wi.search_document @@ `)
	b.languageQuery(search.Request.Words)
	b.write(` OR wi.search_document @@ `)
	b.simpleQuery(search.Request.Words)

	// And the last word as a beginning, where the domain says there is one: the branch that makes
	// a search answer while it is still being typed (ADR-0066 decision 4, view.Search.PrefixTerm).
	if head, prefix := search.Request.PrefixTerm(); prefix != "" {
		b.write(` OR wi.search_document @@ `)
		b.prefixQuery(head, prefix)
	}

	b.attachedMatch(search.Request.Words)

	// The supplement for the scripts a tsquery cannot serve. A run of characters without word
	// boundaries is one token, so a query for part of it matches nothing - which is why this
	// branch is decided by the *words*, in the domain, rather than by the entries (i18n-l10n.md
	// §5, view.Search.WithoutWordBoundaries).
	//
	// ILIKE over the expression migration 0020 indexed, so the trigram index answers it. Below
	// three characters the index cannot narrow anything and the recheck does the work; the answer
	// is the same, which is what matters for a query somebody typed two characters into.
	if search.Request.WithoutWordBoundaries() {
		b.write(` OR `, searchText, ` ILIKE `)
		b.likePattern(search.Request.Words)
	}
}

// attachedMatch writes the branches that find an entry by what is attached to it rather than by its
// own document (F-9): a living comment on it, one of its text-valued custom fields, or the name of
// a file attached to it. Under `simple` and written as migrations 0225 and 0226 indexed them, since
// the planner matches an expression index by its tree. The entry is still the hit: what the actor may see is decided for entries,
// and a comment is readable exactly where its entry is.
func (b *builder) attachedMatch(words string) {
	b.write(` OR EXISTS (SELECT 1 FROM comment cm WHERE cm.tenant_id = wi.tenant_id AND cm.item_id = wi.id`)
	b.write(` AND cm.deleted_at IS NULL AND to_tsvector('simple'::regconfig, cm.body) @@ `)
	b.simpleQuery(words)
	b.write(`)`)
	b.write(` OR EXISTS (SELECT 1 FROM entry_field_value fv JOIN custom_field_definition fd`)
	b.write(` ON fd.tenant_id = fv.tenant_id AND fd.id = fv.definition_id`)
	b.write(` WHERE fv.tenant_id = wi.tenant_id AND fv.entry_id = wi.id AND fv.text IS NOT NULL`)
	b.write(` AND fd.kind IN ('TEXT', 'URL') AND fd.deleted_at IS NULL`)
	b.write(` AND to_tsvector('simple'::regconfig, fv.text) @@ `)
	b.simpleQuery(words)
	b.write(`)`)
	b.write(` OR EXISTS (SELECT 1 FROM item_attachment ia JOIN media_object mo`)
	b.write(` ON mo.tenant_id = ia.tenant_id AND mo.id = ia.media_id`)
	b.write(` WHERE ia.tenant_id = wi.tenant_id AND ia.item_id = wi.id`)
	b.write(` AND mo.status = 'READY' AND mo.file_name IS NOT NULL AND mo.deleted_at IS NULL`)
	b.write(` AND `, attachmentName, ` @@ `)
	b.simpleQuery(words)
	b.write(`)`)
}

// attachmentName is migration 0226's indexed expression, verbatim: a file's name with the dot, the
// underscore and the hyphen read as spaces, so "budget_2026.pdf" holds the word "budget".
const attachmentName = `to_tsvector('simple'::regconfig, translate(mo.file_name, '._-', '   '))`

// attachedRank adds what an attached hit is worth, so an entry found by its comment or a text field
// ranks by how well that matched rather than at zero beside every other such entry. The best
// comment and the best field count, each at attachedWeight, below the entry's own document. Kept
// `real` like ts_rank_cd, so the keyset still compares one real with the cursor's.
func (b *builder) attachedRank(words string) {
	b.write(` + `, attachedWeight, ` * coalesce((SELECT max(ts_rank_cd(to_tsvector('simple'::regconfig, cm.body), `)
	b.simpleQuery(words)
	b.write(`)) FROM comment cm WHERE cm.tenant_id = wi.tenant_id AND cm.item_id = wi.id AND cm.deleted_at IS NULL), 0)`)
	b.write(` + `, attachedWeight, ` * coalesce((SELECT max(ts_rank_cd(to_tsvector('simple'::regconfig, fv.text), `)
	b.simpleQuery(words)
	b.write(`)) FROM entry_field_value fv JOIN custom_field_definition fd`)
	b.write(` ON fd.tenant_id = fv.tenant_id AND fd.id = fv.definition_id`)
	b.write(` WHERE fv.tenant_id = wi.tenant_id AND fv.entry_id = wi.id AND fv.text IS NOT NULL`)
	b.write(` AND fd.kind IN ('TEXT', 'URL') AND fd.deleted_at IS NULL), 0)`)
}

// attachedWeight is what an attached hit is worth beside the entry's own: half, so the same words in
// a title still win, while a comment that matches well outranks one that merely mentions them.
const attachedWeight = `0.5::real`

// neighbourhood is the branch that makes the search find an entry sharing no word with the query.
//
// Without it the semantic half would only ever *reorder* what the words already found, which is not
// what somebody describing a task they cannot name is asking for. With it, an entry is a hit when
// it is among the nearest by meaning **and** near enough - and both halves of that are necessary:
//
//   - Nearest-k alone would return everything in a small workspace, because in a workspace of nine
//     entries all nine are among the nearest. A search would then answer the whole collection for
//     any query, which is worse than finding nothing.
//   - A distance threshold alone cannot use the HNSW index: pgvector answers `ORDER BY … LIMIT`
//     from the index and a comparison in a WHERE by scanning. So the threshold is applied *outside*
//     a subquery that is ordered and limited, which is the shape pgvector's own documentation gives
//     for a filtered nearest-neighbour search.
//
// The candidates are the nearest in the **workspace**, and the scope narrows them afterwards rather
// than inside the subquery. That is a real limitation and worth naming: an anchored search into one
// collection can miss an entry in it, if the workspace holds a whole page of entries nearer to the
// query elsewhere. The alternative is to join `work_item` inside the subquery, which makes the
// scope exact and takes the HNSW index away - the planner then computes a distance for every entry
// the scope allows. The default search is unanchored, so the case this gives up is the narrower
// one; and it gives up finding *more*, never correctness, because everything it returns is still
// narrowed by the scope and by row level security outside.
//
// The set is the same on every page of one walk, which is what keeps the keyset cursor stepping
// through one stable ordering.
//
// The subquery names no tenant, like every other statement in this package: row level security
// bounds it (ADR-0010).
func (b *builder) neighbourhood(meaning string) {
	if meaning == "" {
		return
	}
	b.write(` OR wi.id IN (SELECT n.item_id FROM (SELECT item_id, embedding <=> `)
	b.param(meaning)
	b.write(`::vector AS distance FROM item_embedding WHERE model = `)
	b.param(b.model)
	b.write(` ORDER BY distance LIMIT `, semanticCandidates)
	b.write(`) n WHERE n.distance < `, semanticFloor, `)`)
}

// semanticCandidates bounds how many entries the semantic branch may propose.
//
// Larger than any page, because the branch proposes candidates rather than results: they are ranked
// against the lexical hits afterwards and narrowed by the scope and by what the actor may see, so a
// pool the size of one page would leave the last of those three with nothing to work with. Small
// enough that the pool is a bounded read whatever the workspace holds.
//
// It is a ceiling rather than a promise. An HNSW scan is bounded by `hnsw.ef_search` as well - 40 by
// default - so a large workspace hands back the nearest few dozen rather than two hundred. That is
// the ordinary character of an approximate index and not something to tune blind: raising it costs
// every search time in exchange for candidates the floor below would discard anyway, and what an
// installation with that problem actually has is a corpus to measure against.
const semanticCandidates = `200`

// semanticFloor is how far apart two texts may be and still be about the same thing.
//
// A cosine distance, so 0 is the same direction and 1 is unrelated; 0.35 is a similarity of 0.65.
// Deliberately strict: the lexical half is complete on its own, so a query that finds nothing by
// meaning has lost nothing, while an entry returned because it is vaguely adjacent is a wrong
// answer somebody has to read. It is a constant rather than a setting for the reason the weight is:
// a knob here is a knob whose effect nobody can describe without the corpus in front of them.
const semanticFloor = `0.35`

// languageQuery parses the words under the searcher's configuration, and simpleQuery under the one
// an entry that stated no language was indexed with. Both bind their values; neither writes any
// part of the request into the statement (rule 9, T-06).
func (b *builder) languageQuery(words string) {
	b.write(`websearch_to_tsquery(hubtask_text_config(`)
	b.param(b.language)
	b.write(`::text), `)
	b.words(words)
	b.write(`)`)
}

func (b *builder) simpleQuery(words string) {
	b.write(`websearch_to_tsquery('simple', `)
	b.words(words)
	b.write(`)`)
}

// prefixQuery asks for the finished words *and* an entry holding a lexeme that begins with the
// unfinished one: `Momente Chall` is `'momente' & 'chall':*`.
//
// **Under `simple`, always**, and for two reasons that are the same reason. A configuration stems,
// so `Ann` would become a stem that is not a prefix of the entry's; and a configuration drops stop
// words, so `A` and `An` would become nothing at all - a search that answers nothing. `simple` does
// neither, and every document carries a `simple` copy of itself (ADR-0066), so this reaches an
// entry whatever language it was written in.
//
// The `:*` is written here and the word is **bound**: no byte of the request becomes query text,
// and the domain has already refused anything that is not a letter, a digit or a mark - so
// `to_tsquery`, which unlike `websearch_to_tsquery` has operators of its own, cannot be handed
// one (rule 9, T-06).
func (b *builder) prefixQuery(head, prefix string) {
	if head != "" {
		b.write(`(websearch_to_tsquery('simple', `)
		b.words(head)
		b.write(`) && `)
	}
	b.write(`to_tsquery('simple', `)
	b.words(prefix)
	b.write(` || ':*')`)
	if head != "" {
		b.write(`)`)
	}
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

// fullText writes the MATCHES predicate: the words, read under the searcher's configuration and
// under `simple`, against the document the trigger maintains (ADR-0034).
//
// Two branches rather than one, and neither of them is the entry's own configuration. A document
// is built under the language its entry names, so the exact question - "parse these words the way
// this row was indexed" - would need the row before the query, and an index scan needs the query
// to be one value for the whole scan. Two constants are two index scans and a bitmap OR, which the
// planner does; a per-row tsquery is a sequential scan, which it also does, once per search, over
// every entry in the tenant.
//
// The `simple` branch is what makes the pair honest. An entry that states no language is indexed
// word by word, and the searcher's stemmer would ask for a lexeme such a document never holds.
//
// The configuration is a bound parameter resolved by hubtask_text_config, so no byte of the tag
// reaches the statement's text (rule 9, T-06) - and a tag this installation has no configuration
// for resolves to `simple` rather than failing the query.
func (b *builder) fullText(value view.Value) {
	b.write(`wi.search_document @@ websearch_to_tsquery(hubtask_text_config(`)
	b.param(b.language)
	b.write(`::text), `)
	b.text(value)
	b.write(`) OR wi.search_document @@ websearch_to_tsquery('simple', `)
	b.text(value)
	b.write(`)`)
}

// restriction narrows the answer to the entries the caller may see, when that is fewer than the
// anchor holds.
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

// identifiers is a set of identifiers as a bound text array; empty, never nil, so the array is
// typed and matches nothing.
func identifiers(ids []shared.ID) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	return values
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

	case repository.AnchorTenant:
		// Everything the transaction can see. Row level security has already bounded that to one
		// tenant, and the predicate says so rather than relying on it silently: it is true whether
		// or not a policy is in force, and it is the column a planner can use.
		b.write(`wi.tenant_id = current_tenant_id()`)

	case repository.AnchorCollections:
		// Bound arrays, as the restriction is, so the text does not grow with the workspace. An
		// empty set reads nothing rather than everything; the shared entries are a second array.
		if len(anchor.CollectionIDs) == 0 && len(anchor.ItemIDs) == 0 {
			b.write(`false`)
			return
		}
		b.write(`(wi.collection_id = ANY(`)
		b.param(identifiers(anchor.CollectionIDs))
		b.write(`::uuid[]) OR wi.id = ANY(`)
		b.param(identifiers(anchor.ItemIDs))
		b.write(`::uuid[]))`)

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
		b.fullText(node.Values[0])
		return
	case view.FieldBlocked:
		b.waitingLink(node, true)
		return
	case view.FieldBlocking:
		b.waitingLink(node, false)
		return
	case view.FieldLinkedTo:
		b.linkedTo(node)
		return
	}

	if key, isCustom := view.CustomField(node.Field.Name); isCustom {
		if node.Op == view.OpLt || node.Op == view.OpLte || node.Op == view.OpGt ||
			node.Op == view.OpGte || node.Op == view.OpBetween {
			b.customRange(node, key)
			return
		}
		b.customField(node, key)
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

// waitingLink writes whether an open entry stands across a link whose type waits: for `blocked`,
// one that holds this entry up; for `blocking`, one this entry holds up. The waiting side is the
// target of a `blocks` link, the source of a `depends_on` link, and a workspace's own type's as
// its `waits` says (work.LinkWaits). The value stays a parameter compared with the EXISTS, so the
// text is the same for true and false (ADR-0026); the direction is a constant of this package.
func (b *builder) waitingLink(node view.Node, held bool) {
	b.write(`(EXISTS (SELECT 1 FROM entry_link bl`)
	b.write(` LEFT JOIN link_type lt ON lt.tenant_id = bl.tenant_id AND lt.key = bl.own_type`)
	b.write(` CROSS JOIN LATERAL (SELECT CASE bl.link_type WHEN 'blocks' THEN 'TARGET'`)
	b.write(` WHEN 'depends_on' THEN 'SOURCE' ELSE lt.waits END AS waits) lw`)
	b.write(` JOIN work_item bw ON bw.tenant_id = bl.tenant_id`)
	if held {
		// The other entry is the one waited for: the source when the target waits.
		b.write(` AND bw.id = CASE lw.waits WHEN 'TARGET' THEN bl.source_id ELSE bl.target_id END`)
		b.write(` WHERE bl.tenant_id = wi.tenant_id`)
		b.write(` AND ((lw.waits = 'TARGET' AND bl.target_id = wi.id) OR (lw.waits = 'SOURCE' AND bl.source_id = wi.id))`)
	} else {
		// The other entry is the one waiting: the target when the target waits.
		b.write(` AND bw.id = CASE lw.waits WHEN 'TARGET' THEN bl.target_id ELSE bl.source_id END`)
		b.write(` WHERE bl.tenant_id = wi.tenant_id`)
		b.write(` AND ((lw.waits = 'TARGET' AND bl.source_id = wi.id) OR (lw.waits = 'SOURCE' AND bl.target_id = wi.id))`)
	}
	b.write(` AND NOT bw.is_completed AND bw.deleted_at IS NULL))`)
	if node.Op == view.OpNeq {
		b.write(` IS DISTINCT FROM `)
	} else {
		b.write(` = `)
	}
	b.value(node.Field, node.Values[0])
}

// linkedTo writes whether a link of any type joins the entry to one of the given entries, from
// either end. EQ and IN bind the same array, so one value is a list of one; the identifiers are
// parameters, never text (ADR-0026).
func (b *builder) linkedTo(node view.Node) {
	b.write(`EXISTS (SELECT 1 FROM entry_link ln WHERE ln.tenant_id = wi.tenant_id`)
	b.write(` AND ((ln.source_id = wi.id AND ln.target_id = ANY(`)
	b.array(node.Field, node.Values)
	b.write(`)) OR (ln.target_id = wi.id AND ln.source_id = ANY(`)
	b.array(node.Field, node.Values)
	b.write(`))))`)
}

// customField writes a comparison against one key of the entry's custom field document.
//
// This is the one place ADR-0026's closed vocabulary cannot cover by enumeration, and the answer is
// the one the ADR already gives: the *key* is a value, so it is bound as a parameter exactly like
// the thing being compared. Every character of the SQL below is a constant of this package; the key
// never touches the text (T-06).
//
// Equality goes through the containment operator rather than through `->>`, because containment is
// what the GIN index on `custom_fields` answers (`jsonb_path_ops`, migration 0001). The other
// operators fall back to `->>`, which is a text comparison the index cannot serve - bounded, as
// every query here is, by the mandatory scope and the statement timeout.
func (b *builder) customField(node view.Node, key string) {
	// Every branch is conjoined with the live-definition check, so a filter answers over exactly
	// the document a read answers with: a value whose definition was deleted matches nothing, and
	// IS_NULL treats it as not set. Without this, a filter would be the one read that can still
	// see what every other read hides.
	switch node.Op {
	case view.OpIsNull:
		// The key is absent from the visible document: never stored, cleared, or stored under a
		// definition that is gone.
		b.write(`(NOT jsonb_exists(wi.custom_fields, `)
		b.param(key)
		b.write(`) OR NOT `)
		b.liveDefinition(key)
		b.write(`)`)
	case view.OpEq:
		b.write(`(`)
		b.contains(key, node.Values[0])
		b.write(` AND `)
		b.liveDefinition(key)
		b.write(`)`)
	case view.OpNeq:
		// An entry that visibly holds nothing under the key counts as "not that one", for the
		// reason bucket_id NEQ does: three-valued logic would drop the row instead of answering.
		b.write(`NOT (`)
		b.contains(key, node.Values[0])
		b.write(` AND `)
		b.liveDefinition(key)
		b.write(`)`)
	case view.OpIn:
		b.write(`(`)
		b.customText(key)
		b.write(` = ANY(`)
		b.param(textsOf(node.Values, customValueText))
		b.write(`::text[]) AND `)
		b.liveDefinition(key)
		b.write(`)`)
	case view.OpNotIn:
		b.write(`(NOT `)
		b.liveDefinition(key)
		b.write(` OR `)
		b.customText(key)
		b.write(` IS NULL OR `)
		b.customText(key)
		b.write(` <> ALL(`)
		b.param(textsOf(node.Values, customValueText))
		b.write(`::text[]))`)
	case view.OpContains:
		// Two questions behind one spelling, and both are answered: a MULTI_SELECT that holds the
		// element, or a text that contains it. The document decides which applies per row, which
		// is the only honest reading when the kind is data.
		b.write(`((`)
		b.containsElement(key, node.Values[0])
		b.write(` OR position(lower(`)
		b.text(node.Values[0])
		b.write(`) IN lower(coalesce(`)
		b.customText(key)
		b.write(`, ''))) > 0) AND `)
		b.liveDefinition(key)
		b.write(`)`)
	case view.OpContainsAny, view.OpContainsAll:
		// A set holds any or every element, one indexable containment each; the shape has
		// already refused a field that holds one value (view.Spec.Shaped).
		separator := ` OR `
		if node.Op == view.OpContainsAll {
			separator = ` AND `
		}
		b.write(`((`)
		for index, value := range node.Values {
			if index > 0 {
				b.write(separator)
			}
			b.containsElement(key, value)
		}
		b.write(`) AND `)
		b.liveDefinition(key)
		b.write(`)`)
	case view.OpStartsWith:
		b.write(`(starts_with(lower(`)
		b.customText(key)
		b.write(`), lower(`)
		b.text(node.Values[0])
		b.write(`)) AND `)
		b.liveDefinition(key)
		b.write(`)`)
	default:
		b.fail(errOperatorNotCompilable(node.Op))
	}
}

// liveDefinition is the check that the definition this entry's value was written under still
// lives, in the entry's own collection or workspace-wide. By identity rather than by key, for the
// reason the read projection is: a definition recreated under the same key must not make the old
// value filterable again. A primary-key lookup per row, beside whatever the main predicate
// costs.
func (b *builder) liveDefinition(key string) {
	b.write(`EXISTS (SELECT 1 FROM custom_field_definition cfd WHERE cfd.deleted_at IS NULL ` +
		`AND cfd.id = jsonb_extract_path_text(wi.custom_field_refs, `)
	b.param(key)
	b.write(`)::uuid AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL))`)
}

// contains writes the indexable equality: does the document hold this key with this value.
func (b *builder) contains(key string, value view.Value) {
	b.write(`wi.custom_fields @> jsonb_build_object(`)
	b.param(key)
	b.write(`::text, `)
	b.scalar(value)
	b.write(`)`)
}

// containsElement is the same question about one element of a list value.
func (b *builder) containsElement(key string, value view.Value) {
	b.write(`wi.custom_fields @> jsonb_build_object(`)
	b.param(key)
	b.write(`::text, jsonb_build_array(`)
	b.scalar(value)
	b.write(`))`)
}

// scalar writes one comparison value as the JSON scalar it is.
//
// The cast is chosen by the value's own kind, which the grammar took from the document: a number
// stays a number and a boolean a boolean, so containment matches what a NUMBER or a BOOL field
// actually holds rather than its text spelling.
func (b *builder) scalar(value view.Value) {
	switch value.Kind {
	case view.KindBool:
		b.write(`to_jsonb(`)
		b.param(value.Bool)
		b.write(`::boolean)`)
	case view.KindNumber:
		b.write(`to_jsonb(`)
		b.param(value.Text)
		b.write(`::numeric)`)
	default:
		b.write(`to_jsonb(`)
		b.text(value)
		b.write(`)`)
	}
}

// customText is the value as text, for the comparisons containment cannot express.
func (b *builder) customText(key string) {
	// The function form rather than the `->>` operator, and not for taste: this package may not
	// write a hyphen at all, because two of them begin a SQL comment and the fuzz gate proves that
	// none is ever emitted (ADR-0026, FuzzCompile). jsonb_extract_path_text is the same operation
	// under a name made of letters. jsonb_exists above is the same trade for `?`.
	b.write(`jsonb_extract_path_text(wi.custom_fields, `)
	b.param(key)
	b.write(`)`)
}

// customValueText is the value as `->>` renders the stored one: the text spelling of the JSON
// scalar. A boolean has to be spelled the way jsonb spells it, which is `true` and not `TRUE`.
func customValueText(value view.Value) string {
	if value.Kind == view.KindBool {
		if value.Bool {
			return "true"
		}
		return "false"
	}
	return value.Text
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
		if !b.sortExpression(term, index, prefix) {
			return
		}
		b.write(collationOf(term.Field))
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
	if _, ok := column(term.Field, itemPrefix); !ok {
		if _, custom := view.CustomField(term.Field.Name); !custom {
			b.fail(errFieldNotCompilable(term.Field))
			return
		}
	}
	name := func() { b.sortExpression(term, index, itemPrefix) }

	b.write(`(`)
	b.after(name, term, boundary.Keys[index])
	b.write(` OR (`)
	name()
	b.write(` IS NOT DISTINCT FROM `)
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
func (b *builder) after(name func(), term view.SortTerm, key string) {
	comparison := ` > `
	if term.Descending {
		comparison = ` < `
	}
	if !term.Field.Nullable {
		name()
		b.write(collationOf(term.Field), comparison)
		b.boundaryValue(term, key)
		b.write(collationOf(term.Field))
		return
	}

	b.write(`(CASE `)
	if term.NullsFirst {
		b.write(`WHEN `)
		name()
		b.write(` IS NULL THEN false WHEN `)
		b.boundaryValue(term, key)
		b.write(` IS NULL THEN true `)
	} else {
		b.write(`WHEN `)
		b.boundaryValue(term, key)
		b.write(` IS NULL THEN false WHEN `)
		name()
		b.write(` IS NULL THEN true `)
	}
	b.write(`ELSE `)
	name()
	b.write(collationOf(term.Field), comparison)
	b.boundaryValue(term, key)
	b.write(collationOf(term.Field), ` END)`)
}

// sortExpression writes what one sort term orders by: its column, a custom field's typed value,
// or - in a grouped query's outer select - the alias the inner one carries that value under.
func (b *builder) sortExpression(term view.SortTerm, index int, prefix string) bool {
	if Derived(term.Field) && prefix == groupPrefix {
		b.write(sortAlias[index])
		return true
	}
	if _, custom := view.CustomField(term.Field.Name); custom {
		if prefix == groupPrefix {
			b.write(sortAlias[index])
		} else {
			b.customValue(term.Field)
		}
		return true
	}
	name, ok := column(term.Field, prefix)
	if !ok {
		b.fail(errFieldNotCompilable(term.Field))
		return false
	}
	b.write(name)
	return true
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
