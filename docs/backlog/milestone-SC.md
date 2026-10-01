# Milestone SC — Signing in and the installation, made coherent

The goal: **every person who signs in, administers a workspace or runs an installation can do what
the SI concept promised them — without a database shell, without two screens for one rule, and
without a way into an account that is weaker than the account.** SI built the mechanisms; its
review against the concept on 2026-09-30 found where they do not yet reach the person using them.

This is the first milestone cut against [use cases](../usecases/README.md). Every task names the
use cases and the checks it makes true; a task is done when those checks hold, and the pull request
says how each was confirmed. The *Today* sections of the named use cases are the findings in
detail — this file does not repeat them.

**Where the findings came from.** The owner's walk of the finished screens ("Admin → Workspace" and
"Admin → Sign-in → Second factor" set the same thing differently), and a review of the code against
the concept, the seven deployments of [`../vision/deployments.md`](../vision/deployments.md) and the
principles of [`../vision/principles.md`](../vision/principles.md). Three findings are defects that
lock people out or let the wrong person in; they are SC-01 and SC-03 and come first.

**Two decisions the owner took on 2026-09-30**, both to be confirmed by a failing test before they
are built:

* **E1 — the admission ladder is one axis, "who comes in".** `INVITED_ONLY`, `DOMAINS`, `ANY` stay as
  built after [ADR-0071](../adr/ADR-0071-provider-admission.md); the screen says them in plain words,
  and the two modes that create accounts say what newcomers get (SC-02).
* **E2 — the safeguard belongs to connecting an account, not to the provider kind.** "GENERIC may not
  be INVITED_ONLY" is reversed: an existing account with a credential is connected to a provider only
  after that credential is proven once, for every provider kind and mode; an account with no
  credential yet is connected at once (SC-01).

**Six tasks added on 2026-10-01** (SC-16 to SC-21), from the review of SC-03, SC-06 and SC-09: the
owner agreed the proposals for what those three left open or the use cases left undefined. Two of
them are decisions of their own, [ADR-0075](../adr/ADR-0075-step-up-with-what-the-account-holds.md)
(a step-up proves the account with whatever it holds) and
[ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md) (an offered provider is withdrawn with
a count, a notice and a way back in). The order: SC-19 and SC-21 first, then SC-16, SC-17, SC-18,
SC-20.

**And one design the owner asked for:** AI offered by the installation, with locks on which sources
a workspace may use — [ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md), accepted on
2026-09-30.

What deliberately is **not** in this milestone:

* **A private hub no administrator can open** ([UC-ID-16](../usecases/identity/UC-ID-16-keep-a-hub-private.md))
  and **accounts without a mail address** ([UC-ID-20](../usecases/identity/UC-ID-20-give-someone-an-account-without-an-address.md))
  — decided on 2026-09-30 as ADR-0073 and ADR-0074 and cut into
  [milestone PH](./milestone-PH.md) with the three data protection decisions.
* **Plans, platform events and usage, own domains** — their own milestones, in the order the
  concept set: passkeys, then plans, then domains.

Legend as everywhere: **[L]** local session; every task is **[L]** in the initial phase.

---

## SC-01 — Connecting a provider asks for the account's own proof **[L]**

*Depends on: nothing. First, because it closes a way into other people's accounts.*

**Use cases:** UC-ID-10 (1–7), UC-ID-11 (3, 7), UC-INS-11 (3)

Start with the test that shows the defect: a workspace administrator configures a `GENERIC`
provider set to `ANY`, a subject arrives with the owner's address and `email_verified`, and today a
session for the owner opens without the owner's second factor. Then: a first arrival that meets an
account holding a password, a second factor or another connected identity answers the card's
step machine with a new step, `LINK`, which asks for the account's password and, if armed, its
code; only then is the identity linked and the session opened. An account with no credential yet is
linked at once. With that in place `GENERIC` may be `INVITED_ONLY`, and an installation provider
without a directory claim may be offered as `INVITED_ONLY` only. Configuring, changing and removing a
provider asks for a step-up.

**Acceptance:** the failing test above passes; a table test over every mode × {no credential,
password, password+factor, other identity} × {workspace, installation provider}; the card walks the
`LINK` step at 375 px and by keyboard; ADR-0071 carries the addendum.

---

## SC-02 — Admission in plain words, and what newcomers get **[L]**

*Depends on: SC-01.*

**Use cases:** UC-ID-11 (1, 2, 4, 5, 6), UC-ID-08 (2)

The three modes as "Only people invited here", "Anyone from these organisations", "Anyone this
provider knows"; the contract's values never on screen. `identity_provider` gains what newcomers get
— nothing (default), a role, or a group — applied when the account is created on arrival, in the
same transaction. A newcomer with nothing is told that an administrator still has to give access.
The redirect address in full with a copy button.

