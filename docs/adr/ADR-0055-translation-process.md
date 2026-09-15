# ADR-0055 — The translation process: pull requests in a layout Weblate reads, no instance yet

**Status:** accepted · **Date:** 2026-09-13

## Context

[i18n-l10n.md](../architecture/i18n-l10n.md) §3 has said since the first day that the translation
process is "Weblate or Crowdin against `locales/en.json`; community contributions by pull request",
and the `0.8.0` row of the roadmap promises "the Weblate connection". Nothing in the repository has
ever named an instance, a hosting plan or an account, because until `0.8.0` there was one catalogue
and nobody to translate it.

`0.8.0` builds the second catalogue and the gates around it, so the question is due. It is not an
implementation question: a translation platform holds accounts for translators, which is a
processor and a data-catalogue entry ([data-protection.md](../architecture/data-protection.md));
it pushes commits into the repository, which meets the CLA, `CI required` and the branch
protection; and it is a running service — a server, a database, upgrades — for as long as the
project exists. The owner delegated the decision on 2026-09-13 with one instruction: decide in
favour of the project, and if a self-hosted service is the right thing, take it on.

Three facts shape the answer:

* **Hubtask is BUSL-1.1** ([ADR-0013](ADR-0013-licensing.md)). Weblate's hosted service offers free
  "libre" hosting to projects under an OSI-approved licence; BUSL is not one, so a hosted instance
  would be a paid plan rather than a gift.
* **Weblate needs nothing from the repository that it does not already have.** Its "JSON file"
  format is one flat object per locale, keys to strings, which is exactly `locales/<tag>.json`; it
  reads and writes such files in a git checkout through its own commits. The "connection" the
  roadmap names is therefore not code — it is an instance pointed at a layout that exists.
* **There is one developer and, on the day of this decision, one language.** A service that
  serves translators before there are translators is a cost with no beneficiary.

## Decision

**The repository is made Weblate-ready, and no instance is run or bought until there is somebody
to serve.**

Ready means, concretely — and `0.8.0` builds each of these:

1. One catalogue per locale, `locales/<tag>.json`, flat, keys to ICU strings in the subset both
   renderers implement (M-01, M-02). The layout is what Weblate's "JSON file" format reads
   unchanged — one object, keys to strings, nested keys optional and unused here, the file mask
   `locales/*.json` with `locales/en.json` as the monolingual base — checked against Weblate's
   file-format documentation (docs.weblate.org, *Supported file formats → JSON files*) on
   2026-09-15 and recorded here rather than depended on. What Weblate would need beyond the layout
   is a commit identity and a branch, which are an instance's configuration and not the
   repository's.
2. A gate that holds every translation to the source — an unknown key fails, a missing key is
   reported, the placeholders must agree, every message must parse (M-03) — so that a contribution
   from any source, a person or a platform, is checked the same way.
3. A coverage report, `make locales`, so that the state of a translation is one command (M-03).
4. A section in `CONTRIBUTING.md` that says how a translation is contributed, that a partial file
   is welcome because the fallback is a feature, and that the CLA covers it (M-12).

**What would end the deferral:** a second person translating. The day a contributor offers a
language and asks for a tool rather than a text editor, the deferred half of this decision is a
self-hosted Weblate on the project's own infrastructure — chosen over the hosted plan because the
translators' accounts then live where the rest of the project's data does, and over Crowdin
because Weblate is itself free software that a self-hoster of Hubtask can run the same way. That
choice is recorded now so that it is not re-derived; what it costs and where it runs is the ADR
that supersedes this one.

## Options

**A. A self-hosted Weblate instance now.** Rejected for the reason above: a running service for
zero translators, and an operating commitment on the one person maintaining the project.

**B. A hosted plan at weblate.org.** Rejected: a paid subscription with the same zero
beneficiaries, and translator accounts held by a third party the data catalogue would have to name.

**C. Crowdin.** Rejected: proprietary, and it reads the same files Weblate does — nothing is gained
that the layout does not already give.

**D. Pull requests only, forever.** Not quite this decision: the layout, the gates and the report
are what a platform would need too, and the day one is wanted the switch is a service and not a
rewrite. This ADR keeps that day open rather than closing it.

## Consequences

* `i18n-l10n.md` §3's translation-process row changes from "Weblate or Crowdin" to "pull requests
  against `locales/`, in a layout Weblate reads unchanged; an instance is ADR-0055's deferred half".
* The roadmap's `0.8.0` row stops promising "the Weblate connection" and promises what is built.
* No data-catalogue entry is needed yet: no accounts, no processor.
* A contributor who wants to translate today needs a text editor, the gate and `make locales` —
  and nothing to sign up for.

## Notes

Related: [ADR-0011](ADR-0011-i18n-message-codes.md), [ADR-0013](ADR-0013-licensing.md),
[i18n-l10n.md](../architecture/i18n-l10n.md) §3, `docs/backlog/milestone-0.8.0.md` (M-01, M-03,
M-12). The owner's delegation is recorded in the backlog's decisions.
