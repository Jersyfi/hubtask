// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The group list behind the members screen (F3-01), against the real boundary. Gate SG-3: List
// has its cross-tenant negative below; Find and Members have theirs in identity_test.go.

func groupsListed(ctx context.Context, t *testing.T, tenant shared.ID, page repository.Page) repository.GroupPage {
	t.Helper()
	var listed repository.GroupPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		listed, err = postgres.NewGroupRepository(pageCursors()).List(ctx, page)
		return err
	}); err != nil {
		t.Fatalf("listing the groups: %v", err)
	}
	return listed
}

func groupNamed(ctx context.Context, t *testing.T, tenant shared.ID, name string) identity.Group {
	t.Helper()
	group, err := identity.NewGroup(identity.NewGroupInput{ID: freshID(t), TenantID: tenant, Name: name})
	if err != nil {
		t.Fatalf("building the group: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewGroupRepository(pageCursors()).Insert(ctx, group)
	}); err != nil {
		t.Fatalf("writing the group: %v", err)
	}
	return group
}

// The walk is by name regardless of case, every group exactly once, and the last page says so.
// The database is shared with every other test, so the walk finds its own rows among the rest
// rather than expecting to be alone.
func TestTheGroupsArePagedByName(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	prefix := strings.ToLower(freshName(t))
	own := map[shared.ID]string{}
	for _, suffix := range []string{"c", "A", "b"} {
		group := groupNamed(ctx, t, tenantA, prefix+"-"+suffix)
		own[group.ID] = group.Name
	}

	var walked []identity.Group
	cursor := ""
	for pages := 0; ; pages++ {
		page := groupsListed(ctx, t, tenantA, repository.Page{Cursor: cursor, Size: 2})
		walked = append(walked, page.Groups...)
		if !page.Info.HasMore {
			break
		}
		if pages > 200 {
			t.Fatal("the walk does not end")
		}
		cursor = page.Info.NextCursor
	}

	var mine []string
	seen := map[shared.ID]int{}
	for _, group := range walked {
		if _, ours := own[group.ID]; ours {
			mine = append(mine, group.Name)
			seen[group.ID]++
		}
	}
	if len(mine) != 3 {
		t.Fatalf("the walk found %d of the three groups: %v", len(mine), mine)
	}
	for id, times := range seen {
		if times != 1 {
			t.Errorf("group %s was listed %d times", id, times)
		}
	}
	for i := 1; i < len(mine); i++ {
		if strings.ToLower(mine[i-1]) > strings.ToLower(mine[i]) {
			t.Errorf("%q came before %q; the order is by name regardless of case", mine[i-1], mine[i])
		}
	}
}

// The cross-tenant negative for List (gate SG-3).
func TestTheGroupsOfAnotherTenantAreNotListed(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupNamed(ctx, t, tenantA, freshName(t))

	cursor := ""
	for pages := 0; ; pages++ {
		page := groupsListed(ctx, t, tenantB, repository.Page{Cursor: cursor, Size: 200})
		for _, listed := range page.Groups {
			if listed.ID == group.ID {
				t.Fatalf("tenant B listed tenant A's group %s", group.ID)
			}
		}
		if !page.Info.HasMore || pages > 50 {
			break
		}
		cursor = page.Info.NextCursor
	}
}
