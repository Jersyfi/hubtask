// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestATrashFilterNamesAKindTheTrashLists(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want TrashKind
		ok   bool
	}{
		{raw: "", want: "", ok: true},
		{raw: "CONTAINER", want: TrashContainerKind, ok: true},
		{raw: "ITEM", want: TrashItemKind, ok: true},
		{raw: "PAGE", want: TrashPageKind, ok: true},
		{raw: "WHITEBOARD", want: TrashWhiteboardKind, ok: true},
		{raw: "page"},
		{raw: "LABEL"},
	} {
		got, err := ParseTrashKind(c.raw)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("ParseTrashKind(%q) = %q, %v; want %q", c.raw, got, err, c.want)
		}
		if !c.ok && !errors.Is(err, shared.ErrValidation) {
			t.Errorf("ParseTrashKind(%q) answered %v, want a validation error", c.raw, err)
		}
	}
}
