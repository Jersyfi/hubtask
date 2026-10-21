<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Entries that look like the title being typed, named by their titles, under the field
  // (UC-AI-04 check 7). The seam every place that creates an entry mounts: the add form today, the
  // capture line and the turning of text into entries next.
  //
  // Nothing is drawn while nothing is near, and nothing at all without AI and search by meaning -
  // absence is absence (ai-first.md §2). It offers nothing to press but the entries themselves:
  // whether a near entry is the same thing is the person's to decide, and creating stays one press.

  import { Icon } from '@hubtask/design-system/components';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { followTitle, type Lookalike } from '../data/lookalikes.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The words in the field right now. */
    title: string;
  }

  const { title }: Props = $props();

  /** How long one question may take: a person is typing, and the server bounds it shorter. */
  const LOOKALIKE_TIMEOUT_MS = 5_000;

  const offered = $derived(
    manifest.value?.features?.ai_suggestions === true && manifest.value?.features?.semantic_search === true,
  );

  let found = $state<readonly Lookalike[]>([]);

  const follow = followTitle(
    async (words, signal) =>
      (
        await engine.mutate<{ items?: readonly Lookalike[] }>('POST', '/items:duplicates', { title: words }, {
          timeoutMs: LOOKALIKE_TIMEOUT_MS,
          signal,
          // A read: nothing it answers makes anything the client holds stale.
          invalidates: [],
        })
      ).items ?? [],
    (next) => (found = next),
  );

  $effect(() => {
    if (offered) follow.typed(title);
    else follow.stop();
  });
  $effect(() => () => follow.stop());
</script>

<!-- The live region stands empty while nothing is near, and is filled when something is: one
     created already filled would not be announced. -->
{#if offered}
  <div role="status">
    {#if found.length > 0}
      <div class="lookalikes" data-ai>
        <span class="mark" aria-hidden="true"><Icon name="sparkles" size="sm" /></span>
        <span class="lead">{t('app.lookalikes.lead')}</span>
        <ul>
          {#each found as lookalike (lookalike.item_id)}
            <li><a href={`/items/${lookalike.item_id}`}>{lookalike.title}</a></li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>
{/if}

<style>
  .lookalikes {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-050) var(--sp-100);
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--ai-border);
    border-radius: var(--r-sm);
    background: var(--ai-surface);
    font-size: var(--fs-075);
  }
  .mark { display: inline-flex; color: var(--ai-text); }
  .lead { color: var(--ai-text); }
  ul { display: contents; }
  li { list-style: none; overflow-wrap: anywhere; }
</style>
