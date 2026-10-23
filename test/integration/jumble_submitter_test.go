// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	jumblerepo "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A jumble entry records the account that submitted it, and that is what keeps a kept-out person's
// own capture out of what a colleague's ask sends (data-protection.md §4.1, UC-PRV-05 check 7).

// UC-PRV-05 check 7 against the database: Kim keeps her content out and captures into the jumble
// through the ordinary submission, with no sender. Ada's ask about Kim's capture has nothing to
// send; her ask about her own capture sends it.
func TestAKeptOutPersonsCaptureIsNotInAColleaguesAsk(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	w.record(ctx, t, false, time.Now().UTC().Add(-time.Minute))

	writer := jumble.Writer{
		Entries: jumbleRepo(), Authorizer: realAuthoriser(ctx, t),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:      clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}
	capture := func(account shared.ID, body string) jumbledomain.Entry {
		entry, err := jumble.SubmitJumbleEntry{Writer: writer}.Execute(ctx, w.as(account), jumble.SubmitCommand{
			Channel: jumbledomain.ChannelQuickCapture, RawBody: body,
		})
		if err != nil {
			t.Fatalf("capturing: %v", err)
		}
		return entry
	}
	kims := capture(w.kim, "KIMCAPTURE the venue's door code")
	adas := capture(w.ada, "ADACAPTURE order the drinks")

	readers := jumbleReaders(ctx, t)
	sources := suggestion.CatalogueSources{Catalogue: readers, Kept: keptOutExclusion(), Jumble: jumbleRepo()}
	material := func(entry shared.ID) suggestion.Material {
		var material suggestion.Material
		if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
			var err error
			material, err = sources.Material(ctx, w.as(w.ada), suggestion.Question{
				TargetType: suggestiondomain.TargetJumbleEntry, TargetID: entry, PromptID: "suggest-fields",
			})
			return err
		}); err != nil {
			t.Fatalf("reading the material: %v", err)
		}
		return material
	}

	if stored := findJumbleEntry(ctx, t, w.tenant, kims.ID); stored.CreatedBy != w.kim {
		t.Fatalf("Kim's capture names %q as its submitter", stored.CreatedBy)
	}
	if sent := material(kims.ID).Content; strings.TrimSpace(sent) != "" {
		t.Errorf("Ada's ask about Kim's capture sends %q", sent)
	}
	if sent := material(adas.ID).Content; !strings.Contains(sent, "ADACAPTURE") {
		t.Errorf("Ada's ask about her own capture sends %q", sent)
	}
}

// T-04 for the submitter: an entry is read with its submitter in its own workspace and not at all
// from another one, by the single read or the listing.
func TestAJumbleEntrysSubmitterIsReadOnlyInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	mine, theirs := seedJumbleWorld(ctx, t), seedJumbleWorld(ctx, t)
	entry := seedJumbleEntry(ctx, t, theirs.tenant, func(in *jumbledomain.NewEntryInput) {
		in.CreatedBy, in.Sender = theirs.owner, ""
	})

	if found := findJumbleEntry(ctx, t, theirs.tenant, entry.ID); found.CreatedBy != theirs.owner {
		t.Errorf("the workspace's own entry names %q as its submitter", found.CreatedBy)
	}
	if err := read(ctx, t, mine.tenant, func(ctx context.Context) error {
		_, err := jumbleRepo().Find(ctx, entry.ID)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another workspace's entry answered %v", err)
		}
		page, err := jumbleRepo().List(ctx, jumblerepo.Query{})
		for _, listed := range page.Entries {
			if listed.ID == entry.ID || listed.CreatedBy == theirs.owner {
				t.Error("another workspace's entry was listed with its submitter")
			}
		}
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
}

// An erasure reaches what the person captured as it reaches what they mailed in: a full deletion
// removes it, an anonymisation keeps the text and clears the account - and nothing of anybody
// else's.
func TestAnErasureReachesThePersonsCaptures(t *testing.T) {
	ctx := context.Background()
	w := seedJumbleWorld(ctx, t)
	other := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Olga Other')`,
		other.String(), w.tenant.String(), "olga-"+other.String()+"@example.org"); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	captured := func(by shared.ID) shared.ID {
		return seedJumbleEntry(ctx, t, w.tenant, func(in *jumbledomain.NewEntryInput) {
			in.CreatedBy, in.Sender = by, "widget"
		}).ID
	}
	released, othersOne := captured(w.owner), captured(other)

	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		count, err := privacyRepo().CountIntake(ctx, w.owner)
		if err != nil || count != 1 {
			t.Errorf("counted %d of the owner's intake (%v), want 1", count, err)
		}
		changed, err := privacyRepo().ReleaseIntake(ctx, w.owner)
		if err != nil || changed != 1 {
			t.Errorf("released %d (%v), want 1", changed, err)
		}
		return err
	}); err != nil {
		t.Fatalf("releasing: %v", err)
	}
	if kept := findJumbleEntry(ctx, t, w.tenant, released); !kept.CreatedBy.IsZero() || kept.Sender != "widget" ||
		kept.RawBody == "" {
		t.Errorf("the released capture reads %+v", kept)
	}

	deleted := captured(w.owner)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		removed, err := privacyRepo().DiscardIntake(ctx, w.owner)
		if err != nil || removed != 1 {
			t.Errorf("removed %d (%v), want 1", removed, err)
		}
		return err
	}); err != nil {
		t.Fatalf("discarding: %v", err)
	}
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		if _, err := jumbleRepo().Find(ctx, deleted); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the deleted capture answered %v", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if untouched := findJumbleEntry(ctx, t, w.tenant, othersOne); untouched.CreatedBy != other {
		t.Errorf("somebody else's capture names %q", untouched.CreatedBy)
	}
}

func findJumbleEntry(ctx context.Context, t *testing.T, tenant, id shared.ID) jumbledomain.Entry {
	t.Helper()
	var found jumbledomain.Entry
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = jumbleRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("finding the entry: %v", err)
	}
	return found
}
