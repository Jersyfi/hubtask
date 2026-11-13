<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The line over the content (scene viewcfg, §6.11): what is seen as chips, each removable, in a
  // collection and in the plan alike. A chip says names where its piece holds an identifier (Q-2).
  import { IconButton } from '@hubtask/design-system/components';
  import { segments } from '@hubtask/query';

  import { t } from '../i18n/i18n.svelte.ts';
  import { spokenPiece, type Names } from '../data/viewquery.ts';

  interface Props {
    line: string;
    onchange: (line: string) => void;
    names?: Names;
  }

  const { line, onchange, names }: Props = $props();
  const pieces = $derived(segments(`${line} `).pieces);
</script>

{#if pieces.length > 0}
  <ul class="query-chips" aria-label={t('app.viewcfg.query')} data-query-chips>
    {#each pieces as piece, index (index)}
      {@const spoken = names ? spokenPiece(piece, names) : piece}
      <li><span>{spoken}</span><IconButton icon="x" size="sm" label={t('app.viewcfg.remove_chip', { spelling: spoken })} onclick={() => onchange(pieces.filter((_, at) => at !== index).join(' '))} /></li>
    {/each}
  </ul>
{/if}

<style>
  .query-chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; min-inline-size: 0; }
  .query-chips li { display: inline-flex; align-items: center; gap: var(--sp-025); padding-inline-start: var(--sp-100); font-size: var(--fs-075); background: var(--bg-surface); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); max-inline-size: 100%; }
  .query-chips span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
