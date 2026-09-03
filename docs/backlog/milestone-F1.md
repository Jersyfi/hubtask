# Milestone F1 — Foundations

The goal: the client stops being a scaffold. A workbench exists and the first eighteen components
live in it, drawn from tokens whose contrast is measured rather than asserted; the application
frame renders a message code in the reader's language, turns a problem document into a sentence,
and configures itself from `/meta/capabilities` instead of from constants; a token signs somebody
in and the app can say who they are and which locale to speak to them in; every request leaves
through one seam that F6 will fill with a queue; and `hubtask.eu` carries a page that says what
this is going to be.

F1 is the first milestone of the client track (`roadmap.md` phase 5). It opens with `0.4.0` and
builds the surface for what the core shipped up to `0.3.0` — none of `0.4.0`'s time features are
in it, by the track's own rule that the client runs one milestone window behind the core.

**F1 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. The
client's maturity through this milestone is `experimental`: screens appear, move and disappear
without notice, and F1-10 makes the application say so itself.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone, per the roadmap's client track: hubs, collections
and the item hierarchy, ordering, labels and the query surface (`F2`); the domain components of
wave 3 and every surface for comments, assignment, attachments, due dates, reminders, recurrence
and saved views (`F3`, after `0.4.0` has shipped); administration in any form (`F4`); AI,
accessibility conformance and the RTL audit as a body of work (`F5` — the individual rules still
apply to every component written here); offline behaviour, the Tauri shells, the celebration kit
and the onboarding tour (`F6`); and the website's **content**, which the roadmap's website lane
keeps blocked on a brief that does not exist yet.

Six decisions taken while writing this backlog, so that nobody re-derives them later:

* **There is no sign-in to build.** `api/openapi.yaml` declares exactly one security scheme —
  `bearerAuth`, "An OIDC access token, `hbt_pat_…`, or a service account token" — and no login
  route, no session endpoint and no OIDC redirect flow. Session management and the OIDC connection
  are `0.6.0`. F1 therefore gives the application what `hubctl` already has: a personal access
  token entered once, held behind the platform seam, sent as `Authorization: Bearer`, cleared on
  sign-out (F1-11). The code that a later milestone replaces says so, naming the milestone, the way
  the core's own temporary refusals do.
* **The client cannot say who it is, and that is a gap in the contract rather than in the client.**
  There is no `GET /accounts/me` and no way to read an `Account` at all: `/accounts:invite` creates
  one and `/accounts/{accountId}/preferences` needs an id the client has no way to learn. But
  "locale and time zone handling through the account preference" is a **binding** client
  requirement (`roadmap.md` phase 5), and the `Account` schema already carries `locale`,
  `time_zone` and `week_start`. So F1 opens with a core task, F1-08 — additive, spec first, and the
  first instance of the roadmap's own rule for a requirement that touches both sides.
* **The first HTTP call in the client tree belongs to the sync engine.**
  `packages/api-client/src/index.ts` says it in its own doc comment: "There is deliberately no
  runtime client yet. The fetch layer belongs to the sync engine's Transport port (ADR-0033) and
  arrives with that work package." F1-09 is therefore not optional plumbing — it is what every
  later task calls, and it is deliberately cut down to a Transport with no queue, no HLC and no
  local store, because those implement a protocol that ships in `0.8.5`.
* **The message catalogue must not become two.** `locales/en.json` is the one source
  (`i18n-l10n.md` §3) and the Go binary embeds it for the rendering the server does itself. The
  client renders the same codes from the same file; how that file crosses into the workspace is
  F1-07's decision to record, and a copy under `apps/` is not an answer.
* **`/meta/health` is admin-only.** The deep report requires `admin:read`
  (`observability-reliability.md` §5). `design-system.md` §4's `HealthBanner` is therefore built
  and fed where the actor may read it; an ordinary member learns of a degradation when an operation
  degrades. Inventing a second, unauthenticated health surface is not this milestone's business.
* **Contrast is measured before eighteen components inherit it.** `design-system.md` §9 records
  that the label tokens "are calculated for ≥ 4.5:1 but have not been measured". F1-02 comes before
  the waves for that reason: a token pair that fails is cheap to change while nothing uses it.

