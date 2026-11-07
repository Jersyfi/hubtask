// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package reference_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	entry = shared.MustParseID("0192f000-0000-7000-8000-0000000002a1")
	view  = shared.MustParseID("0192f000-0000-7000-8000-0000000002a2")
)

func TestNewRefusesAReferenceOfNoShape(t *testing.T) {
	cases := []struct {
		name   string
		kind   reference.Kind
		id     shared.ID
		anchor string
		code   string
	}{
		{"an unknown kind", "SPREADSHEET", entry, "", "references.kind_unknown"},
		{"no identifier", reference.KindEntry, "", "", "references.id_required"},
		{"an anchor with a space", reference.KindEntry, entry, "a b", "references.anchor_malformed"},
		{"an anchor too long", reference.KindEntry, entry, strings.Repeat("a", 65), "references.anchor_malformed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reference.New(tc.kind, tc.id, tc.anchor)
			var typed *shared.Error
			if !errors.As(err, &typed) || typed.DetailCode != tc.code {
				t.Errorf("refused with %v, want %s", err, tc.code)
			}
		})
	}
	if ref, err := reference.New(reference.KindPage, entry, "intro"); err != nil || ref.Address() != "hubtask:page/"+entry.String()+"#intro" {
		t.Errorf("a later module's kind is a valid reference: %+v, %v", ref, err)
	}
}

// P-19, UC-PLN-01 check 7: a mention, a card and a live view are one reference at three strengths,
// read from the text that holds them.
func TestExtractReadsTheThreeStrengths(t *testing.T) {
	text := "See [the bug](hubtask:entry/" + entry.String() + ") and the board " +
		"![](hubtask:view/" + view.String() + "#done). Again: ![card](hubtask:entry/" + entry.String() + "). " +
		"Not a reference: hubtask:entry/" + entry.String() + " and [x](hubtask:spreadsheet/" + entry.String() + ")."
	found := reference.Extract(text)
	if len(found) != 2 {
		t.Fatalf("found %+v", found)
	}
	if found[0].Reference.Kind != reference.KindEntry || found[0].Reference.ID != entry || found[0].Strength != reference.Card {
		t.Errorf("the entry, drawn once as a mention and once as a card, is a card: %+v", found[0])
	}
	if found[1].Reference.Kind != reference.KindView || found[1].Reference.Anchor != "done" || found[1].Strength != reference.Embed {
		t.Errorf("a view in place is a live view: %+v", found[1])
	}
	if reference.Extract("no references here") != nil {
		t.Error("a text without an address holds references")
	}
}

func TestExtractStopsAtItsBound(t *testing.T) {
	var b strings.Builder
	for i := 0; i < reference.MaxPerText+10; i++ {
		id := shared.MustParseID("0192f000-0000-7000-8000-" + strings.Repeat("0", 8) + pad(i))
		b.WriteString("[x](" + reference.Reference{Kind: reference.KindEntry, ID: id}.Address() + ") ")
	}
	if got := len(reference.Extract(b.String())); got != reference.MaxPerText {
		t.Errorf("extracted %d references, want %d", got, reference.MaxPerText)
	}
}

func pad(i int) string {
	const digits = "0123456789"
	out := []byte{'0', '0', '0', '0'}
	for p := 3; p >= 0; p-- {
		out[p] = digits[i%10]
		i /= 10
	}
	return string(out)
}
