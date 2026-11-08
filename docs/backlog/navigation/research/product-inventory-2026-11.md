# Hubtask product inventory for the hub-level navigation concept

Branch: `claude/hubtask-navigation-concept-c5bdff` (= main at 5d4ca68f3). Read-only research,
2026-11-07. Every claim carries `file:line`. `api/openapi.yaml:<n>` points at the method line of
the operation (the `operationId` follows within two lines). Permissions are the use-case
descriptor's `Permission` (core/application/service/…) or, where the descriptor decides it at
run time, the operation's own description in `api/openapi.yaml`. **[unverified]** marks what I
could not confirm in code.

Permission vocabulary: READ, WRITE_ITEMS, STRUCTURE, MANAGE_MEMBERS, AUTOMATION, DELETE_CONTAINER,
READ_CONFIGURATION, AUDIT_READ (core/domain/service/Authorization.go:27-62), plus COMMENT,
WRITE_ASSIGNED, REQUEST, APPROVE_TIME, MANAGE_COSTS (core/domain/service/PermissionCatalogue.go:19-33).
A permission is asked along a scope path tenant → hub → (collection); a hub's own scope is in its
path, so STRUCTURE held *on a hub* is enough to rename/reorder/archive that hub
(core/domain/service/ContainerScopes.go:19-28).

Modules (the switchable products): WORKFLOWS, PLANNING, KNOWLEDGE, WHITEBOARD, SERVICE_DESK, ASSETS,
TIME, GOALS, AGENTS, in this fixed order (core/domain/model/module/Module.go:25-36). Off by default;
levels instance → plan → workspace → **hub**; a hub may only switch *off* what the workspace has on
(Module.go:70-90; api/openapi.yaml:19143 `setModule`). **There is no collection-level module switch**:
`listContainerModules` answers, for a collection, the modules of its hub
(api/openapi.yaml:19522). SERVICE_DESK needs WORKFLOWS (Module.go:99-104).

---

## 0. Hubs and collections (the frame everything hangs on)

### 0.1 Objects and scope
* `Container` with `type` HUB | COLLECTION; `parentId`: HUB → null, COLLECTION → a HUB
  (docs/architecture/domain-model.md:663-670). Two levels only — "a collection holds entries, not
  collections" (docs/usecases/work/UC-WRK-01-…md:57-58).
* **A collection cannot exist without a hub**: I-C1 (domain-model.md:681); create refuses
  `containers.collection_needs_parent` (core/domain/model/work/Container.go:235-236), a hub with a
  parent `containers.hub_has_no_parent` (Container.go:231-232).
* Fields relevant to navigation: `name` (unique per level, case-insensitive, NFC), `icon`,
  `color_token`, `order_key` (fractional index among siblings), `archived_at`, `effective_archived`,
  `deleted_at`, `private` (HUB only), `handle`, `key_prefix`, `structure`, `figure`, `milestone`
  (domain-model.md:665-678; api/openapi.yaml:34200-34276).
* `private` only on a HUB; a collection shares its hub's privacy (I-C5, domain-model.md:685).

### 0.2 Order, lists, pagination
* Hubs among the workspace's hubs and collections among their hub's are in **manual order by
  `order_key`** (domain-model.md:674). `ListContainers` reads one level at a time (no parent → hubs,
  a parent → its collections), keyset `(order_key, id)`, `COLLATE "C"`, trashed rows never, archived
  only with `include_archived` (db/queries/Work.sql:89-112).
* `GET /containers` (`listContainers`, api/openapi.yaml:19327): params `type`, `parent_id`,
  `cursor`, `size` (1–200, default 50, api/openapi.yaml:24793-24796), `include_archived`; answer
  `ContainerPage {data, page}` (api/openapi.yaml:24923-24932).
* No bound on hubs per workspace or collections per hub found in core/domain or quotas
  (db/queries/Quota.sql has no container count) **[searched, none found]**. Load target: 10⁶
  entries per workspace (docs/backlog/suite/products.md:37-38).

