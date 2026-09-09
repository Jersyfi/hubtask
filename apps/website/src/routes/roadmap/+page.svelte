<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>How Hubtask is built — the roadmap</title>
  <meta
    name="description"
    content="The plan, milestone by milestone, and the working method behind it: architecture decision records, quality gates that block a merge, and evidence files instead of assertions."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">The road</p>
    <h1>Built in the open, in the order that made it cheap</h1>
    <p class="lede">
      The core was built first and completely; automation, operations and the clients followed. This
      page is the plan and the method — not a set of dates, because a date on a one-person project
      is a guess with a font.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The milestones</p>
      <h2>What was built, and in which order</h2>
    </div>
    <ol class="rail">
      <li data-state="done">
        <div class="rail-head"><span class="rail-version">0.1</span><span class="rail-state">Foundation</span></div>
        <p>
          A walking skeleton with exactly one use case — and every cross-cutting concern driven
          through it once, completely: security gates, resilience, observability, the audit chain,
          the data catalogue, the synchronisation foundations. Deliberately first, because a gate
          that starts biting when there are two hundred methods finds two hundred defects at once.
        </p>
      </li>
      <li data-state="done">
        <div class="rail-head"><span class="rail-version">0.2 – 0.4</span><span class="rail-state">The business core</span></div>
        <p>
          Hierarchy and items, buckets, labels, ordering, the query language, trash and archive.
          Then comments, members and assignment, covers, attachments, custom fields, search,
          notifications and the live stream. Then time: due dates, reminders, recurrence across
          daylight saving, templates, saved views and the calendar feed.
        </p>
      </li>
      <li data-state="done">
        <div class="rail-head"><span class="rail-version">0.4.5</span><span class="rail-state">Backup, retention, audit</span></div>
        <p>
          Backup targets and the encrypted archive format, schedules and generational retention,
          restore in every mode; retention rules with a preview and safeguards; audit query, export
          and verification; legal hold; data subject requests with deadline tracking.
        </p>
      </li>
      <li data-state="done">
        <div class="rail-head"><span class="rail-version">0.5 – 0.6</span><span class="rail-state">Automation and operations</span></div>
        <p>
          The rule engine with a dry run, signed webhooks, personal access tokens and service
          accounts, the intake inbox. Then multi-workspace operation, provisioning and export,
          quotas, sign-in through your identity provider, multi-factor, session management, envelope
          encryption with key rotation, and the fleet deployment under load.
        </p>
      </li>
      <li data-state="now">
        <div class="rail-head"><span class="rail-version">0.7 – 0.9</span><span class="rail-state">AI, languages, offline, ecosystem</span></div>
        <p>
          The MCP server complete, the AI port with a local and a hosted provider, suggestions and
          hybrid semantic search — all optional. Then the full internationalisation surface,
          accessibility, and offline synchronisation with per-field merging and conflict
          preservation. Then the ecosystem: connectors, client SDKs, CalDAV, and imports from the
          tools people are leaving.
        </p>
      </li>
      <li>
        <div class="rail-head"><span class="rail-version">0.9.5</span><span class="rail-state">Convergence</span></div>
        <p>
          Not a feature milestone. The scope window closes, a coverage report records where every
          use case is reachable in every client and why any omission is deliberate, and everything
          with external lead time starts — signed installers, store submissions, and this website’s
          1.0 content.
        </p>
      </li>
      <li>
        <div class="rail-head"><span class="rail-version">1.0</span><span class="rail-state">Stable</span></div>
        <p>
          Twenty-one prerequisites, each demonstrable: the API frozen and its deprecation process
          exercised, a security review including an external audit of the tenant boundary, restore
          drills from every released target type, offline conformance passed by a real client,
          accessibility demonstrated with a published statement, a service level report over at
          least thirty days, and reproducible reference deployments.
        </p>
        <Proof href="{docs}/roadmap.md" label="roadmap.md · the full list" />
      </li>
    </ol>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The method</p>
      <h2>Why the guarantees on this site are checkable</h2>
      <p class="lede">
        None of the claims elsewhere on this site would survive without the working method
        underneath them. It is unusual enough for a project this size to be worth describing.
      </p>
    </div>
    <div class="site-cards site-cards-2">
      <div class="site-card">
        <h3>Decisions are written down before they are coded</h3>
        <p>
          Every architectural choice is an numbered decision record with the alternatives that were
          weighed and the reason one won — including the ones that were declined. The architecture
          document and the decision index are reconciled by a check, so neither can quietly drift
          from the other.
        </p>
        <Proof href="{docs}/adr/README.md" label="the decision records" />
      </div>
      <div class="site-card">
        <h3>The specification moves first</h3>
        <p>
          An API change starts in the contract, then the code is generated, then it is implemented.
          A data model change brings its migration in the same order. Hand-editing generated code is
          not a style question here; the build notices.
        </p>
      </div>
      <div class="site-card">
        <h3>Gates block the merge</h3>
        <p>
          Architecture boundaries, the concurrency rule, security, privacy, licences of every
          dependency, documentation cross-references, the observability catalogue — all of it runs
          on every change. And a self-test deliberately breaks each rule to prove the gate turns
          red, because a gate nobody has seen fail is decoration.
        </p>
        <Proof href="{docs}/architecture/ci-cd.md" label="ci-cd.md" />
      </div>
      <div class="site-card">
        <h3>Evidence rather than assertion</h3>
        <p>
          Resilience runs, load figures, restore drills and maturity decisions are written to
          evidence files with a date on them. When this site says the drill runs, there is a file
          you can open.
        </p>
        <Proof href="{docs}/evidence/README.md" label="docs/evidence" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">After 1.0</p>
      <h2>What the freeze means</h2>
    </div>
    <p class="narrow">
      From the convergence milestone onwards there are no new features until the release — only
      defects. A stabilisation phase that still accepts features is not a stabilisation phase, and
      every date after it is a guess. New requirements go to the next minor, or they arrive with a
      decision record that says what the exception costs.
    </p>
    <p class="narrow">
      After 1.0, API v1 is stable, event schemas are versioned, and a client regression blocks a
      release exactly as an API regression does.
    </p>
  </div>
</section>