One thing that is **not** a task: the **wordmark**. §9 calls the placeholder in
`reference/foundations.html` "not a finished mark", and a brand mark is design work with an owner,
not a session's output. It is named as a blocker of the website's content wave; nothing in F1
depends on it.

---

## F1-01 — The component workbench **[L]**

*Depends on: nothing. One of five independent starts.*

ADR-0029 foresaw `reference/foundations.html` being
replaced by a workbench, and ADR-0030 left the
choice to "the component-layer work, not this ADR". This is that work.

What the workbench must carry follows from the rules it makes checkable: every component in
isolation, every variant, every state, both themes, an RTL toggle and a visible keyboard focus —
because `design-system.md` rules 3, 5 and 6 are rules one verifies by looking. Whether that is
Storybook, Histoire, or a small Vite page inside the package is a supply-chain decision under
CLAUDE.md, proposed with reasoning before anything is installed; the weight is real, because a
repository that has kept its dependency count deliberately small does not acquire a thousand-package
tree by habit.

The workbench is a development tool. It is not built into `apps/webapp`, it never reaches the
embedded bundle, and it must not make `pnpm -r build` depend on it.

**Acceptance:** the workbench runs from `packages/design-system` and shows a placeholder component
in both themes and in RTL, with focus visible; the tool decision and its alternatives are recorded
in the pull request; `pnpm -r build`, `lint`, `typecheck` and `test` stay green; the webapp's built
bundle does not grow by a byte; `go build ./...` and `go test ./...` still succeed with no Node.js
in `PATH`.

**Read:** ADR-0029; ADR-0030; `design-system.md` §1, §4, §5, §6;
`packages/design-system/src/README.md`

---

## F1-02 — Contrast measured, not asserted **[G]**

*Depends on: nothing.*

`design-system.md` §9, in its own words: the label tokens "are calculated for ≥ 4.5:1 but have not
been measured. This belongs in CI as an automated test, not in a one-off check." The test computes
the WCAG 2.2 contrast ratio for every pair the tokens declare as text on a surface — the ten label
pairs, the accent pairs, body and muted text on every surface — in **both** themes, and fails below
4.5:1 for body text and 3:1 for large text and non-text indicators.

It lives in `packages/design-system/test/` beside the existing token test, so the `Workspace` job in
`ci.yml` already runs it. Two things it must not do: read a colour from anywhere but
`tokens/tokens.json` (rule 15), and skip a pair it does not recognise — an unknown role is a
failure, or the check quietly shrinks as the token set grows.

**Acceptance:** `pnpm --filter @hubtask/design-system test` fails on a deliberately darkened token
and passes on the current set, and the failure has been seen once before the check is trusted (the
`gate-selftest` habit); every measured pair appears in the output with its ratio; a pair that fails
today is corrected in `tokens.json` in the same pull request and the correction is named in the
body; §9's contrast line is struck.

**Read:** `design-system.md` §1, §3, §9; ADR-0029; WCAG 2.2 SC 1.4.3 and 1.4.11

---

## F1-03 — Iconography **[L]**

*Depends on: F1-01 — an icon set with nowhere to look at it is a folder of files.*

`design-system.md` §9: a 24 px grid, a 1.5 px stroke, a base of Lucide or Phosphor (both MIT), plus
roughly fifteen of our own for Hub, Collection, Task, Work Package, Activity, Jumble, Bucket and
Capability. Two decisions in one task. **Which base set**, where how it is consumed matters more
than which one is chosen: a set that ships as one bundle of a thousand glyphs is a payload in a
binary that carries the bundle byte for byte (ADR-0028), one that tree-shakes per icon is not.
**How an icon reaches a component**: a single `Icon` taking a name, or a direct import per icon —
the second keeps the bundle honest, the first keeps call sites short, and the pull request says
which and why.

The custom marks are drawn against the domain vocabulary rather than invented: the levels of the
hierarchy, the bucket, the jumble, the capability. They are SVG in the package and use
`currentColor` only — an icon takes the colour of the text it sits in, so it never names a token.

