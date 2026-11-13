// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A cover is a gradient, a gallery picture or an image, shown from a place between its top and
// its foot (E-18, UC-DOC-16); anything else is refused with what was wrong.
func TestACoverIsCheckedAsAskedFor(t *testing.T) {
	cases := []struct {
		name     string
		source   CoverSource
		key      string
		position int
		code     string
	}{
		{"a gradient", CoverGradient, "indigo-teal", 50, ""},
		{"a gallery picture", CoverGallery, "hills", 0, ""},
		{"an image", CoverImage, "01936f2a-7c1e-7000-8000-00000000d001", 100, ""},
		{"a colour is no key", CoverGradient, "#ff0000", 50, "pages.cover_key_malformed"},
		{"a path is no key", CoverGallery, "../etc/passwd", 50, "pages.cover_key_malformed"},
		{"an image needs an id", CoverImage, "hills", 50, "pages.cover_key_malformed"},
		{"an unknown source", CoverSource("VIDEO"), "x", 50, "pages.cover_source_unknown"},
		{"above the top", CoverGradient, "x", -1, "pages.cover_position_out_of_range"},
		{"below the foot", CoverGradient, "x", 101, "pages.cover_position_out_of_range"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cover, err := NewCover(c.source, c.key, c.position)
			if c.code == "" {
				if err != nil || cover.Source != c.source || cover.Position != c.position {
					t.Fatalf("NewCover = %+v, %v", cover, err)
				}
				return
			}
			var e *shared.Error
			if !errors.As(err, &e) || e.DetailCode != c.code {
				t.Fatalf("NewCover answered %v, want %s", err, c.code)
			}
		})
	}
}

// A change sets a cover, takes it away, or leaves it; the same cover again changes nothing.
func TestAPagesCoverIsChanged(t *testing.T) {
	page := Page{Title: "Handbook"}
	cover := Cover{Source: CoverGradient, Key: "indigo-teal", Position: 50}
	next, fields, err := page.Changed(Change{Cover: &CoverChange{Cover: &cover}}, shared.ID(""), page.UpdatedAt, nil)
	if err != nil || next.Cover == nil || *next.Cover != cover || len(fields) != 1 || fields[0] != FieldCover {
		t.Fatalf("setting: %+v %v %v", next.Cover, fields, err)
	}
	if _, fields, _ := next.Changed(Change{Cover: &CoverChange{Cover: &cover}}, shared.ID(""), page.UpdatedAt, nil); len(fields) != 0 {
		t.Fatalf("the same cover again changed %v", fields)
	}
	gone, fields, _ := next.Changed(Change{Cover: &CoverChange{}}, shared.ID(""), page.UpdatedAt, nil)
	if gone.Cover != nil || len(fields) != 1 {
		t.Fatalf("taking it away: %+v %v", gone.Cover, fields)
	}
}