**Acceptance:** a test per E1 mode proving who is admitted, who gets an account and what access it
has; the screen in German and English.

---

## SC-03 — The sign-in paths that lock people out **[L]**

*Depends on: nothing.*

**Use cases:** UC-ID-02 (4, 7), UC-ID-03 (1, 2, 7), UC-ID-04 (5), UC-ID-08 (4, 5)

The recovery code field takes the code as shown — sixteen letters and digits, dashes or not, pasted
— with a text keyboard. The reset card continues into the code step after a 202, with the identity
line and the remaining time. The second-factor step is titled "Second factor". The first setup uses
the code field and the one-time panel for the codes, also during sign-in, with the reason. The OIDC
return renders on the card. A provider session is held to the workspace's session rules.

**Acceptance:** Playwright walks: sign in with a recovery code; reset an account with a second
factor to the end; set up a factor during a forced sign-in. A service test that a provider session
past its idle time is refused.

---

## SC-04 — The first start, and the way back in **[L]**

*Depends on: nothing.*

**Use cases:** UC-INS-01 (1–7), UC-INS-02 (4, 5), UC-INS-03 (1–5), UC-INS-12 (5)

A fresh installation prints a one-time setup code; the web app shows *Set up Hubtask*; one step
creates the workspace, its owner and the owner's operator entry. `HUBTASK_OPERATORS` adds operators
at start. `hubtask operator add --workspace --email` in the server binary restores an operator on a
locked installation. The health report names an empty register on an installation with several
workspaces. A token whose administrative scope the register removes is refused with a sentence
naming the register; the bounded-scope check fails closed on a read error. `dev-workspace.sh
--bootstrap` and the smoke tests use the setup instead of SQL; the pseudo-workspace `operator` is
no longer needed. Migration 0108's and ADR-0070's "hubctl against the database" sentence corrected.

**Acceptance:** `compose-smoke.sh` reaches a signed-in owner without SQL; a test for each of the
three doors; the integration environment's recovery documented with the new command.

---

## SC-05 — Operators and machines **[L]**

*Depends on: SC-04.*

**Use cases:** UC-INS-04 (1, 3, 6), UC-INS-05 (1–7), UC-INS-06 (1, 2, 5), UC-ID-17 (3)

The operators list by name, address and workspace, "you" marked; adding the first entry adds the
person adding it; only active accounts; *Remove* confirms, removing oneself twice. A service account
is made operator by choosing it from a list. `hubctl token create` sends the step-up; `hubctl admin
elevate` raises a terminal session. The raised hour survives a token refresh and a reload. On a
single-workspace installation *Installation* is the last section of *Administration*.

**Acceptance:** a Playwright walk adding a colleague on a private installation without locking the
owner out; a test that a refresh keeps `elevated_until`; `hubctl` creates an administrative token.

---

## SC-06 — One rule, one place: sign-in **[L]**

*Depends on: nothing.*

**Use cases:** UC-ID-12 (1–9), UC-ID-03 (5, 6), UC-ID-11 (8)

`require_admin_totp` becomes derived and read-only on `GET /tenant`; a write is translated into a
`mfa_required_for` patch through the step-up, lock and loosening checks. Turning off one's own factor
asks the resolved rule, for every member under *Everyone*. The *Workspace* switch goes; the profile
says "Your workspace requires a second factor" instead of offering *Turn off*. *Ways to sign in* is
one list with one switch per way (`methods` finally has a control), the provider forms lose their
own switch. *Earliest change after* gets its control. Every rule shows its source and lock; choices
looser than the level above are not offered; refusals attach to the rule; a missing policy is a
sentence and a retry. The 44 German keys.

**Acceptance:** a test proving the two fields can no longer disagree in either direction; a test
that a member cannot remove a factor under *Everyone*; the screen in German.

---

## SC-07 — The installation's defaults, usable **[L]**

*Depends on: SC-06, for the shared controls.*

**Use cases:** UC-INS-09 (1–7), UC-INS-12 (2), UC-ID-18 (3)

The defaults screen uses the same controls as the workspace's sign-in screen, labelled in words,
typed by kind; the lock column worded per kind ("make it stricter", "go higher", none for language).
The workspace's legal links, language, time zone and week start show the installation's value. The
overview's operator count is right on an empty register, with a plural.

Since SC-09 the sign-in card shows the operator's accessibility statement only where one is set
(`design-system.md` §10), so this screen is where the operator sets it. The health report names a
multi-tenant installation (`D5`, `D6`) without one; a single-workspace installation never sees it.

**Acceptance:** every key of every area saved from the screen in a walk; `hubctl admin settings` and
the file refuse the same values; the health hint on a multi-tenant installation without the link.

