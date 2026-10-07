# Milestone SC — Signing in and the installation, made coherent

The goal: **every person who signs in, administers a workspace or runs an installation can do what
the SI concept promised them — without a database shell, without two screens for one rule, and
without a way into an account that is weaker than the account.** SI built the mechanisms; SC makes
them reach the person using them, measured against the seven deployments of
[`../vision/deployments.md`](../vision/deployments.md) and the principles of
[`../vision/principles.md`](../vision/principles.md).

Every task names the use cases and the checks it makes true; a task is done when those checks hold,
and the pull request says how each was confirmed. The *Today* sections of the named use cases are
the findings in detail — this file does not repeat them.

What deliberately is **not** in this milestone:

* **A private hub no administrator can open** ([UC-ID-16](../usecases/identity/UC-ID-16-keep-a-hub-private.md))
  and **accounts without a mail address** ([UC-ID-20](../usecases/identity/UC-ID-20-give-someone-an-account-without-an-address.md))
  — [milestone PH](./milestone-PH.md).
* **Passkeys, plans, platform events and usage, own domains** — their own milestones, in that order.

**Delivers:** UC-AI-02 (5), UC-AI-05 (1–7), UC-AI-06 (1–8), UC-AI-07 (2), UC-AUD-01 (1–8), UC-ID-01 (5), UC-ID-02 (2, 4–7), UC-ID-03 (1, 2, 4–7), UC-ID-04 (1–8), UC-ID-05 (1–5), UC-ID-06 (2, 4, 5), UC-ID-07 (4, 5), UC-ID-08 (2, 4, 5), UC-ID-09 (4), UC-ID-10 (1–7), UC-ID-11 (1–8), UC-ID-12 (1–9), UC-ID-14 (5, 6), UC-ID-17 (3), UC-ID-18 (3–5), UC-ID-19 (1–6), UC-INS-01 (1–7), UC-INS-02 (4, 5), UC-INS-03 (1–5), UC-INS-04 (1, 3, 6), UC-INS-05 (1–7), UC-INS-06 (1, 2, 5), UC-INS-07 (4), UC-INS-08 (1, 5), UC-INS-09 (1–7), UC-INS-11 (1–6), UC-INS-12 (2, 5), UC-INS-16 (1–4)
**Released:** 2026-09-30

## Decisions

1. **E1 — the admission ladder is one axis, "who comes in".** `INVITED_ONLY`, `DOMAINS`, `ANY` stay
   as built after [ADR-0071](../adr/ADR-0071-provider-admission.md); the screen says them in plain
   words, and the two modes that create accounts say what newcomers get (SC-02).
2. **E2 — the safeguard belongs to connecting an account, not to the provider kind.** An existing
   account with a credential is connected to a provider only after that credential is proven once,
   for every provider kind and mode; an account with no credential yet is connected at once (SC-01).
   "GENERIC may not be INVITED_ONLY" no longer holds.
3. **Nobody is locked out.** No user may be locked out of the platform, and so out of their data and
   Hubtask; every door a task touches is checked against it
   ([ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md)).
4. **A step-up proves the account with whatever it holds**
   ([ADR-0075](../adr/ADR-0075-step-up-with-what-the-account-holds.md)).
5. **An offered provider is withdrawn with a count, a notice and a way back in**: the count is
   counted, not kept, and removal follows an ended offer
   ([ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md), amended by ADR-0077).
6. **AI is offered by the installation**, with locks on which sources a workspace may use
   ([ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md)).
7. **The ways back in** ([ADR-0078](../adr/ADR-0078-the-ways-back-in.md)): a provider joins an
   existing account or activates an invited one only with a second proof; the password opens as the
   fallback whenever no way in works, whatever the cause; an operator can open the password for one
   workspace for a limited time; "authoritative for the address" is read as ADR-0078 §5 says.
