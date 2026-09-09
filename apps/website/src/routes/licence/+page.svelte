<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const repo = 'https://github.com/Jersyfi/hubtask/blob/main';
</script>

<svelte:head>
  <title>Licence and editions — Hubtask</title>
  <meta
    name="description"
    content="Business Source License 1.1: free for private use, non-profits, teaching and evaluation; a paid licence for commercial operation. Every version becomes Apache-2.0 three years after it is published. Source available, not open source."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Licence</p>
    <h1>Free for you. Paid if you sell it.</h1>
    <p class="lede">
      Hubtask is <strong>source available</strong> under the Business Source License 1.1 — and every
      version becomes Apache-2.0 three years after it is published, per version, irrevocably. This
      page says exactly who pays, who does not, and which part of this is not settled yet.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The words matter</p>
      <h2>This is not open source, and we do not call it that</h2>
    </div>
    <p class="narrow">
      “Open source” has a definition, and BSL 1.1 does not meet it: it reserves commercial
      production use. Calling it open source anyway would be a legal problem rather than a wording
      preference, so the project does not do it — the build for this website will fail on a page
      that tries.
    </p>
    <p class="narrow">
      What you do get is the whole source, a readable history, the right to run it, read it, change
      it, and contribute to it — and a dated, automatic conversion to a genuine open source licence.
    </p>
    <Proof href="{repo}/LICENSE" label="LICENSE · the text, with the parameters filled in" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Who pays</p>
      <h2>The line, drawn where you can check it</h2>
    </div>
    <div class="table-scroll">
      <table>
        <thead>
          <tr><th scope="col">If you are…</th><th scope="col">Paid licence?</th></tr>
        </thead>
        <tbody>
          <tr><th scope="row">A private person, for yourself or your household</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A registered non-profit, for its charitable purposes</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A school or university, for teaching</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">Anyone, for non-commercial research</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A company evaluating, testing or developing against it</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A company running it for its own operations or its customers</th><td><span class="part">Yes</span></td></tr>
          <tr><th scope="row">Anyone offering it, or a service built on it, to third parties for a fee</th><td><span class="part">Yes</span></td></tr>
          <tr><th scope="row">A freelancer or sole trader, in their professional practice</th><td><span class="part">Yes</span></td></tr>
        </tbody>
      </table>
    </div>
    <p class="narrow section-gap">
      The last line is the debatable one, and it was drawn deliberately rather than by accident: the
      model is free for private persons and paid for commercial activity, and a sole trader’s
      practice is commercial activity. It is also the line most likely to move.
    </p>
    <div class="callout">
      <p>
        <strong>No-cost commercial licences exist.</strong> They are granted on request to
        non-profits, educational institutions and public bodies whose use falls outside the grant
        above, and case by case to small projects where charging would be absurd. That is policy
        rather than licence text, which means it can be widened without amending anything.
        <a href="mailto:info@hubtask.eu">Ask about one</a>.
      </p>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The guarantee</p>
      <h2>What happens if this project stops</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          The Change Date applies <strong>per version</strong>. A version published in March 2027 is
          Apache-2.0 in March 2030 — regardless of what happens afterwards to newer versions, to the
          project, or to the licensor.
        </p>
        <p>
          That is the answer to the two questions everybody sensible asks about a source-available
          project: what if it is abandoned, and what if the terms are quietly worsened later. The
          version you are running today already has its conversion date, and nothing anyone does
          later can take it away.
        </p>
        <p>
          It is also worth being straight about the history of this licence class: the projects
          damaged by BSL are the ones that <em>changed to</em> it after building a community under
          something permissive. This one starts here, in public, on day one.
        </p>
      </div>
      <div>
        <h4>The parameters, in full</h4>
        <div class="table-scroll">
          <table>
            <tbody>
              <tr><th scope="row">Licence</th><td>Business Source License 1.1</td></tr>
              <tr><th scope="row">Licensor</th><td>Jérôme Bastian Winkel</td></tr>
              <tr><th scope="row">Licensed work</th><td>Hubtask, all versions</td></tr>
              <tr><th scope="row">Change date</th><td>Three years after a version is first publicly distributed</td></tr>
              <tr><th scope="row">Change licence</th><td>Apache-2.0</td></tr>
              <tr><th scope="row">Use grant</th><td>Free for any non-commercial purpose, including production self-hosting with the full feature set</td></tr>
            </tbody>
          </table>
        </div>
        <Proof href="https://github.com/Jersyfi/hubtask/blob/main/docs/adr/ADR-0013-licensing.md" label="ADR-0013 · why this and not MIT or AGPL" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Honestly</p>
      <h2>What a commercial licence costs is not decided yet</h2>
    </div>
    <div class="callout">
      <p>
        We would rather leave this blank than guess at it. The pricing and the metering for
        commercial use are still being worked out — a final market analysis comes first — and where
        exactly the commercial boundary falls may still move with it.
      </p>
      <p>
        <strong>Two commitments in the meantime.</strong> It will be published on this page before
        anybody is asked to pay for anything. And it will not touch the part that is already
        settled and cannot be taken back: private use is free, the full feature set is in every
        installation, and every version converts to Apache-2.0 on its own schedule.
      </p>
      <p>
        Running it commercially now, and want to talk before there is a price list?
        <a href="mailto:info@hubtask.eu">Write to us</a> — that conversation is useful to both of us.
      </p>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Editions</p>
      <h2>One code path. The differences are legal, never functional.</h2>
    </div>
    <div class="cards">
      <div class="card">
        <h3>Community</h3>
        <p>
          Private individuals, non-profits, evaluation, development. Every feature active, no
          licence key, no telemetry, no limits that are not operational.
        </p>
      </div>
      <div class="card">
        <h3>Commercial</h3>
        <p>
          Companies and service providers. The same image, plus a licence key as legal evidence and
          optional add-ons — support with a service level, managed instances, advanced sign-in,
          extended retention.
        </p>
      </div>
      <div class="card">
        <h3>Hosted</h3>
        <p>
          Later, and only if it is wanted: multi-workspace mode plus billing, for people who would
          rather not operate it. It changes nothing for anybody who does.
        </p>
      </div>
    </div>

    <div class="section-gap">
      <h3>Four rules that make “one code path” true</h3>
      <ul class="ticks ticks-2">
        <li><strong>No feature kill switch anywhere in the core.</strong> The only limits that exist are ones that make operational sense, and they are unbounded in community mode.</li>
        <li><strong>An invalid licence key restricts nothing.</strong> It produces a notice in the administration area. Technical enforcement would cost more trust than it protects revenue.</li>
        <li><strong>Metering is opt-in</strong> and off in self-hosting. It exists as a port so that any billing model can be built later without touching the domain.</li>
        <li><strong>No telemetry without consent.</strong> An optional, documented, anonymous version ping — off by default.</li>
      </ul>
      <Proof href="https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/licensing-editions.md" label="licensing-editions.md" />
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Two more things</p>
      <h2>The name, and contributing</h2>
    </div>
    <div class="two-up">
      <div>
        <h3>The trademark</h3>
        <p>
          The licence permits forks; the trademark policy means a fork cannot call itself Hubtask.
          Official container images come only from the project registry. That is how an identity
          survives a permissive future — the code converts to Apache-2.0, the name does not.
        </p>
        <Proof href="{repo}/TRADEMARK.md" label="TRADEMARK.md" />
      </div>
      <div>
        <h3>Contributing</h3>
        <p>
          Contributions are welcome and go through a contributor licence agreement, signed once on
          your first pull request. It is what keeps the promised conversion to Apache-2.0 possible —
          a relicensing needs the rights to relicense.
        </p>
        <Proof href="{repo}/CONTRIBUTING.md" label="CONTRIBUTING.md" />
      </div>
    </div>
  </div>
</section>
