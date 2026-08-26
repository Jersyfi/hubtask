// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The calendar feeds (D-08). The controller holds no rules: who may mint a feed over which view,
// and whose feed may be revoked, are decided inwards of here. What is decided here is the one
// thing that is genuinely this layer's - the address a client subscribes to, which is a URL and
// therefore a question about this installation rather than about the domain.

const (
	listCalendarFeedsUseCase  = "ListCalendarFeeds"
	createCalendarFeedUseCase = "CreateCalendarFeed"
	revokeCalendarFeedUseCase = "RevokeCalendarFeed"
)

// ListCalendarFeeds answers GET /integrations/calendar-feeds.
func (c *RestController) ListCalendarFeeds(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), listCalendarFeedsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	feeds := make([]openapi.CalendarFeed, 0, len(rows))
	for _, row := range rows {
		feeds = append(feeds, calendarFeedResponse(row))
	}
	writeJSON(w, r, http.StatusOK, feeds)
}

// CreateCalendarFeed answers POST /integrations/calendar-feeds.
func (c *RestController) CreateCalendarFeed(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateCalendarFeedParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.CalendarFeedCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), createCalendarFeedUseCase, actorOf(r),
		usecase.Input{"view_id": body.ViewId.String()})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	feed := calendarFeedResponse(out)
	token := out.String("token")
	writeJSON(w, r, http.StatusCreated, openapi.CalendarFeedSecret{
		Id:        feed.Id,
		AccountId: feed.AccountId,
		ViewId:    feed.ViewId,
		CreatedAt: feed.CreatedAt,
		RevokedAt: feed.RevokedAt,
		Token:     token,
		Url:       c.feedURL(token),
	})
}

// RevokeCalendarFeed answers DELETE /integrations/calendar-feeds/{feedId}.
func (c *RestController) RevokeCalendarFeed(
	w http.ResponseWriter, r *http.Request, feedID openapi.FeedId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(r.Context(), revokeCalendarFeedUseCase, actorOf(r),
		usecase.Input{"feed_id": feedID.String()}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// feedURL is the address a calendar client is given. Built from the configured base URL rather
// than from the request's own Host header: a Host is whatever the caller sent, and a URL built
// from it would let one request decide what address the next person's calendar client stores.
//
// An installation with no base URL configured answers a relative address rather than a wrong
// absolute one - the client that just minted it knows which server it asked.
func (c *RestController) feedURL(token string) string {
	path := APIBasePath + "/calendar/" + token + ".ics"
	if c.BaseURL == "" {
		return path
	}
	return strings.TrimRight(c.BaseURL, "/") + path
}

// calendarFeedResponse maps the catalogue's projection onto the contract's shape. The token is
// not among the fields: it is carried separately, by the one operation that answers it.
func calendarFeedResponse(out usecase.Output) openapi.CalendarFeed {
	feed := openapi.CalendarFeed{
		Id:        uuidValue(out.String("id")),
		AccountId: uuidValue(out.String("account_id")),
		CreatedAt: timeValue(out["created_at"]),
	}
	if view := out.String("view_id"); view != "" {
		viewID := uuidValue(view)
		feed.ViewId = &viewID
	}
	if at := timeValue(out["revoked_at"]); !at.IsZero() {
		feed.RevokedAt = &at
	}
	return feed
}
