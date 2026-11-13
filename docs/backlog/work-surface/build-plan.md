# Work surface — build plan (second build)

**State:** started 2026-11-13 on the branch `ux/work-surface-v2`, from `main` at `617bd76c5` plus the
released concept ([`README.md`](./README.md)) and its mockups ([`mockups/`](./mockups/index.html)),
nothing else. The first build (`ux/work-surface`, kept only as the local ref
`archive/work-surface-attempt-1`) was discarded by the owner: the concept was right, the way it was cut
into steps was not. This plan replaces the concept's §13 as the order of the build; everything else in
the concept stays binding.

## 1. What the owner asked for

* The concept built **completely and in detail**, as one coherent platform: the products, every product
  view and everything on it — menus, dropdowns, popups, dialogs, deeper pages — brought to the concept's
  structure, layout and UI/UX, not only the 59 drawn scenes.
* The mockups are the look. Where a view has no mockup, it follows the scene closest to it and the
  rules (WB-, ET-, S-, W-, B-, Q-, C-, F-, IN-, PO-, §7 devices, §8 what stays bound). Where a mockup
  contradicts another or the code, a working solution in the spirit of the whole is designed and built.
* Functionality is kept and made to work; nothing faked, nothing claimed that is not measured.
* Out of scope as before: the navigation (except *Beiträge*, B-2) and the layout of the configuration and
  administration pages (the page system).
* The end: a filled demo, and one file that shows every view old beside new to scroll through; then the
  owner's feedback before any pull request.
* The owner's later decisions of 2026-11-12 hold and are built into their areas: the desk template ships
  an automation rule that sets priority from impact × urgency (configurable); the desk home shows "my
  deadlines kept" for any number of deadline kinds over the handler's own tickets; lists, board columns
  and workflows follow the three levels with a column ↔ status mapping, with the one-off repair (§5).

## 2. Why the first build went wrong, and what this build does differently

| First build | This build |
|---|---|
| Cut by use case (17 steps), each worker built a slice end to end and moved on | Cut by **foundation, then families of screens**: the shared pieces first, then each family of views built whole, so the same pattern looks the same everywhere |
| Only drawn scenes were touched; the rest of each product stayed as it was | A **screen inventory** of every view, menu, dialog and deeper page in scope; each row names the scene or rule it follows and is ticked only when walked |
| The builder judged its own result; "kept differences" piled up | A **separate reviewer** compares each family's screenshots with the rendered mockups and the inventory; a difference stays only with a reason the orchestrator accepts |
| Workers spent their budget on bookkeeping and long test runs | Workers build; the e2e set runs at the end of a family, not per commit; documents are written once the family stands |
| Models were redesigned mid-build | The model decisions are settled before their family starts (§5) |
| No picture of "before" | Every in-scope screen is captured on `main` first, at three widths, with menus and dialogs open |

## 3. Phases

**Phase 0 — Groundwork (no product code).**
1. A fresh demo: its own PostgreSQL, migrated by `main`, seeded with the showcase (`make demo`), an empty
   workspace beside it, the story `TAMA-18` and the ticket `SA-12` filled to the edge as in the survey. A
   database dump of that state, so the "before" can be redrawn at any time.
2. The mockups rendered: every scene at 1440 (as drawn), 820 and 390, as PNG files beside a JSON index.
3. The screen inventory: every route, tab, menu, dropdown, popup, dialog and deeper page of the work
   surfaces in `apps/webapp`, each with the scene or rule it follows and the products it belongs to.
4. "Before": every inventory row captured on `main` at 1280, 820 and 390.
5. The e2e baseline on `main`: which suites already fail.

**Phase 1 — Foundation.** Tokens (`product.*`, `chart.categorical.1–7`, `layout.rail`, `layout.reading`,
`density.row.compact`); the design-system blocks of §5 built to the mockups' look and states: `ObjectHead`,
`StatusButton`, `StateLozenge`, `PropertyRail`, `ProductBand`, `Widget`, `Composer`, `DataTable` (groups,
density, hover act, stacked phone row with swipe), `QueryLine`, the jump marks, the self-folding section,
the sheet patterns for the phone. Each with stories for every state; reviewed against mockup crops.

