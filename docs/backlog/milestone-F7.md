# Milestone F7 — The shells, free to install

The goal: **Hubtask installs on Windows, Linux and Android from the project's own releases, without
a store and without anybody paying for an account, and keeps the offline promise there**: the app
signs in to any workspace with every way in it allows, passkeys included, keeps an encrypted copy
under the platform's keystore, carries every change the offline column promises, holds several
workspaces, stays current by itself, and runs the same smoke walk on WebView2, WebKitGTK and
Android's system WebView. macOS, iOS and the stores wait for `2.0`
([ADR-0084](../adr/ADR-0084-the-first-majors-shells-cost-nothing-to-ship.md)).

The rules are in [ADR-0031](../adr/ADR-0031-tauri-app-shell.md) (the framework),
[ADR-0032](../adr/ADR-0032-client-capability-matrix.md) (what the phone leaves out),
[ADR-0033](../adr/ADR-0033-shared-client-architecture.md) (one web app, one engine, the shell at
`apps/webapp/src-tauri/`), [ADR-0035](../adr/ADR-0035-one-product-version.md) (one version and the
platform build counter), [offline-sync.md](../architecture/offline-sync.md) §1, §9 and §12 and
[design-system.md](../design/design-system.md) §9; the sign-in is drafted in
[identity.md](../architecture/identity.md) §15.3 and §23.3 with
[ADR-0096](../adr/ADR-0096-the-installed-apps-sign-in-through-the-system-browser.md). Every task
names its use cases; a task is done when the named checks hold, with the evidence in the pull
request.

**Delivers:** UC-SYN-01 (1, 6, 7), UC-SYN-07 (1, 3, 4, 5, 6, 7, 8, 9), UC-ID-25 (7), UC-WRK-33 (5)

## Decisions

Each measured against the deployments `D1`–`D7`:

1. **Windows, Linux and Android, outside every store, up to `2.0`** (the owner, 2026-10-13,
   ADR-0084): installers and an APK as direct downloads with checksums, the APK signed with the
   project's own key, no commercial certificate; the operating system's warning is explained, not
   avoided.
2. **One Tauri project at `apps/webapp/src-tauri/`, desktop and mobile targets** (ADR-0033): the
   shell renders the web app through its `platform/` seam, the pnpm workspace and Go ignore the
   directory, and only a lane of its own builds it. No second app directory (`apps/* → apps/*` is
   refused).
3. **The apps sign in through the system browser, as the installation's own OAuth2 client**
   (worker, at the cut; ADR-0096): a passkey's RP ID is the workspace's host, which a webview's
   origin can never be; a built-in public client with PKCE reuses every way in and every rule the
   web app has, and needs nobody to register it (P-09).
4. **The Rust toolchain and the Tauri crates are the owner's** (#1763, open): a new third-party
   dependency. The proposal: Tauri with its updater, deep-link and opener plugins, `rusqlite` with
   SQLCipher, `keyring`, a Kotlin bridge of our own to the Android Keystore, and `cargo-deny` for
   licences.
