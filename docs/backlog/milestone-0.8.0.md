# Milestone 0.8.0 — Every language, proved on the server

The goal: [`i18n-l10n.md`](../architecture/i18n-l10n.md) stops being a document the product
agrees with in principle and becomes one it can be checked against, line by line. The requirement
that document opens with — *the application must support every language* — has had its ground
rule in place since `0.1.0`: the server emits codes and never sentences, and one catalogue,
`locales/en.json`, is read by both halves of the product. What the ground rule made possible,
nothing has yet used. There is one catalogue, so nothing renders a second language; the manifest
declares `supported_locales` and nothing fills it, so the language picker `F1` built offers an
empty list; `account.week_start` is stored, audited and never read, so `@start_of_week` is Monday
for everybody; the negotiation the document names is written twice by hand and neither copy knows
which catalogues exist; and the AI translation `0.7.0` moved here is still a row in a table.

So this milestone builds the server half of every line in §1–§7 that is not built, corrects the
lines that promise what the server cannot deliver, and proves the result the way `0.7.0` proved
QS-09: arc42 **QS-08** — *a new language is added; only translation resources plus enabling the
locale; no code change; the RTL flag in the manifest* — is walked at the end rather than assumed,
with the evidence filed. The acceptance of the last task names `i18n-l10n.md` by name, because
J-17's did not name `ai-first.md`, and `0.7.5` is what that cost.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: **every screen**, which is `F5`, one window behind
— language switching, the client's own second catalogue (`catalogue.ts` imports `en.json` and
nothing else, and the `import.meta.glob` that changes that is a client change), `Intl` formatting
of numbers, dates and units, the RTL audit against the design system's `start`/`end` rule, and the
whole of accessibility: WCAG 2.2 AA, keyboard operability, the screen-reader pass and the statement
the European Accessibility Act expects. This milestone *documents* what those owe (M-13); `F5`
builds it and `1.0.0` criterion 16 demonstrates it. **A Weblate instance**, for the reason
ADR-0055 gives: the repository is made Weblate-ready, and running the service is a decision for the
day there are translators to serve. **Detecting an entry's language from its text**, which §5
already declines — a client that knows its language says so. **Working hours and holidays**, which
§4 places after `1.0`. **Push and PDF**, the two §1 exceptions that name surfaces the product does
not have — the exception list is corrected, not extended. **Transliterating slugs**, where the
validation refuses what a transliterator would rewrite and the document is amended to say so. And
**a machine translation of 2,319 keys**: a catalogue that claims to be German and was never read by
a German speaker is worse than the fallback, and the fallback is a feature this milestone exists to
demonstrate.

Fourteen decisions taken while writing this backlog, so that nobody re-derives them:

* **The letter is M, and L is skipped on purpose.** J was `0.7.0`, K was `0.7.5`. `L-01 — … **[L]**`
  is the marker of every task in this file colliding with its own number in an issue title and a
  `Task:` trailer read for years. The alphabet already has two gaps for the same kind of reason —
  F belongs to the client track, I is unreadable beside `1` and `l` — and a third is cheaper than
  a ledger entry somebody misreads.
* **Weblate-ready, not Weblate-connected** ([ADR-0055](../adr/ADR-0055-translation-process.md)).
  The owner delegated the choice on 2026-09-13 with one instruction: decide for the project. A
  Weblate instance is a server, a database, upgrades and accounts for translators — an operating
  commitment and a data-catalogue entry — for a project with one developer and, today, one
  language; and Hubtask's BUSL-1.1 does not qualify for weblate.org's libre hosting, so the hosted
  alternative is a paid plan. What Weblate would *add* to the repository is nothing: it works
  against exactly the layout `locales/` already has, one flat JSON per locale. So the milestone
  makes the repository what an instance would need — the layout named, the gates that hold a
  translation to the source, the coverage report, the contributing section — and the instance is
  the ADR's deferred half, taken up when there is a second translator rather than before.
* **The `1.0` locale set is eleven, and `0.8.0` ships two of them.** The owner's ruling: English
  as the source, German as the second language, and the languages most used in the world
  selectable by `1.0`. By total speakers (Ethnologue 2025) the ten largest are English, Mandarin,
  Hindi, Spanish, Standard Arabic, French, Bengali, Portuguese, Russian and Indonesian; with German
  that is `en`, `zh-Hans`, `hi`, `es`, `ar`, `fr`, `bn`, `pt`, `ru`, `id`, `de`. *Selectable* means
  what §2 says it means — a catalogue file is present — so `0.8.0` ships `en.json` and `de.json`,
  and every other one of the eleven arrives as a file with no code change, which is precisely what
  QS-08 walks. The metadata table M-05 builds covers all eleven from the first day, so a file that
  arrives is fully described the moment it does.
