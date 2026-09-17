# Milestone F6 — Offline, the moments, the ecosystem's screens

The goal: the web application catches up with the last two core milestones, and with everything
the core can do that no screen reaches yet — so that when this milestone closes, **every use case
of the catalogue is reachable from the browser or its omission is written down with a reason**.
That is the state the owner walks the demo in before convergence, and it is the state `0.9.5`'s
coverage report starts from rather than discovers.

Three things are owed. `0.8.5` built the server half of [`offline-sync.md`](../architecture/offline-sync.md)
— `:pull`, `:push`, `:snapshot`, the devices, `ACCESS_REVOKED`, the tombstones — and
`packages/sync-engine/SyncEngine.ts` still says in its own header that it holds no queue, no local
store and no clock because those "arrive in F6": no client code calls a single one of those routes,
the stream's cursor lives in memory for the tab's lifetime, and a browser that loses its connection
for ten seconds shows a skeleton where a list was. `0.9.0` built an import path for four sources
and a template generated from a description, and neither has a screen: `POST /imports` is called
by nothing under `apps/`, and a `TEMPLATE` suggestion falls through `shapeOf` to `unknown`. And two
pieces of the product the design system decided in its first week — the rewarding moments of
[`design-system.md`](../design/design-system.md) §7 and the onboarding tour of §8 — have a motion
role waiting for them (`motion.celebration`, "first user: F6's celebration kit") and no user.

F6 is the sixth milestone of the client track (`roadmap.md` phase 5). It opens with `0.8.5` and
builds the surface for `0.8.5` and `0.9.0`; the contract it works against has stopped moving.

**F6 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. The client's maturity stage is `preview` and
**stays `preview` through this milestone**: ADR-0035 §2 gives `stable` to convergence.

**This file was cut twice.** F6-01 and F6-02 were scheduled on 2026-09-17, when the review of
every `proposed` ADR found two decisions that were accepted, consistent with the product, and built
by nobody — [ADR-0047](../adr/ADR-0047-media-origin-in-the-interface-policy.md) and
[ADR-0048](../adr/ADR-0048-browser-job-driver.md); their origin tasks F3-09 and F3-19 wrote the
records and closed, and the building half was owed to no milestone. The rest, F6-03 onwards, was
cut the same day by the owner's decision on the milestone's scope, recorded below.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue. Both are **[L]** during the initial phase (`CLAUDE.md`).

What deliberately is **not** in this milestone: **the Tauri shells** — desktop and mobile, the
SQLite store behind the `Storage` port, the keystore, the updater, signed distribution, the store
pipeline, the webview smoke matrix, platform adaptation and the capability matrix made real — which
the roadmap's F6 row used to carry and which are now **F7**'s, by the owner's decision of
2026-09-17: there is no Tauri scaffold in the repository, a shell is Rust as a new toolchain, and
signing and store submission need the owner's developer accounts and certificates before a single
task can close. F7 is cut by the owner when those exist; what this milestone owes it is listed in
decision 2. **Character-level merging of notes** (SY-A), past `1.0.0`. **The 1.0 website, the
accessibility statement's publication, and the `stable` stage**, all of which are `0.9.5`'s. **A
mutation kind for anything the push frame does not carry** — reminders, recurrence, template
instantiation, container and label creation — see decision 8. **An inbox for notifications**, the
product question F3-02 recorded and nobody has answered. And **a merge rule anywhere in the
client**, which ADR-0033 §2 forbids by name and `test/rules.test.ts` fails on.

Sixteen decisions taken while writing this backlog, so that nobody re-derives them:

1. **The numbering continues at F6-03 and the milestone keeps its letter.** The GitHub milestone
   is the one #736 and #737 already sit in, renamed to what it now holds. F6's roadmap row is
   rewritten to this file, and a new F7 row — *The shells* — carries what left it, uncut.
