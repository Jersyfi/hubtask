// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// CalDAV, the write half (P-07).
//
// A todo ticked in a client is PUT back with STATUS:COMPLETED; one whose date was dragged with a
// new DUE; one deleted is DELETEd. The controller reads the VTODO, diffs it against the entry,
// and performs the ordinary use cases as the token's account through the registry - which is
// where the input is validated and the permission decided, exactly as a push performs them
// (N-04). Nothing here is a second write path: a completion over CalDAV is CompleteWorkItem
// with the same audit entry, the same activity, the same change log entry as a click.
//
// Two rules a calendar client would otherwise break silently. A PUT to an existing todo needs
// If-Match, or it is 428: a client that lost the race must not overwrite a change it never saw.
// And a property the product does not model - a DESCRIPTION, a PRIORITY, a CATEGORIES - is
// refused by name rather than dropped: the client would read the entry back without it, and
// that is the silent ignoring the contract forbids (domain-model.md §2).

// Catalogue is the use case registry as the controller invokes it.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// nsHubtask is the namespace of the tree's own error conditions, in the <D:error> a refused
// PUT carries (RFC 4918 §16).
const nsHubtask = "https://hubtask.eu/ns/caldav"

const (
	updateWorkItemUseCase   = "UpdateWorkItem"
	setDueDateUseCase       = "SetDueDate"
	clearDueDateUseCase     = "ClearDueDate"
	completeWorkItemUseCase = "CompleteWorkItem"
	reopenWorkItemUseCase   = "ReopenWorkItem"
	trashWorkItemUseCase    = "TrashWorkItem"
	createWorkItemUseCase   = "CreateWorkItem"
)

func (c *Controller) put(w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, target resource) {
	if target.kind != "member" {
		writeStatus(w, http.StatusMethodNotAllowed)
		return
	}
	if c.UseCases == nil {
		writeStatus(w, http.StatusForbidden)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeStatus(w, http.StatusBadRequest)
		return
	}
	parsed, err := ParseTodo(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "valid-calendar-object-resource", err.Error())
		return
	}
	if len(parsed.Unsupported) > 0 {
		writeError(w, http.StatusForbidden, "property-not-supported", strings.Join(parsed.Unsupported, ","))
		return
	}
	if strings.TrimSpace(parsed.Summary) == "" {
		writeError(w, http.StatusForbidden, "summary-required", "SUMMARY")
		return
	}

	feed, err := c.feed(r.Context(), actor, target.feed)
	if err != nil {
		c.refuse(w, err)
		return
	}
	calendar, err := c.calendar(r.Context(), actor, feed)
	if err != nil {
		c.refuse(w, err)
		return
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))

	existing, exists := calendar.byID[target.member]
	if !exists {
		if ifMatch != "" {
			writeStatus(w, http.StatusPreconditionFailed)
			return
		}
		c.create(w, r, actor, calendar, target.member, parsed)
		return
	}
	if ifMatch == "" {
		writeStatus(w, http.StatusPreconditionRequired)
		return
	}
	if ifMatch != "*" && ifMatch != existing.etag {
		writeStatus(w, http.StatusPreconditionFailed)
		return
	}

	version, err := c.apply(r.Context(), actor, existing, parsed)
	if err != nil {
		c.refuse(w, err)
		return
	}
	w.Header().Set("ETag", `"`+strconv.Itoa(version)+`"`)
	w.WriteHeader(http.StatusNoContent)
}

// apply performs the differences between the todo the client sent and the entry, each through
// its use case, and answers the version the entry is at afterwards.
func (c *Controller) apply(ctx context.Context, actor appshared.ActorContext, existing member, parsed ParsedTodo) (int, error) {
	version := existing.version
	invoke := func(name string, in usecase.Input) error {
		in["item_id"] = existing.id
		in["expected_version"] = version
		out, err := c.UseCases.Invoke(ctx, name, actor, in)
		if err != nil {
			return err
		}
		if v := out.Int("version"); v > 0 {
			version = v
		}
		return nil
	}

	if parsed.Summary != existing.todo.Summary {
		if err := invoke(updateWorkItemUseCase, usecase.Input{"title": parsed.Summary}); err != nil {
			return version, err
		}
	}
	if parsed.StartSet && !parsed.Start.Equal(existing.start) {
		if err := invoke(updateWorkItemUseCase, usecase.Input{"start_at": parsed.Start.UTC().Format(time.RFC3339)}); err != nil {
			return version, err
		}
	}
	switch {
	case parsed.Due.IsZero() && !existing.todo.Due.IsZero():
		if err := invoke(clearDueDateUseCase, usecase.Input{}); err != nil {
			return version, err
		}
	case !parsed.Due.IsZero() && (parsed.DueDate != existing.todo.AllDay || !parsed.Due.Equal(existing.todo.Due)):
		if err := invoke(setDueDateUseCase, dueInput(parsed, actor)); err != nil {
			return version, err
		}
	}
	switch {
	case parsed.Completed && existing.todo.Completed.IsZero():
		if err := invoke(completeWorkItemUseCase, usecase.Input{}); err != nil {
			return version, err
		}
	case !parsed.Completed && !existing.todo.Completed.IsZero():
		if err := invoke(reopenWorkItemUseCase, usecase.Input{}); err != nil {
			return version, err
		}
	}
	return version, nil
}