### 0.3 Operations
| Act | operationId (line) | Permission | Notes |
|---|---|---|---|
| Create hub / collection | `createContainer` (19349) | STRUCTURE at the parent's path: a hub needs it at the workspace, a collection at the hub (work/CreateContainer.go:381-388, 511-515; descriptor :615-618) | A new container is ranked after its last sibling (CreateContainer.go:190, `nextOrderKey`) |
| Create private hub | `createContainer` with `private: true` | Any signed-in **person** who holds any role in the workspace (access/PrivateHub.go:24-49), only where the workspace allows private hubs (`containers.private_hubs_off`, CreateContainer.go:63-75); creator becomes OWNER (CreateContainer.go:404-411) | `containers.private_only_hubs` on a collection (CreateContainer.go:174-177) |
| Hub from template | `createHubFromTemplate` (19377) | STRUCTURE (work/CreateHubFromTemplate.go:91) | Makes the hub **plus exactly one collection named as the hub** (CreateHubFromTemplate.go:46-50, 79; api/openapi.yaml:19383-19390) |
| Rename / icon / colour | `renameContainer` (19434) | STRUCTURE (work/UpdateContainer.go:455-457) | |
| Policies / structure | `updateContainerPolicies` (19488), `setContainerStructure` (19617) | STRUCTURE | |
| Reorder within level (hub **or** collection) | `reorderContainer` (19774) | STRUCTURE on the container (work/ReorderContainer.go:201-214) | Only way to rank a hub (api/openapi.yaml:19783-19795); archived refused (I-C4, domain-model.md:684) |
| Move collection to another hub | `moveContainer` (19729) | STRUCTURE at destination **and** source (work/MoveContainer.go:99-110, 325-328) | A hub is refused `containers.hub_not_movable` (Container.go:784-789); naming the current hub = reorder (MoveContainer.go:31-35) |
| Archive / unarchive | `archiveContainer` (19859), `unarchiveContainer` (19888) | STRUCTURE (work/ArchiveContainer.go:99-101, 149-151) | Archived = read-only, children inherit `effective_archived` (I-C3, domain-model.md:683) |
| Trash / restore | `trashContainer` (19466), `restoreContainer` (19829) | DELETE_CONTAINER — "the owner's right", not an administrator's (work/TrashContainer.go:348-350, 377-379; Authorization.go:37-39) | Cascading soft delete, one `trashBatchId`, 30 days (I-C2, domain-model.md:682; api/openapi.yaml:19468-19475) |
| Make private / shared | `setHubPrivacy` (19692) | DELETE_CONTAINER descriptor (work/SetHubPrivacy.go:189); description: create-hub right + OWNER on hub (api/openapi.yaml:19694-19700) | Refused `containers.private_hub_has_groups` while a group holds a role |
| Admin list of private hubs (no names) | `listPrivateHubs` (14403) | MANAGE_MEMBERS (work/ListPrivateHubs.go:181) | Emergency open `openPrivateHub` (14452): DELETE_CONTAINER |
| Star / favourite / pin | — | — | **No such operation or field in the API** (grep of api/openapi.yaml for star/favourite/pinned: none) |
| Duplicate a hub/collection | — | — | **No container copy operation** in the API (op list); entries only (`duplicateWorkItem`) |

**Consequences / refusals of `:move` (collection → other hub):**
* Name must be free at the destination: `containers.name_taken` (MoveContainer.go:175; UC-WRK-01 check 3, UC-WRK-01:37-38).
* `entry_types.hub_bound` (409) while an entry of the collection carries a type defined at the old hub
  (MoveContainer.go:133-140; work/EntryTypeGate.go:98-101).
* `workflows.hub_bound` (409) while the collection's own scheme row runs a workflow defined at the old
  hub (MoveContainer.go:142; EntryTypeGate.go:103-131).
* Destination archived/trashed refused (`EnsureAcceptsChildren`, Container.go:801-803);
  `containers.parent_is_self` (Container.go:793-799); `containers.before_container_not_in_level`
  (MoveContainer.go:254-257).
* Effects: nothing under the collection is rewritten (MoveContainer.go:37-39); devices that lose
  read access are sent revocations (MoveContainer.go:42-44); it inherits the destination hub's
  structure (MoveContainer.go:45-48; Container.go:823-826).
* **[unverified]** what happens to hub-scoped objects that point at the collection's entries after a
  cross-hub move: releases (`release_entry` holds "entries of the hub's collections",
  api/openapi.yaml:602), hub SLA policies (measure "only the hub's entries", domain-model.md:839-845),
  hub calendars, portfolios. I found no subscriber of `ContainerMoved` besides access revocations
  (grep: core/application/service/access/Revocations.go only).

### 0.4 Webapp
* Tree band = hubs and their collections (docs/architecture/user-experience.md:59-65); drag or
  Alt+↑/↓ in the tree sends `:reorder` for a hub and `:move` (same hub) for a collection
  (apps/webapp/src/lib/frame/WorkspaceNav.svelte:360-371). An expanded hub additionally shows its
  product tabs (except Overview) as quieter rows after a hairline (WorkspaceNav.svelte:271-284;
  user-experience.md:132-134), ids `<placeId>/tab/<tab>` (apps/webapp/src/lib/workspace/placeTabs.ts:104-111).
* Place menu (head and tree row identical): Settings…, Move to hub (collection only), Import,
  Change position, Archive/Unarchive, Make private/shared, Trash (apps/webapp/src/lib/workspace/placeMenu.ts:28-50).
* Routes: `/hubs/:id` (`hub`), `/collections/:id` (`collection`), `/hubs/:id/settings[/:section]`,
  `/collections/:id/settings[/:section]` (apps/webapp/src/lib/routes.ts:179, 216-222), drawn by
  `ContainerView` (apps/webapp/src/App.svelte:564-573). Settings sections — HUB: general, modules,
  template, types, workflow, views, rules, calendars, code, chat, people; COLLECTION: general,
  template, types, fields, labels, templates, workflow, views, rules, people
  (apps/webapp/src/lib/workspace/placeSections.ts:32-35).
