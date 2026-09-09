<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>API, MCP and CLI — building on Hubtask</title>
  <meta
    name="description"
    content="A specification-first REST API with a composable query language, idempotency and optimistic concurrency; an MCP server inside your own installation; hubctl; signed webhooks with CloudEvents; and generated SDKs."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Developers and agents</p>
    <h1>The API is the product</h1>
    <p class="lede">
      Hubtask was built backend first. The specification is written before the code and the code is
      generated from it; the interface people click is one client of that contract, with no private
      endpoints and no shortcuts of its own.
    </p>
    <p class="cta-row">
      <a class="cta" href="https://github.com/Jersyfi/hubtask/blob/main/api/openapi.yaml">The OpenAPI contract</a>
      <a class="cta-quiet" href="{docs}/architecture/api-guidelines.md">API guidelines</a>
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">One registry</p>
      <h2>Register once, reachable three ways</h2>
      <p class="lede">
        A use case is declared in one place with its inputs, its permissions and its side effects.
        REST routes, MCP tools and automation actions are all projections of that declaration — which
        is why they cannot fall out of step, and why a check in the build fails if they ever do.
      </p>
    </div>
    <div class="site-cards">
      <div class="site-card">
        <p class="site-card-index">REST</p>
        <h3>For your scripts and services</h3>
        <p>
          Cursor pagination and no page numbers, <code>Idempotency-Key</code> on every mutation,
          <code>ETag</code> and <code>If-Match</code> for concurrency, and problem documents with a
          machine-readable code and parameters instead of prose to regex.
        </p>
      </div>
      <div class="site-card">
        <p class="site-card-index">MCP</p>
        <h3>For agents, inside your instance</h3>
        <p>
          Tools named after the use case, input schemas identical to the REST bodies, read-only and
          destructive hints so a client can ask before acting, resources addressable by URI, and
          prepared prompts.
        </p>
      </div>
      <div class="site-card">
        <p class="site-card-index">RULES</p>
        <h3>For things that should happen without you</h3>
        <p>
          The same catalogue as rule actions, with conditions in an expression language, a dry run,
          and protection against a rule that triggers itself.
        </p>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Ask before you guess</p>
      <h2>An installation describes itself</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          Installations differ: one has the vector extension and one does not, one has object storage
          and one writes to a volume, one has an AI provider configured and one has deliberately
          none. A client that hard-codes those assumptions is a client that breaks on somebody
          else’s box.
        </p>
        <p>
          So the server answers what it can do. Which capabilities each item type carries, which
          fields are queryable and with which comparisons, which layouts exist, whether semantic
          search is available — all of it read from the installation rather than compiled into the
          client.
        </p>
        <p>
          The same principle points the other way: <strong>a client must tolerate a field it does not
          know.</strong> That is a binding requirement on every first-party client and the reason a
          server upgrade does not break an older one.
        </p>
      </div>
      <div>
        <pre class="site-shell"><code><span class="c"># What can this installation do?</span>
GET /api/v1/meta/capabilities

<span class="c"># What is degraded right now?</span>
GET /api/v1/meta/health

<span class="c"># A composable query, not a search box</span>
POST /api/v1/items:query
&lbrace;
  "filter": "due_at &lt; now+7d and label = 'release'",
  "limit": 50
&rbrace;</code></pre>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Agents</p>
      <h2>An MCP server that runs where your data already is</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          Every large task tool now offers an MCP server, and nearly all of them are a hosted
          endpoint you authorise against their cloud — the agent reads your tasks there. Hubtask’s
          MCP server is an adapter in the process you started. Nothing leaves the machine unless you
          configured something for it to leave to.
        </p>
        <p>
          The security model is the interesting part, and it is the one thing that could not be added
          later: because authorisation lives in the application layer, an MCP tool call carries no
          permission logic of its own. An agent holds a scoped credential and is refused precisely
          what a person with that credential would be refused — and its actions are recorded in the
          trail as an agent’s.
        </p>
        <Proof href="{docs}/architecture/ai-first.md" label="ai-first.md §1" />
      </div>
      <div>
        <h4>What an agent gets</h4>
        <ul class="ticks">
          <li><strong>Tools</strong> The use case catalogue, in snake case, with the preconditions and side effects the registry already knows.</li>
          <li><strong>Resources</strong> Hubs, items and views addressable as <code>hubtask://</code> URIs — a read that is already in the catalogue, reached by URI instead of by argument.</li>
          <li><strong>Prompts</strong> Prepared and versioned, from the same store the product’s own provider calls read.</li>
          <li><strong>A stream</strong> A session identifier that any process can verify without shared state, and the same connection limits a browser’s stream gets.</li>
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The rest of the surface</p>
      <h2>Everything else you would ask for</h2>
    </div>
    <div class="site-cards site-cards-2">
      <div class="site-card">
        <h3><code>hubctl</code></h3>
        <p>
          The command line client. Its types are generated from the same specification, it prints a
          table for a person and the API’s own payload with <code>--json</code> for a pipe — one
          document on standard output and every diagnostic on standard error, so it composes.
        </p>
      </div>
      <div class="site-card">
        <h3>Events out</h3>
        <p>
          Webhook subscriptions with an HMAC signature, retries and a dead-letter path, carrying
          published CloudEvents schemas. Plus trigger polling endpoints, and generated nodes for n8n
          and Zapier.
        </p>
      </div>
      <div class="site-card">
        <h3>SDKs</h3>
        <p>
          TypeScript, Go and Python clients generated from the contract — so a rename in the
          specification shows up as a type error in the same change that made it, rather than in
          somebody’s production two months later.
        </p>
      </div>
      <div class="site-card">
        <h3>Offline conformance</h3>
        <p>
          Building your own client? The synchronisation contract is specified — client-assigned
          identifiers, an operation id per mutation, a hybrid logical clock per field change,
          defined behaviour when access is revoked or a cursor is too old — and
          <code>hubctl sync-conformance</code> checks an implementation against a real instance.
        </p>
        <Proof href="{docs}/architecture/offline-sync.md" label="offline-sync.md §9" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Stability</p>
      <h2>What v1 promises</h2>
    </div>
    <ul class="ticks ticks-2">
      <li><strong>The contract is frozen at 1.0</strong> API v1 is stable from the first stable release, with a deprecation process that has been exercised rather than written down.</li>
      <li><strong>Event schemas are versioned</strong> and published, so a consumer is not reverse-engineering a payload.</li>
      <li><strong>Migrations are forward only</strong> and safe for a rolling update — expand, then contract, never a change to a migration that has run.</li>
      <li><strong>One version for everything</strong> The server and every first-party client share a single version number. There is no compatibility matrix to keep in your head.</li>
    </ul>
    <Proof href="{docs}/architecture/versioning-release.md" label="versioning-release.md" />
  </div>
</section>
