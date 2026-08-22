# ADR-0028 — The web UI ships inside the binary, as an adapter

**Status:** accepted · **Date:** 2026-08-22

## Context

[ADR-0027](./ADR-0027-monorepo-structure.md) puts the to-do application in `apps/webapp`. That
decides where its source lives, not how it reaches a user, and the second question has more
consequences than the first.

Two promises constrain the answer, and both are already made in public. The README's first bullet
says self-hosting means **"two containers, the full feature set, no limitations"**, and arc42 §1.2
makes operability quality goal 5 with the measure "one image plus PostgreSQL, `docker compose up`
in < 5 min". `deploy/docker/compose.yaml` today has exactly `db`, a one-shot `migrate`, and `app`.
A UI that arrives as a third long-running service breaks the sentence people decided to self-host
on.

The second constraint is version skew. The client is generated from `api/openapi.yaml`
([ADR-0004](./ADR-0004-api-first-openapi.md)). If the UI and the API can be deployed separately,
they can be deployed at different versions, and a self-hoster who updates one container and not the
other gets a UI calling endpoints that are not there. Nothing in the API can defend against that
beyond returning 404 politely.

The third is the security baseline, which turns out to already have an opinion. `presentation/rest/Security.go`
sends `Content-Security-Policy: default-src 'none'` on every answer, with the comment that it is
"written for an API that answers JSON and nothing else". That header is correct for the API and
fatal for an HTML document: a page served under it may load no script, no stylesheet, no font, and
may submit no form. `security.md` §9 lists the header set but specifies a `Content-Security-Policy`
only "for the media origin" — it has no policy for an HTML origin, because until now there was no
HTML. So a policy has to be decided here rather than invented quietly in a middleware.

## Options

**A. A second container plus a reverse proxy.** The webapp in an nginx or Caddy image, a proxy in
front routing `/api` to the binary and everything else to the static server. Rejected: it makes
self-hosting three or four containers instead of two, it makes version skew possible by
construction, and it puts a web server's configuration file — the classic source of a path-handling
mistake — into every self-hoster's deployment. It also contradicts
[ADR-0014](./ADR-0014-single-image-multi-role.md), whose whole argument is one image and one version
state.

**B. Serving the UI from a CDN.** The API self-hosted, the bundle from a hosted origin. Rejected
outright: it breaks air-gapped installations, it makes the product unusable when the operator's
network cannot reach a foreign domain, and it contradicts the offline-first premise of
[ADR-0021](./ADR-0021-offline-sync.md) — a client that cannot load without the internet is not an
offline client. It would also make a self-hosted Hubtask phone home on every load, which
[ADR-0018](./ADR-0018-privacy-by-design.md) does not permit and which the design system's
self-hosted fonts already refuse for the same reason.

**C. Server-side Go templates.** No bundle, no Node stage, no embedding problem. Rejected: the
documented client is offline-capable, keeps a local store and merges per field
(ADR-0021, offline-sync.md §4). A server that renders every page cannot do any of that, and rule 8
of CLAUDE.md — no display text in the backend — would have to be abandoned to render a single
label.

**D. The built bundle embedded in the binary and served by an inbound adapter (chosen).**

## Decision

**The built `apps/webapp` bundle is embedded into the Go binary with `embed.FS` and served by a new
presentation adapter, `presentation/webui/`.**

It is an inbound adapter in the sense of [ADR-0001](./ADR-0001-hexagonal-architecture.md), sitting
beside `rest`, `mcp`, `sse` and `calendar`. It reaches nothing but the HTTP layer; `core/` does not
learn that it exists. Serving bytes is not a use case, so nothing is registered in
`usecase.Registry` for it.

Four things follow, and each is a decision rather than a detail:

**1. The routing.** The UI is served at `/`. `/api/*` keeps priority absolutely and is never
shadowed — the API route tree is matched first, and a request under `/api` that matches no route
gets the API's own 404 with a `Problem` body, never an HTML page. Any *other* unmatched path falls
back to `index.html`, because a single-page application owns its own routes and a deep link into
one must not 404 on reload. `/healthz`, `/readyz` and the ops listener are equally untouched.

**2. The placeholder, which is what keeps Go buildable without Node.** `//go:embed all:dist`
fails to compile if `dist` does not exist, and `dist` is build output. So
`presentation/webui/dist/index.html` is **committed** — a short plain-text page stating that no UI
bundle was built into this binary — and everything else under `dist/` is ignored. A backend-only
contributor clones, runs `go build ./...`, and never installs a JavaScript toolchain; the container
build overwrites the placeholder with the real bundle. This is one of exactly two generated-looking
files this project commits on purpose; the other is in [ADR-0029](./ADR-0029-design-system-tokens.md).

