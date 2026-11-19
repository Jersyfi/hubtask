<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The queue of the concept's mockup as a calm table (WB-4): one region, hairlines, every row one
  // line, a selection column, groups by status with their count and "+ entry", the row act and ⋯
  // in the title's cell on hover, comfortable or compact remembered on the device, and on a phone
  // the stacked row with an SLA bar and the swipe.
  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import DataTable, { type DataColumn } from './DataTable.svelte';
  import DeadlineBar from './DeadlineBar.svelte';
  import DensitySwitch from './DensitySwitch.svelte';
  import Icon from './Icon.svelte';
  import PriorityMark from './PriorityMark.svelte';
  import ProductTile from './ProductTile.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import { TICKETS, TICKET_GROUPS, type Ticket } from './_workFixtures.ts';
  import type { ListDensity } from './density.ts';

  interface Props {
    mode?: 'queue' | 'compact' | 'empty' | 'busy';
  }

  const { mode = 'queue' }: Props = $props();

  let density = $state<ListDensity>('comfortable');
  $effect(() => {
    if (mode === 'compact') density = 'compact';
  });

  let tickets = $state<Ticket[]>([...TICKETS]);
  let done = $state<string[]>(['SA-10']);
  let selected = $state<string[]>([]);
  let folded = $state<string[]>([]);
  let said = $state('');
  const rows = $derived(mode === 'empty' ? [] : tickets);

  const columns: DataColumn[] = [
    { id: 'key', label: 'Key', priority: 2 },
    { id: 'title', label: 'Request', priority: 1 },
    { id: 'type', label: 'Type', priority: 3 },
    { id: 'requester', label: 'Requester', priority: 3 },
    { id: 'priority', label: 'Priority', priority: 2 },
    { id: 'sla', label: 'SLA', priority: 2 },
    { id: 'assignee', label: 'Assignee', priority: 2, isStatus: true },
  ];
  const groupOf = (ticket: Ticket) => TICKET_GROUPS[ticket.status].label;
  const toneOf = (label: string) => Object.values(TICKET_GROUPS).find((group) => group.label === label)?.tone ?? 'neutral';
  const levelOf = (ticket: Ticket) => Number(ticket.priority.slice(1, 2)) as 1 | 2 | 3 | 4;
  const complete = (ticket: Ticket) => {
    done = done.includes(ticket.key) ? done.filter((key) => key !== ticket.key) : [...done, ticket.key];
    said = `${ticket.key} ${done.includes(ticket.key) ? 'completed' : 'reopened'}`;
  };
</script>

{#snippet bar(ticket: Ticket)}
  <DeadlineBar share={ticket.sla.share} words={ticket.sla.words} tone={ticket.sla.tone} isPaused={ticket.sla.words === 'paused'} />
{/snippet}

{#snippet cell(ticket: Ticket, column: DataColumn)}
  {#if column.id === 'key'}<span class="key">{ticket.key}</span>
  {:else if column.id === 'title'}<span class="title" data-done={done.includes(ticket.key) ? '' : undefined}>{ticket.title}</span>
  {:else if column.id === 'type'}<span class="quiet type"><Icon name={ticket.type === 'Incident' ? 'triangle-alert' : 'inbox'} size="sm" />{ticket.type}</span>
  {:else if column.id === 'requester'}<span class="small">{ticket.requester}</span>
  {:else if column.id === 'priority'}<PriorityMark level={levelOf(ticket)} label={ticket.priority} />
  {:else if column.id === 'sla'}{@render bar(ticket)}
  {:else if ticket.assignee}<Avatar name={ticket.assignee} size="sm" />
  {:else}<span class="quiet nobody" aria-label="Nobody">—</span>{/if}
{/snippet}

{#snippet stack(ticket: Ticket)}
  <span class="meta"><span class="key"><ProductTile icon="inbox" product="desk" size="sm" />{ticket.key}</span><StateLozenge label={groupOf(ticket)} tone={toneOf(groupOf(ticket))} /><PriorityMark level={levelOf(ticket)} label={ticket.priority} /></span>
  {#if ticket.sla.tone !== 'success'}{@render bar(ticket)}{/if}
{/snippet}

{#snippet verb(ticket: Ticket)}<Button tone="subtle" size="sm" onclick={() => (said = `Opened ${ticket.key}`)}>Open</Button>{/snippet}

{#snippet groupHead(name: string)}<StateLozenge label={name} tone={toneOf(name)} icon={toneOf(name) === 'success' ? 'check' : undefined} />{/snippet}

{#snippet empty()}
  <p class="quiet empty">No requests in this queue.</p>
{/snippet}

<div class="tools">
  <DensitySwitch label="Density" comfortableLabel="Comfortable" compactLabel="Compact" list="workbench-queue" bind:density />
</div>

<DataTable
  label="Incidents & requests"
  kind="work"
  {columns}
  {rows}
  key={(ticket) => ticket.key}
  {cell}
  {verb}
  {stack}
  {empty}
  {density}
  isSelecting
  bind:selected
  isBusy={mode === 'busy'}
  group={groupOf}
  {groupHead}
  bind:foldedGroups={folded}
  foldGroupLabel={(name) => `Fold “${name}”`}
  unfoldGroupLabel={(name) => `Show “${name}”`}
  add={{ label: (name) => `Entry in “${name ?? 'Incidents & requests'}”`, onadd: (name) => (said = `Capture in ${name}`) }}
  rowLabel={(ticket) => `${ticket.key} ${ticket.title}`}
  titleText={(ticket) => ticket.title}
  menu={(ticket) => ({ label: `More for ${ticket.key}`, items: [{ id: 'assign', label: 'Assign to me', icon: 'user' }, { id: 'move', label: 'Move…' }] })}
  onmenu={(ticket, id) => (said = `${id} on ${ticket.key}`)}
  swipe={{ toEnd: { label: 'Complete', icon: 'check', onswipe: complete }, isToStartMenu: true }}
  sheetDismissLabel="Close"
  sheetGrabLabel="Close"
  actionsLabel="Acts"
  shownLabel={`${rows.length} of ${rows.length} shown${selected.length > 0 ? ` · ${selected.length} selected` : ''}`}
/>
<p class="quiet hint">Swipe right to complete, left for the menu – the menu does the same.</p>
<p class="quiet" role="status">{said}</p>

<style>
  .tools { display: flex; justify-content: flex-end; margin-block-end: var(--sp-150); }
  .key { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); }
  .title { font-weight: var(--fw-medium); }
  .title[data-done] { color: var(--text-subtle); text-decoration: line-through; }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }
  .type { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .small { font-size: var(--fs-075); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    /* The stacked row shows an avatar only where someone has it (the queue's phone mockup). */
    .nobody { display: none; }
    .tools { display: none; }
  }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .empty { padding: var(--sp-300); text-align: center; }
  .hint { margin-block-start: var(--sp-150); }
</style>
