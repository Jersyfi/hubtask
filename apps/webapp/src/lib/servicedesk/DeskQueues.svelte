<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The handler's queues (user-experience.md §2.2, §7.4; UC-SVC-03): *Mine*, *Unassigned*,
  // *About to breach* and *All open* with their exact counts - a segmented choice, not a second
  // tab strip under the home's, since each is the same open requests narrowed - the chosen queue's open
  // requests below as a table - key, request, requester, status, priority, SLA, assignee - what
  // breaks first on top. One query per queue and hub (`queues.ts`); across hubs the requests stand
  // hub by hub, each hub's in its own order, because the order is the server's and a client cannot
  // merge two orders by a moment it is not sent. The SLA is one read per row shown (at most
  // `QUEUE_SHOWN` a hub): the query answers no goal.
  import { untrack } from 'svelte';
  import { Avatar, DataTable, EmptyState, LabelChip, Skeleton, Stack, ViewSwitcher, type DataColumn } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { ItemQueryResult, SlaPolicyList, WorkItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { policiesPath } from '../data/desk.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { statusLabel, workflowFor, workflows } from '../data/workflows.svelte.ts';
  import EntryKey from '../entries/EntryKey.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import EntryPlace from '../workspace/EntryPlace.svelte';
  import { DESK_QUEUES, QUEUE_SHOWN, priorityOf, queueQuery, queueTotal, type DeskQueue } from './queues.ts';
  import SlaRemaining from './SlaRemaining.svelte';

  interface Props {
    /** Each hub's desk collections, in the order the hubs are drawn; a hub with none is not asked. */
    desks: Readonly<Record<string, readonly string[]>>;
    /** Whether a row names its hub too: on the workspace's home, where requests of several mix. */
    withHub?: boolean;
    onnavigate: (path: string) => void;
    /** Each queue's count across the hubs once every hub answered it, for the home's figure. */
    ontotals?: (totals: Partial<Record<DeskQueue, number>>) => void;
  }
  const { desks, withHub = false, onnavigate, ontotals }: Props = $props();

  /** The queue the reader chose; until then the first that holds a request, once all answered. */
  let chosen = $state<DeskQueue | undefined>(undefined);
  let answers = $state<Record<string, ItemQueryResult>>({});
  let refusal = $state<string | undefined>(undefined);
  const at = (queue: DeskQueue, hubId: string) => `${queue}:${hubId}`;

  const asked = $derived(Object.entries(desks).filter(([, ids]) => ids.length > 0));
  const hubIds = $derived(asked.map(([hubId]) => hubId));
  const key = $derived(JSON.stringify(asked));
  $effect(() => {
    const each = JSON.parse(key) as [string, string[]][];
    return untrack(() => {
      const stops = each.flatMap(([hubId, ids]) =>
        DESK_QUEUES.map((queue) =>
          engine.subscribe<ItemQueryResult>({ path: '/items:query', body: queueQuery(queue, hubId, ids) }, (next) => {
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
  const rows = $derived(shown.flatMap((each) => each.answer?.data ?? []));

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
    { id: 'key', label: t('app.desk_queues.col_key'), priority: 3 },
    { id: 'request', label: t('app.desk_queues.col_request'), priority: 1 },
    { id: 'requester', label: t('app.desk_queues.col_requester'), priority: 3 },
    { id: 'status', label: t('app.desk_queues.col_status'), priority: 1, isStatus: true },
    { id: 'priority', label: t('app.desk_queues.col_priority'), priority: 3 },
    { id: 'sla', label: t('app.desk_queues.col_sla'), priority: 2 },
    { id: 'assignee', label: t('app.desk_queues.col_assignee'), priority: 3 },
  ]);
  const open = (event: MouseEvent, item: WorkItem) => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return;
    event.preventDefault();
    onnavigate(`/items/${item.id}`);
  };
</script>

<Stack gap="200">
  <div class="queues"><ViewSwitcher label={t('app.desk_queues.label')} views={tabs} {selected} onselect={(id) => (chosen = id as DeskQueue)} /></div>
  <div role="region" aria-label={t(`app.desk_queues.${selected}`)}>
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if rows.length === 0 && isReading}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if rows.length === 0}
    <EmptyState kind="settled" title={t('app.desk_queues.empty')} description={t('app.desk_queues.empty_body')} />
  {:else}
    <Stack gap="100">
      <DataTable label={t('app.desk_queues.table', { queue: t(`app.desk_queues.${selected}`) })} {columns} {rows} key={(item: WorkItem) => item.id}>
        {#snippet cell(item: WorkItem, column: DataColumn)}
          {#if column.id === 'key'}<EntryKey collection={containers.find(item.collection_id)} number={item.number} />
          {:else if column.id === 'request'}
            <div class="request">
              <a class="title" href={`/items/${item.id}`} title={item.title} onclick={(event) => open(event, item)}>{item.title}</a>
              <EntryPlace collectionId={item.collection_id} {withHub} />
            </div>
          {:else if column.id === 'requester'}<span class="who">{accounts.nameOf(item.created_by) ?? ''}</span>
          {:else if column.id === 'status'}
            {@const status = statusOf(item)}
            {#if status}<LabelChip name={statusLabel(status, t)} colorToken={status.color_token} />{:else if item.status}{item.status}{/if}
          {:else if column.id === 'priority'}{priorityOf(item, priorityFields) ?? ''}
          {:else if column.id === 'sla'}{#key item.id}<SlaRemaining itemId={item.id} {now} />{/key}
          {:else if item.assignee_id}<Avatar name={accounts.nameOf(item.assignee_id) ?? t('app.people.unnamed')} size="sm" />
          {:else}<span class="quiet">{t('app.desk_queues.nobody')}</span>{/if}
        {/snippet}
      </DataTable>
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
  /* Four queues with their counts are wider than a phone: the choice scrolls, the page does not. */
  .queues { overflow-x: auto; }
  .request { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  /* One line with an ellipsis and the whole title as the tooltip (user-experience.md §2.5), short
     enough that the six columns beside it fit at a desktop's width rather than behind the edge fade. */
  .title { display: block; max-inline-size: 36ch; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .title:hover { text-decoration: underline; }
  /* A phone's list row has the title's whole line to itself: it wraps there, uncut. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px), where DataTable becomes list rows; a media query cannot read a custom property. */
  @media (width < 600px) {
    .title { max-inline-size: none; white-space: normal; }
  }
  .quiet { color: var(--text-secondary); }
  .who { white-space: nowrap; }
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
