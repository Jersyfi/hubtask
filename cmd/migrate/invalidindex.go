// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// createIndexStatement finds a migration's concurrent build of one index, up to its semicolon. An
// index statement holds no semicolon of its own, so the match ends where the statement does.
var createIndexStatement = regexp.MustCompile(
	`(?is)CREATE\s+(UNIQUE\s+)?INDEX\s+CONCURRENTLY\s+IF\s+NOT\s+EXISTS\s+("?)([a-z0-9_]+)("?)\s+ON\b[^;]*;`)

// invalidIndexes lists the indexes of the public schema PostgreSQL marks invalid: what an
// interrupted CREATE INDEX CONCURRENTLY leaves behind.
func invalidIndexes(ctx context.Context, pool *sql.DB) ([]string, error) {
	rows, err := pool.QueryContext(ctx, `
		SELECT c.relname FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT i.indisvalid AND n.nspname = 'public'
		ORDER BY c.relname`)
	if err != nil {
		return nil, fmt.Errorf("reading the invalid indexes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("reading the invalid indexes: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// indexBuilds maps each index a migration builds concurrently to that statement, from the Up
// half of every NO TRANSACTION file. A later file building the same name wins.
func indexBuilds(files fs.FS) (map[string]string, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("reading the migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	builds := make(map[string]string)
	for _, name := range names {
		content, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		text := string(content)
		if !strings.Contains(text, "-- +goose NO TRANSACTION") {
			continue
		}
		if down := strings.Index(text, "-- +goose Down"); down >= 0 {
			text = text[:down]
		}
		for _, match := range createIndexStatement.FindAllStringSubmatch(text, -1) {
			builds[strings.ToLower(match[3])] = match[0]
		}
	}
	return builds, nil
}

// repairInvalidIndexes drops every invalid index a migration builds and runs that build again.
//
// An interrupted concurrent build leaves an invalid index under the file's name; the file's retry
// then skips it on IF NOT EXISTS and the ledger records the file as applied, so the planner never
// uses the index while every write still maintains it. Run before the pending files, this heals a
// file that is still pending as well as one an earlier release recorded over the broken index.
// An invalid index no migration builds (a REINDEX of an operator's) is logged, not touched.
func repairInvalidIndexes(ctx context.Context, pool *sql.DB, files fs.FS) error {
	invalid, err := invalidIndexes(ctx, pool)
	if err != nil || len(invalid) == 0 {
		return err
	}
	builds, err := indexBuilds(files)
	if err != nil {
		return err
	}
	for _, name := range invalid {
		build, ours := builds[name]
		if !ours {
			slog.Warn("an invalid index no migration builds was left as it is", slog.String("index", name))
			continue
		}
		// DDL takes no parameter; the name is the catalogue's, quoted, and one a migration builds.
		drop := "DROP INDEX CONCURRENTLY IF EXISTS " + pgx.Identifier{"public", name}.Sanitize() //nolint:gosec // G202, see above
		if _, err := pool.ExecContext(ctx, drop); err != nil {
			return fmt.Errorf("dropping the invalid index %s: %w", name, err)
		}
		if _, err := pool.ExecContext(ctx, build); err != nil {
			return fmt.Errorf("building the index %s again: %w", name, err)
		}
		slog.Warn("an invalid index was built again", slog.String("index", name))
	}
	return nil
}

// refuseInvalidIndexes fails the run while an index a migration builds is still invalid, naming
// it: a run that reported success over one would leave a board on sequential scans for good.
func refuseInvalidIndexes(ctx context.Context, pool *sql.DB, files fs.FS) error {
	invalid, err := invalidIndexes(ctx, pool)
	if err != nil || len(invalid) == 0 {
		return err
	}
	builds, err := indexBuilds(files)
	if err != nil {
		return err
	}
	var ours []string
	for _, name := range invalid {
		if _, ok := builds[name]; ok {
			ours = append(ours, name)
		}
	}
	if len(ours) > 0 {
		return fmt.Errorf("indexes left invalid after the run: %s", strings.Join(ours, ", "))
	}
	return nil
}
