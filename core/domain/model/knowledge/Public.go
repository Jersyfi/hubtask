// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"html"
	"regexp"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PublicView is what a page's public link may show beside the page's own words (UC-DOC-08): the
// pictures the page holds, each with the address it is drawn from, and the titles of the pages it
// mentions that have a public link of their own. Whatever is absent here is left out: an
// unknown visitor reads no other object's name and follows no internal address.
type PublicView struct {
	Pictures map[shared.ID]string
	Titles   map[shared.ID]string
}

// PublicHTMLFile writes a page as its public link shows it: HTMLFile's document without the page's
// identifiers, with its pictures drawn and its references resolved by the view.
func PublicHTMLFile(page Page, view PublicView) string { return htmlFile(page, &view) }

var publicName = regexp.MustCompile(`hubtask:(media|page)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

// MaxPublicNames bounds what one public page asks about, as reference.MaxPerText bounds a text.
const MaxPublicNames = 200

// PublicNames answers what a body names that its public page may show: the media it draws and the
// pages it mentions, once each, in their order, at most MaxPublicNames of each.
func PublicNames(body string) (media, pages []shared.ID) {
	seen := map[string]bool{}
	for _, match := range publicName.FindAllStringSubmatch(body, -1) {
		key := match[1] + "/" + strings.ToLower(match[2])
		if seen[key] {
			continue
		}
		seen[key] = true
		id := shared.ID(strings.ToLower(match[2]))
		if match[1] == "media" && len(media) < MaxPublicNames {
			media = append(media, id)
		} else if match[1] == "page" && len(pages) < MaxPublicNames {
			pages = append(pages, id)
		}
	}
	return media, pages
}

// spans writes a block's inline runs; without a view, as the export writes them.
func (v *PublicView) spans(runs []span) string {
	if v == nil {
		return htmlSpans(runs)
	}
	var out strings.Builder
	for _, r := range runs {
		if strings.HasPrefix(r.href, "hubtask:") {
			out.WriteString(v.reference(r.href, r.image))
			continue
		}
		out.WriteString(htmlSpans([]span{r}))
	}
	return out.String()
}

// reference draws one `hubtask:` address: a picture the page holds as an image, a public page's
// mention as its title without a link (its token is kept only as a hash), anything else as nothing.
func (v *PublicView) reference(address string, image bool) string {
	match := publicName.FindStringSubmatch(address)
	if match == nil || !strings.HasPrefix(address, match[0]) {
		return ""
	}
	id := shared.ID(strings.ToLower(match[2]))
	switch {
	case match[1] == "media" && image:
		if src, ok := v.Pictures[id]; ok {
			return `<img src="` + html.EscapeString(src) + `" alt="">`
		}
	case match[1] == "page":
		if title, ok := v.Titles[id]; ok {
			return html.EscapeString(title)
		}
	}
	return ""
}
