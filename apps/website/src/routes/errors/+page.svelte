<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { codes } from '#lib/errors/codes.ts';
</script>

<svelte:head>
  <title>Errors — Hubtask</title>
  <meta
    name="description"
    content="Every error code the Hubtask API answers with: what it means and what to do next."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker"><a href="/developers/api/">API reference</a> · errors</p>
    <h1>The error codes</h1>
    <p class="lede">
      Every refusal carries one of these codes, and its <code>docs</code> link points here. The code is stable;
      <code>detail_code</code> narrows it to the concrete case.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <ul class="site-api-tags">
      {#each codes as entry (entry.code)}
        <li class="site-card">
          <p class="site-card-index">HTTP {entry.status}</p>
          <h3><a href="/errors/{entry.code}/"><code>{entry.code}</code></a></h3>
          <p>{entry.title}</p>
        </li>
      {/each}
    </ul>
  </div>
</section>
