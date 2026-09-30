---
id: UC-INS-08
title: Suspend, resume and delete a workspace
context: admin
actors: [PE-operator, PE-platform]
deployments: [D5, D6, D7]
serves: [P-03, P-04, P-15]
state: partial
tasks: [H-06, SI-17]
checked_by: [core/application/service/admin/Lifecycle_test.go, core/application/service/admin/Deletion_test.go]
---

# Suspend, resume and delete a workspace

## Goal

An operator pauses a workspace (for example on non-payment) without losing anything, resumes it at
once, and deletes one with a grace period during which the deletion can be called off.

## Story

*Suspend* asks first and names the workspace; its people are told the workspace is paused and can
still export. *Resume* brings it back as it was. *Delete* asks the operator to type the workspace's
name, says it stops answering at once and is removed for good after the grace period — and during
that period *Cancel deletion* brings it back.

## How to check

1. Suspending asks for confirmation naming the workspace; the operator's own workspace is marked
   and suspending it warns that the operator's own session ends.
2. A suspended workspace refuses work with a sentence that says it is paused, and still allows its
   export.
3. Resuming restores it exactly as it was.
4. Deleting requires typing the name and states the grace period.
5. During the grace period, *Cancel deletion* restores the workspace; after it, nothing remains.
6. Every step is in the instance journal and, where the workspace still exists, in its own trail.

## Where it ends

* No partial deletion (one hub of a customer) from the installation level.
* Why a workspace is suspended is not recorded in Hubtask ([P-15](../../vision/principles.md)).

## Today

* **Check 1 fails:** *Suspend* has no confirmation and the own workspace is not marked.
* **Check 5 fails:** there is no way to cancel a deletion, although the confirmation text promises
  one.