**3. The content security policy, decided here because `security.md` §9 does not have one for an
HTML origin.** The existing `default-src 'none'` stays exactly as it is for `/api` and every other
JSON route — it is right there and nothing about a UI makes it less right. The UI routes get their
own, deliberately close to the API's:

```
default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:;
font-src 'self'; connect-src 'self'; manifest-src 'self'; worker-src 'self' blob:;
base-uri 'none'; form-action 'none'; frame-ancestors 'none'
```

Every source is `'self'`, which is the whole point of embedding: the bundle and the API come from
one origin, so nothing needs a foreign one. There is no `'unsafe-inline'` and no `'unsafe-eval'` in
either list — a framework that requires either does not meet the bar, and that constraint is
deliberately placed *before* the framework choice rather than after it. `font-src 'self'` is what
ADR-0029's self-hosted IBM Plex needs and is why loading from Google Fonts was never an option.
`worker-src` and `blob:` are there because an offline client needs a service worker;
`connect-src 'self'` is what lets it talk to its own API and nothing else. The remaining headers of
`security.md` §9 — HSTS, `nosniff`, `Referrer-Policy`, `Cross-Origin-Resource-Policy`, the
`Permissions-Policy` — apply to UI responses unchanged.

**4. Caching.** Content-hashed assets are served `Cache-Control: public, max-age=31536000, immutable`;
`index.html` is served `no-cache`. That pairing is what makes an update take effect on the next
reload while still letting a browser keep the bundle: the document is always revalidated, and it
names the hashed assets that may be cached forever.

**The UI can be switched off.** `HUBTASK_UI_ENABLED` (default `true`) is added to the existing
configuration surface in `core/port/environment` — the same mechanism as every other `HUBTASK_*`
variable, not a second one. When it is false, `/` answers 404 and the API is unaffected, which is
what an API-only deployment behind someone else's frontend wants. The state is reported through
`GET /api/v1/meta/capabilities` under `features`, alongside the other optional capabilities, so a
client discovers it rather than probing for it.

**The container build gains a UI stage.** `deploy/docker/Dockerfile` becomes `ui` → `build` →
`runtime`: Node and pnpm build `packages/api-client`, then `packages/design-system`, then
`apps/webapp`; the Go stage copies the result into `presentation/webui/dist` and links the binary;
the runtime stage is the distroless image it already is, with the same entrypoint and the same
`HUBTASK_ROLES`. The result is **one image**, and `compose.yaml` stays **two containers**. The Go
dependency download keeps its own layer ahead of the copy, so a frontend change does not invalidate
it.

**`apps/website` is not embedded.** It has no API contract with the binary and no reason to be in
it. It builds to a directory and deploys as static files; where it deploys is out of scope here.

## Consequences

* Version skew between UI and API is structurally impossible: both come from the same commit of the
  same image. The self-hoster's update is one `docker pull`.
* The image grows by the size of the bundle, and the production build now needs Node. That cost
  falls on the container build, never on `go build ./...`, and the placeholder is what enforces the
  distinction.
* The CSP above is a constraint on the framework choice, not a consequence of it. Whichever ADR
  picks the framework inherits "no inline script, no `eval`" as a requirement.
* `presentation/webui` serves bytes and therefore has no use case, no metric on a use case, and
  nothing in the `AuditableAction` registry. It does get the ordinary HTTP request metrics and
  span, like any other route.
* A deployment that wants a different frontend sets `HUBTASK_UI_ENABLED=false` and loses nothing.
* `security.md` §9 gains the UI policy alongside the media one when this is implemented, so the
  document and the middleware keep saying the same thing.

## Notes

Related: [ADR-0014](./ADR-0014-single-image-multi-role.md) (one image, one version state — this is
the same argument applied to the UI), [ADR-0027](./ADR-0027-monorepo-structure.md) (where the
source lives), [ADR-0029](./ADR-0029-design-system-tokens.md) (the fonts `font-src 'self'` serves,
and the second deliberately committed generated file), [ADR-0004](./ADR-0004-api-first-openapi.md)
(the contract both halves are generated from), [ADR-0021](./ADR-0021-offline-sync.md) (why a
rendered server page is not an option), [ADR-0015](./ADR-0015-security-baseline.md) and
[security.md](../architecture/security.md) §9 (the header set this extends),
[ADR-0018](./ADR-0018-privacy-by-design.md) (why no foreign origin is contacted on load).
The webapp's frontend framework is **not** decided here and needs its own ADR.