* Hub Overview: collection cards with figures, product lines (Plan, Pages, Assets, Whiteboards, Desk,
  Time, Goals), *Recently changed* (user-experience.md:925-941).
* `/archive` lists archived hubs and collections (`include_archived` per hub); archived entries are
  found through search `is:archived` (apps/webapp/src/views/ArchiveView.svelte:6-21). `/trash` is
  one trash for CONTAINER, ITEM, PAGE, WHITEBOARD (api/openapi.yaml:9549-9580).

### 0.5 The place tab strip (what a hub/collection shows per product)
Hub: *Overview* · Plan (sub-views Plan, Releases) · Pages · Whiteboards · Desk · Assets · Time
(Actuals, Workload) · Goals — each only if its module is on at that hub
(placeTabs.ts:33-56; user-experience.md:105-116). Goals tab is `/goals?hub=<id>` (placeTabs.ts:53-55).
Collection: *Entries* · Plan · Sprints (only if `runs_sprints`) · Portal (only if it acts as a desk) ·
Time (placeTabs.ts:58-67). `isDesk` and `runsSprints` are computed client-side per collection
(apps/webapp/src/lib/workspace/PlaceStrip.svelte:48-63).

### 0.6 Products band (workspace level)
`PRODUCTS` rows: plans `/plans` (PLANNING), pages `/pages` (KNOWLEDGE; also owns `notes`),
whiteboards `/whiteboards`, desk `/desk`, assets `/assets`, time `/time`, goals `/goals`
(goals, goal, portfolio, dashboard routes), agents `/agents` (apps/webapp/src/lib/navigation.ts:183-192).
WORKFLOWS has no home (user-experience.md:78). Approvals is a primary row `/approvals` with module
SERVICE_DESK (navigation.ts:99). Product homes drawn by the one `ProductHomeView` for
plans/pages/whiteboards/desk/assets/agents; time and goals have own pages
(apps/webapp/src/lib/frame/moduleroute.ts:24-37). Each home also lists **the hubs where the module
is on**, each linking to the hub's product screen (apps/webapp/src/views/ProductHomeView.svelte:448-470;
apps/webapp/src/lib/product/home.ts:58-74).

---

## 1. Core work: entries, buckets, views, templates, inbox

### 1.1 Objects and scope
* `WorkItem` levels TASK → WORK_PACKAGE → ACTIVITY, `MAX_DEPTH` 3/2/1 relative to the collection
  (domain-model.md:40-58); `collectionId`, `parentId` (TASK → null), materialised `path`,
  `orderKey` "rank within the bucket or the parent item" (domain-model.md:689-703).
* `Bucket` (board column) per collection, `orderKey`, `wipLimit`, `isDoneBucket` (domain-model.md:741);
  `Label` per collection (domain-model.md:742).
* `SavedView`: scope personal or shared into a container path (collection, hub, workspace); layouts
  LIST_COLLAPSED, LIST_EXPANDED, KANBAN, TIMELINE (domain-model.md:753; api/openapi.yaml:6286-6320).
  A template's hub part names the default view (configuration.md:287-289).
* Routine `Template` (entry tree) per collection/hub/workspace scope (domain-model.md:752;
  api/openapi.yaml:6081-6090 `listTemplates`).
* **Jumble / Inbox**: `JumbleEntry` is the **workspace's** (tenantId, channel EMAIL/WEBHOOK/QUICK_CAPTURE/API,
  status NEW/PROCESSED/DISMISSED) (domain-model.md:754); list needs READ at the workspace
  (core/application/service/jumble/Descriptors.go:65). Route `/jumble` (routes.ts:150).

### 1.2 Operations (entries)
`createWorkItem` (19940), `updateWorkItem` (19990), `reorderWorkItem` (20648), `moveWorkItem` (20690),
`duplicateWorkItem` (20746), `archiveWorkItem` (20591), `trashWorkItem` (20016), `restoreWorkItem`
(20560), `mergeWorkItems` (20823), `splitWorkItem` (20861), `bulkUpdateWorkItems` (20917) — all
WRITE_ITEMS (desc table: work/MoveWorkItem.go:778, 836; DuplicateWorkItem.go:1138; TrashWorkItem.go:142,168;
ArchiveWorkItem.go:473). `:move` takes `target_collection_id`, may cross hubs; WRITE_ITEMS asked at
destination and source (work/MoveWorkItem.go:238-262); refused `entry_types.not_offered` when the
destination does not offer the type (api/openapi.yaml:20697-20700); references that do not resolve are
dropped and reported (I-W6, domain-model.md:721). Buckets: `createBucket`/`updateBucket`/`reorderBucket`/
`deleteBucket` (21200-21285), STRUCTURE (work/CreateBucket.go:367, UpdateBucket.go:556).
Jumble: `convertJumbleEntry` (6686), `dismissJumbleEntry` (6766).

### 1.3 Who / jobs
PE-person, PE-member (docs/vision/personas.md:14-15); UC-WRK-01…35 (docs/usecases/work/).

---

## 2. WORKFLOWS (types & workflows)