8. **SC-36 and SC-37 wait for the next cut.** SC-36 (#1145) and SC-37 (#1146) build what
   ADR-0078 §6 and §1 decided, and no check of the use cases they name describes it; they wait for
   the next cut rather than widening this milestone; the checks they need are asked in #1172.
9. **Providers are offered best effort** (the owner, 2026-10-03). No real identity-provider account
   is available to the project, so provider features are built and tested up to the provider, and
   the evidence says what was not walked. Not a statement for the website.

---

## SC-01 — Connecting a provider asks for the account's own proof · built

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

## SC-02 — Admission in plain words, and what newcomers get

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

## SC-03 — The sign-in paths that lock people out · built

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

## SC-04 — The first start, and the way back in

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

## SC-05 — Operators and machines

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

## SC-06 — One rule, one place: sign-in · built

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

## SC-07 — The installation's defaults, usable

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

## SC-08 — The workspace lifecycle keeps its promises

*Depends on: nothing.*

**Use cases:** UC-INS-07 (4), UC-INS-08 (1, 5)

*Suspend* confirms and marks the operator's own workspace; *Cancel deletion* during the grace period
(the confirmation already promises it); *New workspace* is absent in single mode.

**Acceptance:** a test cancelling a pending deletion and finding the workspace intact.

---

## SC-09 — Show only what applies · built

*Depends on: SC-03.*

**Use cases:** UC-ID-05 (5), UC-ID-06 (2), UC-ID-01 (5), UC-ID-18 (4, 5), UC-ID-02 (7)

`/accounts/me` answers `has_password`; provider-only accounts meet no password change and no
password step-up. The session list shows how each session was opened. The last used method records a
password sign-in too. No provider button before the rules name a provider. Legal links neutral in
label, absent when unset. A terminology pass: "second factor" everywhere, no field names in
`auth.mfa_code_required`, no "enrolment" or "armed" in user text.

**Acceptance:** a walk as a provider-only member; the catalogue grep for the retired words is empty.

---

## SC-10 — Terms of use, agreed and re-agreed

*Depends on: SC-03.*

**Use cases:** UC-ID-19 (1–6), UC-ID-07 (4)

Agreement at invitation with version and time on the account; a `TERMS` step on the card when the
version changes, for password and provider sign-ins; declining opens no session.

**Acceptance:** a walk through both steps; the data catalogue row for the agreement.

---

## SC-11 — AI offered by the installation

*Depends on: ADR-0072 (accepted 2026-09-30).*

**Use cases:** UC-AI-05 (1–7), UC-AI-06 (1–5, 7, 8), UC-AI-02 (5)

Installation AI rows, the `ai` area with `ai.sources` and `ai.min_jurisdiction`, the workspace's
source choice, the AI screen offering exactly the allowed sources, re-embedding on a model change,
`hubctl admin ai` and the instance file.

**Acceptance:** a consumer switches on the offered model with one control in a walk; a test for each
`ai.sources` value and for a narrowed rule keeping the own configuration.

---

## SC-12 — Budgets per source, and every installation secret re-sealed

*Depends on: SC-11.*

**Use cases:** UC-AI-07 (2), UC-AI-06 (6), UC-INS-16 (1–4), UC-INS-11 (6)

`usage_record` by source; `ai_own_tokens_per_day`; the re-seal driver for the installation's own
scope covering identity-provider and AI secrets; the key census counting them.

**Acceptance:** a rotation drill in which the old key's count reaches zero with an installation
provider and an installation AI model present.

---

## SC-13 — Invitations that reach people without mail

*Depends on: nothing.*

**Use cases:** UC-ID-14 (5, 6), UC-ID-07 (5)

*Copy invitation link* where no mail is configured, shown once; the invitation card offers the
workspace's providers; inviting an existing address says so.

**Acceptance:** a walk on an installation without SMTP inviting a family member end to end.

---

## SC-14 — The brand marks, cleared

*Depends on: nothing.*

**Use cases:** UC-ID-09 (4)

A `THIRD-PARTY-LICENSES.md` row per shipped mark with source and guideline; the simplified Okta and
GitLab redraws replaced by the originals or by the letter tile; marks without a preset removed
([ADR-0069](../adr/ADR-0069-third-party-brand-marks.md) §2, §3).

**Acceptance:** the licences gate names every mark.

---

## SC-16 — A step-up for every account · built

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

## SC-17 — Replace my authenticator · built

*Depends on: SC-16.*

**Use cases:** UC-ID-03 (4, 5, 6), UC-ID-02 (6)

*Replace authenticator* is its own action on the profile, offered whenever a factor is on — also
where the workspace's rule requires one. A step-up with any method the account holds, a new secret
with its QR code, confirmation with a code from the new app, then one atomic swap that answers ten
new recovery codes in the one-time panel; the old factor and the old codes stay valid until the
confirmation, so there is never a moment without a factor. The server keeps the replacement as a
second, unconfirmed enrolment beside the active one, with its own audit action `auth.mfa_replaced` (the task first said `mfa.replaced`; corrected 2026-10-03, SC-29).
Somebody with neither the app, nor a code, nor a password, nor a provider is not let through:
`NG-weaker-recovery` stands, and an administrator removes and re-invites.

**Acceptance:** a test that the old factor signs in until the confirmation and not after; a test
that the rule "required of everyone" is never broken during a replacement; the walk under a
requiring rule.

---

## SC-18 — The code step ends honestly, and a recovery code leaves a note · built

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

## SC-19 — The session list shows only what is open · built

*Depends on: SC-09.* · issue #1104

**Use cases:** UC-ID-06 (5)

`ListSessions` filters with the checks the next request makes — `Session.Verify(now)` for maximum
age and idle time, `VerifyAgainstRotation` for a required new password — so there is one definition
of "open". Server only; no job tidies expired sessions, since nothing may enumerate tenants.

**Acceptance:** one test per bound: a session past it is refused on its next request and absent
from the list.

---

## SC-20 — Withdrawing an offered provider · built

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

## SC-21 — A provider is switched in the list only · built

*Depends on: nothing.* · [ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md)

**Use cases:** UC-ID-12 (6), UC-ID-11 (8)

`PUT /identity-providers/{id}` refuses a change of `enabled` with `identity_provider.switch_in_list`
and accepts the same value; a new provider is created switched off; the field is `deprecated` in the
contract and goes with its next major version. hubctl and MCP follow the contract. Its squash commit
carries a `BREAKING CHANGE:` footer (decided 2026-10-03, SC-28).

**Acceptance:** a test that a changed `enabled` is refused and an echoed one accepted; the contract
test.

---

## SC-22 — A wrong second factor counts · built

*Depends on: SC-16.* · issue #1117

**Use cases:** UC-ID-02 (5), UC-ID-05

A wrong code at a second-factor door is meant to advance the attempt ledger (T-02), so that repeated
guesses meet the lockout curve. Every second-factor door records the failure inside the transaction
that then returns the refusal, so the record is rolled back with it: the sign-in's second step (code
and recovery code), the enrolment's confirmation, the step-up (code, password, recovery code, a
refused provider proof), the LINK step's password, and the authenticator replacement's confirmation.
Each records it the way the password door does - in its own transaction after the refusing one
returns.