* **German is partial, real, and says so.** `de.json` carries the families the *server* renders —
  `email.*`, `seed.*`, `errors.*`, `mail.*` and the codes `hubctl` shows a person most — translated
  by hand and reviewed by a German speaker, and nothing else. Every other code falls back to
  English, which is the behaviour §3 prescribes and the one M-01's acceptance proves rather than
  claims. A German workspace is provisioned with *Zu erledigen*, *In Arbeit* and *Erledigt* on its
  first day; its notifications arrive in German; and its interface stays English until `F5`,
  because that is where the client's second catalogue is.
* **Both renderers implement the same subset, and the plural contradiction goes away instead of
  being scoped around.** §3 requires CLDR plural categories for Arabic, Polish and Russian and, two
  paragraphs later, describes a Go renderer that refuses a plural — a contradiction invisible while
  the only catalogue is English, and one a translated `ar.json` would turn into `{count, plural,
  …}` printed into a subject line. The cheap fix would be a rule that the server-rendered families
  stay simple; it would also be a rule a translator has to know. So M-02 teaches
  `infrastructure/i18n` the client's subset — `plural` with `offset:` and `=n`, `selectordinal`,
  `select`, `#`, nesting, apostrophe quoting — with the categories from
  `golang.org/x/text/feature/plural`, the same CLDR data `Intl.PluralRules` carries. One subset,
  one gate on each side, and §3's paragraph rewritten to say so.
* **`golang.org/x/text` and `golang.org/x/net/idna` become direct dependencies, confirmed rather
  than chosen** ([ADR-0056](../adr/ADR-0056-golang-x-text-and-idna.md)). Both modules are already
  in the module graph as `// indirect` — `x/text v0.41.0`, `x/net v0.58.0` — so the SBOM does not
  grow and no new origin appears. §2 names `language.NewMatcher` and §5 names `unicode/norm`
  explicitly; what the ADR adds is the plural rules and `idna`, all from the same two modules, and
  the confinement: `core/` does not import them, the adapters do.
* **CLDR metadata is a table, not a dependency.** `x/text` exposes no week data and no number
  symbols publicly, and the CLDR XML it can parse is not something a server should carry. What the
  manifest needs per locale is three facts — direction, week start, decimal separator — for eleven
  locales. Direction comes from `language.Tag.Script()` and a list of the right-to-left scripts;
  the other two are rows written once and checked by a test against the values a browser's `Intl`
  answers, recorded in the test as the source. A client formats with `Intl` anyway (§7); the
  server publishes the facts a client cannot ask a browser before it has rendered anything.
* **NFC on the way in, and no backfill.** Unicode lets one visible character be written two ways —
  `é` as one code point, or `e` followed by a combining accent — and §5 says input is normalised
  to one of them before it is stored or compared. M-07 does that on every write from then on and
  leaves stored rows as they are: rewriting them is a rewrite of user content across every text
  column, which is on the list of things nobody decides alone. The consequence, named: a label
  written in the decomposed form before this milestone and the same label typed afterwards are two
  labels until the first one is next edited, at which point it is normalised like everything else.
  The owner has been shown this reasoning and M-07 does not start until it is confirmed.
* **Names sort the same way on every installation, through a collation object the migration
  defines once.** §5's `und-x-icu` is available wherever PostgreSQL was built with ICU — the
  official images, CloudNativePG's, and the managed services the support matrix admits (RDS ships
  ICU since PostgreSQL 10) — and it is a collation *object*, so using it needs no DDL an operator
  is refused. Managed databases stay supported unchanged, which the owner made a condition: the
  migration creates `hubtask_name` `FROM "und-x-icu"` where that collation exists and from the
  database's own where it does not, queries say `COLLATE hubtask_name` unconditionally, and
  `/meta/capabilities` reports which of the two it got. `order_key COLLATE "C"` is untouched: a
  rank key rests on byte order and no name ever sorts beside one. §5's sentence about the
  *database's* collation is rewritten to what a migration can reach.
* **The search remembers which configuration built each document, so the stale ones can be found.**
  ADR-0034 already says a mapping row comes with a migration to rebuild the documents that carry
  the tag; what it does not cover is an installation whose PostgreSQL *gains* a configuration — an
  upgrade, or an operator installing one — after which every entry indexed as `simple` stays
  `simple` until it happens to be edited. M-09 adds a nullable `search_configuration` the trigger
  fills beside the document, a workspace operation that rebuilds exactly the rows whose stored
  configuration differs from the current answer, and the count of them as the operation's report.
  Tenant-scoped and administrator-triggered, because nothing may enumerate tenants; no manifest
  count, because that is a table scan on every read of the manifest.
* **Translation is a read, not a record.** `ai-first.md` §2's row says *display only, not
  persisted*, and `0.7.0` decision 10 moved it here for the surface, not to change its shape. So
  M-11 is the second AI call somebody waits for, after the search's: bounded the same way,
  counted against the budget, refused without consent, and never stored — no `AiSuggestion`, no
  audit of the text, nothing to accept.
* **The frontend's requirements get no new document.** They exist three times already — roadmap
  phase 5, `data-protection.md` §7, `design-system.md` — and a fourth copy is how `0.7.5` came to
  be needed. M-13 makes `i18n-l10n.md` §6 the localisation list and `design-system.md` the
  accessibility list, and the roadmap points at both instead of restating them.