**Acceptance:** an icon renders at 24 px and at 16 px with the stroke scaled correctly and looks
right in both themes; `currentColor` is honoured; the bundle cost of the base set is measured and
stated in the pull request; the custom set covers the domain nouns above; the no-literals lint is
green; §9's iconography line is struck.

**Read:** `design-system.md` §4, §6, §9; `domain-model.md` §2 (the vocabulary the marks name);
ADR-0029; ADR-0028 (why bundle size is not cosmetic)

---

## F1-04 — Voice and tone **[G]**

*Depends on: nothing, but wave 1's copy follows it.*

§9's last documentation gap: one page of writing rules for buttons, errors and empty states, under
`docs/design/`. Not an essay on style — the rules a component author or an assistant applies
without asking. Sentence case or title case. The verb form on a button, and what it says while it
is working. Whether an error names the fix or only the fault. What an empty state says when the
emptiness is normal, and what it says when a filter caused it. The tone §8 already forbids: "no
exclamation marks doing the enthusiasm's work".

One rule the backend imposes and the page must state: every string here is a **message code** in
`locales/en.json` (ADR-0011), so a writing rule is a rule about a catalogue entry, and a component
that hard-codes a sentence has broken two rules rather than one.

**Acceptance:** the page exists, `design-system.md` §9's voice-and-tone line is struck and points at
it; ten existing codes from `locales/en.json` are checked against the rules and any that disagree
are named (fixing them is optional, hiding them is not); each rule is stated so that a reviewer can
point at it and say a label breaks it.

**Read:** `design-system.md` §7, §8, §9; `i18n-l10n.md` §1, §3; ADR-0011; `locales/en.json`

---

## F1-05 — Wave 1a: the components a form is made of **[L]**

*Depends on: F1-01 (somewhere to see them), F1-03 (IconButton), F1-04 (their copy),
F1-13 (the primitives they lay themselves out with).*

`Button`, `IconButton`, `Input`, `Textarea`, `Select`, `Checkbox`, `Radio`, `Switch` — eight of
§4's eighteen, and the half without which no screen in F2 can be built.

The shape is decided by §5 and §6 rather than by taste: PascalCase files, camelCase props with
`is`/`has` for booleans, **states as CSS states and never as variants** (a variant matrix that
contains states explodes), no `left`/`right` anywhere because RTL is a requirement and not a later
port, focus visible on every one of them, and motion confined to `opacity` and `transform`.

Every component arrives with a workbench entry and a test. A component nobody can see in every
state is not finished, and a component with no test is one the next refactor breaks silently.

**Acceptance:** all eight in the workbench, in both themes and in RTL, each fully operable from the
keyboard with a visible focus ring; no literal colour, spacing, radius or duration, proved by the
lint; component styles compile to the external stylesheet, and the webapp's CSP check stays green
once the webapp imports one; props follow §5; a disabled control always carries its reason, which
is the `CapabilityGate` principle applied before that component exists — `ErrCapabilityNotSupported`
must never become silent ignoring.

**Read:** `design-system.md` §4, §5, §6; ADR-0029; ADR-0030; ADR-0028 (the CSP the styles must fit)

---

## F1-06 — Wave 1b: overlays and feedback **[G]**

*Depends on: F1-05.*

`Tooltip`, `Menu`, `Popover`, `Dialog`, `Toast`, `Banner`, `Avatar`, `AvatarGroup`, `Badge`,
`Spinner` — the other ten, and the ones that carry the accessibility surface: focus trapped and
then **restored to the trigger** in `Dialog`, `aria-expanded`/`aria-controls` and roving focus in
`Menu`, `Escape` closing the topmost layer only, a `Toast` a screen reader announces without
stealing focus, and `prefers-reduced-motion` reducing every entrance to the colour change rule 6
fixes as the floor.

`Banner` is the one the frame needs twice: the maturity banner of ADR-0035 §2 and §4's
`HealthBanner` are both a `Banner` with content.

