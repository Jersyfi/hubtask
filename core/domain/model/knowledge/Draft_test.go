// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-DOC-15: a change's title and body go to the draft, built on the draft there is; a draft that
// comes back to what is published holds nothing to publish.
func TestADraftCollectsTheTitleAndBodyOnly(t *testing.T) {
	by := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d001")
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	page := Page{ID: by, Title: "Handbook", Body: "v1"}
	s := func(v string) *string { return &v }
	cases := []struct {
		name        string
		current     *Draft
		change      Change
		title, body string
		differs     bool
	}{
		{"a body", nil, Change{Body: s("v2")}, "Handbook", "v2", true},
		{"on the draft there is", &Draft{Title: "Guide", Body: "v2"}, Change{Body: s("v3")}, "Guide", "v3", true},
		{"back to what is published", &Draft{Title: "Handbook", Body: "v2"}, Change{Body: s("v1")}, "Handbook", "v1", false},
		{"an unpublished page's mark holds no words", &Draft{Unpublished: true}, Change{Title: s("New")}, "New", "v1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			draft, differs, err := Drafted(page, c.current, c.change, by, now, nil)
			if err != nil || draft.Title != c.title || draft.Body != c.body || differs != c.differs || draft.UpdatedBy != by {
				t.Fatalf("draft %+v, differs %v, %v", draft, differs, err)
			}
		})
	}
	if _, _, err := Drafted(page, nil, Change{Title: s("")}, by, now, nil); err == nil {
		t.Fatal("an empty title was drafted")
	}
	icon := "I"
	draftPart, rest := DraftOnly(Change{Title: s("x"), Icon: &icon})
	if draftPart.Icon != nil || rest.Title != nil || rest.Icon == nil || !draftPart.Touches() || rest.Touches() {
		t.Fatalf("split %+v / %+v", draftPart, rest)
	}
}
