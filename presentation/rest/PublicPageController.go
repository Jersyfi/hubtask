// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/knowledge"
	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	pages "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// PublicPageReader is the slice of the public page read this controller needs.
type PublicPageReader interface {
	Execute(ctx context.Context, token integration.PageLinkToken) (knowledge.PublicPage, error)
	// Picture answers the tenant a picture the page draws is read in, or the link's 404.
	Picture(ctx context.Context, token integration.PageLinkToken, mediaID shared.ID) (shared.ID, error)
}

// PictureSender streams a picture's bytes once the page link has admitted it.
type PictureSender interface {
	Send(ctx context.Context, grant usecasemedia.Grant) (usecasemedia.Served, error)
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
	// Pictures sends the bytes of a public page's pictures (UC-DOC-08 check 5); nil draws none.
	Pictures PictureSender
	// Roadmaps serves a portfolio roadmap's public link (UC-GOL-03 check 15).
	Roadmaps PublicRoadmapReader
	// Forms serves a collection's public form (UC-SVC-13).
	Forms PublicFormDoor
	// Satisfaction answers a resolution mail's satisfaction link (UC-SVC-07 check 6).
	Satisfaction SatisfactionLinkDoor
	// Links answers the main address of the workspace a token names, which a feed's links, a data
	// feed's service root and a public page's pictures are composed from (UC-INS-19 check 9); nil
	// keeps them relative or unlinked.
	Links WorkspaceLinks
}

// publicPagePolicy keeps the document inert wherever it is opened: no script, nothing loaded from
// elsewhere, no form, never framed - the page is words somebody wrote, shown to anybody. Pictures
// load from the link's own media route alone; without the workspace's address, from this origin.
func (c *FeedDoor) publicPagePolicy(base, presented string) string {
	pictures := "'self'"
	if base != "" {
		pictures = strings.TrimSuffix(base, "/") + APIBasePath + "/public/pages/" + presented + "/media/"
	}
	return "default-src 'none'; img-src data: " + pictures + "; style-src 'unsafe-inline'; " +
		"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}

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
	view := pages.PublicView{Pictures: map[shared.ID]string{}, Titles: shown.Titles}
	if c.Pictures != nil {
		for _, id := range shown.Pictures {
			view.Pictures[id] = "./" + presented + "/media/" + id.String()
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The pictures load from the page's own workspace's main address (ADR-0095 §2).
	base, err := c.linkBase(r.Context(), token.TenantID())
	if err != nil {
		base = ""
	}
	w.Header().Set("Content-Security-Policy", c.publicPagePolicy(base, presented))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(pages.PublicHTMLFile(page, view))); err != nil {
		reportExportFailure(r, err)
	}
}

// GetPublicPageMedia answers GET /public/pages/{token}/media/{mediaId}: one picture the public
// page draws, streamed, never a rendering path - sandboxed, not sniffed, kept nowhere (T-11).
func (c *FeedDoor) GetPublicPageMedia(w http.ResponseWriter, r *http.Request, presented string, mediaID openapi.MediaId) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Pages == nil || c.Pictures == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	token, err := integration.ParsePageLinkToken(presented)
	if err != nil {
		WriteProblem(w, shared.ErrNotFound.WithDetail("pages.link_not_found"), requestID)
		return
	}
	id := shared.ID(mediaID.String())
	tenant, err := c.Pages.Picture(r.Context(), token, id)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	served, err := c.Pictures.Send(r.Context(), usecasemedia.Grant{TenantID: tenant, MediaID: id})
	if errors.Is(err, shared.ErrNotFound) {
		err = shared.ErrNotFound.WithDetail("pages.link_not_found")
	}
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	defer func() { _ = served.Content.Content.Close() }()
	w.Header().Set("Content-Type", served.Content.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(served.Content.Size, 10))
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, served.Content.Content); err != nil {
		logStreamFailure(r.Context(), mediaID, err)
	}
}
