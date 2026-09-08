# Approved AI providers

Which AI providers this project has assessed, what each of them does with what is sent to it, and
where it does it. Open point **P-3** in
[data-protection.md](../architecture/data-protection.md) §12 asked for exactly this list —
"selection of approved AI providers including zero-retention evidence and an EEA region" — and this
document is the answer.

* **Version:** 0.7.0 · **As of:** 2026-09-08 · **Maintenance:** by pull request, so changes are traceable
* **Concept:** [../architecture/ai-first.md](../architecture/ai-first.md) §2,
  [ADR-0012](../adr/ADR-0012-ai-first-mcp.md), [ADR-0049](../adr/ADR-0049-ai-provider-surface.md)
* **Enforcement:** gate **PG-8** (`test/privacy/PG8_ai_test.go`, run by `make gate-privacy` in
  every pull request) refuses a provider outside the EEA that this installation has not confirmed

> This document describes what the software supports and what the project has looked at. It is
> **not** a recommendation, **not** legal advice, and **not** a substitute for the operator's own
> assessment. Whether a particular transfer is lawful — the adequacy decision or the standard
> contractual clauses, the transfer impact assessment, the processing agreement — is the
> controller's, and no setting in this product decides it.

---

## 1. The default is none

Without configuration, an installation has `NOOP`: nothing is sent anywhere, every AI route answers
`503` with `ai.unavailable`, and the rest of the product is unaffected (QS-09). That is not a
degraded state — it is the state the product is designed around, and
[ADR-0012](../adr/ADR-0012-ai-first-mcp.md) exists to keep it that way.

Two switches stand between that default and a transfer, and they belong to two different people:

| Switch | Whose | Where | Default |
|---|---|---|---|
| `HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER` | The **installation operator**'s | The environment | off |
| `processing_allowed` on the workspace's provider | The **workspace administrator**'s | `PUT /ai-provider` | off |

The split is deliberate and it is the shape of the responsibility rather than of the software. The
operator signs the processing agreement, names the adequacy decision or the standard contractual
clauses, and owes the transfer impact assessment; a workspace administrator cannot take any of that
on for them, so the third-country confirmation is not something they can click past. The workspace
administrator decides whether their own content may be sent at all, which is not the operator's to
decide either.

Configuring a provider and consenting to use it are also two acts. `processing_allowed` is false on
every configuration that does not say otherwise, so a provider can be set up, checked and left
switched off.

---

## 2. What is sent

Only what a feature needs, and only when a person or a rule asked for it. `data-catalog.md` records
the fields; what leaves the installation is:

| Feature | What is sent | What comes back |
|---|---|---|
| Jumble suggestion | The entry's subject and body | A suggested title, date, destination, labels |
| Decomposition | An entry's title and notes | A suggested tree of work packages and activities |
| Automation AI actions | The fields the rule names | A suggestion, or a value the rule applies |
| Semantic search | Titles and notes, as text to embed | A vector |

Every one of them is `PERSONAL_CONTENT`. None of it is sent by a background process nobody asked
for: an embedding follows a write somebody made, a suggestion follows an entry somebody submitted.

**Content is data, never instruction.** A prompt marks user content as context and no server-side
AI call carries out an action "demanded" in the text ([ai-first.md](../architecture/ai-first.md)
§1.3). Actions arise only from automation actions somebody configured.

**Telemetry carries none of it.** No title, note or comment reaches a log, a metric, a trace or an
audit entry on the way to a provider (rule 10, gate PG-4). What the metrics count is calls, tokens
and durations; what a trace span names is the model and the prompt version.

---

## 3. The jurisdictions

`AiJurisdiction` is a declaration by whoever configures the provider. The software cannot verify
it, and it is not meant to: ADR-0018 decision 7 asks for a transfer to be **documented** rather than
prevented.

| Value | Meaning | Needs the operator's confirmation |
|---|---|---|
| `SELF_HOSTED` | A model this installation runs itself. No transfer to anybody | no |
| `EEA` | A provider processing inside the European Economic Area | no |
| `ADEQUACY` | A third country covered by an adequacy decision (Art. 45) | no |
| `THIRD_COUNTRY` | Everything else | **yes** |

A provider of kind `OLLAMA` is `SELF_HOSTED` and the domain refuses any other declaration for it: a
local model transfers to nobody, and recording otherwise would put a transfer in the record that
never happens.

---

## 4. The providers this project has assessed

Two things are recorded per provider: **where** it processes, and **whether it retains**. Neither is
something this repository can prove — both come from the provider's own published terms, and both
are dated, because terms change. An operator re-reads them; this table is a starting point and its
"as of" date is the point.

| Provider | Kind | Region as offered | Retention as published | Assessed |
|---|---|---|---|---|
| A model this installation runs (Ollama, vLLM, LiteLLM against a local backend) | `OLLAMA`, `OPENAI_COMPATIBLE` | The installation's own | None — nothing leaves | 2026-09-08 |
| An OpenAI-compatible endpoint operated inside the EEA by the operator or their processor | `OPENAI_COMPATIBLE` | Declared `EEA` by the operator | The operator's own terms | 2026-09-08 |

**No hosted third-party provider is on this list, and that is the assessment rather than an
omission.** Publishing "provider X is approved" would be this project asserting, on somebody else's
behalf, that a particular company's current terms satisfy a particular controller's obligations —
which is neither ours to assert nor stable enough to be worth writing down: a zero-retention promise
is a contractual term with a date on it, and a table in a repository ages badly against one.

What the project supplies instead is the mechanism and the questions:

1. **Does the endpoint process inside the EEA?** If not, the transfer is `THIRD_COUNTRY` and the
   operator confirms it in the environment, having done the assessment that confirmation stands for.
2. **Does the provider retain what is sent?** A zero-retention arrangement is usually an option
   rather than a default, and often a different endpoint or a contract term. Get it in writing and
   keep the writing.
3. **Is there a processing agreement?** The operator is the processor towards their tenants and the
   controller towards the AI provider; both relationships need one.
4. **Can it be a local model instead?** ADR-0018 decision 7 calls this the recommended path, and it
   is the only row above with nothing to assess.

---

## 5. What is stored, and what an auditor sees

| Stored | Where | Class | Answered by a read |
|---|---|---|---|
| Kind, endpoint, models, jurisdiction, `processing_allowed` | `ai_provider` | `NON_PERSONAL` | yes |
| API key | `ai_provider.api_key_enc` | `SECRET` | **no** |

The key is sealed under the envelope [E-02](../backlog/milestone-0.4.5.md) built, bound to the
workspace it belongs to, and re-sealed by the rotation like every other sealed value
([ADR-0045](../adr/ADR-0045-master-key-in-the-environment.md)). No route answers it and no read
carries it.

Configuring, changing and removing a provider are audited with the provider, the jurisdiction and
the models — the three ADR-0018 decision 7 names, with the fourth, the purpose, being the action
itself. Reading the configuration needs the permission that manages structure or the auditor's
read-only configuration permission (A-4): where a workspace's content may be sent is exactly what an
auditor reads.
