# Milestone F5 — AI, i18n, accessibility

The goal: the interface catches up with two core milestones that changed what the product *is*
without changing a screen. `0.7.0` and `0.7.5` made the server able to propose — a better
title, a tree of work under a task, the labels and the column an entry belongs in, the
duplicates it resembles, a summary of a thread — and nothing in the browser shows a proposal or
lets a person accept one. `0.8.0` made the server speak every language in `locales/` and answer
which way each one runs, and the browser still loads one catalogue, `en.json`, and offers the
account's language as a setting whose only effect is on the notifications that arrive by mail.
And the milestone that owes the product its accessibility conformance is this one: every
component has been built to the rules since F1, and no route has been walked as a whole with a
keyboard or a screen reader, which is the difference between a rule and a claim.

F5 is the fifth milestone of the client track (`roadmap.md` phase 5). It opens with `0.7.0` and
builds the surface for `0.7.0`, `0.7.5` and `0.8.0`; the contract it works against has stopped
moving, because `0.8.5` is built too and touched none of these routes.

**F5 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. The client's maturity stage is `preview` since
#381 and **stays `preview` through this milestone**: ADR-0035 §2 gives `stable` to convergence.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: **the sync engine's offline half** — the local
store, the queue, the conflict screen, `SyncStatus` and `ConflictResolver`, everything `0.8.5`
built the server for — which is `F6`'s, as is every shell; **the accessibility statement's
publication**, which `design-system.md` §10 gives to convergence and the 1.0 site — this
milestone writes the statement's content from its own walks and puts the footer link in place,
and the sentence that says *published* is `0.9.5`'s to write; **level AAA** anywhere; **an inbox
for suggestions** (`IN_APP` notifications are the product question F3-02 recorded and nobody has
answered); **the ecosystem's screens** — an import wizard, a CalDAV address, anchoring settings —
which are `0.9.0`'s surface and therefore a later window's; and **any second catalogue under
`apps/`**, which `i18n-l10n.md` §6 line 1 forbids by name: the client reads the files the binary
embeds, and this milestone widens the one lint exception from one file to the directory rather
than copying anything.

Ten decisions taken while writing this backlog, so that nobody re-derives them:

* **No core task this time, and that is worth recording.** F1, F2, F3 and F4 each found a gap
  in the contract that a screen could not work around. Cutting F5 read every route it needs
  against `api/openapi.yaml` and found none: suggestions are listed by target and status
  (`GET /suggestions`), every AI operation answers a job whose result names the suggestion, the
  provider's configuration carries the consent (`processing_allowed`), `supported_locales`
  answers the direction, the week start and the decimal separator (M-05), `text_languages` is
  the picker line 9 asks for, and `:translate` is a synchronous read. The one thing the
  interface needs and the contract does not give is a reduced-motion preference on the account
  — and that is not a gap, it is ADR-0043's rule applied: motion, like the theme, belongs to the
  device, and the switch keeps its choice where the theme's does.
* **AI is a foundation before it is a component.** `design-system.md` §9's last open gap says
  `AISuggestion` needs "its own colour, elevation, motion and tone" — a treatment, not a row. So
  F5-01 adds the `ai.*` tokens to `tokens.json` (a surface pair measured for contrast like every
  other, a border, the `attach` motion role, and nothing that glows), the voice-and-tone rule for
  a proposal (it is offered, never asserted; it names its model; it is dismissed in one gesture),
  and then the component on top of them. Switching AI off removes the tokens' only consumers,
  which is what "without residue" means in a system where a value cannot be written anywhere
  else.
* **A suggestion is rendered once, in one strip, whatever asked for it.** Fields, a summary, a
  classification, a decomposition, a translation's provenance and a template draft are all a
  `Suggestion` with a kind and a payload; the strip on an entry lists the open ones, renders each
  payload with the editor the product already has for that shape (the field controls, the tree
  from `TemplateEditor`, the label chips), and offers accept and dismiss. Asking is a menu of the
  operations the manifest says this installation serves, each answering a job the strip follows.
  Nothing AI-shaped is built twice.
