<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const repo = 'https://github.com/Jersyfi/hubtask/blob/main';
</script>

<svelte:head>
  <title>Licence — Hubtask</title>
  <meta
    name="description"
    content="Hubtask is open source under the Apache License 2.0: free for any use, commercial included, with every feature and no licence key. The name is a trademark; contributions are inbound = outbound."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Licence</p>
    <h1>Open source. Apache-2.0. Free for everyone.</h1>
    <p class="lede">
      Hubtask is <strong>open source</strong> under the Apache License 2.0 — the whole repository,
      every version, every image. Use it, change it, run it for yourself, your company or your
      clients, and pass it on. Nobody pays, and nobody ever has to.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">What it means</p>
      <h2>Any use, with one condition about notices</h2>
    </div>
    <div class="table-scroll">
      <table>
        <thead>
          <tr><th scope="col">If you are…</th><th scope="col">Licence needed?</th></tr>
        </thead>
        <tbody>
          <tr><th scope="row">A private person, for yourself or your household</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A non-profit, a school, a university, a public body</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A company running it for its own operations, or for its own clients</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">A freelancer or sole trader, in your professional practice</th><td><span class="yes">No</span></td></tr>
          <tr><th scope="row">Anyone offering a service built on it to third parties</th><td><span class="yes">No</span></td></tr>
        </tbody>
      </table>
    </div>
    <p class="narrow section-gap">
      What the licence asks is that a copy you pass on carries the licence and the
      <code>NOTICE</code> file, and that changed files say they were changed. It comes with no
      warranty: Hubtask is provided as is, with no support period and no maintenance commitment,
      and the project may be archived. If it is, the code stays public under Apache-2.0 and anyone
      may carry it on. Every earlier version is available under Apache-2.0 as well.
    </p>
    <Proof href="{repo}/LICENSE" label="LICENSE · the Apache License 2.0, verbatim" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">One edition</p>
      <h2>One code path. Nothing held back.</h2>
    </div>
    <ul class="ticks ticks-2">
      <li><strong>No licence key, no paid edition.</strong> Every feature is in the one image, for everybody.</li>
      <li><strong>The only limits are operational.</strong> Quotas let the operator of a shared installation keep one workspace from becoming everybody’s latency.</li>
      <li><strong>No telemetry without consent.</strong> Nothing phones home.</li>
      <li><strong>Funded by donations</strong> that buy nothing in return — no support, no priority, no feature.</li>
    </ul>
    <Proof href="{repo}/docs/adr/ADR-0080-hubtask-is-apache-2-0.md" label="ADR-0080 · Hubtask is Apache-2.0" />
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
          The licence covers the code, not the name: Apache-2.0 grants no rights in a trademark.
          Fork freely — but give the result your own name. Official container images come only from
          the project registry.
        </p>
        <Proof href="{repo}/TRADEMARK.md" label="TRADEMARK.md" />
      </div>
      <div>
        <h3>Contributing</h3>
        <p>
          Contributions are welcome and are inbound = outbound: what you submit is licensed under
          Apache-2.0, as the licence itself says. No contributor licence agreement, nothing to sign,
          and you keep the copyright in your work.
        </p>
        <Proof href="{repo}/CONTRIBUTING.md" label="CONTRIBUTING.md" />
      </div>
    </div>
  </div>
</section>
