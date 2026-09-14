# ADR-0056 — `golang.org/x/text` and `golang.org/x/net/idna` as direct dependencies, confined to adapters

**Status:** accepted · **Date:** 2026-09-13

## Context

[i18n-l10n.md](../architecture/i18n-l10n.md) names two packages by their import path: §2
"negotiation through `golang.org/x/text/language` (`language.NewMatcher`)" and §5 "input is
NFC-normalised (`golang.org/x/text/unicode/norm`)". Neither has been imported. The negotiation is
written by hand twice — `presentation/rest/Request.go` parses `Accept-Language` and
`infrastructure/i18n/Renderer.go` walks a tag down its hyphens — and nothing normalises anything.
Both were honest choices while one catalogue existed: a matcher over a set of one answers the one.

`0.8.0` builds the second catalogue, and with it four needs that the standard library does not
meet:

| Need | Package | Named by |
|---|---|---|
| Match a preferred tag against the catalogues present, down the fallback chain | `golang.org/x/text/language` | §2 |
| Normalise input to NFC before it is stored or compared | `golang.org/x/text/unicode/norm` | §5 |
| CLDR plural categories, cardinal and ordinal, for the ICU subset the Go renderer learns | `golang.org/x/text/feature/plural` | §3 (plural rules "required for Arabic, Polish, Russian") |
| Punycode normalisation of an email address's domain | `golang.org/x/net/idna` | §7 |

A dependency is a supply chain decision (CLAUDE.md, "what you do not decide yourself"). This one
is unusual in that **both modules are already in the module graph**: `golang.org/x/text v0.41.0`
and `golang.org/x/net v0.58.0` appear in `go.mod` as `// indirect`, pulled in by the OpenTelemetry
exporters and `go-oidc`. Promoting them changes which packages of them the binary links and which
line of `go.mod` they sit on; it adds no module, no origin and no entry to the SBOM that is not
there.

Verified before writing this: `go doc golang.org/x/text/language.NewMatcher`,
`go doc golang.org/x/text/unicode/norm.Form`, `go doc golang.org/x/text/feature/plural.Rules.MatchPlural`
and `go doc golang.org/x/net/idna.Lookup` all resolve against the versions in the module cache.

What `x/text` does **not** offer, checked the same way, is CLDR week data or number symbols as a
public API — `unicode/cldr` is a parser for the CLDR XML archive, not a data set. The manifest's
`week_start` and `decimal_separator` are therefore a table in `infrastructure/i18n` covering the
`1.0` locale set, and not a reason for a further dependency (`milestone-0.8.0.md`, decision 8).

## Decision

**`golang.org/x/text` and `golang.org/x/net` become direct dependencies at the versions already
resolved, and exactly four packages of them are imported, each by one adapter:**

| Package | Imported by | For |
|---|---|---|
| `x/text/language` | `infrastructure/i18n` | the matcher (M-04); the REST middleware calls the adapter |
| `x/text/feature/plural` | `infrastructure/i18n` | the renderer's plural categories (M-02) |
| `x/text/unicode/norm` | `infrastructure/text` | the `Normalizer` port's adapter (M-07) |
| `x/net/idna` | `infrastructure/text` | the domain half of an email address (M-10) |

`core/` imports none of them — rule 1 — which is why M-07 and M-10 introduce a one-method port in
`core/port/text` rather than a call. The confinement is proved by `gate-architecture` the way
`cel-go`'s and `nats.go`'s are: a test that lists the packages allowed to import each of the four
and fails on a fifth.

Both modules are Google's, BSD-3-Clause, maintained alongside the Go toolchain and already covered
by `govulncheck` in `gate-security` through their indirect presence. Dependabot's
`patch-and-minor` group already bumps them.

## Options

**A. Keep the hand-rolled negotiation and write NFC and IDNA by hand.** Rejected. A matcher over
several catalogues is the part of BCP 47 nobody gets right by hand — script and region inference,
`zh-Hant` versus `zh-Hans`, `no` versus `nb` — and NFC over the whole of Unicode is a table this
project should not maintain. §2 and §5 chose the package for that reason.

**B. Vendor the pieces.** Rejected: the module is already resolved and audited; a copy would be a
second version of the same tables to keep current.

**C. A full ICU binding (`cgo`).** Rejected: `cgo` in a static container image, and a dependency on
the system's ICU version, for four functions.

## Consequences

* `go.mod` gains two lines in the direct block and loses two in the indirect one. `go mod tidy`
  produces no other change.
* The Go renderer can implement the client's ICU subset with the same CLDR categories
  `Intl.PluralRules` carries, which is what makes one subset on both sides possible (M-02).
* The negotiation becomes one matcher over `Renderer.Locales()` (M-04), and §2's sentence is true
  as written.
* A test in `test/architecture` enforces the four-package, two-adapter confinement.

## Notes

Related: [ADR-0036](ADR-0036-oidc-token-verification.md) and [ADR-0042](ADR-0042-nats-client.md)
(the shape of a confined dependency), [ADR-0001](ADR-0001-hexagonal-architecture.md),
[i18n-l10n.md](../architecture/i18n-l10n.md) §2, §3, §5, §7, `docs/backlog/milestone-0.8.0.md`
(M-02, M-04, M-07, M-10). The owner asked on 2026-09-13 that the three claims behind this ADR be
verified before it was written; the `go doc` checks above are that verification.
