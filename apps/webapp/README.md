# @hubtask/webapp

The to-do application in the browser: Svelte 5 with runes and TypeScript, built by Vite as a
single-page application. The bundle is embedded into the Go binary and served at `/` by
`presentation/webui` ([`project-structure.md`](../../docs/architecture/project-structure.md) §7), so
the API and the interface always come from the same commit. The same bundle is what the Tauri
shells wrap ([ADR-0031](../../docs/adr/ADR-0031-tauri-app-shell.md)). The rules for changing it are
in [`AGENTS.md`](./AGENTS.md); what each screen draws is
[`design-system.md`](../../docs/design/design-system.md) §11.

```bash
pnpm --filter @hubtask/webapp build    # vite build, then build/check-csp.js
pnpm --filter @hubtask/webapp test
make run ROLES=api                     # the server that embeds and serves the build
```

## How it is put together

* **`App.svelte`** is the root: the route table for `src/lib/router.ts` (real paths over the
  History API, with the server's `index.html` fallback keeping a deep link alive), and the one
  place the language is applied — above the frame, because the signed-out card is not inside it.
* **`src/lib/frame/`** is the shell every signed-in view sits inside: the app bar, the navigation,
  the page head, `SyncLine` (the copy and the server, on every route) and the tour guide.
* **`src/lib/platform/`** is the seam between the bundle and where it runs. Today it has one
  implementation, the browser's; a shell's build selects its own, never runtime sniffing.
* **`src/lib/data/`** is the application's side of `@hubtask/sync-engine`. `engine.ts` builds the
  one engine, attaches the replica per API origin and account, and performs the session exchange.
  `replica.ts` supplies `storeFor` and `mutationFor`, `live.ts` the `pathsFor` that says what a
  change record makes stale, and `touches.ts` the path names a write invalidates — the engine is
  product-agnostic, and only these modules know which path is which record. The `*.svelte.ts`
  modules wrap engine resources for components.

## What the application knows about itself

Four modules hold it, read once and shared:

* `lib/data/capabilities.svelte.ts` reads `/meta/capabilities` at boot and again on every change of
  actor: the route takes no credential, but the request carries one, so a stale bearer is answered
  `401`, and the answer is scoped by the caller.
* `lib/data/account.svelte.ts` reads `GET /accounts/me` when there is a bearer; its `locale`
  outranks the browser's ([`i18n-l10n.md`](../../docs/architecture/i18n-l10n.md) §2).
* `lib/data/health.svelte.ts` reads `/meta/health` only where the actor may: no bearer means no
  request, and a `401` or `403` is silence.
* `lib/maturity.ts` carries the product stage the maturity banner reads.

`lib/data/capability.ts` answers questions such as "may a `TASK` carry a bucket" or "may this role
change this entry" from the manifest alone. Its verdict has three values: permitted, refused, and
undetermined until the manifest is read. A predicted refusal carries the server's own message code
(`items.capability_not_supported`, `items.parent_type_invalid`), so one fact has one sentence
whether the client saw it coming or the server sent it. A refusal the reader might otherwise have
had is drawn by the design system's `CapabilityGate`, with its reason.

## Device, account and words

* **The theme and reduced motion belong to the device.** `lib/theme.ts` owns `data-theme`,
  `lib/motion.ts` owns `data-motion`, and `lib/device.svelte.ts` keeps both choices in this
  browser's `localStorage`; the media query is honoured whatever is chosen. The celebrations switch
  and the tour's completion are the account's preferences, because a person who turned them off did
  so everywhere. `lib/celebration.ts` decides a completion's tier from the replica, `lib/tour.ts`
  names the tour's steps.
* **The session is a bearer pair in `sessionStorage`** (`lib/platform/tokenStore.ts`): it survives
  a reload and ends with the tab. A refused request triggers one refresh in `lib/data/engine.ts` and
  one retry; sign-out ends the session at the server, then calls `engine.reset()` and
  `releaseBearer()` ([`identity.md`](../../docs/architecture/identity.md) §14.4).
* **Words come from codes.** `src/lib/i18n/` holds the ICU renderer, the locale resolution and the
  source-language fallback; `catalogue.ts` reads `locales/en.json`, which also holds the
  application's own `app.*` codes, and `catalogue.test.ts` parses every message. `lib/problem.ts`
  turns a `TransportError`'s problem document into words: it chooses between `code` and the more
  specific `detail_code`, puts each `field_errors[]` entry under its `path`, and adds the
  `request_id` where the sentence does not carry it.
