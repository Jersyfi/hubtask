# Milestone F8 — The rule flow

The goal: a rule is **drawn** rather than filled in. Today `apps/webapp/src/views/RulesView.svelte`
is a form under administration — one `Select` per trigger, a text field per condition, and every
action's parameters typed as JSON — and a person reading it back cannot tell what the rule does
without reading it twice. When this milestone closes, a rule is a path on a canvas: what starts
it at the top, the gate of conditions it has to pass, the chain of steps it performs with a
branch drawn as a fork and a wait as a pause on the line, and the guardrails at the end; a
sample event can be dropped in at the top and watched travelling down, saying at every card what
it decided; and a rule that a later version of the product, or a deleted label, would leave
silently useless is found and said before it fails.

The concept was designed on 2026-09-20 as an interactive prototype outside the repository,
walked by the owner in two passes, and approved in its second pass with the instruction to build
it. What the prototype settled is written here as decisions, so that the code is built from the
decisions and not from a memory of the prototype.

F8 is the eighth milestone of the client track (`roadmap.md` phase 5) and the first cut after
the owner's walk of F6. It is a late requirement in the sense of the roadmap's *Requirements that
arrive late* and is scheduled like any other because `0.9.5` has not opened. Both sides get their
tasks: three core tasks, all additive to the contract, and five client tasks. **F7 keeps its
letter and stays uncut** — the shells wait for the accounts and certificates, and this milestone
does not need them.

**F8 is not a version.** Nothing is released by it; the product version stays the single line
ADR-0035 decided, and the client's maturity stage stays `preview`.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue. Both are **[L]** during the initial phase (`CLAUDE.md`).

What deliberately is **not** in this milestone: **a free node graph.** Hubtask's rule model is a
list with nested branches ([`automation.md`](../architecture/automation.md) §1) and a canvas that
let two arms rejoin anywhere, or a step point back up, would draw freedoms the engine does not
have. **A description field on the rule.** The sentence the client builds from the rule is the
description, and it is the second view of the same rule rather than a text that can drift from
it. **A second usage page.** *Grenzen* (`/administration/quotas`) already answers "what this
workspace may use and how close it is"; automation has no quota row today, and inventing one is
not this milestone's. **A change to the run engine** — nothing about how a rule runs, is
throttled or is replayed changes; the milestone changes how a rule is written, read and checked.
**The shells** (F7).

Eleven decisions taken while writing this backlog, so that nobody re-derives them:

1. **The canvas is a vertical flow, in this order and no other:** the trigger card; the gate,
   one block holding every condition, through which the run has to pass; the chain of actions,
   each a card, with `BRANCH` drawn as a fork into *then* and *otherwise* that rejoins, `WAIT` as a
   pause with its duration written on the line below it, and `STOP` as a terminus that draws no
   line onward; the guardrails card last — `on_error`, `throttle`. Every gap between two cards is
   exactly one line with one insertion point in its middle. The trigger is the only card in the
   signature colour.
2. **The manifest serves each action's fields, additively.** `automation.actions` stays what it
   is — the sorted kind names, and ADR-0004's rule on renaming an existing field applies — and
   `automation.action_fields` is added beside it: for every kind, the fields its use case
   declares (`usecase.Field`: name, kind, required, enum, description), derived from the
   descriptor exactly as `presentation/mcp.InputSchema` derives the tool schema, never declared a
   second time. The editor renders a card's form from it. A field of kind `id` whose name ends in
   `_id` is offered as a picker over the client's own store of that kind (`label_id` → the
   labels, `bucket_id` → the buckets, `group_id` → the groups, `template_id` → the templates,
   `subscription_id` → the webhook subscriptions, `container_id`/`parent_id` → the containers,
   `account_id` → the people); every other field is typed. **A rule names things by their
   identifier**, which is why a renamed label keeps working and only a deleted one becomes a
   finding (decision 6).
3. **A condition is composed as a sentence and stored as CEL.** The composer offers a bounded set
   of subjects — an item's labels, type, assignees, due date, custom fields; the actor; the hour
   of `now` — with the operators each takes, and compiles to the expression the server stores;
   the compiled expression is shown under the sentence, and *edit as expression* switches the
   condition to the raw text the server already accepts. The reverse — reading a stored
   expression back into a sentence — is done by matching the shapes the composer emits, and an
   expression the composer did not write is shown as an expression. The server compiles at the
   write (ADR-0009) and answers line and column; the client shows both at the condition.
4. **The name is generated unless it is owned, and the sentence is always generated.** The
   contract keeps `name` required. The client builds a name from the trigger and the first two
   steps in the person's language; a rule whose stored name equals the name generated from it
   counts as *automatic* and follows every edit, any other name is the person's own and is left
   alone; clearing an own name returns to automatic. Known and accepted: after a language switch
   an automatic name reads as an own one until it is cleared. The sentence under the name is
   built from the whole rule, collapsed to one line, expanded on request, and the expansion is a
   per-viewer convenience kept in the browser.
5. **Health is the client's arithmetic over the last page of runs.** No new statistic on the
   server: the rule's screen reads `GET /automation/runs?rule_id=` (page size 20) and says
   *works* (no failure), *fails sometimes* (a failure among them), *failing* (more failures than
   not), *off* (`enabled: false`) — plus *needs attention* and *broken* from decision 6. The
   same word stands in the rule's head, on its card in the list, and on the runs tab beside the
   last five runs. Two filters are added to the runs listing, additively: `since` and `until` on
   `started_at`. A *run as* filter is not — a run row has no account of its own (`triggered_by`
   is who pressed a manual trigger), and filtering by rule answers the question.
6. **The check, and where its findings live.** A rule that cannot run is not stored
   (`automation.md` §2.2) and a rule that fails five times switches itself off — both after the
   fact. The check speaks before it: it resolves every reference a rule carries against what
   exists now — the action kind against the catalogue, every parameter key against the
   descriptor, every condition against the compiler, every `*_id` parameter against the store of
   its kind through one `ReferenceResolver` port, the `run_as` account against the accounts, and
   the three conditions of §2.1 against the writer's and the account's rights — and writes its
   findings **on the rule** (`findings`, a JSON column, and `checked_at`), so that reading the
   rules costs nothing more than reading the rules. Two levels: `ATTENTION` — the rule runs, but
   a step would find nothing where it points; `BROKEN` — the rule cannot run, it is switched off
   by the check with the audit entry and the notification the auto-disable already sends to the
   author. The check runs **on demand** (`POST /automation/rules:check`, the whole workspace; the
   list screen calls it when it opens, which is what makes "after an update" true without
   anything enumerating tenants — [`multi-tenancy.md`](../architecture/multi-tenancy.md) §2.1),
   and **on the deletion events** of what a parameter may name, as far as those events exist,
   through a subscriber that checks the rules of the tenant the event belongs to. It writes an
   ADR because it adds to `automation.md`, and the ADR is put to the owner by name.
