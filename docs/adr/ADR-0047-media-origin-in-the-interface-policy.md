# ADR-0047 — The interface's policy names the installation's media origin

**Status:** proposed · **Date:** 2026-09-08

## Context

Uploading a file is three steps and, deliberately, the server carries none of the bytes.
`POST /media` stages an object and answers where to put the bytes; the client `PUT`s them to that
URL; `POST /media/{id}:confirm` reads them back, sniffs them and seals the object. arc42 §8.4 lists
"presigned URLs" among the upload rules and C-06 built the flow that way, so that a hundred-megabyte
file never passes through the API process, its request-size limit, its rate limiter or its
deadlines.

`MediaTransfer.url` therefore has two shapes, and which one an installation produces is a
configuration rather than a client's choice:

* under `local`, `LocalTransfers` issues `…/api/v1/media/{id}:content?token=…` — **this server's own
  origin**, with an HMAC token standing in for a signature;
* under `s3`, `S3Storage` presigns a URL **on the storage endpoint** — `s3.eu-central-1.amazonaws.com`,
  or the MinIO or Garage host an operator runs beside Hubtask.

The interface has a content security policy, decided in [ADR-0028](./ADR-0028-embedded-web-ui.md) and
written down in [`security.md`](../architecture/security.md) §9. Every source in it is `'self'`,
which is the dividend of embedding the bundle in the binary: the document and the API come from one
origin, so nothing needs a foreign one. Two of its directives are what this decision is about:

```
img-src 'self' data: blob:; connect-src 'self'
```

Under `local` both media URLs are this server's, so both directives already permit everything the
upload and the cover need, and the surface F3-09 builds works as written. **Under `s3` neither
does.** The browser refuses the `PUT` before it is sent, because `connect-src 'self'` does not
include the bucket; it refuses to draw a cover from a presigned `GET`, because `img-src 'self'` does
not either. Both refusals happen in the browser, in front of the person, and produce a network
failure with no server involvement at all.

**The client cannot resolve this and must not try.** A policy is emitted by whoever serves the
document, which is `presentation/webui`; the bundle does not get a vote. And the one workaround
available to a client — asking the API for the bytes instead — is exactly what the three-step flow
exists to prevent.

So this is a server-side change to a policy that an ADR decided, which is why it is an ADR rather
than a line in a handler.

## Options

**A. Route the bytes through the API.** `POST /media/{id}:content` on this origin, the API streaming
to and from storage. Rejected. It undoes arc42 §8.4: the process that is supposed to hold a request
for milliseconds holds it for the length of an upload, the global body limit of `security.md` §9
(1 MiB) has to grow an exception per route, the anonymous and per-token rate limits start counting
megabytes as requests, and a Hubtask sized for its API traffic falls over on its media traffic. It
also makes the presigning machinery, the media token issuer and the reconciliation job pointless
work. This is not a smaller change than the one below; it is a different architecture.

**B. Widen the policy to `https:` or `*`.** One line, works everywhere, and throws away the reason
`connect-src 'self'` is in the list. That directive is what stops a compromised dependency in the
bundle from posting a workspace to an address of its choosing — the single most valuable line in the
policy, and the one ADR-0028 called out by name. Rejected without qualification.

**C. Fetch the cover and draw it from a `blob:` URL.** `img-src` already permits `blob:`, so the
draw would work. The fetch would not: `fetch` is governed by `connect-src`, which is the directive
that refuses the bucket in the first place. This option solves the half that is not broken.

**D. Keep object storage unsupported in the browser.** Covers and attachments work under `local` and
are gates with a reason under `s3`. Honest, and unacceptable as an end state: `s3` is what every
deployment beyond a single node uses, and a feature that disappears when an operator moves to object
storage is a feature the product does not have.

**E. The policy names the installation's configured media origin (chosen).**

## Decision

**`presentation/webui` adds the installation's configured media origin to `connect-src` and
`img-src`, and to nothing else.**

The server knows the origin because it configured the storage adapter; the bundle does not and never
learns it. Concretely:

