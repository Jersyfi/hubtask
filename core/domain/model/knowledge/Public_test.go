// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	publicPicture = "00000000-0000-0000-0000-00000000a001"
	publicFile    = "00000000-0000-0000-0000-00000000a002"
	publicPage    = "00000000-0000-0000-0000-00000000b001"
	privatePage   = "00000000-0000-0000-0000-00000000b002"
	anEntry       = "00000000-0000-0000-0000-00000000c001"
)

const publicBody = "Intro [](hubtask:page/" + publicPage + ") and [](hubtask:page/" + privatePage + ") and [](hubtask:entry/" + anEntry + ").\n\n" +
	"![](hubtask:media/" + publicPicture + ")\n\n" +
	"![](hubtask:media/" + publicFile + ")\n\n" +
	"![](hubtask:entry/" + anEntry + ")\n\n" +
	"Inline ![](hubtask:media/" + publicPicture + ") picture.\n"

// UC-DOC-08 check 5: the public page draws the pictures it holds from the link's own route and a
// mention of a public page as that page's title; every other mention, a file that is no picture
// and an internal address are left out.
func TestThePublicPageShowsWhatThePageShows(t *testing.T) {
	page := newPage(t, "Guide", publicBody)
	out := PublicHTMLFile(page, PublicView{
		Pictures: map[shared.ID]string{shared.ID(publicPicture): "media/" + publicPicture},
		Titles:   map[shared.ID]string{shared.ID(publicPage): "Getting <started>"},
	})
	for _, want := range []string{
		`<img src="media/` + publicPicture + `" alt="">`,
		"Intro Getting &lt;started&gt; and  and .",
		`Inline <img src="media/` + publicPicture + `" alt=""> picture.`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the public page lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"hubtask:", privatePage, anEntry, publicFile, "[entry]", "[page]", "[media]", "<a "} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the public page carries %q:\n%s", unwanted, out)
		}
	}
}

// UC-DOC-08 check 5: what the public page asks about is what its body names - its pictures and the
// pages it mentions - once each.
func TestThePublicPageAsksOnlyWhatTheBodyNames(t *testing.T) {
	media, pages := PublicNames(publicBody)
	if !slices.Equal(media, []shared.ID{shared.ID(publicPicture), shared.ID(publicFile)}) {
		t.Errorf("media = %v", media)
	}
	if !slices.Equal(pages, []shared.ID{shared.ID(publicPage), shared.ID(privatePage)}) {
		t.Errorf("pages = %v", pages)
	}
}
