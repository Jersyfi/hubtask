<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BlockList from './BlockList.svelte';
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import EditorPage from './EditorPage.svelte';
  import EmptyState from './EmptyState.svelte';
  import Input from './Input.svelte';
  import SearchField from './SearchField.svelte';
  import Select from './Select.svelte';
  import Refused from './_RefusedDemo.svelte';
  import { blockMenu, statuses, steps, workflows } from './_pageFixtures.ts';

  const { content = 'full', start = 'list' }: { content?: 'full' | 'empty' | 'refused'; start?: 'list' | 'detail' } = $props();

  let opened = $state<string | undefined>(undefined);
  let dialog = $state<'status' | 'step' | undefined>(undefined);
  const view = $derived<'list' | 'detail'>(opened !== undefined ? 'detail' : start);
  const isEmpty = $derived(content === 'empty');
</script>

<EditorPage
  head={{
    title: 'Workflows',
    subtitle: isEmpty ? 'How work moves: statuses and the steps between them · none yet' : 'How work moves: statuses and the steps between them · 12 workflows',
    breadcrumb: { trail: [{ id: 'admin', label: 'Administration', href: '#admin' }, { id: 'wf', label: 'Workflows' }], label: 'Where you are', expandLabel: 'Show every level' },
    primary: { label: 'New workflow', icon: 'plus', onclick: () => {} },
  }}
  nav={{ label: 'Workflows', sections: isEmpty ? [] : workflows, current: opened ?? 'standard', onselect: (id) => (opened = id), search }}
  {view}
  back={{ label: 'Workflows', onback: () => (opened = undefined) }}
  draft={isEmpty ? undefined : { label: 'Draft · 2 changes since the published version', actions: draftActions }}
  refused={content === 'refused' ? refused : undefined}
>
  {#if isEmpty}
    <EmptyState kind="unused" title="No workflow yet" description="A workflow says which statuses an entry passes and who may move it on." icon="git-branch">
      {#snippet action()}<Button tone="primary" icon="plus">New workflow</Button>{/snippet}
    </EmptyState>
  {:else}
    <header class="object-head">
      <h2 class="object-title">Standard</h2>
      <p class="object-facts">Used by Task, Story, Bug and Karte in 4 collections</p>
    </header>
    <BlockList title="Statuses" count="4" blocks={statuses} onopen={() => (dialog = 'status')} menu={(block) => blockMenu(block.name)} onmove={() => {}} gripLabel={(block) => `Move ${block.name}`} add={{ label: 'Add status', onclick: () => (dialog = 'status') }} />
    <BlockList title="Steps" count="3" blocks={steps} onopen={() => (dialog = 'step')} menu={(block) => blockMenu(block.name)} onmove={() => {}} gripLabel={(block) => `Move ${block.name}`} add={{ label: 'Add step', onclick: () => (dialog = 'step') }} />
  {/if}
</EditorPage>

<Dialog isOpen={dialog !== undefined} onClose={() => (dialog = undefined)} size={dialog === 'step' ? 'l' : 'm'} title={dialog === 'step' ? 'Step “Finish”' : 'Status “In progress”'} dismissLabel="Close" primary={{ label: 'Done', onclick: () => (dialog = undefined) }}>
  <Input label="Name" value={dialog === 'step' ? 'Finish' : 'In progress'} />
  <Select label="Category" options={[{ value: 'progress', label: 'In progress' }]} value="progress" />
  {#snippet actions()}<Button tone="subtle" onclick={() => (dialog = undefined)}>Cancel</Button>{/snippet}
</Dialog>

{#snippet search()}<SearchField label="Find a workflow" isLabelHidden placeholder="Find a workflow" clearLabel="Clear the search" size="sm" />{/snippet}
{#snippet draftActions()}
  <Button tone="subtle" size="sm">Discard</Button>
  <Button size="sm">Try a step</Button>
  <Button tone="primary" size="sm">Publish…</Button>
{/snippet}
{#snippet refused()}<Refused what="the workflows" />{/snippet}

<style>
  .object-head { display: flex; flex-direction: column; gap: var(--sp-050); }
  .object-title { margin: 0; font-family: var(--font-display); font-size: var(--fs-500); line-height: var(--lh-tight); }
  .object-facts { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
</style>
