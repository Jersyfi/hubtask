<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The queue of the concept's mockup as a calm table (WB-4): one region, hairlines, groups by status
  // with their count and "+ entry", a row act and ⋯ on hover, comfortable or compact remembered on
  // the device, and on a phone the stacked row with an SLA bar and the swipe.
  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import DataTable, { type DataColumn } from './DataTable.svelte';
  import Icon from './Icon.svelte';
  import ProductTile from './ProductTile.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import { TICKETS, TICKET_GROUPS, type Ticket } from './_workFixtures.ts';
  import { readDensity, writeDensity, type ListDensity } from './density.ts';

  interface Props {
    mode?: 'queue' | 'compact' | 'empty' | 'busy';
  }

  const { mode = 'queue' }: Props = $props();

  const store = typeof localStorage === 'undefined' ? undefined : localStorage;
  let density = $state<ListDensity>(readDensity(store, 'workbench-queue'));
  $effect(() => {
    if (mode === 'compact') density = 'compact';
  });
  const setDensity = (next: ListDensity) => {
    density = next;
    writeDensity(store, 'workbench-queue', next);
  };

  let tickets = $state<Ticket[]>([...TICKETS]);
  let done = $state<string[]>(['SA-10']);
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
  const complete = (ticket: Ticket) => {
    done = done.includes(ticket.key) ? done.filter((key) => key !== ticket.key) : [...done, ticket.key];
    said = `${ticket.key} ${done.includes(ticket.key) ? 'completed' : 'reopened'}`;
  };
</script>

