<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Upkeep* (work-surface §6.10, IN-4): the former sub-pages in one place - runs, duplicates,
  // connectors, rounds and the import -, each a part with its own address kept, the tab scrolled to
  // the part an address names. Only the parts the reader may run are drawn (P-05).
  import { tick, untrack } from 'svelte';
  import { Stack } from '@hubtask/design-system/components';
  import type { CiClass } from '@hubtask/sync-engine';

  import type { CarePart, Words } from '../data/cmdb.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import Campaigns from './Campaigns.svelte';
  import Connectors from './Connectors.svelte';
  import Duplicates from './Duplicates.svelte';
  import ImportFile from './ImportFile.svelte';
  import Runs from './Runs.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    /** The parts the reader may run, in their order. */
    parts: readonly CarePart[];
    /** The part the address names: scrolled to once drawn. */
    part?: CarePart;
    /** The head asked for a connector's or a round's dialog; `onasked` gives the ask back. */
    asked?: 'connector' | 'campaign';
    onasked?: () => void;
    /** An import finished: where its objects are. */
    onimported?: () => void;
  }

  const { hubId, classes, words, mayWrite, parts, part, asked, onasked, onimported }: Props = $props();

  $effect(() => {
    const at = part;
    if (!at) return;
    untrack(() => {
      void tick().then(() => document.getElementById(`care-${at}`)?.scrollIntoView({ block: 'start' }));
    });
  });
</script>

<Stack gap="300">
  {#each parts as each (each)}
    <section id={`care-${each}`} class="part" aria-labelledby={`care-${each}-heading`} data-care-part={each}>
      <h2 id={`care-${each}-heading`} class="heading">{t(`app.assets.section.${each}`)}</h2>
      {#if each === 'runs'}
        <Runs {hubId} {classes} {words} {mayWrite} />
      {:else if each === 'duplicates'}
        <Duplicates {hubId} {classes} {words} {mayWrite} />
      {:else if each === 'connectors'}
        <Connectors {hubId} {classes} {words} {mayWrite} isAsked={asked === 'connector'} {onasked} />
      {:else if each === 'campaigns'}
        <Campaigns {hubId} {classes} {words} {mayWrite} isAsked={asked === 'campaign'} {onasked} />
      {:else if each === 'import'}
        <ImportFile {hubId} {classes} {words} onopen={onimported} />
      {/if}
    </section>
  {/each}
</Stack>

<style>
  .part { display: grid; gap: var(--sp-100); scroll-margin-block-start: var(--sp-400); }
  .heading { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
</style>
