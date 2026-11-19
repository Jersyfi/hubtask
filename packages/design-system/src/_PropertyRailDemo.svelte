<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import LabelChip from './LabelChip.svelte';
  import Property from './Property.svelte';
  import PropertyRail, { type RailSection } from './PropertyRail.svelte';
  import DeadlineList from './DeadlineList.svelte';
  import PriorityMark from './PriorityMark.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import { TICKET_DEADLINES } from './_workFixtures.ts';

  interface Props {
    mode?: 'story' | 'ticket' | 'empty';
  }

  const { mode = 'story' }: Props = $props();

  const STORY: RailSection[] = [
    { id: 'details', title: 'Details' },
    { id: 'acceptance', title: 'Acceptance' },
    { id: 'planning', title: 'Planning' },
    { id: 'rating', title: 'Rating' },
    { id: 'time', title: 'Time', tile: { icon: 'clock', product: 'time' } },
    { id: 'more', title: 'More fields', count: '5', isFolded: true },
    { id: 'code', title: 'Code', count: '3', isFolded: true },
    { id: 'goals', title: 'Goals', count: '1', isFolded: true },
  ];
  const TICKET: RailSection[] = [
    { id: 'deadlines', title: 'Deadlines', count: '1 breached' },
    { id: 'triage', title: 'Triage' },
    { id: 'details', title: 'Details' },
    { id: 'requester', title: 'Requester', tile: { icon: 'inbox', product: 'desk' } },
    { id: 'approvals', title: 'Approvals', isEmpty: true, addLabel: 'Ask for approval' },
    { id: 'time', title: 'Time', tile: { icon: 'clock', product: 'time' } },
    { id: 'more', title: 'More fields', count: '5', isFolded: true },
  ];
  const EMPTY: RailSection[] = [
    { id: 'details', title: 'Details' },
    { id: 'goals', title: 'Goal', isEmpty: true, addLabel: 'Link a goal', tile: { icon: 'flag', product: 'goals' } },
    { id: 'links', title: 'Affected items', isEmpty: true, addLabel: 'Name an item', tile: { icon: 'shield', product: 'assets' } },
    { id: 'time', title: 'Time', isEmpty: true, addLabel: 'Log time', tile: { icon: 'clock', product: 'time' } },
  ];
  const sections = $derived(mode === 'ticket' ? TICKET : mode === 'empty' ? EMPTY : STORY);
  let added = $state('');
</script>

{#snippet person(name: string)}<span class="who"><Avatar {name} size="sm" />{name}</span>{/snippet}

{#snippet section(item: RailSection)}
  {#if item.id === 'details' && mode === 'empty'}
    <Property label="Assignee" emptyLabel="Nobody – name someone" requiredLabel="required" />
    <Property label="Dates" emptyLabel="None" />
  {:else if item.id === 'details' && mode === 'ticket'}
    <Property label="Assignee">{@render person('Amelia Hughes')}</Property>
    <Property label="Type"><span><StateLozenge label="Incident" icon="triangle-alert" /></span></Property>
    <Property label="Channel" value="Portal" />
    <Property label="Labels"><span><LabelChip name="customer-portal" colorToken="teal" /></span></Property>
  {:else if item.id === 'details'}
    <Property label="Assignee">{@render person('Amelia Hughes')}</Property>
    <Property label="Dates" value="3 – 27 Nov, 17:00" />
    <Property label="Labels"><span class="chips"><LabelChip name="Accessibility" colorToken="teal" /><LabelChip name="Customer wish" colorToken="blue" /><LabelChip name="Security" colorToken="red" /></span></Property>
    <Property label="Target version" value="4.3.0" />
  {:else if item.id === 'acceptance'}
    <Property label="Acceptance date" value="28 Nov 2026" />
    <Property label="Accepted by">{@render person('Clara Neumann')}</Property>
  {:else if item.id === 'planning'}
    <Property label="Story points" value="8" />
    <Property label="Effort" value="40:00 h" />
    <Property label="Budget" value="€18,500" />
  {:else if item.id === 'rating'}
    <Property label="Impact" value="High" />
    <Property label="Urgency" value="Medium" />
    <Property label="Security relevant" value="Yes" />
  {:else if item.id === 'time'}
    <div class="time"><Button tone="secondary" size="sm" icon="play">Timer</Button><span class="note">31:20 of 40:00 h logged</span></div>
  {:else if item.id === 'deadlines'}
    <DeadlineList deadlines={TICKET_DEADLINES} keptLabel="First reply: 3 of 3 kept" table={{ label: 'All deadlines as a table', onclick: () => (added = 'deadline matrix') }} />
  {:else if item.id === 'triage'}
    <Property label="Impact" value="High" note="many people" editLabel="Change impact" onedit={() => (added = 'impact')} />
    <Property label="Urgency" value="High" note="work stands still" editLabel="Change urgency" onedit={() => (added = 'urgency')} />
    <Property label="Priority" value="P2 High" note="from impact × urgency" editLabel="Change priority" onedit={() => (added = 'priority')}><PriorityMark level={2} label="P2 High" /></Property>
    <Property label="Group" value="Platform" note="since 21:14 · before 1st level" editLabel="Change group" onedit={() => (added = 'group')} />
  {:else if item.id === 'requester'}
    <div class="requester">
      <Avatar name="Hannah Albers" size="md" />
      <span class="who-lines"><b>Hannah Albers</b><span class="note">Stadtwerke Lüneburg GmbH · shares requests</span></span>
    </div>
    <p class="note line">2 more open requests · owns 1 item</p>
  {:else}
    <Property label="Field" value="Value" />
  {/if}
{/snippet}

{#snippet summary()}
  <Property label="Assignee">{@render person('Amelia Hughes')}</Property>
  <Property label="Dates" value="3 – 27 Nov" />
  <Property label="Story points" value="8" />
{/snippet}

<div class="column">
  <PropertyRail
    label="Properties"
    {sections}
    {section}
    onadd={(id) => (added = id)}
    foldLabel={(title) => `Fold ${title}`}
    unfoldLabel={(title) => `Show ${title}`}
    allLabel="All properties (18)"
    {summary}
    closeLabel="Close"
    grabLabel="Close"
  />
  {#if added}<p class="said" role="status">Would add to “{added}”.</p>{/if}
</div>

<style>
  .column { inline-size: 100%; max-inline-size: var(--layout-rail); }
  .who { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .requester { display: flex; align-items: center; gap: var(--sp-100); }
  .who-lines { display: flex; flex-direction: column; min-inline-size: 0; font-size: var(--fs-100); }
  .line { margin: var(--sp-050) 0 0; }
  .time { display: flex; align-items: center; gap: var(--sp-100); }
  .note, .said { color: var(--text-subtle); font-size: var(--fs-075); }
</style>
