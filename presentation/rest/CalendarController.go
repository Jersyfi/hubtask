// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/calendar"
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

// CalendarFeedReader is the slice of the feed read this controller needs. An interface rather than
// the service, so the route can be tested without a database and the presentation layer keeps
// pointing inwards.
type CalendarFeedReader interface {
	Execute(ctx context.Context, token integration.FeedToken) (work.ExportedView, error)
}

// GetCalendarFeedDocument answers GET /calendar/{token}.ics - the one route in this API that
// carries no bearer credential.
//
// What happens here is parsing, and nothing else. The shape of the token is a string question and
// belongs to an adapter; whether the feed exists, is revoked, still has a view and still has an
// owner who may read it are all decided inwards of here (ADR-0005). Every one of those answers
// looks the same from out here, which is the point: a route that distinguished them would answer
// questions for whoever is trying tokens (T-21).
func (c *RestController) GetCalendarFeedDocument(
	w http.ResponseWriter, r *http.Request, presented string,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.CalendarFeeds == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	token, err := integration.ParseFeedToken(presented)
	if err != nil {
		// A malformed token answers what an unknown one answers. Not the parser's own refusal:
		// "that is not the right shape" tells somebody guessing that the shape is what to fix.
		WriteProblem(w, feedNotFound(), requestID)
		return
	}

	exported, err := c.CalendarFeeds.Execute(r.Context(), token)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// The owner's own zone, which is what decides which day an all-day entry falls on for an
	// entry that carries no zone of its own.
	zone := zoneOr(exported.TimeZone, time.UTC)
	events := make([]calendar.Event, 0, len(exported.Items))
	for _, item := range exported.Items {
		event, dated := c.eventOf(item, zone)
		if !dated {
			continue
		}
		events = append(events, event)
	}

	writeCalendar(w, r, calendar.Calendar{
		Name:        exported.View.Name,
		GeneratedAt: exported.GeneratedAt,
		Events:      events,
	})
}

// eventOf turns one entry into a calendar entry, and says whether it is one at all: an entry with
// no due date is not a moment, and a calendar is a set of moments.
func (c *RestController) eventOf(item workmodel.WorkItem, zone *time.Location) (calendar.Event, bool) {
	if item.Due == nil {
		return calendar.Event{}, false
	}

	event := calendar.Event{
		UID:          item.ID.String() + "@hubtask",
		Summary:      item.Title,
		Start:        item.Due.At,
		URL:          c.itemURL(item.ID.String()),
		LastModified: item.UpdatedAt,
	}
	if item.Due.DateOnly {
		// The day it is due, read in the entry's own zone - or the owner's, for an entry stored
		// without one - and carried as that day's wall clock, which is the shape the renderer
		// takes a DATE in.
		day := item.Due.At.In(zoneOr(item.Due.TimeZone, zone))
		event.AllDay = true
		event.Start = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	}
	return event, true
}

// zoneOr loads a named zone, falling back rather than failing: a stored zone this installation no
// longer knows must not stop a subscription from answering.
func zoneOr(named string, fallback *time.Location) *time.Location {
	if named == "" {
		return fallback
	}
	loaded, err := time.LoadLocation(named)
	if err != nil {
		return fallback
	}
	return loaded
}

// feedNotFound is the one answer this route ever gives when it will not serve: the same code the
// application layer produces, built here as well so that a token that never reached the service
// is indistinguishable from one that did.
func feedNotFound() error {
	return shared.ErrNotFound.WithDetail("calendar.feed_not_found")
}