* **Absence is absence, and a `CapabilityGate` is for something else.** `ai_suggestions` off in
  the manifest means the strip, the menu, the semantic toggle and the translate control are not
  rendered — not disabled with a reason. F4 decided this for the jumble's suggest control and
  the reason holds: a gate explains a capability the installation lacks to somebody who expected
  it, and an installation with AI switched off has a product that never mentioned it. What *is*
  gated with a reason is a degradation — a provider configured and unreachable — because that is
  a feature the person was shown and is now refused, and `/meta/health` names why.
* **The second catalogue is loaded, not bundled.** `locales/` is read at build time today through
  one import of `en.json`. Bundling every file would ship every language to every reader; a
  `fetch` of `/locales/<locale>.json` would need a route the server does not serve and
  `connect-src` does name. So the catalogues are Vite's lazy chunks — `import.meta.glob` over
  `locales/*.json`, one chunk per file, loaded when the resolved locale is not the source — and
  the workspace lint's exception widens from `locales/en.json` to `locales/*.json`, still the one
  directory and still the one module that reads it. A locale the manifest lists and no chunk
  exists for falls back to the source, exactly as an untranslated code does.
* **German is the interface's second language, and it is written here, by hand, whole.**
  `de.json` holds 172 messages and none of the 1,692 under `app.*`; the family `make locales`
  reports as 0 % is the one a reader sees. ADR-0055 says a translation is a pull request against
  the file, reviewed like code, and F5-08 is that pull request for the `app.*` family — in the
  voice `voice-and-tone.md` sets, with the ICU subset `catalogue.test.ts` holds it to. The other
  nine locales of the `1.0` set arrive as files when there is a translator; the mechanism is
  proved by one.
* **The RTL audit walks with a real catalogue, and its findings are fixes in the same task.** QS-08
  was walked with an operator's `ar.json` (`docs/evidence/QS-08-2026-09-15.md`); the interface
  half uses the same file, laid over the embedded catalogues in the dev server, so that every
  route is seen in Arabic and running right-to-left rather than in English with `dir="rtl"`
  forced. A physical property found is fixed where it is found — the audit is a task with
  code in it, unlike the R-08 walks, because a `margin-left` is not a product question.
* **The two accessibility walks are two tasks, and each one fixes what it finds.** The keyboard
  walk per route (F5-11) and the screen-reader pass (F5-13) are the two `design-system.md` §10
  names; they are separated because the second needs the first's fixes in place to be a pass
  rather than a list of tab traps. Both file evidence in the shape the resilience files use, both
  are repeated before `1.0.0`, and both fix in place for the RTL audit's reason — with the one
  exception of a finding that is a *product* question, which becomes an issue.
* **Reduced motion has a switch, and it lives with the theme's.** The media query is honoured
  since wave 1; what §10's 2.3.3 row adds is "the product's own preference". The switch sits in
  the profile beside the theme's, sets `data-motion` on the document in the one module that owns
  it, and keeps its choice on the device the way the theme does — which for both means the
  browser's storage until the persistence port ADR-0033 defers arrives with the shells. One
  module, one attribute, one owner.
* **QS-08 is completed for the interface by the milestone's last task, and the roadmap says so.**
  `milestone-0.8.0.md` decided that QS-08 is "walked for the server in this milestone and
  completed for the interface by `F5`". F5-14 is that completion: the operator's `ar.json` again,
  the interface switched to it by a person with no code change, every route in Arabic and
  right-to-left, and the evidence appended to the QS-08 file's own section. Then the two
  requirement lists — `i18n-l10n.md` §6's eleven lines and `design-system.md` §10's twenty rows
  — are read line by line and each one names the task that met it or the issue that still owes
  it, which is the milestone reporting into the documents it was cut from.

---

## F5-01 — The AI treatment, and `AISuggestion` **[L]**

*Depends on: nothing. The task every AI screen in this milestone stands on.*

Decision 2. Three pieces:

