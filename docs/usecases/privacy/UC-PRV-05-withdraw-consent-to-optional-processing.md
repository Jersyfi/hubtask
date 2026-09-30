---
id: UC-PRV-05
title: Object to an optional use of my data
context: privacy
actors: [PE-person, PE-member, PE-admin]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-08, P-11, P-14]
state: partial
tasks: [E-10, F4-20, PH-03]
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
7. **AI, where the workspace lets people keep their content out (the default):** the profile offers
   *Keep my content out of AI*. With it on, nothing the person authored — entries they created,
   their comments, their notes — goes into any prompt, their name appears in none, and no AI action
   is offered to them; for everybody else AI works as the workspace set it.
8. The switch appears only where the workspace has AI on **and** more than one person; alone, the
   workspace's decision is the person's own and there is nothing to switch.
9. **AI, where the workspace made it part of the work for everybody:** the workspace names the
   legal basis (employment contract, works agreement, legal obligation) when it chooses this, and
   cannot choose it without one. People see no switch but a sentence naming that basis and whom to
   ask; a withdrawal recorded earlier stops taking effect, and each person it affects is told once.
10. A formal objection under Art. 21 is still recorded as an objection case and decided by the
    controller; recording it switches nothing off by itself.
11. The installation and a plan can set the workspace's position as a default or lock it, and the
    workspace's screen says which level decided.
12. Administrators see in the consent register who keeps their content out; the list appears
    nowhere else.

## Where it ends

* Hubtask does not ask for consent by default, and no plan or operator gives it for anybody
  ([NG-ai-consent-by-default](../../vision/non-goals.md)).
* Withdrawing consent is not erasure: what was processed before stays lawful and is not undone.
* AI stays the workspace's decision (UC-AI-03); a person keeps only *their own* content out, and
  only where the workspace offers that — decided 2026-09-30
  ([data-protection.md](../../architecture/data-protection.md) §4.1).
* Hubtask does not judge whether the named legal basis holds; it records it and shows it.

## Today

* **Check 4 fails.** The only withdrawal form is on the administrator's *Data subject requests*
  screen (`apps/webapp/src/views/PrivacyView.svelte`); a person has no place of their own for it.
* **Check 5 fails.** Nothing reads a consent record: no processing asks whether a person withdrew,
  so a withdrawal is recorded and changes nothing. There is also no use case that grants consent —
  only one that withdraws it (`core/application/service/privacy/Consent.go`).
* **Checks 7–12 are not built.** Decided 2026-09-30; milestone PH, task PH-03.
