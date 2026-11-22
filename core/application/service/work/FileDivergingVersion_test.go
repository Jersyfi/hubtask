// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

func fileVersion(h *commentHarness) FileDivergingVersion {
	return FileDivergingVersion{Comments: AddComment{Writer: h.writer}}
}

// UC-SYN-02 check 10: the version that loses is filed as a comment on the entry - the writer's
// own text when they keep the other, the other text when they write theirs - for the title as for
// the notes, in the writer's name, with the same four records as any comment.
func TestADivergingVersionIsFiledAsTheEntrysSystemComment(t *testing.T) {
	cases := []struct {
		field, side, code string
	}{
		{domain.FieldNotes, VersionMine, domain.SetAsideVersionCode},
		{domain.FieldNotes, VersionReplaced, domain.ReplacedVersionCode},
		{domain.FieldTitle, VersionMine, domain.SetAsideVersionCode},
		{domain.FieldTitle, VersionReplaced, domain.ReplacedVersionCode},
	}
	for _, c := range cases {
		t.Run(c.field+"/"+c.side, func(t *testing.T) {
			h := newCommentHarness(t)
			h.withItem(domain.ItemTask)

			comment, err := fileVersion(h).Execute(t.Context(), actor(), FileDivergingVersionCommand{
				ItemID: assignedItem, Field: c.field, Side: c.side, Body: "Bring the long ladder",
			})
			if err != nil {
				t.Fatalf("filing failed: %v", err)
			}
			if comment.Kind != domain.CommentBySystem || comment.SystemCode != c.code ||
				comment.Body != "Bring the long ladder" || comment.AuthorID != accountID {
				t.Errorf("filed %+v", comment)
			}
			if comment.SystemParams["field"] != c.field || comment.SystemParams["at"] != "2026-08-17T09:00:00Z" {
				t.Errorf("the heading's parameters are %v", comment.SystemParams)
			}
			if _, named := comment.SystemParams["device"]; named {
				t.Error("a version kept online names a device")
			}
			if len(h.events.appended) != 1 || h.events.appended[0].Type != event.CommentCreated ||
				len(h.changes.recorded) != 1 || len(h.audit.entries) != 1 ||
				h.audit.entries[0].Action != CommentCreatedAction ||
				len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemCommented {
				t.Errorf("records: events %d, changes %d, audit %d, history %d",
					len(h.events.appended), len(h.changes.recorded), len(h.audit.entries), len(h.history.entries))
			}
		})
	}
}

// Rule 10: the kept text is user content; the audit entry carries a fingerprint of it.
func TestAFiledVersionsAuditEntryCarriesNoText(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	if _, err := fileVersion(h).Execute(t.Context(), actor(), FileDivergingVersionCommand{
		ItemID: assignedItem, Field: domain.FieldTitle, Side: VersionReplaced, Body: "A secret plan",
	}); err != nil {
		t.Fatal(err)
	}
	recorded, ok := h.audit.entries[0].Changes["body"].(map[string]any)
	if !ok {
		t.Fatalf("the entry has no body change: %+v", h.audit.entries[0].Changes)
	}
	for _, value := range recorded {
		if value == "A secret plan" {
			t.Fatal("the audit trail carries the kept text in the open")
		}
	}
}

// A type without a discussion keeps no version, and nobody who may not comment files one: the
// same refusals as any comment, and nothing written.
func TestAVersionIsRefusedWhereNoCommentMayBeWritten(t *testing.T) {
	cmd := FileDivergingVersionCommand{ItemID: assignedItem, Field: domain.FieldNotes, Side: VersionMine, Body: "Mine"}

	t.Run("a type without comments", func(t *testing.T) {
		h := newCommentHarness(t)
		h.withItem(domain.ItemActivity)
		_, err := fileVersion(h).Execute(t.Context(), actor(), cmd)
		assertValidation(t, err, "items.capability_not_supported")
		if len(h.comments.inserted)+len(h.events.appended) != 0 {
			t.Error("the refusal still wrote something")
		}
	})
	t.Run("no right to comment", func(t *testing.T) {
		h := newCommentHarness(t)
		h.withItem(domain.ItemTask)
		h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")
		_, err := fileVersion(h).Execute(t.Context(), actor(), cmd)
		if shared.AsError(err).Category != shared.CategoryForbidden {
			t.Fatalf("refused as %v", err)
		}
		if len(h.authorizer.requests) != 1 || h.authorizer.requests[0].TokenScope != "comments:write" {
			t.Fatalf("the permission question was %+v", h.authorizer.requests)
		}
		if len(h.comments.inserted) != 0 {
			t.Error("the refusal still wrote a row")
		}
	})
}