1. **The tokens.** `tokens.json` gains `ai.surface`, `ai.surface-strong`, `ai.border`, `ai.text`
   and `ai.accent` as a pair per mode, measured by `test/contrast.test.js` against `text.primary`
   and against each other like every other surface — a proposal has to be legible *and* visibly
   not the reader's own content, which is a contrast requirement in both directions. `make
   tokens` regenerates `LabelTokens.go`, which must show no diff because these are not label
   tokens. No new duration or easing: a proposal arriving is the `attach` motion role, because it
   arrives beside what it is about.
2. **The rule.** `voice-and-tone.md` gains §7 *A proposal*: offered, never asserted ("Suggested
   title" and not "Title"); names its model where the reader can find it and nowhere the eye
   lands first; dismissed in one gesture and accepted in one; and never counts, nudges or
   celebrates — a suggestion accepted is a field changed, and rule 7's rewarding interactions
   belong to what the person did.
3. **The component.** `AISuggestion` in the design system: a region with the treatment, a
   heading that names the kind (from a code the caller resolves), a slot for the payload rendered
   by the caller, the provenance line (model, when, prompt version) collapsed by default, accept
   and dismiss as `Button`s the caller wires, a `pending` state for a job still running and a
   `stale` state the caller sets when the target has moved since the suggestion was made
   (`suggestion.stale` is the server's refusal, and the strip says so before the server has to).
   The stories cover every kind's shape, both modes, both directions, `motion: reduced` and the
   pseudo-locale; `design-system.md` §4's wave 3 row says built and §9's last gap is closed.

**Acceptance:** the five tokens exist in `tokens.json` with both modes and pass the contrast test
in both directions; `make tokens` produces no diff in `LabelTokens.go`; §7 exists in
`voice-and-tone.md`; `AISuggestion` exists with its stories and the workbench's axes; a story
with `motion: reduced` shows no transition; `pnpm -r build lint typecheck test` green; `make
gate-docs` green.

**Read:** `design-system.md` §4 (the `AISuggestion` row), §6 rules 3 and 6, §9 (the AI gap);
`voice-and-tone.md`; `ai-first.md` §2, §3 (the guardrails: provenance, marked as suggestions);
ADR-0029; ADR-0037; `packages/design-system/test/contrast.test.js`; `CapabilityGate.svelte`
(what a gate is, so this is not one)

---

## F5-02 — Suggestions on an entry **[L]**

*Depends on: F5-01.*

Decision 3. `lib/data/suggestions.svelte.ts` reads `GET /suggestions` for a target, asks through
the six operations of an entry — `:suggest-fields`, `:summarize`, `:classify`, `:decompose`,
`:summarize-thread`, `:duplicates` — follows the job each answers through `lib/data/jobs.ts`,
and performs `:accept` and `:dismiss` with the suggestion's version. The item screen gains the
strip: the open suggestions for the entry, each an `AISuggestion` whose payload is rendered by
the shape it carries — `FIELDS` as the title, the notes and the due date beside the entry's own
with each accepted on its own where the contract allows it or as a whole where it does not (the
task reads `:accept` and records which); a classification as `LabelChip`s and a bucket name;
`DECOMPOSITION` as the tree the `TemplateEditor` already draws, read-only; `DUPLICATES` as
links to the entries with no accept, because nothing accepts it (K-04). The menu that asks
offers only the operations the manifest's `ai_suggestions` and the item's own state allow (no
thread summary for an entry without comments), and it is absent when AI is off (decision 4).
A stale suggestion — the entry's version has moved — is marked before the server refuses it.

**Acceptance:** each of the six operations can be asked from the item screen, the job is
followed, and the resulting suggestion appears in the strip without a reload; accepting a
`FIELDS` suggestion changes the entry and the strip empties; dismissing closes it; a
`DECOMPOSITION` accepted creates the children and the tree appears under the entry; `DUPLICATES`
renders links and no accept; with `ai_suggestions: false` the menu and the strip do not exist in
the DOM; a `503 ai.unavailable` is rendered through `lib/problem.ts` as the one sentence it is;
the data module has tests with a fake transport for the list, the job follow and a stale
refusal; `pnpm -r build lint typecheck test` green.

**Read:** `ai-first.md` §2 (every row but translation and templates), §3; J-05…J-08 in
`milestone-0.7.0.md`; K-01…K-05 in `milestone-0.7.5.md`; the `Suggestion`, `SuggestionKind` and
`AcceptSuggestion` schemas; `apps/webapp/src/views/ItemView.svelte`; `lib/data/jobs.ts`;
`lib/entries/TemplateEditor.svelte`

---

## F5-03 — The jumble's AI path **[L]**

*Depends on: F5-01.*

The half F4-12 left absent on purpose. `:suggest` on a jumble entry answers a job; the
suggestion it records carries a title, notes, a due date and the subtasks the material implies
(J-06, K-01), and `JumbleInboxItem` has had a slot for "optionally with an AI suggestion" since
wave 3. The inbox asks per entry (a control on the card, absent when AI is off), follows the
job, and renders the proposal in the card's slot as an `AISuggestion`; accepting it is
`:convert` with the suggested fields and the collection the person chooses — a model does not
choose a destination (`Producing.go`'s filter, restated on the screen by the collection picker
being required) — and the subtasks appear under the entry the conversion made. A person may
also convert without asking, exactly as today.

**Acceptance:** an entry asked for a suggestion shows it on its card once the job ends; accepting
converts with the proposed title, notes and date and creates the subtasks; the collection is
chosen by the person and never proposed; dismissing leaves the entry `NEW`; with AI off the card
is the F4 card; `pnpm -r build lint typecheck test` green.

**Read:** F4-12 in `milestone-F4.md`; J-06 in `milestone-0.7.0.md`, K-01 in `milestone-0.7.5.md`;
`#529`'s note in `ai-first.md` §2 (the notes and the date are applied by the acceptance);
`packages/design-system/src/JumbleInboxItem.svelte`; `apps/webapp/src/views/JumbleView.svelte`,
`lib/data/jumble.svelte.ts`

