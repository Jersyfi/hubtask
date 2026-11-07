<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What an erasure does, kind by kind, from the eraser's own list (user-experience.md §5.9). One
  // component for the mode note and the confirmation, so both say the same words. Given `lines`
  // it shows them; given a case and a mode it reads the preview itself, again when the mode
  // changes, and shows nothing while it reads or where the preview could not be read.

  import type { ErasureLine } from '@hubtask/sync-engine';

  import { erasureLinePhrase, privacy, type ErasureMode } from '../data/privacy.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    lines?: readonly ErasureLine[];
    requestId?: string;
    mode?: ErasureMode;
  }

  let { lines, requestId, mode }: Props = $props();

  let read = $state<readonly ErasureLine[]>([]);

  $effect(() => {
    if (lines !== undefined || requestId === undefined || mode === undefined) return;
    let current = true;
    read = [];
    void privacy.preview(requestId, mode).then((answer) => {
      if (current) read = answer?.lines ?? [];
    });
    return () => {
      current = false;
    };
  });

  const shown = $derived(lines ?? read);
</script>

{#if shown.length > 0}
  <p class="small">{t('app.privacy.lines_intro')}</p>
  <ul class="small erasure-lines" data-erasure-lines>
    {#each shown as line, index (index)}
      {@const phrase = erasureLinePhrase(line)}
      <li>{t(phrase.code, phrase.params)}</li>
    {/each}
  </ul>
{/if}

<style>
  .erasure-lines { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-050); }
</style>
