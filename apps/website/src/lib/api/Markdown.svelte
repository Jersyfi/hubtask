<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A description from the contract, rendered element by element from the tree markdown.ts
  // reads. No `{@html}`: a string from the document becomes a text node and nothing else.
  import { blocks, type Inline } from './markdown.ts';

  const { source }: { source: string } = $props();
  const tree = $derived(blocks(source));
</script>

{#snippet inline(parts: readonly Inline[])}
  {#each parts as part, i (i)}
    {#if part.kind === 'code'}<code>{part.text}</code>
    {:else if part.kind === 'strong'}<strong>{part.text}</strong>
    {:else if part.kind === 'em'}<em>{part.text}</em>
    {:else if part.kind === 'link'}<a href={part.href}>{part.text}</a>
    {:else}{part.text}{/if}
  {/each}
{/snippet}

<div class="site-api-prose">
  {#each tree as block, i (i)}
    {#if block.kind === 'paragraph'}
      <p>{@render inline(block.inlines)}</p>
    {:else if block.kind === 'list'}
      <ul>
        {#each block.items as item, j (j)}<li>{@render inline(item)}</li>{/each}
      </ul>
    {:else}
      <pre class="site-shell"><code>{block.text}</code></pre>
    {/if}
  {/each}
</div>
