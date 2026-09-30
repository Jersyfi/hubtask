# ADR-0072 — AI at the installation level: an offered model, and who may bring their own

**Status:** accepted · **Date:** 2026-09-30 · **Accepted:** 2026-09-30

## Context

An AI provider exists **per workspace** today ([ADR-0049](./ADR-0049-ai-provider-surface.md)): a
workspace configures kind, address, models and a sealed key, declares where the provider processes
content (`jurisdiction`), and consents separately (`processing_allowed`). The budget is a quota row,
`ai_tokens_per_day` (J-15). That is exactly what a company bringing its own model needs, and it is
the whole story for a private installation.

It is not enough for a provider running Hubtask for others — the owner's words from the concept
review on 2026-09-30: a provider must be able to **offer its own AI model directly**, **lock the
options for every workspace**, or **offer its model and leave workspaces free to use their own**.
The sign-in concept's §6.3 sketched this under plans ("PROVIDED, OWN, BOTH, NONE") and deferred it
with them. The use cases say why that is too late: a B2C customer (`D5`) who has to paste an API key
into a settings screen will not use AI at all, and a B2B customer (`D6`) whose data-protection
officer requires EEA processing needs the provider to be able to *hold* a minimum rather than hope
for one. See [UC-AI-05](../usecases/suggestion/UC-AI-05-use-the-model-our-provider-offers.md) and
[UC-AI-06](../usecases/suggestion/UC-AI-06-decide-which-ai-workspaces-may-use.md).

Three constraints frame the answer:

* **Consent is a workspace's own decision** ([P-14](../vision/principles.md#p-14-ai-is-optional-and-consent-is-a-persons),
  `NG-ai-consent-by-default`). An installation or a plan may offer a model; it may never switch
  processing on.
* **One model for every value that crosses workspaces** ([ADR-0070](./ADR-0070-the-instance-layer.md)):
  default and lock at the installation, the plan between (later), the workspace where no lock lies.
  A second mechanism for AI would be the drift the concept review found in sign-in.
* **Nothing enumerates workspaces.** A change at the installation takes effect where it is read.

## Decision

### 1. The installation may offer AI models, as rows with no workspace

`ai_provider` gains an `id` and a nullable `tenant_id`, the way `identity_provider` did in SI-10: a
row with `tenant_id IS NULL` is a model **the installation offers**. It carries a `display_name`
("Mistral Large (EU)") beside what a workspace row already carries — kind, address, models, sealed
key, `jurisdiction`. Written by operators through `/admin/ai-providers`, `hubctl admin ai`, and the
instance file; readable by every workspace **without** address, model names or key: name, processor
and jurisdiction only. Several may be offered; a workspace picks one by name.

The row-level policy follows `identity_provider`'s three policies (read admits `tenant_id IS NULL`,
write does not, the installation's own scope writes them), and the tenant-boundary test names it.
The installation's re-seal driver ([UC-INS-16](../usecases/admin/UC-INS-16-rotate-the-installations-keys.md))
covers these rows together with the installation's provider secrets.

### 2. Which sources a workspace may use is an instance value with a lock

A new area `ai` in `instance_setting`, resolved `Effective(product, instance, plan, workspace)`:

| Key | Values | "Stricter" means | Product default |
|---|---|---|---|
| `ai.sources` | `NONE` · `OFFERED` · `OWN` · `EITHER` | a subset: `EITHER` ⊃ `OFFERED`, `OWN` ⊃ `NONE` | `OWN` — today's behaviour |
| `ai.min_jurisdiction` | `ANY` · `ADEQUACY` · `EEA` · `SELF_HOSTED` | further right | `ANY` (the environment's third-country switch still applies) |

A workspace may narrow `ai.sources` where it is not locked, and may **always** choose `NONE` — a
lock can hold a workspace *out* of a source, never *in* one, because being held in would be consent
by default. `ai.min_jurisdiction` applies to a workspace's **own** model; an offered model's
declaration is the operator's, shown to the workspace, and not second-guessed by it.

The workspace's choice is one field beside its provider: `ai.source = OFFERED:<id> | OWN | NONE`.

### 3. Consent stays where it is, and applies to either source

`processing_allowed` stays the workspace's switch and is checked before every call, whichever
source answers. Switching on an offered model *is* the consent act — one control, naming the
processor and the jurisdiction. No installation or plan value writes it.

### 4. The budget counts per source

`usage_record` gains the source. `ai_tokens_per_day` limits the **offered** source (the provider
pays for it); a workspace's own model is limited only by the workspace's own optional budget,
`ai_own_tokens_per_day`, off by default. Both are quota rows like every other
([UC-AI-07](../usecases/suggestion/UC-AI-07-keep-ai-within-a-budget.md)).

### 5. Narrowing stops, it never deletes

When the rule in force no longer allows the source a workspace uses, calls stop with
`ai.source_not_allowed`; the workspace's own configuration stays sealed and applies again when the
rule allows it. The same as every downgrade ([P-03](../vision/principles.md#p-03-nothing-is-lost)).

### 6. Changing the model behind search re-embeds in the background

An embedding is comparable only with embeddings from the same model (ADR-0049 §4). A workspace that
switches source, or an operator who changes the offered embedding model, queues a re-embedding job
**in that workspace** — self-seeded by the write that changed it, never by a scheduler visiting
workspaces. Search by words works throughout; search by meaning says it is rebuilding.

### 7. The private installation sees none of it

With no offered model, `ai.sources` resolves to `OWN` and the workspace's AI screen is today's
screen. `D1` meets no "offered", no lock and no minimum ([P-10](../vision/principles.md#p-10-the-smallest-deployment-comes-first)).

## Consequences

* A B2C provider offers one model, locks `ai.sources = OFFERED`, sets the budget — and a consumer
  turns AI on with one click.
* A B2B provider offers its model, leaves `EITHER` open and sets `ai.min_jurisdiction = EEA`; a
  customer with its own gateway uses it, one without uses the provider's.
* Plans (their own milestone) carry the same two keys with no second model — the resolver already
  takes the plan.
* The contract gains `/admin/ai-providers`, the `ai` area of `/admin/settings`, `ai.source` on the
  workspace's AI resource, and a source on usage; nothing is renamed or removed.
* `ai-first.md` §2 and `docs/privacy/ai-providers.md` gain the installation's row; the data catalogue
  gains `usage_record.source`.

## Options considered

**A. Wait for plans (the concept's §6.3).** Rejected: the owner asked for it now, and the
installation level already exists; plans only add a level in between.

**B. Let the operator's model be the fallback when a workspace has none.** Rejected: a fallback is
a model a workspace uses without having chosen it, which is one step from consent by default.

**C. An instance value `ai.provider` that every workspace inherits.** Rejected: it cannot express
"either", and it would put an operator's address and key where a workspace could read them.

**D. Offered rows plus a sources value with a lock (chosen).** The same shape as sign-in providers,
which operators and administrators already know: offered, on nowhere, each workspace decides.

## What this decision does not settle

* Routing a workspace between several offered models by cost or load.
* Prices for AI: Hubtask counts tokens and makes the count readable
  ([UC-INS-14](../usecases/admin/UC-INS-14-tell-a-platform-what-happened.md)); the platform prices it.
