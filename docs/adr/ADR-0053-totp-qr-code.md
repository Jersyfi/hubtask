# ADR-0053 — The TOTP QR code: a dependency, an encoder, or neither

**Status:** proposed · **Date:** 2026-09-09

## Context

`POST /auth/mfa/totp:enroll` answers two things about one secret: `secret`, base32, "for typing by
hand where no camera reaches the QR", and `otpauth_uri`, whose description says in as many words
that **"the rendering is the client's job"**. H-02 made the same decision from the server's side —
"answers the provisioning URI once (the QR is a client's job)" — and left the client with a
question nobody has answered: what draws it.

A QR code is not a picture the server can send instead. Rendering one is arithmetic: the payload is
segmented and encoded, Reed–Solomon error correction is appended, the modules are laid out in a
matrix with the three finders, the alignment patterns, the timing patterns and the format
information, and then eight masks are scored against four penalty rules to choose one. It is a
well-specified, well-bounded piece of work (ISO/IEC 18004) and it is not five lines.

**This workspace has nothing that draws one.** `pnpm-lock.yaml` carries no QR library. Adding one is
a supply-chain decision, which `CLAUDE.md` reserves — "every dependency is a supply chain decision"
— and that is why this is an ADR rather than a pull request with a lockfile change in it.

Three things bound the decision and are worth stating before the options:

* **Enrolment works without it.** Every authenticator worth the name accepts a base32 secret typed
  in, and the contract answers one for exactly that reason. A QR is convenience, not capability.
* **The bundle is embedded in the binary** ([ADR-0028](./ADR-0028-embedded-web-ui.md)), so whatever
  is chosen is shipped to every self-hoster, in the image, forever. A dependency here is not a
  dependency of a build step.
* **The content security policy has no `'unsafe-inline'` and no foreign origin.** An
  externally-hosted QR service is not an option for a second reason as well as the obvious first
  one: sending the provisioning URI — which contains the shared secret — to a third party would be
  handing somebody else the second factor.

## Options

**A. A dependency.** `qrcode`, `qrcode-generator`, `qr-code-styling` or similar from npm.

The cost is not the code size; it is the standing obligation. It becomes a row in the lockfile
that Dependabot raises pull requests against, a line in `THIRD-PARTY-LICENSES.md`, an entry in the
supply-chain surface `security.md` §7 asks about, and a thing that is in the image of every
self-hosted installation for as long as the product exists — for one screen somebody sees once per
account. The smallest credible candidates are a few kilobytes and a handful of transitive
dependencies; the smallest are also the least maintained, which is the usual trade.

**B. An encoder in the design system.** Roughly two hundred lines of TypeScript in
`packages/design-system`, plus a component that draws the matrix as an SVG.

The arithmetic is fixed by a public standard and the input is bounded — an `otpauth://` URI is well
under a hundred characters, which sits inside version 4 at error correction level M, so the encoder
needs one version's alignment pattern table rather than all forty. It is testable against published
vectors, which is what makes it maintainable by somebody who did not write it. The honest costs:
it is a body of code with no second caller today, it has to be got exactly right or it produces a
code that scans as the wrong secret, and "we wrote our own QR encoder" is a sentence a reviewer is
entitled to be suspicious of.

**C. No QR at all: the base32 secret, formatted for typing.**

The screen shows the secret in groups of four, with a copy control and the account and issuer the
URI carries. Every authenticator accepts it. The cost is a worse enrolment: somebody types
thirty-two characters into a phone instead of pointing a camera at a screen, once per account, and
some people will get it wrong on the first attempt — which the confirmation step catches, because
enrolment arms only after a valid code.

**D. A link that opens the URI.** `otpauth://…` as an anchor, so that a person enrolling *on the
device that holds the authenticator* is handed straight to it.

Not an alternative to the other three — it is a strict addition, it costs nothing, and it happens
to be the best answer for a phone, where a QR on the same screen cannot be photographed by the
camera above it. Its limit is that it does nothing on a desktop with no handler registered, which
is the common case.

## Decision

**Deferred to the owner.** This ADR is `proposed` and F4-04 ships **C and D** meanwhile: the secret
in groups of four with a copy control, and the provisioning URI as a link for the reader who is
already on the device that will hold it.

That is not a placeholder in the sense of something that has to be replaced. It is a complete
enrolment, and it is what the contract's own `secret` field exists to make possible. What it is not
is the enrolment somebody expects from a product in 2026, and choosing A or B later is additive: the
screen grows an image beside the secret it already shows.

**The recommendation, for whoever decides, is B** — on the reasoning that this project applies to
every other dependency question it has taken. `router.ts` is in-house because the need was path
matching against a stable browser API; `positioning.ts` is in-house because the need was one
fallback; the ICU renderer is in-house because a second catalogue was not an option. A QR encoder
is the same shape of need: fixed by a standard, bounded by one input, with no second caller and no
version churn ahead of it. The counter-argument is real and should be weighed rather than dismissed:
correctness here is not obvious by reading, and a wrong code is a *silent* wrong code — it scans,
it produces six digits, and they are the wrong six digits. That is why B is a recommendation and not
a decision taken in a backlog task.

## Consequences

* **F4-04 is not blocked.** Enrolment ships, is confirmed by a code from a real authenticator, and
  arms. Nothing in the second-factor work waits on this answer.
* **The screen has a seam for it.** The secret and the URI are rendered by one component, so
  whichever of A or B is chosen adds an image inside it rather than rearranging a screen.
* **If A is chosen**, the pull request that adds it carries the lockfile change, the licence entry
  and the answer to `security.md` §7's supply-chain questions, and this ADR is superseded by one
  naming the package and the version policy.
* **If B is chosen**, the encoder ships with the published test vectors as its tests, in
  `packages/design-system`, and `design-system.md` §4 gains a component row for what draws it.
* **If C stands**, this ADR is accepted as it is and the enrolment screen stops being described as
  provisional — which is the outcome that needs to be *decided* rather than reached by nobody
  choosing.

## References

* [`security.md`](../architecture/security.md) §5, §7 — MFA, and the supply-chain questions
* H-02 in [`backlog/milestone-0.6.0.md`](../backlog/milestone-0.6.0.md) — the server half
* F4-04 in [`backlog/milestone-F4.md`](../backlog/milestone-F4.md) — the task this opens
* [ADR-0028](./ADR-0028-embedded-web-ui.md) — the bundle is in the binary
* [ADR-0030](./ADR-0030-svelte-frontend-framework.md), [ADR-0039](./ADR-0039-overlay-positioning.md)
  — the two precedents for building rather than depending
* RFC 6238 (TOTP), ISO/IEC 18004 (QR)
