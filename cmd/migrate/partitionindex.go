// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
)

// missingPartitionIndex is one partition that a parent's ON ONLY index does not reach yet.
type missingPartitionIndex struct {
	parentIndex string
	partition   string
	definition  string // pg_get_indexdef of the parent index
}

// missingPartitionIndexes lists, for every invalid index of a partitioned table in the public
// schema, the leaf partitions that hold no index attached to it.
func missingPartitionIndexes(ctx context.Context, pool *sql.DB) ([]missingPartitionIndex, error) {
	rows, err := pool.QueryContext(ctx, `
		SELECT pi.relname, part.relname, pg_get_indexdef(pi.oid)
		FROM pg_index i
		JOIN pg_class pi ON pi.oid = i.indexrelid AND pi.relkind = 'I'
		JOIN pg_namespace n ON n.oid = pi.relnamespace AND n.nspname = 'public'
		JOIN pg_inherits pt ON pt.inhparent = i.indrelid
		JOIN pg_class part ON part.oid = pt.inhrelid AND part.relkind = 'r'
		WHERE NOT i.indisvalid
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_inherits ci JOIN pg_index c ON c.indexrelid = ci.inhrelid
		    WHERE ci.inhparent = pi.oid AND c.indrelid = part.oid)
		ORDER BY pi.relname, part.relname`)
	if err != nil {
		return nil, fmt.Errorf("reading the partitions an index does not reach: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var missing []missingPartitionIndex
	for rows.Next() {
		var m missingPartitionIndex
		if err := rows.Scan(&m.parentIndex, &m.partition, &m.definition); err != nil {
			return nil, fmt.Errorf("reading the partitions an index does not reach: %w", err)
		}
		missing = append(missing, m)
	}
	return missing, rows.Err()
}

// partitionIndexName names a partition's share of a parent index, within PostgreSQL's 63 bytes.
func partitionIndexName(partition, parentIndex string) string {
	name := partition + "_" + parentIndex
	if len(name) <= 63 {
		return name
	}
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(name))
	return fmt.Sprintf("%s_%08x", name[:54], sum.Sum32())
}

// partitionIndexBuild turns the parent's definition into a concurrent build on one partition.
func partitionIndexBuild(m missingPartitionIndex) (string, error) {
	using := strings.Index(m.definition, " USING ")
	if using < 0 {
		return "", fmt.Errorf("the definition of %s names no access method: %s", m.parentIndex, m.definition)
	}
	unique := ""
	if strings.HasPrefix(m.definition, "CREATE UNIQUE INDEX") {
		unique = "UNIQUE "
	}
	child := pgx.Identifier{partitionIndexName(m.partition, m.parentIndex)}.Sanitize()
	table := pgx.Identifier{"public", m.partition}.Sanitize()
	return "CREATE " + unique + "INDEX CONCURRENTLY IF NOT EXISTS " + child + " ON " + table + m.definition[using:], nil
}

// attachPartitionIndexes builds an ON ONLY index on every partition it does not reach, without
// holding writes, and attaches it, so that the parent's index becomes valid.
//
// A migration cannot do this itself: CREATE INDEX CONCURRENTLY runs outside a transaction and
// names its table, and which monthly partitions a database holds is known only at run time. The
// application's partition job cannot either - it has none of the owner's rights. A partition
// created after the parent's index takes it at once. A share left invalid by an interrupted build
// is dropped and built again, so a later run heals what an earlier one left.
func attachPartitionIndexes(ctx context.Context, pool *sql.DB) error {
	missing, err := missingPartitionIndexes(ctx, pool)
	if err != nil {
		return err
	}
	for _, m := range missing {
		build, err := partitionIndexBuild(m)
		if err != nil {
			return err
		}
		name := partitionIndexName(m.partition, m.parentIndex)
		qualified := pgx.Identifier{"public", name}.Sanitize()
		var invalid bool
		if err := pool.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			               JOIN pg_namespace n ON n.oid = c.relnamespace
			               WHERE n.nspname = 'public' AND c.relname = $1 AND NOT i.indisvalid)`,
			name).Scan(&invalid); err != nil {
			return fmt.Errorf("reading the index %s: %w", name, err)
		}
		// DDL takes no parameter; every name is the catalogue's, quoted.
		if invalid {
			if _, err := pool.ExecContext(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+qualified); err != nil { //nolint:gosec // G202, see above
				return fmt.Errorf("dropping the invalid index %s: %w", name, err)
			}
		}
		if _, err := pool.ExecContext(ctx, build); err != nil {
			return fmt.Errorf("building %s on %s: %w", m.parentIndex, m.partition, err)
		}
		attach := "ALTER INDEX " + pgx.Identifier{"public", m.parentIndex}.Sanitize() + " ATTACH PARTITION " + qualified //nolint:gosec // G202, see above
		if _, err := pool.ExecContext(ctx, attach); err != nil {
			return fmt.Errorf("attaching %s to %s: %w", name, m.parentIndex, err)
		}
		slog.Info("a partition's index was built and attached",
			slog.String("index", m.parentIndex), slog.String("partition", m.partition))
	}
	return nil
}