* **The `0.8.0` roadmap row is corrected, not reinterpreted.** It promises *CLDR formats* from a
  server that delivers no display text (§7: formatting is the client's), *language-dependent
  search* that `0.3.0` built, *localised emails* whose mechanism `0.3.0` built, and it omits the
  translation its own `0.7.0` row says moved here. M-14 rewrites the row to what this file builds.
* **QS-08 is walked for the server in this milestone and completed for the interface by `F5`.**
  The walk adds a language through `HUBTASK_LOCALE_DIR` — an operator's `ar.json` and nothing
  else — and shows the manifest's `rtl`, a German notification, a seeded German workspace and
  `hubctl` reading the new file; it says in its evidence which half the client still owes.

---

## M-01 — Every catalogue in `locales/`, and the directory laid over it **[L]**

*Depends on: nothing.*

`locales/Embed.go` embeds `en.json` by name and `infrastructure/i18n.NewRenderer` builds a map of
one. Both were written as "the shape that makes `de.json` a file rather than a change" — this task
is the day that arrives. `Embed.go` embeds `*.json`; the renderer loads every file, keyed by the
lower-cased tag the filename carries, refuses a filename that is not a well-formed tag or a file
that is not a flat string map, and answers `Locales()` — the tags it holds, `en` first and the rest
sorted — which M-04 and M-05 read.

§1's second half: *the catalogue is embedded and can additionally be overridden from a directory*.
`HUBTASK_LOCALE_DIR`, empty by default, names a directory read once at start. A file there for a
tag the binary carries overrides it key by key — a partial file falls back to the embedded one,
then to the source — and a file for a new tag adds the locale, which is the whole of "enabling the
locale" QS-08 speaks of. A directory that does not exist is a configuration error with a code; a
file that does not parse is one too, naming the file and not its content. Nothing is watched:
translations change with a restart, like every other piece of configuration.

The second catalogue ships here: `de.json`, hand-written, for the families the server renders —
`email.*`, `seed.*`, `errors.*`, `mail.*`, and the codes `hubctl` shows a person — and nothing
else, which is what makes the fallback demonstrable. Until M-02 lands, the German messages stay
within the simple-argument subset the renderer has today; the gate that says so is extended from
the source catalogue to every catalogue in this task, and M-02 widens what it allows.

**Acceptance:** every `locales/*.json` is embedded and loaded, and a file whose name is not a tag
or whose content is not a flat map fails the build; `HUBTASK_LOCALE_DIR` overrides key by key, adds
a locale from a new file, and refuses to start on a directory that is missing or a file that does
not parse — each with a `config.*` code; the fallback chain is proved by one test with a
half-translated locale — a translated key renders translated, a missing one renders the source,
never the key; two recipients in one workspace, one `de` and one `en`, receive the same
notification in two languages; a workspace provisioned with `default_locale: de` is seeded with
German structure; the configuration surface documents the variable; `make verify` is green.

**Read:** `i18n-l10n.md` §1–§3; `locales/Embed.go`; `infrastructure/i18n/Renderer.go`,
`Catalogue.go`, `Catalogue_test.go`; `core/port/i18n`;
`core/application/service/notification/DeliverNotification.go` (`send`);
`core/application/service/admin/Provision.go` (`seedStructure`); `core/port/environment/Port.go`
(`LocaleConfig`); ADR-0011

---

## M-02 — The Go renderer learns the client's subset **[L]**

*Depends on: M-01.*

`infrastructure/i18n.substitute` implements simple arguments and `Catalogue_test.go` refuses
anything else, which was honest while every message was English. It is not a shape a translated
catalogue can live in: German needs a plural where English phrased around one, Arabic needs six
categories, and a message the renderer cannot parse is printed at the recipient with its braces
on. So the renderer implements what `apps/webapp/src/lib/i18n/format.ts` implements — simple
arguments, `plural` with `offset:` and `=n`, `selectordinal`, `select`, `#`, nested messages in
every branch, apostrophe quoting — and refuses by name everything `format.ts` refuses: `number`,
`date`, `time` and every other argument type. The categories come from
`golang.org/x/text/feature/plural`, cardinal and ordinal, which is the one part that cannot be got
right by hand; the port's `map[string]string` stays, and a plural argument parses its operand from
the string, refusing one that is not a number.

This task opens with [ADR-0056](../adr/ADR-0056-golang-x-text-and-idna.md): `golang.org/x/text`
promoted from indirect to direct, its three packages named — `language`, `unicode/norm`,
`feature/plural` — and `golang.org/x/net/idna` beside it for M-10, both confined to adapters. The
draft is written before the first import and the owner's answer is recorded in it.

**Acceptance:** the ADR is accepted and `go.mod` lists both modules as direct with no new module in
the graph; a table test mirrors `format.test.ts` case for case — the same patterns, the same
parameters, the same expected sentences — so that the two renderers cannot drift apart unnoticed;
a message using a construct outside the subset is refused with the construct's name; the
architecture test that keeps `core/` free of third-party imports stays green; `i18n-l10n.md` §3's
"two renderers, one catalogue" paragraph is rewritten to one subset with one gate each; `make
verify` is green.

**Read:** `apps/webapp/src/lib/i18n/format.ts` and `format.test.ts` (the subset, and its tests);
`infrastructure/i18n/Catalogue.go` (`substitute`); `i18n-l10n.md` §3; `go doc
golang.org/x/text/feature/plural`; ADR-0036 and ADR-0042 (the shape of a dependency ADR here)

---

## M-03 — What a second catalogue is held to **[L]**

*Depends on: M-01, M-02.*

§3's CI row — *missing keys = warning, unknown keys = error; placeholder consistency is checked* —
has had nothing to check. Now it has, and the gate lives where the catalogue's other gates live,
in `test/architecture`: for every catalogue that is not the source, a key the source does not have
fails (a translation of a message that was renamed or removed is a translation of nothing), a key
the source has and the translation lacks is reported by count and by family without failing (a
half-translated file is the normal state of a translation), and the argument names a translation
uses are exactly the ones its source uses — fewer is a lost parameter, more is a placeholder
nobody fills. Every message in every catalogue parses under M-02's subset, which is the same line
`catalogue.test.ts` draws for the client.

The report is also a target: `make locales` prints, per catalogue, how many of the source's keys
it carries and which families are complete, so that the state of a translation is one command
rather than a diff. It is what a contributor runs before a pull request and what ADR-0055's
process points at.

**Acceptance:** an unknown key, a placeholder mismatch and an unparsable message each fail the
gate, proved by `gate-selftest` planting one of each; a missing key is reported and does not fail;
`make locales` prints the coverage of `de.json` and the number matches the test's; `i18n-l10n.md`
§3's CI row names the test; `make verify` is green.

**Read:** `test/architecture/messagecodes_test.go` (the catalogue's existing gates, and the shape
to extend); `apps/webapp/src/lib/i18n/catalogue.test.ts`; `Makefile` (`gate-selftest`);
`i18n-l10n.md` §3

---

## M-04 — One matcher, over the catalogues that exist **[L]**

*Depends on: M-01, M-02 (the ADR).*

`presentation/rest/Request.go` parses `Accept-Language` by hand and picks the highest-weighted tag
without asking whether anything can render it; `infrastructure/i18n/Renderer.go` walks a tag down
its hyphens by hand and says in its own comment that `language.NewMatcher` is *for the day there
is more than one catalogue*. That day is M-01. Both places use one matcher, built over
`Renderer.Locales()` with `en` as its default, so that `de-AT` resolves to `de` when `de` exists
and to `en` when it does not, `pt-BR` prefers `pt-BR` over `pt` and takes `pt` otherwise, and a
request listing `fr, de;q=0.8` on an installation with German and no French gets German — which is
what the header meant and what the hand-rolled parser could not know.

The resolved tag is what the actor carries and what the manifest's `supported_locales` are
matched against, so the client and the server agree on which of the eleven a request landed on. A
tag the matcher cannot place at all resolves to the installation default, as before: a wrong
language is a nuisance, a refused request is an outage.

**Acceptance:** one matcher, constructed once from the catalogues present, used by the REST
middleware and the renderer; the cases above are a table test, including the ones the hand-rolled
chain got wrong; `Accept-Language` longer than the bound or malformed still falls back rather than
failing; the account's and the tenant's locale pass through the same matcher; `i18n-l10n.md` §2's
negotiation row is true as written; `make verify` is green and the contract tests pass.

**Read:** `presentation/rest/Request.go` (`Localised`, `preferredLocale`), `Auth.go` (where the
account's locale replaces the request's); `infrastructure/i18n/Renderer.go` (`catalogue`);
`i18n-l10n.md` §2; `go doc golang.org/x/text/language.NewMatcher`

---

## M-05 — `supported_locales`, answered **[L]**

*Depends on: M-01, M-04.*

The manifest has declared `supported_locales` — `locale`, `direction`, `week_start` — since `0.1.0`
and `MetaController.go` has never set it. The client reads it in three places and gets nothing:
`preferences.ts` builds the account's language picker from it, `InstallationView.svelte` lists it,
`locale.ts` asks it for the writing direction. This is the one task in the milestone a built
surface is waiting on.

Specification first: `decimal_separator` is added to the entry, additive, because §6 names it and
the schema does not; `week_start` is declared as the enum the account already uses (`MONDAY`,
`SUNDAY`, `SATURDAY`) rather than a free string, because a client compares it against the account's
value. Then the answer: one entry per locale in `Renderer.Locales()`, in that order, with the
metadata from a table in `infrastructure/i18n` that covers the eleven of the `1.0` set — direction
from `language.Tag.Script()` against the right-to-left scripts, week start and decimal separator
as rows — and a test that checks the rows against the values `Intl` gives for the same locales,
recorded as the source in the test. A locale the table does not know is answered honestly:
`ltr`, `MONDAY`, `.`, and a log line at start saying the table lacks a row, so that a twelfth
language is a row rather than a surprise.

**Acceptance:** `openapi.yaml` gains `decimal_separator` and types `week_start`, `make generate`
produces no diff, `make api-client` is regenerated in the same pull request and the client
type-checks; `/meta/capabilities` answers one entry per catalogue present, `ar` reads `rtl` and
`SATURDAY` when its file is laid over through `HUBTASK_LOCALE_DIR`; the eleven locales have rows
and the test that checks them against `Intl` passes; the picker in the account preferences shows
`en` and `de` on a default installation — a screenshot in the pull request; `i18n-l10n.md` §2 says
which locales `1.0` selects and §6's example is the schema's shape; `make verify` is green and the
contract tests pass.

**Read:** `api/openapi.yaml` (`Capabilities.supported_locales`, `Account.week_start`);
`core/application/service/meta/GetCapabilities.go`; `presentation/rest/MetaController.go`;
`apps/webapp/src/lib/data/preferences.ts` (`localesOf`), `src/lib/i18n/locale.ts`;
`i18n-l10n.md` §2, §6; `api-guidelines.md` §1

---

## M-06 — The week a person starts on **[L]**

*Depends on: M-05.*

`core/domain/model/view/Placeholder.go` says it in a comment: *the capability manifest has a
`week_start` per locale and nothing answers it yet; when it does, this is the one line that reads
it.* `@start_of_week` and `@end_of_week` are Monday for everybody, `account.week_start` is written
by `UpdateAccountPreferences`, audited, echoed by `GET /accounts/me` — and dropped before it reaches
`ActorContext`, so no server-side computation has ever seen it.

§4's rule is *from CLDR per locale, overridable per account*: the account's value where one is
set, otherwise the locale's row from M-05's table, otherwise Monday. `WeekStart` travels on the
actor beside `Locale` and `TimeZone`, the credential reads carry it as they carry the other two,
and `Resolution` takes it. The same task settles the calendar week: ISO 8601 by default, and the
CLDR variant where the locale's week starts on a Sunday (`en-US`) — one function, one table test
across the year boundary, and the saved-view placeholders resolve through it.

**Acceptance:** an account with `week_start: SUNDAY` asking for `@start_of_week` on a Wednesday
gets the Sunday before, an account with none on a `de` locale gets Monday, on `ar` gets Saturday;
the view export and the MCP prompts that use the week anchors agree with the query; the week
number follows ISO or CLDR by locale with a test across a year boundary in each; the placeholder's
comment is gone; `make verify` is green.

**Read:** `core/domain/model/view/Placeholder.go` (`weekStart`, `daysSinceWeekStart`);
`core/application/service/work/QueryItems.go` (`resolvePlaceholders`);
`core/application/shared/ActorContext.go`; `core/application/repository/identity/Session.go`
(what a credential read carries); `i18n-l10n.md` §4; `api-guidelines.md` §3 (placeholders)

---

## M-07 — NFC, once, on the way in **[L]**

*Depends on: M-02 (the ADR). Does not start before the owner confirms the no-backfill decision.*

§5: *input is NFC-normalised before being stored or compared.* Nothing normalises anything. Two
spellings of `Café` — one code point, or `e` and a combining acute — are two labels to the unique
index, two search tokens to the document, and one word to every person reading them.

The normalisation happens in one place per text kind rather than at every call site: the domain
constructors that already trim and bound a title, a name, a body, a display name, a comment run
the input through `norm.NFC` first, and a test per constructor proves the decomposed form is stored
composed. The domain may not import `x/text` (rule 1), so the form is applied behind a port —
`core/port/text.Normalizer`, one method, with the `x/text` adapter in `infrastructure/text` and a
fake in the tests — wired where the constructors are called. Nothing stored is rewritten; the
decision above says why, and the consequence is written into §5 beside the rule.

**Acceptance:** every constructor that stores user text normalises it, enumerated in the pull
request against the data catalogue's list of text fields; a decomposed title is stored composed
and found by a composed search; the query language's string comparisons normalise the value they
compare against; no backfill, and `i18n-l10n.md` §5 says what that means for rows written before
`0.8.0`; `core/` imports nothing new; `make verify` is green.

**Read:** `i18n-l10n.md` §5; `docs/privacy/data-catalog.md` (the text fields);
`core/domain/model/work/Container.go`, `Structure.go`, `Comment.go`, `Template.go`,
`core/domain/model/identity/Invitation.go` (the constructors); `core/port/i18n` (the shape of a
one-method port); ADR-0001

---

## M-08 — Names sorted the same way on every installation **[L]**

*Depends on: nothing.*

`ORDER BY name` in `Structure.sql`, `Identity.sql` and `Backup.sql` sorts in whatever collation the
database was created with — `en_US.utf8` on the development stack, `C` on many containers, a
provider's choice on a managed service — so *Ärger* sorts after *Zebra* on one installation and
between *Apfel* and *Zebra* on the next. §5 asks for `und-x-icu`, the ICU root collation: the same order on every
installation, language-independent, and present wherever PostgreSQL was built with ICU, which
includes every image the support matrix names and the managed services ADR-0052 admits.

Migration `0080` creates the collation object `hubtask_name`: `FROM "und-x-icu"` where
`pg_collation` has it, from the database's default otherwise, inside a `DO` block that reads the
catalogue — no superuser, no extension, nothing a managed service refuses. Every name-ordered read
then says `COLLATE hubtask_name`, which is constant SQL text for sqlc and rule 9. No index is built
on it: the lists it orders are a collection's buckets and labels and a tenant's containers, and a
collation with an index is a collation whose ICU version has to be tracked across upgrades. The
manifest's `features` gains `natural_ordering`, answered from `pg_collation.collprovider` — a
client that sorts a list itself with `Intl.Collator` can see whether the server already did.
`order_key COLLATE "C"` is untouched, in the queries and in the indexes: a rank key is a
fractional index and rests on byte order.

**Acceptance:** the migration is expand-only, runs on a database with and without ICU (Testcontainers
with the Alpine image for the second), and both paths are tested; every `ORDER BY` over a name
carries the collation and a test lists the ones that do not; the order of `Ärger`, `Apfel`,
`Zebra` is the same on both paths where ICU is present and documented where it is not; the
feature flag is answered and named in `api-guidelines.md` §1's list; `db/schema.sql` mirrors the
object; `i18n-l10n.md` §5 says the collation is the query's, not the database's, and the support
matrix says what the absence of ICU changes; `make verify` is green.

**Read:** `i18n-l10n.md` §5; `db/migrations/0019_language_search.sql` (a migration that asks the
catalogue); `db/queries/Structure.sql`, `Identity.sql`, `Backup.sql`; `docs/architecture/
support-matrix.md`; ADR-0052; the `order_key` reasoning in migration `0007`

---

## M-09 — The search configuration: what it covers, and the entries indexed before it did **[L]**

*Depends on: nothing.*

ADR-0034 built the language-dependent document in `0.3.0` and left one sentence for later: *adding
a language is a row in the resolver's mapping and a migration to rebuild the documents of the items
that carry the tag.* It says nothing about the other way a configuration arrives — an installation
upgrades its PostgreSQL, or an operator installs one — after which every entry that was indexed as
`simple` for lack of it stays `simple` until it happens to be edited. A search that answers a
shorter list than the truth, permanently, for the entries written first.

Two changes, both small. Migration `0081` adds `work_item.search_configuration text`, nullable, no
default, filled by the existing trigger beside the document with the configuration it used — so a
row knows what built it, and `NULL` means "before this milestone", which is treated as stale. And
a use case, `ReindexSearch`, tenant-scoped and administrator-only: it counts the rows whose stored
configuration differs from `hubtask_text_config(content_language)` now, rewrites exactly those in
batches through the worker as a job, and reports the count before and after. `POST
/api/v1/workspace/search:reindex` starts it, the job status carries the numbers, and `hubctl`
gets the verb. The mapping itself is reviewed against the eleven of the `1.0` set: `hi`, `id`,
`ar`, `ru`, `pt`, `es`, `fr`, `de`, `en` have stock configurations and are rows; `zh-Hans` and `bn`
have none in any PostgreSQL the matrix names and are `simple` plus the trigram index, which §5
says is the search for a script without word boundaries — and the manifest's `text_languages`
already tells a client which is which.

**Acceptance:** the column is filled by the trigger on insert and on the three-column update, and
old rows stay `NULL`; the operation counts and rewrites only rows whose configuration changed, in
batches, as a job with the numbers in its status; a workspace member without the administrator
role is refused; cross-tenant negative test; use case registered, MCP and automation parity, a
metric and a span, an audit action; `hubctl search reindex`; `db/schema.sql` mirrors the column;
ADR-0034's consequences name the operation; `i18n-l10n.md` §5 says what an installation does after
gaining a configuration; `make verify` is green and the contract tests pass.

**Read:** ADR-0034; `db/migrations/0019_language_search.sql` (the trigger and the batched
backfill); `core/application/service/meta/GetCapabilities.go` (`TextLanguages`); a tenant-scoped
job such as `RunRetention` for the batch shape; `core/application/service/work/Embedding.go` (the
pass that walks a tenant's entries, J-10); `i18n-l10n.md` §5

---

## M-10 — An address with a Unicode domain **[G]**

*Depends on: M-02 (the ADR).*

§7: *IDN-capable validation (Punycode normalisation).* `identity.emailAddress` lower-cases the
whole address and checks its shape; `anna@müller.de` is accepted as written and stored as written,
and `anna@xn--mller-kva.de` — the same mailbox, as a mail server sees it — is a second account.
The domain half is passed through `idna.Lookup.ToASCII` before the address is compared or stored,
so the two spellings are one row; a domain the profile refuses is a malformed address with the
code the constructor already has. The local part is not touched: it is the mailbox's business, and
case-folding it is already more than RFC 5321 allows.

Like M-07, the domain may not import the library, so the same port shape carries it: one method on
a port in `core/port/text`, the adapter beside the normaliser.

**Acceptance:** the two spellings of the address above resolve to one account, on invitation and on
sign-in; a domain with a character the profile refuses is refused with `accounts.email_malformed`;
the ASCII form is what the mail adapter sends to; a table test over the IDNA test vectors that
matter (mixed scripts, trailing dot, an already-encoded label); no backfill, and the data catalogue
notes the stored form; `make verify` is green.

**Read:** `core/domain/model/identity/Invitation.go` (`emailAddress`); `go doc
golang.org/x/net/idna`; `i18n-l10n.md` §7; `docs/privacy/data-catalog.md` (the email field)

---

## M-11 — Translation on request, display only **[L]**

*Depends on: nothing (the AI port and the provider resolver from `0.7.0`).*

`ai-first.md` §2's last row, moved here by `0.7.0` decision 10. A person reading an entry in a
language they do not have asks for it in theirs: `POST /api/v1/items/{id}:translate` with a
`target_locale` — defaulting to the caller's — answers the title and the notes translated, marked
as AI output with the provider's provenance, and stores nothing. It is a read, in the sense the
semantic search is one: the same consent check, the same budget row, the same breaker, a bounded
timeout, and a refusal with `ai.unavailable` for every way of not getting an answer.

The prompt is a file in the store like the others, `translate.v1.md`, with the content fenced as
content and the target language as the only instruction; the injection test gains the case where
the notes contain an instruction to translate something else. `content_language` is what the
prompt is told the source is, and an entry without one is still translated — the model can read.
The audit records that a translation was asked for an entry, never its text (rule 10).

**Acceptance:** the use case is registered and reachable by REST and MCP with parity; a caller who
may not read the entry is refused before any provider is called; without consent, without a
provider, over budget or past the timeout the answer is `503 ai.unavailable` and the entry is
unchanged; nothing is written to `ai_suggestion` or anywhere else, proved by a test that counts;
the response carries `source: AI`, model and prompt version like every other AI output; the
injection test covers the notes; cross-tenant negative test; a metric and a span; `ai-first.md` §2's
row says it shipped and where; `make verify` is green and the contract tests pass.

**Read:** `ai-first.md` §2 (the row and the guardrails); `core/application/service/work/
Embedding.go` and the search's bounded call (J-10); `core/application/service/suggestion/
Producing.go` (the prompt store, the fence, the injection test); ADR-0049; `i18n-l10n.md` §7

---

## M-12 — Weblate-ready: the process written down **[G]**

*Depends on: M-03.*

The half of [ADR-0055](../adr/ADR-0055-translation-process.md) that ships now. `CONTRIBUTING.md`
gains a section on translating: where the files are, that a file is a flat map of the source's
keys in the source's ICU subset, that `make locales` says how complete it is and the gate says what
is wrong, that a partial file is welcome because the fallback is a feature, that the CLA covers a
translation like any other contribution, and what a pull request adding a locale needs — the file,
a row in M-05's table if the locale is not among the eleven, nothing else. `i18n-l10n.md` §3's
translation-process row says *pull requests against `locales/`, in a layout Weblate reads
unchanged; an instance is ADR-0055's deferred half* rather than *Weblate or Crowdin*. The layout
is checked against Weblate's documented JSON format once, and the check is a sentence in the ADR
with the version it was read at.

**Acceptance:** the section exists and is linked from the README's contributing pointer; §3's row
is rewritten; the ADR is accepted with the delegation recorded and the deferred half named with
what would trigger it; `make gate-docs` is green.

**Read:** ADR-0055; `CONTRIBUTING.md`; `i18n-l10n.md` §3; Weblate's file-format documentation for
JSON (read, not linked as a dependency)

---

## M-13 — What the frontend owes: localisation and accessibility, written down **[L]**

*Depends on: M-05, M-06.*

The `0.8.0` roadmap row's last clause, and the one thing in it the server cannot build: *the
accessibility and localisation requirements for the frontend documented*. They exist today in three
places that do not agree on their length — roadmap phase 5's bullet list, `data-protection.md` §7's
two rows, `design-system.md`'s rules 4 and 5 and its §3 alignment sentence — and `F5` will be cut
from whichever one its author reads.

`i18n-l10n.md` §6 becomes the localisation list: read `supported_locales` and speak the negotiated
locale; render every code through the catalogue with the fallback, never a key; `Intl` for numbers,
dates, units and collation, the account's zone for moments and the entry's for due dates; `dir`
from the manifest's direction, `start`/`end` and logical properties throughout, the workbench's
direction axis on every component; the +40 % rule; grapheme clusters for truncation; the week from
M-06's answer. `design-system.md` gains an accessibility section: WCAG 2.2 AA as the bar, the
success criteria the product commits to by number, keyboard operability and `focus-visible` as
rule 5 already says, reduced motion as rule 6 says, the 24 px target floor density already
enforces, contrast measured in CI, the screen-reader pass as a walk with evidence, and the
accessibility statement's contents and where it is published — the website at convergence, per
roadmap phase 5. The roadmap's bullet list and `data-protection.md` §7 then point at the two
sections instead of restating them, and `F5`'s contents cell names both.

**Acceptance:** the two sections exist and every requirement the roadmap or `data-protection.md`
stated is in one of them or explicitly retired with a reason; the roadmap and `data-protection.md`
link rather than restate; `F5`'s row in the roadmap names the two sections; `make gate-docs` is
green.

**Read:** `roadmap.md` phase 5 (the binding requirements, F5's row); `data-protection.md` §7;
`design-system.md` §3, §6, §9; `i18n-l10n.md` §6; WCAG 2.2 (the AA criteria, read from the
source); ADR-0037 (the axes the workbench already renders)

---

## M-14 — QS-08 walked, and the documents that report it **[L]**

*Depends on: everything above.*

arc42 QS-08: *a new language (Arabic, for example) is added — only translation resources plus
enabling the locale; no code change; the RTL flag in the manifest.* The walk, scripted like
`0.7.0`'s QS-09 walk and filed as `docs/evidence/QS-08-<date>.md`: an installation started with
`en` and `de` embedded; an operator writes an `ar.json` with a handful of `email.*` and `seed.*`
keys into `HUBTASK_LOCALE_DIR` and restarts; `/meta/capabilities` lists `ar` with `rtl` and
`SATURDAY`; a workspace provisioned with `default_locale: ar` is seeded right to left; an account
set to `ar` receives its notification in Arabic and an account set to `de` in German, from one
event; `Accept-Language: ar-EG` resolves to `ar`; `@start_of_week` for the Arabic account is a
Saturday; `hubctl` reads the new file. And the evidence says, in one paragraph, what the client
still owes — the second catalogue in `catalogue.ts`, the switch, the audit — and that `F5` owes it.

Then the documents, and this is the list J-17 lacked: **`i18n-l10n.md` itself**, every section —
§1's exception list without push and PDF, §2 with the eleven and the matcher, §3 with one subset
and the gate, §4 with the week, §5 with the collation as the query's and NFC's consequence, §6 as
M-13 left it, §7 with the slug sentence amended to what the validation does and the translation
row pointing at M-11; `arc42.md` QS-08 with the evidence linked; `roadmap.md`'s `0.8.0` row
rewritten to what this file built — the catalogues and the directory, one subset and its gates,
the matcher, the manifest's metadata, the week, NFC, the collation, the reindex, IDN, the
translation, the process, the frontend's two lists — and the `0.7.0` row's *moved to 0.8.0* now
pointing at M-11; `ai-first.md` §2's translation row; the phase 3 paragraph under the table in the
shape the `0.7.0` and `0.7.5` paragraphs have.

**Acceptance:** the evidence file exists with the transcript and the paragraph on what remains;
QS-08 in `arc42.md` links it; every section of `i18n-l10n.md` describes the product as built,
checked line by line in the pull request body; the roadmap rows are corrected; `make verify` and
`make gate-docs` are green.

**Read:** `docs/evidence/QS-09-2026-09-09.md` (the shape of a walk); `deploy/integration/
hubctl-e2e.sh` (the scripted session, and the rate-limit note in its tail); `arc42.md` §10.2;
`roadmap.md` (the `0.7.0`, `0.7.5`, `0.8.0` rows and the phase 3 paragraph); `i18n-l10n.md`, all
of it

---

## The order at a glance

```
M-01 (the second catalogue; everything else is invisible without it)
 ├── M-02 (the subset, and ADR-0056)
 │    ├── M-03 (the gates)  ── M-12 (the process, ADR-0055)
 │    ├── M-04 (the matcher) ── M-05 (the manifest; the picker fills) ── M-06 (the week) ── M-13
 │    ├── M-07 (NFC — after the owner confirms)
 │    └── M-10 (IDN)
M-08 (the collation)              — independent
M-09 (the search's stale rows)    — independent
M-11 (translation)                — independent
M-14 (QS-08, and the documents)   — last
```

M-01 first because nothing in the milestone can be seen without a second language to see it in.
M-05 as early as its dependencies allow, because it is the only task a built surface is waiting
on. M-08, M-09 and M-11 fit between the others wherever the chain is blocked. M-14 last, and its
acceptance names the document this milestone is measured against.

**Definition of Done for the milestone:** a second catalogue renders — email, seeded structure,
`hubctl` — and a third arrives through a directory with no code change, walked as QS-08 with the
evidence filed; both renderers implement one ICU subset and each side has a gate that proves it; a
translation is held to its source by a gate and reported by a target; negotiation is one matcher
over the catalogues that exist; `/meta/capabilities` answers `supported_locales` with direction,
week start and decimal separator, and the language picker `F1` built has something to offer; the
week starts where the account or the locale says; text is normalised on the way in, names sort the
same on every installation, an address with a Unicode domain is one address, and a search that
was indexed before its configuration existed can be brought current; an entry can be read in
another language without anything being stored; the translation process is written where a
contributor looks; the frontend's localisation and accessibility requirements are in two sections
`F5` can be cut from; and `i18n-l10n.md`, every section of it, describes the product that exists.
