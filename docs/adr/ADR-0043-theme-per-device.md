# ADR-0043 — The theme is a property of the device, not of the account

**Status:** accepted · **Date:** 2026-09-03

## Context

Three places in the client promised an account preference for the theme that no decision had ever
made. `apps/webapp/src/lib/theme.ts` said the module follows `prefers-color-scheme` "until the
account preference exists to override it"; `apps/webapp/CLAUDE.md` repeated it; and F1-10's task
text asked for "the theme following the system preference until an account preference overrides it
(F1-08 makes that readable)".

F1-10 built the frame and found the promise had nothing behind it. `Account` in `api/openapi.yaml`
carries `locale`, `time_zone` and `week_start` and nothing about appearance, and no ADR says it
should. The frame therefore shipped with the theme following the system, and named the gap in its
pull request rather than inventing a field.

The question is not "how do we store a theme" — it is **whose property the theme is**. That
decides where it is kept, and it is a question the roadmap's binding client requirement does not
answer: it names "locale and time zone handling through the account preference" and stops there.

## The distinction that decides it

The three preferences the account already carries are properties of the **person**. Somebody's
language, their time zone and the day their week starts are the same on every device they use, and
a difference between two of their devices would be a defect — which is exactly why
`i18n-l10n.md` §2 resolves them through the account and why F1-07 made the account's locale outrank
the browser's.

The theme is the one that is legitimately different **per device**. The same person wants dark on a
phone at night and light on a monitor in a bright office; an e-ink screen wants neither. That is
why every operating system exposes it per device, why `prefers-color-scheme` exists at all, and why
"follow the system" is the correct default rather than a placeholder for something better.

## Options

**A. Add it to the contract (rejected).** A field on `Account` and `AccountPreferences` —
`appearance: null | LIGHT | DARK`, with `null` meaning "follow the device". It needs no new
endpoint: `PATCH /accounts/{accountId}/preferences` already carries preferences.

Rejected because of what it costs *permanently* against what it buys *today*. It buys one thing:
the choice follows the person to a new device. It costs a field in a published contract
(`api-guidelines.md` §6 makes a code and a field part of the contract, and
`versioning-release.md` does not walk them back), a forward-only migration, a row in the data
catalogue with a deletion path, a merge rule for offline synchronisation (`offline-sync.md` §4),
and a use-case surface to validate it. Five permanent obligations for a preference whose correct
value is a property of the screen in front of the person, not of the person.

And it is the wrong shape for the one case that matters: somebody who sets `DARK` on their account
because their phone is dark then gets dark on the bright monitor at work. The account-level answer
has to be `null` — "follow the device" — for most people, which is what the client already does
without a field.

**B. Keep it on the device, and say so (chosen).** The theme follows `prefers-color-scheme`, and a
person's own override is kept where the choice applies: on that device. Nothing enters the
contract, nothing is migrated, and the field stays available as an additive change if a real need
for it appears.

**C. Both — a contract field as the default, overridden per device (rejected for now).** This is
what the large products do, and it is defensible. It is rejected as a *starting* point rather than
on principle: it is option A's five obligations plus option B's implementation, taken on before
anybody has asked for the syncing half. If B turns out to be insufficient, C is where it goes, and
nothing decided here has to be undone to get there.

## Decision

**Option B.** The theme is a property of the device. `apps/webapp/src/lib/theme.ts` stays the one
place that sets `data-theme`, it follows the system preference, and `Account` gains no appearance
field.

A visible switch — System / Light / Dark — needs somewhere to keep the choice for that device.
That place is the local persistence port [ADR-0033](./ADR-0033-shared-client-architecture.md)
names and defers: "added here when the sync-engine and shell work packages bring their first real
implementation, not before". So the switch lands with that port, or with the first settings screen
that has somewhere to put it. Until then "follows the system" is complete rather than half-built,
which is the honest state and not a gap.

## Consequences

* The three sentences that promised an account preference are corrected to say what is true. A
  promise in prose that no decision stands behind is how F1-10 came to look for a field that was
  never specified.
* `theme.ts` keeps its shape unchanged: the DOM seam, `applyTheme`, and `followSystemTheme`
  returning its own stop function. The stop function is what a device-level override will call,
  exactly as the account preference would have.
* `i18n-l10n.md` §2's resolution order is untouched, and the asymmetry is now deliberate rather
  than accidental: language, time zone and week start resolve through the account; appearance does
  not resolve at all, because there is nothing above the device to resolve to.
* F1-10's acceptance criterion in `docs/backlog/milestone-F1.md` asked for an override that does
  not exist. It is corrected there with a note saying so, and the closed issue carries a comment
  pointing here — the same repair ADR-0025 made when it invalidated three criteria, and for the
  same reason: the next reader of a task must not be sent looking for something the product does
  not have.
* **What would reopen this:** somebody asking for their theme to follow them across devices, or a
  tenant needing to impose one (a kiosk, a workshop floor, an accessibility requirement written
  into a contract). Either is a real second use case, and either makes option C right. The field is
  additive when that day comes; nothing here breaks.
