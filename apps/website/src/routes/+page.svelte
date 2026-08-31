<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<svelte:head>
  <title>Hubtask</title>
  <meta
    name="description"
    content="Hubtask — task management in five levels. Self-hostable, API-first, agent-ready, with backups that restore and an audit trail that verifies. Source available under BSL 1.1."
  />
</svelte:head>

<!-- What the page claims is split on one line: everything above "The road ahead" is shipped and
     checked in CI today; the roadmap section is labelled as the roadmap. That split is the F1-12
     rule ("only what is already true") surviving a fuller page. -->
<main>
  <header class="hero">
    <h1>Hubtask</h1>
    <p class="lede">
      Task management with room to think — for one person on one box, and for providers running
      many workspaces. Built backend first and API first: the core and its API are the product,
      and every frontend, integration and AI agent is an equal client.
    </p>
    <p class="levels" aria-label="The five levels">
      <span class="chip chip-slate">Hub</span>
      <span class="sep" aria-hidden="true">→</span>
      <span class="chip chip-blue">Collection</span>
      <span class="sep" aria-hidden="true">→</span>
      <span class="chip chip-teal">Task</span>
      <span class="sep" aria-hidden="true">→</span>
      <span class="chip chip-violet">Work&nbsp;package</span>
      <span class="sep" aria-hidden="true">→</span>
      <span class="chip chip-amber">Activity</span>
    </p>
    <p class="cta-row">
      <a class="cta" href="https://github.com/Jersyfi/hubtask">Source on GitHub</a>
      <a class="quiet" href="https://github.com/Jersyfi/hubtask/tree/main/docs/architecture"
        >Read the architecture</a
      >
    </p>
  </header>

  <section class="block">
    <p class="kicker">The model</p>
    <h2>Five levels, one shape</h2>
    <p>
      A <strong>hub</strong> is an area of life or a client. A <strong>collection</strong> is a
      project or a list. A <strong>task</strong> is the unit of work, a
      <strong>work&nbsp;package</strong> a step of it, an <strong>activity</strong> the smallest
      tick. Due dates, reminders, labels, members, comments, attachments and custom fields live
      where you need them — and the hierarchy rolls completion upwards when you tell it to.
    </p>
  </section>

  <section class="block">
    <p class="kicker">The contract</p>
    <h2>Three doors, one catalogue</h2>
    <div class="cards">
      <div class="card">
        <h3><code>REST</code></h3>
        <p>
          A complete, documented HTTP API with a composable query language, idempotency and
          optimistic concurrency — built for scripts and integrations, not exported from a UI.
        </p>
      </div>
      <div class="card">
        <h3><code>MCP</code></h3>
        <p>
          An MCP server for AI agents, speaking the same catalogue. An agent can do whatever
          your role allows — and nothing more.
        </p>
      </div>
      <div class="card">
        <h3><code>Automation</code></h3>
        <p>
          Every use case is registered once and exposed everywhere. A parity check in CI fails
          the build if the three channels ever disagree.
        </p>
      </div>
    </div>
  </section>

  <section class="block">
    <p class="kicker">In the box today</p>
    <h2>Already shipped, already tested</h2>
    <ul class="features">
      <li><code>search</code> full-text search and a composable filter DSL</li>
      <li><code>views</code> kanban buckets, saved views, CSV/JSON/ICS export</li>
      <li><code>time</code> due dates, reminders, RFC 5545 recurrence across DST</li>
      <li><code>templates</code> whole task trees instantiated in one call</li>
      <li><code>calendar</code> revocable ICS feeds for any calendar app</li>
      <li><code>live</code> server-sent events for real-time updates</li>
      <li><code>cli</code> hubctl — the whole product from a terminal</li>
      <li><code>i18n</code> message codes end to end; any language, zone, direction</li>
    </ul>
  </section>

  <section class="block trust">
    <p class="kicker">The difference</p>
    <h2>Your data, provably yours</h2>
    <div class="cards">
      <div class="card">
        <h3>Backups that restore</h3>
        <p>
          Encrypted archives on your own target — S3, SFTP, WebDAV or local — with generational
          retention. The restore drill runs in CI on every change: a backup that has never been
          restored is a hypothesis.
        </p>
      </div>
      <div class="card">
        <h3>An audit trail that verifies</h3>
        <p>
          Every auditable action lands in a hash chain that stores no content. One call checks
          the chain and names the first broken link — tampering is found, not suspected.
        </p>
      </div>
      <div class="card">
        <h3>Retention with brakes</h3>
        <p>
          Deletion rules announce what they will delete before they delete it, can be stopped,
          and are overridden by a legal hold. Nothing disappears quietly.
        </p>
      </div>
      <div class="card">
        <h3>GDPR as a feature</h3>
        <p>
          Access, export and erasure are use cases with a statutory deadline the system watches
          — served by the product, not by a support ticket.
        </p>
      </div>
    </div>
  </section>

  <section class="block">
    <p class="kicker">Operations</p>
    <h2>Run it yourself</h2>
    <p>
      One container image, PostgreSQL as the only required dependency, Compose for one box and
      Helm for a fleet. The tenant boundary is enforced in the database itself, twelve security
      gates run in the pipeline, and <code>/meta/health</code> says what is missing instead of
      crashing.
    </p>
    <pre class="shell"><code>docker compose up -d</code></pre>
  </section>

  <section class="block">
    <p class="kicker">Roadmap</p>
    <h2>The road ahead</h2>
    <p>
      Hubtask is built in the open, milestone by milestone — what follows is the plan, not a
      promise with a date on it.
    </p>
    <ul class="roadmap">
      <li><code>0.5</code> automation rules, webhooks, personal access tokens, the jumble inbox</li>
      <li><code>0.6</code> multi-tenant operations, OIDC sign-in, MFA, quotas</li>
      <li><code>0.7</code> optional AI: suggestions, semantic search — switchable off</li>
      <li><code>0.8.5</code> offline synchronisation with conflict-preserving merges</li>
      <li><code>1.0</code> the first stable release</li>
    </ul>
  </section>
</main>