**Acceptance:** as F1-05, and in addition: focus returns to the trigger when a dialog closes; a menu
is fully operable from the keyboard; `Escape` closes one layer at a time with two open; reduced
motion is honoured and tested; wave 1 is complete at eighteen components and §4's wave 1 line can be
ticked.

**Read:** as F1-05, plus `design-system.md` §6 (rules 5 and 6), §7 (the motion floor)

---

## F1-07 — Message codes rendered in the client **[L]**

*Depends on: nothing technically; the frame and everything after it consume it.*

The ground rule of `i18n-l10n.md` §1 — the server delivers no display text, only a code and
parameters — has never had a client to render it. This task builds one: an ICU MessageFormat
renderer, the resolution order of §2 (request → account → tenant → installation), CLDR plural
categories, and §3's fallback — a missing translation falls back to the source language, **never**
to a key and never to an empty string.

Two decisions belong here and are recorded in the pull request.

* **Where the catalogue comes from.** `locales/en.json` is the one source and it lives at the
  repository root because the Go binary embeds it. The client needs the same file rather than a
  copy; whether that is a package that owns the read, a build-time import or a Vite alias is this
  task's call, under the constraint that a second catalogue under `apps/` is not an answer.
* **The ICU implementation.** A dependency decision under CLAUDE.md's rule. A hand-rolled subset is
  an option only if it fails loudly on syntax it does not implement: a plural rule that silently
  renders the `other` branch is a defect nobody sees in English and everybody sees in Polish.

Also here, because it is one line and a rule rather than a project: the document's `dir` follows the
locale's direction, which `/meta/capabilities` already reports per supported locale.

**Acceptance:** a code with parameters renders in English; an unknown code renders something a
person can read rather than a blank or a key; plural and select forms are tested including a
non-English CLDR category; `errors.*` codes render as the sentences `locales/en.json` gives them;
`dir` follows the locale; no display text is written in a component where a code would do, and the
catalogue exists exactly once in the repository.

**Read:** `i18n-l10n.md` §1–§3; ADR-0011; `locales/en.json`; `api-guidelines.md` §3

---

## F1-08 — The client learns who it is: `GET /accounts/me` **[L]**

*Depends on: nothing. The one core task of this milestone, and the reason the header names it.*

A binding client requirement says locale and time zone come from the **account preference**. A
client cannot honour it: there is no way to read an `Account`. `/accounts:invite` creates one,
`/accounts/{accountId}/preferences` writes to an id the client never learns, and nothing answers
"who am I". The `Account` schema already carries `locale`, `time_zone` and `week_start`, so what is
missing is one read.

Specification first (ADR-0004): `GET /accounts/me` returning `Account` for the authenticated actor,
then `make generate`, then the implementation from the inside out. It is a use case like any other,
which in this project means the full price: a descriptor in the registry, reachability through
REST, MCP **and** automation (the parity gate is not negotiable), a metric and a span (RT-12), and
a cross-tenant negative test — an account is a tenant-scoped row and the actor of another tenant
must not resolve.

Two questions the task answers out loud. **What a service account gets**: it has an `Account` row
with `kind: SERVICE_ACCOUNT`, and the honest answer is the same document rather than a refusal.
**Whether `me` is a reserved path segment** next to `/accounts/{accountId}`: it is, and the
specification says so where a reader will look, because an account id that could ever be the string
`me` would be a routing defect rather than a curiosity.

This task is additive and therefore MINOR (`versioning-release.md` §2). It renames nothing and
removes nothing, so it needs no ADR — but it is the first worked example of the roadmap's rule for a
requirement that touches both sides, and the pull request says which client task consumes it.

**Acceptance:** `GET /accounts/me` answers the authenticated actor's account with its preferences;
an unauthenticated call is `401` with `errors.unauthenticated`; a token of another tenant cannot
reach a foreign account, proved by a cross-tenant test; the parity test passes with the new
descriptor registered in all three channels; `make generate` produces no diff; the operation carries
its metric and span; `hubctl` gains nothing here — it already has its token and its own session
notion, and F1's client is the consumer.

**Read:** `domain-model.md` (the identity aggregate and its use case catalogue);
`api-guidelines.md` §2, §3; ADR-0004; ADR-0005; ADR-0010; `i18n-l10n.md` §2; the parity test in
`test/architecture/`

