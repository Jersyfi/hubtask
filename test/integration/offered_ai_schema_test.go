// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Migration 0129 against the real boundary, as the application role: an offered model is read by
// every workspace and written by none, and the previous binary's statements - unscoped, relying on
// the one policy they knew - send nothing while one exists.

var (
	offeredSchemaTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb01")
	offeredSchemaModel  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb11")
)

func seedOfferedSchema(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'offered-schema', 'Offered schema')
		 ON CONFLICT (id) DO NOTHING`, offeredSchemaTenant.String()); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO ai_provider (id, tenant_id, display_name, processor, kind, base_url,
		   completion_model, jurisdiction, processing_allowed, created_at)
		 VALUES ($1, NULL, 'Schema test model', 'A processor', 'OPENAI_COMPATIBLE',
		   'https://offered.example/v1', 'offered-model', 'EEA', false, now())
		 ON CONFLICT (id) DO NOTHING`, offeredSchemaModel.String()); err != nil {
		t.Fatalf("seeding the offered model: %v", err)
	}
	// Offered rows are visible to every workspace, so one left behind would surprise every other
	// test of this package that reads ai_provider.
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `DELETE FROM ai_provider WHERE id = $1`, offeredSchemaModel.String())
		_, _ = admin.Exec(done, `DELETE FROM ai_provider WHERE tenant_id = $1`, offeredSchemaTenant.String())
	})
}

func TestAWorkspaceReadsAnOfferedModelAndCannotWriteIt(t *testing.T) {
	ctx := context.Background()
	seedOfferedSchema(ctx, t)
	app := appPool(ctx, t)

	inRawTenant(ctx, t, app, offeredSchemaTenant.String(), false, func(tx pgx.Tx) {
		var name string
		if err := tx.QueryRow(ctx, `SELECT display_name FROM ai_provider WHERE id = $1`,
			offeredSchemaModel.String()).Scan(&name); err != nil {
			t.Fatalf("the workspace cannot read the offered model: %v", err)
		}
		for _, write := range []string{
			`UPDATE ai_provider SET display_name = 'mine' WHERE id = $1`,
			`DELETE FROM ai_provider WHERE id = $1`,
		} {
			tag, err := tx.Exec(ctx, write, offeredSchemaModel.String())
			if err == nil && tag.RowsAffected() != 0 {
				t.Errorf("a workspace changed the installation's row: %s", write)
			}
		}
	})

	inRawTenant(ctx, t, app, offeredSchemaTenant.String(), false, func(tx pgx.Tx) {
		_, err := tx.Exec(ctx,
			`INSERT INTO ai_provider (tenant_id, display_name, processor, kind, base_url,
			   completion_model, jurisdiction, created_at)
			 VALUES (NULL, 'Forged', 'Somebody', 'OPENAI_COMPATIBLE', 'https://x.example/v1', 'm', 'EEA', now())`)
		if err == nil {
			t.Error("a workspace offered a model to every workspace")
		}
	})
}

func TestAnOfferedRowCanNeverCarryConsent(t *testing.T) {
	ctx := context.Background()
	seedOfferedSchema(ctx, t)

	_, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE ai_provider SET processing_allowed = true WHERE id = $1`, offeredSchemaModel.String())
	if err == nil {
		t.Error("an offered row was given consent")
	}
}

// The rolling window: the previous binary's reads have no WHERE. With an offered row present they
// may answer it instead of the workspace's own - and then they must find nothing to send to.
func TestThePreviousBinarysStatementsSendNothingWhileAModelIsOffered(t *testing.T) {
	ctx := context.Background()
	seedOfferedSchema(ctx, t)
	app := appPool(ctx, t)

	// The previous binary's own write, with its `ON CONFLICT (tenant_id)` - it must still infer.
	inRawTenant(ctx, t, app, offeredSchemaTenant.String(), true, func(tx pgx.Tx) {
		for range 2 {
			if _, err := tx.Exec(ctx,
				`INSERT INTO ai_provider (tenant_id, kind, base_url, completion_model, embedding_model,
				   jurisdiction, processing_allowed, created_at)
				 VALUES (current_tenant_id(), 'OPENAI_COMPATIBLE', 'https://own.example/v1', 'own', '',
				   'EEA', true, now())
				 ON CONFLICT (tenant_id) DO UPDATE SET version = ai_provider.version + 1`); err != nil {
				t.Fatalf("the previous binary's upsert: %v", err)
			}
		}
	})

	inRawTenant(ctx, t, app, offeredSchemaTenant.String(), false, func(tx pgx.Tx) {
		rows, err := tx.Query(ctx,
			`SELECT tenant_id IS NULL, kind, base_url, processing_allowed FROM ai_provider`)
		if err != nil {
			t.Fatalf("the previous binary's read: %v", err)
		}
		defer rows.Close()
		var own, offered int
		for rows.Next() {
			var installation, consented bool
			var kind, baseURL string
			if err := rows.Scan(&installation, &kind, &baseURL, &consented); err != nil {
				t.Fatalf("scanning: %v", err)
			}
			if installation {
				offered++
				if consented {
					t.Error("an old pod would send to an offered model nobody chose")
				}
				continue
			}
			own++
		}
		if own != 1 || offered < 1 {
			t.Errorf("the unscoped read saw %d own and %d offered rows", own, offered)
		}
	})
}
