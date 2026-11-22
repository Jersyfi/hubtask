<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import StateLozenge from './StateLozenge.svelte';
  import { LOZENGE_VOCABULARIES, type LozengeVocabulary } from './lozenge.ts';

  interface Props {
    mode?: 'vocabularies' | 'emphasis' | 'long';
  }

  const { mode = 'vocabularies' }: Props = $props();

  const WORDS: Record<string, string> = {
    todo: 'To do', progress: 'In progress', done: 'Done', running: '3 h 10 min', atRisk: 'At risk', breached: 'Breached 40 min', paused: 'Paused', met: 'Met',
    onTrack: 'On track', offTrack: 'Off track', notStarted: 'Not started', healthy: 'In operation', degraded: 'Degraded', down: 'Down', unknown: 'Unknown',
    retired: 'Retired', draft: 'Draft', approved: 'Approved', live: 'Live', failing: 'Failing', on: 'On', off: 'Off', needsLook: 'Needs a look',
    planned: 'Planned', inProgress: 'In progress', released: 'Released', late: 'Late', archived: 'Archived', synced: 'Saved', pending: 'Waiting to sync',
    offline: 'Offline', refused: 'Refused',
  };
  const vocabularies = Object.keys(LOZENGE_VOCABULARIES) as LozengeVocabulary[];
</script>

{#if mode === 'vocabularies'}
  <dl class="grid">
    {#each vocabularies as vocabulary (vocabulary)}
      <dt>{vocabulary}</dt>
      <dd>
        {#each Object.keys(LOZENGE_VOCABULARIES[vocabulary]) as state (state)}
          <StateLozenge {vocabulary} {state} label={WORDS[state] ?? state} />
        {/each}
      </dd>
    {/each}
  </dl>
{:else if mode === 'emphasis'}
  <div class="rows">
    <p><StateLozenge label="Incident" icon="triangle-alert" /> <StateLozenge label="Story" icon="square-check" /> <StateLozenge label="Mine" tone="brand" icon="user" /></p>
    <p><StateLozenge vocabulary="sla" state="breached" label="Breached 40 min" emphasis="bold" /> <StateLozenge vocabulary="health" state="down" label="Down" emphasis="bold" /></p>
    <p><StateLozenge vocabulary="goal" state="atRisk" label="At risk" size="md" /> <StateLozenge vocabulary="category" state="progress" label="In progress" size="md" /></p>
    <p><StateLozenge vocabulary="category" state="progress" label="In progress" hasChevron /> <StateLozenge vocabulary="category" state="done" label="Done" hasChevron /></p>
  </div>
{:else}
  <div class="narrow">
    <StateLozenge vocabulary="sla" state="atRisk" label="Next reply due within the business hours of the requester's organisation" />
  </div>
{/if}

<style>
  .grid { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--sp-150) var(--sp-300); margin: 0; }
  dt { color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); }
  dd { display: flex; flex-wrap: wrap; gap: var(--sp-100); margin: 0; }
  .rows p { display: flex; flex-wrap: wrap; gap: var(--sp-100); margin: 0 0 var(--sp-150); }
  .narrow { inline-size: var(--sp-1600); }
</style>