---

## F1-09 — The data seam: `packages/sync-engine` **[L]**

*Depends on: nothing. The start everything else calls.*

ADR-0033 §2's package, cut to what F1 can honestly
build: the three ports (`Transport`, `Storage`, `Clock`), the store schema as types only, and a
`Transport` that speaks to a real server over `fetch` — online-only, no queue, no HLC, no local
store. Those arrive in F6 with the protocol they implement (`0.8.5`), and building them now would
mean writing a client for a `:pull` that does not exist.

What must be right today is the **shape**: the engine defines the interfaces, `@hubtask/api-client`
supplies the generated types, and a thin binding in `apps/webapp` wraps the engine's subscription
API in runes, so that no component ever imports a transport. Every request carries the bearer token
the platform seam holds (F1-11), the `Idempotency-Key` header where the operation takes one, and a
deadline — a client call without a timeout is the same defect as a server one.

The task also carries the plumbing a new workspace package needs: the `Workspace` matrix entry and
the change-detection filter in `.github/workflows/ci.yml`, the allowed edge `sync-engine →
api-client` in the dependency lint W-09 built, and a nested `CLAUDE.md`.

Two things this package must never acquire, written down now because they are cheap to prevent and
expensive to remove: **no merge rule** — merging is the server's (ADR-0021), and a merge here is a
bug against that decision — and **no Svelte**, because the engine has to be exercisable headlessly
as the first-party counterpart to `hubctl sync-conformance`.

**Acceptance:** `pnpm --filter @hubtask/sync-engine test` runs headless against an in-memory fake
`Transport` and a controlled `Clock`; a call through the engine reaches a running server and returns
typed data; the dependency lint accepts `sync-engine → api-client` and still refuses
`packages/* → apps/*`; the CI `Workspace` job runs the new package, and skips it when nothing it
depends on changed; `apps/webapp` imports the engine and not `@hubtask/api-client`; no Svelte import
and no merge appears anywhere in the package.

**Read:** ADR-0033 §2–§4; ADR-0021; `offline-sync.md` §9; ADR-0027 rule 3; W-09 in
`milestone-0.3.5.md`; `.github/workflows/ci.yml` (the matrix and the filters)

---

## F1-10 — The application frame **[L]**

*Depends on: F1-06 (Banner, Spinner, Dialog), F1-07 (text), F1-09 (the seam).*

What every later view sits inside: the layout and navigation shell, the route table grown past its
single entry, the theme following the system preference, and three things the server says about
itself made visible.

> **Corrected after the fact**, the way ADR-0025 corrected the criteria it invalidated: this task
> asked for "the theme following the system preference **until an account preference overrides
> it** (F1-08 makes that readable)". There is no such preference — `Account` carries `locale`,
> `time_zone` and `week_start` and nothing about appearance — and
> [ADR-0043](../adr/ADR-0043-theme-per-device.md) decided there should not be one: the theme is a
> property of the device. What F1-08 made readable is the *language*, and that is what the frame
> made override the browser. The line above is what was built and what is right.

* **`/meta/capabilities`** is read once at start and held. Item types and their capability profiles,
  view layouts, query fields, supported locales with their direction, the installation's limits.
  Nothing hard-codes what the manifest answers — that is what the manifest is for, and F2's query
  surface is built from `query_fields` rather than from a list somebody typed.
* **A problem document becomes a sentence.** RFC 9457 with `code`, `detail_code`, `params`,
  `field_errors[]` and `request_id` (ADR-0025): the code selects the message, a field error lands on
  its field by `path`, and `request_id` is shown where a person can copy it — an internal error
  without its reference is a support thread that cannot be traced.
* **The maturity banner** of ADR-0035 §2: while the stage is not `stable` the application says so
  itself — once, dismissible for the session, never in the way. The stage is one value in one place,
  so that convergence changes it by changing that value.

`HealthBanner` is built here too, fed from `/meta/health` where the actor may read it — see the
header's decision, and do not invent a second health surface for those who may not.