7. **Only what can take a dragged piece lights up.** While a piece is lifted, a line above the
   canvas says where it may go; the places that may take it become labelled targets (*insert
   here*, *replace the trigger*, *add the condition*) and everything else is inert and refuses
   the drop; letting go elsewhere says why in one sentence. A card cannot be moved into its own
   branch. Every drag has a keyboard equivalent (move up, move down, the `+` popover), because
   rule 5 of the design system is not optional for an editor.
8. **Narrow and deep.** Below `bp.expanded` (905 px) the palette is gone and the `+` popover is
   the one way to insert; the inspector is a sheet over the canvas — `Drawer` from the bottom,
   one glass surface at a time (rule 2) — opened by selecting a card, and a bar at the bottom
   opens it again and starts the probe; a branch shows one arm at a time behind a *then (n) ·
   otherwise (n)* switch. The same switch appears on any width from the second nesting depth,
   because two branches side by side inside a branch are four columns of 180 px. Any branch
   can be folded to one line saying how many steps each arm holds.
9. **The probe is `POST /automation/rules:test` drawn onto the canvas.** The sample is the
   trigger's event type and a real entry named as `subject` (chosen through the client's own
   search; a read, never a write), plus the payload document for an inbound rule. The result is
   drawn as the run: the line fills down, each condition says *held* or *did not hold*, the arm
   a branch would take stays lit and the other dims, every action says *would run* with the
   `path` the result names, and the status the run would have stands at the bottom. A recorded
   run from the runs tab is drawn the same way from its own `condition_results` and
   `action_results`.
10. **Where the code lives.** The editor is product-specific and lives in
    `apps/webapp/src/lib/automation/` beside `ActionList.svelte`, which it replaces; the design
    system gains no automation component, only the icons the cards need in
    `build/icons.js`'s declared set (a bolt, a pause, a fork, a square, a globe, a hand, an inbox,
    a play mark). `Drawer`, `Popover`, `Tabs`, `Select`, `Input`, `Textarea`, `Switch`, `Badge`,
    `Callout`, `Banner`, `OneTimeSecret`, `RunStatusBadge` and `SearchField` are what it is built
    from. Every colour, spacing, radius and duration is a token; the prototype used none that
    `tokens.json` does not have, and neither does the build.
11. **The list is the entry, the runs page is the record.** `/administration/rules` becomes a
    list of cards — name, trigger, scope, account, last run, state, health, the first finding —
    with the check's banner above it when there are findings; a card opens
    `/administration/rules/{id}`, *write a rule* opens `/administration/rules/new`. The existing
    `/administration/runs` stays the one place every run of every rule is, gains the two time
    filters and a summary strip, and every rule links to it prefiltered; a failed run is replayed
    from there as today.

---

## F8-01 — The manifest serves each action's fields **[L]**

*Depends on: nothing. Issue #794.*

Decision 2. `api/openapi.yaml` first: `CapabilityManifest.automation` gains `action_fields`, a
map from kind to an array of `AutomationActionField` (`name`, `kind` — the catalogue's
`string | id | boolean | integer | id_list | object | list | any` —, `required`, `enum`,
`description`); `automation.actions` is unchanged. `catalogue.AutomationActionFields()` derives
it from the descriptors beside `AutomationActions()`, `GetCapabilities` carries it, the
composition root hands it over, `MetaController` answers it — always a map, empty for a kind
with no fields, for the reason every list there is always an array. `make generate`,
`make api-client`, the contract test; the capability store in `apps/webapp` types it. No
display text: `description` is protocol documentation, as it is for the MCP tool.

**Acceptance:** `GET /meta/capabilities` answers `automation.action_fields` with one entry per
kind in `automation.actions` and the fields the MCP tool schema declares for the same use case,
asserted by a test that compares the two; `make generate` and `make api-client` produce no diff
after the commit; `go test -tags contract ./test/contract/...` passes; `make verify` green.

**Read:** `core/application/catalogue/Catalogue.go` (`AutomationActions`);
`presentation/mcp/ToolRegistry.go` (`InputSchema`); `core/application/usecase/Input.go`
(`Field`, `Kind`); `presentation/rest/MetaController.go`; `api/openapi.yaml`
(`CapabilityManifest`); `apps/webapp/src/lib/data/capabilities.svelte.ts`

---

## F8-02 — The runs listing takes a time window **[L]**

*Depends on: nothing. Issue #795.*

Decision 5. `GET /automation/runs` gains `since` and `until` (`date-time`, optional, on
`started_at`), additively and specification first; `ListRuleRuns` in `db/queries/Automation.sql`
gains the two nullable arguments in the one statement it already is; the port, the repository,
`ReadRuns` and the controller carry them; a window with `until` before `since` is refused with
a field. The client's runs store takes both.

**Acceptance:** the two parameters narrow the listing and combine with the three that exist;
the repository test covers the window and the cross-tenant negative (SG-3); `make generate` and
`make api-client` produce no diff; the contract test passes; `make verify` green.

**Read:** `db/queries/Automation.sql` (`ListRuleRuns`);
`core/application/service/automation/ReadRuns.go`; `presentation/rest/AutomationRuleController.go`
(the runs route); `api/openapi.yaml` (`listRuleRuns`); `apps/webapp/src/lib/data/runs.svelte.ts`

---

## F8-03 — The check **[L]**

*Depends on: nothing; F8-07 draws it. Issue #796.*

Decision 6, and the milestone's one ADR — written first, as `proposed`, named to the owner. Then,
specification first: `AutomationRule` gains `findings` (an array of `RuleFinding`: `level`
`ATTENTION | BROKEN`, `path` — the action path, `conditions/1`, `trigger`, `run_as` —, `code`
a message code, `params` for it) and `checked_at`; `POST /automation/rules:check` answers the
workspace's rules with their findings after checking every one. One expand migration adds the two
columns. The `CheckRules` use case, registered and reachable over the three channels, runs the
check the ADR describes with a `ReferenceResolver` port (`Exists(kind, id)`) implemented in
`infrastructure/postgres` for the seven reference kinds decision 2 names; a `BROKEN` rule that is
enabled is disabled by the check under its own audit action, and the author is told through the
path the auto-disable uses. A subscriber on the deletion events of labels, buckets, containers,
templates and webhook subscriptions checks the rules of that tenant. Message codes for every
finding in `locales/en.json` and `de.json`; the data catalogue's rule row gains the column; no
merge rule is needed — the row is the server's alone.

**Acceptance:** a rule naming a label that was deleted answers `ATTENTION` at the parameter's
path after `:check` and after the deletion event; a rule naming an action kind the catalogue no
longer has answers `BROKEN`, is disabled, audited and notified; a rule with nothing wrong answers
an empty array and a fresh `checked_at`; the check writes nothing but the findings and the
disable; a cross-tenant negative for the resolver (SG-3); parity, metric, span and audit
registry as the Definition of Done asks; `make verify` green.