* The origin is derived from `StorageConfig` — the endpoint under `s3`, resolved the same way
  `NewS3Storage` resolves it, so `HUBTASK_STORAGE_ENDPOINT` empty means `https://s3.<region>.amazonaws.com`
  and an operator's MinIO means their own host.
* It is an **origin**: scheme, host and port, with any path, query or trailing slash removed. A
  policy source with a path is a policy source most of a browser ignores.
* **Under `local` the policy stays byte for byte what ADR-0028 wrote.** The media URLs are already
  `'self'`, so there is nothing to add, and a test asserts the produced string equals the constant
  rather than merely containing it. That is what stops this change from quietly widening the default
  installation.
* Exactly one origin, never a list and never a wildcard. An installation has one storage
  configuration.
* No other directive moves. `script-src`, `style-src`, `font-src`, `worker-src`, `base-uri`,
  `form-action` and `frame-ancestors` are untouched, and there is still no `'unsafe-inline'` and no
  `'unsafe-eval'`.

`webui.NewHandler` gains the origin as a parameter and composes the policy once at construction, the
way it computes its entity tags once; the composition root passes what it already parsed for the
storage adapter. `security.md` §9's interface row gains the sentence, so the document and the
middleware keep saying the same thing.

**The bucket's CORS is the operator's half, and only the upload needs it.** A cross-origin `<img>`
is not a CORS request — `img-src` is the whole of what governs it — so a cover draws once the policy
permits the origin. The `PUT` is: it is a cross-origin request with a `Content-Type`, so the bucket
has to answer a preflight. The requirement is narrow and belongs in the operator's checklist rather
than in a client:

* allowed origin: the interface's origin, exactly, never `*`;
* allowed methods: `PUT`;
* allowed headers: `Content-Type`;
* credentials: **not** allowed — the presigned URL is the credential and the transfer deliberately
  sends neither bearer nor cookie.

Hubtask cannot set this itself. It does not own the bucket policy, the operator may have given the
application's credentials no permission to change it, and S3, MinIO and Garage each configure it
differently. Naming it precisely is what this project can do about it.

## Consequences

* The policy stops being a constant and becomes a value computed once at startup. The constant stays
  as the `local` case and as the thing the test compares against, so ADR-0028's string remains
  checkable rather than becoming folklore.
* An `s3` installation whose bucket has no CORS rule still fails — but it fails at the operator's
  configuration with a browser message that names the bucket, rather than failing at a policy the
  operator cannot see and did not write. That is a better failure, not the absence of one.
* The interface can now open a connection to one foreign origin. That is a real widening, and the
  argument for accepting it is that the origin is the installation's own storage, chosen by the
  operator, already trusted with every byte the workspace holds.
* A misconfigured `HUBTASK_STORAGE_ENDPOINT` produces a policy that refuses the upload. It fails
  closed, which is the direction `security.md` requires.
* The media origin's own policy does not change: it stays `sandbox` (`security.md` §9, T-11). An
  attachment is still served as a download and is still never rendered by this application, and the
  cover is still drawn only from the download URL the server answers for a `READY` object, which is
  the one path where the sniffed inline allowlist (C-05) has already judged the bytes.
* Until this is accepted, the client surface F3-09 builds is proved against `local` only. Nothing in
  it is conditional on the decision: the same three steps run against a presigned URL the moment the
  browser is allowed to reach it.

## Notes

Related: [ADR-0028](./ADR-0028-embedded-web-ui.md) (the policy this amends, and why every source in
it is `'self'`), [ADR-0015](./ADR-0015-security-baseline.md) and
[security.md](../architecture/security.md) §9 (the three origins and the header set),
[ADR-0018](./ADR-0018-privacy-by-design.md) (why the interface contacts no foreign origin it was not
configured with), [ADR-0027](./ADR-0027-monorepo-structure.md) (why the bundle cannot know the
origin). arc42 §8.4 is where "the server never carries the bytes" is written down; C-05 and C-06 in
`docs/backlog/milestone-0.3.0.md` are the tasks that built the pipeline.
