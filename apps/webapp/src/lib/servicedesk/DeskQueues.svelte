<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The handler's queues (user-experience.md §2.2, §7.4; UC-SVC-03): *Mine*, *Unassigned*,
  // *About to breach* and *All open* as tabs with their exact counts, the chosen queue's open
  // requests below, what breaks first on top. One query per queue and hub (`queues.ts`); across
  // hubs the requests stand hub by hub, each hub's in its own order, because the order is the
  // server's and a client cannot merge two orders by a moment it is not sent.
  import { untrack } from 'svelte';
  import { Banner, EmptyState, Icon, ListRow, Skeleton, Stack, Tabs } from '@hubtask/design-system/components';
  import type { ItemQueryResult } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import DueMark from '../entries/DueMark.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import EntryPlace from '../workspace/EntryPlace.svelte';
  import { DESK_QUEUES, QUEUE_SHOWN, queueQuery, queueTotal, type DeskQueue } from './queues.ts';

  interface Props {
    /** Each hub's desk collections, in the order the hubs are drawn; a hub with none is not asked. */
    desks: Readonly<Record<string, readonly string[]>>;
    /** Whether a row names its hub too: on the workspace's home, where requests of several mix. */
    withHub?: boolean;
    onnavigate: (path: string) => void;
  }
  const { desks, withHub = false, onnavigate }: Props = $props();

  let selected = $state<DeskQueue>('mine');
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
  const shown = $derived(hubIds.map((hubId) => ({ hubId, answer: answers[at(selected, hubId)] })));
  const isReading = $derived(shown.some((each) => each.answer === undefined));
  const rows = $derived(shown.flatMap((each) => each.answer?.data ?? []));
</script>

<Tabs label={t('app.desk_queues.label')} {tabs} selected={selected} onselect={(id) => (selected = id as DeskQueue)}>
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if rows.length === 0 && isReading}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if rows.length === 0}
    <EmptyState kind="settled" title={t('app.desk_queues.empty')} description={t('app.desk_queues.empty_body')} />
  {:else}
    <Stack gap="100">
      <ul class="rows">
        {#each rows as item (item.id)}
          <li>
            <ListRow href={`/items/${item.id}`} onactivate={() => onnavigate(`/items/${item.id}`)} label={item.title}>
              {#snippet leading()}<Icon name="inbox" size="sm" />{/snippet}
              <span class="name" title={item.title}>{item.title}</span>
              {#snippet trailing()}<EntryPlace collectionId={item.collection_id} {withHub} /><DueMark {item} />{/snippet}
            </ListRow>
          </li>
        {/each}
      </ul>
      {#each shown as each (each.hubId)}
        {#if each.answer && typeof each.answer.total === 'number' && each.answer.total > each.answer.data.length}
          <p class="hint">{t('app.desk_queues.partial', { shown: String(Math.min(QUEUE_SHOWN, each.answer.data.length)), total: String(each.answer.total), hub: containers.find(each.hubId)?.name ?? '' })}</p>
        {/if}
      {/each}
    </Stack>
  {/if}
</Tabs>

<style>
  .rows { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* One line with an ellipsis and the whole title as the tooltip (user-experience.md §2.5). */
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