**Acceptance:** an integration test per door that a wrong proof advances the ledger against the real
database (the fake unit of work never rolls back, which is how no test saw it).

---

## SC-23 — Sign out everywhere else · built

*Depends on: SC-19.* · issue #1113

**Use cases:** UC-ID-06 (4)

A server verb that ends every session of the account but the caller's, and the session list's bulk
action becomes *Sign out everywhere else*, saying it leaves this device signed in. `DELETE
/auth/sessions` (every session, this one included) stays in the contract as the API's and hubctl's
emergency door; the screen does not need it, since *Sign out* ends this one. The sentence after it is
true again ("Every other session ended").

**Acceptance:** a test that exactly the caller's session survives, through the registry; a walk of
the button.

---

## SC-24 — The password switch is honoured by the server · built

*Depends on: SC-20.* · issue #1119 · [ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) §4

**Use cases:** UC-ID-12 (6), UC-ID-04 (1)

Where a workspace's resolved methods do not include `PASSWORD`, every password door refuses: the
sign-in, the reset's request and its completion, and every other door that ends in a session through
a password. The refusal is the same for every address (no enumeration, UC-ID-04 check 1), and the
reset sends no mail that offers a closed door. Stored passwords are kept: switching the password back
on restores them. The one exception is ADR-0076 §4's fallback (`WaysIn.PasswordFallback`), which
becomes the real exception to a real refusal. Where a workspace's provider is unreachable, the way back
today is the installation's level of the rule (ADR-0068): locking the methods with the password among
them, which opens it in every workspace; a lever for one workspace does not exist (ADR-0077 §4).

**Acceptance:** a test per door that a switched-off password is refused and the fallback lets it
through; UC-ID-12 moves to `built` if every check then holds.

---

## SC-25 — An account without a password gets back in by mail · built

*Depends on: SC-24.* · issue #1122 · [ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) §3

**Use cases:** UC-ID-04 (1–7), UC-ID-12 (6)

While a workspace's fallback stands (its last way in was an offer that ended), *Forgot your password?*
mails an account that holds no password a link to **set** one, under the workspace's rules - instead of
the mail that says "use your organisation's provider", which has no provider left to point to. The
request still answers byte for byte the same for every address. Outside the fallback nothing changes.

