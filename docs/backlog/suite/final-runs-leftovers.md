# What the final runs left open

The fixers of the coverage round 3, the role reviews, the walks and the demo data reported what
they did not finish, verbatim below (2026-11-03). A later step of S-24 turns every item that is not
yet built into a row of "Gaps a step found" in [README.md](README.md) and closes it, or records why
it is already closed. Nothing here is done until a test or a measurement proves it.

```text
=== coverage 3 fix 1 done=False
Handed over before the context limit, with three high findings not started. All three are confirmed against the code: no hub_id or copied_from anywhere; no NOTIFY_* action in api/ or core/; no table, KaTeX, Mermaid, lowlight or emojibase in packages/editor. (1) Building blocks at a hub plus 'Make a copy here' with copied_from. Contract, then an expand migration adding hub_id and copied_from on entry_type, workflow, sla_policy and custom_field_definition with per-(tenant,hub,key) uniqueness. Then: a nearest-block resolver, config.block_shared, a CopyBlockHere use case on every door, hub document sections, read-only shared blocks with 'Make a copy here' in the web app, archive rows, cross-tenant tests and UC-CFG-01/UC-FLW-01 checks. (2) NOTIFY_ACCOUNT, NOTIFY_GROUP and NOTIFY_ROLE automation actions (UC-AUT-09, fourth answers 6). Contract first; role holders resolved by the authoriser along the entry's path; sent through the notification path under the SLA category; recipients narrowed to people who may READ the entry; the save check refuses an unknown recipient; then the rule inspector UI and moving UC-AUT-09 to built. (3) Editor blocks (fourth answers 2): @tiptap/extension-table@3.32.0, katex, mermaid, lowlight and emojibase-data through the licence gate as lazy chunks. Then the table, math, Mermaid, highlighted code and emoji-picker nodes, their Markdown round-trip, the server-side Markdown/HTML/DOCX export, mapping wiki tables in the importer, THIRD-PARTY-LICENSES.md, and closing gap rows 184 and 208. Small follow-ups on the download: no hubctl command yet (a binary stream to a file), and no MCP door (a binary stream does not fit MCP). Not run: make check / make verify and the full integration suite; only the touched packages' tests and the four export and restore integration tests were run.

=== coverage 3 fix 1.2 done=False
I handed over before the context limit. Two high findings are not started.

(1) Building blocks at a hub, plus 'Make a copy here' with copied_from. Steps:
- the contract;
- an expand migration adding hub_id and copied_from on entry_type, workflow, sla_policy and custom_field_definition, with uniqueness per (tenant, hub, key);
- the nearest-block resolver and config.block_shared;
- a CopyBlockHere use case on every door, and the hub document sections;
- read-only shared blocks with 'Make a copy here' in the web app;
- archive rows, cross-tenant tests and the UC-CFG-01 / UC-FLW-01 checks.

(3) Editor blocks (fourth answers 2):
- add @tiptap/extension-table@3.32.0 (and extension-details at the core's version), katex, mermaid, lowlight and emojibase-data through the licence gate, each as a lazy chunk;
- the table, math, Mermaid, highlighted-code and emoji-picker nodes, each round-tripping through Markdown;
- the server-side Markdown/HTML/DOCX export, and mapping wiki tables and details in the importer;
- THIRD-PARTY-LICENSES.md, and closing gap rows 184 and 208.

Smaller follow-ups:
- the owner's archive download still has no hubctl command (it is a binary stream to a file) and no MCP door;
- a custom role deleted after a NOTIFY_ROLE step was saved is refused when the rule runs, but the rule check does not flag it yet;
- make check and make verify have not been run on the branch.

My choices are appended to the pull request notes. The branch is pushed and the working tree is clean.

=== coverage 3 fix 1.3 done=False
The rest of high finding (1): workflows, SLA policies and custom fields at a hub, following the entry-type pattern (hub_id and copied_from columns via an expand migration, Within-style resolution, copy-here use cases on every door, refusing moves out of the hub, config document hub_id, read-only shared blocks with 'Make a copy here' in the web app, cross-tenant tests, UC-CFG-01 checks). workflow_scheme already places a workflow per hub. High finding (3), the editor blocks, is not started: table/details/katex/mermaid/lowlight/emojibase as lazy chunks through the licence gate, Markdown round-trip nodes, server-side export, wiki importer mapping, THIRD-PARTY-LICENSES.md, gap rows 184 and 208. The hub-type UI was not walked in the running app (unit tests, typecheck and lint only). make verify has not been run; make check is green and the branch is pushed. Pushed commit 46cc83e9b (postgres) does not build on its own because it needs the domain fields from 9ed4b2511, which comes after it; the reorder needed a force push, which was refused, and the squash merge holds the head. Notes are appended to .git/suite-pr-notes.md.

=== coverage 3 fix 2 done=False
Five findings from the 10⁷ run (R1/R3) are open, listed in docs/evidence/capacity-10000000-2026-11-01.md. They are real, but none is fixed or explained yet, because the dataset is torn down with each run:
(a) a collection's page is 345-561 ms at P95 in the middle and smallest workspaces, against 8-11 ms elsewhere; it looks like a plan switch.
(b) the board and the count over the hub take about 1 s in the 1.2-million-entry workspace.
(c) the hub's level is 349 ms at P95 with kept figures in the largest workspace, against 7 ms at 10⁶.
(d) 1,965 PostgreSQL 'canceling statement due to user request' errors, which the 10⁶ runs did not have.
(e) R3's walk saw only 403s because the load token's scopes (items:read, items:write, containers:read, automation:manage in test/load/stack_test.go:378) are too narrow for the web app. It needs a walk token carrying every scope the app reads.
Next step: re-seed 10⁷ (about 30 min), EXPLAIN (ANALYZE, BUFFERS) each slow path, fix test-first, then run R2 with HUBTASK_LOAD_WALK=1.

Not started:
- Finding 4, editor blocks (fourth answers 2): six MIT packages through the licence gate, each lazy-loaded; Markdown round-trips in markdown.ts and Render.go; wiki import mapping of tables and details; an emoji picker on emojibase-data.
- Findings 5/8, the public roadmap link per portfolio (fourth answers 9): follow UC-DOC-08's pattern from commits 49e00c9aa..0dc007ebc — domain token, migration with keyed hash, archive rows and cross-tenant test, service with create/revoke/list audited on every door, contract and an unauthenticated route, sync-engine types, a web-app share dialog and switch, and a UC-GOL-03 check.
Both are already decided by the owner; they are large and did not fit this context. Choices are appended to the pull request notes. Everything is pushed to origin suite/concept.

=== coverage 3 fix 2.2 done=False
The context is near its limit, so I am handing over. Everything is pushed to origin suite/concept (HEAD e5368f146). Open: (b) In the 1.2-million-entry workspace, the board is 430 ms P95 and the count over the hub is 974 ms P95. Both read every entry they cover: the board's window function numbers 407,923 rows, and the count reads 1,223,768 rows. That is not a wrong plan. The fix is design work a worker can decide: keep per-group counts the way collection_figure keeps figures (UC-WRK-29), and read the board per column with LIMIT probe (wi_board_idx already serves bucket_id with order_key). The plans are in docs/evidence/capacity-10000000-2026-11-01/plans-10000000.json. Also left: 352 cancelled figure statements in the mixed phase (each write makes the kept count stale, and the first read after it is cut off). Finding 4 (editor blocks, fourth answers 2) and findings 5/8 (public roadmap link per portfolio, fourth answers 9) are not started; the previous worker's notes on them still apply. Not covered by a test: the ContainerView fix is proven only by R3's walk, because there is no component test setup for that view. Rerun `HUBTASK_LOAD_WALK=1 make capacity ITEMS=10000000 TENANTS=2000` (about 43 min) after the board and count work, and add HUBTASK_LOAD_EXPLAIN=1 only for diagnosis. Choices are appended to the pull request notes.

=== coverage 3 fix 2.3 done=False
Everything is pushed to origin suite/concept (HEAD 1300fb26e). Ran: go build ./..., go vet, the query compiler tests, the work service tests, the grouping/query integration tests (Testcontainers), make generate (no diff beyond schema.sql), go run ./tools/checkdocs, and the full 10^7 capacity run with the walk. Not run: make check and gates.

Open:
(1) A board grouped by bucket_id or status, which is what the web app's board uses, still uses the window function. Their values are data. A LATERAL over the buckets would be one parameterized plan that walks the whole order index for a small column. The web app's board also reads include_archived, and wi_board_idx is partial on archived_at IS NULL. The capacity run does not measure this path.
(2) Editor blocks (fourth answers 2) are not started.
(3) The portfolio's public roadmap link (fourth answers 9) is not started. It would mirror the page public link: about 50 files over 8 commits (49e00c9aa..0dc007ebc), plus a public roadmap screen. It needs a fresh worker.
(4) The ContainerView fix is still proven only by R3's walk.

All choices are appended to the pull request notes.

=== coverage 3 fix 3 done=False
Handed over to keep context small. A fresh worker should build the following; all are decided in accepted.md, so none is the owner's.
(3) The passphrase-encrypted owner download (gap row 207 reworded). Write members AES256_GCM under a key derived with Argon2id from the owner's passphrase (infrastructure/crypto.Passphrase, archive.Encryption.KDF fields already exist). Accept a passphrase in the download body, contract first. Serve decryption_passphrase on restore, which backup-restore.md and RefusePassphrase refuse today. Name the passphrase mode in tenant-export.md §1 (which says 'never encrypted'). Add a test that restores a passphrase download elsewhere.
(4) Approving from chat (new gap row 'Coverage 3'). Link a person's chat account, post an 'approval waiting' card to a linked approver, and decide it through DecideApproval as that person; refuse anyone who is not an approver and record the trail with the request's version. Add a UC-SVC-09 check and a test per adapter (Slack, Teams, Mattermost, Matrix).
(5) Workflows, SLA policies and custom fields at a hub, with copied_from and nearest-first resolution, following the entry types pattern of migration 0317 (gap row 189).
(8) The editor blocks of fourth answers 2: @tiptap/extension-table and @tiptap/extension-details at 3.32.0, katex, mermaid, lowlight and emojibase-data, each through the licence gate and loaded lazily; Markdown round-trip tests; the wiki import mapping tables and details. None is started.

=== coverage 3 fix 3.2 done=False
I handed over to keep the context small. All of the following are decided in accepted.md, so none is the owner's. (4) Approving from chat (gap row 'Coverage 3'): link a person's chat account to a person (a new table and a link command or code), post an 'approval waiting' card to a linked approver, and decide it through DecideApproval (core/application/service/servicedesk/Approvals.go) as that person; refuse anyone who is not an approver and record the trail with the request's version. Add a UC-SVC-09 check and a test per adapter in infrastructure/chatbot (Slack.go, Teams.go, Mattermost.go, Matrix.go; port core/port/chatbot/Port.go has no proactive post to a person yet, only Room.Post for Matrix). (5) Workflows, SLA policies and custom fields at a hub, with copied_from and nearest-first resolution, following the entry types pattern (commits d8ff75de1, 46cc83e9b, 9ed4b2511, d9c4d74ae, 02ecba0ef, 40f8cba3d; migration 0317; gap row 189). Note that custom_field_definition already has collection_id (NULL means workspace-wide), so first check whether a hub id already works there; workflow and sla_policy have no container column. (8) Editor blocks (fourth answers 2): @tiptap/extension-table and @tiptap/extension-details at 3.32.0, katex, mermaid, lowlight and emojibase-data, each through the licence gate and loaded lazily, with Markdown round-trip tests and the wiki import mapping tables and details. None of (4), (5) or (8) is started.

=== coverage 3 fix 3.3 done=False
I handed over before the context limit. Not started:

(5) Workflows, SLA policies and custom fields defined at a hub, with copied_from and nearest-first resolution, following the entry types pattern (commits d8ff75de1, 46cc83e9b, 9ed4b2511, d9c4d74ae, 02ecba0ef, 40f8cba3d; migration 0317; gap row 189). One premise is now checked and is false: a hub id in custom_field_definition.collection_id does NOT work today. Every resolving query matches only `cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL` (db/queries/Sync.sql:177, and likewise Figures.sql, Work.sql, Fields.sql, BackupExport/Import.sql). Hub fields therefore need an expand-only hub_id column, or a match on the item's hub_id, in each of those queries. workflow and sla_policy have no container column. Each entity is about the size of the entry types work (around 1,600 lines across six commits).

(8) Editor blocks (fourth answers 2): @tiptap/extension-table and @tiptap/extension-details at 3.32.0, katex, mermaid, lowlight and emojibase-data. Each must pass the licence gate and load lazily, with Markdown round-trip tests, and the wiki import must map tables and details.

Next worker: continue on suite/concept at 7e09017a0 (pushed). My choices for (4) are appended to .git/suite-pr-notes.md.

=== coverage 3 fix 4 done=False
5 (real, partly built) Hub-defined building blocks: entry types at a hub and 'Make a copy here' with copied_from are done (migration 0317, UC-FLW-01 check 10, commits d8ff75de1..c7408f901). Still open: the same for workflows (0216), SLA policies (0240) and custom field definitions. Each needs an expand migration (hub_id plus copied_from, keys staying unique per workspace as 0317 chose), hub-before-workspace resolution, the read-only shared block with 'Make a copy here' in the hub settings, archive and replica rows, cross-tenant tests and use case checks. Gap row 189 of docs/backlog/suite/README.md describes this. Each block is about 1,700 lines, too big for what was left of this context. 7 (real) UX design direction with mockups under docs/design/ and a seed command for a realistic demo workspace in every product: neither exists. Both belong to the UI/UX run named in the owner's word of 2026-10-31, which says the direction is built, not presented for approval. Not started. Notes are appended to .git/suite-pr-notes.md.

=== coverage 3 fix 4.2 done=False
The final make check is still running in the background (log: (a local log)) and I don't have its result. The previous run failed only on gofmt in the servicedesk files. That is fixed, and the fix was folded into 1f42e20d5 by an autosquash rebase of commits that were not yet pushed. Commits after 02e7b51bc are NOT pushed: the next worker runs make check and, if green, git push origin suite/concept. Still to build: custom field definitions at a hub, the last part of gap row 189. Definitions already carry collection_id, so check first whether a hub id already works there; follow the workflow and SLA commits. Also not started: the UX design direction with mockups under docs/design/ and a seed command for a realistic demo workspace in every product (UI/UX run, owner's word of 2026-10-31). The new hub screens (workflows block in the hub's StructurePanel, HubSlaPolicies) pass typecheck, lint and the web app's tests but were not looked at in the running app.

=== coverage 3 fix 4.3 done=False
The final make check is green and HEAD 0bfbd047b is pushed to origin/suite/concept.

Ran: go build, the touched Go packages' tests including test/architecture, the integration tests for custom fields, hub fields, kept figures, backup/restore, snapshot and query (Docker), and the web app's typecheck, lint and full tests.

Not done:
(1) The new hub screens were not looked at in the running app. This covers this run's custom-fields block in the hub's StructurePanel and, from the earlier run, the workflows block and HubSlaPolicies. The e2e fixture answers /custom-fields?hub_id= with an empty list, so the block stays hidden in a fixture walk unless the fixture is extended.
(2) Not started, from the owner's UI/UX word of 2026-10-31: the design direction with mockups under docs/design/, and a seed command that fills a demo workspace with realistic data in every product. The owner said it is to be built as planned, not presented for approval. A fresh worker should take this, because it does not fit in one context.
(3) Lead, not verified: a rough scan of db/queries/BackupImport.sql against db/schema.sql suggests other import statements may also miss columns (container.logo_media_id, automation_rule.paused_for_deletion, jumble_entry.meant_for_*, account.avatar_media_id, …). Some are deliberate, such as secrets and the tenant row. Each needs a round-trip test like TestARealCustomFieldSurvivesTheRoundTripThroughTheArchivesShape before a fix.

Choices and these leads are in the pull request notes.

=== coverage 3 fix 5 done=True
The hub Calendars screen (apps/webapp/src/lib/servicedesk/HubCalendars.svelte) lists only the workspace's calendars, because GET /business-calendars does not answer the installation's. A hub's version of an installation calendar can therefore be kept only through REST, MCP and hubctl. This is recorded in gap row 178. Fixing it needs the list (or a new read) to answer the installation's calendars with a flag. Also: the full make check and the full integration suite were not run; only the touched packages' tests and the targeted integration tests were.

=== coverage 3 fix 6 done=True
Nothing remains from these three findings. Everything is pushed to origin suite/concept and the tree is clean. Checks run: go build ./...; go test on infrastructure/mailbox, core/application/service/mail, presentation/worker (with -race), infrastructure/observability and core/port; make generate; make gate-architecture; make gate-docs; and the integration test TestOpenKeysNameEachLivingJobsWorkspaceAndKey, which was green. make check and the full suites were not run.

=== roles fix 1 done=False
Every finding was confirmed against the code; none was not real. Left for the next worker, in order:
(a) Finding 5: rewrite the 24 suite migration files listed in test/architecture/migrationlocks_test.go notYetConcurrent. They are unreleased and not merged (main and v0.9.1 stop at 0206), so they can still be changed. Move each index on a released table into a trailing NO TRANSACTION file (0328+) as CREATE INDEX CONCURRENTLY IF NOT EXISTS. Add NOT VALID to each CHECK or FOREIGN KEY on a released table. Remove the same-file VALIDATEs and validate the final constraints in that trailing file. Trap: 0278's unique index on calendar_feed (tenant_id, id) backs a foreign key in the same file, so it cannot simply move. After the rewrite, run cmd/migrate TestUpAppliesAMigrationMergedAfterAHigherNumberedOne and the integration tests.
(b) Finding 8, not started: a CI job that starts the last release's image, migrates its database to the new schema while the old app is up, re-runs the old hubctl e2e against it, then runs the new app with the new e2e, rolls back to the old image, and runs the old e2e again.
(c) Finding 4, UI and door tests: a per-hub modules_on for the container read (contract first) or a hub-scoped capabilities call, with ContainerView and ItemView gated on the hub's set. Add door tests that one PLANNING operation is refused through REST, MCP and an automation action.
(d) Finding 3: the bounded execution pool (claim only as many jobs as there are free slots) is not built. Handlers have not been audited for running concurrently in one process, so a long job still holds back the others on a worker.
(e) Finding 7: a cursor ledger so a future backfill resumes where it stopped, and a test that kills the migrator between batches and runs it again.
Not run: make check, make verify, the full integration suite, helm template (helm is not on PATH here), and any walk of the UI. Choices are appended to the pull request notes. Everything is pushed to origin suite/concept.

=== roles fix 1.2 done=False
Left for the next worker:
(b) Finding 8 is not built. The plan does not work as written. scripts/hubctl-e2e.sh owns its stack: it runs `down -v` when it exits and starts by spending the first start's setup code, so the v0.9.1 session cannot run twice against one database. The CI job needs its own upgrade session script: start the old stack from the v0.9.1 image, seed data with the old hubctl, run the new migrator while the old app is serving, check that the old hubctl can still read and write, start the new app and read with the new hubctl, then put the old app back on the new schema and run the old hubctl again.
(d) Finding 3, the bounded execution pool, is not started. The handlers still need checking for state they share within one process.
Also open: inline CHECKs on ADD COLUMN in 0284 and 0286 still scan their table under an exclusive lock, which the architecture test does not catch, and the per-row key backfill in 0286 runs inside its own transaction.

What ran: go build ./..., the tests of every touched Go package, the architecture tests, test/doors, test/privacy, gate-docs, and webapp/sync-engine typecheck, test and lint. Full integration suite: the second run had a single failure, TestSchemaReferenceMatchesTheMigrations. That run's test binary was compiled before I edited schema.sql (which the test reads at run time), and the test passes on its own now. I did not rerun the whole suite on the final HEAD; the tests the ledger change touches (migration, schema reference, boundary, backfill resume) were rerun and pass.

Not run: make check, make verify, helm template, and any walk of the UI in the browser. Notes are appended to the pull request notes. Everything is pushed to origin suite/concept.

=== roles fix 1.3 done=True
Not run: make check, make verify, the full integration suite on the final HEAD, helm template, and any walk of the UI in the browser. The upgrade job has not run in CI yet; it ran once locally. The check for in-process handler state (needed before jobs run in parallel) was a regex scan, not a proof. It looked at types with one-letter pointer receivers that change their own fields in core/application, infrastructure and presentation/worker. Every hit was either state created per call or guarded by a mutex, and every worker handler has a value receiver. Shared state reached any other way, such as package-level variables or longer receiver names, was not checked. Nobody has run a real installation under load with 4 slots. The chart has no value for HUBTASK_QUEUE_CONCURRENCY yet; the default of 4 applies. Everything is pushed to origin suite/concept, and the notes are appended to the pull request notes.

=== roles fix 2 done=False
Not done:
- Finding 5 (workspace download holds a REPEATABLE READ snapshot and a pool connection while the client reads, up to 6h): not started.
- Finding 4, the rest:
  - an hourly scheduler reconcile that re-seeds enabled mailboxes, connectors and duties that have no open job;
  - an operator/admin operation plus a screen to list and requeue dead letters with audit (contract first). Recovery is still the psql UPDATE in RB-A07.
- Finding 2, the rest: marking a rejected address as undeliverable for its recipient needs a schema change and UI.
- Finding 6, the rest:
  - the MCP stream is not rechecked;
  - no immediate close when RevokeAll revokes sessions (it can take up to about 30-60 s);
  - I did not check how the webapp handles close code 4401.
- Finding 8, the rest:
  - author FKs to RESTRICT (expand/contract);
  - the erasure preview screen does not show the new writings count.
- Finding 1, the rest: the NotificationDelivery handler's Refusal mapping has no unit test of its own. The long backup still holds its snapshot for hours, which pins xmin (the same class as finding 5).
- Not run: make check, make verify, the full integration suite, the UI.
- Run: go build ./..., the touched packages' unit tests (worker, mail, notification, privacy, realtime, rest, environment), test/architecture, PG-2, and the privacy_kept integration tests.

Notes are appended to the pull request notes. Everything is pushed to origin suite/concept.

=== roles fix 2.2 done=False
Not built:
- Finding 4: an operator/admin operation and screen to list and requeue dead letters. This means contract first, then a service behind admin:tenants and the operator register, an instance_event journal entry, hubctl, a webapp screen and en/de texts. Non-duty jobs are still recovered with psql, as RB-A07 describes.
- Finding 6: an immediate close of held connections when RevokeAll revokes sessions. Every held connection still closes within 30 s. Closing at once on every pod needs a cross-pod signal: a NOTIFY channel for revocations wired into stream.Registry.
- Finding 2: marking a refused address as undeliverable for its recipient (needs a column and a screen).
- Finding 8: the parent FKs of page and board comments still delete replies with their parent (ON DELETE CASCADE). The erasure blanks such a comment instead of deleting it. Changing the FKs means checking every delete path first: retention, backup import, page and board purge.

Known limits of what was built:
- A slow but steady download reader still holds the snapshot for as long as it reads, up to 6 h. A-25 now makes that visible.
- Dead letters of duties purged by the 7-day rule before this release are not revived.
- A duty that keeps failing keeps its dead-letter row past 7 days, because each revival renews finished_at.

What ran:
- go build ./...
- Unit tests: presentation/rest, mcp, stream, realtime, worker, infrastructure/observability, packages/sync-engine (174 pass; typecheck and lint also pass).
- test/architecture, make gate-observability, make gate-docs.
- Integration tests: TestADeadDutyIsRevivedAfterItsWait, TestFinishedJobsGoAfterTheCataloguesPeriod, TestTheOldestTransactionIsMeasured, TestSchemaReferenceMatchesTheMigrations.

Not run: make check, make verify, the full integration suite, the webapp's own tests and typecheck (the sync-engine change has no API change), and any UI walk.

Everything is pushed to origin suite/concept. Notes are appended to the pull request notes.

=== roles fix 2.3 done=False
Not built: finding 2, marking an address refused for good (SMTP 5xx at RCPT/DATA) as undeliverable for its recipient. It needs an expand-only column on the recipient's address, a queue/notification check that skips such an address, and a profile or people screen that shows it and lets the person clear it. Today every 5xx fails the one notification on its first attempt and is not counted by the breaker, so the next message to that address is tried again.

Choices made: one requeue per call, with no bulk requeue by kind. The dead-letter screen was not added as a numbered check of UC-INS-12, because a new check is the owner's; the coverage document carries its two rows. undeclaredPermissionsAtMost goes from 198 to 200, with the reason in the comment. A credential ring triggers a recheck, not a forced close, so the same account's other live sessions keep their streams. A hard DELETE of a session row does not ring; the 30 s recheck covers it.

What ran: go build ./...; unit tests for presentation/{stream,mcp,rest,realtime}, infrastructure/postgres (listener), core/application/... (including the catalogue), presentation/... and cmd/...; test/architecture and make gate-architecture; make gate-docs; test/security; locales; make generate and make api-client (no diff left); the webapp's node tests, svelte-check and lint; the sync-engine typecheck. Integration tests that passed: TestARevokedSessionRingsTheCredentialChannel, TestSchemaReferenceMatchesTheMigrations, TestRecordingAChangeWakesTheListener, TestTheOperatorRequeuesADeadLetter. The first Testcontainers attempt failed to start its reaper container; the retry passed.

Not run: make check, make verify, the full integration suite, the upgrade session. Not walked in the browser: the Failed jobs screen.

Everything is pushed to origin suite/concept. My notes are appended to the pull request notes.

=== roles fix 3 done=False
Finding 6 is not started. A restored workspace's SLA, OLA, approval and page-review clocks still never fire. Confirmed in the code: archive/Record.go leaves durable_timer out, SLAClocks takes no replays, and follow() skips ErrNotFound. A fix needs a re-arm after restore, or a self-healing sweep in the timer job. A RUNNING entry_sla can be re-armed from its due_at, and one already past due fired at once so it breaches through the normal path. A PAUSED record stores no consumed time (EntrySLA has no such field), so re-arming it needs that recorded first, or a rule for what to assume. OLA, approval and page-review timers need the same treatment. Then a restore round-trip test.

Finding 2, still open: pages are not stripped by an erasure or expiry. ListIntakeLinesFrom does not yet read page.body or page_draft.body. It needs its own design: UpdatePage writes into the draft in a space with drafts, and page_version and the live document keep the old text.

Finding 8, still open:
- Board columns still draw 50 cards. Continuing a column means asking for that group again with its key as filter and its cursor, as a separate subscription, which the engine leaves to the caller.
- The list has no IntersectionObserver auto-load and no row windowing.
- Not checked: whether a live refresh of the list resubscribes to the first page only and drops the pages already loaded.
- The finding asked for a component test with 250 entries and a 120-card column; the webapp has no component-test setup (node:test only), so the paging logic is tested as a pure function instead.

Finding 5: 84 foreign keys that cascade without an index remain, listed in test/integration/cascade_index_test.go (unindexedCascades). Several sit on busy tables: notification has six, plus item_label, item_member, item_attachment, reminder, comment parent, page_version author. Each needs judging on write cost against how often the parent is deleted.

Not run: make check, make verify, the full make gate-privacy-full, a running-app walk. Docker was available, and the PG-7, cascade-index, schema-reference and erasure-arrangements integration tests ran green.

=== roles fix 3.2 done=False
Still open:
- Finding 2: an erasure or expiry does not strip pages. ListIntakeLinesFrom still does not read page.body or page_draft.body, and page_version and the live document would keep the old text. This needs its own design.
- Finding 8: the collection list still has no IntersectionObserver auto-load and no row windowing. The board 'Show more' and the list's paged reload have not been walked in the running app.
- Finding 5: 73 cascading foreign keys without an index remain, listed in test/integration/cascade_index_test.go. They are on deletes of rare parents (accounts, identity providers, tenants, backup runs and schedules, agents, page_version and whiteboard_version authors). I judged them not worth an index each now; take each off the list when it gets one.

Not fixed, test-only so far: a NEW_TENANT copy's slug is 'restored-' plus the first 12 hex digits of its id. Two restores minted in the same millisecond would collide on tenant_slug_key. I saw this only between two tests in one integration run, and the new test now uses a different id prefix.

Not run: make check, make verify, the full integration suite, make gate-privacy-full, a running-app walk.

Run and passing: go build ./...; unit tests of core/domain, core/application/service/..., core/application/archive, infrastructure/postgres, cmd/server and test/architecture; make generate; make gate-docs; golangci-lint on the touched Go packages; the integration tests TestARestoredWorkspacesGoalsGetTheirClocksBack, TestAnOperatorRestoresAnExportFromAnotherInstallationAsANewWorkspace, TestEveryCascadingForeignKeyHasAnIndex, TestSchemaReferenceMatchesTheMigrations and Import*; sync-engine test, typecheck and lint (175 pass); webapp svelte-check, lint and tests.

All commits are pushed to origin suite/concept. My choices are appended to the pull request notes.

=== roles fix 3.3 done=True
Not built, by judgement: row windowing for the collection list. The list only grows when the reader presses the button, 200 rows per page. Windowing would break drag, keyboard order and find-in-page across the whole screen, and no measured problem calls for it. Finding 5: the 73 listed cascading foreign keys without an index stay as fixer 7 judged them. Not run: make verify, the full integration suite, make gate-privacy-full, and a walk in the running app. Nobody has walked the board's 'Show more', the list's paged reload, the Failed jobs screen or the webapp's 4401 handling. Run and passing: go build ./...; the unit tests of core/domain/model/backup, core/application/service/backup, presentation/mcp, presentation/rest, test/architecture and cmd/migrate (with Testcontainers); golangci-lint on the touched packages; make check (green). Everything is pushed to origin suite/concept, and my choices are appended to the pull request notes.

=== roles fix 4 done=True
Checks: make check is green, everything is pushed to origin/suite/concept, and notes are appended to .git/suite-pr-notes.md. Not done or not verified: (a) none of the UI changes (whiteboard keyboard, 'Live updates paused', membership paging) were tried in the running app; apps/webapp has no component-test harness, so findings 7 and 8 are proven only by the pure keys.ts tests. (b) Comment pins on the canvas still cannot be reached with Tab; their threads should be in the board's comment panel, but I did not check that. (c) The stream defaults are fixed numbers; they are not worked out from memory or RLIMIT, and nothing was measured. (d) GET /approvals still returns the whole list without a cursor in the contract, with a limit of 5,000 candidates per reader and a log warning at that limit. (e) Pickers do not search the server once a scope has more than 4,000 grants; past that, the role stays unknown. (f) The clamd recognition of a size refusal after a failed write is best effort, and the upload limit (HUBTASK_CLAMD_STREAM_MAX_BYTES) is the main protection. (g) Migration 0336 was not applied in a test run; the CHECK change is checked only by the architecture migration gate passing.

=== roles fix 5 done=False
Not built: finding 2, multi-mode OAuth returns. Confirmed in the code: cmd/server/main.go sets the mail, calendar-push and file-store RedirectURLs on HUBTASK_BASE_URL. apps/webapp/src/views/MailView.svelte and MailboxSection.svelte complete the flow with the session of the host they land on, and none of the three flows has a locate bridge. Proposed build:
- (a) Put the tenant ID in the state: '<tenant>.<random>'. The hash lookup stays the same.
- (b) Add one anonymous use case, LocateConnectorReturn {state, kind}. It checks in the tenant's scope that a live pending authorization exists for that state hash: a new non-consuming sqlc query on each of the three repositories, with a cross-tenant test each. It answers workspaceAddress, and every refusal is the same one.
- (c) Contract /auth/connector:locate, then make generate.
- (d) The three webapp return pages call locate when they are not on the workspace host and forward the path and query unchanged, reusing oidc.ts forwardTo with the path as a parameter.
- (e) automation.md §5 and .env.example state the multi-mode behaviour; a multi-host e2e test.

Also not built:
- Finding 8's spool: read the rows under a snapshot into an encrypted temporary file, end the transaction, then stream. It needs archive.Writer split into data and media phases, and a spool location: compose runs the app read_only with /tmp on tmpfs (RAM).
- Findings 6/7's plan-resolved quota rows (streams_per_workspace, api_requests_per_minute_workspace in GET /quotas, resolved product → installation → plan → exception). Today both are per-process configuration only.
- Finding 5's measurement: no timed upgrade of a v0.9.1-shaped database at 10^6 entries exists, and I did not run one.
- Finding 3's showing the UNSCANNABLE reason to the uploader in the UI.

Checks run: go build; go test on the touched packages; golangci-lint on the touched Go packages; make generate, api-client, gate-docs, locales; packages/whiteboard test, typecheck, lint; webapp svelte-check, lint and node tests (1391 in the full run before restore.test.ts was added; restore.test.ts's 2 tests run on their own).

Not run: make check, make verify, the integration suite, a running-app walk. Notes are appended to .git/suite-pr-notes.md.

=== roles fix 5.2 done=False
Not built:
- Finding 8: ending the snapshot before the transfer. The archive writer is streaming-only by design (comment in core/application/archive/Writer.go). Media are fetched inside the snapshot, and ExportMedia.Open looks up MediaLocation in the database. Proposed: resolve each digest's storage key inside the snapshot, end it after the data files, then stream the media. That removes the bulk of the bytes from the snapshot without a spool. A full spool of the data files would land on tmpfs (RAM) in compose and needs a spool location first.
- Findings 6/7: plan-resolved workspace rate and stream-count rows in GET /quotas. Both are per-process configuration today. The per-token row is resolved in the SQL of the token and session lookups. Doing the same per workspace needs a cached per-tenant resolution in the rate limiter and the stream registry.
- Finding 5: no timed upgrade of a v0.9.1-shaped database at 10^6 entries has been run or measured.

Not run: make check, make verify, the full integration suite, a running-app walk, and a real two-host installation with a real OAuth provider. Notes are appended to the pull request notes. All commits are pushed to origin suite/concept.

=== roles fix 5.3 done=False
Not built: findings 6/7, a per-workspace request rate and live-connection count that a plan or an operator exception can change and that GET /quotas shows. It needs new quota names in the contract enum, the settings catalogue (catalogue.gen.json), QuotasView and InstanceWorkspacesView, en/de texts, the admin PATCH, and a cached per-workspace lookup in rest.Limited and the stream registry. It also reverses multi-tenancy.md §4, which an earlier fixer on this branch rewrote to say these two limits are per-process configuration. That is a feature of its own, clearly more than one step, so it should become a finding issue. No issue was opened.

Seen but not changed: seeding 10^6 entries in one INSERT did not finish in 20 minutes. v0.9.1's work_item_assign_number trigger updates the collection's counter row once per inserted row inside the same transaction. The test now seeds in batches of 10,000. Not checked whether any real bulk path inserts that many entries in one transaction.

Not verified: what a restored pre-0.10 archive is missing in meaning. The workflow and entry-type tables stay empty for that workspace; only custom_role is referenced by another entity.

Not run: make check, make verify, the full integration suite, a walk through the running app, and a backup with media against a real Postgres snapshot (test/backup's media runs use a fake export repository). The integration tests for the download and export paths that were run (TestTheOwnersDownloadRestoresAndComesOnceADay, TestAPassphraseDownloadRestoresElsewhereWithItsPassphrase, TestAnInstallationWideCaseIsCarriedOutByItsOwnJob, TestAnOperatorRestoresAnExportFromAnotherInstallationAsANewWorkspace) pass but carry no media.

All commits are pushed to origin suite/concept, and the notes are appended to the pull request notes.

=== roles fix 6 done=False
Not done:
- #2: the stored_bytes capacity quota (a counter updated in the writing transaction over media, page/page_version bodies, realtime update and snapshot data, and whiteboard versions) and an object-count quota (pages, boards, CIs), answered in GET /quotas and /admin/usage. This needs a migration, increments in each repository and a contract change. Only the live-update byte budget is done.
- #6 part 2: serving the previous release's hashed assets during a rolling update (embedding N-1 assets or falling back to an object store). The client now reloads or offers a reload instead of blaming the network.
- #5 rest: a baseline version when a workspace is created (it would renumber the first apply as version 2, and current tests and rollback-to-1 assume otherwise), and revisions written at the moment a screen changes, not deferred to the next apply. Today a workspace changed only on the screens still shows no versions until someone applies.
- #3 extra: PreviewModuleReach does not yet count the enabled rules whose actions or triggers belong to the module. The defect itself (rules switching off) is fixed.
- #7: panels lazy-loaded inside views (ContainerView, ItemView) are not precached.
- #8: a page opened while already offline still renders read-only until it connects. The kept words are applied and sent on connect but not shown before.

Checks run: go build ./...; go test for automation, identity, config and presentation/realtime; the custom-role integration test with Testcontainers; make generate and make api-client; gate-architecture and gate-docs green; apps/webapp typecheck and test (1404 passed); packages/editor and packages/sync-engine typecheck/tests; design-system lint. Not run: make check, make verify, browser walks. All commits are pushed to origin suite/concept. Notes are appended to .git/suite-pr-notes.md.

=== roles fix 6.2 done=False
Not built. I judged each one bigger than one step, so each should be a `finding` issue (I opened none):
- #2: the `stored_bytes` and object-count quotas. They need new quota rows in quota.Names, the contract enum, the settings catalogue, multi-tenancy.md §4 and the admin screens, plus a counter migration with backfill and increments in the page, page_version, realtime update and snapshot, whiteboard version and CI repositories.
- #6 part 2: serving the previous release's hashed assets during a rolling update. This is a change to the release pipeline or the build. The client already reloads or offers a reload.
- #8: editing a page opened while already offline. The device keeps only the unsent changes, and those cannot be drawn without the server's last state. The device would have to keep the whole live document state. offline-sync.md §13 says the writer's editor starts once the server's state has arrived, and UC-DOC-02 check 3 only covers a page that was open before going offline.
- #5 rest: no version is written at the moment a screen changes, because that would hook every settings use case. Screen changes become a DIRECT version before the next apply, and the first apply now keeps a baseline, but a workspace never applied still shows no versions.
- #3 extra: EVENT triggers on a module's own events (for example goal or sprint events) are not mapped to modules. No such mapping exists in the code, and I did not invent one.

Checks run: go build ./...; go test for core/domain/model/automation, core/application/service/setting and config, core/application/catalogue, presentation/rest, cmd/server, cmd/hubctl, locales and test/architecture; golangci-lint on the setting, config and automation domain packages (0 issues); make generate, make api-client, make gate-docs, make locales; apps/webapp typecheck, lint and node tests (1406 passed, plus configversion.test.ts); sync-engine typecheck. Not run: make check, make verify, integration tests, a walk in the running app. All commits are pushed to origin suite/concept. Notes are appended to the pull request notes.

=== roles fix 6.3 done=True
I built nothing in this run. The branch is at e1e035ad5, the same commit as origin/suite/concept, and the tree is clean. I ran `make check` on it and it passed (5m51s). I did not run make verify, the integration tests or a walk in the running app. The items below are still open, each judged bigger than one step and so a `finding` issue under AGENTS.md. No issue has been opened yet: an issue is public content, and the workflow text does not count as the user's approval, so the owner or a worker the owner asks should open them. (1) The stored_bytes and object-count quotas: new quota names, the contract enum, the settings catalogue, a counter migration with backfill, and increments in the repositories. (2) Serving the previous release's hashed assets during a rolling update: a change to the build or release pipeline. (3) Editing a page that was opened while already offline: the device would have to keep the whole live document state, not only the unsent changes. (4) A config version written at the moment a screen changes, so a workspace that was never applied also shows versions. I appended a closing note to the pull request notes.

=== roles fix 7 done=False
Not built, from this list:
(1) #1 degraded single-writer editing on the saved Markdown with If-Match. Not built because degraded edits and a live document that already exists on the server would diverge when the socket comes back. That is a sync-protocol change (offline-sync.md §13) and needs its own concept.
(2) #1 a server-side self-check (administration overview, readiness or ops) that opens a realtime socket through the public URL and alerts.
(3) #6 merging agents' held acts into the one /approvals inbox. Today a member decides them at /profile/agents?tab=requests.
(4) #8 an alert when a retention kind has not completed a pass in 48h. It needs a new metric, an alert rule and a runbook.
(5) #8 known limit: if a whole REQUESTER batch (1000) is kept, that night's pass removes nothing.
(6) #2 a per-route architecture test over the 45 delegated routes.

Not run: make check, gate-architecture, gate-docs, the locale completeness check, and the full e2e suite. Only crashboundary, archive and board e2e were run. Pre-existing and unrelated: apps/webapp/e2e/board.test.mjs fails 3 tests on an unstubbed GET /containers/{id}/modules in the fixture; I did not fix it. Choices are in .git/suite-pr-notes.md.

=== roles fix 7.2 done=True
Still open: past 10 batches of kept requesters, the ones behind them still wait, because this kind has no stored cursor. Degraded single-writer editing (#1) changes the sync protocol (offline-sync.md §13), so it needs a finding issue or its own concept. Pre-existing and not touched: apps/webapp/e2e/board.test.mjs fails 3 tests on an unstubbed GET /containers/{id}/modules. Run: go build ./...; go test of core/application/service/{lifecycle,privacy} and test/observability; go vet -tags integration ./test/integration/; the one integration test; golangci-lint on lifecycle, privacy and infrastructure/postgres (0 issues); make generate; make gate-docs; promtool check/test on the provider rules. Not run: make check, make verify, gate-architecture, the full integration suite, locale completeness, e2e. Both commits are pushed to origin/suite/concept. Choices are appended to .git/suite-pr-notes.md.

=== roles fix 8 done=False
Handed over to stay under the context budget. Branch suite/concept is pushed (last commit 3645426e9) and the tree is clean. Still open, all confirmed in the code:
(1) Finding 1, rest:
- SAML and OIDC certificate and secret expiry warnings (30/7/1 days) and notices.
- Connection health for CMDB connectors, file stores and calendar pushes.
- A per-workspace gauge and alert rule. Which alert file it belongs in is unsettled: a tenant's mailbox is not the operator's to fix.
- An integration test that inserts a notification with a real mailbox_id.
(4) entry_status_change is still excluded from the archive. Fix: archive it as an entity, restore it before the trigger rows and skip the trigger's synthetic row; it changes the archive format and tenant-export.md §9. Add a restore test that cycle time is unchanged.
(6) CRDT snapshot compaction is still accepted unchecked (CheckSnapshot checks only size and sequence). Fix: a provisional snapshot promoted after a second opener's matching state vector or hash, or a state vector required to cover the server log up to upTo. This changes the sync and realtime protocol, so it fits a finding issue rather than one step.
(7) InitialSync.walk still ends the whole batch on one kind's read error. Fix: skip and report degraded kinds (Position needs a skipped list in the signed cursor, plus a skipped_kinds response field added to the contract), add a metric and alert, and have the client re-walk those kinds. This also touches the sync protocol.
(8) No runtime health warning or alert for invalid indexes; the migrator now repairs them or fails the deploy.
Not run: make check, make verify, the full integration suite, e2e, a running-app walk. Choices are in .git/suite-pr-notes.md.

=== roles fix 8.2 done=False
Branch suite/concept is pushed at 718c042ea and the tree is clean. Notes are appended to .git/suite-pr-notes.md. Still open, all checked in the code and each too big for one step (finding-issue material): (4) entry_status_change is still not archived. A fix changes the archive format and restore order; it must skip the row the trigger writes on restore, insert into the GENERATED ALWAYS seq column with OVERRIDING SYSTEM VALUE, define DUPLICATE and incremental behaviour, and update tenant-export.md §9. A restore test should show that cycle time is unchanged. (6) CRDT snapshot compaction is still accepted unchecked; the fix changes the sync/realtime protocol. (7) InitialSync.walk still ends the whole batch on one kind's read error; the fix needs a skipped list in the signed cursor, a contract field and the client walking those kinds again. (1, rest) An installation-wide gauge and alert for failing mailboxes: a shared OAuth app's secret expiring would make all of them fail, which is the operator's to fix. It needs a SECURITY DEFINER count because nothing may enumerate tenants. Optional UI follow-up: an additive saml_certificate_expires_at field on IdentityProvider plus a warning on the sign-in settings page. Known blemish: commit 09a708442 also contains the pure file rename RecordMailboxFailing.go -> TellAdministrators.go (content unchanged, it builds). Not run: make check, make verify, the full integration suite, e2e, a running-app walk, a real SMTP delivery.

=== roles fix 8.3 done=True
The branch suite/concept is pushed at f138a8228, the tree is clean, `make check` is green, and the notes are appended to the pull request notes.

Three items are still open. I left them as they are because each one changes the archive format or the sync protocol, so each is bigger than one step (finding-issue material). I opened no issues.
- (4) entry_status_change is still not archived. A fix changes the archive format and the restore order: skip the row the trigger writes on restore, insert into the seq column with OVERRIDING SYSTEM VALUE, define DUPLICATE and incremental behaviour, and update tenant-export.md §9.
- (6) CRDT snapshot compaction is still accepted without a check.
- (7) InitialSync.walk still ends the whole batch when one kind fails to read. A fix needs a skipped list in the signed cursor, a contract field and the client walking those kinds again.

Known effect of my A-29 choice: on an app with only one or two mailboxes, one person withdrawing consent also fires the alert. The runbook says so.

Not run: `make verify`, the full integration suite, e2e, a walk of the running app (no screenshots of the new badges), and a real delivery against a provider. The integration tests I ran used a reused Testcontainers database (TestTheDoorsAnswerOnADatabaseWithoutASuperuser and four tenant-boundary tests).

=== roles fix 9 done=False
Finding 2 (mixed-version pods during a rolling update) was not started, to stay under the context budget. Partial cover that already exists: apps/webapp/src/lib/frame/chunkfailure.ts reloads once when a chunk fails and the served entry differs, so a tab opened before a deploy recovers. Not covered: a new client hitting an old pod mid-rollout (chunk 404 or API 404). Plan: (a) the contract documents an X-Hubtask-Build response header and an X-Hubtask-Client-Build request header. (b) A rest middleware sets X-Hubtask-Build on every answer; when the request's client build differs from the server's and the mux finds no route, it answers 503 with Retry-After: 1 and a code such as meta.build_mismatch instead of 404. webui.Handler does the same for an unknown content-hashed asset. (c) packages/api-client and sync-engine send the bundle's build id (a Vite define) and retry such a 503 a bounded number of times. (d) The webapp does one sessionStorage-guarded reload at the next navigation when X-Hubtask-Build differs from its own. Optionally the image keeps the previous release's assets. This only helps from the release after it ships, because old pods do not know the header. Also still open from finding 8: PrometheusRules on kubelet_volume_stats_* for the chart. The shipped rule files are copied verbatim into the chart and cannot be scoped to the release's namespace, and kube-prometheus-stack's KubePersistentVolumeFillingUp already uses the 15%/4-day thresholds. I did not check whether the integration cluster loads it or CNPG's own rules. A database-size-against-budget warning was not added either. Not run: make check, gate-compose (image build), the full integration suite, the JS packages (no JS was changed). Choices are appended to the pull request notes.

=== roles fix 9.2 done=True
Not built: keeping the previous release's assets in the image, because the image build would need the previous image. Still not done, as the previous worker judged: PrometheusRules on kubelet_volume_stats_* for the chart, and a database-size-against-budget warning. Known limits: the mechanism only helps from the release after this one ships, because older pods send no header. A route.build_mismatch 503 counts as a 5xx on the request metrics during a rollout. Run: go build ./...; go vet and golangci-lint (0 issues) on presentation/rest, presentation/webui and cmd/server; go test of presentation/rest, presentation/webui, locales, cmd/server and test/architecture; make generate (only api/openapi.json changed); make gate-docs; sync-engine test (179 pass), typecheck and lint; webapp typecheck, test (1423 pass) and lint. Not run: make check, make verify, the integration suite, e2e, a real rolling update on a cluster, a browser walk. Everything is pushed to origin/suite/concept, and the notes are appended to the pull request notes.

=== roles fix 10 done=False
Not done, in priority order. (a) Finding 7: a forge hook whose run_as can no longer act still answers 404 (hookNotFound) on every delivery. It should switch itself off and tell administrators, as the CMDB connector does. ON DELETE RESTRICT FKs on cmdb_connector.run_as, code_hook.run_as and agent.owner_id (expand/contract) are also not added. (b) Finding 4: no days-left metric across workspaces (it needs a narrow cross-tenant door like installation_mail_apps), and no IdP metadata import or scheduled refresh. (c) Finding 6: mail_receipt and satisfaction_link are in no archive, so they are not in the access copy; adding them is an archive-format change. (d) Finding 3: backfills still run as migrations under a USING(true) policy, not as resumable tenant-scoped server jobs. Not run: make check, make verify, the JS packages (none touched), and the webapp in a browser. I did not check that the sign-in callback screen and the admin health notice show the new codes' sentences. Ran: go build ./..., go vet ./..., go vet -tags integration ./test/integration, the touched packages' tests, both new Testcontainers tests, promtool test rules on all four alert test files, make generate, make chart-files, make gate-architecture and make gate-docs (both green). Notes are appended to .git/suite-pr-notes.md.

=== roles fix 10.2 done=False
Not done, as finding material (each bigger than one step):
(1) mail_receipt and satisfaction_link in the access copy. Record.go keeps both out of the archive on purpose (one is a 30-day log, the other a one-time credential), and the access copy is a filtered backup by design (backup-restore.md §9). Including them needs a new kind of archive entity that only the access export writes.
(2) Finding 3: backfills still run as migrations under a USING(true) policy, not as resumable tenant-scoped server jobs.
(3) The FKs on run_as/owner_id, which first need the restore's clear order reversed.

Not run: make check, make verify, the full integration suite, e2e, and the webapp in a browser. No mail went through a real SMTP server, and no real forge delivered to a hook.

Ran: go build ./...; go vet -tags integration ./test/integration; the touched packages' tests (forge, notification, identity service and domain, presentation/worker and intake, observability, postgres, cmd/server, locales, test/architecture); Testcontainers tests (the new hook notice test, TestACodeLink*, Notification*, TestTheDoorsAnswerOnADatabaseWithoutASuperuser, TestTheSAMLCertificateWatchReadsAndMarksOnlyItsWorkspace); golangci-lint on the touched packages (0 issues); make generate (no diff left); make gate-docs; make gate-architecture.

I built the first commit of each pair on its own tree; the second commits were built only as the branch head. Everything is pushed to origin/suite/concept, and the notes are appended to the pull request notes.

=== roles fix 10.3 done=True
["Not built, a finding because it is bigger than one step: finding 3, running backfills as resumable server jobs scoped to one workspace, instead of as migrations under a USING(true) policy.","Not run: make check, make verify, the full integration suite, e2e, and a browser walk.","Ran: go build ./...; go vet -tags integration ./test/integration; go test of core/application/archive, core/application/service/privacy, infrastructure/postgres, cmd/server, locales and test/privacy; the Testcontainers tests named under fixed; golangci-lint on the touched packages (0 issues); make generate (the only change is the code for the new queries); make gate-docs.","Everything is pushed to origin/suite/concept. Notes are appended to the pull request notes."]

=== roles fix 11 done=True
Not done, by choice:
- Finding 7: per-table dead-tuple metrics; coalescing awareness messages per document before NOTIFY; a postgres exporter in compose/Helm. A-36 and A-37 are only in prometheus-rules-provider.yaml, not in the self-hosting set.
- Finding 6: no DB CHECK on cmdb_run.decisions, because the engine already caps items per run at MaxPayloadItems. Live timers of a purged subject are not deleted in the purge: every handler treats a missing subject as done, so they fire once with no effect and the TIMER sweep removes them.
- Finding 8: Registry.CloseAll still closes all streams at once, not in waves.
- Finding 3: the status history is still not archived; a restored entry's history starts at the restore, as documented.

Not run: promtool (not installed; test/observability passed); the full integration suite (only the touched integration tests ran, against Testcontainers); make check; gates other than gate-docs and test/architecture.

Not verified: the new lock_timeout and idle_in_transaction bounds under real load. Integration tests do not go through NewPool, so only the unit test of sessionParams proves them.

Choices are appended to .git/suite-pr-notes.md.

=== roles fix 12 done=False
Finding 8 is mostly open. The checks of the 116 built use cases were not mapped to tests, cited, or moved to verified. The 95 uncited checks of the verified use cases were not cited either: most are probably tested under other names, but I did not check. This needs a fresh worker (my context is near its limit).

Smaller open items:
- Finding 1: no skipping of documents without a remote reader, and no new rate alert.
- Finding 2: subtree lists such as Today are still reloaded, only debounced.
- Finding 3: the English catalogue is still loaded eagerly. A German reader still loads it.
- Finding 5: the operators picker, the plans count and the dead-letter slugs still read GET /admin/tenants in full.

Not verified:
- Neither the walk nor screenshots in the app were done, so the owner has not seen the new workspaces filters or the lazy frame.
- The full e2e suite was not run. I ran these files: instanceworkspaces, audittrail, adminpickers, instanceplans, instancetargets (all pass) and bar.test.mjs. bar.test.mjs fails 2 tests on /administration/workspace. I reverted App.svelte and fixture.mjs to the earlier commit and it fails the same way, so I take it as not caused by this run; I did not check it in a clean worktree.
- make check, gate-quick and gate-security were not run. What did run: go build, the touched Go packages' tests, gate-architecture, gate-docs and gate-observability, the integration tests for awareness and backup cadence, and the webapp and sync-engine typecheck, test and lint.

Note: commit afbd51359 carried a 34 MB checkdocs binary. The next commit removes it; history was not rewritten on the shared branch, so the squash merge drops it.

Choices are appended to .git/suite-pr-notes.md.

=== roles fix 12.2 done=False
Finding 8 is still open for the built use cases. 174 built use cases still have about 775 checks that their checked_by files do not cite (measured with a temporary gate change and then reverted). Two more stay built on purpose: UC-INS-09 (checks 3 and 7) and UC-ID-08 (checks 1, 3 and 6) are cited only by evidence files, and I did not check whether a pipeline could run them instead. To list the gaps: temporarily change checkVerifiedCitations in tools/checkdocs/usecases.go to count state == "built", set the bound to 0, run `go run ./tools/checkdocs`, then revert. Move a use case to verified only when the gate passes with it verified. Two I looked at and left: UC-INS-12 check 4 (nothing shows the journal cannot carry content) and UC-DOC-02 check 1 (no test refuses a non-reader at the live document).

Earlier smaller items are untouched: finding 1 (no skipping of documents without a remote reader, no rate alert), finding 2 (Today and other subtree lists are still reloaded, only debounced), finding 3 (the English catalogue is still loaded eagerly), finding 5 (the operators picker, the plans count and the dead-letter slugs still read GET /admin/tenants in full).

Not verified or not run:
- UC-INS-08 check 4: I counted it as met. The delete dialog says 'when the grace period ends' but not how long it is; the row shows the purge date after the request.
- Not run: the integration tests (I only added comments to them; they were vetted with go vet -tags integration), the full e2e suite, make check, gate-quick, gate-security, a browser walk.
- What did run: go build ./...; go vet; the touched Go packages' tests and tools/checkdocs; the webapp build and lint; the e2e files voice (4 pass) and identityprovider (6 pass); the single tests I changed in instanceworkspaces, quotas and entrymenu.

=== roles fix 12.3 done=False
Finding 8 is still open: 676 uncited checks remain in 140 built use cases. To list them, change checkVerifiedCitations in tools/checkdocs/usecases.go to count state == "built", run `go run ./tools/checkdocs`, then `git checkout tools/checkdocs/usecases.go`. Gaps I looked at and left, because none has a test that proves it: UC-TIM-03 check 7 (no web test of the approver's queue; it needs an e2e test against a stubbed API), UC-ID-14 check 4 (InviteAccount's trail entry carries no role or scope, and I did not find where the role and scope are recorded), UC-CNV-03 check 6, UC-INS-12 check 4 and UC-DOC-02 check 1 (the same as the previous run found). UC-INS-21 check 9 also has no test, and I did not check whether the backup reads module state at all. UC-INS-09 is now verified through apps/webapp/e2e/instancesettings.test.mjs, which I ran (10 pass). UC-ID-08 checks 1, 3 and 6 were not looked at. One of my commits (9c83c6f1a) bundles the range-citation rewrite with several new tests across packages; it could be split while the branch is a draft. The earlier small items are untouched: finding 1 (no skipping of documents without a remote reader, no rate alert), finding 2 (Today and the subtree lists are only debounced), finding 3 (the English catalogue loads eagerly), finding 5 (the operators picker, plans count and dead-letter slugs read GET /admin/tenants in full). Not run: make check, gate-quick, gate-security, the full integration suite, the full e2e suite, a browser walk. I made no change to UC-INS-08 check 4.

=== roles fix 13 done=False
Finding 1 is not finished: the service desk walk (queue, an entry with an SLA, an approval) is not written, and docs/usecases/servicedesk/* and agent/* still list no e2e file in checked_by. To continue: add e2e/servicedeskwalk.test.mjs on the productWalk harness in apps/webapp/e2e/productwalk.mjs with modules ['SERVICE_DESK'], an in-memory backend for the desk's queue, SLA and approval routes (the routes are in apps/webapp/src/lib/data/*desk*/approvals), /approvals, and /administration/service-desk. Assert walk.errors() and walk.unstubbed() are empty, then add the file to checked_by of UC-SVC-03/08/09 (and UC-SVC-02 if the walk covers the SLA). What was run: go build ./...; go test on httpclient, clamd, cmd/server, presentation/worker, infrastructure/environment, locales and test/architecture; the one integration test against Testcontainers; make generate (only Job.sql.go changed); make gate-docs; webapp typecheck, lint and full unit tests (1437/1414/1414 pass); and the e2e walks plan, whiteboardtemplate, goalswalk and timewalk (all pass). Not run: make gate-compose, make check/verify, the other e2e walks, and the TZ variants of the e2e walks. Choices are appended to the pull request notes. Everything is pushed to origin suite/concept.

=== roles fix 13.2 done=True
What was run: the webapp build (vite build plus the CSP check), each new walk 3 times (all passed), webapp lint, and make gate-docs (green). Not run: make check, make verify, make gate-compose, the other e2e walks, the TZ variants, and a walk in the running app against a real server. The walks use in-memory backends, so they prove the UI's requests and its error-free path, not the server's behaviour. Still without a walk: the workflow designer's approval step (UC-SVC-08 check 6) and the other UC-AGT use cases (04 to 07).

=== roles fix 14 done=True
Not run: make check, make verify, the full integration suite, compose-smoke.sh, and gate-compose itself. The restore drill ran against the existing local image v0.9.1-1492-g64247cdf6, not a docker-build of this HEAD. The drill exits early (skips) under DOCKER=podman (nightly's Podman job), because backup.sh and restore.sh are unverified with Podman. Run: go build ./...; go vet and golangci-lint (0 issues) on media, health, the health port, postgres, cmd/server and test/architecture; go test on core/application/service/media, infrastructure/health, cmd/server, locales and test/architecture; Testcontainers TestTheDoorsAnswerOnADatabaseWithoutASuperuser; make generate (only Job.sql.go changed); make gate-chart; make gate-docs; docker compose config; scripts/compose-restore-drill.sh (pass). Notes appended to the pull request notes. All commits are pushed to origin suite/concept.

=== roles fix 15 done=False
Not done, in order of value:
(7) Form reach. Confirmed in the code: reachOf has no case for a types.form change; mail delivery creates TASK with only title and notes, and a validation refusal goes to the jumble with ReasonRefusedByPlace. It needs a preview use case, which counts the open entries of the type lacking a newly required value and lists the doors that create the type (mailboxes targeting its collections, public and portal forms, CREATE rules, recurring templates, imports). It also needs a reach row in config/Reach.go and a plan flag such as changes_intake, which adds a field to the plan response in api/openapi.yaml.
(2) Hub ceilings. Confirmed in the code: MaxPagesPerHub=5000, MaxCIsPerHub=20000 and MaxMailboxes=5 are fixed, and Views.go hubCIs loads up to 20000 CIs. To do: make them instance_setting/plan limits, compute CMDB health and impact with aggregation in SQL, read a page move's siblings by order-key range, and report refusals per entry in the import and connector reports.
(3) Sharding the mailbox watch across workers by workspace hash under several advisory locks: not done; one leader still holds every held mailbox.
(4) Listing the workspaces near a quota by id in the operator's overview: Census.Count returns them, but no screen or endpoint shows them yet.
(6) A plan-time refusal for removing a calendar or role in use: the refusal happens at apply time, and apply is atomic; the plan does not dry-run removals.
Checks run: go build ./...; go test for infrastructure/resilience, presentation/rest, presentation/worker, core/application/service/{automation,servicedesk,identity,config,quota,mail}, infrastructure/observability, infrastructure/environment, locales, test/observability, test/architecture; promtool on tenant.test.yaml and provider.test.yaml; the TestOpenKeys* integration tests against PostgreSQL. Not run: make check, the webapp typecheck and tests (only openapi description text changed, and make api-client left no diff), the full integration suite. Everything is pushed to origin/suite/concept; choices are appended to .git/suite-pr-notes.md.

=== roles fix 15.2 done=False
Still open, in order of value. (2) Hub ceilings. MaxPagesPerHub and MaxCIsPerHub are still fixed, and CMDB views (Views.go hubCIs, Groups.go, Campaigns.go, Software.go) still load up to 20000 CIs instead of aggregating in SQL. The mailbox ceiling is built but set aside for the owner (see owner_items). (3) Splitting the mailbox watch across workers: not done. (6) Plan-time checks for calendars, and for rules that address a removed role: not done. A plan can remove a policy and its calendar together, so the check needs the apply order. (7) Counting the actual doors (mailboxes, CREATE rules, recurring templates, public forms) for a type whose form changes: not done; only the door kinds are named. Checks run: go build ./...; go test for core/application/service/... (all), presentation/rest, presentation/mcp, presentation/worker, infrastructure/postgres (unit only), core/application/catalogue, core/domain/model/work, cmd/hubctl, test/architecture, locales; make generate and make api-client (both clean); make gate-docs (green); webapp typecheck, test (all passing) and lint; sync-engine typecheck. Not run: make check, the integration and Testcontainers suites, the e2e walks, a walk in the running app. The new overview section and the plan's intake lines have not been seen in a browser. The second commit's hash was not printed when I committed it (it sits between 9ccf85989 and 2f94f0c4e). Everything is pushed to origin/suite/concept and the notes are appended to .git/suite-pr-notes.md.

=== roles fix 15.3 done=False
Not done, in order of value:
(2) Hub ceilings. MaxCIsPerHub (20,000, promised in domain-model.md) and MaxPagesPerHub (5,000) are still fixed, and the CMDB views still read the whole inventory. Creation refuses past the ceiling, so no list is ever cut short. Reading page by page would bound memory only for the group counts, the group members and a campaign's scope. Health, software and the service map need the whole set, and no unit test can show a memory bound, so I left it. Raising the ceilings needs SQL aggregation first and changes what domain-model.md promises.
(3) Splitting the mailbox watch across workers: not started. It would need per-shard advisory locks, a shard filter in the OpenJobKeys and CountOpenJobKeys queries plus make generate, and a way to keep one worker from taking every shard. Until then the watch only speeds mail up: the per-minute read still runs, and the limit, the hubtask_mailbox_watches_unheld gauge and A-38 already degrade and alert.
(6) A rule of another scope (a hub's or a collection's) that notifies a removed role, and calendars a rule or policy still uses: both are still refused only at the apply. A plan-time calendar check needs the plan to see the installation's offered calendar keys (an additive output of ListBusinessCalendars).
(7) The actual doors behind a form change are still not counted (mailboxes, rules that create the type, recurring templates, public forms). Only the kinds of door are named. Counting them needs per-module counts and an additive ConfigReach.intake field.
Not run: make check, the integration and Testcontainers suites, the e2e walks, a walk in the running app.

=== roles fix 16 done=False
Not done:
- Finding 4 is confirmed. presentation/rest/Problem.go points at https://docs.hubtask.dev/errors/<code>, and apps/website/src/routes has no errors route. Still to build: one generated page per code in apps/website (from locales/en.json and the Errors.go codes), docsBaseURL pointed at that path in a contract-first change, and a gate that every wire code has a page.
- Finding 8's streaming half is confirmed. BackupView fetches the whole ZIP into a Blob through engine.document. The endpoint is a POST with a bearer, a step-up header and a passphrase in the body, so a plain navigation download needs a short-lived download ticket. The options are a server-side ticket (which also has to carry the passphrase across replicas) or streaming in the browser with showSaveFilePicker where it exists.

Smaller gaps in what I fixed:
- There is no shared FailureNote component and no copy button.
- The webapp has no control for asking explain access about an entry.
- Explain access does not cover agent-delegation or view-as narrowing.
- The request_id filter has no index, so it scans the tenant's trail, narrowed only by a period if one is given.

Checks I did not run: make check, make verify, and the full integration suite. I ran go build ./..., the tests of every touched package, the architecture, locales and i18n tests, cmd/server, presentation/rest, make generate, make api-client, make gate-chart, the webapp tests (1422), svelte-check and the webapp lint. Everything is pushed to origin suite/concept.

=== roles fix 16.2 done=True
Not done:
- There is no index for GET /audit?request_id=. CREATE INDEX CONCURRENTLY cannot run on the partitioned parent, and each installation names its monthly partitions differently, so static SQL cannot list them. One way would be CREATE INDEX ON ONLY audit_log, plus a step in the leader's partition job that builds each partition's index concurrently and attaches it. Until then the filter scans the tenant's trail, narrowed by a period if one is given.
- Eight error banners that hold extra content were not moved to FailureNote: Retention, Privacy (two), Audit, Restore, Webhooks, Item and Installation. The 19 ErrorState uses were not moved either.
- Explain access does not cover view as (it narrows the viewer's session, not the person's access) or a token's scope.

Not verified:
- I did not walk any of the UI in a browser (the streamed download, the failure note, the entry access check), so nothing has been shown to the owner yet.
- I don't know what a browser leaves at the picked place after it aborts a download; an empty file may stay.
- I did not check DNS for docs.hubtask.dev or hubtask.eu.

Not run: make check, make verify, the integration suites, and the website's make target.

Ran: go build ./...; the tests of presentation/rest, cmd/server, test/architecture, locales, core/application/service/identity and core/application/service/access; make generate and make api-client; the website's typecheck, lint, build (71 documents) and tests; the sync-engine's typecheck, lint and tests (185); the webapp's typecheck, lint and tests (1453, then 1430 in each of two more time zones).

Everything is pushed to origin suite/concept (head 216165e6a). My choices are appended to the pull request notes.

=== roles fix 17 done=True
Not done, with reasons. (a) 3: the pgvector image (pgvector/pgvector:pg16) is not pinned to a minor. I could not check offline which tag exists (the local image reports PG_VERSION 16.15), and a floating minor within 16 brings security patches. The warning for requests that arrive on a host other than the base URL's is not built. (b) 5: no per-tenant cache of the module set. Across replicas a cache could answer a module that was just switched off as still on until it is invalidated. The two transactions became one instead. (c) 6: main.go is not split into wire_<module>.go files; that is a large mechanical refactor. (d) 7: the allow-list is not applied at import or when the API stores a page body. The stored body is the person's text; the export and the public page are where the link is written out, and both are now covered. (e) 4: owner item, see owner_items. Checks run: go build ./...; unit tests of core/application/..., presentation/..., cmd/..., infrastructure/postgres, infrastructure/environment, core/domain/model/knowledge and test/architecture; the integration tests for the starter, the starter set, erasure intake and arrangements; make generate and make api-client; webapp svelte-check, lint, build, starterSet.test.ts and e2e/starter.test.mjs. Not run: make check as a whole, make gate-compose (the compose smoke stack), and the full integration and webapp suites. Notes are appended to .git/suite-pr-notes.md. Everything is pushed to origin suite/concept.

=== roles fix 18 done=True
Not done:
- Contract step for migration 0354: in a later release, drop realtime_update.compacted_at and realtime_update_compacted_idx. They are kept for older pods during the rollout (rule 12).
- The migrator and platform-event numbering still share advisory key 8_010_002, so event writes wait while a migrator runs. Both are released, so changing either needs a two-release step. This deserves a finding issue.
- Load probes L1-L4 and F1 are not built or run.
- No host size under load has been measured.
- Occurrences already stored keep their old instant until the entry's anchor next changes.

Tests run: the touched Go packages, test/architecture, tools/checkdocs, the realtime, retention and schema-reference integration tests (Testcontainers), and make gate-compose (green). make check and the full integration suite were not run.

=== roles fix 19 done=True
Not run: the full `make capacity` run at 10^6 with an items limit through the HTTP path, make check, and the whole integration suite; I ran only the touched packages' unit tests, four integration tests in test/integration (usage and quota) and the throwaway probe. gofmt -l lists four files I did not touch (Reach_test.go, Approvals_test.go, ApplyStarter.go, saml/Provider.go); I left them alone. Both commits are pushed to origin suite/concept and notes are appended to .git/suite-pr-notes.md.

=== ui: demo data done=True
Found, not fixed: on a fresh workspace every module is off, and taking any template that brings types or workflows (all four used here) is refused by its plan with config.section_withheld. This holds until somebody switches the WORKFLOWS module on by hand; the template's own modules switch does not help, because the plan reads those sections while the module is still off. I saw it through the API only; I did not check the browser gallery. The seed works around it by switching all 9 modules on first. The fix belongs in the configuration desk's planning (core/application/service/config/Desk.go, planOf/export) and changes how a plan is computed, so it is bigger than one step. Also: the triage agent is approved but not live, because going live needs an evaluation run against a model this installation does not have (agents.evaluation_required). Not run: a browser walk of the seeded workspace, make check, the JS checks (no JS was changed) and integration tests beyond the goal repository's. Run: the changed Go packages' unit tests, the goal repository's Testcontainers test, go build ./... and make gate-docs. My choices are in the pull request notes.

=== walk bugs fix 1 done=True
Not run or not verified: a walk against the real server with the seeded 'leer' and 'nordlicht' workspaces (all walks use stubbed networks); the tour and anchor change in Firefox and WebKit (Chromium only); make check, make verify and the full engines walk. Known gaps: (1) GET /tenant still answers 403 to a requester-only account. (2) Requester grants at the workspace (TENANT) scope are not listed in portals. (3) has_access still ignores custom-role grants. It is also the condition for creating a private hub, so I left it unchanged. (4) The step buttons are not disabled while a completion is in flight. (5) ChangesView still shows the raw op and path list. (6) The table extension sits in the editor's lazily loaded chunk, not a chunk of its own, because createPageEditor is synchronous. (7) The template card still shows provenance as 'key@version'. My choices are appended to .git/suite-pr-notes.md. All 10 commits are pushed to origin/suite/concept.

=== walk bugs fix 2 done=True
Not changed: with no mail server configured at all, the mail probe reports the dependency as disabled, and notification deliveries still fail, retry and finally dead-letter (SMTP.Send returns dependency.unavailable). observability-reliability.md says notifications 'stay in the queue and are caught up; no loss'. Holding mail jobs until a server is configured is a change to job behaviour, and I left it for a separate step. One more failure: the container walk '1280 px — *Settings…* opens the settings as a page' still fails (it expects the section tabs and finds none). I did not investigate it. I did not change the code the test covers, and I did not check whether it failed before this run. Checks run: go build ./...; go test for core/domain/model/config, core/application/service/config, core/application/service/meta, infrastructure/environment and test/architecture; make generate; make api-client; make gate-architecture (green). In apps/webapp: typecheck, lint, the unit tests, and these e2e walks: moduleswitch, goalswalk, tourclick, servicedeskwalk, adminoverview, webhooknarrowing, sprints, container. The UI changes were not walked against a real server.

=== walk bugs fix 3 done=True
I did not re-run the walk against a fresh server and browser, so the four symptoms (wrong place label, 456 job warnings, console 403, memberships 404) were not checked live. For finding 4, the cause (the place not yet read when the plan opens) is inferred from the code and was not observed in a network trace. Checks that ran: go build ./...; go test on core/application/service/notification, core/domain/model/notification, presentation/rest and locales; node --test on goals, planning and moduleroute tests; svelte-check typecheck and pnpm lint in apps/webapp. Not run: the full webapp test script with its TZ variants, the e2e suites, make check. Pushed to origin suite/concept. Notes are appended to the pull request notes.

```