* Objects: `EntryType` (per level; workspace or **hub-bound** with `hub_id`), `Workflow` (workspace or
  hub-bound), the scheme rows (which hub/collection runs which workflow for which type)
  (api/openapi.yaml:17667 `listEntryTypes`, 17876 `listWorkflows`). `LinkType` workspace-wide
  (domain-model.md:747).
* No nesting; listed "by level and order" (api/openapi.yaml:17669-17672).
* Ops: `createEntryType` (17689) STRUCTURE at workspace, or at hub with `hub_id`; `copyEntryTypeHere`
  (17771) STRUCTURE at hub; `createWorkflow` (17897) same rule; `copyWorkflowHere` (17984);
  `saveWorkflowDraft`, `previewWorkflowPublish`, `publishWorkflow`; `setWorkflowScheme` (18131)
  STRUCTURE where the container is, `workflows.hub_bound` outside the hub.
* Webapp: no product home (user-experience.md:78); screens are administration `/administration/types`
  and `/administration/workflows` (navigation.ts:291-292; routes.ts:113, 122) and the place settings
  sections *Types & forms* / *Workflow* (placeSections.ts:32-35).
* Who: PE-admin; UC-FLW-01…05 (docs/usecases/workflow/).
* Navigation relevance: hub-bound types/workflows are the main thing that pins a collection to its
  hub (§0.3).

---

## 3. PLANNING (plans, sprints, releases, baselines; roadmaps)

### 3.1 Objects and scope
* **A plan is not an object**: "a plan is a hub or a collection, its entries those neither trashed nor
  archived" (domain-model.md:856-857); `planId` in `/plans/{planId}/…` is a container id. Hub plan and
  collection plan both exist (routes.ts:199-200).
* `Baseline` per plan, ≤20, never changed (domain-model.md:867-869).
* `Sprint` — **per collection**, one ACTIVE per collection, PLANNED → ACTIVE → CLOSED, ≤92 days
  (domain-model.md:870-874; core/domain/model/planning/Sprint.go:40-43); a hub has none
  (`sprints.collection_required`, api/openapi.yaml:677-680). ≤1000 entries/sprint (Sprint.go:42).
* `Release` — **per hub**, PLANNED → RELEASED, ≤200 entries; a collection has none
  (`releases.hub_required`) (domain-model.md:877-882; api/openapi.yaml:475-479).
* Roadmap = a **portfolio's** roadmap (GOALS module objects), `/portfolios/:id/roadmap` (routes.ts:171-172;
  api/openapi.yaml:1421 `readPortfolioRoadmap`).

### 3.2 Order, pagination
Sprints: latest first (`start_on DESC`), ≤200 per list (db/queries/Sprint.sql:61; api/openapi.yaml:658-661).
Releases: latest day first, ≤200 (db/queries/Release.sql:44; api/openapi.yaml:458). Running sprints of
me: soonest end first, ≤20 (api/openapi.yaml:751-755). No pagination cursor on these lists.

### 3.3 Operations
`readPlanSchedule` (110, READ), `applyPlanSchedule` (137), `importPlan` (170, per collection),
`takeBaseline` (325, WRITE_ITEMS), `deleteBaseline`; sprints `createSprint` (675), `updateSprint`,
`deleteSprint`, `startSprint` (842), `closeSprint`, `addSprintEntries` (892) — all WRITE_ITEMS
(planning/Sprints.go:213, 317, 708, 1033, 1215); releases `createRelease` (473), `updateRelease`,
`deleteRelease` (hard delete, "a release has no trash", user-experience.md:842-844),
`publishRelease`, `addReleaseEntries` (602) — WRITE_ITEMS (planning/Releases.go:199, 286, 500, 914).
Refusals: `sprints.entry_in_another_sprint`, `releases.entry_in_another_release`. No move of a sprint
or release to another place, no archive, no copy (op list).

### 3.4 Webapp
* `/plans` (ProductHomeView, PLANNING): running sprints (≤8), releases due (≤8), hubs list
  (ProductHomeView.svelte:123-132, 359-395). §2.2's *Roadmaps as a tab* (user-experience.md:79) is
  **not built**: `homeTabs('PLANNING')` returns no tabs (home.ts:20-52).
* Hub: `/hubs/:id/plan` (PlanView), `/hubs/:id/releases` (ReleaseView); collection:
  `/collections/:id/plan` (PlanView), `/collections/:id/sprints` (SprintView) (App.svelte:511-549).
* Collection "runs sprints" = `runs_sprints` of `SprintList`: a sprint was planned there, or the
  collection or its hub took a template that works in sprints (Scrum team) (api/openapi.yaml:29670-29680).
* Who: PE-planner (personas.md:18); UC-PLN-01…09.

---

## 4. KNOWLEDGE (pages, spaces, notes)

### 4.1 Objects and scope
* **A hub is a space** — exactly one space per hub; there is no separate Space object and a hub
  cannot hold several (domain-model.md:943; api/openapi.yaml:22490-22499 "The hub's space").
  Collections hold no pages.
