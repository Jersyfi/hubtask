// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/service/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	pages "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// PublicPageReader is the slice of the public page read this controller needs.
type PublicPageReader interface {
	Execute(ctx context.Context, token integration.PageLinkToken) (knowledge.PublicPage, error)
}

// FeedDoor serves the routes a token alone opens, delegated to it by the contract: a calendar
// feed's .ics document, a data feed's OData service and a page's public link. None is a catalogue
// entry: each answers a credential nobody in this system holds, in a format of its own, and there
// is nothing for MCP or an automation rule to call. A nil reader answers the route not wired.
type FeedDoor struct {
	Calendar CalendarFeedReader
	// Data serves a data feed's OData routes (UC-INT-15).
	Data CalendarFeedReader
	// Pages serves a page's public link (UC-DOC-08).
	Pages PublicPageReader
	// BaseURL is the installation's public address the links back are composed from (feedURL).
	BaseURL string
}

// publicPagePolicy keeps the document inert wherever it is opened: no script, nothing loaded from
// elsewhere, no form, never framed - the page is words somebody wrote, shown to anybody.
const publicPagePolicy = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// GetPublicPage answers GET /public/pages/{token}: a page opened by its public link, with no
// bearer credential. Parsing happens here and nothing else; every question is asked inwards, and
// every refusal looks the same from out here (T-21).
func (c *FeedDoor) GetPublicPage(w http.ResponseWriter, r *http.Request, presented string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Pages == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	token, err := integration.ParsePageLinkToken(presented)
	if err != nil {
		WriteProblem(w, shared.ErrNotFound.WithDetail("pages.link_not_found"), requestID)
		return
	}
	shown, err := c.Pages.Execute(r.Context(), token)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	page := shown.Page
	// The page alone: nothing names the pages around it.
	page.ParentID = ""
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", publicPagePolicy)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(pages.HTMLFile(page))); err != nil {
		reportExportFailure(r, err)
	}
}
