// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package integration runs against a real PostgreSQL. Not a mock: the subject of these tests is
// row level security, and a mock of RLS would only test the mock (engineering-guidelines.md §1).
package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The container and the two pools come from test/dbtest, which is where they moved when a second
// suite needed them (B-10). The local names stay, so that the hundred call sites in this package
// read as they did - and so that "which pool is this test using" is still answered by one word.

// appPool connects as the application role, the way the server does. No BYPASSRLS, not an owner:
// everything under test goes through it.
func appPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.AppPool(ctx, t)
}

// adminPool connects as the superuser, for fixtures and for catalogue queries - the writes across
// tenants that the application role must not be able to make.
func adminPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.AdminPool(ctx, t)
}
