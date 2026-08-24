// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func TestACoverIsExactlyOneOfTwoThings(t *testing.T) {
	color, err := work.NewCover(work.CoverColor, "blue", "")
	if err != nil || color.ColorToken != "blue" || !color.MediaID.IsZero() {
		t.Fatalf("a colour cover came out as %+v (%v)", color, err)
	}

	image, err := work.NewCover(work.CoverImage, "", "0192f000-0000-7000-8000-0000000000a1")
	if err != nil || image.MediaID.IsZero() || image.ColorToken != "" {
		t.Fatalf("an image cover came out as %+v (%v)", image, err)
	}

	cases := []struct {
		name  string
		kind  work.CoverKind
		token string
		media shared.ID
		code  string
	}{
		{name: "a colour with an image is a contradiction", kind: work.CoverColor,
			token: "blue", media: "0192f000-0000-7000-8000-0000000000a1",
			code: "items.cover_contradictory"},
		{name: "a colour without a token is nothing", kind: work.CoverColor,
			code: "items.cover_contradictory"},
		{name: "an image with a token is a contradiction", kind: work.CoverImage,
			token: "blue", media: "0192f000-0000-7000-8000-0000000000a1",
			code: "items.cover_contradictory"},
		{name: "an image without a media identifier is nothing", kind: work.CoverImage,
			code: "items.cover_contradictory"},
		{name: "an unknown kind is refused by name", kind: work.CoverKind("GRADIENT"),
			code: "items.cover_kind_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := work.NewCover(tc.kind, tc.token, tc.media)
			if got := shared.AsError(err).DetailCode; got != tc.code {
				t.Fatalf("answered %q, want %s", got, tc.code)
			}
		})
	}
}

func TestCoveringIsIdempotentBothWays(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	item := work.WorkItem{ID: "i1", UpdatedAt: at}
	cover, err := work.NewCover(work.CoverColor, "blue", "")
	if err != nil {
		t.Fatal(err)
	}

	covered := item.Covered(cover, at.Add(time.Minute))
	if covered.Cover == nil || covered.Cover.ColorToken != "blue" {
		t.Fatalf("covered %+v", covered.Cover)
	}
	if again := covered.Covered(cover, at.Add(time.Hour)); !again.UpdatedAt.Equal(covered.UpdatedAt) {
		t.Error("the same cover again moved the item")
	}

	bare := covered.Uncovered(at.Add(2 * time.Minute))
	if bare.Cover != nil {
		t.Fatal("the cover survived its clearing")
	}
	if again := bare.Uncovered(at.Add(time.Hour)); !again.UpdatedAt.Equal(bare.UpdatedAt) {
		t.Error("clearing nothing moved the item")
	}
}

func TestTheCoverAndAttachmentGatesAskTheCapabilityFirst(t *testing.T) {
	item := work.WorkItem{ID: "i1"}
	bare := work.CapabilityProfile{Type: work.ItemActivity}
	able := work.CapabilityProfile{Capabilities: []work.Capability{
		work.CapabilityCover, work.CapabilityAttachments,
	}}

	if err := item.EnsureCoverable(able); err != nil {
		t.Errorf("a coverable type was refused: %v", err)
	}
	if err := item.EnsureCoverable(bare); shared.AsError(err).DetailCode != "items.capability_not_supported" {
		t.Errorf("an activity earned a cover: %v", err)
	}
	if err := item.EnsureAttachable(able); err != nil {
		t.Errorf("an attachable type was refused: %v", err)
	}
	if err := item.EnsureAttachable(bare); shared.AsError(err).DetailCode != "items.capability_not_supported" {
		t.Errorf("an activity earned attachments: %v", err)
	}
}