**Read:** `automation.md` §2.1, §2.2; `core/application/service/automation/Actions.go`
(`checkActions`), `Rules.go` (the write-time checks and the auto-disable); ADR-0009; the reseal
census in `Reseal.go` for the shape of a whole-workspace pass; `multi-tenancy.md` §2.1 (nothing
enumerates tenants); `docs/privacy/data-catalog.md`

---

## F8-04 — The editor **[L]**

*Depends on: F8-01. Issue #797.*

Decisions 1, 2, 3, 4, 10, 11 — the canvas and the inspector, without drag and drop (F8-05) and
without the probe (F8-06). Two routes, `/administration/rules/new` and
`/administration/rules/{id}`, both `administration`. The head: the name, automatic or own, with
the pencil and the *automatic* mark; the sentence, collapsed; the chips *applies in*, *runs as*.
The canvas: the trigger card with its kind's own fields — the event type and `changed_fields`
for `EVENT`, the recurrence and time zone for `SCHEDULE`, `anchor` and `offset` for
`RELATIVE_DATE`, the address with `OneTimeSecret` for `INBOUND_WEBHOOK` as `RulesView` has it
today, the *start now* button for `MANUAL`; the gate with a condition row per condition and the
composer in the inspector; the chain with every action kind the manifest names, its form built
from `action_fields`, and `WAIT`, `BRANCH`, `STOP` drawn as decision 1 says, a branch foldable;
the guardrails card. Every card selects into the inspector; the `+` in every gap opens a
`Popover` listing the kinds by group with a search; the palette on the left offers the same and
inserts at the end on click. Save, enable, disable, delete with a `Dialog`; the server's
refusals rendered at the field they name, as today. `ActionList.svelte` is removed. The old form
goes; the list screen is F8-07's, so until then the list keeps its cards and opens the editor.

**Acceptance:** every rule the old form could write can be written on the canvas, and every
stored rule opens on it without loss — asserted by a round-trip test over the fixtures in
`rules.test.ts` extended by a branch, a wait, a stop and a raw-expression condition; a condition
composed as a sentence stores the expression the test names; a name equal to its generated
form follows an edit and an own one does not; the workbench axes hold for the pieces the design
system contributes (the icons); `pnpm -r build && pnpm -r lint && pnpm -r typecheck && pnpm -r
test` green; the Go lane green (a message code changes `locales/`).

**Read:** `apps/webapp/src/views/RulesView.svelte`, `apps/webapp/src/lib/automation/ActionList.svelte`,
`apps/webapp/src/lib/data/rules.svelte.ts` and `rules.ts`; `automation.md` §1; ADR-0009;
`design-system.md` §6; `voice-and-tone.md`; `apps/webapp/CLAUDE.md`

---

## F8-05 — Drag and drop, keyboard, narrow **[L]**

*Depends on: F8-04. Issue #798.*

Decisions 7 and 8. Palette pieces and cards are draggable with the browser's own drag and drop;
lifting one sets the targets and the hint line, letting go inserts or moves, letting go
elsewhere explains; a card cannot enter its own branch. Move up and move down on every card's
tools, reachable by keyboard, do what the drag does. Below `bp.expanded`: the palette is gone,
the inspector is a `Drawer` from the bottom, the bar at the bottom opens it and starts the probe
(the probe itself is F8-06's; the button exists and says so until then), a branch shows one arm
behind the switch; the switch also from the second nesting depth on any width.

**Acceptance:** a Playwright run under `apps/webapp/e2e` moves a card by drag and by keyboard
and asserts the stored order; the drop of a trigger on a gap is refused with the sentence; the
narrow layout at 375 px shows the sheet on selection and the arm switch on a nested branch; the
workbench's keyboard-order axis reports the editor's order; `pnpm -r …` green.

**Read:** F8-04's code; `packages/design-system/src/Drawer.svelte`, `Popover.svelte`;
`design-system.md` §6 rule 5; the `e2e/` helpers; memory of the browser pane never ticking
`requestAnimationFrame` — verify motion and keyboard with Playwright, not in the pane.

---

## F8-06 — The probe, and the runs tab **[L]**

*Depends on: F8-04, F8-02. Issue #799.*

Decisions 5 and 9. The inspector gains two tabs beside the selected piece: *probe* — the event
type from the trigger, an entry chosen through `SearchField` as the subject, the payload for an
inbound rule, and *run it through*, which calls `rules:test` with the rule as it stands on the
canvas (the definition, not the stored rule, so an unsaved change is what is tested) and draws
the answer — and *runs* — the health word with its reason, the last runs as a row of bars and
as five rows, and the link to `/administration/runs?rule_id=`. A run row draws its recorded
path onto the canvas. Motion in `opacity` and `transform` only, and none under reduced motion
beyond the colour change.

**Acceptance:** with a rule whose condition names a label, a probe against an entry with the
label draws every condition *held* and every action *would run*, and against one without draws
the gate *did not hold* and the chain dim, asserted by a Playwright run against the fake
transport; the drawn arm of a branch is the one `matched` names; the health word matches the
arithmetic over a fixture of runs in a unit test; `pnpm -r …` green.

**Read:** `api/openapi.yaml` (`RuleTest`, `RuleTestResult`, `RuleRun`);
`apps/webapp/src/views/RunsView.svelte` (the dry run and replay as they are);
`apps/webapp/src/lib/data/runs.svelte.ts`; `design-system.md` §6 rule 6, §7 (the motion roles)

---

## F8-07 — The list, the findings, and the runs page **[L]**

*Depends on: F8-04, F8-03, F8-02. Issue #800.*

Decisions 6, 11. `/administration/rules` is the list of cards decision 11 describes, calling
`:check` when it opens and showing the banner with the count of rules that need attention or are
broken; the first finding on each card, every finding in the rule's head and on the card it is
about on the canvas, with the reason from the message code; a `BROKEN` rule opens with the card
marked and the enable button refused with the finding. `/administration/runs` gains `since` and
`until` in its filters, the summary strip over the page (runs, succeeded, skipped, throttled,
failed in the window), and is what every *all runs of this rule* link opens prefiltered.
`AutomationRuleCard` gains the health and the finding as resolved text, with its story.

**Acceptance:** a workspace with a rule naming a deleted label shows the banner, the card's
finding and the marked card on the canvas, from the fake transport in a Playwright run; the
runs page filters by window and its strip counts what the filter shows; the coverage report's
automation rows are current; `pnpm -r …` green and the Go lane green.

**Read:** F8-03's contract; `apps/webapp/src/views/RunsView.svelte`;
`packages/design-system/src/AutomationRuleCard.svelte` and its story;
`docs/evidence/COVERAGE-2026-09-18.md`

---

## F8-08 — The walk, and the documents current **[L]**

*Depends on: everything above. Issue #801.*

The walk of the whole: a rule written on the canvas from nothing, probed, saved, switched on,
run by a real event, read back on the runs page, its label deleted and the finding seen,
repaired and switched on again — on a local server first and on the integration environment
once `main` is deployed, filed as `docs/evidence/F8-<date>.md` with what it found. Then the
documents: `automation.md` gains its §1.5 on the editor's obligations (what a client must build
from the manifest and never compile in) and the check's section the ADR points to; the ADR moves
to `accepted` on the owner's word; `roadmap.md`'s F8 row says what was built; `de.json` is
complete for every code the milestone added (`make locales`). Anything the walk finds is an
issue, not a fix in this task (F6-15's rule).

**Acceptance:** the evidence file exists with the walk's steps and findings; `make gate-docs`
green; `make locales` reports the German catalogue complete for the milestone's codes; every
issue of the milestone closed by its pull request.

---

## The second round — from the owner's walk of the built editor

The owner walked the built editor on 2026-09-21 against the concept and sent what the concept
had and the build does not, and what the build does badly. Six more tasks, each one pull request
in milestone 20, all client work but the words in the catalogue; the engine, the contract and
the check stay as they are. Four more decisions:

12. **A building block carries its icon.** The prototype drew one per kind and decision 10
    reserved icons for the cards only; the palette and the `+` menu without them read as a list
    of words. The icon is the card's (`FLOW_ICON`, the kind's group), chosen per kind in one
    table in `words.ts`, declared in `build/icons.js` and nowhere else; a kind the table does not
    know gets its group's icon. And the palette's *everything else* — every use case the
    installation serves, ninety names of which `Confirm TOTP` and `Create backup target` are two
    — is folded behind the `+` menu's search rather than listed: the curated groups are the
    palette, the rest is found by typing.