{#snippet bar(ticket: Ticket)}
  <span class="sla" data-tone={ticket.sla.tone}>
    <span class="track"><span class="fill" data-share={Math.round(ticket.sla.share / 10) * 10}></span></span>
    <span class="words">{#if ticket.sla.tone === 'danger'}<Icon name="triangle-alert" size="sm" />{/if}{ticket.sla.words}</span>
  </span>
{/snippet}

{#snippet cell(ticket: Ticket, column: DataColumn)}
  {#if column.id === 'key'}<span class="key">{ticket.key}</span>
  {:else if column.id === 'title'}<span class="title" data-done={done.includes(ticket.key) ? '' : undefined}>{ticket.title}</span>
  {:else if column.id === 'type'}<span class="quiet"><Icon name={ticket.type === 'Incident' ? 'triangle-alert' : 'inbox'} size="sm" /> {ticket.type}</span>
  {:else if column.id === 'requester'}{ticket.requester}
  {:else if column.id === 'priority'}<span class="priority">{ticket.priority}</span>
  {:else if column.id === 'sla'}{@render bar(ticket)}
  {:else if ticket.assignee}<Avatar name={ticket.assignee} size="sm" />
  {:else}<span class="quiet">—</span>{/if}
{/snippet}

{#snippet lead(ticket: Ticket)}
  <Checkbox label={`Complete ${ticket.key}`} isLabelHidden checked={done.includes(ticket.key)} onchange={() => complete(ticket)} />
{/snippet}

{#snippet stack(ticket: Ticket)}
  <span class="meta"><span class="key"><ProductTile icon="inbox" product="desk" size="sm" />{ticket.key}</span><StateLozenge label={groupOf(ticket)} tone={toneOf(groupOf(ticket))} /><span class="priority">{ticket.priority}</span></span>
  {#if ticket.sla.tone !== 'success'}{@render bar(ticket)}{/if}
{/snippet}

{#snippet verb(ticket: Ticket)}<Button tone="subtle" size="sm" onclick={() => (said = `Opened ${ticket.key}`)}>Open</Button>{/snippet}

{#snippet groupHead(name: string)}<StateLozenge label={name} tone={toneOf(name)} icon={toneOf(name) === 'success' ? 'check' : undefined} />{/snippet}

{#snippet empty()}
  <p class="quiet empty">No requests in this queue.</p>
{/snippet}

<div class="tools">
  <div class="density" role="radiogroup" aria-label="Density">
    <button type="button" role="radio" aria-checked={density === 'comfortable'} onclick={() => setDensity('comfortable')}>Comfortable</button>
    <button type="button" role="radio" aria-checked={density === 'compact'} onclick={() => setDensity('compact')}>Compact</button>
  </div>
</div>

<DataTable
  label="Incidents & requests"
  kind="work"
  {columns}
  {rows}
  key={(ticket) => ticket.key}
  {cell}
  {lead}
  {verb}
  {stack}
  {empty}
  {density}
  isBusy={mode === 'busy'}
  group={groupOf}
  {groupHead}
  bind:foldedGroups={folded}
  foldGroupLabel={(name) => `Fold “${name}”`}
  unfoldGroupLabel={(name) => `Show “${name}”`}
  add={{ label: (name) => `Entry in “${name ?? 'Incidents & requests'}”`, onadd: (name) => (said = `Capture in ${name}`) }}
  rowLabel={(ticket) => `${ticket.key} ${ticket.title}`}
  menu={(ticket) => ({ label: `More for ${ticket.key}`, items: [{ id: 'assign', label: 'Assign to me', icon: 'user' }, { id: 'move', label: 'Move…' }] })}
  onmenu={(ticket, id) => (said = `${id} on ${ticket.key}`)}
  swipe={{ toEnd: { label: 'Complete', icon: 'check', onswipe: complete }, isToStartMenu: true }}
  sheetDismissLabel="Close"
  sheetGrabLabel="Close"
  actionsLabel="Acts"
  shownLabel={`${rows.length} of ${rows.length} shown`}
/>
<p class="quiet hint">Swipe right to complete, left for the menu - the menu does the same.</p>
<p class="quiet" role="status">{said}</p>

<style>
  .tools { display: flex; justify-content: flex-end; margin-block-end: var(--sp-150); }
  .density { display: inline-flex; gap: var(--bw-ring); padding: var(--bw-ring); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .density button { padding: var(--sp-050) var(--sp-150); border: 0; border-radius: var(--r-sm); background: transparent; color: var(--text-secondary); font: inherit; font-size: var(--fs-090); cursor: pointer; }
  .density button[aria-checked='true'] { background: var(--bg-surface); color: var(--text-primary); font-weight: var(--fw-semibold); box-shadow: var(--shadow-flat); }
  .density button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .key { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); }
  .title { font-weight: var(--fw-medium); }
  .title[data-done] { color: var(--text-subtle); text-decoration: line-through; }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }
  .priority { color: var(--text-secondary); font-size: var(--fs-075); }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .sla { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: var(--sp-1600); }
  .track { flex: 1; block-size: var(--sp-050); border-radius: var(--r-full); background: var(--bg-surface-sunken); overflow: hidden; }
  .fill { display: block; block-size: 100%; inline-size: 0; border-radius: inherit; background: var(--accent-primary); }
  .sla[data-tone='warning'] .fill { background: var(--status-warning-accent); }
  .sla[data-tone='danger'] .fill { background: var(--status-danger-accent); }
  .sla[data-tone='success'] .fill { background: var(--status-success-accent); }
  .sla[data-tone='danger'] .words { font-weight: var(--fw-semibold); }
  .words { display: inline-flex; align-items: center; gap: var(--sp-025); color: var(--text-secondary); font-size: var(--fs-075); white-space: nowrap; }
  .fill[data-share='10'] { inline-size: 10%; } .fill[data-share='20'] { inline-size: 20%; } .fill[data-share='30'] { inline-size: 30%; }
  .fill[data-share='40'] { inline-size: 40%; } .fill[data-share='50'] { inline-size: 50%; } .fill[data-share='60'] { inline-size: 60%; }
  .fill[data-share='70'] { inline-size: 70%; } .fill[data-share='80'] { inline-size: 80%; } .fill[data-share='90'] { inline-size: 90%; }
  .fill[data-share='100'] { inline-size: 100%; }
  .empty { padding: var(--sp-300); text-align: center; }
  .hint { margin-block-start: var(--sp-150); }
</style>