**Acceptance:** a test that a provider-only account in a fallback workspace sets a password and signs
in, and that the same account outside the fallback gets the provider mail; the reset's checks hold.

*Widened on 2026-10-06* ([ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §4, the owner's E5): the
first-password link opens wherever the password is open and no provider the account is connected to
lets it in - under the fallback, under the operator's opening, and where the workspace keeps the
password on but the account's provider ended. A new reset link spends the earlier unspent ones, and so
does a change of the password or the address by any other way (UC-ID-04 check 2).

---

## SC-26 — An offered provider's count is counted, not kept · built

*Depends on: SC-20.* · issue #1121 · [ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) §1

**Use cases:** UC-INS-11 (5)

The number of workspaces that have an offered provider switched on is computed when it is read, by a
read-only database function that answers only the number (P-01). `offered_workspaces` and
`move_provider_offer` are dropped by a later migration (expand/contract: stop writing and reading
first, then drop); the switch stops moving anything. *Withdraw now* compares against the counted
number inside its transaction.

**Acceptance:** an integration test that a deleted, a restored and an imported workspace leave the
number true, and that a workspace reads none. The function depends on its owner bypassing row level
security, exactly as `resolve_tenant` does; an installation without that could sign nobody in either,
so it adds no failure of its own. The column and `move_provider_offer` are dropped by a migration one
release later (expand/contract).

---

## SC-27 — An offered provider is removed only after its offer ended · built

*Depends on: SC-20, SC-26.* · issue #1123 · [ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) §2

**Use cases:** UC-INS-11 (5)

`DELETE /admin/identity-providers/{id}` refuses while the offer stands and a workspace uses it
(`identity_provider.withdraw_first`); after the withdrawal's day, or with no workspace using it, it
removes. The dialog says the cost: the connections between people and the provider are deleted and
offering it again does not restore them. A workspace's own provider is unchanged (SC-06's last-way-in
guard already holds there).

**Acceptance:** a test that a used, standing offer is refused and an ended one removed; the walk.

---

## SC-28 — A deprecated field says so · built

*Depends on: SC-16, SC-21.* · issue #1124

**Use cases:** none - the contract's own promise (`api-guidelines.md`, `versioning-release.md` §5)

A field marked `deprecated` in `openapi.yaml` carries the version it goes away with. From that one
mark: an entry in `/meta/capabilities` listing the deprecated fields and their sunset, and the
`Deprecation` and `Sunset` headers (RFC 8594) on the answer to a request that used one. Today that is
`enabled` on a provider's configuration (SC-21) and `password` when the second factor is turned off
(SC-16). The squash commit of SC-21 carries a `BREAKING CHANGE:` footer, as `versioning-release.md`
asks before 1.0.

**Acceptance:** a contract test that every `deprecated` field is in the manifest with a sunset, and a
test that the headers arrive when the field is sent and not otherwise.

---

## SC-29 — The second factor's trail is one family · built

*Depends on: SC-17.* · issue #1125

**Use cases:** UC-AUD-01

The second factor's audit actions are named `auth.mfa_*`: `mfa.recovery_regenerated` becomes
`auth.mfa_recovery_regenerated` for new entries. Stored entries keep their name - the hash chain
covers the stored shape - and the trail's filters read the old name as the same action, so a search
for the family finds both. SC-17's `auth.mfa_replaced` stays as built (its task text said
`mfa.replaced`).

**Acceptance:** a test that a new regeneration is written under the new name and that filtering by it
finds an old entry too.

---

## SC-30 — The stored offer count is dropped

*Depends on: SC-26, released - and one release more.* · issue #1134 · [ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) §1

**Use cases:** UC-INS-11 (5) - unchanged; this removes what nothing reads any more.

SC-26 counts an offered provider's workspaces where the count is read and leaves the stored count in
place: expand, not yet contract. A new, forward-only migration drops the column
`identity_provider.offered_workspaces` and the function `move_provider_offer` with its grant, and
`db/schema.sql` with it. Not before the first release after the one that contains SC-26: the release
before SC-26 reads the column and calls the function, and during a rolling update it runs beside the
new one (ADR-0003). The migration's number is taken from `origin/main` right before it is written.

**Acceptance:** the column and the function are gone from a migrated database and from
`db/schema.sql`; `make generate` produces no diff; `make verify` and the integration suite are green.

---

## SC-31 — The password opens whenever no way in works · built

*Depends on: SC-24, and lands with it.* · issue #1138 · [ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §2