13. **An event is said in words, and the words come from the type's own name.** The manifest
    answers forty-odd `de.hubtask.<area>.<entity>.<verb>.v1` names, and a select of forty wire
    names is unreadable. The client derives the words from the segments — the entity's group
    (*Entries*, *Hubs and collections*, *Comments*, *People*, *Labels*, *The inbox*, *Rules*,
    *Integrations*, *Everything else*) and the verb in the person's language, from a small table
    of verbs with the raw segment as the fallback — and every select of an event (the trigger's,
    the probe's) is grouped by entity with the words, the wire name in the hint under it. Nothing
    is compiled in that the manifest does not serve: an event the table has no verb for is
    shown as its segments, never hidden.
14. **`STOP` is a terminus and goes last.** A stop in the middle of a list would make every step
    after it unreachable and the canvas cannot draw "unreachable" honestly. So a `STOP` may only
    be inserted as the last step of a list — the chain or an arm — and the drop targets and the
    `+` menu say so: while a stop is lifted only the last gap of each list lights, and the menu
    offers *Stop* only from the last gap. After a stop the line ends (no gap, no `+`, no line to
    the guardrails); a stored rule with steps after a stop — the server accepts one — draws them
    faded with *never reached* on the first, and the check is not involved. A stop is moved by
    the same rule: only to the end of a list.
15. **A condition is a tree, and the tree is composed everywhere a condition is.** The gate and
    a branch's condition both take the same composer, which now composes a *group* — *all of* /
    *any of* / *none of* over sentences and nested groups — and compiles it to CEL with
    parentheses (`(a && b) || !(c)`); the reader takes the composer's own shapes back apart by
    the same grammar (parentheses, `&&`, `||`, a leading `!`) and anything else stays an
    expression, as decision 3 says. The gate's rows stay the top-level *all of*: each row is a
    sentence or a group, so `conditions[]` keeps its meaning on the server. The subjects gain
    what the run's document answers and the prototype offered: *title* (contains, does not
    contain, starts with), *notes* (is empty, is not empty, contains), *due date* (is before,
    is after, is within the next N days, is set, is not set), *archived*, *depth* (is, is at
    most), beside the existing *type*, *completed*, *assignee*, *bucket*, *parent*, *actor*,
    *hour* and *custom field* — every one with *is* / *is not* where a value is compared.
    Labels stay out until issue 807 is fixed.

## F8-09 — Building blocks with their icons, and the rest behind the search **[L]**

*Depends on: nothing. Issue #841.*

Decision 12: an icon per kind in `words.ts` (one table, the group's icon as the fallback),
drawn in the palette, the `+` menu and on the card; the icons the table needs declared in
`packages/design-system/build/icons.js` — `tag`, `calendar`, `check`, `archive`, `trash`,
`copy`, `image`, `users`, `user`, `message-square`, `inbox`, `repeat`, `globe`, `zap`,
`sparkles`, `folder`, `columns-3`, `sliders-horizontal`, `arrow-right-left`, `rotate-ccw`,
`user-plus`, `user-minus`, `skip-forward`, `layout-template`, `plus`, `x`, `undo-2` — whichever
of those lucide has, and the declared set is what `pnpm build` checks. The palette shows the
curated groups only; *everything else* goes: the `+` menu's search finds every served kind.

**Acceptance:** every palette item and every menu item has an icon; a kind outside the curated
groups is found by typing in the `+` menu and not listed in the palette; the Playwright walk
inserts one that way; `pnpm -r …` green.

## F8-10 — Events in words, grouped by what they are about **[L]**

*Depends on: nothing. Issue #842.*

Decision 13: `eventWords(type)` in `words.ts` answers `{ group, words }` from the segments and
a verb table in the catalogue (`app.flow.event_verb_*`, `app.flow.event_group_*`); the trigger's
event select and the probe's are grouped selects (`<optgroup>`, which `Select` gains as an
optional `groups` prop) with the words as the option and the wire name shown once under the
select; the trigger card, the sentence and the runs page say the words. `changed_fields` on an
*updated* event keep their checkboxes.

**Acceptance:** with the manifest's full list every option reads as words in a group; an event
type the verb table does not know reads as its segments; the e2e asserts the grouping and the
words on the card; `pnpm -r …` green, the catalogue complete in `en` and `de`.

## F8-11 — Stop is a terminus **[L]**

*Depends on: nothing. Issue #843.*

Decision 14. `insertAt` and `moveStep` refuse a `STOP` anywhere but the end of a list (the
refusal sentence `app.flow.refused_stop`); `gapTakes` answers only the last gap of each list
for a lifted stop; the `+` menu lists *Stop* only from a last gap; after a stop no gap, no `+`,
no line onward, and the guardrails stand apart; steps stored after a stop draw faded with the
word on the first. The probe and a drawn run already skip them.

**Acceptance:** a stop dragged to a middle gap is refused with its sentence and the canvas is
unchanged; from the last gap it lands and the line ends; a fixture with a step after a stop
draws it faded; the e2e covers all three; `pnpm -r …` green.

## F8-12 — Drop targets you can read, and a hint that overlaps nothing **[L]**

*Depends on: nothing. Issue #844.*

The drag hint leaves the canvas: it becomes a line of its own between the header and the
bench, reserved whether or not a piece is lifted, so nothing moves and nothing is covered. A
gap that may take the piece widens into a pill that says *Insert here* on one line, centred on
the line, the circle gone while it is a target; the trigger card and the gate, when they may
take the piece, say *Replace the trigger* and *Add the condition* as a strip along their top
edge inside the card rather than a badge over the line. The palette's group hints (*replaces
the trigger*, *goes into the gate*, *goes into the chain*) stay.