---

## SC-08 — The workspace lifecycle keeps its promises **[L]**

*Depends on: nothing.*

**Use cases:** UC-INS-07 (4), UC-INS-08 (1, 5)

*Suspend* confirms and marks the operator's own workspace; *Cancel deletion* during the grace period
(the confirmation already promises it); *New workspace* is absent in single mode.

**Acceptance:** a test cancelling a pending deletion and finding the workspace intact.

---

## SC-09 — Show only what applies **[L]**

*Depends on: SC-03.*

**Use cases:** UC-ID-05 (5), UC-ID-06 (2), UC-ID-01 (5), UC-ID-18 (4, 5), UC-ID-02 (7)

`/accounts/me` answers `has_password`; provider-only accounts meet no password change and no
password step-up. The session list shows how each session was opened. The last used method records a
password sign-in too. No provider button before the rules name a provider. Legal links neutral in
label, absent when unset. A terminology pass: "second factor" everywhere, no field names in
`auth.mfa_code_required`, no "enrolment" or "armed" in user text.

**Acceptance:** a walk as a provider-only member; the catalogue grep for the retired words is empty.

---

## SC-10 — Terms of use, agreed and re-agreed **[L]**

*Depends on: SC-03.*

**Use cases:** UC-ID-19 (1–6), UC-ID-07 (4)

Agreement at invitation with version and time on the account; a `TERMS` step on the card when the
version changes, for password and provider sign-ins; declining opens no session.

**Acceptance:** a walk through both steps; the data catalogue row for the agreement.

---

## SC-11 — AI offered by the installation **[L]**

*Depends on: ADR-0072 (accepted 2026-09-30).*

**Use cases:** UC-AI-05 (1–7), UC-AI-06 (1–5, 7, 8), UC-AI-02 (5)

Installation AI rows, the `ai` area with `ai.sources` and `ai.min_jurisdiction`, the workspace's
source choice, the AI screen offering exactly the allowed sources, re-embedding on a model change,
`hubctl admin ai` and the instance file.

**Acceptance:** a consumer switches on the offered model with one control in a walk; a test for each
`ai.sources` value and for a narrowed rule keeping the own configuration.

---

## SC-12 — Budgets per source, and every installation secret re-sealed **[L]**

*Depends on: SC-11.*

**Use cases:** UC-AI-07 (2), UC-AI-06 (6), UC-INS-16 (1–4), UC-INS-11 (6)

`usage_record` by source; `ai_own_tokens_per_day`; the re-seal driver for the installation's own
scope covering identity-provider and AI secrets; the key census counting them.

**Acceptance:** a rotation drill in which the old key's count reaches zero with an installation
provider and an installation AI model present.

---

## SC-13 — Invitations that reach people without mail **[L]**

*Depends on: nothing.*

**Use cases:** UC-ID-14 (5, 6), UC-ID-07 (5)

*Copy invitation link* where no mail is configured, shown once; the invitation card offers the
workspace's providers; inviting an existing address says so.

**Acceptance:** a walk on an installation without SMTP inviting a family member end to end.

---

## SC-14 — The brand marks, cleared **[L]**

*Depends on: nothing.*

**Use cases:** UC-ID-09 (4)

A `THIRD-PARTY-LICENSES.md` row per shipped mark with source and guideline; the simplified Okta and
GitLab redraws replaced by the originals or by the letter tile; marks without a preset removed
([ADR-0069](../adr/ADR-0069-third-party-brand-marks.md) §2, §3).

**Acceptance:** the licences gate names every mark.

---

## SC-16 — A step-up for every account **[L]**

*Depends on: SC-09.* · [ADR-0075](../adr/ADR-0075-step-up-with-what-the-account-holds.md)

**Use cases:** UC-ID-05 (5), UC-ID-03 (6), UC-ID-12 (4)

The step-up learns `RECOVERY` (a recovery code, consumed) and `PROVIDER` (a fresh sign-in at the
connected provider with `prompt=login` and `max_age=0`; the subject must be the identity already
connected, `auth_time` must be present and fresh, otherwise `auth.step_up_provider_not_fresh`).
`stepup.Methods` answers what the account holds; the dialog offers exactly that and, for a provider,
names it and says after the return that the confirmation holds. `DisableTotp` takes the step-up
token like every privileged action; its body's `password` stays accepted for one release, marked
`deprecated`. The profile's "set up a second factor first" bridge from SC-09 goes, and *Turn off* is
offered again wherever the workspace's rule does not require the factor.

**Acceptance:** a test per method that a stolen session alone proves nothing; a test that a
different identity at the same provider is refused; a test that a provider without `auth_time` is
refused with the sentence; a walk as a provider-only administrator without a factor changing a
sign-in rule, and as a provider-only member with a factor turning it off.

