<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The agent guide as a documentation page (configuration/config-document.md §4): the same
  // procedure the MCP prompt `configure-workspace` hands an agent, for the person reading along.
  import Proof from '#lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>The configuration document — configuring a workspace with an agent</title>
  <meta
    name="description"
    content="A workspace's whole configuration as one document with stable keys: schema, export, explain, validate, plan, apply, history and rollback on REST, hubctl and MCP, and the procedure an agent follows."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Developers and agents</p>
    <h1>The configuration as one document</h1>
    <p class="lede">
      Everything that sets a workspace up — its entry types, fields, link types, workflows, roles,
      modules, structure and settings — reads as one document, every block under a stable key. A
      person, a script and an agent change it the same way: plan a change, look at what it does,
      apply exactly that plan, and take it back when it was wrong.
    </p>
    <p class="cta-row">
      <a class="cta" href="/developers/api/">The API reference</a>
      <a class="cta-quiet" href="{docs}/architecture/api-guidelines.md">API guidelines</a>
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The verbs</p>
      <h2>The same on every door</h2>
      <p class="lede">
        Each verb is one use case of the registry, so the REST route, the <code>hubctl</code> command
        and the MCP tool or resource take the same input and answer the same shape.
      </p>
    </div>
    <div class="site-cards">
      <div class="site-card">
        <p class="site-card-index">READ</p>
        <h3>Schema, export, explain</h3>
        <p>
          <code>GET /meta/config-schema</code> is the document's JSON Schema, taken from the use cases
          that write each section. <code>GET /config</code> is the document in force with its hash as
          the <code>ETag</code>. <code>GET /config/explain?key=</code> answers one key: its value, the
          level it comes from, who locked it and the operation that changes it.
        </p>
      </div>
      <div class="site-card">
        <p class="site-card-index">PLAN</p>
        <h3>Validate and plan</h3>
        <p>
          <code>POST /config:validate</code> answers refusals only. <code>POST /config:plan</code>
          answers every change by key with before and after, the operations it runs, the flags
          <code>hides_data</code> and <code>changes_permissions</code>, whether it needs a fresh
          proof, and a plan hash. Neither writes anything.
        </p>
      </div>
      <div class="site-card">
        <p class="site-card-index">APPLY</p>
        <h3>Apply, history, rollback</h3>
        <p>
          <code>POST /config:apply</code> runs a plan by its hash, all or nothing, against the
          configuration it was planned on, and writes one version. <code>GET /config/versions</code>
          lists them; a rollback is a plan back to a version, applied the same way.
        </p>
      </div>
      <div class="site-card">
        <p class="site-card-index">SCOPE</p>
        <h3>A hub's own document</h3>
        <p>
          The same doors with <code>hub_id</code> read and change what a hub decides itself: its
          modules, its structure settings and which workflow it runs for which type. The hub is
          named by the request, never inside the document, so one hub's document fits another.
        </p>
      </div>
      <div class="site-card">
        <p class="site-card-index">SANDBOX</p>
        <h3>Try it in a copy</h3>
        <p>
          <code>/config/sandbox</code> is a copy of the configuration without content where
          nothing runs. Promoting it with <code>from_sandbox</code> plans only what it changed
          since its copy, and applies like any other plan.
        </p>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The agent guide</p>
      <h2>How an agent changes a workspace</h2>
      <p class="lede">
        The MCP server offers this procedure as the prompt <code>configure-workspace</code>, so
        every agent follows it the same way.
      </p>
    </div>
    <ol class="ticks">
      <li><strong>Read the schema</strong> (<code>hubtask://config/schema</code>) for the sections, their keys and the attributes each block takes.</li>
      <li><strong>Read the configuration in force</strong> (<code>hubtask://config/current</code>) and its hash; a withheld section is left out.</li>
      <li><strong>Explain</strong> what is unclear, key by key.</li>
      <li><strong>Validate</strong> the smallest document that does what the person asked, and correct every refusal: each names the key path and a message code.</li>
      <li><strong>Plan, and show the plan</strong> to the person in plain words — what is added, changed and removed, every flag, and what it reaches: entries that move status, people who gain or lose a permission.</li>
      <li><strong>Apply only what the person approved</strong>, with the plan's hash. A workspace keeps an agent's plan for a person by default: it is answered <code>PROPOSED</code> and waits under the administration's <em>Changes</em>.</li>
      <li><strong>Undo with a rollback</strong>, which is a plan like any other.</li>
    </ol>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Safety</p>
      <h2>What an agent cannot do on its own</h2>
    </div>
    <ul class="ticks ticks-2">
      <li><strong>Apply without a person</strong> unless the workspace allows it: <code>settings.config.agent_apply</code> is <code>APPROVAL</code> by default, and a plan touching that setting always waits for a person.</li>
      <li><strong>Hide data or change permissions unseen</strong>: such a plan is flagged and is not applied without the flag acknowledged.</li>
      <li><strong>Change the sign-in rule</strong>: that needs a fresh proof only a person's session can give.</li>
      <li><strong>Apply a stale plan</strong>: a configuration changed since the plan is refused with <code>config.version_conflict</code>.</li>
    </ul>
    <Proof href="{docs}/architecture/api-guidelines.md" label="api-guidelines.md" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Templates</p>
      <h2>A way of working in one step</h2>
      <p class="lede">
        A template bundles blocks under a version. Taking one is a plan that records
        <code>template@version</code>; nothing follows a template silently. A newer version is a
        three-way comparison of what the template changed against what the workspace changed itself,
        and a change the workspace made is never overwritten without being shown.
      </p>
    </div>
  </div>
</section>

