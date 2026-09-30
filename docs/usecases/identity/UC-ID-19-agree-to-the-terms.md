---
id: UC-ID-19
title: Agree to the terms, and again when they change
context: identity
actors: [PE-person, PE-member, PE-operator, PE-owner]
deployments: [D4, D5, D6]
serves: [P-02, P-11, P-12]
state: specified
tasks: [SC-10]
checked_by: []
---

# Agree to the terms, and again when they change

## Goal

Where a provider or a company has terms of use, a person agrees to them before their first
sign-in, the agreement is kept with the version and the time, and a new version is put to them once
at their next sign-in — without a separate flow and without locking anybody out.

## Story

The provider publishes terms version 3. Anna, invited yesterday, sees on the invitation card "I
agree to the terms of use (version 3)" with the link, and cannot finish without ticking it. When
version 4 appears, her next sign-in shows one extra step on the card: "The terms changed on
1 November. Read and agree to continue." A person who does not agree is not signed in and is told
whom to contact.

## How to check

1. Where terms are set (installation or workspace), accepting an invitation requires ticking the
   agreement; the account stores the version and the time.
2. When the terms' version changes, the next sign-in of every person who agreed to an older version
   shows an agreement step on the sign-in card before the session opens.
3. Declining is possible and opens no session; the card says whom to contact.
4. The workspace's trail records each agreement with the version, not the text.
5. Where no terms are set, no step and no checkbox appear anywhere.
6. The step appears for provider sign-ins too.

## Where it ends

* Hubtask does not host the terms' text; the link points to wherever the provider keeps it.
* No per-clause consent, no marketing opt-ins.

## Today

* **Not built.** The concept's §5.6 and §11 describe it; only the footer link exists.