**Acceptance:** a deep link survives a reload, which is what ADR-0028's `index.html` fallback exists
for; the manifest drives at least one visible thing rather than being fetched and ignored; a
validation failure shows its messages on the fields and a `500` shows its `request_id`; the maturity
banner appears, is dismissible, and reads its stage from one place; the CSP check stays green; and
nothing in the frame knows that a Tauri shell exists — every platform difference goes through
`src/lib/platform/`.

**Read:** ADR-0025; ADR-0028; ADR-0033; ADR-0035 §2; `api-guidelines.md` §3–§5;
`observability-reliability.md` §5; `apps/webapp/CLAUDE.md`

---

## F1-11 — Signing in with a token **[G]**

*Depends on: F1-08 (to know who signed in), F1-09 (to send the header), F1-10 (to live in), F1-05
(the form).*

The honest version of "sign-in and session", per the header's first decision. The application asks
for a personal access token, verifies it with `GET /accounts/me`, holds it behind the platform
seam's storage, and sends it as `Authorization: Bearer` on every request. A `401` clears it and
returns to the token screen without losing the path the person was on. Sign-out clears the token and
everything else the client holds — `offline-sync.md` §9.6's rule applies from the first day there is
anything to clear, not from the day there is a lot.

The screen says what it is rather than pretending: a token from `hubctl auth`, how to create one,
and one sentence that account sign-in arrives with the OIDC connection in `0.6.0`. The code that
milestone will replace carries a comment naming it.

The token is a secret and is treated as one: never in a URL, never in a log, never in an error
message, never in the DOM beyond the field that accepts it, and the field is a password field with
autocomplete off.

**Acceptance:** a valid token signs in and survives a reload; an invalid one shows
`errors.unauthenticated` rather than a raw status; a `401` mid-session returns to the token screen
and keeps the intended path; sign-out leaves no token and no cached data behind, proved rather than
asserted; the token appears in no log, URL or error; the screen is keyboard-operable and correct in
RTL; the temporary nature is documented in the code and in `apps/webapp/CLAUDE.md`.

**Read:** ADR-0005; `security.md` (the PAT format, its hashing and its prefix);
`offline-sync.md` §9.6; `api-guidelines.md` §3; ADR-0033 (the platform seam)

---

## F1-12 — The website: SvelteKit, a holding page, and a way to deploy it **[L]**

*Depends on: nothing. An independent start, and the milestone's only public artefact.*

`apps/website` is still the plain Vite placeholder W-02 built.
ADR-0030 decided SvelteKit with `adapter-static`,
fully prerendered, for exactly this application: many mostly static pages, filesystem routing,
prerendering for search engines, and no Node at runtime. This task converts it, gives it a holding
page, and answers open point **CI-4** in `ci-cd.md` §8 — today `make website` builds into
`apps/website/dist` and nothing publishes it.

Where it is served from is proposed in the pull request rather than assumed here. GitHub Pages is
the obvious candidate — it is inside the platform ADR-0022
already chose, it costs nothing and it serves static files — but `hubtask.eu` is a domain its owner
points, so the task delivers the workflow and the hosting half is confirmed rather than decided in
passing.

**The content is not this task.** The roadmap's website lane names what waits on a brief:
positioning, page structure, visual direction, what may be promised about dates, editions and
price, and whether anything collects an address. The holding page carries only what is already true
and already checked — what the product is, that it is **source-available** under BSL 1.1 with a
Change Date and not "open source" (`apps/website/CLAUDE.md` is explicit, and it is a legal statement
rather than a wording preference), and that it is self-hostable. No dates, no editions, no signup: a
signup is personal data with a legal basis and a deletion path, which is a decision and not a form.

**Acceptance:** `pnpm --filter @hubtask/website build` produces prerendered static files and starts
no server; the built output contains no inline script or style — SvelteKit is configured to that
end and the check proves it rather than the configuration being trusted; every value comes from the
design system and the lint proves it; the deploy workflow publishes the built directory from `main`
and has run once; CI-4 is closed, or the half that remains with the domain owner is named in the
open point; no API call, no `@hubtask/api-client`, no account.

