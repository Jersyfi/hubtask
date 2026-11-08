<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import EmptyState from './EmptyState.svelte';
  import PlaceTabs from './PlaceTabs.svelte';
  import ViewSwitcher from './ViewSwitcher.svelte';
  import WorkPage from './WorkPage.svelte';
  import Refused from './_RefusedDemo.svelte';

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' } = $props();
  let view = $state('chart');
  const rows = ['Kick-off · 3 Nov – 4 Nov', 'Design review · 5 Nov – 12 Nov', 'Pilot · 13 Nov – 28 Nov', 'Go-live · 1 Dec'];
</script>

<WorkPage
  head={{
    title: 'Plan',
    subtitle: content === 'empty' ? undefined : '4 entries · ends 1 Dec',
    breadcrumb: { trail: [{ id: 'hub', label: 'Produktentwicklung', href: '#hub' }, { id: 'plan', label: 'Plan' }], label: 'Where you are', expandLabel: 'Show the whole trail' },
  }}
  refused={content === 'refused' ? refused : undefined}
>
  {#snippet tabs()}
    <PlaceTabs label="Produktentwicklung" current="plan" tabs={[{ id: 'overview', label: 'Overview', href: '#overview' }, { id: 'plan', label: 'Plan', href: '#plan' }, { id: 'pages', label: 'Pages', href: '#pages' }]} />
  {/snippet}
  {#snippet toolbar()}
    <ViewSwitcher label="Views of the plan" bind:selected={view} views={[{ id: 'chart', label: 'Chart' }, { id: 'table', label: 'Table' }, { id: 'dependencies', label: 'Dependencies' }, { id: 'releases', label: 'Releases' }]} />
  {/snippet}
  {#if content === 'empty'}
    <EmptyState kind="unused" title="Nothing is planned yet" description="Give an entry a start or a due date and it appears here." />
  {:else}
    <ul class="rows" data-view={view}>
      {#each rows as row (row)}<li>{row}</li>{/each}
    </ul>
  {/if}
</WorkPage>

{#snippet refused()}<Refused what="the plan" />{/snippet}

<style>
  .rows {
    display: grid;
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .rows li { padding: var(--sp-100) var(--sp-200); }
  .rows li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
</style>
