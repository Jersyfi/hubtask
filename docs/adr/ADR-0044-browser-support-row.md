# ADR-0044 — Which browsers a Hubtask client is required to work in

* Status: **accepted**
* Date: 2026-09-04
* Deciders: the project owner
* Related: [ADR-0039](./ADR-0039-overlay-positioning.md), [ADR-0030](./ADR-0030-svelte-frontend-framework.md),
  [ADR-0028](./ADR-0028-embedded-web-ui.md), [`support-matrix.md`](../architecture/support-matrix.md),
  [`design-system.md`](../design/design-system.md) §9

## Context

`support-matrix.md` says which operating systems the server runs on, which PostgreSQL versions it
supports, and which architectures are proven by which CI job. It says **nothing about browsers**,
and a client has now been built: eighteen components of wave 1, twelve of wave 2, a board, a
hierarchy. `design-system.md` §9 has carried the gap as an open point since F1 and calls it what it
is — "a support-scope decision rather than an implementation one".

This ADR produces the evidence for that decision. It does not take it. Which browsers a product
supports is a promise to the people who use it and a cost to the people who maintain it, and
neither is a session's to choose (`CLAUDE.md`, "what you do not decide yourself").

## What the client actually depends on

Derived from the code that is there, not from a list of things a client might use. Every row names
where it is used, so that a row can be checked rather than believed.

| Feature | Where | What happens without it |
|---|---|---|
| **CSS Anchor Positioning** | `positioning.ts` | Nothing: `supportsAnchor()` tests for it and ADR-0039's own fallback positions the overlay in script. This is the one feature that is already insured. |
| **`<dialog>` + `showModal()`** | `Dialog.svelte`, `Drawer.svelte` | The modal does not open. There is no fallback: ADR-0039's siblings chose the platform dialog precisely to avoid reimplementing the top layer, the focus trap and `inert` by hand. |
| **`inert`** | `CapabilityGate.svelte`, and `<dialog>`'s own behaviour | A refused control stays reachable by keyboard — the gate looks off and is not. This is a correctness failure rather than a cosmetic one. |
| **`:has()`** | `Input`, `Select`, `Checkbox`, `Switch`, `SearchField` | The focus ring is drawn on the wrong element, or not at all. Rule 5 fails silently. |
| **`popover`** | `Tooltip.svelte`, `focus.ts` | The tooltip does not raise into the top layer and can be painted over. |
| **`clip-path: inset(50%)`** | every visually-hidden label | Announced text becomes visible. Cosmetic, but everywhere. |
| **Logical properties** (`inline-size`, `padding-inline`, `translate`) | almost every component | RTL breaks, which is a binding requirement rather than a nicety. |
| **`structuredClone`** | nowhere | — listed because it is the usual candidate and this client does not use it. |

Two things follow from the table that are worth stating before any row is chosen.

**The fallback is one feature's, not the client's.** ADR-0039 bought insurance for anchor
positioning and for nothing else. `<dialog>`, `inert` and `:has()` have no fallback and would each
need one — the first two are not small — so a row that includes an engine without them is a row
that commissions work, not merely a row that widens a promise.

**Nothing here is exotic.** Every feature above is in every current engine. The question the row
answers is not *which features may we use* but *how far back must we keep working*.

## The problem the row cannot solve on its own

`support-matrix.md` §1 defines `supported` as **"A CI job runs the software on it."**

**No CI job runs a browser.** There is no Playwright, no Puppeteer, no headless anything; the
client's gates are `build`, `lint`, `typecheck` and `node --test`, and every behavioural claim about
a component has been checked by a person driving it.

So on the matrix's own terms, **no browser can be `supported` today**, whatever is decided here. The
row can honestly say `best effort` for engines nobody has proven, and that is a true statement about
a project without a browser job — but a row of nothing but `best effort` is a row that promises
little. Turning any of it into `supported` is a second decision with a cost attached, and it is the
one this ADR most wants to put in front of its reader.

## Options

**A. The current and the previous major of each engine** (Chrome, Edge, Firefox, Safari).
The narrow row. Every feature above is available in all of them, ADR-0039's fallback becomes dead
code and can be deleted, and nothing else has to be written. It excludes Safari on an iPhone that
has stopped receiving iOS updates, which is a real population and not a hypothetical one.

**B. Anything still receiving security updates from its vendor.**
The wide row. It reaches older Safari in particular, where `:has()` (16.4) and `inert` (15.5) are
recent enough to matter. ADR-0039's fallback stays load-bearing for years, and `<dialog>` and
`inert` acquire fallbacks that do not exist — the modal ones are the expensive kind.

**C. Name a floor by feature rather than by version.**
"Any engine with `<dialog>`, `inert` and `:has()`." It is honest about what the client needs and it
ages without edits. It is also unusable as a support promise: nobody can look at their browser and
tell whether it qualifies, and a bug report cannot be triaged against it.

**D. Say `best effort` for everything and add a browser job first.**
Decide the row *after* there is a CI job that can prove any part of it, so that the matrix's own
definition of `supported` is reachable. It is the only option under which the row ever says
`supported` — and it defers the promise by however long that job takes.

## Decision

**Option A is accepted: the current and the previous major of each engine** — Chromium (Chrome,
Edge), Gecko (Firefox), WebKit (Safari).

The reasoning is the project's stage rather than the technology. A narrow promise that is true is
worth more than a wide one nobody checks, and B's extra reach is bought with work that does not
exist yet: `<dialog>`, `inert` and `:has()` have no fallback, and writing the modal ones is not a
small job for a project this size. C is honest and unusable — a support promise nobody can check
their own browser against cannot be used to triage a bug report.

**The status stays `best effort`, and that is not a hedge.** §1 of `support-matrix.md` defines
`supported` as "a CI job runs the software on it", and no browser job exists. A decides the
**scope** — which engines the client is *intended* to work in — and nothing about scope can turn
`best effort` into `supported`. Option D is what does, and it remains open as a separate decision
with a browser job attached to it; it is now the only thing standing between this row and a real
`supported`.

What is deliberately not claimed: that the client has been checked in Firefox or Safari. It has
been driven, by a person, in one Chromium build. The row says where defects will be fixed, not
where they have been looked for.

## Consequences

**For ADR-0039.** Its fallback's lifetime is what this row decides, and it said so when it was
accepted: under A the fallback is dead code and can be deleted. Every engine in the row above has
CSS Anchor Positioning, so `positioning.ts`'s script path is now unreachable by any browser this
project promises to work in.

It is **not deleted here**, and the reason is the paragraph above rather than caution: the status is
`best effort`, so nothing proves the row. Seventy-nine lines that keep an overlay in the right place
in an engine outside the row are cheap insurance while no job checks any engine inside it. Deleting
them is its own change with its own test to remove, and it is worth doing **once a browser job
exists** — that is, together with option D. Recorded as an issue so it is a decision somebody takes
rather than a thing that quietly never happens.

**For F2-12.** Dragging is the interaction with the widest spread in engine behaviour, and it is in
this milestone. It is built on Pointer Events, which every candidate engine has, and the keyboard
path is built first and works regardless — so F2-12 is not blocked by this ADR, and the row will
narrow what its pointer path has to tolerate rather than whether it can exist.

**For F5.** WCAG 2.2 AA is demonstrated for `1.0.0` rather than asserted, and a conformance claim is
made *against a set of browsers*. That set is this row. F5 cannot produce an accessibility statement
without it.

**What is still open.** Option D — a browser job, and with it a row that can honestly say
`supported`. This ADR is not superseded by that; the scope decided here is what such a job would be
written against.