**Read:** ADR-0030 (the website half); ADR-0028; ADR-0027; ADR-0013 and `LICENSE` (what may be
claimed); `apps/website/CLAUDE.md`; `roadmap.md` phase 5, the website lane; `ci-cd.md` §8

---

## F1-13 — Wave 0: the primitives, and a scale to stack on **[L]**

*Depends on: F1-01 (somewhere to see them). Runs **before** F1-05 — the number is an identifier,
not a position.*

Two holes found while building the workbench, both cheap now and expensive after fifty components.

**The layout primitives.** `design-system.md` §4 used to begin at `Button`, but §0's rule means no
component may write a bare spacing value and `lint-no-literals` enforces it inside
`packages/design-system/src/`. Every component that lays anything out therefore needs the space
scale reachable through something. Without `Box`, `Stack`, `Inline` and `VisuallyHidden`, each of
the fifty writes its own flex with an exemption comment — and an exemption that appears fifty times
is not an exemption, it is the rule the lint was meant to prevent.

They carry spacing, direction and alignment and **no visual style of their own**: no colour, no
border, no shadow. A primitive that decorates is a component, and belongs in a wave that plans it.

**The layering scale.** `tokens.json` has no `z` step. F1-06 lands `Tooltip`, `Menu`, `Popover`,
`Dialog` and `Toast` together and its acceptance already says "`Escape` closes one layer at a time
with two open" — which presupposes an order that does not exist. Tokens first (rule 15), then one
place that knows which layer is on top; five components each picking a number is the failure mode
this milestone can still avoid.

One decision belongs here rather than to whoever writes `Tooltip` first: **how an overlay is
positioned against its anchor.** CSS Anchor Positioning, checked against `support-matrix.md`, or a
library — which is a supplier and therefore CLAUDE.md's rule rather than a component author's call.
The pull request says which and why.

**Acceptance:** the four primitives exist with stories and tests, take their spacing from the
tokens and declare no colour, border or shadow of their own; the `z` scale is in `tokens.json` and
reaches the generated CSS; one register decides which layer is on top, and `Escape` closing the
topmost is testable against it before any overlay exists; the positioning decision is recorded with
its alternatives; `design-system.md` §4's wave 0 is ticked and §9's layering and positioning lines
are struck; the no-literals lint and `check-stories` are green.

**Read:** `design-system.md` §0, §4 (wave 0), §5, §6, §9; ADR-0029; ADR-0037;
`support-matrix.md` (what a browser here may be required to have)

---

## The order at a glance

```
F1-01 ── F1-03 ──┐
F1-01 ── F1-13 ──┤
F1-04 ───────────┴── F1-05 ── F1-06 ─┐
F1-02                                │
F1-07 ───────────────────────────────┼── F1-10 ── F1-11
F1-09 ───────────────────────────────┘           │
F1-08 ───────────────────────────────────────────┘
F1-12
```

Five tasks depend on nothing and can start at once: **F1-01** (the workbench, which F1-03 and the
waves need), **F1-02** (contrast, which should land before the waves inherit a token), **F1-07**
(text), **F1-08** (the core read), **F1-09** (the seam) — and **F1-12**, which shares nothing with
the application at all. F1-11 is last: it consumes the frame, the seam and the new endpoint.

**Definition of Done for the milestone:** the design system has a workbench, four layout
primitives and a layering scale, eighteen components
that are keyboard-operable in both themes and in RTL, an icon set, a page of writing rules, and
contrast that is measured in CI rather than asserted; the web application renders message codes and
problem documents in the reader's language, configures itself from `/meta/capabilities`, states its
own maturity, and reaches a server only through the sync engine's `Transport`; a person signs in
with a token, and the client knows who they are and which locale, time zone and week start to speak
to them in; `hubtask.eu` is a prerendered SvelteKit site with a holding page and a workflow that
publishes it; every value still comes from `tokens.json`, no bundle carries an inline script or
style, `go build ./...` and `go test ./...` still succeed with no Node.js installed, and the one new
API operation is reachable through REST, MCP and automation with a cross-tenant test to its name.
