<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import PriorityMark from './PriorityMark.svelte';
  import Breadcrumb from './Breadcrumb.svelte';
  import IconButton from './IconButton.svelte';
  import ObjectHead from './ObjectHead.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import StatusButton from './StatusButton.svelte';
  import { IN_PROGRESS, STORY_STATUSES, STORY_STEPS } from './_workFixtures.ts';
  import { lozengeLook } from './lozenge.ts';
  import type { StatusRef, WorkflowStep } from './steps.ts';

  interface Props {
    mode?: 'grammar' | 'story' | 'ticket' | 'empty' | 'readonly' | 'long';
  }

  const { mode = 'grammar' }: Props = $props();

  let title = $state('Pay with Apple Pay and Google Pay in the app’s checkout');
  const NEW: StatusRef = { id: 'new', label: 'New', category: 'todo' };
  const START: WorkflowStep[] = [{ id: 'start', label: 'Start', to: IN_PROGRESS, isNext: true }];
  const menu = { label: 'More for TAMA-18', items: [{ id: 'copy', label: 'Copy link', icon: 'link' as const }, { id: 'move', label: 'Move…' }, { id: 'delete', label: 'Move to trash', icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true }], onselect: () => {} };
  const crumbs = [{ id: 'hub', label: 'Product development' }, { id: 'col', label: 'Team Atlas – Mobile app' }];
</script>

