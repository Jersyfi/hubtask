# ADR-0049 — The AI provider surface: two adapters, no dependency, prompts as files

**Status:** proposed · **Date:** 2026-09-08

## Context

[ADR-0012](./ADR-0012-ai-first-mcp.md) decided the outbound direction in one sentence —
"`core/port/ai/Port.go` with `Complete`/`Embed`; adapters for OpenAI-compatible APIs, local Ollama,
and `NoopAi` (the default)" — and nothing has built it. `0.7.0` does
([`milestone-0.7.0.md`](../backlog/milestone-0.7.0.md), J-01…J-08, J-10), and three questions have
to be answered before the first line of the port is written, because each of them is expensive to
change once four features sit on it.

**Whether an adapter brings a dependency.** Both vendors publish a Go SDK, and the reflex is to use
one. This project's rule is that every dependency is a supply chain decision
(`CLAUDE.md`, "What you do not decide yourself"), and two of the three candidates of `0.6.0` were
accepted through their own ADR while a third was declined by
[ADR-0040](./ADR-0040-no-imap-intake.md). So the question is asked here rather than answered by an
import.

**Where the limits live.** An AI call needs a timeout, a circuit breaker and a per-tenant budget.
Each of the three could plausibly sit in the adapter, in the port's contract or in the application
layer, and if the answer is not written down the second adapter will place them differently from
the first — which is how two providers come to have two different definitions of "unavailable".

**Where the prompts live.** ADR-0012's countermeasures end with "prompts as versioned resources",
which rules out string literals in Go and says nothing more. `ai-first.md` §2 names
`infrastructure/ai/prompts/`. What a *version* is, and who else reads the store, is undecided — and
`0.7.0` has a second reader, because MCP prompts (J-12) are the same artefacts published to an
agent.

## Decision

**1. Neither adapter brings a third-party dependency.** Both providers are JSON over HTTP:
OpenAI-compatible is `POST /v1/chat/completions` and `POST /v1/embeddings`, Ollama is `POST
/api/chat` and `POST /api/embed`. Both adapters use `infrastructure/httpclient.GuardedClient`, which
rule 6 already mandates for every outbound call and which carries the timeout, the retry policy,
the SSRF guard and the refusal to resolve an address that arrived as data
([ADR-0015](./ADR-0015-security-baseline.md), T-07). What an SDK would add on top of that is a typed
request struct — which is thirty lines of Go — and a retry policy this project already has and would
have to switch off.

**2. The three limits, one place each.**

| Limit | Where | Why there |
|---|---|---|
| Timeout | The port's contract, honoured by every adapter | A caller passes a context with a deadline; an adapter that could ignore it would make "no call without a timeout" (rule 7) an adapter's promise rather than the system's |
| Circuit breaker | The adapter, one per configured provider | It is per endpoint, and the endpoint is the adapter's knowledge. The state is a metric and reaches `/meta/health` the way object storage's and SMTP's do |
| Budget | The application layer, before the call is built | It is per tenant and it is a *quota* (J-15, `multi-tenancy.md` §4). An adapter has no tenant, and putting a ceiling behind the port would make every adapter re-implement it |

**3. Prompts are files with a version in their name, and there is one store.**
`infrastructure/ai/prompts/<id>.<version>.md`. The version is part of the filename rather than
front matter, so that a changed prompt is a *new file* and the old one stays readable — which is
what makes a suggestion's recorded `prompt_version` resolvable a year later. The outbound adapters
render from that store and the MCP prompts endpoint (J-12) publishes from it; two stores would be
how one prompt comes to exist in two versions.

**4. `Embed` answers a result, not a bare slice.** `ai-first.md` §2's sketch is
`Embed(ctx, texts []string) ([][]float32, error)`. The port takes the vectors *and* the model that
produced them, because an embedding is only comparable with others from the same model: a provider
reconfigured from one embedding model to another silently mixes two vector spaces, and a hybrid
search over the mixture ranks by nothing. Recording the model beside the vector is what lets J-10
notice and re-embed instead. `Complete` and `Capabilities` keep the documented shape;
`ai-first.md` §2 is updated to this signature in the same change, so the document and the code agree.

**5. `NoopAi` is the default everywhere, and it refuses rather than pretends.** Every call answers
`ErrUnavailable` with the detail code `ai.unavailable` — a `503` with a problem document, which is
the shape arc42 QS-09 fixes. Not an empty result: a summarisation that answers "" and a search that
answers no semantic half are both indistinguishable from a working provider with nothing to say,
and QS-09 is a claim about what an installation *tells* you.

## Options

1. **Two hand-written adapters over the guarded client (chosen).**
2. **The official SDKs.** `openai-go` plus an Ollama client: two modules, their transitive trees,
   and two retry policies to disable. Rejected — the wire format is two endpoints wide, and the
   thing the SDK is worth having for (streaming, tool calls, structured outputs) is not what these
   four features ask of it.
3. **One "LLM gateway" dependency** covering many providers behind one interface. Rejected twice
   over: it is the port this ADR is defining, bought from somebody else, and it would put a
   provider-selection layer *inside* the adapter that the tenant configuration already is.
4. **A single OpenAI-compatible adapter, with Ollama reached through its compatibility endpoints.**
   Tempting, and it is one adapter instead of two. Rejected because Ollama's compatibility surface
   is a translation layer whose gaps are the local models' — `Capabilities()` is precisely how an
   installation learns its model cannot embed, and a compatibility shim answers that question with
   a runtime error instead.

## Consequences

**Positive:** the milestone adds no dependency, so `0.7.0`'s supply chain is `0.6.0`'s; every
outbound AI call inherits the guarded client's timeouts, breaker and SSRF refusal without an
adapter opting in; one prompt store means a suggestion's provenance and an agent's prompt are the
same artefact; and a provider swap stays what ADR-0012 promised — configuration, not code.

**Negative:** a wire-format change at a provider is ours to follow rather than a dependency bump,
and a feature that genuinely needs streaming or tool calls would have to add it by hand. Both are
bounded by decision 1's own reason: two endpoints.

**Countermeasures:** the adapters are covered by a port-level suite both must pass (J-03, J-04), so
a second adapter cannot quietly behave differently; `Capabilities()` makes a provider's gaps a
value rather than an error; and the ADR is revisited if a use case arrives that needs streaming,
which none of `0.7.0`'s does.