**Acceptance:** at 1400 px and at 375 px a lifted trigger, condition and step each show their
targets with whole words, nothing clipped, nothing overlapping the trigger card; the e2e reads
the hint's text from its own line; `pnpm -r …` green.

## F8-13 — Conditions as a tree, everywhere **[L]**

*Depends on: nothing. Issue #845.*

Decision 15. `model.ts` gains the group (`{ mode: 'all' | 'any' | 'none', items: (Sentence |
Group)[] }`), `compileGroup` and `readGroup` with tests over every shape — nesting two deep,
`none` as `!(…)`, a sentence alone unchanged — and the new subjects and operators with their
compile and read; `Composer.svelte` composes a group (a row per item, *and* / *or* / *not*
chosen once per group, *add a sentence* / *add a group*, *edit as expression* on the whole);
the gate's rows and the branch use it; `conditionWords` says a group as one sentence
(*the type is TASK and (the due date is set or the entry is archived)*). The server compiles as
before — nothing on it changes, and a group is one `expr`.

**Acceptance:** `model.test.ts` round-trips every operator and every group shape; an expression
the composer did not write is shown as an expression and stays editable; the e2e composes
*any of* two sentences in the gate and one in a branch and asserts the compiled CEL on the
write; `pnpm -r …` green, the catalogue complete.

## F8-14 — The second walk, and the concept's leftovers **[L]**

*Depends on: F8-09 to F8-13. Issue #846.*

The editor walked again on a local server with the five above merged, the evidence appended to
`docs/evidence/F8-2026-09-20.md` as a second section, and the concept read against the build
one more time. What the concept had and the build does not, put to the owner by name rather
than built: the list card's last runs as bars (decision 5 chose one word on the card), the
runs page's link to *Limits* (the concept's "the week's number and a link"), and the check of
the runner's rights (decision 6 promised it, ADR-0060 left it to the run, issue 817 asks for
the role half).

**Acceptance:** the evidence's second section exists; every issue of the round closed by its
pull request; the leftovers named in the pull request body with a question each.

---

## The third round — the whole concept, redrawn

The second round fixed spots, and the owner refused the third set of spots: *"Grundsätzlich soll
es einen Visuellen Editor geben. Die Benutzererfahrung ist ein wichtiger Punkt und die
Konfigurationsvielfalt. Das Konzept muss gesamtheitlich passen."* So the concept was redrawn as
one picture — the prototype's fourth edition, walked by the owner through four passes on
2026-09-21 and approved with *"bring our progress to main"*. What that edition settled is written
here, as the first one was, so that the code is built from the decisions. Six tasks, each one
pull request in milestone 20; the engine and the run stay as they are, the manifest grows three
additive things, the check two findings. Eight decisions:

16. **The canvas shows, the panel sets.** No form on the canvas. The trigger, the gate, a step,
    a branch and a rung of a ladder are *selected* on the canvas and *edited* in the panel; the
    canvas draws their state in words. One panel, the same on every width, five tabs: **Rule**
    (name, description, scope, runs as, the guardrails — the head shows them and its pencil and
    pills lead here), **Blocks** (decision 17), **Details** (what is selected), **Probe**,
    **Runs**. The panel reaches the bottom of the viewport and scrolls on its own, the canvas
    scrolls on its own and takes every other pixel of width; below `bp.expanded` the same panel
    is the bottom sheet, opened by the same five tabs in the bar. The palette on the left is gone.