* `Page` tree: `parent_id`, fractional `order_key` among siblings, **≤12 levels, ≤5,000 pages per
  space** (domain-model.md:943-944; core/domain/model/knowledge/Page.go:36-39). Judged at the hub:
  READ to read, WRITE_ITEMS to write (domain-model.md:945-946).
* A page may be a **template of its space** (`template` flag; space templates = a page with subpages,
  ≤100 per write) (domain-model.md:955, 993-995; Page.go:42). Hub template pages are separately
  tracked: `listHubTemplatePages`/`addHubTemplatePage` (api/openapi.yaml:22660-22677), STRUCTURE
  (config/TemplatePages.go:37, 86).
* Restriction narrows readers per page; `PageShare` per person; `PublicLink`; `Article` = page
  published to a desk collection's portal; `LanguageVariant` (domain-model.md:961-978).
* **Personal notes** = `personal_space`, a private hub of the person's own made on first use; daily
  notes one per day (domain-model.md:982-984; api/openapi.yaml:24014-24036).
* Drafts mode per space (`knowledge.drafts`) (domain-model.md:987-993).

### 4.2 Order, pagination
`listPages` returns **the whole tree** without bodies, parents before children, no pagination
(api/openapi.yaml:22490-22499; ORDER BY `parent_id NULLS FIRST, order_key, id`, db/queries/Page.sql:55).
Archived only with `include_archived`; templates alone with `templates`.

### 4.3 Operations (all WRITE_ITEMS on the hub unless noted)
`createPage` (22516; Pages.go:339) with `parent_id`, `template_id`, `with_subpages`, `subpages`
(api/openapi.yaml:41265-41290); `updatePage` (22796) — title, body, icon, template mark, language,
**`archived`** (api/openapi.yaml:22802-22806, 41305-41311); `movePage` (22846) — under another page
of **the same space** or top, after a sibling: refusals `pages.parent_cycle`,
**`pages.parent_other_space`** (no cross-hub move), `pages.too_deep` (api/openapi.yaml:22852-22856);
`copyPage` (22878) — **same space only**, no "Copy of" (api/openapi.yaml:22884-22890); `trashPage`
(22824) takes subpages; `restorePage` (22921); `verifyPage`, versions, comments, labels, restrict,
share, publish (drafts), public link, portal article, `exportPage`, `searchPages` (22944-23283).
No star/favourite.

### 4.4 Webapp
* `/pages` (ProductHomeView KNOWLEDGE) tabs *Spaces* · *My notes* (`/notes`) (home.ts:45-50); lists
  pages to review, recent pages (≤10), hubs (ProductHomeView.svelte:115, 396-433).
* `/hubs/:id/pages` and `/pages/:id` → `PagesView` (App.svelte:482-494); a page's address names only
  the page (routes.ts:180-183). Three regions: tree, page, comments sheet (user-experience.md:667-674).
* Who: PE-member, PE-person; UC-DOC-01…15 (docs/usecases/knowledge/).

---

## 5. WHITEBOARD

* `Board` per **hub**, ≤1,000 outside the trash (domain-model.md:999; core/domain/model/whiteboard/Board.go:29);
  ≤5,000 objects per board (domain-model.md:1000). READ / WRITE_ITEMS at the hub.
* **No nesting, no folders, no manual order**: listed last changed first (`updated_at DESC`,
  db/queries/Whiteboard.sql:48), cursor + `limit` ≤1000 (api/openapi.yaml:24067-24090).
* Ops: `createWhiteboard` (24100, WRITE_ITEMS, Boards.go:94) — empty, a template's objects, or an
  export's elements; `updateWhiteboard` (24148) rename or save scene; `trashWhiteboard` (24175),
  `restoreWhiteboard` (24197) (WRITE_ITEMS per description); `exportWhiteboard`, versions, comments,
  backlinks. **No move to another hub, no copy op, no archive** (op list). "Copy" only via
  export → create with `elements`.
* Templates are a client gallery: Blank, Retrospective, Brainstorm, User story map, Kanban sketch,
  Flowchart (user-experience.md:710-717).
* Webapp: `/whiteboards` home = boards as preview cards, recent first (≤12) (user-experience.md:81;
  ProductHomeView.svelte:110, 433-445); `/hubs/:id/whiteboards`, `/whiteboards/:id` → `WhiteboardsView`
  (App.svelte:495-506).
* Who: PE-member; UC-CNV-01…09.

---

## 6. SERVICE_DESK (desks, queues, portal, approvals, SLAs)

