<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>Security, privacy and compliance — Hubtask</title>
  <meta
    name="description"
    content="Tenant isolation enforced by the database, a hash-chained audit trail with verification, retention with a legal hold, GDPR rights as use cases, twelve security gates in the pipeline, signed images and an SBOM."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Security and privacy</p>
    <h1>The part that is checked, not claimed</h1>
    <p class="lede">
      Every product in this category has a page like this one. What makes this one worth reading is
      that each paragraph names a mechanism and links to the document or the gate behind it. If a
      sentence here has no link, it does not belong here.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Isolation</p>
      <h2>The tenant boundary is in the database</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          In multi-workspace mode, separation is not a filter the application remembers to apply.
          Every table carries the tenant, row level security enforces it, and every query runs inside
          a transaction that has declared which tenant it acts for. A query that has not is refused
          by PostgreSQL, not by a code review.
        </p>
        <p>
          Every repository method carries a cross-tenant negative test, and a gate in the pipeline
          fails the build when a new method arrives without one. That is the difference between an
          isolation property and an isolation intention.
        </p>
        <Proof href="{docs}/architecture/multi-tenancy.md" label="multi-tenancy.md" />
      </div>
      <div>
        <h4>Where authorisation happens</h4>
        <p>
          In the application layer, and nowhere else. Never in an HTTP handler, never in an MCP
          tool, never in a repository. That is what makes “an agent can do exactly what your role
          allows” a structural fact instead of three separate implementations that will drift.
        </p>
        <Proof href="{docs}/adr/ADR-0005-authorization-in-application-layer.md" label="ADR-0005" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Getting in</p>
      <h2>Sign-in, sessions and credentials</h2>
    </div>
    <div class="cards">
      <div class="card">
        <h3>Passwords, done properly</h3>
        <p>Argon2id, and every token stored as a hash. A credential that leaks from the database is not a credential.</p>
      </div>
      <div class="card">
        <h3>Your identity provider</h3>
        <p>OpenID Connect for sign-in, and an OAuth2 provider for third-party applications that want to act for a user.</p>
      </div>
      <div class="card">
        <h3>Multi-factor</h3>
        <p>Time-based one-time codes with single-use recovery codes, enforceable per workspace for the owner and administrator roles.</p>
      </div>
      <div class="card">
        <h3>Step-up</h3>
        <p>Sensitive operations ask again. Sessions are listable and revocable, and a revoked session stops working immediately.</p>
      </div>
      <div class="card">
        <h3>Tokens with scopes</h3>
        <p>Personal access tokens and service accounts, scoped rather than all-powerful — the credential an integration or an agent should hold.</p>
      </div>
      <div class="card">
        <h3>Secrets, sealed</h3>
        <p>Envelope encryption with key rotation for the credentials the system stores on your behalf, such as a backup target’s.</p>
      </div>
    </div>
    <Proof href="{docs}/architecture/security.md" label="security.md" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The trail</p>
      <h2>Tamper-evident, and content-free</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          Every auditable action is registered in a catalogue — a gate fails the build when a new
          one is not — and lands in a hash chain. Each entry commits to the one before it, so an
          altered or removed row breaks the chain at exactly that point. A verification call walks
          it and names the first broken link.
        </p>
        <p>
          <strong>It stores no content of yours.</strong> Not a task title, not a note, not a
          comment. What is recorded is who did what to which entry and when — the same rule that
          keeps your text out of logs, metrics and traces. An audit log you would be embarrassed to
          hand to an auditor is not an audit log.
        </p>
        <Proof href="{docs}/architecture/audit.md" label="audit.md" />
      </div>
      <div>
        <h4>What it is good for</h4>
        <ul class="ticks">
          <li><strong>An auditor role</strong> Reads the trail and the configuration, and no work items at all.</li>
          <li><strong>Export to your SIEM</strong> The trail leaves the system in a form your tooling already reads.</li>
          <li><strong>Agent actions marked</strong> An action taken by an AI agent is recorded as one, so “what did the automation do” is answerable.</li>
          <li><strong>No delete grant</strong> The application’s database role cannot update or delete an audit row. Not “does not” — cannot.</li>
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Data protection</p>
      <h2>GDPR as operations, not as a policy document</h2>
    </div>
    <div class="rows">
      <div class="row">
        <h3>Rights with deadlines</h3>
        <div><p>
          Access, rectification, erasure, restriction and portability are use cases. A request is a
          record with a statutory deadline the system tracks, so “we will get to it” has a date
          attached to it.
        </p></div>
      </div>
      <div class="row">
        <h3>A catalogue of every field</h3>
        <div><p>
          Personal data is inventoried with its legal basis, its storage location and the path that
          deletes it — including the copies in a backup archive and the reference in the trail. A
          gate in the pipeline fails when a new personal field arrives without an entry.
        </p>
        <Proof href="{docs}/privacy/data-catalog.md" label="data-catalog.md" /></div>
      </div>
      <div class="row">
        <h3>Retention with brakes</h3>
        <div><p>
          A rule previews what it would delete, warns in advance, keeps a grace period and can be
          stopped mid-run. A legal hold overrides it. Writing one asks for the owner’s authority,
          because a standing instruction to destroy work is not an administrator’s errand.
        </p></div>
      </div>
      <div class="row">
        <h3>No content in telemetry</h3>
        <div><p>
          Titles, notes and comments never reach a log line, a metric label, a trace attribute or an
          audit entry. Redaction is a property of the logging layer rather than a discipline applied
          per call site.
        </p></div>
      </div>
      <div class="row">
        <h3>Where the data sits</h3>
        <div><p>
          Wherever you put it. Self-hosted means self-hosted: there is no control plane, no vendor
          account, and no default backend anywhere. The one optional outbound path is an AI provider
          you configure, and an installation that configures none makes no outbound call at all.
        </p></div>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The pipeline</p>
      <h2>Twelve security gates, and one that tests the gates</h2>
      <p class="lede">
        Security requirements that live in a document get skipped under deadline. These run on every
        change, and a red one blocks the merge.
      </p>
    </div>
    <div class="cards">
      <div class="card">
        <h3>Known vulnerabilities</h3>
        <p>Dependency scanning, secret scanning, an SBOM published with every release, and a signature on every image.</p>
      </div>
      <div class="card">
        <h3>No unguarded outbound call</h3>
        <p>Every outbound HTTP request goes through a guarded client with timeouts and server-side request forgery protection. There is a gate for the ones that try not to.</p>
      </div>
      <div class="card">
        <h3>No string-built SQL</h3>
        <p>Queries are generated and parameterised. No byte from a request becomes SQL text, including in the filter language.</p>
      </div>
      <div class="card">
        <h3>No cross-tenant read</h3>
        <p>A negative test per repository method, and a gate that notices a missing one.</p>
      </div>
      <div class="card">
        <h3>Headers and rate limits</h3>
        <p>A content security policy without inline execution, security headers, and limits that shed load rather than falling over.</p>
      </div>
      <div class="card">
        <h3>A gate that fails on purpose</h3>
        <p>A self-test deliberately breaks each rule and proves the gate turns red. A gate nobody has seen fail is a gate nobody should trust.</p>
      </div>
    </div>
    <Proof href="{docs}/architecture/security.md" label="security.md · the gates and the threat model" />

    <div class="callout section-gap">
      <p>
        <strong>Reporting a vulnerability.</strong> The path, the response deadlines and the advisory
        process are in <code>SECURITY.md</code> in the repository. Please use it rather than an
        issue.
      </p>
      <Proof href="https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md" label="SECURITY.md" />
    </div>
  </div>
</section>
