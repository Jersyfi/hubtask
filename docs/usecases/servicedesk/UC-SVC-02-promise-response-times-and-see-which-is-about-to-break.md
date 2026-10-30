---
id: UC-SVC-02
title: Promise response times and see which promise is about to break
context: servicedesk
actors: [PE-admin, PE-handler, PE-requester]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Sla_test.go, core/application/service/servicedesk/Clocks_test.go, test/integration/service_desk_repository_test.go, apps/webapp/src/lib/servicedesk/desk.test.ts]
---

# Promise response times and see which promise is about to break

## Goal

A team promises how fast it answers and resolves a kind of request, and everybody - the handler and the person who asked - sees the same clock running, standing still while the team waits for an answer, and knows before a promise breaks.

## Story

Lena's policy *incidents* measures entries of the type *incident*: a first response within an hour and a resolution within ten working hours, a priority P1 within 15 minutes and four hours, at risk at half the time, standing still while an incident waits for the reporter. Mehmet raises an incident at P1 at 09:00; Jonas answers at 09:10 and the first response is met. At 11:00 the resolution is at risk and a rule pages the on-call group; Jonas resolves it at 12:30.

## How to check

1. Somebody holding STRUCTURE at the workspace defines an SLA policy under a key: the entry types it measures, a first-response and a resolution goal with targets in working minutes and per option of a priority field, the statuses a goal stands still in, the share of the target at which a goal is at risk, and a calendar - or none, counting around the clock. A goal named twice, a target under a minute or over a year, a share of 100 or a calendar the workspace does not hold are refused with their codes.
2. An entry that appears with, or takes, a type a policy covers is measured by the first enabled policy by position that covers it: each goal at the target of the entry's priority, counted on the policy's calendar. An entry no policy covers has no clock.
3. The first response is met by a comment of somebody other than the person who raised the entry; the requester's own comment meets nothing.
4. While the entry is in a status a goal stands still in, that goal's clock stands still and the time does not count; leaving the status runs it again with what was left. The completion meets every goal still running; the trash stands them still.
5. When a goal passes its share, it is marked at risk and `item.sla_at_risk` is announced once; when its time runs out, it is breached and `item.sla_breached` is announced once - a rule on either escalates. A breached goal stays breached.
6. Whoever may read the entry reads how its goals stand: target, running, standing still, met or breached, when it falls due and when it was at risk; somebody who may not read it is told it does not exist.
7. Somebody holding STRUCTURE at the workspace counts the goals armed in a period of at most a year per policy, goal and standing; the count names no entry.

## Where it ends

* One policy measures an entry; a policy changed or deleted later moves no clock already running.
* A policy takes effect for entries that appear, or take its type, after it is defined; nothing re-measures older entries.
* The goals are a first response and a resolution; next-response goals, OLAs and supplier contracts on the same entry are the next release's.