---

## F5-04 — Semantic search, and the collection's summary **[L]**

*Depends on: F5-01.*

Two reads. **Search**: `ItemSearchQuery.mode` is `AUTO` or `LEXICAL`, and the manifest's
`semantic_search` says whether `AUTO` means anything here. The search screen gains the
switch — words only, or words and meaning — shown only where the manifest says meaning is
available, defaulting to `AUTO`, and a line under the results that says which ranking answered
(the page's own `ranking`, where the contract carries one; the task reads the response and
records what it found). Where `semantic_search` is degraded rather than absent, the switch is
gated with the health reason (decision 4's exception). **The collection's summary**:
`POST /containers/{id}:summarize` on the collection screen — what is open, what moved, what is
overdue — rendered as an `AISuggestion` with no accept, because a collection has nowhere to put
one (K-05), and dismissed when read.

**Acceptance:** the switch exists only where the manifest says so and the query carries the mode;
a degraded provider gates it with the reason `/meta/health` names; the collection summary can be
asked, followed and dismissed; both absent when AI is off; `pnpm -r build lint typecheck test`
green.

**Read:** J-10 in `milestone-0.7.0.md`, K-05 in `milestone-0.7.5.md`; ADR-0050, ADR-0054; the
`ItemSearchQuery` schema and `/search`'s description; `apps/webapp/src/views/SearchView.svelte`,
`ContainerView.svelte`; `lib/data/health.svelte.ts`

---

## F5-05 — The AI settings, and the product with AI off **[L]**

*Depends on: F5-01.*

The administration area gains `/administration/ai` (tagged, like every route under it): the
provider as `GET /ai-provider` answers it — the kind, the base URL, the two models, the
jurisdiction, whether a key is held, and `processing_allowed`, which is the consent
`ai-first.md` §3 makes a per-workspace switch — with `PUT` to configure and `DELETE` to remove,
the API key through `OneTimeSecret`'s discipline (entered, never read back, `has_api_key` the
only echo), and the budget as the quota row it is, read from `/quotas` and linked rather than
drawn twice. The screen states what consent means in the catalogue's words: which fields leave
the installation and to whom. Then the other half, which is a test rather than a screen: with
`ai_suggestions: false` in a fake manifest, every route of the application is mounted and
asserted to contain no element the AI tokens style and no control that asks — the "without
residue" of the component's own row, proved for the whole client rather than per component.