{#snippet stepButton(current: StatusRef, steps: readonly WorkflowStep[])}
  <StatusButton {current} {steps} label={`Status: ${current.label}. Change status`} menuTitle={`From “${current.label}” on`} closeLabel="Close" />
{/snippet}

{#snippet storyEnd()}
  <IconButton icon="star" label="Star" tone="secondary" size="sm" />
  <IconButton icon="link" label="Copy link" tone="secondary" size="sm" />
{/snippet}

{#snippet points()}<PriorityMark level={2} label="P2" /><span>8 points</span>{/snippet}

{#if mode === 'grammar'}
  <div class="grammar">
    <section class="card">
      <ObjectHead isCompact objectKey={{ label: 'SA-1', icon: 'inbox', product: 'desk' }} type={{ label: 'Incident', icon: 'triangle-alert' }} title="Mail to customers arrives an hour late" titleLabel="Title" next={{ label: 'Start', onclick: () => {} }} deadline={{ label: 'First reply in 1 h 40 min', tone: 'warning' }}>
        {#snippet status()}<StatusButton current={NEW} steps={START} label="Status: New" menuTitle="From “New” on" closeLabel="Close" />{/snippet}
      </ObjectHead>
    </section>
    <section class="card">
      <ObjectHead isCompact objectKey={{ label: 'TAMA-1', icon: 'square-check' }} type={{ label: 'Story' }} title="As a customer I want to sign in with a passkey" titleLabel="Title" next={{ label: 'Request review', onclick: () => {} }} person={{ name: 'Amelia Hughes' }} due={{ label: '20 Nov' }}>
        {#snippet status()}{@render stepButton(IN_PROGRESS, STORY_STEPS)}{/snippet}
      </ObjectHead>
    </section>
    <section class="card">
      <ObjectHead isCompact objectKey={{ label: 'Application service', icon: 'shield', product: 'assets' }} title="Portal API" titleLabel="Name" next={{ label: 'Confirm', onclick: () => {} }} due={{ label: 'Review 9 Feb 2027' }}>
        {#snippet status()}<StatusButton current={{ id: 'op', label: 'In operation', category: 'done' }} look={lozengeLook('health', 'healthy')} steps={[]} label="State: In operation" menuTitle="From “In operation” on" closeLabel="Close" />{/snippet}
      </ObjectHead>
    </section>
    <section class="card">
      <ObjectHead isCompact objectKey={{ label: 'Goal · H2 2026', icon: 'flag', product: 'goals' }} title="App rating of 4.6 stars in both stores" titleLabel="Title" next={{ label: 'Check in', onclick: () => {} }} person={{ name: 'Amelia Hughes' }}>
        {#snippet status()}<StatusButton current={{ id: 'risk', label: 'At risk', category: 'progress' }} look={lozengeLook('goal', 'atRisk')} steps={[]} label="Confidence: At risk" menuTitle="Confidence" closeLabel="Close" />{/snippet}
        {#snippet facts()}<span>Confidence 5/10</span>{/snippet}
      </ObjectHead>
    </section>
    <section class="card">
      <ObjectHead isCompact objectKey={{ label: 'Agent', icon: 'zap', product: 'agents' }} title="Ticket pre-sorting" titleLabel="Name" next={{ label: 'Go live', onclick: () => {} }}>
        {#snippet status()}<StatusButton current={{ id: 'ok', label: 'Approved', category: 'done' }} look={lozengeLook('agent', 'approved')} steps={[]} label="State: Approved" menuTitle="From “Approved” on" closeLabel="Close" />{/snippet}
        {#snippet facts()}<span>200 000 tokens a day · 0 % used</span>{/snippet}
      </ObjectHead>
    </section>
    <section class="card">
      <ObjectHead isCompact objectKey={{ label: 'Release', icon: 'calendar', product: 'plans' }} title="App 4.2 “Keyless”" titleLabel="Name" next={{ label: 'Release', onclick: () => {} }} due={{ label: '25 Nov' }}>
        {#snippet status()}<StatusButton current={{ id: 'planned', label: 'Planned', category: 'todo' }} steps={[]} label="State: Planned" menuTitle="From “Planned” on" closeLabel="Close" />{/snippet}
        {#snippet facts()}<span>1 of 3 done</span>{/snippet}
      </ObjectHead>
    </section>
  </div>
{:else if mode === 'story' || mode === 'long'}
  <ObjectHead
    objectKey={{ label: 'TAMA-18', icon: 'square-check' }}
    type={{ label: 'Story', icon: 'square-check' }}
    meta="Backlog · target version 4.3.0 · created 3 Nov by Clara Neumann"
    title={mode === 'long' ? 'Pay with Apple Pay and Google Pay in the app’s checkout, including the fallback to the card form when no wallet is set up and the receipt in the customer’s account' : title}
    titleLabel="Title"
    editTitleLabel="Edit title"
    onrename={(next) => (title = next)}
    next={{ label: 'Request review', onclick: () => {} }}
    person={{ name: 'Amelia Hughes' }}
    due={{ label: '3 – 27 Nov' }}
    facts={points}
    end={storyEnd}
    {menu}
  >
    {#snippet trail()}<Breadcrumb label="Where TAMA-18 lives" expandLabel="Show the whole path" trail={crumbs} />{/snippet}
    {#snippet status()}{@render stepButton(IN_PROGRESS, STORY_STEPS)}{/snippet}
  </ObjectHead>
{:else if mode === 'ticket'}
  <ObjectHead
    objectKey={{ label: 'SA-12', icon: 'inbox', product: 'desk' }}
    type={{ label: 'Incident', icon: 'triangle-alert' }}
    meta="through the portal · 11 Nov, 07:52"
    title="Customer portal: sign-in fails with a gateway timeout – every dispatcher in Lüneburg affected"
    titleLabel="Title"
    editTitleLabel="Edit title"
    onrename={() => {}}
    next={{ label: 'Solve', onclick: () => {} }}
    secondary={{ label: 'Wait for Hannah', onclick: () => {} }}
    person={{ name: 'Amelia Hughes' }}
    deadline={{ label: '1 deadline breached · next in 1 h 05', tone: 'danger' }}
    end={storyEnd}
    menu={{ ...menu, label: 'More for SA-12' }}
  >
    {#snippet status()}{@render stepButton(IN_PROGRESS, STORY_STEPS)}{/snippet}
    {#snippet facts()}<PriorityMark level={2} label="P2 High" />{/snippet}
  </ObjectHead>
{:else if mode === 'empty'}
  <ObjectHead objectKey={{ label: 'TAMA-31', icon: 'square-check' }} type={{ label: 'Task' }} title="Untitled entry" titleLabel="Title" editTitleLabel="Edit title" onrename={() => {}} person={{ emptyLabel: 'Nobody – name someone' }} next={{ label: 'Done', icon: 'check', onclick: () => {} }}>
    {#snippet status()}<StatusButton current={STORY_STATUSES[1]!} steps={[]} label="Status: Ready" menuTitle="From “Ready” on" closeLabel="Close" />{/snippet}
  </ObjectHead>
  <ObjectHead objectKey={{ label: 'TAMA-32', icon: 'square-check' }} type={{ label: 'Task' }} title="Order the tiles" titleLabel="Title" isDone person={{ emptyLabel: 'Nobody – name someone' }} secondary={{ label: 'Reopen', icon: 'undo-2', onclick: () => {} }}>
    {#snippet status()}<StatusButton current={{ id: 'done', label: 'Done', category: 'done' }} steps={[]} label="Status: Done" menuTitle="Status" closeLabel="Close" />{/snippet}
  </ObjectHead>
{:else}
  <ObjectHead objectKey={{ label: 'TAMA-18', icon: 'square-check' }} type={{ label: 'Story' }} {title} titleLabel="Title" person={{ name: 'Amelia Hughes' }} due={{ label: '3 – 27 Nov' }}>
    {#snippet status()}<StatusButton current={IN_PROGRESS} steps={STORY_STEPS} label="Status" menuTitle="" disabledReason="Read only: you are a guest in Team Atlas." closeLabel="Close" />{/snippet}
  </ObjectHead>
{/if}


<style>
  .grammar { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-dialog-m)), 1fr)); gap: var(--sp-200); }
  .card { padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
</style>
