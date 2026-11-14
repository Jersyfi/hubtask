<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The handler's queues (user-experience.md §2.2, §7.4; UC-SVC-03): *Mine*, *Unassigned*,
  // *About to breach* and *All open* with their exact counts - a segmented choice, not a second
  // tab strip under the home's, since each is the same open requests narrowed - the chosen queue's open
  // requests below as a calm table (WB-4, scene queue) - key, request, type, requester, priority,
  // SLA, assignee - grouped by status with the count and "+ request", what breaks first on top. One query per queue and hub (`queues.ts`); across hubs the requests stand
  // hub by hub, each hub's in its own order, because the order is the server's and a client cannot
  // merge two orders by a moment it is not sent. The SLA is one read per row shown (at most
  // `QUEUE_SHOWN` a hub): the query answers no goal.
  import { untrack } from 'svelte';
  import { Avatar, Button, DataTable, DensitySwitch, EmptyState, FilterBar, Icon, PriorityMark, Radio, Skeleton, Stack, StateLozenge, type DataColumn, type Filter, type ListDensity } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { ItemQueryResult, SlaPolicyList, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { capture } from '../capture/capture.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { typeNameOf } from '../data/entrytypes.ts';
  import { items } from '../data/items.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { policiesPath } from '../data/desk.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { statusLabel, workflowFor, workflows } from '../data/workflows.svelte.ts';
  import EntryKey from '../entries/EntryKey.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { matchesSearch } from '../page/list.ts';
  import EntryPlace from '../workspace/EntryPlace.svelte';
  import { DESK_QUEUES, QUEUE_SHOWN, groupTone, groupedByStatus, priorityLevel, priorityOf, queueQuery, queueTotal, type DeskQueue } from './queues.ts';
  import SlaRemaining from './SlaRemaining.svelte';

  interface Props {
    /** Each hub's desk collections, in the order the hubs are drawn; a hub with none is not asked. */
    desks: Readonly<Record<string, readonly string[]>>;
    /** Whether a row names its hub too: on the workspace's home, where requests of several mix. */
    withHub?: boolean;
    onnavigate: (path: string) => void;
    /** Each queue's count across the hubs once every hub answered it, for the home's figure. */
    ontotals?: (totals: Partial<Record<DeskQueue, number>>) => void;
    /**
     * One queue alone, without the choice: a hub's queue page (the navigation concept §2.4). `query`
     * is a saved queue's own body, asked in place of the standard queue's.
     */
    only?: { readonly queue: DeskQueue; readonly query?: Record<string, unknown>; readonly name?: string };
  }
  const { desks, withHub = false, onnavigate, ontotals, only }: Props = $props();

  /** The queue the reader chose; until then the first that holds a request, once all answered. */
  let chosen = $state<DeskQueue | undefined>(undefined);
  let answers = $state<Record<string, ItemQueryResult>>({});
  let refusal = $state<string | undefined>(undefined);
  const at = (queue: DeskQueue, hubId: string) => `${queue}:${hubId}`;

  const asked = $derived(Object.entries(desks).filter(([, ids]) => ids.length > 0));
  const hubIds = $derived(asked.map(([hubId]) => hubId));
  const key = $derived(JSON.stringify([asked, only ?? null]));
  $effect(() => {
    const [each, alone] = JSON.parse(key) as [[string, string[]][], Props['only'] | null];
    return untrack(() => {
      const stops = each.flatMap(([hubId, ids]) =>
        (alone ? [alone.queue] : DESK_QUEUES).map((queue) =>
          engine.subscribe<ItemQueryResult>({ path: '/items:query', body: alone?.query ?? queueQuery(queue, hubId, ids) }, (next) => {
            if (next.status === 'ready') answers = { ...answers, [at(queue, hubId)]: next.data };
            if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
          }),
        ),
      );
      return () => stops.forEach((stop) => stop());
    });
  });

  const totalOf = (queue: DeskQueue) => queueTotal(Object.fromEntries(hubIds.map((hubId) => [hubId, answers[at(queue, hubId)]?.total])), hubIds);
  const tabs = $derived(
    DESK_QUEUES.map((queue) => {
      const name = t(`app.desk_queues.${queue}`);
      const total = totalOf(queue);
      return { id: queue, label: total === undefined ? name : t('app.desk_queues.with_count', { queue: name, count: String(total) }) };
    }),
  );
  const selected = $derived.by((): DeskQueue => {
    if (only) return only.queue;
    if (chosen) return chosen;
    const totals = DESK_QUEUES.map((queue) => totalOf(queue));
    if (totals.some((total) => total === undefined)) return 'mine';
    return DESK_QUEUES.find((_queue, index) => (totals[index] ?? 0) > 0) ?? 'mine';
  });
  $effect(() => {
    const totals = Object.fromEntries(DESK_QUEUES.map((queue) => [queue, totalOf(queue)]).filter(([, total]) => total !== undefined));
    untrack(() => ontotals?.(totals));
  });
  const shown = $derived(hubIds.map((hubId) => ({ hubId, answer: answers[at(selected, hubId)] })));
  const isReading = $derived(shown.some((each) => each.answer === undefined));
  let search = $state('');
  const all = $derived(shown.flatMap((each) => each.answer?.data ?? []));
  const statusName = (item: WorkItem) => {
    const status = statusOf(item);
    return status ? statusLabel(status, t) : (item.status ?? t('app.desk_queues.no_status'));
  };
  const rows = $derived(groupedByStatus(all.filter((item) => matchesSearch(search, [item.title, String(item.number)])), statusName));
  let density = $state<ListDensity>('comfortable');
  let folded = $state<string[]>([]);
  let said = $state('');
  /** The group's tone from its status's category: done reads as success, the rest as work. */
  const toneOf = (name: string) => {
    const item = rows.find((row) => statusName(row) === name);
    return groupTone(item ? statusOf(item)?.category : undefined);
  };
  const complete = async (item: WorkItem) => {
    try {
      await items.setCompleted(item.id, true, crypto.randomUUID());
      said = t('app.desk_queues.completed', { title: item.title });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  };
  const assignToMe = async (item: WorkItem) => {
    const me = actor.account?.id;
    if (!me) return;
    try {
      await items.assign(item.id, me, item.version, crypto.randomUUID());
      said = t('app.desk_queues.assigned', { title: item.title });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  };
  const menuOf = (item: WorkItem) => ({
    label: t('app.entry_menu.label', { title: item.title }),
    items: [
      { id: 'open', label: t('app.entry_menu.open'), icon: 'arrow-right' as const },
      ...(actor.account?.id && item.assignee_id !== actor.account.id ? [{ id: 'assign', label: t('app.desk_queues.assign_me'), icon: 'user' as const }] : []),
      { id: 'complete', label: t('app.entry_menu.complete'), icon: 'check' as const },
    ],
  });
  const onmenu = (item: WorkItem, id: string) => {
    if (id === 'open') onnavigate(`/items/${item.id}`);
    if (id === 'assign') void assignToMe(item);
    if (id === 'complete') void complete(item);
  };
  const typeName = (item: WorkItem) => (item.entry_type ? typeNameOf(entryTypes.all, item.entry_type, (level) => t('app.entry_types.level', { level })) : '');
  /** "+ request" at a group's end opens capture in the desk its first request stands in (C-4). */
  const deskOfGroup = (name: string | undefined) => (rows.find((row) => statusName(row) === name) ?? rows[0])?.collection_id;
  // The queue is the filter bar's first chip, its counts in the popover (a sheet on a phone):
  // never a second strip under the home's view bar (the navigation concept §4, PS-2).
  const chips = $derived<Filter[]>([{ id: 'queue', label: t('app.desk_queues.chip', { queue: tabs.find((each) => each.id === selected)?.label ?? '' }), isActive: true, isPinned: true }]);

  $effect(() => untrack(() => workflows.open()));
  // Which field names a priority is the policies' word; a member may read them (UC-SVC-02).
  const policies = untrack(() => resource<SlaPolicyList>(policiesPath));
  const priorityFields = $derived(policies.state.status === 'ready' ? policies.state.data.data.map((policy) => policy.priority_field) : []);
  $effect(() => accounts.resolve(rows.flatMap((item) => [item.created_by, item.assignee_id])));
  // One moment for every row, renewed each minute, so two rows never disagree about now.
  let now = $state(new Date());
  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 60_000);
    return () => clearInterval(timer);
  });

  function statusOf(item: WorkItem) {
    if (!item.status) return undefined;
    const collection = containers.find(item.collection_id);
    const shape = workflowFor(workflows.list, item.collection_id, collection?.parent_id ?? undefined, item.entry_type ?? 'task')?.published;
    return shape?.statuses.find((status) => status.key === item.status);
  }
  // On a phone a request is its title with its SLA under it and its status at the end; below a
  // desktop's width the key, the requester, the priority and the assignee give way (§4.4).
  const columns = $derived<DataColumn[]>([
    { id: 'key', label: t('app.desk_queues.col_key'), priority: 2 },
    { id: 'request', label: t('app.desk_queues.col_request'), priority: 1 },
    { id: 'type', label: t('app.desk_queues.col_type'), priority: 3 },
    { id: 'requester', label: t('app.desk_queues.col_requester'), priority: 3 },
    { id: 'priority', label: t('app.desk_queues.col_priority'), priority: 2 },
    { id: 'sla', label: t('app.desk_queues.col_sla'), priority: 2 },
    { id: 'assignee', label: t('app.desk_queues.col_assignee'), priority: 2, isStatus: true },
  ]);