**Acceptance:** the route is tagged `administration` and the area test still passes; a provider is
configured, read back without its key, and removed; `processing_allowed` is a switch with the
consent sentence beside it; the budget links to the quota; the residue test mounts every route
with AI off and finds nothing; `pnpm -r build lint typecheck test` green.

**Read:** `ai-first.md` §3, §4; J-02, J-03, J-15 in `milestone-0.7.0.md`; ADR-0049; the
`AiProvider` and `AiProviderConfiguration` schemas; `apps/webapp/src/lib/routes.ts` and
`router.test.ts`; `views/WorkspaceSettingsView.svelte` (the shape of a settings screen);
`packages/design-system/src/OneTimeSecret.svelte`

---

## F5-06 — The entry read in another language, and its own **[L]**

*Depends on: F5-01.*

Two lines of `i18n-l10n.md` §6 and one row of §10. **Line 9**: `content_language` is data — the
entry editor gets the picker, `text_languages` from the manifest with the person's locale
preselected on a new entry, and a language not in the list is still stored (the contract's
tolerance, restated by the picker accepting a typed tag). **§10's 3.1.2**: the rendered entry
carries `lang` from its `content_language` where it differs from the document's, on the title
and the notes, so that a screen reader switches voice. **Line 7 of §7**: `:translate` — a control
on the entry (absent when AI is off) that asks for the entry in a language chosen from the same
list, the person's own preselected, and renders the answer beside the original as an
`AISuggestion` with no accept: display only, stored nowhere, the provenance line naming the
model, and `lang` on the translated part from the language asked for.

**Acceptance:** an entry created with the picker carries its language; a typed tag outside the
list is accepted; the title and the notes carry `lang` when the entry's language differs from
the page's; a translation is rendered beside the original with `lang` and disappears when
dismissed, with nothing written; `pnpm -r build lint typecheck test` green.

