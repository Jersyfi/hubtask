# ADR-0022: GitHub as the development platform, GitHub Actions as CI/CD

* **Status:** accepted
* **Date:** 2026-08-16
* **Concerns:** operations, process, supply chain
* **Supersedes:** constraint C-06 in its previous form (GitLab CI per the in-house template)
* **Related:** [ADR-0015](./ADR-0015-security-baseline.md), [ADR-0013](./ADR-0013-licensing.md), [versioning-release.md](../architecture/versioning-release.md)

## Context

The underlying in-house template ships an `example-gitlab-ci.yml` and assumes GitLab as the registry
and CI (constraint C-06). For this project, **GitHub** was chosen as the public home — fitting the
open orientation (C-10), the visibility for contributors, and the availability of ecosystem tooling.

The template's hexagonal structure (C-02) is unaffected: it concerns the source, not the pipeline.

## Decision

1. **GitHub** is the home of the source and the platform for issues and releases. The repository is public; the licence construct that had to be settled before publication is decided and in place (ADR-0013).
2. **GitHub Actions** replaces the GitLab pipeline. The gates from [versioning-release.md](../architecture/versioning-release.md) §6 are carried over one to one; the template file `example-gitlab-ci.yml` is not used and is not present in the repository.
3. **The GitHub Container Registry (`ghcr.io`)** is the default registry. The Helm chart value `image.repository` is configurable; a mirror on Docker Hub stays possible but is not a prerequisite.
4. **No dependency on GitHub specifics in the product.** The pipeline may use GitHub features; the application, the Helm chart, and the Compose files may not. Switching platform must stay possible without a code change — verified by the fact that `make verify` reproduces every PR gate locally in containers.
5. **Hardening of the pipeline** as part of the supply chain controls from ADR-0015:
   * Actions pinned by commit SHA, not by tag; permitted actions restricted through an allowlist.
   * `permissions:` minimal per workflow, defaulting to `contents: read`.
   * No `pull_request_target` workflows with access to secrets.
   * Publication only through GitHub environments with a protection rule and manual approval.
   * Image signing with `cosign` keyless through OIDC; provenance attestation enabled.
6. **AI assistance in the pipeline** is permitted, but exclusively **advisory**: it may comment, suggest, and produce drafts — it may not replace a gate, merge anything automatically, or publish anything. Details in [ci-cd.md](../architecture/ci-cd.md) §5.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: GitHub + Actions + GHCR** | The widest reach for an open project, good tooling support, free runners for public repositories. |
| Staying with GitLab (the template unchanged) | Consistent with the in-house template, but less visibility for external contributors; contradicts the project's direction. |
| GitHub as a mirror, GitLab as the truth | Double maintenance of two pipelines, and unclear responsibility for pull requests. Rejected. |
| Platform-neutral CI (Dagger, Earthly) | Avoids lock-in, but costs an extra abstraction layer and learning time. The chosen substitute — every gate runnable locally as a `make` target — achieves the same goal with less effort. |

## Consequences

**Positive**

* Contributions, issues, and security reports happen where the audience already is.
* GHCR, Dependabot, CodeQL, advisories, and OIDC signing without additional infrastructure.
* Public traceability of the gates strengthens the security commitments from ADR-0015.

**Negative / countermeasures**

* *Lock-in to one vendor.* → Every gate is a `make` target; `make verify` reproduces the PR pipeline locally. Switching platform touches only the workflow files.
* *Actions are executable third-party code in the supply chain.* → SHA pinning, an allowlist, minimal permissions, and no secrets in workflows from forks.
* *The in-house template is deviated from.* → The deviation is documented here; the template's folder and file conventions (C-02) remain valid unchanged.
* *AI in the pipeline can make hallucinations look authoritative.* → Advisory only, clearly marked as an AI comment, without write access to the code and without influence on the gate status.
