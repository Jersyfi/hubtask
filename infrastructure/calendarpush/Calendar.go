// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendarpush writes a calendar push's events through Microsoft Graph and Google's
// Calendar API, spoken through the guarded client (rule 6): an event in the signed-in person's
// own calendar, shown as free and without a reminder, holding the entry's title, its moment and
// a link back - the feed's own shape (data-protection.md §9).
package calendarpush

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/calendarpush"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// TargetClass labels the calls for the outbound duration histogram.
const TargetClass = "calendar_push"

// timedLength is how long a timed entry's event lasts: a due date is a moment, and a calendar
// draws an event with no length as nothing.
const timedLength = 30 * time.Minute

const (
	graphEvents  = "https://graph.microsoft.com/v1.0/me/events"
	googleEvents = "https://www.googleapis.com/calendar/v3/calendars/primary/events"
)

// Calendar is both providers' events.
type Calendar struct {
	Client httpport.Port
}

var _ port.Calendar = Calendar{}

func (c Calendar) Put(ctx context.Context, provider integration.PushProvider, access secret.Secret, remoteID string, event integration.PushEvent) (string, error) {
	body, base := bodyOf(provider, event)
	method, address := http.MethodPost, base
	if remoteID != "" {
		method, address = http.MethodPatch, base+"/"+url.PathEscape(remoteID)
	}
	status, answer, err := c.call(ctx, method, address, access, body)
	if err != nil {
		return "", err
	}
	if remoteID != "" && (status == http.StatusNotFound || status == http.StatusGone) {
		// Removed at the provider by its owner: written anew.
		status, answer, err = c.call(ctx, http.MethodPost, base, access, body)
		if err != nil {
			return "", err
		}
	}
	if err := refusal(status); err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(answer, &created) != nil || created.ID == "" {
		if remoteID != "" && status/100 == 2 {
			return remoteID, nil
		}
		return "", unreachable(errors.New("calendar push: the provider answered no event"))
	}
	return created.ID, nil
}

func (c Calendar) Delete(ctx context.Context, provider integration.PushProvider, access secret.Secret, remoteID string) error {
	_, base := bodyOf(provider, integration.PushEvent{})
	status, _, err := c.call(ctx, http.MethodDelete, base+"/"+url.PathEscape(remoteID), access, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		return nil
	}
	return refusal(status)
}

func (c Calendar) call(ctx context.Context, method, address string, access secret.Secret, body []byte) (int, []byte, error) {
	if c.Client == nil {
		return 0, nil, unreachable(errors.New("calendar push: no outbound client is wired"))
	}
	header := map[string][]string{
		"Authorization": {"Bearer " + access.Reveal()},
		"Accept":        {"application/json"},
	}
	if body != nil {
		header["Content-Type"] = []string{"application/json"}
	}
	response, err := c.Client.Do(ctx, httpport.Request{Method: method, URL: address, Header: header, Body: body, TargetClass: TargetClass})
	if err != nil {
		return 0, nil, unreachable(err)
	}
	return response.Status, response.Body, nil
}

// refusal codes a provider's answer: an access token just drawn that is not taken is a grant
// gone; a throttle or the provider's own failure passes.
func refusal(status int) error {
	switch {
	case status/100 == 2:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return shared.ErrForbidden.WithDetail("calendar_push.grant_revoked").
			WithCause(errors.New("calendar push: the provider answered " + http.StatusText(status)))
	}
	return unreachable(errors.New("calendar push: the provider answered " + http.StatusText(status)))
}

func unreachable(cause error) error {
	return shared.ErrUnavailable.WithDetail("calendar_push.provider_unreachable").WithCause(cause)
}

// bodyOf answers the event in the provider's words and its events' address.
func bodyOf(provider integration.PushProvider, event integration.PushEvent) ([]byte, string) {
	start, end := event.Start.UTC(), event.Start.UTC().Add(timedLength)
	if event.AllDay {
		end = start.AddDate(0, 0, 1)
	}
	if provider == integration.PushGoogle {
		when := func(at time.Time) map[string]any {
			if event.AllDay {
				return map[string]any{"date": at.Format("2006-01-02")}
			}
			return map[string]any{"dateTime": at.Format(time.RFC3339), "timeZone": "UTC"}
		}
		body, _ := json.Marshal(map[string]any{
			"summary": event.Summary, "description": event.URL, "source": map[string]any{"title": event.Summary, "url": event.URL},
			"start": when(start), "end": when(end), "transparency": "transparent",
			"reminders": map[string]any{"useDefault": false},
		})
		return body, googleEvents
	}
	layout := "2006-01-02T15:04:05"
	if event.AllDay {
		layout = "2006-01-02T00:00:00"
	}
	body, _ := json.Marshal(map[string]any{
		"subject": event.Summary, "body": map[string]any{"contentType": "text", "content": event.URL},
		"start":    map[string]any{"dateTime": start.Format(layout), "timeZone": "UTC"},
		"end":      map[string]any{"dateTime": end.Format(layout), "timeZone": "UTC"},
		"isAllDay": event.AllDay, "showAs": "free", "isReminderOn": false,
	})
	return body, graphEvents
}