17. **One list, three ways in — and nothing hidden.** Decision 12's *folded behind the search*
    is withdrawn: the owner wants every kind the installation serves reachable by eye, not only
    by typing. The *Blocks* tab holds one list with two sub-tabs and a search above both:
    **Blocks** — *Frequent* (the six kinds this workspace's rules use most, counted on the
    client from the rules it already loaded, each with its count), *Starts at* (the trigger
    kinds), then the curated categories in this fixed order: *Entries, Assignment, Structure,
    Content, Outbound, AI, Flow* — and **All** — every kind in `automation.actions`, grouped by
    the use case's area, each with its one sentence. Both sub-tabs drag and click alike (a click
    appends to the chain). The `+` in a gap opens the same list as a popover, with the same two
    sub-tabs and search, filtered to what that gap may take (decision 19: no trigger, *End the
    run* only at an arm's end), and says so under the list. The sentence comes from the manifest:
    `action_summaries` maps each kind to its use case's `Summary` — the sentence the MCP tool for
    it already carries, protocol documentation in English like every description in the contract
    (ADR-0011 is not touched: it is the same text the agent reads, shown as a hint under a name
    in the person's language, and a client that lacks the summary shows the name alone). The
    name is what decision 13's rule for events gives a kind: the catalogue's word where the
    curated table has one, the kind's segments otherwise. Nothing is compiled in that the
    manifest does not serve.
18. **The colour says what the card is.** Seven families, the same in the list, the popover and
    on the card: the trigger in the signature colour, the gate amber, *flow* violet — the branch,
    the wait and *End the run* — entries blue, assignment teal, outbound slate, AI green. The
    branch is a flow card, not a condition block: its condition stands on the card in the gate's
    notation (the funnel, the sentences, small chips for *and* / *or*), and only the gate itself
    is amber. The list offers no *Condition* or *Only if* block — the gate is always there, and
    a condition is a property of the gate or of a branch, never a piece of its own.
19. **The branch is *if / else*, a rung is a `+`, and the run ends where the chain ends.**
    Under every branch stands **+ Else if**. It appends a rung — a branch as the sole step of
    the else arm, which the engine runs today — and the canvas draws a chain of such branches as
    a *ladder*: then / else if / … / else, each rung selected like a card and edited in Details,
    each rung's arm taking steps like any arm. There is no block *Else if*, and the reader takes
    a ladder back apart by the same rule: an else arm whose only step is a branch is a rung.
    The chain's end draws the end mark *Run ends*. The block `STOP` is called **End the run**,
    because that is what it does — it ends the run, not the arm — and it may stand only as the
    last step of an arm, once; in the chain it is refused with a sentence, since the chain's end
    ends anyway. When *every* arm ends so (every rung of a ladder too), nothing may follow the
    branch: no gap, no `+`, the end mark *Run ends — on every path* stands there, and steps a
    stored rule carries after it draw faded with *never reached* (decision 14's rule, now for
    the branch as well as the stop). Every card is movable — along the chain, into an arm, out
    of one, into a rung, a whole branch with its arms — to every *Insert here* that may take it,
    and the arrows on a card move it within its list; a move that may not happen (a card into
    its own branch, anything after an end) is refused with its sentence in the hint line.
20. **The hint line takes no room until it speaks.** It stays between the header and the canvas
    but is zero height while empty and sticky at the canvas's top edge while a piece is lifted,
    so it covers nothing, leaves no gap and is read while scrolling. The branch is drawn with
    lines that are lines: a stem from the card to a bar, the bar between the two arms' centres
    with a corner at each end, a lead from each arm's pill to its first card, the join per arm
    (whole, half with its corner, or none), a stem after the join, a stem above every end mark;
    the arm pills *then* / *else* with icon and word centred; the arms with a gap between them.
21. **A step's form shows what a rule can decide.** Issue 855, way (a): `usecase.Field` gains
    `Rule bool` — false for the caller's plumbing (`id` minted by the caller, `expected_version`,
    the reserved `cascade_children`) — and `Format string` (`date-time`, `date`, `duration`,
    `uri`) where a string has a shape; `AutomationActionField` carries both as `rule` and
    `format`, additively. The form hides a `rule: false` field, draws a `date-time` as a date
    and time field, a `duration` as a select of spans, and keeps what the run supplies as its
    one line. A field the manifest does not mark is drawn as today.
22. **Two more findings.** `automation.finding.parameter_missing` (issue 856): a required
    parameter the rule does not carry and the run cannot supply, `ATTENTION`, at
    `/actions/N/params/<field>`, drawn at the card. `automation.finding.runner_without_role`
    (issue 817): the acting account holds no membership covering the rule's scope, `ATTENTION`,
    at `/run_as`, drawn at the *Runs as* pill with `run_as_hint` saying what to do about it.
    Both inside ADR-0060's frame — the check names them, the run is untouched. The rest of
    §2.1 (the writer's rights, per-action permissions) stays with the run, as ADR-0060 says.
23. **The owner's four answers, 2026-09-21.** *The list card:* the word stays, and under it one
    line *last run* — when, and how it ended; the bars the concept drew were only the words,
    coloured and side by side, and are not missed until somebody misses them. The rule carries
    its last run (`last_run`, additive) so the list still costs one request. *The runs page:*
    one line under the strip — this hour's runs against `automation_runs_per_hour` from
    `GET /quotas`, with the link to *Limits*; no new endpoint. *Issue 814:* the notification
    gets a subject of its own — `item_id` becomes optional beside a `rule_id`, exactly one set
    (expand-only migration, check constraint), no pseudo-entry — as its own pull request before
    the third walk. *The deploy:* not a decision — everything on `main` is on the integration
    environment, and no release is being held back for it.
24. **The guardrails are the rule's, not a card on the canvas.** The canvas draws the run's
    path — what starts it, what it has to pass, what it does, where it ends — and nothing that is
    not on that path. `on_error` and the throttle are properties of the rule like its name, its
    scope and the account it runs as, and those three are already said in the head and set on the
    *Rule* tab. So the guardrails card leaves the canvas and becomes the head's third chip, beside
    *Applies in* and *Runs as*, saying what they are in the same words the card said; pressing it
    opens the *Rule* tab where they are set. One place to change them, the same place as the other
    three, and the reader learns the pattern once: **the head is the rule, the canvas is the run.**
25. **A card's width is its border box.** Every card on the canvas declared `width: min(44ch, 100%)`
    under the project's `content-box` default, so at a width where `100%` won it stood its padding
    and border wider than the column that holds it — the selection ring fell into the canvas's own
    gutter and the scroll container cut it. The canvas's cards, the gate, the ladder and the folded
    line are `border-box`, and the canvas keeps a gutter no smaller than the ring it has to show.
26. **Empty canvas deselects.** A click on the canvas's background, and `Escape` while the canvas
    has the focus, clear the selection: the *Details* tab has nothing to show and is not the place
    to leave the reader, so the panel moves to *Blocks* — what one does next after letting go of a
    card is add another — unless it is on *Probe* or *Runs*, which are about the whole rule and
    stay. On a narrow screen the sheet closes instead, because there the panel is over the canvas
    and nothing is behind it to read.
27. **The sheet is sized by the reader, and keeps its size.** Below the expanded breakpoint the
    panel is a sheet over the canvas, and a sheet that scrolls its own header away is a sheet one
    cannot close without scrolling back up. The sheet's head — its title, its tabs, its close —
    stays put and only the body scrolls; above the head sits a handle, and the handle is dragged
    to size the sheet between a third and nine tenths of the screen, let go below the third to
    close it. Half the screen is what it opens at; the size the reader dragged it to is kept in
    this browser for the next time. `Drawer` gains `resizable` and a bindable `size` for it; where
    the size is kept is the caller's, never the component's.
28. **The gate is one thing, a group is offered from the first sentence, and a rung is a card
    like any other.** Four things the owner's test found in the conditions: the *Only when* block
    holds every condition, so its *Details* holds every condition too — each composer under its
    *and*, added and removed there — and clicking a single condition on the canvas still opens
    that one; the composer offers *Add a sentence* **and** *Add a group* from the first sentence,
    the group's mode appearing when there is something to hold together, rather than the group
    becoming possible only once a second sentence exists; an *else if* rung carries the trash
    every other card carries, and removing it hands its *otherwise* on to the rung above, exactly
    as *+ Else if* took it; and a folded ladder counts its rungs rather than reporting
    *otherwise 1*, which was the next rung counted as a step.
29. **The editor says what is missing before the probe is pressed.** ADR-0060's check runs on a
    *stored* rule against what exists in the workspace; it cannot speak about the draft under the
    hands, which is where writing a rule actually goes wrong: an event trigger with no event, an
    action whose required parameter is empty, a branch with two empty arms, a schedule that feeds
    a step needing the entry no schedule has. The client reads the draft against the manifest it
    already holds and says so itself — the same marks at the same cards the check's findings use,
    and a list in the *Probe* tab above the button, each line pressing through to the card it is
    about. Its codes are the client's (`app.flow.review_*`), never the server's, because it is the
    draft they are about and no server was asked; where the server's own finding says the same
    thing at the same card, the finding wins. Nothing is refused: the probe still runs, and a rule
    that a review complains about can still be saved — the review says what will happen, it does
    not decide.

**Two more, from his second pass over the round:**

30. **The gate holds one condition.** Decision 28 gave the gate's panel every condition, each
    under its *and* — and the owner asked why there are several at all, since the first one can
    already say everything: a condition is a tree of sentences under *all of* / *any of* /
    *none of*, nested as deep as the writer likes (decision 3). A second condition beside it is a
    second way to write the same *and*, and two ways to write one thing is what this concept
    keeps refusing. So the editor offers exactly one: the way in is there while there is none,
    and gone once there is one. **Nothing about the contract changes** — `conditions` stays an
    array and the engine still ands them — and a stored rule that carries several keeps them,
    each with its own remove, because the client does not silently rewrite what somebody wrote.
    It follows that **the one condition is the gate**: it is not a card of its own to select
    beside it, the gate's head says nothing about how conditions join where there is only one to
    join, and a click anywhere in the block opens the one panel that edits it. Only a rule that
    carries several keeps them apart, one card each.
31. **A page that fills the region takes its height too.** `page.fill()` (issue 918) said a
    canvas draws its own edges; it did not say how tall the region is, so the editor grew past
    the fold and the panel — as tall as the viewport but starting below everything above it —
    ended out of reach, cut off at the bottom. A filled page is one screen: the frame gives it a
    region of a definite height, and what scrolls inside it is the canvas and the panel, each on
    its own. The page itself does not scroll.


## F8-15 — The manifest says more about each action: summary, rule flag, format **[L]**

*Depends on: nothing. Issue #863.*

Decisions 17 and 21, the server half. `Descriptor.Summary` is already what the MCP tool reads;
the manifest answers it once per kind as `action_summaries` (object, kind → sentence, always
present, every kind in `actions` in it). `usecase.Field` gains `Rule` (default true; the
registry's field check is untouched) and `Format`; the three plumbing fields of the use cases
the second walk saw — and every other descriptor whose field is one of `id`, `expected_version`,
`cascade_children` — are marked `Rule: false` in the descriptor, and every RFC 3339 field
`Format: "date-time"`, every ISO 8601 span `"duration"`. `AutomationActionField` gains `rule`
and `format`; the manifest test asserts both and the summaries; `make generate` clean,
`make api-client`.

**Acceptance:** `GET /manifest` answers a summary for every kind in `actions`, `rule: false` on
the plumbing fields, `format` on the date and duration fields; the contract test covers it; a
descriptor that marks a field `Rule: false` still accepts it from a caller; `make verify` green.

## F8-16 — The panel: five tabs, the blocks and the catalogue **[L]**

*Depends on: F8-15. Issue #864.*

Decisions 16 and 17, the panel. `RuleInspector` becomes the one panel with the five tabs
(*Rule, Blocks, Details, Probe, Runs*; `app.flow.tab_*`), to the bottom, scrolling on its own;
the *Rule* tab takes name, description, scope, runs as and the guardrails from the head, which
keeps showing them and leads there; the palette component goes and its list becomes the
*Blocks* tab — sub-tabs *Blocks* / *All*, the search over both, *Frequent* counted from the
loaded rules, the fixed category order, *All* from `automation.actions` grouped by area with
`action_summaries` as the sentence; `InsertMenu` draws the same list through the same component,
filtered by `gapTakes` with the line under it; below `bp.expanded` the panel is the sheet with
the same tabs. Drag and click from every item in every place.

**Acceptance:** at 1400 px the panel reaches the bottom and the canvas has the rest; every
served kind is found in *All* by eye and by typing; *Frequent* names the workspace's most used
kinds with their counts; the `+` popover offers no trigger and, from a middle gap, no *End the
run*; the e2e inserts a catalogue kind from *All* and one from the popover; `pnpm -r …` green,
the catalogue complete.

## F8-17 — The flow: End the run, the ladder, moving, and lines that are lines **[L]**

*Depends on: nothing. Issue #865.*

Decisions 19 and 20. `model.ts`: `endsAllPaths`, `canPlace(list, at, kind, moving)` as the one
rule for insert, move and the `+` (a stop only at an arm's end, nothing after an end), the
rung as a shape the reader and the compiler both know; `STOP` reads *End the run*
(`app.flow.kind_stop`), and the refusal sentences say why. `RuleCanvas`: the end mark at the
chain's end, *on every path* after an all-ending branch with the never-reached fade after it,
the ladder drawn as rungs, **+ Else if** under every branch, every card draggable including a
branch with its arms, the arrows on the card; the fork's stem, bar, corners, leads, joins and
after-stem as decision 20 draws them; the hint line zero-height and sticky.

**Acceptance:** a stop dropped in the chain is refused with its sentence; after a branch whose
arms both end nothing can be inserted and the end mark says *on every path*; *+ Else if* appends
a rung and the canvas draws a ladder that the reader rebuilds from the stored actions; a branch
is moved with its arms; at 1400 px and 375 px the fork's lines meet the cards (screenshots in
the pull request); `pnpm -r …` green.

## F8-18 — Conditions in Details, and the branch as a flow card **[L]**

*Depends on: F8-16. Issue #866.*

Decisions 16 and 18. The composer leaves the canvas: the gate and a branch draw their tree in
words with the *and* / *or* chips (`nodeWords`) and are edited in the *Details* tab through the
same `Composer`; a rung's Details says *Else if*. The branch takes the flow family's colour
everywhere (list, popover, card), the condition on it in the gate's notation; the gate alone
stays amber; `InsertMenu` and the blocks list offer no condition block. The findings a condition
carries (a label that is gone) stay at the card.

**Acceptance:** clicking the gate or a branch opens Details with the tree; the canvas has no
input in it; the compiled CEL on the write is unchanged for every fixture of F8-13; a branch's
colour is the flow family's in all three places; the e2e edits a branch's condition from
Details; `pnpm -r …` green.

## F8-19 — The check names a missing parameter and a runner without a role **[L]**

*Depends on: nothing. Issue #867.*

Decision 22. `Check.go`, `inspectActions`: for every action, a `Required` field of its
descriptor that is not in the run's supplied set and not in the params is
`automation.finding.parameter_missing` at `/actions/N/params/<field>`; `inspect`: the acting
account's memberships read through the existing port, none covering the scope is
`automation.finding.runner_without_role` at `/run_as`. Both `ATTENTION`. Table tests for each,
the catalogue sentences in `en` and `de`, `run_as_hint` reworded; the client draws the first at
the card (it already draws findings by path) and the second at the *Runs as* pill.

**Acceptance:** a rule with an `ADD_COMMENT` step and no `body` is found at the write and by the
check; a rule running as a service account without a membership is found at `/run_as`; the
e2e sees both at their places; `make verify` green.

## F8-20 — The third walk **[L]**

*Depends on: F8-15 to F8-19 and F8-21, and issue 814 fixed. Issue #868.*

The editor walked a third time on a local server with the round merged and 814 fixed — this
time the BROKEN path as well, since the notification no longer violates the key — and the
evidence appended to `docs/evidence/F8-2026-09-20.md` as a third section; the documents read
once more against the build (`automation.md` §1, the client's `CLAUDE.md`, the milestone's
Definition of Done). Anything the walk finds is an issue.

**Acceptance:** the third section exists with the BROKEN path walked; every issue of the round
closed by its pull request; the leftovers of decision 23 restated with what the owner decided
where he did.

## F8-21 — The list card's last run, and the runs page's line to Limits **[L]**

*Depends on: nothing. Issue #871.*

Decision 23, the two small halves. `AutomationRule` gains `last_run` (`{ at, outcome }`,
absent for a rule that never ran) — additive, read with the rule from the runs table's latest
row, so the list keeps costing one request; the list card says it in one line under the word
(`app.flow.last_run`, *never ran* when absent). The runs page draws one line under the strip:
this hour's runs against `automation_runs_per_hour` from `GET /quotas` — *34 of 500 this hour*,
*no limit* when the limit is null — and the link to *Limits* (`/administration/quotas`).

**Acceptance:** `GET /automation/rules` answers `last_run` for a rule that ran and omits it for
one that never did (contract and repository test); the card shows the line; the runs page shows
the hour's number and the link, and the e2e reads both; `make verify` green, `make generate`
clean, the catalogue complete.

## The fourth round — what the owner's own testing found

The third round was built, walked and merged; the owner then wrote rules with it himself and
came back with six things — one of them a concept fault (the guardrails set in two places), two
visual (a ring cut off, a ring overlapping), two about reaching what is there (a sheet whose
close scrolls away, conditions that take a second click each), and one about the hardest moment
of writing a rule: **finding out why the probe does nothing.** Nothing here reopens the concept;
all six are it, held to. Six decisions, five tasks, each one pull request in milestone 20.

## F8-22 — The guardrails in one place **[L]**

*Depends on: nothing. Issue #921.*

Decision 24. The guardrails card leaves `RuleCanvas`; the head gets a third chip that says
`on_error` and the throttle in the card's words and selects `{ kind: 'guardrails' }`, which the
view already maps to the *Rule* tab. `RuleInspector`'s standalone guardrails panel goes with the
card — the *Rule* tab is the one place — and `app.flow.card_guardrails*` keep their words, now on
the chip. The e2e presses the chip and sets a bound.

**Acceptance:** the canvas ends at the end mark and draws no guardrails card; the chip says the
same sentence the card said; pressing it opens *Rule* with the guardrails on it; nothing else
sets them; `pnpm -r …` green, the catalogue complete.

## F8-23 — The canvas: the ring fits, and empty space deselects **[L]**

*Depends on: nothing. Issue #922.*

Decisions 25 and 26. `box-sizing: border-box` on every card, the gate, the ladder, the rungs and
the folded line; the canvas's inline padding is at least the ring's offset and width, so a
selected card at the narrowest side-by-side width is drawn whole. A click on the canvas's
background clears the selection (`{ kind: 'none' }`), `Escape` on the canvas does the same; the
panel leaves *Details* for *Blocks* and stays where it is on *Probe* and *Runs*; on a narrow
screen the sheet closes. The *Details* tab with nothing selected keeps its sentence.

**Acceptance:** at 960 px a selected card's ring is inside the canvas on both sides (measured in
the pull request); clicking the background deselects and the panel shows *Blocks*; `Escape`
does the same; on a phone the sheet closes; `pnpm -r …` green.

## F8-24 — The sheet the reader sizes **[L]**

*Depends on: nothing. Issue #923.*

Decision 27. `Drawer` at `block-end` gains a sticky head — the panel is a column, the body the
only scroller — and, with `resizable`, a handle above it: pointer and keyboard (arrow keys, Home,
End) size the sheet between `0.33` and `0.9` of the screen, a drag let go below the floor closes
it, `size` is bindable so the caller keeps it. The story shows both. The rule editor opens the
sheet at half the screen and keeps what the reader dragged in `localStorage`, falling back in
silence where storage is refused.

**Acceptance:** the sheet's tabs and close stay visible while its body scrolls; the handle sizes
the sheet by pointer and by keyboard; dragging it to the bottom closes it; the size is there on
the next open of the same browser; `prefers-reduced-motion` is respected; the workbench story
exists; `pnpm -r …` green.

## F8-25 — The gate as one panel, a group from the first sentence, the rung's trash **[L]**

*Depends on: nothing. Issue #924.*

Decision 28. The gate's *Details* draws every condition, each with its *and*, a remove beside it
and *Add a condition* under them; a condition selected on the canvas still opens alone.
`Composer` renders its root as a group from the first sentence — both adds always, the mode
select from the second item — with `compileNode` unchanged, so no stored expression moves.
`removeRung` in `model.ts` with its table tests; the rung's head carries the trash; the rungs get
the room the selection ring needs; `card_folded` counts a ladder's rungs
(`app.flow.card_folded_ladder`).

**Acceptance:** the gate's Details edits and removes every condition without a second click on
the canvas; a fresh sentence offers both adds; the mode appears with the second item; every F8-13
fixture compiles to the same CEL; an *else if* is removed by its trash and its *otherwise* stays;
a folded ladder says its rungs; `pnpm -r …` green, the catalogue complete.

## F8-26 — What is missing, before the probe runs **[L]**

*Depends on: F8-15 (the manifest's `rule` flag and required fields). Issue #925.*

Decision 29. `review.ts`: a pure pass over the draft and the manifest answering a list of
`{ level, card, code, params }` — no trigger event, no schedule, no address minted, no runner, no
step at all, a required parameter that is empty and the run does not supply, an empty condition, a
branch with two empty arms, a step that needs the entry a scheduled run has not. The view merges
them into the marks the canvas already draws (a server finding at the same card wins), and the
*Probe* tab lists them above the button, each line selecting its card, with the sample's own
mismatch — an event other than the one the rule starts on — said there too. `app.flow.review_*`
in `en` and `de`, table tests per rule.

**Acceptance:** a new rule with an empty `ADD_LABEL` says so at the card and in the list before
anything is saved; a schedule feeding a step that needs an entry is named; the probe still runs
and the rule still saves; the list's line selects the card it is about; `pnpm -r …` green, the
catalogue complete.

## The order at a glance

```
F8-01 ──┬── F8-04 ──┬── F8-05 ─────────────────────┐
        │           ├── F8-06 ─────────────────────┤
F8-02 ──┼───────────┘         (F8-06 needs F8-02)  ├── F8-08
        │                                          │
F8-03 ──┴─────────────────── F8-07 ────────────────┘
                     (F8-07 needs F8-04 and F8-02)

F8-09 ─┐
F8-10 ─┤
F8-11 ─┼── F8-14          (the second round, after the owner's walk)
F8-12 ─┤
F8-13 ─┘

F8-15 ──── F8-16 ──── F8-18 ─┐
F8-17 ──────────────────────┼── F8-20      (the third round; #814 before F8-20)
F8-19 ──────────────────────┤
F8-21 ──────────────────────┘

F8-22 ─┐
F8-23 ─┤
F8-24 ─┼──                                 (the fourth round; F8-26 needs F8-15's fields)
F8-25 ─┤
F8-26 ─┘
```

Three tasks depend on nothing and can start at once: the three core tasks **F8-01**, **F8-02**
and **F8-03**. The editor **F8-04** waits for the manifest's fields and everything on the canvas
hangs from it; **F8-05** and **F8-06** are independent of each other; **F8-07** waits for the
check and the runs window as well; F8-08 is last by definition.

**Definition of Done for the milestone:** a rule is written on a canvas whose every card is a
piece of `automation.md` §1 and nothing else; its actions' forms come from the manifest and no
kind is compiled into the client; a condition is composed as a sentence and stored as CEL; the
rule's name follows the rule until somebody owns it; a piece can be dragged only to where it may
go, and everything a drag does the keyboard does; at phone width the canvas stays and the
details come to it; a sample entry can be watched through the rule before it is saved; the rule
says whether it works, and a rule that a deletion or an update would leave silently useless is
found, said, and — where it cannot run — switched off with a word to its author; every value
still comes from `tokens.json`; the Go tree builds with no Node.js installed; and the documents
say what the client does.