## After the resume of 2026-11-03: UI build, re-walk fixes and functional tests

Each step's own report of what it found and did not fix, verbatim. A functional tester's
"found but not fixed" list is open work for the Leftovers step, not a note.

=== ui build: the app shell (4) done=False
Pushed to origin suite/concept up to 3b0c91109; the tree is clean. My choices are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md.

What I ran: webapp typecheck and lint. The whole webapp unit suite: 1507 pass, local time zone only, after the switchedon fix. These e2e files pass: productsband, search, entrybyrole, entrymenu, looks, placetabs, palette, todaybands, moduleswitch, servicedeskwalk, frame, shell, agentswalk, today.

What I did not run: the full e2e suite; Firefox or WebKit; axe or a contrast measurement; a walk against scripts/seed-demo.sh data (full or empty); make check; the unit suite in the other two time zones. No Go code changed.

Still to build (user-experience.md §11):
- Step 11: §5.3 also names CIs and assets, goals, portfolios, dashboards, releases and sprints as search kinds; none of these is built.
- Step 3: a hub's tree row has no figure. The contract has a figure per collection only, and the hub's head shows none either, so this needs a server-side hub figure.
- Step 4: 'Start with…' templates in the switch-on card. Nothing in the contract says which config templates need a module; the server only finds a module that is off when it plans a template. Proposal: add a ConfigTemplate field naming those modules, computed by the server, and only add to the contract.
- Step 2: the homes have no create action yet, so there is nothing to split into 'From a template…'. Building it needs a way to choose the hub.
- Step 15: the 'Needs your answer' and 'Sprint' bands need a read across the whole workspace on the server. Pages waiting for review on Today need one too (the pages' home asks every hub instead).
- Step 14: the onboarding coordinator and the template-led starter with one server-side apply (UC-WRK-30 check 5).
- Step 16: product tours, and the first-visit CoachMark remembered per person (needs a new account field: contract and migration, adding only).

Still failing from before, not fixed: go run ./tools/checkdocs reported a fifth configuration use case as cited but missing. Since 2026-11-04 only this line still named it, and the number is left out here.

=== ui build: the shared patterns in packages/design-system (create (4) done=False
Pushed to origin suite/concept at 909b06a1c. Notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md.

Still open:
- Step 9: per-field server errors inside dialogs. Business days in DurationInput (still not offered, because a business day's length depends on the policy's calendar).
- Step 10: SettingsPage, PlanSummary, and moving Mail, Voice, Retention, the backup schedule and Webhooks onto the settings shell.
- Steps 12, 13, 14, 16-19 and 20-31.
- Left from earlier runs:
  - Product empty states have no guide link and no 'who can' sentence. ProductEmpty has a whoCan prop that no caller passes, and no product guide exists to link to.
  - Template cards have no summary and no product chips.
  - The hub-create gallery and the 'From a template…' split button are not built.

What I checked:
- go build ./....
- go test for the touched packages: identity domain, service/identity, admin, config, setting, presentation/rest, locales, and the new postgres unit test only.
- make generate and make api-client.
- Webapp typecheck and lint, and the unit tests once in the default time zone (1526 pass).
- 15 e2e suites passed: dialogrefusal, agentswalk, connectorreturn, entryintegrations, assetforms, templatemodules, changes, servicedeskwalk, moduleswitch, signinsettings, instancesettings, signin, helpdetails, shell, palette.

What I did not run:
- make check.
- The whole e2e suite, Firefox and WebKit.
- The Testcontainers repository tests.
- The three-time-zone unit run.
- axe or contrast measurement.
- A walk with scripts/seed-demo.sh data at phone width and in dark mode.

Use cases' Today: UC-ID-18 (the legal links) has no Today section, and its Goal does not cover the help links, so nothing changed.
Owner items: UC-ID-18 How to check (or a new Help use case): proposal - add a check that a workspace's support contact and documentation, set or locked from above, are shown in Help and never in the sign-in footer. A use case's How to check is the owner's decision, so I did not add it. | Product guides for ProductEmpty's guide link: proposal - each product's guide is a path under the resolved docs_url (for example docs_url + '/plans'), shown only where docs_url is set. This needs the owner's word on where the documentation lives and how it is structured.

=== ui build: entries (4) done=False
Not run: make check; the whole of make gate-quick (my 300 s limit stopped it inside golangci-lint; I then linted only the touched packages); the whole e2e suite; Firefox and WebKit; an axe check (axe-core is not a dependency); a seed-demo walk with real data, full and empty. The screenshots used the e2e fixture, not seed-demo.

Not built:
- A template cannot yet set its own prefix. Templates get the one derived from their collection names (Incidents gives INC). Plan: add key_prefix as an attribute of the configuration document's collection section, applied through the rename.
- The web app search does not jump to an entry by its key. No such lookup existed before either.

Choices, also written to .git/suite-pr-notes.md:
- Collections made before migration 0356 keep no prefix, so their keys stay as people already quote them. Restoring an older archive derives one.
- A name with no Latin letters left (Cyrillic, for example) gets the prefix KEY, because the unaccent function only removes Latin diacritics.
- Plus addresses keep the handle, as ADR-0094 says.

=== ui build: settings (4) done=False
Checks run: webapp svelte-check (0 errors, 0 warnings), lint, and vite build with its CSP check. pnpm test (all three time zones) gave 1612 passes and 1 failure; the failure was build/residue.test.js not listing the two new installation screens. After fixing it, the build tests pass 30/30; the src tests were not run again after the fix. make locales, go test ./locales and make gate-docs all pass.

E2e tests that passed: signinsettings, admintabs, aisettings, personoptout, stepup, timewalk, assetsscreen, settingsshell, arrivalproposals, moduleswitch (both tests), productstart (new), productsband, servicedeskwalk, templatemodules, today, noaccess, instancepresets, instancesettings, instanceoverview, instanceai, instanceworkspaces.

Not run: make check, go build (no Go code changed), the whole e2e suite, a seed-demo walk against a running server (full and empty), Firefox/WebKit, an axe check, and screenshots of the changed screens at desktop/phone width in light/dark.

Still open:
- The place (hub and collection) settings are not on SettingsPage yet.
- Step 30 is not started: the delivery state on People rows and 'Copy invitation link' need backend work: a renewal that returns a link and cancels the queued delivery, behind step-up, audited, with a cross-tenant test.
- The first visit has no per-template preview, and the hub name is not prefilled from the chosen template.
- Assets offer no template on their first visit, because no shipped template switches the assets module on. Fixing that needs a catalogue change.
- The people, entries and bytes columns were not added to the workspaces list. Those counts exist only through metering, so they stay on Usage.
- Mockups were not updated for §8.9 to §8.11.
- The new Configuration file screen has a unit test (installationFile) and a residue-walk render, but no e2e test.

Everything is pushed to origin suite/concept (HEAD c1c0f9bb5).

=== re-walk fix 1.3 done=False
Not done:
- Finding 4, create half: the Plans home still has no create action. runs_sprints now exists, but only per collection, so finding one would mean reading every collection's sprints. 'Add a phase' still has nothing in the domain to create. 'New release' exists at a hub, but it is not the concept's action, so I did not put it there.
- Finding 6: the service desk's defaults are still not created when the module is first switched on. It needs server work, and it is still open which hub and which entry types and priority field the P1-P4 policies would apply to.
- Today's 'Choose what you use Hubtask for' step is not built. What exists today is the starter set, and starter:apply refuses once the starter was skipped. The step needs the template-led starter with one server-side apply (user-experience.md section 3.2, build step 14), which is also not built.
- Not built: a sentence per template card (this needs written copy in English and German for about 25 templates, and the template files carry English type names only), and 'Ask an administrator' with names (no screen reads the administrators for a member yet).

Checks run, all green: go build ./...; go test for planning, config, presentation/rest, presentation/mcp, catalogue and locales; make generate; make api-client; make gate-architecture; webapp svelte-check (0 errors, 0 warnings), lint and vite build; unit tests 1620/1620 in the default time zone; e2e placetabs, sprints, createhub, productstart and templatemodules.

Not run: make check, pnpm test in the other two time zones, the whole e2e suite, a live walk against a running server, screenshots. No Testcontainers test was run; no repository method was added.

Everything is pushed to origin suite/concept (HEAD 21ceb641a). The first pushes were rejected with a remote Internal Server Error; a retry went through. My choices are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md.
Owner items: Which templates count as working in sprints: I took Scrum team only, because the docs name only 'Scrum on the sprint board' (planning.sprintTemplates). Proposal: confirm that, or add Hybrid project and Scaled agile.

=== func: entries done=False
I did not do the live functional walk the task named: no running server, hubctl, MCP session or web app was used. Every finding came from reading the code, and each one was proven with a failing unit or Testcontainers test before the fix. Concurrent edits and offline-then-online were not exercised against a running server. hubctl was not traced to see whether it goes through the use-case registry, so the integer-bound fix is only known to cover MCP. Not run: make check, the full integration and e2e suites, and any web app tests (no web app file changed). What did run: go build ./...; go test for the touched packages (view, work domain, usecase, work service, importer service, presentation/rest, infrastructure/postgres, cmd/server); integration tests filtered to copy, duplicate, materialisation, occurrence, subtree, import, workflow, template and instantiate; make generate; make gate-architecture; make gate-docs. Possible next step: a live REST/MCP walk of the status board with entries that were copied, templated and imported, plus the concurrent and offline scenarios. The run's choices are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md. The branch is pushed and the tree is clean at fb0b7c471.

=== func: knowledge done=True
Nothing is left for this tester's scope. Everything is pushed to origin suite/concept at 4731d7a53, and the notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md. What I ran: go build ./...; the Go tests of the knowledge domain and service packages, infrastructure/postgres, infrastructure/importer, presentation/rest, cmd/server, core/application/catalogue and locales; the page integration tests with Testcontainers (-run 'Page|Article|Space|Daily'), plus the two new ones, each also shown failing without its fix; make generate, make api-client, make gate-architecture, make gate-docs, and golangci-lint on the three changed Go packages. Not run: make check, the full integration and e2e suites, and any webapp or editor test (no client file changed). There was also no live walk through REST, hubctl, MCP or the web app, and no offline replay against a running server. One cost to note: a public page now checks the link of each mentioned page, up to 200 mentions, each with a few extra queries on the unauthenticated route. Choice to note: UC-DOC-08 check 5 was left as written; a link that no longer answers is treated as no link.
Owner items: Copying a restricted page (CopyPage, UC-DOC-01 check 8 says restrictions are not copied) puts the copy unrestricted beside the original, so the whole hub can read words the restriction kept to a few people. Proposal: the copy is restricted to the same people as the page it was copied from, and the copier can lift that. | Pre-existing, not changed in this run: the importer's subtree depth checks and restoring separately trashed subpages under a parent that moved deeper are covered only for the restored page's own subtree. No owner decision needed; named only for completeness.

=== func: whiteboards done=True
Not run: make check, make verify, e2e tests, and a walk in the running app. None of the webapp changes (paste/import placing, side panel size and link fields, link dialog) was seen in a browser. The webapp node tests (1619) ran in the default time zone only. Found but not fixed:
- A hub's trash has no limit, but the trash list reads at most 1000 boards, so boards past that cannot be seen to restore. Fixing it needs a paged trash list, which is a contract change.
- Two CreateWhiteboard calls at the same moment on a hub with 999 boards are not serialised (count, then insert) and can reach 1001.
- A DELETE retry on a board comment already taken back answers 404, as page comments do.
- Two devices of the same voter voting at the same instant keep only one of the two votes.
- Page titles in the pages module (knowledge CheckTitle) have the same zero-byte gap.
The notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md, and everything is pushed to origin suite/concept.

=== func: CMDB done=True
Found but not fixed:
- An Alertmanager group of more than 200 alerts is refused whole with a 422, and Alertmanager does not retry a 4xx, so those alerts are lost. Options: take the first 200 and count the rest, or document max_alerts: 200 for the webhook.
- A stale firing alert retried after its resolution reopens the alert. The signal's startsAt is parsed but not used; using it needs a stored signal time, which means a migration.
- Two repeated firings arriving at the same moment for an alert whose first entry write had failed could still open two entries, because the entry is written outside the lock.
- Relating CIs across hubs answers cmdb.relation_across_hubs, which tells a writer that a CI id exists in a hub they cannot read.
- Review-due days and the 'disputed: due the same day' rule use UTC days, not the workspace's time zone.

Ran: go build ./...; go test on core/domain/model/cmdb, core/application/service/cmdb, presentation/rest, presentation/intake, presentation/mcp, cmd/server and locales; the Testcontainers test TestAConnectorAndItsAlertsStayInTheirWorkspace; make gate-architecture; golangci-lint on the three touched Go packages (one revive finding in IdentityController.go, a file this run did not touch); the webapp's cmdb.test.ts (34 pass, default time zone only) and svelte-check (0 errors).

Not run: make check, make verify, the full integration and e2e suites, webapp lint, and any walk through REST, hubctl, MCP or the running app. That PostgreSQL refuses a zero byte in text and jsonb was not measured here; it is the known behaviour.

All commits are pushed to origin/suite/concept. Notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md.

=== func: service desk done=True
Found but not fixed (recorded in suite-pr-notes.md):
(1) Deleting an organisation that a policy names is not refused. The policy then measures nobody, and every later edit of the policy is refused with sla_policies.organisation_unknown.
(2) The web app's requester panel lists "other open requests" by creator, so for a mail-filed request it lists everything the intake filed.
(3) An organisation's domain has to be typed in its ASCII (xn--) form; müller.de is refused as malformed.
(4) A changed priority does not re-target goals that are already running.
(5) A reopened request is not measured again; the use case does not say what should happen.
(6) A person who is counted at a level and is also an absent approver's delegate can only decide for themselves, never for the absent one.

Run: go build ./...; go test of the servicedesk domain and service, config, chatbot, locales, presentation/rest and presentation/mcp packages; go vet ./cmd/server; make gate-architecture (green); golangci-lint on the three touched packages (0 issues).
Not run: make check, make verify, the integration and e2e suites, any webapp test, a walk in the running app, and the hubctl and MCP doors live.
No earlier service desk tester run was found in the notes or the log, so nothing had to be resumed.

=== func: planning done=True
All findings came from reading the code; each fix was shown failing in a test first. I did not run a server, hubctl, MCP or a walk in the web app.

What I ran (all passed):
- go build ./... and make generate (the new sqlc queries).
- go test of planning domain and service, sync, locales, presentation/rest, presentation/mcp, catalogue, cmd/server and infrastructure/postgres.
- The sprint and release integration tests against Testcontainers.
- make gate-architecture, and golangci-lint on the touched Go packages (0 issues).
- The webapp's planning, cmdb and imports node tests (default time zone only), svelte-check (0 errors) and lint.

Not run: make check, make verify, the full integration and e2e suites.

The release unit tests use a fake store that does not roll back, so 'no notes page on a refused publish' is shown by 'no page was written', not by a real database rollback.

Found but not fixed:
- Two TakeBaseline calls at the same time at 19 baselines can reach 21.
- ReadPlanTolerance writes an audit entry on every read that finds an exception; its descriptor says so, and the notification is sent only once.
- CompareBaseline counts variance days in UTC.
- ApplyPlanSchedule writes a start date on an automatic milestone it moves.
- A project XML in Windows-1252 that declares its encoding is refused as not XML.
- A release dated 1970-01-01 is refused as having no date.
- The tolerance fix assumes a due that is all-day now was all-day when the baseline was taken, because a baseline stores only the instant.

Notes appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md. Everything is pushed to origin suite/concept up to 85d006d33.

=== func: time done=True
Not run: make check, make verify, the full integration and e2e suites, any web app test (no client file changed), and a walk in the running app. Ran: go build ./...; make generate; make api-client; go test on the touched packages plus presentation/rest, presentation/mcp, cmd/server, the catalogue and locales; three integration tests with Testcontainers; make gate-architecture; golangci-lint (0 issues).

Found but not fixed:
- Period close grades at most 2000 goals; any beyond stay ungraded.
- A record's day can be any year from 0001 to 9999, so a typo year makes a budget's burn chart thousands of weeks long.
- A time note's length is counted before the database normalises it (NFC), so a few characters could push it over 2000 and fail as 503. Reasoned only, not reproduced.
- Two goal moves at the same moment (A under B, B under A) can still form a cycle. The progress walk has a depth limit, so it does not hang.
- An offline timer stop stamped before a start set by another device answers timers.not_started, and the timer keeps running.

Not examined: concurrent absence overlap, the sync engine's replay of time operations, and the web app's goal and dashboard screens.

Run notes were appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md. Everything is pushed to origin suite/concept.
Owner items: Aggregated time visibility (UC-TIM-04 check 3): a report still says how many minutes it withheld, so with a narrow filter (one entry, one person) it reveals that person's minutes. Proposal: give the withheld-minutes figure only when at least two groups are withheld, or when the withheld total covers at least three people. This changes what the use case's How to check says, so it is the owner's decision. | UC-TIM-03 How to check does not name the new refusal time.correction_below_zero. Proposal: add 'a correction takes away at most what the record and its corrections still hold' to check 3. This wording change is the owner's decision.

=== func: agents done=True
This was a fresh run: the notes and the log hold no earlier tester run for this area, so nothing was lost to the interruption. Each fix has a test that failed before the fix. All work came from reading the code; I did not use a running server, hubctl, an MCP session or the web app.

What ran: go build ./...; make generate and make api-client; the tests of every touched package and its neighbours (rest, mcp, cmd/server, catalogue, locales); three integration tests against Testcontainers; make gate-architecture (green); golangci-lint on the touched packages (0 issues). For the two concurrency fixes, the unit tests check that the row is held before the count is read. I did not reproduce either race against a real database.

Not run: make check, make verify, the full integration and e2e suites, any web app test (no client file changed).

Found but not fixed, recorded in .git/suite-pr-notes.md:
- An agent request's approval commits before the act runs, in a separate transaction, so a crash between the two leaves an approved request that was never done.
- The hourly agent budget resets at the top of the clock hour, so near the turn of an hour an agent can spend almost two budgets within minutes.
- A poll cursor dated in the future (only possible if forged) is taken as is, and the poller then sees nothing until that moment.
- I did not check how Microsoft's API writes a date-only due for Microsoft To Do; if it is the person's midnight converted to UTC, those dues land as timed dues.
- A CalDAV client that names its own custom zone (Outlook-style names) is now refused rather than read from the zone definition it sends along.
- TZID=Local reads the server's own zone.

The choices I made are in /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md. Everything is pushed to origin/suite/concept (546f45ee8..99d8b8a94).

=== func: settings done=True
Not fixed:
- A config draft key may be at most 50 characters (config_draft CHECK), but a custom role key may have 63, so a role with a longer key cannot be drafted. Fixing it needs a migration that widens the CHECK.
- A view-as keeps running for up to its 30 minutes after the viewer loses MANAGE_MEMBERS or the role ceiling. AuthenticateToken.viewedAs only ends it on the viewed person's state.
- DeleteRole still counts the rules that name the role without a lock.
- A retried apply after a lost answer is refused as config.version_conflict instead of being answered as the earlier success.

Not run: make check, make verify, the full integration and e2e suites, webapp tests (no client file changed), any walk in the running app, and hubctl or MCP against a running server. The races were proven with fakes and lock-wait integration tests, not reproduced end to end. Notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md and everything is pushed to origin/suite/concept at d5f0c47b3.
Owner items: View-as after the viewer loses their rights (UC-ID-29): proposal is to also end the view at authentication when the viewer no longer holds MANAGE_MEMBERS at the workspace. That puts one more question on every request of a running view, so it is the owner's call.

=== leftovers 1 fix 1 done=False
I handed over before the context limit. These findings are confirmed in the code but not fixed:
- 5: CheckSnapshot in core/domain/model/realtime/Document.go checks only size and sequence, and Documents.Compact stores the snapshot unchecked. A proper fix needs either a Go reader for Yjs update-v1 struct headers (to compare the snapshot's state vector with the log's up to upTo) or a provisional snapshot that a second opener promotes. That is a realtime protocol change, so it is its own step, with offline-sync.md updated alongside.
- 3: packages/sync-engine/src has no 100 000-entry bound and no hub-bounded replication, offline-sync.md §1 is unchanged, and PLAN.md has no L3 snapshot run.
- 6: StarterSet still holds only hub_name and hub_preset; there is no template-led single apply.
- 8: packages/editor/package.json adds only @tiptap/extension-table; there is no katex, mermaid, lowlight or emojibase-data, and Render.go does not render tables.
Not run: make check, make verify, the full integration and e2e suites, golangci-lint over the whole repo (on the touched packages it found one issue in my code, now fixed, and one older issue at cmd/server/main.go:5595), and a walk in the running app. All work is pushed to origin suite/concept, and my choices are appended to .git/suite-pr-notes.md.

=== leftovers 1 fix 1.2 done=False
I resumed from a clean, fully pushed branch. The first worker's commits were all on origin, so no task was lost to the usage limit. I worked on the four findings it handed over. Everything is pushed to origin suite/concept (head ece9b23d7) and my choices are appended to .git/suite-pr-notes.md.

Still open:
- Finding 5, the rest: the server still does not check a writer's snapshot against the log's content, so a buggy or hostile writer's snapshot is accepted. The fix is a Go reader of Yjs update-v1 struct headers (written here, no new dependency) that compares the snapshot's state vector and delete set with the log's up to the snapshot's place. It is its own step.
- Finding 6, not started: the template-led starter with one server-side apply (user-experience.md §3.2, build step 14). StarterSet still holds only hub_name and hub_preset. It needs a contract change (a template key and answers on starter:apply), the configuration plan and apply run inside ApplyStarter's transaction with the configuration hold, the family gallery dialog with the template's questions, and UC-WRK-30 check 5.
- Finding 8, not started: the editor blocks (fourth answers item 2). Only @tiptap/extension-table is in packages/editor. Still missing: extension-details, katex, mermaid, lowlight and emojibase-data (lazy chunks, through the licence gate), their nodes, the Markdown round-trip, tables in Render.go, the wiki import mapping and THIRD-PARTY-LICENSES.md.

Choices I made for finding 3, which the owner may overrule:
- 'Or pinned' is not built, because the app has no pinned hub.
- Membership is not checked per hub; the server applies the scope after its own permission check.
- A newly opened hub joins the copy at the next start, not at once.
- A workspace that shrinks below the bound stays narrowed.

Ran: go build ./...; go test ./presentation/realtime and ./locales; golangci-lint on presentation/realtime (0 issues); the sync-engine tests, typecheck and lint (all pass); the webapp svelte-check (0 errors, 0 warnings), node tests in the default time zone (pass) and lint.

Not run: make check, make verify, the webapp tests in the two other time zones, e2e, the integration suites, a walk in the running app, and a real browser sync of 100 000 entries (L3 is not built).


## Lost to a network failure on 2026-11-04

The first Leftovers round's second fixer batch ended at 18:41 on "Can't reach the API server" before
it fixed anything. The second triage found its other findings again, but not these four, which the
first triage had confirmed in the code:

- Mail notifications dead-letter when no mail server is configured.
- An address refused for good (SMTP 5xx) is never marked undeliverable.
- The migrator and the platform-event numbering share advisory lock 8_010_002.
- The audit filter by request_id has no index.

## What the second Leftovers round left open (2026-11-05)

Each batch's last report, verbatim. Note: "the 10^7 run needs the owner's host" is wrong; it runs on the
workstation (accepted.md, third answers, item 9).

```
===== leftovers 2 fix 1.3 done False
Not run or not verified: make verify; the whole integration and e2e suites; and any walk against a running server. Nobody has applied a real shipped template through starter:apply end to end yet: the integration test uses a fake template taker and the config unit test uses the config fakes. The starter screenshots (stored in the session scratchpad) come from the e2e stubs only.

Found and not fixed: apps/webapp/e2e/engines.test.mjs 'a limit set by the plan says so...' (the administration/quotas chip) fails in Chromium, Firefox and WebKit. It fails the same way with this session's TourGuide change reverted, and this session touched no quotas code. Its cause was not traced.

Gaps left in finding 1:
- Today's 'Choose what you use Hubtask for' after a skip is not built, because starter:apply refuses once the question was closed (named in user-experience.md §3.2 'As built').
- The plan shown in words is only the workspace part's, because the hub part can only be planned once the hub exists; one acknowledge covers both parts.

Gaps from finding 7 still stand. 'Or their groups queue' is not built. Rows across hubs are nearest first within each hub only: WorkItem does not carry sla_due, so a merge across hubs would need each entry's SLA read.

Still open and not started:
- 2 [high] The seeded UI walk: full and empty, desktop and phone, light and dark, Firefox and WebKit, with axe.
- 6 [medium] Editor blocks: math, Mermaid, highlighted code, emoji picker. This needs katex, mermaid, lowlight and emojibase-data installed at pinned versions; nothing was installed this session.
- 8 [medium] Product tours and the first-visit CoachMark. The products-seen list touches about 20 files across the stack: migration, schema.sql, the Identity, IdentityProvider and BackupImport queries, the identity domain Account, UpdateAccountPreferences, InviteAccount output, IdentityRepository and IdentityProviderRepository, IdentityController, two contract schemas, the webapp, and backup round-trip tests. That is too much for what is left of this context.
===== leftovers 2 fix 2.3 done False
The branch is pushed to origin suite/concept at e1dae74c0 and the tree is clean. make check is green and the notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md. I handed over at this point because of context size.

Still open (each checked against the code):
- Finding 5: People rows still show only the invitation mail, not a person's other mail.
- Finding 6: `hubctl workspace demo` is not built. It needs a sample mark outside entries first: domain, migration and contract. The capacity run that proves the R3 walk was not run.
- Finding 7 is not started: archiving entry_status_change touches the archive format, the restore order, the trigger's synthetic row, OVERRIDING SYSTEM VALUE for the GENERATED ALWAYS seq, and tenant-export.md.
- One trash: the hubs' own page and whiteboard trash lists still exist and are not yet links into /trash narrowed by kind.
- One trash: who trashed a page or board is not recorded on the row, so those trash rows name nobody.

Not run: make verify, the webapp tests in the other two time zones, any e2e test, the whole integration suite, a walk in the running app, and the capacity run. None of the new UI (trash filter, Copy invitation link, erasure mode note lines) was seen in a browser. The German wording is mine and was not reviewed by a native speaker.
OWNER: ["Remove the per-hub trash endpoints GET /hubs/{hubId}/trashed-pages and the whiteboards' trashed listing in favour of /trash?kind=PAGE|WHITEBOARD. Proposal: deprecate them now and remove them in the next major. Removing an operation is the owner's decision.", 'Record who trashed a page or a whiteboard (deleted_by_type/deleted_by_id columns, expand-only migration). Proposal: add them so /trash can name the person, as it already does for entries and containers.']
===== leftovers 2 fix 3.3 done False
Not done:
(1) Load runs L2, L3, L4 and F1 are not built.
(2) The 10^7 capacity run, now including the board path, needs the owner's host. Not run.
(3) 644 checks of built use cases are still cited by no test. To list them, temporarily print from checkBuiltCitations in tools/checkdocs/usecases.go.
(4) The stored_bytes and object-count quotas, not built. I judged them bigger than one step again: new quota names, the contract enum, the settings catalogue, admin screens, a counter migration with backfill, and counts in each repository. Live document updates should stay out of a per-workspace day counter.
(5) Not checked: whether erasing an account clears its mail_undeliverable row, which holds the address. Today it is deleted only when the account row itself is deleted or the person clears the mark. The table also has no row in docs/privacy/data-catalog.md.
(6) Known side effect of the bounded count: when a count is left out, the board's WIP-limit warning reads the missing count as 0.

Checks run: go build ./...; make generate and make api-client; make gate-docs; make gate-architecture; make check (green at 7c3c5eb9e); the query integration tests (Testcontainers); the identity, locales, archive and postgres unit tests; webapp build, typecheck and lint; e2e for signin, invitationlink, terms, managedaddress and the new settings test.
Not run: make verify, the full integration suite, the full e2e suite, a browser walk.
Notes are appended to .git/suite-pr-notes.md.
===== leftovers 2 fix 4.2 done True
Finding 8 is only partly done. CMDB health (ReadCmdbHealth, which calls domain.Judge), software (Normalise) and the service map still read a hub's whole inventory in each request, capped at MaxCIsPerHub (20 000). They judge the inventory as one set, so moving them to SQL aggregation would mean rewriting domain.Judge's rules in SQL. That is more than one step and was not done. One extra commit: I wanted to amend the CMDB test's gofmt fix into its commit, but the force-push was denied, so the formatting is its own commit (c2cd2b595). make check is green at HEAD 892344230, which equals origin/suite/concept. Not run: make verify, the full integration and e2e suites, the webapp unit tests in the other two time zones, and a browser walk against a real server. Neither the hub figure nor the place settings change has been shown to the owner in the running app. Notes are appended to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md.
===== leftovers 2 fix 5.3 done False
Not done:
(a) Finding 6, CalDAV custom zone: an Outlook-style custom zone is still refused. The proposal is to map Windows zone names to IANA zones with an embedded CLDR windowsZones table, or to read the VTIMEZONE the client sends.
(b) Finding 6, a stale firing reopening a resolved alert: still open. It needs a stored signal time (an expand-only migration) that is compared with the alert's resolution.
(c) Finding 6, the year range of time records (0001-9999): still not bounded. The previous worker chose not to invent a bound, and I did not choose one either.
(d) Finding 7, offline editing of a page opened while offline: this changes the sync protocol and needs a `finding` issue. I did not open one, because posting to GitHub needs the user's go-ahead.
(e) Recurring templates are not in the intake door count. I did not check whether an occurrence's copy goes through the type's form.

Checks run: go build ./...; the tests of every touched package (config, knowledge, servicedesk, goal, planning, cmdb, whiteboard, rest, mcp, catalogue, cmd/server, locales); go vet with the integration tag; three integration tests against PostgreSQL (TestAGoalMoveHoldsTheTreeOfItsWorkspaceOnly and two organisation tests); make generate; make api-client; webapp typecheck and lint; the configreach and whiteboards node tests; packages/whiteboard tests, typecheck and lint; make check, green at 3c5ebf1c0.

Not run: make verify, the full integration and e2e suites, and any walk in the running app. The intake lines and the requester panel have not been seen in a browser.

Everything is pushed to origin suite/concept and the notes are appended to .git/suite-pr-notes.md.
OWNER: ["Delegate for an absent approver (UC-SVC-10): a person who is counted at a level themselves and is also the delegate of an absent approver can decide only for themselves. Proposal: one decision fills one seat; deciding for the absent approver is a separate, explicit 'on behalf of' choice, so one person never fills two seats with a single act (four-eyes). This changes what an approver observes, so it is the owner's decision."]
===== leftovers 2 fix 6 done True
Not covered: a German account on an English browser still loads English at boot, because the account's locale only arrives after sign-in. This was already the case before. Found but not mine: 3 tests in apps/webapp/e2e/privacy.test.mjs fail (2 writes where 1 is expected). They fail the same way with the previous source checked out, so this change did not cause them; I did not fix them. Not run: the full e2e suite (I ran about 10 files), the full pnpm test under the other two time zones (I ran the unit tests in the default time zone only), and the Go checks (nothing on the Go side changed). Pushed to origin suite/concept. Notes are added to /Users/jeromewinkel/Dev/hubtask-repo/.git/suite-pr-notes.md.
```
