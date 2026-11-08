<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import DetailPage from './DetailPage.svelte';
  import EmptyState from './EmptyState.svelte';
  import Refused from './_RefusedDemo.svelte';

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' } = $props();
  const details = [['Assignee', 'Mara Lindqvist'], ['Due', '14 Nov'], ['Labels', 'Pilot, Customer'], ['Estimate', '3 days']];
  const steps = ['Collect the pilot sites', 'Agree the schedule', 'Brief the support team'];
</script>

<DetailPage
  head={{
    title: 'Prepare the pilot',
    subtitle: content === 'empty' ? undefined : 'Task · Produktentwicklung › Roadmap',
    breadcrumb: { trail: [{ id: 'hub', label: 'Produktentwicklung', href: '#hub' }, { id: 'collection', label: 'Roadmap', href: '#collection' }, { id: 'entry', label: 'Prepare the pilot' }], label: 'Where you are', expandLabel: 'Show the whole trail' },
  }}
  details={content === 'full' ? detailRows : undefined}
  refused={content === 'refused' ? refused : undefined}
>
  {#if content === 'empty'}
    <EmptyState kind="unused" title="No notes and no steps yet" description="Write what this is about, or add a step under it." />
  {:else}
    <p class="notes">Three sites, one week each, the support team briefed before the first.</p>
    <ul class="region">
      {#each steps as step (step)}<li>{step}</li>{/each}
    </ul>
  {/if}
</DetailPage>

{#snippet detailRows()}
  <dl class="region">
    {#each details as [label, value] (label)}<div><dt>{label}</dt><dd>{value}</dd></div>{/each}
  </dl>
{/snippet}

{#snippet refused()}<Refused what="this entry" />{/snippet}

<style>
  .notes { margin: 0; max-inline-size: 64ch; }
  .region {
    display: grid;
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .region li, .region div { padding: var(--sp-100) var(--sp-200); }
  .region li + li, .region div + div { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .region div { display: flex; justify-content: space-between; gap: var(--sp-200); }
  dt { color: var(--text-subtle); }
  dd { margin: 0; }
</style>
