<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { ParameterTable } from '@hubtask/design-system/components';

  import Markdown from '$lib/api/Markdown.svelte';
  import { reference } from '$lib/api/document.ts';

  const headings = { name: 'Name', type: 'Type', required: 'Required', description: 'Description' };
  const words = { required: 'required', optional: 'optional', deprecated: 'deprecated' };
</script>

<svelte:head>
  <title>Schemas — API reference</title>
  <meta name="description" content="Every named schema of the Hubtask REST API contract, with its fields, its enum values and its bounds." />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker"><a href="/developers/api/">API reference</a> · schemas</p>
    <h1>The schemas</h1>
    <p class="lede">
      {reference.schemas.length} named shapes the operations share. A request body or a response names one of
      these, and the operation pages link here.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="site-api-operations">
      {#each reference.schemas as schema (schema.name)}
        <section class="site-api-schema" id={schema.name}>
          <h2><a href="#{schema.name}"><code>{schema.name}</code></a> <span class="site-api-type">{schema.type}</span></h2>
          {#if schema.description}<Markdown source={schema.description} />{/if}
          {#if schema.facts.length > 0}
            <ul class="site-api-codes">
              {#each schema.facts as fact (fact)}<li><code>{fact}</code></li>{/each}
            </ul>
          {/if}
          {#if schema.rows.length > 0}
            <ParameterTable label={schema.name} isLabelHidden {headings} {words} rows={schema.rows} />
          {/if}
        </section>
      {/each}
    </div>
  </div>
</section>
