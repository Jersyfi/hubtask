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
