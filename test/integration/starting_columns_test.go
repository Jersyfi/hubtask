// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

// catalogueRenderer is the production renderer over the catalogues the build embeds.
func catalogueRenderer(t *testing.T) i18n.Renderer {
	t.Helper()
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	return renderer
}

type column struct {
	name     string
	limit    *int
	isDone   bool
	colorTag string
}

func columnsOf(ctx context.Context, t *testing.T, collection string) []column {
	t.Helper()
	rows, err := adminPool(ctx, t).Query(ctx, `
		SELECT name, wip_limit, is_done_bucket, coalesce(color_token, '') FROM bucket
		WHERE collection_id = $1 AND deleted_at IS NULL ORDER BY order_key`, collection)
	if err != nil {
		t.Fatalf("reading the columns: %v", err)
	}
	defer rows.Close()
	var columns []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.name, &c.limit, &c.isDone, &c.colorTag); err != nil {
			t.Fatalf("reading a column: %v", err)
		}
		columns = append(columns, c)
	}
	return columns
}

// UC-WRK-31 check 1 against PostgreSQL: a collection on *Flow* - here following its hub - starts
// with its states as columns in the workspace's language, the one in progress limited, the last
// completing; one on *Project* has the columns without a limit; one on *List* has none. Each
// column reaches a synchronising client.
func TestACollectionOnAPresetWithABoardStartsWithItsColumns(t *testing.T) {
	ctx := context.Background()
	registry := structureCatalogue(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE tenant SET default_locale = 'de' WHERE id = $1`,
		tenant.String()); err != nil {
		t.Fatalf("setting the workspace's language: %v", err)
	}
	create := func(in usecase.Input) string {
		t.Helper()
		out, err := registry.Invoke(ctx, "CreateContainer", owner, in)
		if err != nil {
			t.Fatalf("creating %v: %v", in, err)
		}
		return out.String("id")
	}

	hub := create(usecase.Input{"type": "HUB", "name": freshName(t), "preset": "FLOW"})
	flow := create(usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t)})
	project := create(usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "PROJECT"})
	list := create(usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "LIST"})

	columns := columnsOf(ctx, t, flow)
	if len(columns) != 3 {
		t.Fatalf("a Flow collection starts with %d columns, want three: %+v", len(columns), columns)
	}
	if columns[0].name != "Zu erledigen" || columns[1].name != "In Arbeit" || columns[2].name != "Erledigt" {
		t.Errorf("the columns are not named in the workspace's language: %+v", columns)
	}
	if columns[1].limit == nil || *columns[1].limit != 3 || columns[0].limit != nil || columns[2].limit != nil {
		t.Errorf("only the work in progress is limited, to three: %+v", columns)
	}
	if columns[0].isDone || columns[1].isDone || !columns[2].isDone {
		t.Errorf("only the last column completes: %+v", columns)
	}

	columns = columnsOf(ctx, t, project)
	if len(columns) != 3 || columns[1].limit != nil {
		t.Errorf("a Project collection starts with %+v, want three columns without a limit", columns)
	}
	if columns := columnsOf(ctx, t, list); len(columns) != 0 {
		t.Errorf("a List collection starts with columns: %+v", columns)
	}
	if hubColumns := columnsOf(ctx, t, hub); len(hubColumns) != 0 {
		t.Errorf("a hub has columns: %+v", hubColumns)
	}

	if rows := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity = 'bucket' AND container_id = $1`,
		flow); rows != 3 {
		t.Errorf("%d change entries for the Flow collection's columns, want three", rows)
	}
}
