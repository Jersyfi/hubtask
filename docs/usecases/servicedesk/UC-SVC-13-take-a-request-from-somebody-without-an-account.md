---
id: UC-SVC-13
title: Take a request from somebody without an account through a public form
context: servicedesk
actors: [PE-requester, PE-handler, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/work/Form_test.go, core/application/service/work/FormGate_test.go, core/domain/model/integration/FormLinkToken_test.go, core/domain/model/servicedesk/PublicForm_test.go, core/application/service/servicedesk/PublicForms_test.go, presentation/rest/PublicFormController_test.go, test/integration/public_form_test.go, apps/webapp/src/lib/forms/work.test.ts, apps/webapp/src/lib/data/publicforms.test.ts, apps/webapp/src/lib/data/entryforms.test.ts]
---

# Take a request from somebody without an account through a public form

## Goal

Somebody without an account asks a team for help through a link the team hands out - on its
website, in a mail signature - and the request lands where the team works, held to the type's form,
while the workspace decides whether it takes such requests at all and nobody learns from the form
who has an account.

## Story

The workspace allows public forms. Jonas, who manages the members of the *IT* collection, opens
*Feedback* for public forms in its type's form and makes the collection's public form; the link is
copied once. Frau Kaya opens it from the school's website, writes *Beamer in Raum 12 flackert*,
gives her name and address and sends. Her browser works for a moment, the card says the request has
arrived, and the request is in *IT*, filed by Jonas for her, with *From: Frau Kaya
<kaya@example.org>* above her words. When the workspace switches public forms off, the link answers
that there is no form here.

## How to check

1. Who manages a collection's members makes its public form where the workspace allows public forms
   (`servicedesk.public_forms`, off by default; refused `public_forms.off` otherwise) and is shown
   its link once; a new link replaces the old, which answers nothing from then on, and taking it
   away does the same. The link is found in its own workspace only, and only a keyed hash of it is
   kept.
2. The form offers the top-level types of the collection whose form names the `PUBLIC` channel and
   leaves it open; a type whose form does not name it - an empty form included - refuses an entry
   arriving through `PUBLIC` with `forms.channel_closed`, and the type editor opens `PUBLIC` (and the
   portal) per type and keeps that rule when the form is saved.
3. The link answers the same "there is no form at this link" (404 `public_forms.link_not_found`)
   when it is unknown, replaced or taken away, when the workspace switches public forms off, when
   the collection is gone or archived, when the service desk is off at its hub, and when the person
   who made it is no longer active or may no longer create entries there.
4. A submission carries the challenge the form handed out, answered by the visitor's browser (a
   number whose SHA-256 with it starts with 18 zero bits) - no third-party CAPTCHA. A challenge of
   another form, a forged or unsolved one is refused `public_forms.work_invalid`, one past its ten
   minutes `public_forms.work_expired`, and one already answered `public_forms.work_spent`; a request
   refused for what it said may be corrected and sent with the same work.
5. Submissions are counted per client address and per network origin (its /24 or /64, four
   addresses' budget); past either budget the server answers 429 before anything is read. A submission is answered 202 with nothing else.
6. The request is created through `CreateWorkItem` on the `PUBLIC` door as the link's maker, held to
   the type's form there; a given name and address become the line *From: name <address>* above the
   words and a requester's record (`REQUESTER`), which an erasure by address and the requester's
   expiry reach; an anonymous request names nobody and records no requester.
7. The address is never looked up among the workspace's accounts and nothing is mailed to it on
   submission: the answer is the same whoever the address belongs to.

## Where it ends

* The team answers a requester who gave an address by mail (UC-SVC-12); a visitor has no portal of
  their own and follows the request in their mailbox only.
* An embedded form inside another site and a guest-only form are not built; the link opens the
  form on Hubtask's own card.
* The challenges answered are remembered per server process; a replica that never saw a challenge
  takes it once more, which the per-address and per-network budgets bound.
* Files are not attached through a public form.
* The panel that makes the link is shown in the portal to a workspace administrator; somebody who
  manages only the collection's members makes it through the API or hubctl.
