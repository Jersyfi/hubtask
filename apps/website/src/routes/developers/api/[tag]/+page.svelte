<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Markdown from '$lib/api/Markdown.svelte';
  import OperationCard from '$lib/api/OperationCard.svelte';
  import { reference } from '$lib/api/document.ts';

  const { data } = $props();
  const tag = $derived(data.tag);
</script>

<svelte:head>
  <title>{tag.name} — API reference</title>
  <meta name="description" content="The {tag.operations.length} operations under the {tag.name} tag of the Hubtask REST API, rendered from the contract." />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker"><a href="/developers/api/">API reference</a> · {reference.title} {reference.version}</p>
    <h1>{tag.name}</h1>
    {#if tag.description}
      <div class="lede"><Markdown source={tag.description} /></div>
    {/if}
    <nav class="site-api-toc" aria-label="Operations under {tag.name}">
      <ul>
        {#each tag.operations as operation (operation.id)}
          <li><a href="#{operation.id}"><code>{operation.method}</code> <code>{operation.path}</code></a></li>
        {/each}
      </ul>
    </nav>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="site-api-operations">
      {#each tag.operations as operation (operation.id)}
        <OperationCard {operation} />
      {/each}
    </div>
  </div>
</section>