5. **Android updates** (#1764, open): Tauri's updater has no Android; the proposal is a notice that
   opens the new APK's download. UC-SYN-07 check 6 is written as proposed.
6. **A file captured offline** (#1765, open): UC-SYN-01 check 6 and UC-MED-01's *Where it ends*
   disagree; the proposal is that the installed apps keep and upload it later, the browser not.
7. **SY-B: the phone synchronises everything, as the browser does** (worker, at the cut): UC-SYN-01
   check 1 promises a copy of everything the person may read; F7-07 measures it on a reference
   phone, and only a measurement that rules it out brings a smaller default to the owner, because
   it would change that check.
8. **The download page is `0.9.5`'s** (worker, at the cut): the site says the installed apps in the
   future tense until `0.9.5` (`apps/website/AGENTS.md`, roadmap.md), so UC-SYN-07 check 2 and
   #1353 move to milestone `0.9.5`. F7 makes the releases carry the apps (check 1).
9. **The formats** (worker, at the cut): Windows a per-user NSIS installer (no administrator rights
   needed), Linux an AppImage and a `.deb`, Android one universal APK; `versionCode` is ADR-0035's
   platform build counter.
10. **The two keys are the project's and the owner creates them once** (worker, at the cut): the
    APK signing key and the update key live as secrets of the release's environment, their public
    halves and fingerprints on the download page and in versioning-release.md. A lost APK key means
    no update for installed Android apps, so the owner keeps an offline copy; versioning-release.md
    gains the procedure in F7-08.
11. **The shell's screens are shown to the owner before they are built** (worker, at the cut, as the
    owner asked for PL and CD): the server's address, the workspace list and the phone's
    administration affordance start with a preview.
12. **The app speaks to a server of its own version or newer** (worker, at the cut; ADR-0035):
    tolerant of unknown fields, and a server older than the app is named, not worked around.

---

## F7-01 — The desktop shell builds and runs

*Depends on: #1763.* · issue #1348 · ADR-0031, ADR-0033

**Use cases:** UC-WRK-33 (5)

The Tauri project at `apps/webapp/src-tauri/` with its own CI lane on Windows and Linux runners,
claimed by the paths filter and its actions on the allowed list; `cargo-deny` under the licence
policy beside `go-licenses`. The `platform/` seam gains the `desktop` target, chosen at build time:
the API's base is the server the person chose instead of the page's origin, the device's name comes
from the system. The app's icons on every size come from the mark's app-icon source
(`packages/design-system`). Administration is all there (ADR-0032).

**Acceptance:** the lane builds both targets on every change to `src-tauri/` or the web app; a unit
test of the seam's target choice; a check in the lane that the icons are generated from the mark,
not committed by hand; UC-WRK-33 check 5 proven for the desktop with F7-06 for Android.

---

## F7-02 — Signing in through the system browser

*Depends on: F7-01.* · ADR-0096 · identity.md §15.3, §23.3

**Use cases:** UC-SYN-07 (3)

The contract first: the built-in public client in §15.3's provider, accepted by every workspace,
redirecting only to a loopback address (desktop) or `eu.hubtask.app:/oauth` (Android). The app asks
for a workspace's address, opens the authorization at that host in the system browser, and redeems
the code with its PKCE verifier; the consent names the app and the device. The pair goes into the
platform's keystore, never the webview's storage, and the app refreshes once per refusal (§14.4). A
step-up that wants a passkey goes the same way. ADR-0096 moves to `accepted`.

**Acceptance:** application tests that the built-in client accepts only its redirects and only
`S256`, that its grant carries a session's scopes and is listed and revocable; an integration test
of the code exchange; a shell test that no token reaches web storage; a walk signing in with a
password, a provider and a virtual passkey through the browser; the preview shown to the owner first
(Decision 11).

---

## F7-03 — Several workspaces in one app

*Depends on: F7-02; CD-09 for the signed entry.* · multi-tenancy.md §3.1

**Use cases:** UC-ID-25 (7), UC-SYN-07 (4)

The app keeps a list of workspaces, of one installation or several, each with its own grant in the
keystore and its own copy; *Switch workspace* and *Add a workspace* (by address) in the account
menu, *Forget this workspace* revoking the grant and deleting the copy. Entries hold host and
display name, as in the web app.

**Acceptance:** shell tests that two workspaces of two installations hold separate grants and copies,
that forgetting one revokes its grant and deletes its copy and nothing else; a walk switching
between two workspaces; the preview shown to the owner first (Decision 11).

---

## F7-04 — The encrypted copy under the keystore

*Depends on: F7-01.* · offline-sync.md §1, §9 rule 6, SY-D · ADR-0033 §4

**Use cases:** UC-SYN-01 (7)

A `Storage` implementation on SQLite encrypted at rest, its key created once per account and
workspace and held in the platform's keystore, behind the engine's existing port. Signing out,
`ACCESS_REVOKED` and `sync.gone` delete what they delete in the browser, and signing out also
removes the key. The copy survives a restart and the app works from it offline.

**Acceptance:** the engine's storage contract tests run against the SQLite implementation; a shell
test that the database file is unreadable without the key and that sign-out removes file and key;
`hubctl sync-conformance`'s rule 6 recorded as proven for the shells, with its evidence.

---

## F7-05 — The changes the push frame does not carry yet

*Depends on: nothing; the attachment part on #1765.* · issue #1351 · offline-sync.md §1, §3.2, §4.2

**Use cases:** UC-SYN-01 (6)

The contract first: mutation kinds for a reminder (whole on creation and deletion, then per field),
a recurrence rule (the same), applying a template, and the structure (containers, label definitions,
buckets), each merged by offline-sync.md §4.2's rule for its kind and refused with codes where the
server must decide. The server applies them through the same use cases as the routes; the engine
queues them; the web app offers them offline where it today answers `sync.needs_connection`.
`hubctl sync-conformance` rule 8, which today asserts that applying a template is refused, follows
§1. A file captured offline follows #1765.

**Acceptance:** table tests for the merge of each kind; integration tests of each kind through
`/sync/push`, a replayed `op_id` applied once, and a cross-tenant negative test; engine tests that
each kind is queued and replayed in order; the conformance rule changed with its test; an e2e walk
offline in the browser.

---

## F7-06 — The Android app

*Depends on: F7-02, F7-04.* · issue #1349 · ADR-0032 · design-system.md §9

**Use cases:** UC-SYN-07 (7, 8), UC-WRK-33 (5)

The mobile target of the same project, in the lane on an Android runner: one APK signed with the
project's key, `versionCode` from the build counter. The build leaves out the routes and navigation
rows tagged `administration` and `instance`; each place where they would be names them and opens
the web app of the server the app is signed in to. The conventions only a shell can follow: the back
gesture as history, the share sheet, the keyboard's accessory row, the status bar's colour. The
icon comes from the mark.

**Acceptance:** a build test that no administration route is in the mobile bundle; a walk on an
emulator showing the affordance, the back gesture and the share sheet; UC-WRK-33 check 5 proven for
Android; the preview shown to the owner first (Decision 11).

---

## F7-07 — What the phone synchronises

*Depends on: F7-06.* · issue #1352 · offline-sync.md §12 (SY-B)

**Use cases:** UC-SYN-01 (1, 7)

SY-B decided by measurement: the first synchronisation of the measured workspace (66,867 entries)
on a reference phone, its time, storage and memory, and the resumption after an interruption. With
everything within bounds (Decision 7), the phone asks for everything like the browser, and
offline-sync.md §12 closes SY-B with the figures.

**Acceptance:** the measurement as an evidence record with the device named; an emulator test that a
first synchronisation resumes after an interruption from where it stopped; offline-sync.md §12
updated.

---

## F7-08 — Releases carry the apps

*Depends on: F7-01, F7-06.* · ADR-0084 · versioning-release.md

**Use cases:** UC-SYN-07 (1)

`release.yml` builds the Windows installer, the AppImage and the `.deb`, and the APK from the
release's commit, signs the APK and the update manifest with the project's keys, and publishes them
with one checksum file beside the image, the chart and the SBOM. The platform build counter is
written into each artefact. versioning-release.md gains the keys' procedure, including what a lost
key costs (Decision 10).

**Acceptance:** a release dry run on a tag in a fork or a pre-release that publishes every file and
the checksum file; a test that the checksums match the files; the keys' fingerprints in
versioning-release.md.

---

## F7-09 — Staying current

*Depends on: F7-08; the Android part on #1764.* · issue #1350 · ADR-0084

**Use cases:** UC-SYN-07 (5, 6)

On Windows and Linux, Tauri's updater reads the release's signed manifest, checks it against the
project's update key, and installs after the person agrees; a manifest or a package with a wrong
signature is refused and the app says so. On Android, as #1764 decides. One updater run is exercised
end to end from a released version to its successor (`1.0.0` prerequisite 19).

**Acceptance:** a shell test that a wrongly signed update is refused; the end-to-end run from one
pre-release to the next as an evidence record; the Android notice's test.

---

## F7-10 — The smoke walk on three webviews

*Depends on: F7-02, F7-06.* · `1.0.0` prerequisite 18

**Use cases:** UC-SYN-07 (9)

One smoke walk — sign in, open a hub, create, edit and complete an entry, sign out — driven through
WebDriver in the Windows app on WebView2, the Linux app on WebKitGTK, and the Android app on the
system WebView in an emulator, in the shell's lane. A failure in one engine fails the lane.

**Acceptance:** the walk green on all three in CI; support-matrix.md names the three engines as
proven by the job.

---

## The matrix

| Door | What F7 changes |
|---|---|
| Web app | The offline kinds of F7-05 in the browser too; nothing else. |
| Desktop app (Windows, Linux) | Every end-user, profile, administration and installation screen of the web app; sign-in through the browser, the keystore, the encrypted copy, the updater. |
| Android app | End-user and profile screens; administration and the installation's area named and linked to the web app. |
| REST | The built-in OAuth client; the new mutation kinds on `/sync/push`. |
| MCP, `hubctl`, automation | Nothing; `hubctl sync-conformance` follows §1. |
| Background worker | Nothing new. |
| Installation level | Nothing: the apps talk to any installation through the API. |

| Account | In the app |
|---|---|
| Person with a password or a factor | Signs in through the browser, as on the web. |
| Provider-only person | The provider's sign-in in the system browser, never in a webview. |
| Person with a passkey | The passkey at the workspace's host in the system browser. |
| Managed account (no address) | Its password through the browser. |
| Service account | No app; tokens only. |
| Restricted, disabled, suspended | As at every door; `ACCESS_REVOKED` deletes the copy. |

Deployments: `D1`–`D7` alike; the app holds workspaces of any installation, single or multi mode,
and needs nothing from the operator.

## The cut checklist

1. **What it delivers** — UC-SYN-07 is new (in `specified`); UC-SYN-01 checks 6 and 7 were written
   for the shells (check 1 is re-proven on the phone); UC-ID-25 check 7 comes from CD; UC-WRK-33
   check 5 was given to F7 by decision #1671. UC-SYN-07 check 2 is `0.9.5`'s (Decision 8). Walked
   through `D1`–`D7` above.
2. **Principles** — P-02 (every way in at the workspace's own host, never weakened in a webview),
   P-03 (the copy survives a restart; a refused offline change stays visible), P-05 (the phone
   names what it leaves out and links it), P-09 (no store, no account, no administrator's
   registration needed), P-11 (the update and the operating system's warning explained), P-13
   (the shells render the web app, its accessibility included).
3. **The matrix** — above.
4. **The premise** — checked against `main` at d8443afee: no Tauri, Rust or Cargo anywhere, only
   `src-tauri` already skipped by `build/lint-workspace-map.mjs`; the `Storage` port has an
   IndexedDB and a memory implementation, unencrypted by decision; the push frame carries seven
   kinds, and reminders, recurrence, templates and structure are read-only offline; `hubctl
   sync-conformance` asserts applying a template is refused, against offline-sync.md §1; the
   `platform/` seam knows only `browser`, the API base is the page's origin, and routes and rows
   carry an `area` but nothing excludes them; a phone layout exists; releases publish the image,
   the chart, the SBOM and the licences, no binary and no checksum file; Tauri's updater supports
   no Android; the website's download page says `hubctl` is published, which no release does (fixed
   on this branch).
5. **Decisions** — above; three wait on the owner: #1763, #1764, #1765.
6. **The rules** — identity.md §15.3 and §23.3 drafted for the sign-in, ADR-0096 proposed and
   accepted when F7-02 merges; offline-sync.md §1, §3.2 and §12 change in F7-05 and F7-07;
   versioning-release.md in F7-08; support-matrix.md in F7-10.
7. **Findings** — none open of the area. Found at the cut: the conformance rule against §1 (F7-05),
   UC-MED-01 against UC-SYN-01 (#1765), and the website's `hubctl` claim (fixed on this branch).
8. **Tasks** — each names its checks, dependencies and acceptance; together they carry `Delivers`.
9. **The standing rules** — rule 1 and rule 14 (no Go under `apps/`, the core knows no shell), rule
   2 (the built-in client authorised in the application layer like every client), rule 3 and rule
   9 (the new kinds through the transaction wrapper and sqlc), rule 5 (the server's side of the new
   kinds without a bare goroutine), rule 6 and rule 7 (the updater talks to the release, not
   through the server; the server's side unchanged), rule 10 (no title or note in the shell's
   logs), rule 11 (the contract first for the client and the kinds), rule 13, rule 15 (the shell's
   chrome from the tokens); known traps: a write on a refusal path in the push handler, credentials
   in web storage.
10. **A look before the build** — F7-02, F7-03 and F7-06 change screens; each starts with a preview
    the owner sees (Decision 11).