</script>

<Stack gap="200">
  {#if !only}
    <FilterBar searchLabel={t('app.desk_queues.search')} clearSearchLabel={t('app.page.clear_search')} bind:search filters={chips}
      moreLabel={t('app.page.more_filters')} filtersLabel={t('app.page.filters', { count: '1' })} sheetTitle={t('app.desk_queues.label')}
      sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')} showLabel={t('app.page.show_results', { count: String(rows.length) })}
      countLabel={isReading ? undefined : t('app.page.found', { shown: String(rows.length), count: String(all.length) })}>
      {#snippet picker()}
        <div data-desk-queues><Radio label={t('app.desk_queues.label')} bind:value={() => selected, (next) => (chosen = next as DeskQueue)}
          options={tabs.map((each) => ({ value: each.id, label: each.label }))} /></div>
      {/snippet}
    </FilterBar>
  {/if}
  <div role="region" aria-label={only?.name ?? t(`app.desk_queues.${selected}`)}>
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if rows.length === 0 && isReading}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if rows.length === 0}
    <EmptyState kind="settled" title={t('app.desk_queues.empty')} description={t('app.desk_queues.empty_body')} />
  {:else}
    <Stack gap="100">
      <div class="tools"><DensitySwitch label={t('app.list.density')} comfortableLabel={t('app.list.density_comfortable')} compactLabel={t('app.list.density_compact')} list="desk-queue" bind:density /></div>
      <DataTable label={t('app.desk_queues.table', { queue: only?.name ?? t(`app.desk_queues.${selected}`) })} kind="work" {columns} {rows} key={(item: WorkItem) => item.id}
        {density} group={statusName} bind:foldedGroups={folded}
        foldGroupLabel={(name) => t('app.list.fold_group', { group: name })} unfoldGroupLabel={(name) => t('app.list.unfold_group', { group: name })}
        add={{ label: (name) => t('app.desk_queues.add_in', { desk: containers.find(deskOfGroup(name) ?? '')?.name ?? '' }), onadd: (name) => { const desk = deskOfGroup(name); if (desk) capture.open({ collectionId: desk }); } }}
        rowLabel={(item) => item.title} titleText={(item) => item.title} href={(item) => `/items/${item.id}`}
        menu={menuOf} {onmenu} swipe={{ toEnd: { label: t('app.entry_menu.complete'), icon: 'check', onswipe: (item) => void complete(item) }, isToStartMenu: true }}
        sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')} actionsLabel={t('app.list.acts')}>
        {#snippet groupHead(name: string)}<StateLozenge label={name} tone={toneOf(name)} />{/snippet}
        {#snippet verb(item: WorkItem)}<Button tone="subtle" size="sm" onclick={() => onnavigate(`/items/${item.id}`)}>{t('app.entry_menu.open')}</Button>{/snippet}
        {#snippet stack(item: WorkItem)}
          {@const priority = priorityOf(item, priorityFields)}
          <span class="meta"><EntryKey collection={containers.find(item.collection_id)} number={item.number} /><StateLozenge label={statusName(item)} tone={toneOf(statusName(item))} />{#if priority}<PriorityMark level={priorityLevel(priority)} label={priority} />{/if}</span>
          {#key item.id}<SlaRemaining itemId={item.id} {now} />{/key}
        {/snippet}
        {#snippet cell(item: WorkItem, column: DataColumn)}
          {#if column.id === 'key'}<EntryKey collection={containers.find(item.collection_id)} number={item.number} />
          {:else if column.id === 'request'}
            <div class="request">
              <span class="title">{item.title}</span>
              <EntryPlace collectionId={item.collection_id} {withHub} />
            </div>
          {:else if column.id === 'type'}<span class="type">{#if typeName(item)}<Icon name="inbox" size="sm" />{typeName(item)}{/if}</span>
          {:else if column.id === 'requester'}<span class="who">{accounts.nameOf(item.created_by) ?? ''}</span>
          {:else if column.id === 'priority'}
            {@const priority = priorityOf(item, priorityFields)}
            {#if priority}<PriorityMark level={priorityLevel(priority)} label={priority} />{/if}
          {:else if column.id === 'sla'}{#key item.id}<SlaRemaining itemId={item.id} {now} />{/key}
          {:else if item.assignee_id}<Avatar name={accounts.nameOf(item.assignee_id) ?? t('app.people.unnamed')} size="sm" />
          {:else}<span class="quiet">{t('app.desk_queues.nobody')}</span>{/if}
        {/snippet}
      </DataTable>
      <p class="hint swipe-hint">{t('app.list.swipe_hint')}</p>
      <p class="visually-hidden" role="status">{said}</p>
      {#each shown as each (each.hubId)}
        {#if each.answer && typeof each.answer.total === 'number' && each.answer.total > each.answer.data.length}
          <p class="hint">{t('app.desk_queues.partial', { shown: String(Math.min(QUEUE_SHOWN, each.answer.data.length)), total: String(each.answer.total), hub: containers.find(each.hubId)?.name ?? '' })}</p>
        {/if}
      {/each}
    </Stack>
  {/if}
  </div>
</Stack>

<style>
  .request { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  /* One line with an ellipsis and the whole title as the tooltip (user-experience.md §2.5), as wide
     as the request column the table gives it (scene queue). */
  .title { display: block; max-inline-size: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  /* A phone's list row has the title's whole line to itself: it wraps there, uncut. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px), where DataTable becomes list rows; a media query cannot read a custom property. */
  @media (width < 600px) {
    .title { max-inline-size: none; white-space: normal; }
  }
  .quiet { color: var(--text-secondary); }
  .who { white-space: nowrap; }
  .tools { display: flex; justify-content: flex-end; }
  .type { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); white-space: nowrap; }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .swipe-hint { display: none; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px), where DataTable becomes list rows; a media query cannot read a custom property. */
  @media (width < 600px) {
    .tools { display: none; }
    .swipe-hint { display: block; }
  }
  .visually-hidden { position: absolute; inline-size: var(--bw-hairline); block-size: var(--bw-hairline); overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
