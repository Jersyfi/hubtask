// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// CalDAV, the read half (P-06, RFC 4791).
//
// One calendar per calendar feed the account owns: the feed already names a view, an owner and
// a token, and a CalDAV calendar is that feed with a second transport. The principal is the
// account, the calendar home lists its feeds, each calendar holds one VTODO per entry the view
// answers, and the whole tree is served by the `api` role as the feed's owner - the same
// selection the ICS feed performs, through the same use case, with the same restraint about what
// an entry carries (D-08).
//
// Authentication is HTTP Basic with a personal access token as the password, resolved by the
// middleware the API's routes go through (presentation/rest/Auth.go): every CalDAV client can
// send Basic and none can send a bearer, and a token is revocable where a password is not. The
// controller therefore sees an actor and asks nothing about credentials; what it asks is
// answered inwards of here, with the caller's own permission (ADR-0005).
//
// What is deliberately not here: `sync-token` and the sync-collection REPORT. A token a client
// presents later expects only what changed, deletions included, and a calendar that answered
// everything current would leave a deleted todo on the client for ever. `getctag` says whether
// anything changed and a depth-1 PROPFIND says what, which is how every client on the row
// discovers a deletion today; RFC 6578 waits for a change log per view.

// Prefix is where the tree is mounted. Not under /api/v1: a CalDAV client is configured with a
// host and discovers the rest (RFC 6764), and the tree is WebDAV rather than the REST contract.
const Prefix = "/caldav/"

// WellKnown is the discovery address (RFC 6764 §5): a client given a host asks here first and
// follows the redirect to Prefix.
const WellKnown = "/.well-known/caldav"

const (
	nsDAV    = "DAV:"
	nsCalDAV = "urn:ietf:params:xml:ns:caldav"
	nsCS     = "http://calendarserver.org/ns/"
)

// FeedLister answers the calendars: the account's own feeds and no others.
type FeedLister interface {
	Execute(ctx context.Context, actor appshared.ActorContext) ([]integration.CalendarFeed, error)
}

// ViewSelector answers a calendar's members: the view's result, selected as the actor and
// recorded nowhere per fetch, exactly as the feed selects (ExportView.Select).
type ViewSelector interface {
	Select(ctx context.Context, actor appshared.ActorContext, viewID shared.ID) (work.ExportedView, error)
}

// Controller serves the tree.
type Controller struct {
	Feeds FeedLister
	Views ViewSelector
	// BaseURL is the installation's own address, for the URL a todo carries back into the
	// product; empty means a relative one.
	BaseURL string
	// Now stamps DTSTAMP where the selection carries no moment of its own.
	Now func() time.Time
}

/* ── Routing ───────────────────────────────────────────────────────────────────────────── */

type resource struct {
	kind    string // root, principal, home, calendar, member
	account string
	feed    string
	member  string
}