// dueInput spells a parsed DUE as SetDueDate's trio. A DATE is the day in the account's zone -
// the zone the read side renders it back in - so a date dragged in a client lands on the same
// day it was dropped on; a DATE-TIME is the instant.
func dueInput(parsed ParsedTodo, actor appshared.ActorContext) usecase.Input {
	if parsed.DueDate {
		zone := actor.TimeZone
		location := zoneOr(zone, time.UTC)
		if zone == "" {
			zone = "UTC"
		}
		day := time.Date(parsed.Due.Year(), parsed.Due.Month(), parsed.Due.Day(), 0, 0, 0, 0, location)
		return usecase.Input{"due_at": day.Format(time.RFC3339), "due_date_only": true, "due_time_zone": zone}
	}
	return usecase.Input{"due_at": parsed.Due.UTC().Format(time.RFC3339)}
}

// create answers a PUT to an address the calendar has no member at: a todo made in the client.
//
// The address a client chooses is its UID, and the entry created has to live at that address
// afterwards or the client will find its todo gone and make it again. CreateWorkItem takes a
// client identifier when it is a UUIDv7 (N-04, offline-sync.md §9); a client whose UIDs are
// anything else is refused by name, and creates its todos in the application instead. The
// collection is the view's, where the view names exactly one - a todo made in a calendar has to
// land somewhere the person meant.
func (c *Controller) create(w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, calendar calendarView, address string, parsed ParsedTodo) {
	id, err := shared.ParseID(address)
	if err != nil || !isUUIDv7(id) {
		writeError(w, http.StatusForbidden, "creation-needs-uuidv7", address)
		return
	}
	collection, ok := singleCollection(calendar.view)
	if !ok {
		writeError(w, http.StatusForbidden, "no-single-collection", calendar.name)
		return
	}
	in := usecase.Input{
		"id": id.String(), "type": "TASK", "collection_id": collection.String(), "title": parsed.Summary,
	}
	if parsed.StartSet {
		in["start_at"] = parsed.Start.UTC().Format(time.RFC3339)
	}
	if !parsed.Due.IsZero() {
		for k, v := range dueInput(parsed, actor) {
			in[k] = v
		}
	}
	out, err := c.UseCases.Invoke(r.Context(), createWorkItemUseCase, actor, in)
	if err != nil {
		c.refuse(w, err)
		return
	}
	version := out.Int("version")
	if parsed.Completed {
		done, err := c.UseCases.Invoke(r.Context(), completeWorkItemUseCase, actor,
			usecase.Input{"item_id": id.String(), "expected_version": version})
		if err != nil {
			c.refuse(w, err)
			return
		}
		version = done.Int("version")
	}
	w.Header().Set("ETag", `"`+strconv.Itoa(version)+`"`)
	w.WriteHeader(http.StatusCreated)
}

// singleCollection answers the one collection a view names, where it names one: through its
// query's anchor, or through a scope that is a collection.
func singleCollection(saved view.SavedView) (shared.ID, bool) {
	if raw, ok := saved.Query["collection_id"].(string); ok && raw != "" {
		if id, err := shared.ParseID(raw); err == nil {
			return id, true
		}
	}
	if saved.ScopeType == view.ViewScopeCollection && !saved.ScopeID.IsZero() {
		return saved.ScopeID, true
	}
	return shared.ID(""), false
}

// isUUIDv7 reads the version nibble.
func isUUIDv7(id shared.ID) bool {
	s := id.String()
	return len(s) == 36 && s[14] == '7'
}

func (c *Controller) delete(w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, target resource) {
	if target.kind != "member" {
		writeStatus(w, http.StatusMethodNotAllowed)
		return
	}
	if c.UseCases == nil {
		writeStatus(w, http.StatusForbidden)
		return
	}
	feed, err := c.feed(r.Context(), actor, target.feed)
	if err != nil {
		c.refuse(w, err)
		return
	}
	calendar, err := c.calendar(r.Context(), actor, feed)
	if err != nil {
		c.refuse(w, err)
		return
	}
	existing, exists := calendar.byID[target.member]
	if !exists {
		writeStatus(w, http.StatusNotFound)
		return
	}
	in := usecase.Input{"item_id": existing.id}
	if ifMatch := strings.TrimSpace(r.Header.Get("If-Match")); ifMatch != "" && ifMatch != "*" {
		if ifMatch != existing.etag {
			writeStatus(w, http.StatusPreconditionFailed)
			return
		}
		in["expected_version"] = existing.version
	}
	if _, err := c.UseCases.Invoke(r.Context(), trashWorkItemUseCase, actor, in); err != nil {
		c.refuse(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeError is a refusal with a reason a client can show: RFC 4918 §16's <D:error> with one
// condition element, the property or the address it is about as its text.
func writeError(w http.ResponseWriter, status int, condition, detail string) {
	body, err := xml.Marshal(el(nsDAV, "error", text(nsHubtask, condition, detail)))
	if err != nil {
		writeStatus(w, status)
		return
	}
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}
