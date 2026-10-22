// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A proposal about a jumble entry is read, accepted and dismissed through a read of its target,
// and so is the material it is made from. Against a real registry and a real database, because a
// fake listing that answers every entry agrees with the defect this pins: the reads used to search
// the first page of the jumble and nothing behind it.

// jumbleWorld is one workspace with an owner.
type jumbleWorld struct {
	tenant, owner shared.ID
}

func seedJumbleWorld(ctx context.Context, t *testing.T) jumbleWorld {
	t.Helper()
	w := jumbleWorld{tenant: freshID(t), owner: freshID(t)}
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Jumble world')`,
			[]any{w.tenant.String(), slugOf(w.tenant)}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Owen Owner')`,
			[]any{w.owner.String(), w.tenant.String(), "owen-" + w.owner.String() + "@example.org"}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.owner.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	return w
}

func (w jumbleWorld) as(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: w.tenant, AccountID: account, AccountName: "Someone",
		Locale: "en", TimeZone: "UTC",
		Scopes: []string{"items:read", "items:write", "containers:read", "suggestions:read", "suggestions:write"},
	}
}

// jumbleReaders is the jumble's reads and the suggestion listing, wired as main.go wires them.
func jumbleReaders(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	authoriser := realAuthoriser(ctx, t)
	writer := jumble.Writer{
		Entries: jumbleRepo(), Authorizer: authoriser, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}
	catalogue := &catalogueLater{}
	registry, err := usecase.NewRegistry(nil,
		jumble.ListJumbleEntries{Writer: writer}.Descriptor(),
		jumble.GetJumbleEntry{Writer: writer}.Descriptor(),
		suggestion.ListSuggestions{Cases: suggestion.Cases{
			Suggestions: postgres.NewSuggestionRepository(pageCursors()),
			Targets:     suggestion.EntryTargets{Catalogue: catalogue},
			Authorizer:  authoriser, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the readers: %v", err)
	}
	catalogue.registry = registry
	return registry
}

// C1 (UX-21): behind fifty newer arrivals an entry's proposal was not found, and the producer had
// no material for it.
func TestAProposalAboutAnEntryBehindFiftyNewerArrivalsIsFound(t *testing.T) {
	ctx := context.Background()
	w := seedJumbleWorld(ctx, t)
	old := seedJumbleEntry(ctx, t, w.tenant, func(in *jumbledomain.NewEntryInput) {
		in.RawSubject = "OLDEST arrival"
	})
	for i := 0; i < 51; i++ {
		seedJumbleEntry(ctx, t, w.tenant, nil)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	proposal, err := suggestiondomain.New(suggestiondomain.NewInput{
		ID: freshID(t), TenantID: w.tenant,
		TargetType: suggestiondomain.TargetJumbleEntry, TargetID: old.ID, Kind: suggestiondomain.KindFields,
		Payload:     map[string]any{"title": "Call the customer back"},
		Provenance:  suggestiondomain.Provenance{Model: "stub", PromptID: "suggest-fields", PromptVersion: "3", ProducedAt: now},
		InputDigest: suggestiondomain.Digest(old.RawSubject, old.RawBody),
		Now:         now,
	})
	if err != nil {
		t.Fatalf("building the proposal: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return postgres.NewSuggestionRepository(pageCursors()).Record(ctx, proposal)
	}); err != nil {
		t.Fatalf("recording the proposal: %v", err)
	}

	readers := jumbleReaders(ctx, t)
	out, err := readers.Invoke(ctx, suggestion.ListSuggestionsName, w.as(w.owner), usecase.Input{
		"target_type": string(suggestiondomain.TargetJumbleEntry), "target_id": old.ID.String(),
	})
	if err != nil {
		t.Fatalf("listing the proposals about the oldest entry: %v", err)
	}
	if items, _ := out["items"].([]usecase.Output); len(items) != 1 {
		t.Errorf("the listing answered %d proposals, want the one recorded: %v", len(items), out)
	}

	var material suggestion.Material
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		var err error
		material, err = suggestion.CatalogueSources{Catalogue: readers}.Material(
			ctx, w.as(w.owner), suggestion.Question{TargetType: suggestiondomain.TargetJumbleEntry, TargetID: old.ID, PromptID: "suggest-fields"})
		return err
	}); err != nil {
		t.Fatalf("reading the material of the oldest entry: %v", err)
	}
	if !strings.Contains(material.Content, "OLDEST arrival") {
		t.Errorf("the material does not carry the oldest entry's subject: %q", material.Content)
	}
}

// The single-entry read stays inside its workspace: another workspace's entry is not found, as a
// missing one is (T-04).
func TestAJumbleEntryIsReadOnlyInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	mine, theirs := seedJumbleWorld(ctx, t), seedJumbleWorld(ctx, t)
	entry := seedJumbleEntry(ctx, t, theirs.tenant, nil)
	readers := jumbleReaders(ctx, t)

	out, err := readers.Invoke(ctx, jumble.GetJumbleEntryName, theirs.as(theirs.owner), usecase.Input{"entry_id": entry.ID.String()})
	if err != nil || out.String("id") != entry.ID.String() {
		t.Fatalf("the workspace's own entry: %v %v", out, err)
	}
	_, err = readers.Invoke(ctx, jumble.GetJumbleEntryName, mine.as(mine.owner), usecase.Input{"entry_id": entry.ID.String()})
	if got := shared.AsError(err); got == nil || got.DetailCode != "jumble.entry_not_found" {
		t.Errorf("another workspace's entry answered %v, want jumble.entry_not_found", err)
	}
}
