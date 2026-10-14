// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package db carries the migrations into the binary.
//
// The alternative would be to ship the .sql files beside the image and hope the two versions match.
// They would not: a pod running one release with the migrations of another is exactly the schema
// drift that /readyz is supposed to catch, and it is cheaper to make it impossible than to detect
// it (ADR-0003, deployment.md §5).
//
// Only the migrations are embedded. db/queries is sqlc's input and exists at build time, not at
// run time, and db/schema.sql is a reference for readers.
package db

import (
	"embed"
	"sort"
	"strconv"
	"strings"
)

// Migrations is db/migrations, in the order goose applies them.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// Versions answers the version of every embedded migration, ascending - the set a database must
// hold all of before this binary may serve against it (deployment.md §5). The set rather than the
// highest, because migrations are applied out of order.
func Versions() []int64 {
	entries, err := Migrations.ReadDir("migrations")
	if err != nil {
		return nil
	}
	versions := make([]int64, 0, len(entries))
	for _, entry := range entries {
		number, _, found := strings.Cut(entry.Name(), "_")
		if !found {
			continue
		}
		if version, err := strconv.ParseInt(number, 10, 64); err == nil {
			versions = append(versions, version)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions
}
