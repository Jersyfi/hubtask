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
    content="Business Source License 1.1, per version: until Licensing Start any use is free, commercial use included, with one exception; every version becomes Apache-2.0 three years after it is published. Source available, not open source."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Licence</p>
    <h1>Free for everyone, until further notice — given in advance.</h1>
    <p class="lede">
      Hubtask is <strong>source available</strong> under the Business Source License 1.1, licensed
      separately for each version — and every version becomes Apache-2.0 three years after it is
      published, irrevocably. This page says who pays today (nobody), what is reserved, what the
      owner intends to charge for one day, and what is deliberately not promised.
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
      “Open source” has a definition, and BSL 1.1 does not meet it: it lets the licensor reserve a
      use. Calling it open source anyway would be a legal problem rather than a wording preference,
      so the project does not do it.
    </p>
    <p class="narrow">
      What you do get is the whole source, a readable history, the right to run it, read it, change
      it, and contribute to it — and a dated, automatic conversion to a genuine open source licence.
      The SDKs, the API contract and the n8n node are Apache-2.0 outright, so a product of yours may
      build on them today.
    </p>
    <Proof href="{repo}/LICENSE" label="LICENSE · the text, with the parameters filled in" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Who pays today</p>
      <h2>Nobody — with one thing reserved</h2>
    </div>
    <div class="table-scroll">
      <table>
        <thead>
          <tr><th scope="col">If you are…</th><th scope="col">Licence needed?</th></tr>
        </thead>
        <tbody>
          <tr><th scope="row">A private person, for yourself or your household</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A non-profit, a school, a university, a public body</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A company evaluating, developing against, or contributing to it</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A company running it for its own operations, or for its own clients</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A freelancer or sole trader, in your professional practice</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">Anyone offering Hubtask itself, or a service built on it, to third parties for a fee</th><td><span class="part">Reserved</span></td></tr>
        </tbody>
      </table>
    </div>
    <p class="narrow section-gap">
      The last line is the provider case: hosting, managing, embedding or white-labelling Hubtask
      as the thing you sell. It is reserved, and until Licensing Start there is no licence to buy
      for it — so it is a case to <a href="mailto:licensing@hubtask.eu">write to us</a> about, not
      one to guess at. Using Hubtask to do your own work for your own clients — an agency running
      its projects in it, a consultancy inviting its clients in as users — is not that case, and the
      licence says so in its own words.
    </p>
    <div class="callout">
      <p>
        <strong>A version keeps its terms.</strong> Every version published before Licensing Start
        stays under these terms until its own Change Date, whatever later versions say. The version
        you install today is yours to run, commercially, for as long as it exists — and three years
        after its release it is Apache-2.0.
      </p>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Licensing Start</p>
      <h2>What changes on a day named at least 60 days ahead</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          <strong>Licensing Start</strong> is a date the owner announces — here, in the release
          notes and in the repository — at least sixty days in advance. It is not tied to a version
          number. From the first release published after it, the use grant changes to the one
          drafted on the right; nothing changes for any version already published.
        </p>
        <p>
          <strong>Until then, nothing is promised.</strong> Hubtask is provided as is: no support
          period, no service levels, no response deadlines, no roadmap commitment, and the project
          may be archived at any time. Security fixes are best effort. What is promised is written
          into each version's licence and needs nobody to keep it: the Change Date, and that a
          published version is never tightened.
        </p>
        <p>
          <strong>What is intended, not promised, from that day:</strong> a one-time licence per
          major version, priced by organisation size, covering every release of that major with
          security updates, and never a lock-out — a licence key restricts nothing. An optional
          annual support contract. An annual licence for providers. A paid major at most every two
          years, a free upgrade for anyone who bought in the year before it, and half price for
          everyone else. No price is published anywhere before that day.
        </p>
      </div>
      <div>
        <h4>The draft grant from Licensing Start</h4>
        <div class="table-scroll">
          <table>
            <thead>
              <tr><th scope="col">Free of charge</th><th scope="col">Licence required</th></tr>
            </thead>
            <tbody>
              <tr><td>Private and household use</td><td>Every other organisation in production use</td></tr>
              <tr><td>Non-profits and public-benefit organisations</td><td>Providers — hosting, managing, embedding or white-labelling Hubtask — whatever their size</td></tr>
              <tr><td>Teaching and non-commercial research</td><td></td></tr>
              <tr><td>Evaluation, development and contribution, by anyone</td><td></td></tr>
              <tr><td>Organisations with fewer than five persons, professional use included</td><td></td></tr>
            </tbody>
          </table>
        </div>
        <p>
          A draft, requiring legal review, effective for no version published before that day.
        </p>
        <Proof href="https://github.com/Jersyfi/hubtask/blob/main/docs/adr/ADR-0059-licensing-phases-and-licensing-start.md" label="ADR-0059 · the phases, the grants and the intended model" />
      </div>
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
          project: what if it is abandoned, and what if the terms are quietly worsened later. If the
          project is archived, the code stays public and every version converts on its date — the
          licensor may bring that forward, and cannot push it back. The version you are running
          today already has its conversion date, and nothing anyone does later can take it away.
        </p>
        <p>
          It is also worth being straight about the history of this licence class: the projects
          damaged by BSL are the ones that <em>changed to</em> it after building a community under
          something permissive. This one starts here, in public, on day one — and the one change it
          has made so far loosened the terms rather than tightened them.
        </p>
      </div>
      <div>
        <h4>The parameters, in full</h4>
        <div class="table-scroll">
          <table>
            <tbody>
              <tr><th scope="row">Licence</th><td>Business Source License 1.1</td></tr>
              <tr><th scope="row">Licensor</th><td>Jérôme Bastian Winkel</td></tr>
              <tr><th scope="row">Licensed work</th><td>Hubtask, in the version the licence file is distributed with</td></tr>
              <tr><th scope="row">Change date</th><td>Three years after a version is first publicly distributed</td></tr>
              <tr><th scope="row">Change licence</th><td>Apache-2.0</td></tr>
              <tr><th scope="row">Use grant</th><td>Any use, commercial production use included, except offering Hubtask itself to third parties</td></tr>
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
      <p class="kicker">Editions</p>
      <h2>One code path. The differences are legal, never functional.</h2>
    </div>
    <div class="site-cards">
      <div class="site-card">
        <h3>Community</h3>
        <p>
          Everyone the use grant covers — until Licensing Start, everyone but a provider. Every
          feature active, no licence key, no telemetry, no limits that are not operational.
        </p>
      </div>
      <div class="site-card">
        <h3>Commercial</h3>
        <p>
          From Licensing Start: organisations the grant does not cover, and providers. The same
          image, plus a licence key as legal evidence and optional add-ons — support with a service
          level, managed instances, advanced sign-in, extended retention.
        </p>
      </div>
      <div class="site-card">
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
          a relicensing needs the rights to relicense — and it binds the licensor in return: your
          work is never placed under stricter terms than it was published with.
        </p>
        <Proof href="{repo}/CONTRIBUTING.md" label="CONTRIBUTING.md" />
      </div>
    </div>
  </div>
</section>
