---
id: UC-PRV-05
title: Object to an optional use of my data
context: privacy
actors: [PE-person, PE-member, PE-admin]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-11, P-14]
state: partial
tasks: [E-10, F4-20]
checked_by: [core/application/service/privacy/Consent_test.go, core/domain/model/privacy/Consent_test.go]
---

# Object to an optional use of my data

## Goal

A person who objects to an optional use of their data — one the core features do not need — records
that objection once, and from then on that use stops for them while everything else keeps working.
The record of what was allowed when is kept.

## Story

A member objects to their activity being used for an optional purpose the workspace has switched
on. They withdraw their consent to that purpose; the withdrawal is recorded with its moment and is
not erased, because what was lawful when is what somebody will ask about later. From then on that
processing leaves them out. Their tasks, reminders and sharing work exactly as before. An
administrator who receives an objection by mail records the withdrawal for the person and closes
the objection case.

## How to check

1. A person withdraws their own consent for a named purpose; a withdrawal without a purpose is
   refused with `privacy.purpose_required`, one for a purpose never consented to with
   `privacy.consent_not_found`.
2. An administrator can record a withdrawal for another person; a plain member cannot.
3. The withdrawal is kept with its moment, and `dsr.consent_withdrawn` is written.
4. A person can withdraw their own consent in the web app, not only through the API.
5. After a withdrawal, the processing it names leaves that person out.
6. The core features — capturing, planning, sharing, reminders — work the same after a withdrawal.

## Where it ends

* Hubtask does not ask for consent by default, and no plan or operator gives it for anybody
  ([NG-ai-consent-by-default](../../vision/non-goals.md)).
* Withdrawing consent is not erasure: what was processed before stays lawful and is not undone.
* Whether sending content to an AI model is among the purposes a single person can object to is
  **not decided**: [data-protection.md](../../architecture/data-protection.md) §4 lists AI under
  objection, while UC-AI-03 decides AI consent per workspace and not per person. The owner decides
  which holds.

## Today

* **Check 4 fails.** The only withdrawal form is on the administrator's *Data subject requests*
  screen (`apps/webapp/src/views/PrivacyView.svelte`); a person has no place of their own for it.
* **Check 5 fails.** Nothing reads a consent record: no processing asks whether a person withdrew,
  so a withdrawal is recorded and changes nothing. There is also no use case that grants consent —
  only one that withdraws it (`core/application/service/privacy/Consent.go`).