// The door, through the registry: the contract's names in, the comment out; a field or a side
// outside the declared ones never reaches the write.
func TestTheVersionDoorTakesTheContractsInputThroughTheRegistry(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	registry, err := usecase.NewRegistry(nil, fileVersion(h).Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), FileDivergingVersionName, actor(), usecase.Input{
		"item_id": assignedItem.String(), "field": "title", "side": "MINE", "body": "Old title",
	})
	if err != nil {
		t.Fatalf("filing through the registry: %v", err)
	}
	if out["kind"] != "SYSTEM" || out["system_code"] != domain.SetAsideVersionCode || out["body"] != "Old title" {
		t.Errorf("answered %v", out)
	}

	for _, in := range []usecase.Input{
		{"item_id": assignedItem.String(), "field": "due_date", "side": "MINE", "body": "x"},
		{"item_id": assignedItem.String(), "field": "notes", "side": "THEIRS", "body": "x"},
		{"item_id": assignedItem.String(), "field": "notes", "side": "MINE"},
	} {
		if _, err := registry.Invoke(t.Context(), FileDivergingVersionName, actor(), in); shared.AsError(err).Category != shared.CategoryValidation {
			t.Errorf("%v: refused as %v", in, err)
		}
	}
	if len(h.comments.inserted) != 1 {
		t.Errorf("rows written: %d", len(h.comments.inserted))
	}
}

// UC-SYN-02 check 10 for notes longer than a comment may be: the notes carry no bound of their
// own, so a version of them is kept whole - through the use case, through the registry, and
// through the push's own filing - while a comment somebody writes keeps its 20000.
func TestNotesLongerThanACommentAreStillKept(t *testing.T) {
	long := strings.Repeat("n", domain.MaxCommentBodyLength+1)

	t.Run("the use case", func(t *testing.T) {
		h := newCommentHarness(t)
		h.withItem(domain.ItemTask)
		comment, err := fileVersion(h).Execute(t.Context(), actor(), FileDivergingVersionCommand{
			ItemID: assignedItem, Field: domain.FieldNotes, Side: VersionMine, Body: long,
		})
		if err != nil || comment.Body != long {
			t.Fatalf("filing %d characters: %v", len(long), err)
		}
	})
	t.Run("the registry", func(t *testing.T) {
		h := newCommentHarness(t)
		item := h.withItem(domain.ItemTask)
		item.Notes = long
		h.items.stored[assignedItem] = item
		registry, err := usecase.NewRegistry(nil, fileVersion(h).Descriptor())
		if err != nil {
			t.Fatal(err)
		}
		out, err := registry.Invoke(t.Context(), FileDivergingVersionName, actor(), usecase.Input{
			"item_id": assignedItem.String(), "field": "notes", "side": "REPLACED", "body": long,
		})
		if err != nil || out["body"] != long {
			t.Fatalf("filing %d characters through the registry: %v", len(long), err)
		}
	})
	t.Run("the push's filing", func(t *testing.T) {
		h := newCommentHarness(t)
		h.withItem(domain.ItemTask)
		comment, err := AddComment{Writer: h.writer}.FileDisplaced(t.Context(), actor(), assignedItem, long,
			map[string]string{"field": domain.FieldNotes, "device": "d1", "at": "2026-08-17T09:00:00Z"})
		if err != nil || comment.Body != long || comment.SystemCode != domain.DisplacedVersionCode {
			t.Fatalf("the push filed %d characters: %v", len(long), err)
		}
	})
	t.Run("a person's comment", func(t *testing.T) {
		h := newCommentHarness(t)
		h.withItem(domain.ItemTask)
		_, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), addCmd(long))
		assertValidation(t, err, "comments.body_too_long")
	})
}
