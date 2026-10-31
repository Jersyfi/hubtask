---
id: UC-SVC-01
title: Keep our working hours and holidays
context: servicedesk
actors: [PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/application/service/servicedesk/InstallationCalendars_test.go, infrastructure/instancefile/File_test.go, core/domain/model/servicedesk/Sla_test.go, core/domain/model/calendar/BusinessCalendar_test.go, core/application/service/servicedesk/Clocks_test.go, test/integration/service_desk_repository_test.go, apps/webapp/src/lib/servicedesk/desk.test.ts, core/application/service/servicedesk/HubCalendars_test.go, test/integration/service_desk_hub_calendar_test.go]
---

# Keep our working hours and holidays

## Goal

A workspace states once when it works - the hours of each weekday, its holidays, its time zone - so that every promise of a response time counts only the time the team is there.

## Story

Lena runs the IT desk of a district office. She defines the calendar *office*: Monday to Friday 08:00 to 18:00 in Europe/Berlin, with the public holidays of the year. The incident policy counts on it, so a ticket raised on Friday at 17:30 is due on Monday morning, not on Saturday.

## How to check

1. Somebody holding STRUCTURE at the workspace defines a calendar under a key with working spans per weekday, holidays as whole days and a time zone; an unknown zone, overlapping spans, a week without working time or more than 1,000 holidays are refused with their codes.
2. Defining a calendar again under its key replaces it; a clock already running keeps the calendar it was armed on.
3. A calendar an SLA policy names cannot be deleted (`calendars.in_use`); an unused one can.
4. Any member of the workspace reads the calendars; another workspace neither reads one nor names it in a policy.
5. Somebody holding STRUCTURE on a hub keeps the hub's own version of a workspace calendar under its key - its own hours, holidays and zone -, and every policy naming the key counts on it for the hub's entries; a key the workspace holds no calendar under is refused, a workspace calendar a hub keeps its own version of cannot be deleted (`calendars.in_use`), and dropping the hub's version counts the hub's entries on the workspace's again.
6. The operator offers business calendars in the instance file (`business_calendars`, each in the
   shape a workspace defines one); a plan or a capacity that names a key the workspace holds no
   calendar under counts on the installation's, a workspace calendar of the same key stands in
   front of it, and a malformed one or a key named twice refuses the start.

## Where it ends

* One calendar per key at the workspace, and one version of it per hub; the installation's change
  only through the instance file, and an SLA policy names a calendar the workspace holds.
* A holiday is a whole local day; half days are spans of their own weekday.