**Use cases:** UC-ID-12 (6), UC-ID-04, UC-INS-11 (5)

The fallback of ADR-0076 §4 answers every cause: wherever the methods a workspace's rule resolves to
leave the password out and no provider is switched on there, the password opens for every account that
holds one - an installation default or lock without the password, a rescue lock lifted, a restore or an
import, two administrators racing, a workspace provisioned under such a default. Each use is recorded
with its cause; a door reads the fallback once.

**Acceptance:** a service test per cause and for the fallback ending once a way is switched on; an
integration test under an installation default without the password, with the real resolver.

---

## SC-32 — An invited account is activated only with a second proof · built

*Depends on: SC-31; SC-24 lands with it.* · issue #1139 · [ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §1, §5

**Use cases:** UC-ID-10 (4), UC-ID-07 (5)

A provider activates or connects an invited account only if it is authoritative for the address, or
the person arrives through the invitation's own link - the invitation bound to the provider flow on the
server, single use, expiring, not spent by an arrival that fails. Under *Only people invited here* the
link admits a provider that is not authoritative, for that account only; the provider's verified
address must equal the invited one. A link made earlier without that proof activates nothing.
"Authoritative" is read as ADR-0078 §5 says (the domain-ownership claim must be `true`; Google for its
consumer domains or a matching hosted domain; any other issuer not). A refused provider sign-in is
recorded in a transaction of its own, so the refusal is not rolled back with it.

**Acceptance:** under the permissive mode an arrival without the link is refused and links nothing; with
the link it is accepted; a lapsed, spent or foreign invitation is one refusal; the authority table per
claim shape; a PostgreSQL test that a refusal's trail entry is stored.

---

## SC-33 — A workspace without the password connects its provider by mail · built

*Depends on: SC-32.* · issue #1140 · [ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §1

**Use cases:** UC-ID-04 (8), UC-ID-10 (1, 6), UC-ID-12 (6)

In a workspace that switched the password off, *Forgot your password?* mails an account that holds a
password but is connected to no provider switched on there - or holds only an identity at an offer that
ended, or holds no credential at all (no password, no factor, no identity; decided 2026-10-06, so that
SC-32's `identity_provider.link_needs_mailbox` has its way back where the password is off) - a link to
connect the workspace's provider. The link and a fresh sign-in at the provider together are the
account's proof at the LINK step; an armed second factor is still asked; no password is stored; the
provider's verified address must equal the account's. The LINK step keeps accepting the password as a
proof where the password is off as a way in. Under *Only these domains/directories* the list decides who
comes in new, not whether an existing member may connect with its own proof (decided 2026-10-06). Where
the password is off, the sign-in card offers the link as "Get a sign-in link by mail". Before the password is switched off, the switch says how
many people have never signed in through a provider.

**Acceptance:** all three cases connect and sign in; an armed factor is still asked; a mismatched
address or a sign-in that is not fresh connects nothing and leaves the link unspent; a link is spent
once; the switch's count excludes connected, invited and service accounts.

---

## SC-34 — An operator opens the password for one workspace · built

*Depends on: SC-31.* · issue #1141 · [ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §3

**Use cases:** UC-INS-11, UC-ID-12 (6)

For a provider that is switched on but broken, an operator opens the password for one named workspace -
24 hours by default, at most seven days - with the requester and the reason recorded in the workspace's
trail and the installation's journal; the workspace's administrators are notified when it opens and when
it closes. It is the fallback with the cause `OPERATOR`; it reads and changes nothing of the workspace's
content. From the installation's screen and from `hubctl`.

**Acceptance:** scope, operator register and step-up; the opening ends on its own; both trails; the
notices; it overrides the workspace's switch and a lock; it can be closed early.

---

## SC-15 — The walk, by use case

*Depends on: all.*

**Use cases:** every check in `Delivers` — the walk confirms them and adds none.

The use case checklist (`docs/usecases/README.md`, "Checking work against its use cases") over the
milestone, then a walk per deployment — `D1` fresh compose to first task;
`D2` a household without mail; `D4` a company on Entra; `D5` a consumer on an offered model — with
the evidence under `docs/evidence/`. The project has no real identity-provider account (Entra ID,
Google Workspace): the walks go up to the provider and say in their evidence that the provider
itself was not walked (Decision 9). Every use case named in this milestone moves to `built` or
`verified` with `checked_by`, or keeps its *Today* with the reason.

**Acceptance:** the milestone's use cases have no *Today* entry that names an SC task.
