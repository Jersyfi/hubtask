# ADR-0062 — The pinned tools are cached as binaries, and a stamp says what a cache holds

**Status:** accepted (2026-09-22) · **Date:** 2026-09-22

## Context

`make tools` installs nine pinned tools into `.tools`. Eight of them are compiled from source
through the module proxy, which is [ADR-0015](./ADR-0015-security-baseline.md)'s answer to what a
tool is: a pinned version, fetched through the Go checksum database, reproducible on a laptop and
on a runner by the same command.

It takes **215 seconds**, measured as the median over the 189 finished CI runs of 2026-09-17…21,
and eight jobs of `ci.yml` need it. That is **29 of a Go pull request's 69 runner-minutes**, and
because `quick` and `selftest` are the two ends of the critical path, seven of the twenty-one
minutes a contributor waits (#911).

It was supposed to be paid for once. `actions/setup-go` is asked for `cache: true` in every job,
and the module cache it restores would hold what those eight `go install` invocations need — but
the action saves its cache **only when the key is not already present**, so the entry belongs to
whichever job finishes first. That is `docs` or `licences`: the two that compile nothing. The
entry every other job then restored was the 9 MB they left behind, and had been since the cache
was introduced.

Fixing that inside `setup-go` is not possible — one key per go.sum, one saver, and the saver is
the cheapest job by construction. What the eight jobs actually want is not the modules the tools
are built from; it is the tools.

## Decision

**CI restores `.tools` from a cache and installs only what is not in it. The decision about what
is in it is made by the Makefile, from a stamp, and never by the cache key alone.**

1. `make tools` writes `.tools/.installed` as its last step — one line holding the nine pins and
   the `go env GOVERSION` that compiled them. It is removed at the start of the recipe, so it
   exists only after a run that reached the end.
2. `make tools-ensure` is what a job with a cache in front of it calls. It installs the whole set
   unless the stamp matches today's pins *and* every binary the pins name is present. Anything
   else — no stamp, an older pin, a moved toolchain, a missing file, a half-written directory — is
   a full install.
3. The cache key is the platform, the Go version and a hash of the `Makefile`, which is where
   every pin lives. The restore key drops the hash, so a moved pin starts from the previous set
   rather than from nothing; `tools-ensure` then throws that set away, which is the point of
   deciding from the stamp rather than from the hit.
4. `release.yml` keeps `make tools`. What is published is built by a job that compiled its own
   tools from the pins in that commit, with no cache anywhere near it.

## Why this does not weaken ADR-0015

The rule ADR-0015 states is that a tool version is pinned and vouched for, not that it is compiled
again in every process that uses it. What the cache changes is who ran the compiler, so the
question is whether anything can put a binary into `.tools` that the pins do not describe.

* **A pull request cannot.** A cache written by a workflow run on a pull request is created for
  `refs/pull/N/merge` and can only be restored by re-runs of that same pull request — not by
  `main`, and not by another pull request. So a contributor's branch cannot leave a binary behind
  that anything else will pick up; the worst it can do is poison its own run, in which every gate
  it would fool also fails to prove anything about a change that is not merged.
* **A drifted entry cannot survive.** The stamp is compared, not trusted: a `.tools` restored from
  a key that no longer describes today's pins is reinstalled in full. That is stricter than the
  state before this decision, where `tools-promtool` skipped its download whenever the binary
  existed, whatever version it was — latent only because every run started from an empty
  directory, and the defect that would have become reachable here. It now compares the version.
* **The release path is untouched.** The artefacts anybody installs are built by `release.yml`,
  which compiles the tool set from the pins of the tagged commit.

What is genuinely accepted is narrower: a gate on `main` may run a linter that a previous run on
`main` compiled, from the same pinned version, in the same repository, on the same platform.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: cache `.tools`, decide from a stamp** | Removes the 215 seconds from eight jobs. The correctness question moves into the Makefile, where it can be run and mutation-tested on a laptop instead of only in CI. |
| Per-job Go build and module caches | Attacks the second-order cost (compiling the project, not the tools) and is worth doing next, but thirteen jobs each holding a build cache is a real question against the repository's 10 GB, and the answer needs measuring first. Deliberately left out of this decision. |
| Download release binaries instead of compiling | Faster still, and a second trust anchor per tool: a release page rather than the module proxy and the checksum database. Rejected for the reason `promtool` is the one exception rather than the rule. |
| Vendor the binaries into the repository | Ends the question of who compiled them and begins a worse one: 470 MB of platform-specific binaries in git, updated by hand. |
| Leave it | 29 runner-minutes and seven minutes of waiting per pull request, to recompile nine tools that have not changed since the pin was last moved. |

## Consequences

**Positive**

* `make tools` runs when a pin moves, and not otherwise. Eight jobs lose 215 seconds each.
* `tools-ensure` is honest on a laptop too: a stamp that names the pins and the toolchain is a
  better answer to "is my `.tools` current" than the file dates a contributor used to guess from.
* Adding a tool now has one more place that must name it — `TOOLS_PINS` — and forgetting it makes
  the set look complete when it is not. The tools it lists are the ones the gates call through
  `require_tool`, which fails loudly rather than silently skipping, so the failure is a red gate
  naming the missing binary.

**Negative / countermeasures**

* *A cache is a place a gate's input can come from without being looked at.* → The stamp, the
  branch scoping above, and `release.yml` compiling its own set.
* *Any `Makefile` edit changes the key.* → The restore key catches it; `tools-ensure` decides. A
  Makefile change that does not move a pin costs one upload, not one install.
* *The entry is about 203 MB compressed.* → One entry per platform and Go version, against a 10 GB
  repository limit; the day-keyed Trivy caches are a bigger share of it today.
