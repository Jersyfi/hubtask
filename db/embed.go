// SPDX-License-Identifier: BUSL-1.1
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

import "embed"

// Migrations is db/migrations, in the order goose applies them.
//
//go:embed migrations/*.sql
var Migrations embed.FS