**Read:** `i18n-l10n.md` §5, §6 (line 9), §7; `design-system.md` §10 (3.1.1, 3.1.2); M-11 in
`milestone-0.8.0.md`; ADR-0034; the `/items/{itemId}:translate` operation; `lib/data/searchlanguages.ts`
(the manifest's `text_languages`, already read once)

---

## F5-07 — The second catalogue, loaded **[L]**

*Depends on: nothing.*

Decision 5. `catalogue.ts` gains the lazy half: `import.meta.glob('../../../../../locales/*.json')`
— one chunk per file, none of them in the initial bundle — and `messages.adopt` loads the
resolved locale's chunk when it is not the source, lays it over `SOURCE` in the chain §3
describes, and re-renders when it arrives, with the source rendering meanwhile rather than
nothing. A locale the manifest lists and no file exists for renders the source and is logged
once in development. The workspace lint's exception widens from `locales/en.json` to
`locales/*.json` and its selftest still proves an escape to anything else is refused.
`catalogue.test.ts` parses every message of every file, not only the source's, so a translator's
ICU error fails the build the way M-02's gate fails it on the server. The `_comment` prefix is
skipped in every file. The profile's language picker, which already writes the preference and
watches `/accounts/me`, now changes the language it renders in.

**Acceptance:** with the account's locale set to `de`, the frame renders `de.json`'s messages
where it has them and `en.json`'s where it does not, without a reload; the German chunk is not
in the initial bundle, proved by the build's manifest; a locale with no file renders the source;
every file under `locales/` is parsed by the test; the lint accepts the directory and refuses a
sibling; `pnpm -r build lint typecheck test` green.

**Read:** `i18n-l10n.md` §2, §3, §6 (lines 1 and 2); M-01…M-04 in `milestone-0.8.0.md`;
`apps/webapp/src/lib/i18n/catalogue.ts`, `i18n.svelte.ts`, `locale.ts`, `catalogue.test.ts`;
`build/lint-workspace-map.mjs`; `project-structure.md` §2.1

---

## F5-08 — The German interface **[L]**

*Depends on: F5-07.*

Decision 6. The `app.*` family translated into `de.json`, every one of the 1,692 codes, in the
voice `voice-and-tone.md` sets (the informal address the German catalogue already uses for the
notifications is kept — one register per file), with ICU plurals where German counts differently
(`{n, plural, one {…} other {…}}` inside the subset), with the `_comment` notes a translator
needs where a code's parameter is not obvious, and with nothing invented: a code whose English
is a placeholder stays a placeholder. `make locales` reports the family complete; the translation
process ADR-0055 wrote is followed and, where it turned out to be missing a step, corrected in
the same pull request.

**Acceptance:** `make locales` reports `app.*` at 100 % for `de`; `catalogue.test.ts` and the
Go loader both accept every message; the application in German shows no English sentence on any
route, walked in the browser and recorded in the pull request; `make gate-architecture` (the
translation gate) is green; the pull request touches `de.json` and the process document and
nothing else.

**Read:** ADR-0055; `docs/architecture/i18n-l10n.md` §3; `voice-and-tone.md`; `locales/de.json`
(the register); `tools/locales`; M-12 in `milestone-0.8.0.md`

---

## F5-09 — `Intl` throughout **[L]**

*Depends on: F5-07.*

Lines 3, 4, 5, 8 and 10 of `i18n-l10n.md` §6, audited against the client and closed. What exists
is used: `Intl.DateTimeFormat` and `Intl.RelativeTimeFormat` in `datetime.ts`, `Intl.NumberFormat`
in `bytes.ts` and in the ICU renderer. What the audit looks for: a list the client sorts without
`Intl.Collator` (and `natural_ordering` read to know whether the server already did); a title cut
by `slice` rather than `Intl.Segmenter` (line 8), and a length counted in UTF-16 units where the
server counts code points; a `NUMBER` custom field typed with a dot in a locale that writes a
comma (line 10 — `supported_locales[].decimal_separator`, parsed before the raw value reaches the
contract); a due date shown without its zone when the zone is not the reader's (line 4); a week
number computed as ISO where `Intl.Locale.getWeekInfo()` says otherwise (line 5); and a calendar
that starts the week on a day the account did not choose. Each finding is fixed in
`lib/i18n/` — one module per concern, tested in plain Node — and the components that had the
logic inline call it.

**Acceptance:** every finding is listed in the pull request with the file and the fix; the
timeline and the recurrence editor start the week where the account says; a `NUMBER` field
accepts `1,5` under `de` and sends `1.5`; a long title is cut by grapheme; a client-sorted list
uses the collator for the resolved locale; a due date in another zone names it; the new
modules have tests; `pnpm -r build lint typecheck test` green.

**Read:** `i18n-l10n.md` §4, §5, §6 (lines 3–5, 8, 10), §7; M-05, M-06 in `milestone-0.8.0.md`;
`apps/webapp/src/lib/i18n/`; `lib/entries/TimelineView.svelte`, `RecurrencePanel.svelte`,
`CustomFieldPanel.svelte`; `packages/design-system/src/CustomFieldRenderer.svelte`

---

## F5-10 — The RTL audit **[L]**

*Depends on: F5-07.*

Decision 7. Line 6 of §6: the document runs the way the locale's script runs. `dir` on the root
is set since F1; what has never been checked is whether every component and every route survive
it. The workbench's `dir` axis is walked for every story and the application is walked route by
route with the operator's `ar.json` (the QS-08 file, laid over the embedded catalogues by the
dev server's `HUBTASK_LOCALES_DIR`) — so that the walk sees Arabic text running right-to-left,
mirrored icons where an icon has a direction (a back arrow, a chevron; not a clock), a
breadcrumb collapsing from the correct end, a kanban whose columns start at the right, a
timeline whose axis runs the other way, a drag whose keyboard alternative moves the right way,
and a number that stays a number. Every physical property found (`left`, `right`,
`margin-left`, `text-align: left`, a translate with a sign) becomes a logical one; an icon that
should mirror gains the `mirrored` mark ADR-0041's set provides for; the design system's
conventions test gains the rule that a physical inline property fails the build, so that the
audit is the last one that has to be walked for this.

**Acceptance:** `docs/evidence/RTL-<date>.md` lists every story and every route with its result;
every finding is fixed in the same pull request or is an issue with a reason; the conventions
test refuses a physical inline property and its selftest proves it; `pnpm -r build lint typecheck
test` green.

**Read:** `i18n-l10n.md` §6 (line 6); `design-system.md` §3 (logical properties), §6 rule 4;
ADR-0037 (the axes); ADR-0041 (mirroring); `packages/design-system/test/conventions.test.js`;
`docs/evidence/QS-08-2026-09-15.md` (the `ar.json` and how it was laid over)

---

## F5-11 — The keyboard walk **[L]**

*Depends on: F5-01 … F5-06 (the routes that exist by then are the routes walked).*

Decision 8, the first walk. Every route in the capability manifest, from the address bar, with
no pointer: `Tab` and `Shift+Tab` through the whole page and the order recorded; every control
reached and operated; every overlay entered, escaped and the focus found back where it was;
every drag done by its keyboard alternative (2.5.7 — the reorder menu, the move dialog, the
bucket change) and the alternative's announcement heard through the one live region; the focus
ring visible on every stop and never under a sticky region (2.4.11); no trap (2.1.2); nothing
that navigates on focus or submits on a change alone (3.2.1, 3.2.2). A skip link to `<main>` at
the top of the frame if the walk finds the navigation costs more than a few stops, which it
will. Each finding is fixed in the same pull request; a finding that is a product question is an
issue.

**Acceptance:** `docs/evidence/A11Y-keyboard-<date>.md` lists every route with its tab order,
its stops and its findings; every finding is fixed or an issue; the frame has a skip link;
`router.test.ts` or a sibling asserts that every route's view renders exactly one `<main>` and
one `<h1>`; `pnpm -r build lint typecheck test` green.

**Read:** `design-system.md` §6 rule 5, §10 (2.1.1, 2.1.2, 2.4.3, 2.4.7, 2.4.11, 2.5.7, 3.2.1,
3.2.2); `packages/design-system/workbench/lib/focus-walk.ts` (the per-component walk this
extends to a route); `packages/design-system/src/focus.ts`, `layers.ts`; `apps/webapp/src/lib/announce.svelte.ts`;
`lib/entries/dragging.svelte.ts`

---

## F5-12 — Status messages, reduced motion, redundant entry, text spacing **[L]**

*Depends on: F5-11.*

Four rows of §10 that are reviews rather than walks. **4.1.3**: every change that is not
focused is announced — a save landing, a job finishing, the health banner appearing, a bulk
action's count — through the one region `announce.svelte.ts` owns, audited by grepping every
write in `lib/data/` for what it tells the reader afterwards. **2.3.3**: decision 9's switch,
`lib/motion.ts` beside `theme.ts`, the profile's control beside the theme's, the media query
honoured when no choice is made. **3.3.7**: every flow that asks for something twice — a
password on a step-up that just had one, a collection on a conversion that has one selected —
reviewed per flow and each duplicate removed or its reason written down. **1.4.12**: the
text-spacing bookmarklet on every route, and what breaks fixed.

**Acceptance:** the pull request lists every write and what it announces; the motion switch
sets `data-motion` and keeps its choice on the device; the profile shows it; every flow reviewed
for 3.3.7 is listed with its verdict; the text-spacing pass is recorded per route; `pnpm -r
build lint typecheck test` green.

**Read:** `design-system.md` §6 rule 6, §10 (4.1.3, 2.3.3, 3.3.7, 1.4.12); ADR-0043 (the device
preference); `apps/webapp/src/lib/theme.ts`, `announce.svelte.ts`; `views/ProfileView.svelte`

---

## F5-13 — The screen-reader pass **[L]**

*Depends on: F5-12.*

Decision 8, the second walk. VoiceOver on macOS through every route, and VoiceOver on iOS
through the routes the mobile shell will ship (end-user and profile), in Safari; NVDA and Orca
where a machine is available and recorded as not walked where it is not, because a pass that
claims a reader nobody ran is the false statement §10 warns about. What is listened for: every
control's name, role and state (4.1.2); the structure read as headings, lists and tables
(1.3.1); every `Icon` named or silent (1.1.1); `lang` switching the voice on an entry in
another language (3.1.2); a refusal read with its field (3.3.1); a live region heard once and
not twice (4.1.3). Findings are fixed in place, product questions become issues, and the
statement's content is written from the result: `apps/website`'s `/accessibility/` page gains
the standard (EN 301 549, WCAG 2.2 AA), the status as the walk found it, the known exceptions
with their reasons and dates, the way to report a barrier, and the date of the assessment — with
the sentence that says *published* reserved for convergence, which is when the site's 1.0 content
goes live. The application's footer links to it.

**Acceptance:** `docs/evidence/A11Y-<date>.md` in the shape `design-system.md` §10 asks for, with
every route, every reader run and every finding; every finding fixed or an issue; the statement's
content on the website page with the exceptions the walk found; the footer link in the frame;
`pnpm -r build lint typecheck test` green; `make gate-docs` green.

**Read:** `design-system.md` §10 (the whole table, the two walks, the statement);
`data-protection.md` §7; ADR-0044 (the browsers a claim is made against); `apps/website/src/routes/accessibility/`;
`docs/evidence/README.md`

---

## F5-14 — QS-08 completed for the interface, and the lists read back **[L]**

*Depends on: everything above.*

Decision 10. The walk: the operator's `ar.json` laid over the integration environment's
catalogues, a person switching their account to `ar` in the profile and nothing else, every
route in Arabic and right-to-left, the notifications already in Arabic since the server's walk —
appended to `docs/evidence/QS-08-2026-09-15.md` as its interface section, and arc42's QS-08 row
completed. Then the two lists: `i18n-l10n.md` §6's eleven lines and `design-system.md` §10's
twenty rows, each read against the client and each gaining the task that met it or the issue
that still owes it, in the *proved by* column where the row has one. `roadmap.md` gains the
"F5 is done" paragraph in the shape F1's…F4's have — what cutting it found (no core task, and
why), what building it found — and the maturity stage stays `preview`, with the sentence that
says `stable` is convergence's.

**Acceptance:** the QS-08 evidence file carries the interface section and arc42's row points at
it; every line of §6 and every row of §10 names a task or an issue; the roadmap paragraph is
written; `make gate-docs` is green; no code change is in this pull request.

**Read:** `docs/evidence/QS-08-2026-09-15.md`; `arc42.md` §10 (QS-08); `i18n-l10n.md` §6;
`design-system.md` §10; `docs/roadmap.md` (the F1…F4 paragraphs); `deploy/integration/README.md`

---

## The order at a glance

```
F5-01 ──┬── F5-02 ─┐
        ├── F5-03 ─┤
        ├── F5-04 ─┼── F5-11 ── F5-12 ── F5-13 ─┐
        ├── F5-05 ─┤                            ├── F5-14
        └── F5-06 ─┘                            │
F5-07 ──┬── F5-08 ──────────────────────────────┤
        ├── F5-09 ──────────────────────────────┤
        └── F5-10 ──────────────────────────────┘
```

Two tasks depend on nothing and can start at once: the AI treatment **F5-01** and the second
catalogue **F5-07**. The five AI screens hang from the first, the three localisation tasks from
the second, and the accessibility chain from the screens existing. F5-14 is last by definition.

**Definition of Done for the milestone:** a proposal is visibly a proposal, is accepted or
dismissed in one gesture, and is nowhere at all when AI is off — proved for every route; the six
operations of an entry, the jumble's suggestion, the collection's summary and the semantic
switch are operable from the application; the AI provider is configured from the administration
area with its consent stated; an entry says what language it is in and can be read in another;
the interface renders the second catalogue lazily and German is complete; numbers, dates, weeks,
graphemes and decimal input follow the locale; every component and every route runs
right-to-left; every route has been walked with a keyboard and with a screen reader, the
findings fixed, the evidence filed, and the statement's content written; QS-08 is complete for
the interface; every value still comes from `tokens.json`, the bundle carries no inline script
or style and contacts no origin the policy does not name; `go build ./...` and `go test ./...`
still succeed with no Node.js installed; and the two requirement lists say, line by line, what
met them.