**Phase 2 — Shared capabilities on the server, contract first.** The Hubtask query (one parser for both
spellings onto `FilterNode`, fields per product, counts, the doors API/MCP/`hubctl`/OData); allowed and
blocked transitions with reason codes and the read-only workflow picture (E-23); the list/board/workflow
levels with the column ↔ status mapping and the repair; status names for the portal; the side findings
of §10 (internal flag, OLA reassignment, OOXML upload, "0 on you", desk home). Each test-first, with a
cross-tenant test per new repository method.

**Phase 3 — Families of screens, each built whole, then reviewed.**

| # | Family | Scenes | Also every view of the family without a scene |
|---|---|---|---|
| 3.1 | Objects in detail | `grammar`, `task`, `wf`, `deskentry`, `ci` | goal, release, agent, portfolio, activity, work package; pane and page; every menu and dialog on them |
| 3.2 | Collections | `queue`, `board`, `viewcfg`, `sprint`, `backlog`, `qline` | outline, timeline, calendar, every collection kind, the queue's and the board's menus, bulk edit, capture in place |
| 3.3 | Find and create | `palette`, `search`, `create` | every "new …" button the concept retires or keeps |
| 3.4 | Homes | `today`, `hub`, `deskhome`, `homeplans`, `homeassets`, `homeagents`, `time`, `goals`, `emptygoals`, `emptytime`, `empty` | every product home's tabs and sub-pages, every empty state |
| 3.5 | Dashboards | `dashboard`, `dedit`, `wedit`, `wcat` | key results counting a query, an automation's scope line |
| 3.6 | The plan | `plan`, `gantt`, `planview`, `plantable`, `plandeps`, `planrel`, `plansprints` | the plan's dialogs, settings menu, exports |
| 3.7 | Pages and posts | `pread`, `pcover`, `pwrite`, `phist`, `phome`, `pnew`, `posts`, `post`, `postwrite` | page details, permissions dialog, templates, the *Beiträge* section |
| 3.8 | Whiteboards | `wboard`, `wsel`, `wsess`, `whome`, `wnew` | every panel and menu of the board |
| 3.9 | Files | `viewer`, `wbfiles` | files on every object; the document-server row |
| 3.10 | Portal | `portal`, `portaldir`, `portalext` | request form, request detail, approvals, knowledge |
| 3.11 | Inventory | `invlist`, `ci`, `invmap`, `invcare`, `invreg` | every former sub-page under *Pflege*, alerts, devices |