### 6.1 Objects and scope
* **A desk is a collection**, not an object. A collection "acts as a desk" when the types it offers
  (its own or its hub's `structure.entry_types`) include one whose form leaves the PORTAL channel
  open; a collection offering every type is no desk (apps/webapp/src/lib/portal/portal.ts:156-166;
  user-experience.md:115-122). Requester holds REQUEST at the collection (domain-model.md:920-927).
* Per collection: portal catalogue (`listRequestCatalogue`, api/openapi.yaml:4087), `PublicForm`
  (one per collection, domain-model.md:928-933), desk logo (`getDeskLogo` 19546), mail `handle`
  (`setCollectionHandle` 7040, STRUCTURE, mail/Descriptors.go:70).
* Queues are **saved views**, not objects: the ITIL template's hub part brings views `mine`,
  `unassigned`, `about_to_breach`, `unhappy` (core/application/service/config/shipped/itil_service_desk.json, hub.views);
  a group's queue is the query field `group_queue_of` (user-experience.md:259-262).
* Workspace-or-hub: `SlaPolicy` (hub's own with `hub_id`, fixed hub, tried first) (domain-model.md:827-845),
  business calendars (workspace; hub keeps its own version, `defineHubCalendar` STRUCTURE on hub,
  api/openapi.yaml:3565-3572). Workspace-only: reply templates, requester organisations, SLA report
  (STRUCTURE) (api/openapi.yaml:3718, 3851).
* Per person: approvals waiting for me (`listMyApprovals` 3945, cursor, limit 50) and delegation.

### 6.2 Operations
`defineSlaPolicy` (3636)/`deleteSlaPolicy`/`copySlaPolicyHere`; `raiseRequest` (4221, REQUEST);
`decideApproval`; `defineBusinessCalendar`/`defineHubCalendar`; reply templates; organisations. No
"create desk" operation: a desk is made by a service template (§12) or by offering a portal type.

### 6.3 Webapp
* `/desk` (DeskHome inside ProductHomeView) tabs *Queues* · *Desks* (`/desk/desks`, desks across hubs) ·
  *Settings* (`/desk/settings`) (home.ts:21-27; App.svelte:463-464); queues = Mine, Unassigned in my
  groups, About to breach, All open (user-experience.md:82, 767-786). Each row reads `/items/{id}/sla`,
  at most 25 a hub (user-experience.md:777-778).
* Hub tab `/hubs/:id/desk` → `HubDeskView`: the hub's desk collections' queue (routes.ts:195-196;
  App.svelte:507-509). Collection tab *Portal* → `/portal/:id` (placeTabs.ts:65).
* Portal shell for requesters: `/portal/:id`, `/portal/requests/:id`, `/portal/:id/articles/:pageId`
  (routes.ts:188-192; user-experience.md:294-301).
* `/approvals` is a primary-band row, not under the desk (navigation.ts:99; user-experience.md:770-771).
* Who: PE-handler, PE-requester, PE-admin (personas.md:19-20, 27); UC-SVC-01…13.

---

## 7. ASSETS (CMDB)

* Per hub inventory of `CI`s, **≤20,000 per hub** (domain-model.md:1040; core/domain/model/cmdb/CI.go:19).
  READ / WRITE_ITEMS at the hub. A CI is not an entry; no collection (domain-model.md:1055).
* Workspace-wide: CI classes (single inheritance, shipped hierarchy + own), relation types, sources,
  software aliases (api/openapi.yaml `/cmdb/classes`, `/cmdb/relation-types`, `/cmdb/sources`).
* Per hub: relations (only between two CIs **of one hub**, domain-model.md:1056), connectors ≤50
  (cmdb/Connector.go:41), groups ≤200 (cmdb/Group.go:18), campaigns, alerts, runs/holds/duplicates,
  service map, software count, health (`/hubs/{hubId}/…`, api/openapi.yaml:4494-6019).
* Nesting: class hierarchy and relations (graph), not a tree of CIs. Listing: stable order, `after`
  cursor + `limit` ≤500, class filter (api/openapi.yaml:4496-4507).
* Ops: `createCi` (4516, WRITE_ITEMS, cmdb/Items.go:212), `updateCi` (WRITE_ITEMS, Items.go:274),
  `reconcileCis`, `importCiFile`, `relateCis`/`unrelateCis`, `resolveCiDuplicate` (merge), asset
  lifecycle `moveCiAsset` (lifecycle stage, **not** a move between hubs). **No CI delete** ("no use
  case deletes a CI", domain-model.md:1066), no trash, no move to another hub (op list). Connectors are
  switched off, never deleted (domain-model.md:1078).
* Webapp: `/assets` home tabs *Inventory* · *Settings* (home.ts:38-44); inventory across hubs + by class
  + health (ProductHomeView.svelte:328-355); `/hubs/:id/assets[/:section]`, `/cis/:id` → `AssetsView`
  (App.svelte:551-563). Hub strip: Inventory · Health · Alerts · Groups and map · Manage (Runs,
  Duplicates, Connectors, Campaigns, Register, Software, Import) (user-experience.md:731-735).
* Who: PE-admin, PE-auditor, PE-handler; UC-AST-01…13.

---

## 8. TIME

* Per person: `Record` (minutes on one day, on an entry or none), one `Timer` per person, `Timesheet`
  per person-week, `Capacity`, `Absence` (domain-model.md:883-911). Workspace: `WorkType`, `Policy`,
  `CostRate`. Per plan (hub or collection): `Budget`, `BillingRate`, actuals, workload
  (domain-model.md:914-919; api/openapi.yaml `/plans/{planId}/actuals|workload|budget|billing-rates`).
* Lists: `listTimeRecords` ≤2000, earliest first (api/openapi.yaml:2362-2368).
* Ops: `logTime`, `updateTimeRecord`, `deleteTimeRecord`, `correctTimeRecord`, timer start/stop,
  timesheet submit/approve/reject (APPROVE_TIME or MANAGE_MEMBERS), `setBudget` (MANAGE_COSTS or
  MANAGE_MEMBERS) (api/openapi.yaml:2362-3026).
* Webapp: `/time` own page, tabs week · (weeks to decide) · report · absences · capacity · settings
  (apps/webapp/src/views/TimeView.svelte:69-81; user-experience.md:866-867); hub/collection tab *Time*
  = `/…/actuals` and `/…/workload` (placeTabs.ts:70-78; App.svelte:535-542).
* Who: PE-member, PE-planner; UC-TIM-01…08.

---

## 9. GOALS (goals, portfolios, dashboards)

* `Period` workspace-wide (STRUCTURE to define, api/openapi.yaml:1005-1007). `Goal` belongs to a period
  and to **the workspace or one hub**; parent goal ≤8 deep; ≤20 key results; **the hub stays** on update
  (domain-model.md:1095-1103; api/openapi.yaml:1160-1163). Listed in creation order
  (db/queries/Goal.sql:48-57); filters period, hub, workspace_only, owner, parent (api/openapi.yaml:1094-1105).
* `Portfolio` **workspace-only**, ≤200 hubs/collections as projects, nests ≤6 deep, listed by name
  (domain-model.md:1109-1113; goal/Portfolio.go:17-19; db/queries/Goal.sql:134-138). WRITE_ITEMS at
  the workspace (goal/Portfolios.go:439).
* `Dashboard` at **the workspace or in a hub**, PRIVATE/SCOPE/GROUPS, ≤24 widgets, listed by name
  (domain-model.md:1114-1121; db/queries/Goal.sql:196-200); create: private READ, shared STRUCTURE
  (api/openapi.yaml:1603-1607).
* Ops: goals create/update/delete (WRITE_ITEMS, goal/Goals.go:586-760; delete is a hard delete — no
  trash op), check-ins, link entries; portfolios create/update/delete, status updates, roadmap, public
  link; dashboards create/update/delete, figures, digest. No move between hubs, no archive.
* Webapp: `/goals` (GoalsView) tabs Goals · Portfolios · Dashboards, `?hub=` narrows to a hub
  (GoalsView.svelte:61-66, 82-83, 248); `/goals/:id`, `/portfolios/:id`, `/portfolios/:id/roadmap`,
  `/dashboards/:id` (routes.ts:168-173). Hub tab *Goals* → `/goals?hub=<id>` (placeTabs.ts:53-55).
  GOALS is the one module whose records leave the offline replica when off (Module.go:95).
* Who: PE-planner, PE-owner, PE-admin; UC-GOL-01…04.

---

## 10. AGENTS

* Workspace-wide: `Agent` (DRAFT → APPROVED → ACTIVE ↔ SUSPENDED → RETIRED), `Model`, evaluation
  `Case`/`Run`, `Request` (held act), `Action` ledger, `Job` (domain-model.md:1127-1162). No hub scope
  (a duplicate-review job targets a hub's inventory, domain-model.md:1157-1158).
* Ops: `registerAgent` MANAGE_MEMBERS (api/openapi.yaml:1840), approve/activate/suspend/retire,
  `startAgentJob`, `decideAgentRequest`, halt/policy/metering (api/openapi.yaml:1822-2121).
* Webapp: `/agents` tabs Agents · Waiting for a person · Runs · Settings (home.ts:29-37); also
  `/profile/agents` (routes.ts:79). No hub tab (placeTabs.ts has none).
* Who: PE-admin, PE-member, PE-auditor; UC-AGT-01…07.

---

## 11. Other things a hub or person holds (not products in the band)

| Thing | Scope | Where |
|---|---|---|
| Automation rules | TENANT, HUB, COLLECTION or PERSON (api/openapi.yaml:35656-35668; automation.md:42) | `/administration/rules`, `/profile/rules` (routes.ts:74-76, 102-106); hub/collection settings *Rules* (placeSections.ts:33-34) |
| Webhooks | TENANT/HUB/COLLECTION scope (domain-model.md:757) | `/administration/webhooks` |
| Mail intake / mailboxes | workspace; collection `handle` for mail into a collection (api/openapi.yaml:7040) | `/administration/mail` |
| Code hooks (forge) | per hub, ≤20 (forge/Forge.go:75; domain-model.md:1166-1172) | hub settings *code* (placeSections.ts:33) |
| Chat bots (Matrix/Slack/Teams/Mattermost) | per hub, ≤10 (integration/ChatBot.go:53; domain-model.md:1199-1206) | hub settings *chat* |
| Calendar feeds / pushes | per person (per view) (domain-model.md:758, 1183-1187) | `/profile/calendars` |
| File-store connections, file links | per person; links per entry (domain-model.md:1176-1195) | `/profile/files` |
| Public forms | per collection (domain-model.md:928-933) | collection settings **[unverified which section]** |
| Voice | workspace settings (`/voice/settings`) | `/administration/voice` |

---

## 12. Templates and presets (typical hub constellations)

* **Presets** (structure only, no content): Hubtask's four LIST, TASKS, PROJECT, FLOW
  (core/domain/model/work/Presets.go:71-74), plus installation/workspace (≤100)/own (≤50) presets
  (Presets.go:50-67). A hub without preset starts from the workspace's default (CreateContainer.go:333-355).
* **Starter "Simple" sets** (client-side, apps/webapp/src/lib/workspace/starterSet.ts:36-68):
  MYSELF → hub *Personal* (TASKS) with To do, Shopping (LIST), Ideas (LIST); HOUSEHOLD → *Home* with
  Shopping, Chores, Projects (PROJECT); CLIENTS → *Clients* (PROJECT) with First client, Pipeline
  (FLOW); TEAM → *Team* (PROJECT) with Projects, Requests (FLOW), Meetings (TASKS). Each with marked
  sample entries.
* **Catalogue templates** (core/application/service/config/shipped/*.json; families in
  apps/webapp/src/lib/workspace/hubGallery.ts:13-27; table configuration.md:269-277). A hub template
  always creates **one hub + one collection named as the hub** (CreateHubFromTemplate.go:46-50); the
  rest is configuration (types, workflows, modules, views, rules), not more containers. Modules
  switched on (workspace + hub part):

| Family | Template (version) | Modules on | Types | What else the hub part brings |
|---|---|---|---|---|
| personal | household, shopping, habits, travel (v2) | workflows | chore / shopping_item / habit / trip_booking | scheme |
| teams | kanban_team (v4) | workflows | card | default view KANBAN, 3 samples |
| teams | scrum_team (v3) | workflows, planning, knowledge | story, bug | default view KANBAN; collection runs sprints (api/openapi.yaml:29675-29680) |
| teams | bug_tracking, content_calendar, event_planning (v2) | workflows | defect / content_piece / event_task, event_guest | scheme |
| teams | team_wiki (v1) | workflows, knowledge | — | template pages *Start here*, *Team handbook* |
| projects | classic_project (v5) | workflows, planning, knowledge | deliverable, risk, issue, change_request | default view TIMELINE |
| projects | hybrid_project (v2) | workflows, planning, knowledge | hybrid_deliverable, hybrid_story, raid_item | scheme |
| projects | client_project (v2) | workflows, planning, time | client_deliverable, raid_item | scheme |
| programmes | portfolio, okr (v2) | workflows, goals | initiative / okr_initiative | scheme |
| programmes | scaled_agile (v2) | workflows, planning | feature, train_story, roam_risk | scheme |
| service | itil_service_desk (v15) | workflows, service_desk, knowledge | incident, service_request, problem, change | collection offers those 4 types (→ desk); views mine, unassigned, about_to_breach, unhappy; 6 rules (acknowledge, ask_for_information, close_resolved, sla_at_risk, sla_breached, p1_escalation); 3 samples; template pages *How we work with this desk*, *Knowledge base*; workspace part: roles cab_member, requester; fields impact/urgency/priority/responding_group; calendar office; SLA policies incidents, requests; 3 reply templates |
| service | internal_it, facility_requests, hr_requests, customer_support (v4) | workflows, service_desk | it_request / facility_request / hr_request / support_case | structure (entry types → desk) |
| processes | approvals, procurement (v4) | workflows, service_desk | purchase/leave/travel_request; procurement_request | structure; questions |
| processes | onboarding, capa, risk_controls (v2) | workflows | staff_change / nonconformity / isms_risk, control | scheme |
| processes | records_of_processing (v2) | workflows | processing_activity | **no hub part** (workspace only) |
| registers | it_asset_register, risk_register, contact_register, vendor_contracts | workflows | asset / risk_record / organisation, contact / vendor, contract | **no hub part** (record types for the workspace, configuration.md:277) |
| registers | configuration_items (v1) | workflows, assets | — | hub part switches Assets on |

(Values from a script over the shipped JSON files; versions/keys as in the files.)
* Showcase (`hubctl workspace demo`): Scrum, Kanban, project and desk hubs, pages, three
  whiteboards, goals, two portfolios, two dashboards (user-experience.md:313-318).
* Per-product first-use defaults and templates offered: user-experience.md:347-356 (Pages: space per
  hub with home page; Desk: desk collection, queues, portal, Mon–Fri 8–18 calendar, P1–P4 SLAs;
  Plans: plan over the hub's dated entries; Assets: shipped CI types; Goals: current quarter).

---

## 13. What I could not verify
* Effect of a cross-hub collection move on hub-scoped releases, hub SLA policies, hub calendars and
  portfolio projects (no `ContainerMoved` subscriber found besides revocations).
* Effect of moving an entry to another collection on its open sprint membership / release membership.
* Where in a collection's settings the public form is managed.
* Whether Today's *Recently opened* band (user-experience.md:257) is server-backed — I found no API
  for recents; assume client-side.
* Real-world typical counts (hubs/collections per workspace) — the docs give only bounds (above).
