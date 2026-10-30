// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/odata"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A data feed (UC-INT-15): a saved view's rows as an OData v4 service for a reporting tool. As on
// the calendar's route, what happens here is parsing and rendering; whether the token opens a
// feed, and which rows, is decided inwards (ADR-0005), and every refusal looks the same (T-21).

// dataFeedRoot is the service root a reporting tool is given, from the configured base URL for
// feedURL's reason.
func dataFeedRoot(base, token string) string {
	path := APIBasePath + "/odata/" + token
	if base == "" {
		return path
	}
	return strings.TrimRight(base, "/") + path
}

// dataFeed reads the feed's rows, or answers the one refusal and false.
func (c *FeedDoor) dataFeed(w http.ResponseWriter, r *http.Request, presented string) ([]odata.Entry, bool) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Data == nil {
		WriteProblem(w, errNotWired, requestID)
		return nil, false
	}
	token, err := integration.ParseFeedToken(presented)
	if err != nil {
		WriteProblem(w, feedNotFound(), requestID)
		return nil, false
	}
	exported, err := c.Data.Execute(r.Context(), token)
	if err != nil {
		WriteProblem(w, err, requestID)
		return nil, false
	}
	rows := make([]odata.Entry, 0, len(exported.Items))
	for _, item := range exported.Items {
		e := odata.Entry{
			ID: item.ID.String(), Number: item.Number, Level: string(item.Type), EntryType: item.EntryType,
			Title: item.Title, Status: item.Status, Completed: item.Completion.IsCompleted,
			CompletedAt: item.Completion.CompletedAt, StartAt: item.StartAt, CollectionID: item.CollectionID.String(),
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		}
		if item.Due != nil {
			due := item.Due.At
			e.DueAt = &due
		}
		if !item.AssigneeID.IsZero() {
			e.AssigneeID = item.AssigneeID.String()
		}
		if !item.ParentID.IsZero() {
			e.ParentID = item.ParentID.String()
		}
		rows = append(rows, e)
	}
	return rows, true
}

// odataHeaders marks an answer as OData v4's.
func odataHeaders(w http.ResponseWriter) {
	w.Header().Set("OData-Version", "4.0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// GetDataFeedService answers GET /odata/{token}.
func (c *FeedDoor) GetDataFeedService(w http.ResponseWriter, r *http.Request, token openapi.DataFeedToken) {
	if _, ok := c.dataFeed(w, r, token); !ok {
		return
	}
	odataHeaders(w)
	writeJSON(w, r, http.StatusOK, odata.Service(dataFeedRoot(c.BaseURL, token)))
}

// GetDataFeedMetadata answers GET /odata/{token}/$metadata.
func (c *FeedDoor) GetDataFeedMetadata(w http.ResponseWriter, r *http.Request, token openapi.DataFeedToken) {
	if _, ok := c.dataFeed(w, r, token); !ok {
		return
	}
	odataHeaders(w)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(odata.Metadata())
}

// GetDataFeedEntries answers GET /odata/{token}/Entries.
func (c *FeedDoor) GetDataFeedEntries(w http.ResponseWriter, r *http.Request, token openapi.DataFeedToken, params openapi.GetDataFeedEntriesParams) {
	rows, ok := c.dataFeed(w, r, token)
	if !ok {
		return
	}
	page := odata.Page{}
	if params.Skip != nil {
		page.Skip = *params.Skip
	}
	if params.Top != nil {
		page.Top, page.HasTop = *params.Top, true
	}
	odataHeaders(w)
	writeJSON(w, r, http.StatusOK, odata.Entries(dataFeedRoot(c.BaseURL, token), rows, page))
}