---

## SC-17 — Replace my authenticator **[L]**

*Depends on: SC-16.*

**Use cases:** UC-ID-03 (4, 5, 6), UC-ID-02 (6)

*Replace authenticator* is its own action on the profile, offered whenever a factor is on — also
where the workspace's rule requires one. A step-up with any method the account holds, a new secret
with its QR code, confirmation with a code from the new app, then one atomic swap that answers ten
new recovery codes in the one-time panel; the old factor and the old codes stay valid until the
confirmation, so there is never a moment without a factor. The server keeps the replacement as a
second, unconfirmed enrolment beside the active one, with its own audit action `mfa.replaced`.
Somebody with neither the app, nor a code, nor a password, nor a provider is not let through:
`NG-weaker-recovery` stands, and an administrator removes and re-invites.

**Acceptance:** a test that the old factor signs in until the confirmation and not after; a test
that the rule "required of everyone" is never broken during a replacement; the walk under a
requiring rule.

---

## SC-18 — The code step ends honestly, and a recovery code leaves a note **[L]**

*Depends on: SC-17, for the note's link.* · issue #1100

**Use cases:** UC-ID-02 (2, 6)

Sixty seconds before the code step expires, the line under the countdown says so, also through a
live region. At `0:00` the card returns to step one with the address kept, empties both code fields
and says that the sign-in waited too long. The window itself is not extended — it is a security
bound. After a sign-in with a recovery code, the first page shows a note at the top of its content
area (not in the header: ADR-0065 decision 4 concerns statements about the application, this one is
about this person's sign-in): "You signed in with a recovery code. 7 of 10 left.", in the danger tone
at zero, linking to *Replace authenticator*. It stays in the tab across a reload until it is closed
or the authenticator is replaced — the tab, because the number arrived with this sign-in only.

**Acceptance:** a walk that lets the step expire; a walk that signs in with a recovery code and
follows the note's link.

---

## SC-19 — The session list shows only what is open **[L]**

*Depends on: SC-09.* · issue #1104

**Use cases:** UC-ID-06 (5)

`ListSessions` filters with the checks the next request makes — `Session.Verify(now)` for maximum
age and idle time, `VerifyAgainstRotation` for a required new password — so there is one definition
of "open". Server only; no job tidies expired sessions, since nothing may enumerate tenants.

**Acceptance:** one test per bound: a session past it is refused on its next request and absent
from the list.

---

## SC-20 — Withdrawing an offered provider **[L]**

*Depends on: SC-21.* · [ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md)

**Use cases:** UC-ID-12 (6), UC-INS-11 (5)

An offered provider carries the count of workspaces that switched it on, moved only by the
workspace's own switch, in the same transaction. Withdrawing asks for a date (fourteen days by
default) and shows the count; until then the provider works, the affected workspaces' sign-in screens
say when it ends, and the operator can cancel. *Withdraw now* stays behind a step-up for a
compromised provider. A workspace left with no way in after an offer ended falls back to the
password for accounts that hold one — recorded in its trail, shown to its administrators until they
switch on another way. The date is honoured where the offer is read; no scheduled job.

**Acceptance:** a test that the count moves with the switch and never names a workspace; a test
that a workspace whose only way was withdrawn signs in by password and an account without a
password does not; a walk of announce, cancel and *Withdraw now*.

---

## SC-21 — A provider is switched in the list only **[L]**

*Depends on: nothing.* · [ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md)

**Use cases:** UC-ID-12 (6), UC-ID-11 (8)

`PUT /identity-providers/{id}` refuses a change of `enabled` with `identity_provider.switch_in_list`
and accepts the same value; a new provider is created switched off; the field is `deprecated` in the
contract and goes with its next major version. hubctl and MCP follow the contract.

**Acceptance:** a test that a changed `enabled` is refused and an echoed one accepted; the contract
test.

---

## SC-15 — The walk, by use case **[L]**

*Depends on: all.*

**Use cases:** UC-ID-01, UC-ID-02, UC-ID-03, UC-ID-04, UC-ID-05, UC-ID-06, UC-ID-08, UC-ID-10, UC-ID-11, UC-ID-12, UC-INS-01, UC-INS-04, UC-INS-05, UC-INS-09, UC-INS-11, UC-AI-05

`/usecase-check` over the milestone, then a walk per deployment — `D1` fresh compose to first task;
`D2` a household without mail; `D4` a company on Entra; `D5` a consumer on an offered model — with
the evidence under `docs/evidence/`. Every use case named in this milestone moves to `built` or
`verified` with `checked_by`, or keeps its *Today* with the reason.

**Acceptance:** the milestone's use cases have no *Today* entry that names an SC task.