2. **What F6 owes F7, named so that nothing is rediscovered.** The `Storage` port gets its second
   implementation there (SQLite, encrypted, key in the keystore); the product default for the sync
   scope on mobile (*subscribed containers*, SY-B) is set there with the phone's storage to
   measure against; the mutation kinds §1's left column promises and the frame does not carry
   (decision 8) are a core task there, because the offline *promise* is the installed clients'
   ([ADR-0031](../adr/ADR-0031-tauri-app-shell.md)) and a browser cache does not need them; the
   admin routes F4 tagged are excluded from the mobile build there, with ADR-0032's affordance;
   platform adaptation (`design-system.md` §9's last open point) is raised there by the shell that
   raises the question; and the webview smoke matrix (`1.0.0` prerequisite 18) is driven there.
3. **The engine stays product-agnostic, and the product supplies three functions.** F3-04 set the
   pattern: the engine reads `entity`, `entity_id`, `container_id` and `op` off a change record and
   asks `pathsFor` — supplied by `apps/webapp/src/lib/data/live.ts` — what that makes stale,
   because "the seam is the network, the paths are the product". The offline half follows it
   exactly. Two more functions live beside `pathsFor`, in `lib/data/replica.ts`: `storeFor(request)`
   says which resource paths the replica can answer and how (which collection, which filter, which
   order), and `mutationFor(method, path, body)` says which writes are one of the seven mutation
   kinds and how each is framed. The engine never learns that `/items?container_id=` is a
   collection's entries or that `POST /items/{id}:complete` is an `ITEM_PATCH` of `completed`.
   Both functions are tested beside `live.test.ts`.
4. **Reads go through to the server while it answers, and the replica answers when it does not.**
   [ADR-0033](../adr/ADR-0033-shared-client-architecture.md) §4 gives the browser a *best-effort
   cache: resilience across brief disconnects, never the offline promise*. That is read-through:
   a subscribed resource is read from the server as today; when the transport fails to reach it —
   a network failure, not a status — the replica answers for every path `storeFor` knows, and the
   `ready` state says so (`source: 'replica'`, `at` the replica's last synchronisation) so that a
   screen can mark what it shows as *as of*. A path the replica cannot answer (`/items:query`,
   `/search`, an activity history, anything administrative) fails as it does today, and the screen
   says what it needs (`ErrorState`, `sync.needs_connection`). The replica is never consulted
   while the server answers: two sources of truth on one screen is how a stale row survives.
5. **A write is direct while nothing is queued and the server answers; otherwise it is queued or
   refused, never both.** Today every write goes to its own route with its `Idempotency-Key` and
   its `If-Match`. That stays the path while the queue is empty and the server reachable. When the
   server is not reachable, or the queue is not empty — the second condition is what preserves
   §3.2's *ordering within one device* — a write `mutationFor` can frame is appended to the queue
   with its `op_id` and its HLC, applied to the replica as the device's **prediction** (§9.5 calls
   it that, and the server's answer overwrites it), and pushed in order when the connection
   returns. A write it cannot frame is refused with `sync.needs_connection`, shown where the
   person made it. Nothing is queued behind a person's back: the pending mark is on the row and in
   `SyncStatus`. And no rollback is ever invented: a `REJECTED` mutation leaves the queue and stays
   visible with its code until the person dismisses it (§9.5), and the replica shows what the
   server holds.
6. **Replication is not merging.** The engine's `CLAUDE.md` says *a record is a signal to re-read,
   never data to apply* and *no optimistic apply, and therefore no rollback* — both true of F1's
   engine and both written with the sentence that F6 changes them. Applying a change record to the
   replica is writing down what the server decided; applying a push result's `server_state` is
   the same. Neither reconciles two values, and the rule that fails the build on a symbol that
   merges stays. F6-03 rewrites the two paragraphs to what is true afterwards, and adds the
   distinction so that the next reader does not reintroduce the F1 rule against the F6 code.
7. **The web client synchronises the workspace, measured before it is decided.** ADR-0033 §4 put
   SY-B's product default in the sync-engine work package "where real payload sizes exist to
   measure". F6-04 measures the snapshot of the integration environment's `demo` workspace and of
   its largest, in bytes and seconds, on the stack's own link, and writes the figures into the pull
   request. The default is *everything the account may read* unless the figures say a workspace of
   ordinary size costs more than a few seconds, in which case the scope narrows to the hubs the
   person has opened in this browser and the pull request says so; `offline-sync.md` §12's SY-B
   row records the web's answer either way, and mobile's stays F7's.
8. **The browser offers offline what the frame carries, and the document says so per client.**
   `SyncMutation.kind` has seven values; `offline-sync.md` §1's left column promises more —
   *setting reminders* and *applying templates* have no kind, and creating a container, a label or
   a bucket has none either. `0.8.5` said a kind "is an additive enum value for the milestone that
   finds the need", and the milestone that finds it is the one carrying the offline promise. So
   this milestone frames what exists — an entry created, patched, completed, moved, trashed; a
   label, a member or an attachment added or removed; a comment added — and F6-03 adds a sentence
   to §1 that says what a *browser* offers offline, which is those, and that saved views, search and
   the query language are online there. F7's core task closes the gap for the installed clients.
9. **The initial synchronisation is the snapshot, with the page sequence as the fallback.**
   `POST /sync:snapshot` streams the walk as newline-delimited JSON with the cursor last (P-12);
   a device whose stream ended without the last line starts again. The engine takes the snapshot
   first and falls back to `:pull` without a cursor when the snapshot ends twice without a cursor
   — a workspace above the stream's byte budget is exactly what the page sequence is for. The
   transport gains one primitive for it, `Transport.snapshot` — *a body that is lines* — beside
   the three F3 and F3-15 added, because it is `fetch` and there is one caller of `fetch`.
10. **A document the replica assembled from field records carries no version the server
    confirmed.** A delta `UPSERT` is one field under its own clock (the contract's `payload`
    description); applying it to the stored document updates the field and not `version`. So a
    read answered by the replica publishes no `etag`, a direct write made from such a state
    carries no `If-Match`, and a queued mutation's `base_version` is the version the initial
    synchronisation or the last full-object record delivered — which is what the frame wants
    anyway. The trap is named here because it would otherwise be found as a `409` on every write
    after every reconnect.
11. **One core task, and it is the account's.** F1…F4 each found a gap in the contract; F5 found
    none; F6 finds one, and it is small: §7 says a celebration is *on by default, and each user
    can switch it off — one preference, all tiers*, and §8 says the tour runs *on first start* and
    is *restartable from the help menu*. Neither is knowable from a client. They are properties
    of the person, not of the screen — [ADR-0043](../adr/ADR-0043-theme-per-device.md) keeps the
    theme and reduced motion on the device because the *operating system* sets them, and nothing
    sets these but the person — so F6-12 adds `celebrations` and `onboarding_completed_at` to
    `AccountPreferences`, additive and specification first, the shape F1-08's `GET /accounts/me`
    took. The "first-ever completion" §7 names is not a query: it is the completion that happens
    while `onboarding_completed_at` is null, whether the tour led there or was skipped.
12. **The tiers are read off the replica, and the day's limit is the device's.** §7 says trigger
    evaluation runs client-side "from the domain events and hierarchy state the client already
    holds"; after F6-03 the client holds the collection's entries and, for the workspace scope,
    everything due today. Tier 2 is the completed entry's siblings under its parent all being
    complete; tier 3 is a collection or a hub emptied of open work, the last entry due today, or
    the first completion of decision 11. *At most one tier-3 per user per day* is kept in the
    replica's own metadata, which makes it per device rather than per person where somebody
    completes work on two browsers in one day — recorded as the known deviation, cheap to move to
    the account if it ever matters, and not worth a third preference field today.
13. **A slot is a component with a tier, and the animation is an asset.** §7 asks for one
    celebration slot per tier with the intensity guardrails as tokens. F6-13 adds the guardrails to
    `tokens.json` — a duration per tier under `motion.celebration`'s role, an area and an extent —
    and one component, `Celebration`, that takes the tier and fills the slot from the design
    system's assets; what fills tier 3 is a design-system file that can be replaced without
    touching the trigger. Under `data-motion="reduced"` every tier is the colour change rule 6
    already fixes, and the acknowledgement is never absent.
14. **The tour is a pattern in the design system and content in the application.** §8 decides
    the pattern — coach marks on the real interface, a spotlight with the glass overlay of rule 2,
    next and skip, keyboard-operable — so `Tour` and `CoachMark` are wave-3 components with
    stories, and the six steps are `apps/webapp`'s, anchored by `data-tour` attributes on the
    elements they point at. The last step leads to creating and completing one entry, and that
    completion is decision 11's first one.
15. **The import lives on the hub, and the report is the one the restore already draws.** `POST
    /imports` needs `STRUCTURE` on the hub, so the wizard opens from a hub's container screen. The
    file goes through the media flow the application has (`upload` gains the `IMPORT` usage), the
    kind is the contract's `ImportKind` — four values, all served since P-10, and no manifest field
    is invented for what the enum already says — the CSV mapping is a form of seven selects
    over the file's header row read in the browser, and the run's `report` is a `RestoreReport` —
    the component `RestoreView` renders for a dry run renders it here, with the `refused` rows
    beneath it by number and code. Nothing import-shaped is drawn twice.
16. **The coverage report is held to the catalogue by the documentation gate.** `0.9.5` asks for
    "every use case of the catalogue, where it is reachable in each client, and every deliberate
    omission with its reason". F6-15 writes it for the web column, 231 rows, and `tools/checkdocs`
    gains one check: every name `catalogue.Descriptors()` returns appears in the report, and no
    name the report carries is absent from the catalogue. A use case added later without a row
    turns `make gate-docs` red, which is what makes the report a document rather than a snapshot.

---

## F6-01 — The policy names the media origin **[L]**

*Depends on: nothing. Issue #736.*

ADR-0047, accepted 2026-09-17, built here. Under `s3` the transfer URL `POST /media` answers is a
presigned URL on the bucket's origin and the cover's download URL is another; the interface's
policy is `connect-src 'self'` and `img-src 'self' data: blob:` (`security.md` §9, ADR-0028), so
the browser refuses the `PUT` before it is sent and refuses to draw the cover, in front of the
person, with no server involvement. The chart defaults `storage.kind: s3` (`k8s/values.yaml`), so
this is the default Kubernetes installation, not a corner; Compose and the integration environment
run `local`, which is why no walk has found it.

The change is the server's and small. The composition root derives the **origin** — scheme, host,
port, nothing else — from the storage configuration it already parsed, the same way `NewS3Storage`
resolves an empty endpoint to `https://s3.<region>.amazonaws.com`; `webui.NewHandler` takes it as a
parameter and composes the policy once at construction, beside the entity tags it already computes;
under `local` the produced string **equals** `ContentSecurityPolicy` byte for byte, and a test
compares rather than contains, which is what keeps the default installation from widening. Exactly
one origin, never a list, never a wildcard; no other directive moves. `security.md` §9's interface
row gains the sentence, and `deployment.md` gains the operator's half — the bucket's CORS rule:
the interface's origin exactly, method `PUT`, header `Content-Type`, no credentials — because
Hubtask does not own the bucket policy and cannot set it.

Then the proof the record could not have: the integration environment, or a local stack with
MinIO, run under `s3`, a cover uploaded and drawn from the browser, and the evidence in the pull
request. That is the walk F3-09 was proved against `local` for want of.

**Acceptance:** `webui.NewHandler` takes the media origin and the policy under `s3` names it in
`connect-src` and `img-src` and nowhere else; under `local` the policy equals the ADR-0028 constant
and a test asserts equality; the origin is derived from `StorageConfig` in `cmd/server/main.go`
and nowhere in `apps/`; `security.md` §9 and `deployment.md` say what the code does; a cover
uploaded from the browser against an `s3` installation is shown in the pull request; `make verify`
green.

**Read:** ADR-0047 (all of it, including the amendment); ADR-0028; `security.md` §9;
`presentation/webui/Handler.go`; `infrastructure/storage/S3Storage.go` (how the endpoint is
resolved); F3-09 in `milestone-F3.md`; `milestone-F4.md`'s export paragraph (why a backup target
is *not* a second origin)

---

## F6-02 — The browser job **[L]**

*Depends on: nothing. Issue #737.*

ADR-0048, accepted 2026-09-17, built here: Playwright, pinned to an exact version, a
`devDependency` of `apps/webapp` and of nothing else, one job in `ci.yml` that serves the built
`dist/` over a static server of a few lines and loads it in Chromium, Firefox and WebKit. The
dependency is the one CLAUDE.md reserves to the owner, and the owner took it in the record; the
pull request carries the lockfile change with the count it costs, in F1-01's manner.

What the job asserts is ADR-0044's feature table and not a journey: a dialog opens and traps focus;
a gated control is unreachable by keyboard; the focus ring lands where rule 5 puts it; a
visually-hidden label is not visible; an overlay is positioned by CSS and not by the fallback. Each
is a fact about the engine and fails loudly in one that lacks the feature. Three assertions are a
fine first job; the list grows in the pull requests that need it. The job is required through
`CI required` — `main` lists exactly one context, and a browser job that does not gate proves
nothing about what is merged (`ci-cd.md` §5).

Then the two things that waited on it, each in its own commit. **The row**: `support-matrix.md` §5
reads `supported` for the three engines and names the job, with the column saying what actually
ran — Playwright's WebKit is the engine, not Safari, and the row says so rather than overclaiming.
**The fallback**: `packages/design-system/src/positioning.ts` and its test are deleted, and
ADR-0039's status line records that its lifetime ended here. A reviewer sees the deletion as one
thing whose justification is the commit before it.

**Acceptance:** `pnpm-lock.yaml` carries Playwright at an exact version and nothing else new;
`ci.yml` has the job, `CI required` depends on it, and a deliberately broken assertion goes red on a
branch before the job is merged green; `support-matrix.md` §5 names the job and reads `supported`
for the engines it runs; `positioning.ts` is gone and ADR-0039 says so; `ci-cd.md` names the job;
the pull request states the dependency's transitive count.

**Read:** ADR-0048 (all of it, including the amendment); ADR-0044; ADR-0039; `support-matrix.md`
§1, §5; `ci-cd.md` §5; F3-19 in `milestone-F3.md`; F1-01 in `milestone-F1.md` (how a tool decision
is recorded in a pull request)

---

## F6-03 — The replica, and the initial synchronisation **[L]**

*Depends on: nothing. The task the offline chain stands on. Issue #738.*

Decisions 3, 6, 8, 9 and 10. `packages/sync-engine` gains what its header has promised since F1,
in this order:

1. **Two stores.** `Storage` gets its first two implementations: `IndexedDbStorage` in the
   package (one database per API origin and account, one object store per collection of
   `schema.ts`, `clear()` deleting the database rather than emptying it — §9.6 says *discarded
   completely*), and `MemoryStorage`, which the tests and F6-08's runner use. The browser
   platform seam (`src/lib/platform/browser.ts`) supplies the first; nothing under `apps/`
   touches IndexedDB. No encryption in the browser, by ADR-0033 §4's word ("no browser-side
   encryption theatre"), and the package's `CLAUDE.md` says so where a reader would look for it.
2. **The device.** A `device_id` — a UUIDv7 minted once per store from the `Clock` and Web
   Crypto, held in the store, gone with it at sign-out — and the `platform` and `display_name`
   the pull and the push carry (`web`, and the browser's name from the platform seam). The HLC
   the queue will need is written here too, as a module: physical time from the `Clock`, a
   counter, the device, in `ParseHLC`'s textual form and with `Tick`'s rule that the physical part
   never moves backwards.
3. **The initial synchronisation.** `Transport.snapshot` reads `POST /sync:snapshot` line by line
   and hands each `SyncChange` over as it arrives; the engine writes every record into the
   replica by entity and keeps the cursor from the last line; a stream that ends without one is
   started again, and after the second such end the engine walks `:pull` without a cursor,
   page by page, and takes the cursor from the last page. Whatever the frame reports as
   `tombstone_window_days` and `server_time` is kept beside the cursor.
4. **The delta, and the stream applying.** On start, on every reconnect and after every push,
   `:pull` from the held cursor until `has_more` is false; the stream — unchanged in how it
   connects, F3-04's reconnects and refusals — now *applies* every record to the replica before
   it invalidates what `pathsFor` names: an `UPSERT` with a whole object replaces the stored
   document, one with a field updates that field, a set record adds or removes an element in the
   entry's set, a `DELETE` removes the entity and, for a container, everything under it by path
   prefix, and `ACCESS_REVOKED` does what a subtree deletion does at the root it names (§6). The
   cursor advances in the store, not in memory: a reload continues where the tab was.
5. **The two refusals that empty the store.** `sync.cursor_too_old` — a cursor past the window,
   or from an older epoch after a restore (§8) — clears the replica and runs step 3 again;
   `sync.cursor_invalid` restarts the stream with no cursor and keeps the replica. Both already
   exist in `#listen`; what changes is that "drops everything held" now means the store.
6. **Sign-out.** `engine.reset()` calls `Storage.clear()`; the session module already calls
   `reset()`, so the replica, the queue-to-be, the device identifier and the cursor go together.

Then the documents: `packages/sync-engine/CLAUDE.md`'s two paragraphs rewritten (decision 6), and
`offline-sync.md` §1 gains the sentence per client (decision 8). Nothing on a screen changes in
this task except that the stream's cursor survives a reload, which the pull request shows.

**Acceptance:** `IndexedDbStorage` and `MemoryStorage` implement `Storage` and pass one shared
test file; a fresh sign-in against the fakes takes a snapshot and holds every record of it, a
second start takes the delta from the held cursor and no snapshot; a snapshot ending twice without
a cursor is followed by the page walk; an `ACCESS_REVOKED` for a hub leaves nothing of the hub in
the store; `sync.cursor_too_old` empties the store and resynchronises; sign-out deletes the
database; the engine's test suite covers §9's points 3, 4, 6 (the report says the browser has no
encryption) and 7 (a field the engine has never seen is stored and read back unchanged) as named
tests; `apps/webapp` has no import of IndexedDB and no second caller of `fetch`; the two
documents say what the code does; `pnpm -r build lint typecheck test` green; `make gate-docs`
green.

**Read:** `offline-sync.md` §1, §3, §6, §7, §8, §9; ADR-0033 §2, §4; ADR-0021; ADR-0031 (the
browser/installed split); `packages/sync-engine/CLAUDE.md`; `SyncEngine.ts` (`#listen`, `reset`);
`schema.ts`; `ports.ts`; `core/domain/model/shared/HLC.go` (the textual form); N-01, N-02, N-08
in `milestone-0.8.5.md`; P-12 in `milestone-0.9.0.md`; `hubctl sync snapshot` in
`cmd/hubctl/Sync.go` (the reference reader of the stream)

---

## F6-04 — Reads answered by the replica **[L]**

*Depends on: F6-03. Issue #739.*

Decisions 3, 4, 7 and 10. `lib/data/replica.ts` gains `storeFor`: for `/containers` and
`/containers/{id}` the tree and one node; for `/items` by `container_id`, `parent_id` and the
list's other query parameters the entries, ordered by `order_key` as the server orders them; for
`/items/{id}` one entry; for a container's `/buckets` and `/labels`, an entry's `/comments`,
`/reminders` and `/recurrence`, and `/templates`, the collections the snapshot delivered. Every
other path answers `undefined`, and the function's test lists the ones that do on purpose:
`/items:query` and `/search` because the query language is the server's, `/activity` because the
history is not synchronised, and everything under `/administration`'s data because §1's right
column is administration.

The engine's `#load` learns the fallback: a read that fails to *reach* the server — the transport
distinguishes a network failure from a status today — asks `storeFor` and, where it answers,
publishes `ready` with `source: 'replica'`, `at` the store's last synchronisation, and no `etag`
(decision 10). A `ready` state that came from the server carries `source: 'server'`, so the shape
has no optional that means two things. While the server is unreachable, the subscription is
retried on the stream's reconnect schedule rather than on every render, and the first server
answer after the reconnect replaces the replica's.

On screen: `EntryList`, `Board`, the container tree and the item screen mark a replica state with
one line — `app.offline.as_of` with the moment, through the `Intl` formats F5-09 built — and a
panel whose path the replica cannot answer shows `ErrorState` with `sync.needs_connection`. The
mark is one component used in four places, not four sentences.

Then the measurement decision 7 asks for: the snapshot of the `demo` workspace and of the
integration environment's largest workspace, bytes and seconds, in the pull request, and the web
scope written into `offline-sync.md` §12's SY-B row.

**Acceptance:** `storeFor` is tested for every path it answers and for the four it refuses by
design; against the fakes, a transport that stops answering after the snapshot leaves every
answered path `ready` from the replica and every other one `failed`; the entry list, the board,
the tree and the item screen render the *as of* mark from a replica state in a story or a test;
the first server answer after a reconnect replaces the replica's state; the two measurements are
in the pull request and SY-B's row names the web default; `pnpm -r build lint typecheck test`
green; `make gate-docs` green.

**Read:** F6-03; `SyncEngine.ts` (`#load`, `ResourceState`); `lib/data/live.ts` and `live.test.ts`
(the shape `replica.ts` follows); `lib/data/resource.svelte.ts`; `lib/i18n/` (the formats);
`offline-sync.md` §1, §12 (SY-B); ADR-0033 §4; `voice-and-tone.md` §4 (what an empty state and a
failure are)

---

## F6-05 — The queue, the clock, and `:push` **[L]**

*Depends on: F6-04. Issue #740.*

Decisions 3, 5, 6 and 10. `lib/data/replica.ts` gains `mutationFor`: `POST /items` becomes
`ITEM_CREATE` with an `item_id` the client mints (§9.1 — the identity is final; the direct path
keeps the server's identifier, because it is not a mutation); `PATCH /items/{id}`, `:complete`,
`:reopen`, `PUT /items/{id}/due`, `DELETE /items/{id}/due` and a custom field written become
`ITEM_PATCH` with one HLC per field; `:move` and `:reorder` become `MOVE`; a label, a member or an
attachment added or removed becomes `SET_ADD` or `SET_REMOVE` on the named set; `DELETE
/items/{id}` becomes `ITEM_DELETE`; `POST /items/{id}/comments` becomes `COMMENT_ADD`. Everything
else answers `undefined` and the test says which and why.

The engine gains the queue: a `queue` collection in the store holding `PendingMutation`s in
order; `mutate` consults `mutationFor` under decision 5's rule — direct while the queue is empty
and the last call reached the server, queued otherwise — and a queued mutation is applied to the
replica as the prediction and published to its subscribers with `pending: true` on the record.
The push: batches of at most 500, in order, on every reconnect and immediately when a mutation is
queued while the server answers (the *queue not empty* case); the frame carries the device and
its name; every result is applied as the server's word — `APPLIED` and `MERGED` write
`server_state` over the prediction, `CONFLICT` writes the server's state and keeps both values
for F6-06 to show, `REJECTED` removes the mutation from the queue and keeps it, with its code, in
a `rejected` collection until dismissed; `sync.gone` and `forbidden` are the two codes §7 and §6
name and are rendered by name. The push's `cursor` advances the store's. A push that fails to
reach the server leaves the queue as it was. `sync.device_revoked` on a push clears the store and
mints a new device, which is what N-03 says a forgotten device does.

`engine.queue()` is a subscription like `subscribe`: the count, the oldest moment, the rejected
list — what `SyncStatus` will render. The engine's `CLAUDE.md` gains the queue's paragraph.

**Acceptance:** `mutationFor` is tested for each of the seven kinds and for the writes it refuses;
against the fakes, a write while the transport is down is queued, shown pending, applied to the
replica, pushed on reconnect and overwritten by the server's `server_state`; the same push sent
twice — the transport drops the first answer — applies once (§9.2, the `op_id`); a `REJECTED`
result is kept with its code and removed only by dismissal (§9.5); a `sync.gone` is discarded and
its local text is offered back; the queue survives a reload of the fakes' store; two writes queued
in order arrive in order; the HLC never moves backwards under a clock that does; the engine's
suite names §9's points 1, 2 and 5; `pnpm -r build lint typecheck test` green.

**Read:** F6-03, F6-04; `offline-sync.md` §3.2, §4, §5, §6, §7, §9; N-04…N-07, N-09 in
`milestone-0.8.5.md` (what the server does with each kind, and what it answers); `SyncEngine.ts`
(`mutate`); `schema.ts` (`PendingMutation`); `hubctl sync push` in `cmd/hubctl/Sync.go` (the
reference framer); `core/application/service/sync/` (the appliers, for the exact field names an
`ITEM_PATCH` takes)

---

## F6-06 — `SyncStatus` and `ConflictResolver` **[L]**

*Depends on: F6-05. Issue #741.*

The wave-3 row `design-system.md` §4 has carried since the first week — *offline operation,
"concurrent changes are never lost"* — built. Two components, with stories:

**`SyncStatus`** is one line in the frame's header: connected, reconnecting, offline; the count of
queued changes and the moment of the oldest; the last synchronisation. It is fed by `engine.queue()`
and by the stream's state, which `HealthNotice` already reads for its own purpose, and it renders
no sentence of its own. The offline state is a `status` live region (F5-12's rule: a change the
reader cannot see is announced), the pending count is text, and a click opens the list: every
queued change as *what* and *where* — "title changed · Write the reference" — and every rejected
one with its code and a dismiss, in `voice-and-tone.md`'s voice. The row itself marks a pending
entry in `TaskRow` and `WorkItemCard` with the `pending` motion role, which exists for this.

**`ConflictResolver`** is a dialog for the one case §5 leaves to the person: a `CONFLICT` on
`notes`. Both versions side by side — the server's, and the device's that lost — the server's
already in place, the displaced one already a system comment on the entry (`preserved_comment_id`
links to it), and two buttons: keep the server's (dismiss), or write mine again — which is an
ordinary `PATCH` of `notes` from the current version, never a merge and never an automatic
retry. It is opened from `SyncStatus`'s list and from the entry's own strip.

Then `design-system.md` §4's row says built, in the manner of the others, with what the build
decided.

**Acceptance:** both components exist with stories for every state, both modes, both directions
and reduced motion; the frame shows `SyncStatus` on every route; a story or a test shows a queued
change listed, a rejected change dismissed, and a conflict resolved both ways; the resolver's
"write mine again" is a `PATCH` and nothing else; §4's row says built; `pnpm -r build lint
typecheck test` green; `make gate-docs` green.

**Read:** F6-05; `design-system.md` §4 (the row), §6 rules 5 and 6, §10; `voice-and-tone.md`
§4, §5; `offline-sync.md` §5; `HealthNotice.svelte` (how the frame reads the stream's state);
`lib/announce.svelte.ts` (F5-12's live regions); `TaskRow.svelte`, `WorkItemCard.svelte`

---

## F6-07 — The devices **[L]**

*Depends on: F6-03. Issue #742.*

`GET /sync/devices` and `DELETE /sync/devices/{deviceId}`, called by nothing under `apps/` today.
The profile gains a *Devices* section: every device of the account — platform, name, last contact,
whether it is blocked — with this browser marked as *this device* (its identifier is in the store),
and *forget* on every other one, confirmed. A forgotten device is blocked, not erased (N-03), and
the row says *forgotten* rather than disappearing. What forgetting does to the device is
F6-05's: its next push is refused and it starts over.

**Acceptance:** the section lists the devices and marks this one; forgetting another device
refuses its next push in a test against the fakes; the route stays under the `profile` area;
`pnpm -r build lint typecheck test` green.

**Read:** F6-03; `offline-sync.md` §6, §10 (`sync_device`); N-03 in `milestone-0.8.5.md`;
`ProfileView.svelte` and `lib/data/sessions.svelte.ts` (the sessions list: the sibling list of
things that can be ended)

---

## F6-08 — The engine's conformance run **[L]**

*Depends on: F6-05. Issue #743.*

`1.0.0` prerequisite 17: `hubctl sync-conformance` passed by `packages/sync-engine` against a real
instance. The runner N-13 built drives *the server* as two devices; what the criterion asks is the
first-party client's own obligations proved against a server, and that is a second runner in the
package: `pnpm --filter @hubtask/sync-engine conformance --base-url … --token …`, a Node script
that constructs the engine over `MemoryStorage` and `FetchTransport`, plays §9's eight points
through the engine's own API — mints an identifier and reads it back, queues one mutation and
pushes it twice, loses access and watches the store empty, presents a cursor the server refuses
and resynchronises, is rejected and keeps the rejection, holds a field it has never seen — and
prints one row per point in the evidence files' shape, with 6 marked *not tested from a browser
engine, by ADR-0033 §4* and 8 marked *the engine frames no such kind, by construction*.

Then the job: *The engine's session (Compose)* in `ci.yml`, beside the hubctl session and not
inside it — the e2e's rate-limit budget is spent by its own last sections, and a second client on
the same budget would make both flaky — Node installed, the stack started, the runner against it,
required through `CI required`. `offline-sync.md` §9's last paragraph gains the sentence that
names the second runner and `ci-cd.md` names the job.

**Acceptance:** the runner passes against the Compose stack; each point prints its number and
its result; a deliberately wrong engine — one test flips the store's behaviour on
`ACCESS_REVOKED` — fails exactly point 3; the job exists, is required, and went red on a branch
before it was green; §9 and `ci-cd.md` name it; `make verify` green.

**Read:** F6-05; `offline-sync.md` §9, §11; N-13 in `milestone-0.8.5.md`; `cmd/hubctl/Conformance.go`
(the report's shape); `scripts/hubctl-e2e.sh` (how the stack is started and a token minted);
`.github/workflows/ci.yml` (`e2e`, `ci-required`); `docs/evidence/SY-2026-09-16.md`

---

## F6-09 — Import: the wizard and the report **[L]**

*Depends on: nothing. Issue #744.*

Decision 15. A hub's container screen gains *Import…* under `STRUCTURE`: a dialog in four steps
that are one component. The kind, from the contract's `ImportKind` (`CSV`, `TRELLO`,
`GOOGLE_TASKS`, `MICROSOFT_TODO`), each with one sentence saying what file it takes and where the
source exports it — the manifest declares no import capability and this task adds none, because a
kind the build refuses answers by name (P-08) and the report shows it; the file, through `UploadField` and `upload(file, 'IMPORT', …)`, with the
content types the confirmation accepts for that kind; for `CSV` only, the mapping — the header row
read in the browser, seven selects (`title`, `notes`, `due`, `completed`, `labels`, `bucket`,
`parent`) each offering the file's columns, prefilled where a header already matches, and
`mapping` sent only for what differs; then `POST /imports` and the job followed through
`lib/data/jobs.ts` the way every job is. The result: `ImportRun.report` rendered by the restore's
report component, the `refused` rows beneath it by number and code, and a link to the collection
that landed. A second import of the same file is the no-op P-08 promised, and the dialog says so
in the report rather than warning beforehand.

**Acceptance:** the four kinds are offered from the generated enum; a CSV's header row fills
the mapping and a column named like a field is prefilled; the run is followed and its report and
refused rows are rendered; the dialog is keyboard-operable and announces each step (F5-11,
F5-12); `pnpm -r build lint typecheck test` green.

**Read:** P-08…P-10 in `milestone-0.9.0.md`; `api/openapi.yaml` (`ImportRequest`, `ImportRun`,
`RestoreReport`, the `IMPORT` media purpose); `lib/data/media.svelte.ts` (`upload`);
`lib/data/jobs.ts`; `RestoreView.svelte` (the report); `UploadField.svelte`;
`hubctl import` in `cmd/hubctl/Import.go` (the reference caller)

---

## F6-10 — Template generation **[L]**

*Depends on: nothing. Issue #745.*

The last row of `ai-first.md` §2 reaches a screen. `TemplatesDialog` gains *Generate from a
description…* when the manifest's `ai_suggestions` is on (F5's decision 4: absence is absence): a
description in the person's words, `POST /templates:generate`, and the job followed. The
suggestion it records has the collection as its target, so it appears in the collection's strip
F5-04 built; `shapeOf` learns `TEMPLATE` — the payload is a `Template`, drawn read-only by
`TemplateEditor`'s tree as a `DECOMPOSITION` is — the heading and the accept verb get their codes
(`app.suggestions.kind_template`, `app.suggestions.accept_template`), and accepting is
`:accept`, after which the template is in the dialog's list because `CreateTemplate` put it
there. A node the profile refused is absent from the payload and the job's result count says so;
the strip shows the count.

**Acceptance:** the control is present exactly when AI is on; the job is followed and the
suggestion rendered as a tree; accepting creates the template and the list shows it; dismissing
closes it; `shapeOf`'s test covers `TEMPLATE`; the German catalogue carries the new codes;
`pnpm -r build lint typecheck test` green.

**Read:** P-11 in `milestone-0.9.0.md`; F5-02, F5-04 in `milestone-F5.md`;
`lib/data/suggestions.ts` (`shapeOf`, `headingCodeOf`, `acceptCodeOf`); `TemplatesDialog.svelte`,
`TemplateEditor.svelte`; `voice-and-tone.md` §7

---

## F6-11 — The CalDAV address **[L]**

*Depends on: nothing. Issue #746.*

P-06 serves every calendar feed as a `VTODO` calendar at
`/caldav/calendars/<account>/<feed>/`, under HTTP Basic with a personal access token as the
password, and no screen says so. `FeedsDialog` shows, beside each feed's `.ics` address, its
CalDAV address and one sentence — the account's address as the user name, a personal access
token as the password, a link to the tokens screen — and `TokensView` says, on the token it has
just minted, that it is what a calendar client asks for. Two sentences and one address; no new
component.

**Acceptance:** each feed shows its CalDAV address composed from the API base, the account and
the feed identifier; the tokens screen carries the sentence; both are in the German catalogue;
`pnpm -r build lint typecheck test` green.

**Read:** P-06, P-07 in `milestone-0.9.0.md`; `presentation/calendar/CalDav.go` (the tree);
`FeedsDialog.svelte`; `TokensView.svelte`; `docs/evidence/ECO-2026-09-16.md` (how Reminders
was pointed at it)

---

## F6-12 — The account remembers its moments **[L]**

*Depends on: nothing. The one core task, and the one F6-13 and F6-14 hang from. Issue #747.*

Decision 11. `AccountPreferences` gains two optional fields, additive and specification first
(ADR-0004): `celebrations` (`boolean`, absent means on — §7's default) and `onboarding_completed_at`
(`date-time` or `null`; the client writes it when the tour ends or is skipped, and clears it to
run the tour again). `Account` answers both. One expand migration adds the two columns to
`account`; `UpdateAccountPreferences` writes them under the rule it has for the others — an empty
string clears — and the merge rule for both is last writer wins by the account's own write, as for
`locale`. The data catalogue's account row gains the two fields with the deletion path the row
already has, `make generate` and `make api-client` produce the types, and the contract test passes.
No display text, no new use case, no new event.

**Acceptance:** `GET /accounts/me` answers both fields; `PATCH /accounts/{id}/preferences` writes
and clears both; the migration is expand-only; the data catalogue names both; `make generate` and
`make api-client` produce no diff after the commit; `go test -tags contract ./test/contract/...`
passes; `make verify` green.

**Read:** F1-08 in `milestone-F1.md` (the shape of the last account-side core task);
`api/openapi.yaml` (`AccountPreferences`, `Account`); `core/application/service/identity/UpdateAccountPreferences.go`;
`db/migrations/0001_init.sql` (the `account` table); `docs/privacy/data-catalog.md`;
`offline-sync.md` §4.2 (the merge rule to declare); ADR-0043 (why these are the account's and the
theme is not)

---

## F6-13 — The celebration kit **[L]**

*Depends on: F6-12, F6-04. Issue #748.*

Decisions 12 and 13. Three pieces:

1. **The guardrails as tokens.** `tokens.json` gains, under the `celebration` motion role, one
   duration per tier — tier 1 is the role's existing pair, tiers 2 and 3 longer and still short —
   and the two limits §7 names for the slot: the share of the viewport a celebration may claim
   and the extent of its motion, as dimension tokens. `make tokens` shows no diff in
   `LabelTokens.go`.
2. **The component.** `Celebration` in the design system: `tier` 1, 2 or 3, fills its slot from
   an asset per tier — tier 1 the micro-animation on the completed row, which `TaskRow` and
   `WorkItemCard` already carry the role for; tier 2 a movement across the parent row; tier 3
   the one that may be enjoyed, in the brand colours and inside the tokens' limits — never
   blocking (it is `inert`, outside the tab order, and the next completion is not delayed),
   announced once through a `status` region in the voice §7 asks for (no exclamation mark doing
   the work), and under `data-motion="reduced"` every tier is rule 6's colour change. Stories per
   tier, both modes, reduced motion.
3. **The triggers.** `lib/celebration.ts` in the application: from a completion the client just
   performed and the replica's entries, the tier — every completion is 1; siblings under the
   parent all complete is 2; a collection or a hub with nothing open, the last entry due today
   across the scope, or a completion while `onboarding_completed_at` is null is 3, capped at one
   tier-3 per day in the store's metadata and falling back to 2 (§7's table). Nothing heuristic,
   nothing random, and the test is a table over hierarchies. The profile gains the switch beside
   the theme's and motion's, bound to `celebrations`; off means no component is mounted.

Then `design-system.md` §7's heading gains *built* in the manner §4's rows have, with what the
build decided.

**Acceptance:** the tokens exist and pass the token test; the component exists with its stories;
the tier function is table-tested for every row of §7 and for the daily cap; a completion in the
entry list or on the board mounts the component at the tier the table says; the switch is on the
profile and off mounts nothing; `data-motion="reduced"` shows no movement in a story; `make
tokens` produces no diff in `LabelTokens.go`; `pnpm -r build lint typecheck test` green; `make
gate-docs` green.

**Read:** F6-12, F6-04; `design-system.md` §6 rule 6, §7 (all of it), §9 (the motion roles
paragraph); `voice-and-tone.md`; `tokens.json` (`motion.celebration`, `duration.celebrate`);
`lib/motion.ts`, `lib/device.svelte.ts`, `ProfileView.svelte` (the switches); F5-12 in
`milestone-F5.md` (how a change is announced)

---

## F6-14 — The onboarding tour **[L]**

*Depends on: F6-13. Issue #749.*

Decision 14. Two pieces:

1. **The pattern.** `Tour` and `CoachMark` in the design system: a spotlight on one element of
   the real interface — a cut-out over the glass overlay of rule 2, the element itself left
   interactive — with one caption and one body line in `voice-and-tone.md`'s register, *next*,
   *back* and *skip* as `Button`s, a step count, the whole thing a dialog for focus purposes
   (focus moves to the mark, `Escape` skips, the tab order is the mark's three controls and the
   element it points at), positioned by CSS anchoring the way every overlay is since ADR-0039.
   Stories: a step, the last step, both modes, both directions, reduced motion.
2. **The content.** Six steps in `apps/webapp`, anchored by `data-tour` attributes: the hub tree
   (where work lives), a collection's list and board (the same entries, two ways), the entry
   (what an entry carries), the jumble (where things arrive before they are work), the search
   field (the query language in one line), and the profile (language, theme, moments) — and the
   seventh, which is not a step: *now make one*, leading to the creation dialog and ending when
   the entry is completed, which fires decision 11's first celebration and writes
   `onboarding_completed_at`. The tour starts on the first sign-in of an account whose
   `onboarding_completed_at` is null and the help menu — a new entry in the frame's header,
   beside the footer's accessibility link — restarts it by clearing the field. Skipping writes
   the field too: a tour that comes back on every sign-in is one that teaches people to skip.

**Acceptance:** both components exist with stories; the six steps point at elements that exist on
the routes named and each is keyboard-operable end to end; a fresh account is led through and the
first completion celebrates at tier 3; skipping at any step ends the tour and writes the field;
the help menu restarts it; the German catalogue carries every step; F6-02's job asserts the
spotlight's cut-out is positioned by CSS; `pnpm -r build lint typecheck test` green.

**Read:** F6-13, F6-12; `design-system.md` §6 rules 2 and 5, §8, §10; `voice-and-tone.md`;
ADR-0039 (overlay positioning), ADR-0044; `Dialog.svelte`, `Popover.svelte`, `layers.ts`,
`overlay.ts`; `AppFrame.svelte`; `CreateContainerDialog.svelte` and the entry creation dialog

---

## F6-15 — The coverage report, the walk, and the documents current **[L]**

*Depends on: everything above. Issue #750.*

Decision 16, and the milestone's own acceptance. Three halves, in this order:

**The coverage report.** `docs/evidence/COVERAGE-<date>.md`: one row per use case
`catalogue.Descriptors()` returns — 231 today — with the web client's answer: the route and the
control that reaches it, or *deliberately omitted* with the reason (an agent's or a rule's use
case with no human surface; an operator's, reached through `hubctl` by `deployment.md`'s word;
a product question recorded and unanswered, by issue). `tools/checkdocs` gains the check decision
16 names, so the report and the catalogue cannot drift. Every omission whose reason is *nobody
built it* is not a reason: it is an issue, filed, and named in the row.

**The walk.** The integration environment's `demo` workspace, in a browser, in the shape the R-08
walks have: the connection cut with the developer tools while a list is open and the list staying
(F6-04); an entry completed, retitled and moved while offline, the pending marks, the connection
restored, the queue pushed, the server's word visible (F6-05, F6-06); a notes conflict made with
`hubctl` on the other side and resolved both ways; a device forgotten from another session
(F6-07); a CSV of two hundred rows imported into a hub, twice (F6-09); a template generated
against the stub provider (F6-10); a fresh account led through the tour to its first completion,
the celebration seen at tier 3 and then never twice in a day (F6-13, F6-14); and every route
opened once more with the keyboard, because the frame changed. Every defect found is an issue and
a pull request of its own, never a fix folded into this one; the evidence is
`docs/evidence/F6-<date>.md`.

**The documents.** `roadmap.md` gains the "F6 is done" paragraph in the shape F1's…F5's have —
what cutting it found (one core task, and why), what building it found — and its `0.9.5` row is
read against what this milestone leaves it: the coverage report exists for one client and
convergence completes it for three. `offline-sync.md` §1, §9 and §12 read line by line against
the client; ADR-0033 §4's browser row against `IndexedDbStorage`; `design-system.md` §4, §7 and
§8 against the components; `apps/webapp/CLAUDE.md` and `packages/sync-engine/CLAUDE.md` against
the code. The maturity stage stays `preview`, with the sentence that says `stable` is
convergence's.

**Acceptance:** the coverage report exists, `make gate-docs` holds it to the catalogue, and every
row is a route or a reason; the evidence file names each scenario, what was done and what was
seen; every defect found is an issue; the roadmap paragraph is written; the documents named are
current; `make verify` green; no code change in this pull request beyond `tools/checkdocs`.

**Read:** every task above; `docs/evidence/R-08-2026-09-11.md` (the shape of a walk);
`docs/roadmap.md` (`0.9.5`, the F1…F5 paragraphs); `core/application/catalogue/Catalogue.go`;
`tools/checkdocs/main.go`; `deploy/integration/README.md` (the `demo` workspace);
`offline-sync.md`; ADR-0033; `design-system.md`

---

## The order at a glance

```
F6-01 ─────────────────────────────────────────────────────┐
F6-02 ─────────────────────────────────────────────────────┤
F6-03 ──┬── F6-04 ── F6-05 ──┬── F6-06 ───────────────────┤
        │                    └── F6-08 ───────────────────┤
        └── F6-07 ─────────────────────────────────────────┤
F6-09 ─────────────────────────────────────────────────────┼── F6-15
F6-10 ─────────────────────────────────────────────────────┤
F6-11 ─────────────────────────────────────────────────────┤
F6-12 ──┬──────────── F6-13 ── F6-14 ──────────────────────┤
        │  (F6-13 also needs F6-04)                        │
        └──────────────────────────────────────────────────┘
```

Seven tasks depend on nothing and can start at once: the two ADR builds **F6-01** and **F6-02**,
the replica **F6-03**, the three ecosystem screens **F6-09**, **F6-10** and **F6-11**, and the
core task **F6-12**. The offline chain runs F6-03 → F6-04 → F6-05 and then forks into the two
components and the conformance run; the moments run F6-12 → F6-13 → F6-14, with F6-13 waiting for
the replica as well. F6-15 is last by definition.

**Definition of Done for the milestone:** a list stays on screen when the connection goes, says
*as of* when it does, and comes back current when the connection does; a change made while
offline is visibly pending, is pushed in order on reconnect, and is overwritten by the server's
word — rejected changes stay visible with their code until dismissed, a notes conflict is shown
with both versions and resolved by the person, and nothing in the client ever merges; the replica
is gone at sign-out; the account's devices are listed and can be forgotten; the engine passes its
own conformance run against a real instance in a required job; a CSV, a Trello export, a Google
Tasks export and a Microsoft To Do export can be imported from a hub's screen with the report shown;
a template can be generated from a description and accepted; a calendar client can be pointed at
a feed from what the screen shows; a completion is acknowledged at the tier the hierarchy says, at
most one big moment a day, switchable off, and reduced to a colour change under reduced motion; a
new account is led through the interface to its first completion and can restart the tour; every
use case of the catalogue is a route or a reason in a report the documentation gate holds to the
catalogue; every value still comes from `tokens.json`, the bundle carries no inline script or
style and contacts no origin the policy does not name; `go build ./...` and `go test ./...` still
succeed with no Node.js installed; and the documents say, line by line, what the client does.
