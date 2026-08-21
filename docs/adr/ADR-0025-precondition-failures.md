# ADR-0025 — The status of a failed precondition

**Status:** accepted · **Date:** 2026-08-19

## Context

Two documents in this repository disagree about one status code, and the disagreement blocks B-05 and B-06.

[api-guidelines.md](../architecture/api-guidelines.md) §5 states the write semantics:

> **Optimistic locking** | `ETag` on `GET`, `If-Match` on `PATCH`/`PUT`; a conflict → `409 version_conflict`
> with the current version in the payload

§6 lists the standard mapping of every status this API produces, and `412` does not appear in it. The code
follows the guidelines: `shared.ErrVersionConflict` maps to `409`, `test/contract/api_test.go` asserts that
mapping against the document, and `identity.UpdateGroup` is the working precedent — `If-Match` becomes
`expected_version`, a mismatch answers `409 version_conflict`.

The backlog asks for something else. B-04's acceptance criterion reads *"an ETag round trip works (`If-Match`
on a later update returns `412` on a stale version)"*, and B-05 will inherit the same expectation when it adds
the first `PATCH` for an item.

The task text is not careless. RFC 9110 §13.1.1 is explicit that a failed `If-Match` produces
`412 Precondition Failed`, and §15.5.13 describes exactly this use: a client that read a state, formed a
request against it, and must be told the state moved. `409 Conflict` in RFC 9110 §15.5.10 is about a conflict
with the *target resource's state* more generally. On HTTP grounds `412` is the better-specified answer, and a
generic HTTP client or cache understands it without reading our error model.

Against that: `409` is what this API has documented and shipped. The error `code` is part of the contract and
SemVer-relevant (api-guidelines.md §6, versioning-release.md §8), so `version_conflict` cannot quietly become
something else. And there is a real semantic difference worth keeping — a version conflict discovered *without*
a precondition (two writers, no `If-Match`, the second one loses) is not a failed precondition at all, and
`412` would be wrong for it.

B-04 emits the `ETag` either way and needs no decision. B-05 and B-06 add the first `PATCH` for an item and a
container, and they cannot be written without one.

## Options

**A. `409 version_conflict` everywhere.** The documents and the code stay as they are; the acceptance criteria
in `docs/backlog/milestone-0.2.0.md` are corrected to say `409`. One status, one code, one precedent already
in production for groups. Costs: a client cannot distinguish "you sent a stale `If-Match`" from "somebody beat
you to it", and a caching proxy sees a status it has no special handling for.

**B. `412` for a failed `If-Match`, `409` for a conflict found without one.** Follows RFC 9110 and keeps the
distinction that matters: the precondition the client stated was not met, versus the state moved under a client
that stated nothing. Costs: a new error code (`precondition_failed`), a new sentinel in `core/domain/model/shared/Errors.go`,
a `locales/en.json` entry, a change to the §6 mapping table, a change to `UpdateGroup` and its tests, and every
client that already special-cases `409` has a second case to learn. Additive for a client that treats unknown
4xx as terminal; breaking for one that switches exhaustively.

**C. `412` everywhere, replacing `409 version_conflict`.** Rejected without further discussion: it removes a
documented error code, which versioning-release.md §8 lists as a breaking change, and it would answer `412` for
a request that carried no precondition — which is what RFC 9110 §15.5.13 is not about.

## Decision

**Option A is accepted.** A failed `If-Match` answers `409 version_conflict`, exactly as api-guidelines.md §5
and §6 document and as `ErrVersionConflict`, the contract test and `identity.UpdateGroup` already implement.
`412` is not introduced, and the code does not change.

The reason outweighs the RFC argument: a distinct `412` justifies itself only if a client can act differently
on it, and the two branches of that action are the same — re-read the resource and reapply. A second status
code that leads to the same recovery is a second thing to document, translate, test and support for no
behavioural gain, and it would arrive with a change to an operation (`UpdateGroup`) that is already shipped
and working.

This decision does not foreclose `412`. Should a real client need for the distinction arise, it can be
introduced additively at any time through a new ADR that supersedes this one — `precondition_failed` would be
a new code beside `version_conflict`, not a replacement, so nothing decided here would have to break.

## Consequences

**A, as accepted:** B-05 and B-06 implement `409 version_conflict` with the current version in the payload, which is what
`ErrVersionConflict` already does. The three acceptance criteria that say `412` (B-04's, and whatever B-05 and
B-06 inherit) are corrected in the backlog and in the corresponding issues, so the next reader of those tasks
is not sent looking for a status the API does not produce. Nothing in the code changes.

**B, not taken (kept for the record and for a possible successor ADR):** a new category and code, `412 precondition_failed`, joins the error model and the §6 table. The
distinction has to be drawn where the version is compared, which is the application layer: a use case that
received an explicit `expected_version` answers `precondition_failed`, one that fell back to the version it
read answers `version_conflict`. `UpdateGroup` and its tests change with it, and the contract test gains the new
mapping. The REST layer needs no new logic — it already turns `If-Match` into `expected_version`, and the
distinction falls out of whether that field was set.

Either way the acceptance criteria and the documents must end up saying the same thing. The present state — a
backlog asking for `412` and a contract promising `409` — is the only outcome that is certainly wrong.

## Notes

Related: [ADR-0004](./ADR-0004-api-first-openapi.md) (the contract is the source),
[api-guidelines.md](../architecture/api-guidelines.md) §5 and §6,
[versioning-release.md](../architecture/versioning-release.md) §8 (removing an error code is breaking).
Raised on issue #39 while implementing B-04; B-04 itself is unaffected and emits the `ETag` regardless.
