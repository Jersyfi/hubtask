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
  <h1>Hubtask</h1>
  <p class="lede">
    Task management in five levels — from a hub down to the single step. Built API-first: every
    feature is reachable through REST, through MCP for AI agents, and through automation, with
    the same catalogue behind all three.
  </p>

  <section>
    <h2>Self-hostable</h2>
    <p>
      Hubtask runs on your own infrastructure as a single container image, with PostgreSQL as the
      only required dependency. Backups that restore, an audit trail that verifies, and data
      subject rights served by the system itself — not by a support ticket.
    </p>
  </section>

  <section>
    <h2>Source available</h2>
    <p>
      The source code is public and licensed under the Business Source License 1.1. That is a
      source-available license, not an open source license: each release converts to the Apache
      License 2.0 three years after its publication, as its Change Date passes.
    </p>
  </section>

  <section>
    <h2>Follow the work</h2>
    <p>
      Hubtask is in active development, milestone by milestone, in the open. The repository holds
      the code, the architecture decisions, and the roadmap.
    </p>
    <p><a href="https://github.com/Jersyfi/hubtask">github.com/Jersyfi/hubtask</a></p>
  </section>
</main>