// resolve reads the address. The tree is shallow and fixed, so the path is a switch rather than
// a router: a segment in the wrong place is a 404 like any other.
func resolve(p string) (resource, bool) {
	rest := strings.TrimPrefix(p, Prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	switch {
	case rest == "" || rest == "/":
		return resource{kind: "root"}, true
	case len(parts) == 2 && parts[0] == "principals":
		return resource{kind: "principal", account: parts[1]}, true
	case len(parts) == 2 && parts[0] == "calendars":
		return resource{kind: "home", account: parts[1]}, true
	case len(parts) == 3 && parts[0] == "calendars":
		return resource{kind: "calendar", account: parts[1], feed: parts[2]}, true
	case len(parts) == 4 && parts[0] == "calendars" && strings.HasSuffix(parts[3], ".ics"):
		return resource{kind: "member", account: parts[1], feed: parts[2], member: strings.TrimSuffix(parts[3], ".ics")}, true
	}
	return resource{}, false
}

func principalPath(account string) string { return Prefix + "principals/" + account + "/" }
func homePath(account string) string      { return Prefix + "calendars/" + account + "/" }
func calendarPath(account, feed string) string {
	return homePath(account) + feed + "/"
}
func memberPath(account, feed, member string) string {
	return calendarPath(account, feed) + member + ".ics"
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := appshared.ActorFrom(r.Context())
	if !ok || actor.AccountID.IsZero() {
		// The middleware answers the missing credential; an actor without an account (a
		// service account, an automation) has no calendars and is told so.
		writeStatus(w, http.StatusForbidden)
		return
	}
	target, found := resolve(r.URL.Path)
	if !found {
		writeStatus(w, http.StatusNotFound)
		return
	}
	// The account in the address is the actor's or it is nobody's: a token grants nothing the
	// owner cannot see, and another account's tree is answered as absent (T-04's shape).
	if target.kind != "root" && target.account != actor.AccountID.String() {
		writeStatus(w, http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("DAV", "1, 3, calendar-access")
		w.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD")
		w.WriteHeader(http.StatusOK)
	case "PROPFIND":
		c.propfind(w, r, actor, target)
	case "REPORT":
		c.report(w, r, actor, target)
	case http.MethodGet, http.MethodHead:
		c.get(w, r, actor, target)
	default:
		w.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD")
		writeStatus(w, http.StatusMethodNotAllowed)
	}
}

/* ── XML ───────────────────────────────────────────────────────────────────────────────── */

// element is the one shape every request is read into and every response is written from: a
// name, attributes, text, children. Reading through encoding/xml means no entity is ever
// expanded beyond the five the language defines - the decoder knows no DTD - and writing through
// it means a title with a `<` in it is text, never markup.
type element struct {
	XMLName  xml.Name
	Attr     []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []element  `xml:",any"`
}

func el(space, local string, children ...element) element {
	return element{XMLName: xml.Name{Space: space, Local: local}, Children: children}
}

func text(space, local, value string) element {
	return element{XMLName: xml.Name{Space: space, Local: local}, Text: value}
}

func href(p string) element { return text(nsDAV, "href", p) }

func (e element) find(space, local string) (element, bool) {
	for _, child := range e.Children {
		if child.XMLName.Local == local && (space == "" || child.XMLName.Space == space) {
			return child, true
		}
	}
	return element{}, false
}

func readBody(r *http.Request) (element, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return element{}, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return element{}, nil
	}
	var root element
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.Strict = true
	if err := decoder.Decode(&root); err != nil {
		return element{}, err
	}
	return root, nil
}

func writeMultistatus(w http.ResponseWriter, responses []element) {
	body, err := xml.Marshal(el(nsDAV, "multistatus", responses...))
	if err != nil {
		writeStatus(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}

func writeStatus(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

// response builds one <D:response>: the found properties at 200, the asked-for ones the resource
// does not have at 404 - which is the answer RFC 4918 §9.1 gives for "not this property here",
// and what lets a client ask for everything it knows without a server refusing the request.
func response(p string, asked []xml.Name, have map[xml.Name]element) element {
	var found, missing []element
	for _, name := range asked {
		if value, ok := have[name]; ok {
			found = append(found, value)
		} else {
			missing = append(missing, element{XMLName: name})
		}
	}
	children := []element{href(p)}
	if len(found) > 0 {
		children = append(children, el(nsDAV, "propstat", el(nsDAV, "prop", found...), text(nsDAV, "status", "HTTP/1.1 200 OK")))
	}
	if len(missing) > 0 {
		children = append(children, el(nsDAV, "propstat", el(nsDAV, "prop", missing...), text(nsDAV, "status", "HTTP/1.1 404 Not Found")))
	}
	return el(nsDAV, "response", children...)
}

// askedProps reads which properties a PROPFIND wants; allprop and an empty body mean every one
// the resource has.
func askedProps(body element, have map[xml.Name]element) []xml.Name {
	if prop, ok := body.find(nsDAV, "prop"); ok && len(prop.Children) > 0 {
		names := make([]xml.Name, 0, len(prop.Children))
		for _, child := range prop.Children {
			names = append(names, child.XMLName)
		}
		return names
	}
	names := make([]xml.Name, 0, len(have))
	for name := range have {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i].Space != names[j].Space {
			return names[i].Space < names[j].Space
		}
		return names[i].Local < names[j].Local
	})
	return names
}

func name(space, local string) xml.Name { return xml.Name{Space: space, Local: local} }

/* ── PROPFIND ──────────────────────────────────────────────────────────────────────────── */

func (c *Controller) propfind(w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, target resource) {
	body, err := readBody(r)
	if err != nil {
		writeStatus(w, http.StatusBadRequest)
		return
	}
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "0"
	}
	account := actor.AccountID.String()

	var responses []element
	switch target.kind {
	case "root":
		have := c.rootProps(account)
		responses = append(responses, response(Prefix, askedProps(body, have), have))
	case "principal":
		have := c.principalProps(actor)
		responses = append(responses, response(principalPath(account), askedProps(body, have), have))
	case "home":
		have := c.homeProps(actor)
		responses = append(responses, response(homePath(account), askedProps(body, have), have))
		if depth != "0" {
			feeds, err := c.Feeds.Execute(r.Context(), actor)
			if err != nil {
				c.refuse(w, err)
				return
			}
			for _, feed := range feeds {
				if !feed.RevokedAt.IsZero() || feed.ViewID.IsZero() {
					continue
				}
				calendar, err := c.calendar(r.Context(), actor, feed)
				if err != nil {
					c.refuse(w, err)
					return
				}
				responses = append(responses, response(calendarPath(account, feed.ID.String()), askedProps(body, calendar.props), calendar.props))
			}
		}
	case "calendar", "member":
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
		if target.kind == "calendar" {
			responses = append(responses, response(calendarPath(account, feed.ID.String()), askedProps(body, calendar.props), calendar.props))
			if depth != "0" {
				for _, member := range calendar.members {
					responses = append(responses, response(member.path, askedProps(body, member.props(false)), member.props(false)))
				}
			}
		} else {
			member, ok := calendar.byID[target.member]
			if !ok {
				writeStatus(w, http.StatusNotFound)
				return
			}
			responses = append(responses, response(member.path, askedProps(body, member.props(false)), member.props(false)))
		}
	}
	writeMultistatus(w, responses)
}

func (c *Controller) rootProps(account string) map[xml.Name]element {
	return map[xml.Name]element{
		name(nsDAV, "resourcetype"):           el(nsDAV, "resourcetype", el(nsDAV, "collection")),
		name(nsDAV, "current-user-principal"): el(nsDAV, "current-user-principal", href(principalPath(account))),
		name(nsDAV, "principal-URL"):          el(nsDAV, "principal-URL", href(principalPath(account))),
		name(nsDAV, "displayname"):            text(nsDAV, "displayname", "Hubtask"),
	}
}

func (c *Controller) principalProps(actor appshared.ActorContext) map[xml.Name]element {
	account := actor.AccountID.String()
	display := actor.AccountName
	if display == "" {
		display = account
	}
	return map[xml.Name]element{
		name(nsDAV, "resourcetype"):                 el(nsDAV, "resourcetype", el(nsDAV, "principal")),
		name(nsDAV, "displayname"):                  text(nsDAV, "displayname", display),
		name(nsDAV, "current-user-principal"):       el(nsDAV, "current-user-principal", href(principalPath(account))),
		name(nsDAV, "principal-URL"):                el(nsDAV, "principal-URL", href(principalPath(account))),
		name(nsCalDAV, "calendar-home-set"):         el(nsCalDAV, "calendar-home-set", href(homePath(account))),
		name(nsCalDAV, "calendar-user-address-set"): el(nsCalDAV, "calendar-user-address-set", href(principalPath(account))),
		name(nsDAV, "supported-report-set"):         supportedReports(),
	}
}

func (c *Controller) homeProps(actor appshared.ActorContext) map[xml.Name]element {
	account := actor.AccountID.String()
	return map[xml.Name]element{
		name(nsDAV, "resourcetype"):           el(nsDAV, "resourcetype", el(nsDAV, "collection")),
		name(nsDAV, "displayname"):            text(nsDAV, "displayname", "Calendars"),
		name(nsDAV, "current-user-principal"): el(nsDAV, "current-user-principal", href(principalPath(account))),
		name(nsDAV, "owner"):                  el(nsDAV, "owner", href(principalPath(account))),
	}
}

func supportedReports() element {
	report := func(local string) element {
		return el(nsDAV, "supported-report", el(nsDAV, "report", el(nsCalDAV, local)))
	}
	return el(nsDAV, "supported-report-set", report("calendar-query"), report("calendar-multiget"))
}

/* ── A calendar and its members ────────────────────────────────────────────────────────── */

type member struct {
	id   string
	path string
	etag string
	todo Todo
	body []byte
}

func (m member) props(withData bool) map[xml.Name]element {
	have := map[xml.Name]element{
		name(nsDAV, "resourcetype"):     el(nsDAV, "resourcetype"),
		name(nsDAV, "getetag"):          text(nsDAV, "getetag", m.etag),
		name(nsDAV, "getcontenttype"):   text(nsDAV, "getcontenttype", "text/calendar; charset=utf-8; component=VTODO"),
		name(nsDAV, "getcontentlength"): text(nsDAV, "getcontentlength", strconv.Itoa(len(m.body))),
		name(nsDAV, "getlastmodified"):  text(nsDAV, "getlastmodified", m.todo.LastModified.UTC().Format(http.TimeFormat)),
	}
	if withData {
		have[name(nsCalDAV, "calendar-data")] = text(nsCalDAV, "calendar-data", string(m.body))
	}
	return have
}

type calendarView struct {
	feed    integration.CalendarFeed
	name    string
	ctag    string
	members []member
	byID    map[string]member
	props   map[xml.Name]element
}

// feed finds one of the account's own feeds by identifier; anything else is absent.
func (c *Controller) feed(ctx context.Context, actor appshared.ActorContext, id string) (integration.CalendarFeed, error) {
	feeds, err := c.Feeds.Execute(ctx, actor)
	if err != nil {
		return integration.CalendarFeed{}, err
	}
	for _, feed := range feeds {
		if feed.ID.String() == id && feed.RevokedAt.IsZero() && !feed.ViewID.IsZero() {
			return feed, nil
		}
	}
	return integration.CalendarFeed{}, shared.ErrNotFound.WithDetail("calendar.feed_not_found")
}

// calendar selects a feed's view as the actor and turns every dated entry into a member. The
// ctag is a digest over what the members are and which version each is at, so it moves when
// any entry of the view changes, arrives or leaves - which is the one thing a client polls for.
func (c *Controller) calendar(ctx context.Context, actor appshared.ActorContext, feed integration.CalendarFeed) (calendarView, error) {
	exported, err := c.Views.Select(ctx, actor, feed.ViewID)
	if err != nil {
		return calendarView{}, err
	}
	zone := zoneOr(exported.TimeZone, time.UTC)
	stamp := exported.GeneratedAt
	if stamp.IsZero() && c.Now != nil {
		stamp = c.Now()
	}
	account := actor.AccountID.String()

	// The roll-up a parent shows: how many of its children in the same result are done.
	children := map[shared.ID][2]int{}
	for _, item := range exported.Items {
		if item.ParentID.IsZero() {
			continue
		}
		count := children[item.ParentID]
		count[0]++
		if item.Completion.IsCompleted {
			count[1]++
		}
		children[item.ParentID] = count
	}

	view := calendarView{feed: feed, name: exported.View.Name, byID: map[string]member{}}
	digest := sha256.New()
	for _, item := range exported.Items {
		todo, dated := c.todoOf(item, zone, children)
		if !dated {
			continue
		}
		body := RenderTodo(todo, stamp)
		m := member{
			id:   item.ID.String(),
			path: memberPath(account, feed.ID.String(), item.ID.String()),
			etag: `"` + strconv.Itoa(item.Version) + `"`,
			todo: todo,
			body: body,
		}
		view.members = append(view.members, m)
		view.byID[m.id] = m
		fmt.Fprintf(digest, "%s:%d\n", item.ID, item.Version)
	}
	view.ctag = hex.EncodeToString(digest.Sum(nil))[:32]
	view.props = map[xml.Name]element{
		name(nsDAV, "resourcetype"):                        el(nsDAV, "resourcetype", el(nsDAV, "collection"), el(nsCalDAV, "calendar")),
		name(nsDAV, "displayname"):                         text(nsDAV, "displayname", view.name),
		name(nsDAV, "owner"):                               el(nsDAV, "owner", href(principalPath(account))),
		name(nsDAV, "current-user-principal"):              el(nsDAV, "current-user-principal", href(principalPath(account))),
		name(nsDAV, "supported-report-set"):                supportedReports(),
		name(nsCalDAV, "supported-calendar-component-set"): el(nsCalDAV, "supported-calendar-component-set", element{XMLName: name(nsCalDAV, "comp"), Attr: []xml.Attr{{Name: xml.Name{Local: "name"}, Value: "VTODO"}}}),
		name(nsCalDAV, "calendar-description"):             text(nsCalDAV, "calendar-description", view.name),
		name(nsCS, "getctag"):                              text(nsCS, "getctag", view.ctag),
		name(nsDAV, "getetag"):                             text(nsDAV, "getetag", `"`+view.ctag+`"`),
		name(nsDAV, "current-user-privilege-set"):          el(nsDAV, "current-user-privilege-set", el(nsDAV, "privilege", el(nsDAV, "read"))),
	}
	return view, nil
}

// todoOf turns one entry into a todo, and says whether it is one at all: as the feed's eventOf,
// an entry with no due date is not shown - a calendar is a set of moments.
func (c *Controller) todoOf(item workmodel.WorkItem, zone *time.Location, children map[shared.ID][2]int) (Todo, bool) {
	if item.Due == nil {
		return Todo{}, false
	}
	todo := Todo{
		UID:             item.ID.String() + "@hubtask",
		Summary:         item.Title,
		Due:             item.Due.At,
		URL:             c.itemURL(item.ID.String()),
		Created:         item.CreatedAt,
		LastModified:    item.UpdatedAt,
		PercentComplete: -1,
	}
	if item.Due.DateOnly {
		day := item.Due.At.In(zoneOr(item.Due.TimeZone, zone))
		todo.AllDay = true
		todo.Due = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	}
	if item.Completion.IsCompleted && item.Completion.CompletedAt != nil {
		todo.Completed = *item.Completion.CompletedAt
	}
	if count, ok := children[item.ID]; ok && count[0] > 0 {
		todo.PercentComplete = count[1] * 100 / count[0]
	}
	return todo, true
}

func (c *Controller) itemURL(id string) string {
	p := "/items/" + id
	if c.BaseURL == "" {
		return p
	}
	return strings.TrimRight(c.BaseURL, "/") + p
}

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

/* ── REPORT ────────────────────────────────────────────────────────────────────────────── */

func (c *Controller) report(w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, target resource) {
	if target.kind != "calendar" {
		writeStatus(w, http.StatusForbidden)
		return
	}
	body, err := readBody(r)
	if err != nil || body.XMLName.Space != nsCalDAV {
		writeStatus(w, http.StatusBadRequest)
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
	asked := reportProps(body)

	var chosen []member
	switch body.XMLName.Local {
	case "calendar-multiget":
		for _, child := range body.Children {
			if child.XMLName.Local != "href" {
				continue
			}
			id := strings.TrimSuffix(path.Base(strings.TrimSpace(child.Text)), ".ics")
			if m, ok := calendar.byID[id]; ok {
				chosen = append(chosen, m)
			} else {
				// A member the client remembers and the view no longer answers: 404 in the
				// multistatus, which is how a client learns a todo is gone.
				chosen = append(chosen, member{path: memberPath(actor.AccountID.String(), feed.ID.String(), id)})
			}
		}
	case "calendar-query":
		from, to, componentOK := queryRange(body)
		for _, m := range calendar.members {
			if componentOK && inRange(m.todo, from, to) {
				chosen = append(chosen, m)
			}
		}
	default:
		writeStatus(w, http.StatusForbidden)
		return
	}

	var responses []element
	for _, m := range chosen {
		if m.body == nil {
			responses = append(responses, el(nsDAV, "response", href(m.path), text(nsDAV, "status", "HTTP/1.1 404 Not Found")))
			continue
		}
		responses = append(responses, response(m.path, asked, m.props(true)))
	}
	writeMultistatus(w, responses)
}

// reportProps reads the properties a REPORT asks for; calendar-data is what a report is for and
// is answered when nothing was asked.
func reportProps(body element) []xml.Name {
	if prop, ok := body.find(nsDAV, "prop"); ok && len(prop.Children) > 0 {
		names := make([]xml.Name, 0, len(prop.Children))
		for _, child := range prop.Children {
			names = append(names, child.XMLName)
		}
		return names
	}
	return []xml.Name{name(nsDAV, "getetag"), name(nsCalDAV, "calendar-data")}
}

// queryRange reads the filter of a calendar-query: whether it asks for VTODO at all, and the
// time range applied to DUE where one is given. A filter naming another component answers
// nothing, because there is nothing else here.
func queryRange(body element) (from, to time.Time, ok bool) {
	filter, found := body.find(nsCalDAV, "filter")
	if !found {
		return time.Time{}, time.Time{}, true
	}
	outer, found := filter.find(nsCalDAV, "comp-filter")
	if !found || attr(outer, "name") != "VCALENDAR" {
		return time.Time{}, time.Time{}, false
	}
	inner, found := outer.find(nsCalDAV, "comp-filter")
	if !found {
		return time.Time{}, time.Time{}, true
	}
	if attr(inner, "name") != "VTODO" {
		return time.Time{}, time.Time{}, false
	}
	if span, found := inner.find(nsCalDAV, "time-range"); found {
		from, _ = time.Parse("20060102T150405Z", attr(span, "start"))
		to, _ = time.Parse("20060102T150405Z", attr(span, "end"))
	}
	return from, to, true
}

func attr(e element, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// inRange applies RFC 4791 §9.9's rule for a VTODO with a DUE and no DTSTART: it overlaps the
// range when its due moment is inside it. An undated range side is open.
func inRange(todo Todo, from, to time.Time) bool {
	if from.IsZero() && to.IsZero() {
		return true
	}
	due := todo.Due
	if todo.AllDay {
		// The day covers its whole 24 hours.
		if !to.IsZero() && !due.Before(to) {
			return false
		}
		return from.IsZero() || due.Add(24*time.Hour).After(from)
	}
	if !from.IsZero() && due.Before(from) {
		return false
	}
	return to.IsZero() || due.Before(to)
}

/* ── GET ───────────────────────────────────────────────────────────────────────────────── */

func (c *Controller) get(w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, target resource) {
	if target.kind != "member" {
		writeStatus(w, http.StatusMethodNotAllowed)
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
	m, ok := calendar.byID[target.member]
	if !ok {
		writeStatus(w, http.StatusNotFound)
		return
	}
	w.Header().Set("ETag", m.etag)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8; component=VTODO")
	w.Header().Set("Content-Length", strconv.Itoa(len(m.body)))
	if r.Header.Get("If-None-Match") == m.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(m.body)
	}
}

// refuse maps the application's answer onto a WebDAV status. A calendar client reads the
// status and nothing else, so the problem document the API would write says nothing to it;
// the four statuses below are the whole vocabulary.
func (c *Controller) refuse(w http.ResponseWriter, err error) {
	var typed *shared.Error
	if errors.As(err, &typed) {
		switch typed.Category {
		case shared.CategoryNotFound, shared.CategoryGone:
			writeStatus(w, http.StatusNotFound)
			return
		case shared.CategoryForbidden:
			writeStatus(w, http.StatusForbidden)
			return
		case shared.CategoryUnauthenticated:
			writeStatus(w, http.StatusUnauthorized)
			return
		case shared.CategoryValidation:
			writeStatus(w, http.StatusBadRequest)
			return
		}
	}
	writeStatus(w, http.StatusInternalServerError)
}
