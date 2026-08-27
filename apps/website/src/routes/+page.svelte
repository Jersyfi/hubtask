<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<svelte:head>
  <title>Hubtask</title>
  <meta
    name="description"
    content="Hubtask — task management in five levels. Self-hostable, API-first, source available under BSL 1.1."
  />
</svelte:head>

<!-- The holding page: only what is already true and already checked (F1-12). What the product
     is, how it is licensed, and that it is self-hostable. No dates, no editions, no signup - a
     signup is personal data with a legal basis and a deletion path, which is a decision and not
     a form. The content wave with positioning and visual direction waits on its brief. -->
<main>
  <header class="hero">
    <h1>Hubtask</h1>
    <p class="lede">
      Task management with room to think — from the big picture down to the single step.
    </p>
    <p class="levels" aria-label="The five levels">
      <span>Hub</span> <span aria-hidden="true">→</span> <span>Collection</span>
      <span aria-hidden="true">→</span> <span>Task</span> <span aria-hidden="true">→</span>
      <span>Work&nbsp;package</span> <span aria-hidden="true">→</span> <span>Activity</span>
    </p>
  </header>

  <div class="cards">
    <section class="card">
      <h2>API-first</h2>
      <p>
        Every feature is reachable three ways with one catalogue behind them: REST for
        applications, MCP for AI agents, and the built-in automation. Nothing exists only in a
        user interface.
      </p>
    </section>

    <section class="card">
      <h2>Self-hostable</h2>
      <p>
        One container image, PostgreSQL as the only required dependency. Backups that restore,
        an audit trail that verifies, and data subject rights served by the system itself — not
        by a support ticket.
      </p>
    </section>

    <section class="card">
      <h2>Source available</h2>
      <p>
        The source code is public under the Business Source License 1.1 — source available, not
        open source: each release converts to the Apache License 2.0 three years after its
        publication, as its Change Date passes.
      </p>
    </section>

    <section class="card">
      <h2>Built in the open</h2>
      <p>
        Milestone by milestone, with the architecture decisions and the roadmap in the
        repository next to the code.
      </p>
      <p><a href="https://github.com/Jersyfi/hubtask">github.com/Jersyfi/hubtask</a></p>
    </section>
  </div>
</main>
