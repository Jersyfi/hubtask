# ADR-0038 — The workbench is published at `workbench.hubtask.eu`

**Status:** accepted · **Date:** 2026-09-02

## Context

[ADR-0037](./ADR-0037-component-workbench.md) built the component workbench and said, in its own
words, that it "never reaches a user". That sentence was about three things and it still holds for
all three: the workbench is not part of `pnpm -r build`, nothing of it enters `apps/webapp`'s
bundle, and the binary that embeds that bundle (ADR-0028) is unchanged by it to the byte.

What the sentence also did, without meaning to, was leave the workbench reachable only by cloning
the repository and starting a dev server. That is a real cost. The workbench exists so that six
rules of `design-system.md` can be *checked by looking* — and a link into a state
(`?story=…&theme=dark&dir=rtl&text=long`) is the form that check takes in a pull request. A link
that only resolves on a machine with the repository checked out is not a link.

The reason to keep it unpublished was stated once in this session and was wrong: that a public
workbench would expose the wave progress of a product that has not been announced. **The
repository is public.** `docs/backlog/`, every ADR and `design-system.md` §4 — the very inventory
the workbench renders — are readable by anyone today. Publishing the workbench discloses nothing
that is not already disclosed, and it is the ordinary shape for a design system: Atlassian's lives
on a public URL for the same reason.

## Decision

**The workbench is published at `workbench.hubtask.eu`, from `main`, on every change to
`packages/design-system/`.**

**A subdomain, not a path under `hubtask.eu`.** The website is the product's public face and
[F1-12](../backlog/milestone-F1.md) deliberately keeps it to what is already true and already
checked. The workbench is a development artefact — unfinished components, wave status, an axis bar
— and it does not belong inside that. A subdomain separates the two without a second host: an
IONOS webspace serves a subdomain from its own directory in the same account.

**The same transport, the same credentials, one new variable.** `website.yml` already mirrors over
SFTP with `lftp` and a pinned host key, chosen there over a third-party deploy action because an
action is a supply-chain decision and because the sftp backup adapter refuses trust-on-first-use.
None of that reasoning is weaker here, so `workbench.yml` is the same shape. It reuses
`WEBSITE_SFTP_HOST`, `WEBSITE_SFTP_HOST_KEY` and the user and password secrets — it is the same
webspace account — and adds exactly one variable, `WORKBENCH_REMOTE_DIR`, for the directory the
subdomain serves.

GitHub Pages was the alternative. It would have kept the product domain untouched at the price of
enabling Pages in the repository settings and pinning three further actions, for a surface this
project would then maintain alongside the one it already has. The webspace is already there, is
already proven, and needs nothing new.

**Its own mirror, so the two cannot delete each other.** `website.yml` mirrors with `--delete`, so
that a renamed asset does not linger. Two surfaces publishing into overlapping directories with
that flag is a way to lose one of them, so the workbench mirrors into `WORKBENCH_REMOTE_DIR` and
nothing else, and the two workflows take separate concurrency groups.

**The page says what it is.** A public URL showing components that do not exist yet has to say so
in its own words rather than leave a reader to infer it. The workbench header names its stage —
`experimental`, ADR-0035's vocabulary — states that it is a development tool for building
`hubtask`, and links to the repository. That is not decoration: it is the same obligation ADR-0035
put on the application's maturity banner, applied to the surface that shows the parts.

**What does not change.** The workbench is still absent from `pnpm -r build`, from the embedded
bundle and from the binary. It gains no analytics, no cookie, no form and no request to a foreign
domain — the fonts are self-hosted for reasons ADR-0029 and ADR-0018 already gave, and they do not
weaken because the page is now reachable. Nothing on the page is a promise about the product.

## Consequences

* A pull request can link to a state of a component and the reader can open it. That is what the
  axis matrix was built for, and it did not work off a developer's own machine until now.
* The domain owner has a half of this that a workflow cannot do: the DNS record for
  `workbench.hubtask.eu` and the webspace mapping from that subdomain to a directory. Until both
  exist and `WORKBENCH_REMOTE_DIR` is set, the workflow builds, checks and then skips the publish
  with a message — the same politeness `website.yml` shows on a fork.
* A third surface now carries the design system's output: the tokens (`dist/`), the website, and
  this. All three come from `tokens.json`, so the number of surfaces is not a number of sources.
* Publishing an unfinished thing invites being read as finished. The stage line on the page is the
  mitigation, and it is the only one — this is a development tool on a public URL, which is what
  a source-available project's design system normally is.

## Notes

Related: [ADR-0037](./ADR-0037-component-workbench.md) (what the workbench is, and the clause this
qualifies — the product artefacts it names are all still untouched),
[ADR-0035](./ADR-0035-one-product-version.md) (the maturity vocabulary the page uses),
[ADR-0029](./ADR-0029-design-system-tokens.md) (why the fonts are self-hosted here too),
[ADR-0028](./ADR-0028-embedded-web-ui.md) (the bundle this still does not touch),
[ADR-0022](./ADR-0022-github-platform.md) (the platform the workflow runs on),
[`ci-cd.md`](../architecture/ci-cd.md) §8 (CI-4, the open point `website.yml` closed and this one
follows).
