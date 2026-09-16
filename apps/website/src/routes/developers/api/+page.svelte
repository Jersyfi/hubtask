<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Markdown from '$lib/api/Markdown.svelte';
  import { contractUrl, reference } from '$lib/api/document.ts';
</script>

<svelte:head>
  <title>API reference — {reference.title}</title>
  <meta
    name="description"
    content="Every operation of the Hubtask REST API, rendered from the OpenAPI contract: {reference.operationCount} operations under {reference.tags.length} tags, the event schemas, the schemas, and the conventions every request follows."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">API reference · version {reference.version}</p>
    <h1>{reference.title}</h1>
    <p class="lede">
      Rendered from <a href={contractUrl}>the contract</a>, which is the source: {reference.operationCount}
      operations under {reference.tags.length} tags, every schema, every event. The base address is
      <code>{reference.baseUrl}</code>.
    </p>
    <p class="cta-row">
      <a class="cta" href="/developers/api/conventions/">Conventions first</a>
      <a class="cta-quiet" href="/developers/api/events/">The events</a>
      <a class="cta-quiet" href="/developers/api/schemas/">The schemas</a>
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">About</p>
      <h2>What the contract says of itself</h2>
    </div>
    <div class="narrow">
      <Markdown source={reference.description} />
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">By tag</p>
      <h2>The operations</h2>
    </div>
    <ul class="site-api-tags">
      {#each reference.tags as tag (tag.slug)}
        <li class="site-card">
          <p class="site-card-index">{tag.operations.length} {tag.operations.length === 1 ? 'operation' : 'operations'}</p>
          <h3><a href="/developers/api/{tag.slug}/">{tag.name}</a></h3>
          {#if tag.lede}<p>{tag.lede}</p>{/if}
          <ul class="site-api-taglist">
            {#each tag.operations as operation (operation.id)}
              <li><a href="/developers/api/{tag.slug}/#{operation.id}"><code>{operation.method}</code> <code>{operation.path}</code></a></li>
            {/each}
          </ul>
        </li>
      {/each}
    </ul>
  </div>
</section>