A family is done when: its inventory rows are walked at three widths beside their mockup or rule; the
reviewer's findings are fixed; its use cases are written or updated with checks proven by tests; `make
check` is green.

**Phase 4 — Coherence.** One reviewer walks the whole inventory across families for consistency (heads,
toolbars, menus, sheets, empty states, wording, spacing), the heights the concept measures (an entry ≤ 1.25
screens on desktop, Today ≤ 1.5 to the switch on the phone), keyboard and focus, both themes. Findings
fixed.

**Phase 5 — Proof and documents.** The whole e2e set; the subject documents of the concept's §12 item 6;
the three ADRs named to the owner; the use cases' `state:`, `checked_by:` and *Today* moved.

**Phase 6 — Presentation.** The demo filled and running; one HTML file with every inventory row — before,
after (and the mockup where one exists) — at the three widths, with what changed and why; then the owner's
feedback.

## 4. How the work is run

* One builder at a time on this branch (one checkout); read-only research and reviews may run beside it.
* Every builder gets: this plan, the concept, its family's rendered mockups and inventory rows, the rules
  of `AGENTS.md`. It commits by concern with the `Task: work-surface` trailer, pushes nothing, and writes
  where it stopped into §6 below.
* The reviewer is never the builder. It reports differences with screenshots; the orchestrator decides.
* The demo server is restarted only by its own PID; the demo's data is never discarded.

## 5. The list, board and workflow model (owner's decision of 2026-11-12)

Three levels per entry type in a collection, derived and never a setting: **L0** a list without columns
and workflow — a checkbox in the list, *Offen/Erledigt* in the entry; **L1** board columns without a
workflow — the columns are the states, exactly one done column, the server keeps done column and
completion together through every door, the list shows a column chip instead of a checkbox; **L2** a
workflow — the board keeps its own columns, each column shows one or more statuses (Jira-like mapping),
unmapped statuses in a folded *Ohne Spalte* column that accepts drops, a drop takes the one possible step,
asks among several, refuses none with a reason; the list shows the status chip. Switching levels always
previews (column ↔ status, *Workflow aus diesen Spalten*, back to a list keeps completion). One control in
the same place on the list row, the card, the entry head and the phone sheet. Owner's answers: open entries
standing in a done column are completed by the repair; unmapped statuses are shown folded, never hidden;
the L2 list chip shows the status; unused *Projekt* starting columns go (soft delete); every L1 board has
exactly one done column. The repair is a one-off, idempotent job with a dry run, run by an administrator.

## 6. Progress

| Phase | State | Where it stands |
|---|---|---|
| 0 Groundwork | started 2026-11-13 | — |
| 1 Foundation | built 2026-11-13, for review | Tokens (`product.<id>`, `chart.categorical.1–7`, `layout.rail`, `layout.reading`, `density.<mode>.row.height`) and wave 7 of `design-system.md` §4: `StateLozenge`, `StatusButton`, `ObjectHead`, `Property`, `PropertyRail`, `JumpMarks`, `FoldingSection`, `ProductBand`, `Widget`, `Composer`, `QueryLine` (a shell: language and catalogue handed in), `DataTable` with `kind="work"` (groups, density, hover act, stacked row, swipe). Stories per block and state, the gallery `Wave 7 · Work surface/Gallery`, `test/worksurface.test.js`. Comparison images in `.work-surface-walk/phase1/` (not committed). Open from the self-check: the stacked phone row draws the caller's check beside the title instead of in a column of its own, and the work table's rows stand a little taller than the mockup's 40 px because the checkbox and avatar set the height. Not yet: no product screen uses the blocks (Phase 3); the pages' and boards' blocks of §5 (`FloatingToolbar` … `BoardThumbnail`) belong to families 3.7 and 3.8. |
| 2a Server capabilities | built 2026-11-13, for review | Each test-first, contract additive. **Where an entry can go next (E-23, UC-FLW-06 new, UC-FLW-03 check 1 points to it):** `GET /items/{id}/steps` (`listEntrySteps`, `hubctl item steps`) answers beside `steps` (each now with `approval_levels`) the new `blocked` (key, name, to, `reason` `workflows.step_who_unmet` or `workflows.step_condition_unmet`, `roles`, `people` - the accounts the step's person fields name on this entry -, `groups`, `permission`), `done_route` (the fewest steps into a done status: key, name, to), `picture` (`WorkflowPicture`: statuses with name, category, colour token; `initial`; steps with from, to, `limited`, `approval`), `by_default` (no workflow: the picture is the shipped to do / in progress / done, every status reachable, and completing reaches done) and `may_design` (STRUCTURE where the entry is). A reader gets the picture, no steps and no blocked. The check stays in `WorkflowGate` (`Standing` splits offered and blocked). **Portal status names (P-12):** `PortalRequest` carries `status_name` (null where no workflow runs or the status has no name - the shipped workflow's - so the client words it by category) and `status_category`; `GET /requests`, `GET /requests/{id}` and the raise answer. **§10 side findings:** `Comment.internal` is set in every REST comment answer; OLA clocks hear `ItemAssigned`/`ItemUnassigned` (a handover through the assignment use case meets or arms the team's goals); a DOCX/XLSX/PPTX/ODF claim over a ZIP sniff is stored as claimed; the household seeding's "reading milestones: conn busy": a container read inside the configuration's apply transaction (`HubScope` reads the hub by `GetContainer`) counted its figures in that transaction, under a 300 ms deadline and with a late count in a goroutine - both break the caller's connection, proven against PostgreSQL; counts now run on their own (`persistence.Detached`, known-traps.md); the seeding itself was not rerun (the demo is not touched); a trigger's `changed_fields` now filters (it never did); the German preset words (`app.instance.presets_*`) said *Vorlage*, which failed `locales` on `main`. **Priority (owner, 2026-11-12, UC-SVC-02 check 14):** `itil_service_desk@17` ships `priority_on_create` and `priority_on_change` (narrowed to impact and urgency), switched on, editable rules (a desk taken before @17 is not checked here). **My deadlines kept (owner, 2026-11-12, UC-SVC-02 check 15):** `GET /deadlines-kept?days=` (`readMyDeadlinesKept`, `hubctl sla kept`), any member, always the caller's own assigned entries, default 30 days, at most 365: `from`, `to`, `data[]` of `kind` SLA/OLA, `goal`, `group_id` (OLA), `ended`, `kept` - the share is `kept / ended`, the client's arithmetic. **For the web families:** 3.1 draws the status menu from `steps` + `blocked` (blocked as one line of information, not a control - P-05, §12 item 5) and *view workflow* from `picture`, *open in designer* only on `may_design`, and *✓ Erledigt* where `by_default`; 3.10 shows `status_name`, else a word for `status_category`; 3.4 draws *my deadlines kept* from `/deadlines-kept`, groups named by `/groups`. **Not on the server, noted for the web families:** "Today showed 0 on you" - *Today*'s *mine* panel asks only entries with a due date (`lib/data/today.svelte.ts`, `MINE`), so two undated assigned entries count 0 (family 3.4); "no desk takes requests" - the desk home decides which collections are desks in the client (`desksOf` in `lib/servicedesk/queues.ts`, from the collections' offered task types), not from the server (family 3.4). **Open, for the owner:** "OLA clocks follow a change of responding group" - an OLA is armed and met by the assignee's membership of its group (UC-SVC-02 checks 10, 11); following the ITIL desk's `responding_group` field instead or as well changes what those checks say, so it is not built. |
| 2b The Hubtask query | language built 2026-11-13, for review; search over every kind not started | **Built (test-first):** one parser in the domain for both spellings (`view.ParseQuery`: `field: value` with comma/space/`-`, `< <= > >=`, `a..b`, `( … OR … )`, `NOT`, free words and phrases, `today` `-7d` `+2w` `this-week` `last-month` `overdue`, `me` `none`, `type:`, `sort:`, `is: open/done/blocked/archived/trashed`, `cf.<key>`, `desk.sla/standing/satisfaction`; and the `=`/`AND`/`ORDER BY` line) onto `FilterNode` + sort, `view.FormatQuery` writing the canonical text back; errors are codes with `at` and `suggestion`. Shared lines `test/querylanguage/lines.json` (70) held by `test/querylanguage/lines_test.go` and `packages/query/test/lines.test.ts`; a fuzz test of read → write → read. Keys served as `query_keys` in `/meta/capabilities` (only keys the server reads). `POST /items:query` reads `q` in both spellings (REST, MCP, `hubctl item query` - one use case), `POST /items:aggregate` takes `q` (the count while typing: `scope.workspace` + `COUNT`; the values with counts: `group_by` the key's field - permission-scoped per reader), `POST /search` takes `query` (words ranked, rest narrows; `hubctl` gained the flag). Browser twin `packages/query` (`parseQuery`, `formatQuery`, `createLanguage`, `createCatalogue`, `countRequest`, `valuesRequest`, `KEYWORDS`). UC-WRK-43 (specified), ADR-0102 (amends ADR-0026, ADR-0066), `api-guidelines.md` §3.1. **For the web families:** 3.2 and 3.3 hand `QueryLine` `createLanguage(wording)` and `createCatalogue(capabilities.query_keys, wording, counted)` - wording from the locales (key names, value names, `problem` from the error code and its params, rule 8) - send `countRequest(line, scope)` debounced and `valuesRequest(key, line, scope)` through the sync engine (POST bodies only, UC-WRK-18/4), replace `lib/data/searchquery.ts` by the package and resolve `in: here` and names (people, labels, collections) to identifiers before sending; saved views keep the tree. **Not built, left for the next builder:** search over every kind (E-11: pages, boards, goals, dashboards, inventory, people, file names, comments - each a sqlc query with Testcontainers, a cross-tenant and a permission test), verified pages first (E-8), finding #1491 (one embedding per request), OData `$filter` accepting the query (P-08), keys for `plan.`, `asset.`, `page.`, `post.`, `goal.`, `time.` and `hub:`, `under:`, `my-groups`, `members(…)`, `affects:` (the parser refuses them as unknown rather than faking them), UC-WRK-17/18 and UC-INT-10/15 updates, Q-6. |
