---
id: UC-PRV-07
title: Correct what the workspace holds about a person
context: privacy
actors: [PE-admin, PE-owner, PE-member]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-11]
state: built
tasks: [E-10, F4-20]
checked_by: [core/application/service/privacy/Requests_test.go]
---

# Correct what the workspace holds about a person

## Goal

A person who says something recorded about them is wrong gets it corrected, and the workspace can
show when the request came in, what was done and when it was answered.

## Story

A member points out that their display name and address are misspelt. The administrator records a
rectification case, corrects the details through the ordinary screens — or asks the member to do it
themselves — and completes the case. The correction appears in the trail as the ordinary change it
is; the case records that the request was answered in time.

## How to check

1. A rectification case can be recorded, started and completed like any other case; starting it
   runs no job and changes no data by itself.
2. The correction itself is an ordinary edit, and it appears in the trail as that edit, with no
   old or new value of a sensitive field in clear.
3. An *objection* case behaves the same way: it runs no job and is completed by hand once the
   objection has been acted on.
4. Completing the case writes `dsr.completed`.

## Where it ends

* No special editing tool for rectification: the ordinary screens and API are the tool.
* Hubtask does not decide whether the correction is justified; the controller does, and may reject
  the case with a reason.
