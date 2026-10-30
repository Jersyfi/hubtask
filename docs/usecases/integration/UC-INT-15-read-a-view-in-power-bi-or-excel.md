---
id: UC-INT-15
title: Read a saved view in Power BI or Excel
context: integration
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/integration/CalendarFeed_test.go, core/application/service/work/CalendarFeed_test.go, core/application/service/work/ReadCalendarFeed_test.go, presentation/odata/OData_test.go, presentation/rest/DataFeedController_test.go, test/integration/calendar_feed_test.go]
---

# Read a saved view in Power BI or Excel

## Goal

A person who reports on the work - a team lead's dashboard in Power BI, a controller's sheet in
Excel - reads a saved view's rows in that tool directly and keeps them current, without exporting
files by hand, without a connector to install, and without the tool reading more than the person
may read.

## Story

Mara keeps a view *Open requests by due date* in the service desk's collection. In the view's
subscriptions she chooses *Read this view in Power BI or Excel* and copies the address shown once.
In Power BI she chooses *Get data → OData feed*, pastes it and builds a chart of open requests per
week; the chart refreshes every morning. When Mara leaves the service desk team, the feed narrows
with her and the chart shows what she may still read; when she revokes the feed, the refresh fails.

## How to check

1. A person who may read a saved view mints a data feed over it from the view's subscriptions;
   its address - the OData service root - is shown once.
2. The address answers an OData v4 service document and a CSDL `$metadata` naming the entity set
   `Entries`, which Power BI's and Excel's *OData feed* read without a bearer.
3. `Entries` answers the view's rows - identifier, number, level and type, title, status,
   completion, dates, assignee, collection and parent, never notes - a thousand per answer with
   `@odata.nextLink` to the rest, `$top` and `$skip` windowing them.
4. One token opens one kind: a data feed's token answers nothing on the calendar's routes or in
   CalDAV, and a calendar's nothing on the OData routes; an unknown, revoked, view-less or
   owner-less token answers the calendar feed's one `404`.
5. The feed reads as its owner at every fetch: a lost membership narrows it, a disabled account
   silences it, and revoking it stops it at once.

## Where it ends

* Read-only: nothing is written through the feed.
* One entity set of the view's entries; no `$filter`, `$select`, `$orderby` or `$expand` - the
  view's own query is the filter - and no custom fields, labels or time records yet.
* No SQL access to the database and no read-only tenant views; no Power BI connector of its own.
