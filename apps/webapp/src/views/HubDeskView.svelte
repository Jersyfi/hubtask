<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's queue (the navigation concept §2.4, UC-SVC-03 check 10): the sidebar's *Queues* opens
  // one queue as a table over every desk of the hub, named by `?queue=` - a saved view marked as a
  // queue, or a standard queue's key. Without one the address takes the hub's first queue. A hub
  // with no desk says how a collection becomes one and offers whoever shapes the hub the ITIL
  // desk's plan at its *Products*; anyone else is told whom to ask (UX-5, §5.5). *Filter* narrows
  // the queue in place; once it does, whoever shapes the hub may keep it as a queue of its own
  // (*Save as queue*, a hub-scoped view marked as one, §5.1); a saved queue is starred and deleted
  // from the head.
  import ToolHead from '../lib/workspace/ToolHead.svelte';
  import { untrack } from 'svelte';
  import type { HubQueueList, SavedView } from '@hubtask/sync-engine';
  import { engine } from '../lib/data/engine.ts';
  import { DESK_QUEUES, QUEUE_SHOWN, desksOf, queueQuery, type DeskQueue } from '../lib/servicedesk/queues.ts';
  import { queuePath } from '../lib/sidebar.ts';
  import { Button, Dialog, Input, Skeleton, Stack, WorkPage, type PageAction } from '@hubtask/design-system/components';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { ITIL_TEMPLATE } from '../lib/servicedesk/desk.ts';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { entryTypes } from '../lib/data/entrytypes.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import DeskQueues from '../lib/servicedesk/DeskQueues.svelte';
  import QueryPanel from '../lib/entries/QueryPanel.svelte';
  import type { ItemsQuery } from '../lib/data/items.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import StarToggle from '../lib/workspace/StarToggle.svelte';

  interface Props {
    hubId: string;
    /** The queue the address names: a saved view's id or a standard queue's key. */
    queue?: string;
    onnavigate: (path: string) => void;
    /** Where the address is replaced: the hub's first queue when it names none. */
    onreplace?: (path: string) => void;
  }
  const { hubId, queue, onnavigate, onreplace }: Props = $props();

  const hub = $derived(containers.find(hubId));

  // The hub's queues, one read, as the sidebar reads them.
  let queues = $state<HubQueueList['data'] | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() => engine.subscribe<HubQueueList>({ path: `/hubs/${at}/queues` }, (next) => {
      if (next.status === 'ready') queues = next.data.data;
      if (next.status === 'failed') queues = [];
    }));
  });
  const chosen = $derived(queues?.find((each) => each.view_id === queue || (each.view_id === null && each.key === queue)) ?? queues?.find((each) => each.key === queue));
  $effect(() => {
    const first = queues?.[0];
    if (!first || chosen) return;
    untrack(() => (onreplace ?? onnavigate)(queuePath(hubId, { viewId: first.view_id, key: first.key })));
  });
  // A saved queue is asked as opening it asks it; a standard one, and a saved one named as a
  // standard one (the ITIL desk's), as the desk's queues are.
  const STANDARD: Readonly<Record<string, DeskQueue>> = { mine: 'mine', unassigned: 'unassigned', breaching: 'breaching', about_to_breach: 'breaching', open: 'open' };
  let saved = $state<SavedView | undefined>(undefined);
  $effect(() => {
    const id = chosen?.view_id;
    saved = undefined;
    if (!id) return;
    return untrack(() => engine.subscribe<SavedView>({ path: `/views/${id}` }, (next) => {
      if (next.status === 'ready') saved = next.data;
    }));
  });
  const queueName = $derived(chosen ? (chosen.name ?? ((DESK_QUEUES as readonly string[]).includes(chosen.key) ? t(`app.desk_queues.${chosen.key}`) : chosen.key)) : t('app.nav.tool_queues'));
  const only = $derived.by(() => {
    // No queue answered - the read refused, or none kept yet -: the desk's own queue the address
    // names, else its open requests, rather than a page that waits for ever.
    if (!chosen && queues?.length === 0) {
      const fallback = STANDARD[queue ?? ''] ?? 'open';
      return narrowed({ queue: fallback, name: t(`app.desk_queues.${fallback}`), query: queueQuery(fallback, hubId, deskIds) });
    }
    if (!chosen) return undefined;
    const standard = STANDARD[chosen.key];
    if (standard) return narrowed({ queue: standard, name: queueName, query: queueQuery(standard, hubId, deskIds) });
    if (!saved) return undefined;
    const asked = saved.query as Record<string, unknown>;
    return narrowed({
      queue: 'open' as const,
      name: queueName,
      query: {
        scope: { container_id: (asked['scope_container_id'] as string | undefined) ?? hubId, include_descendants: true },
        ...(asked['filter'] ? { filter: asked['filter'] } : {}),
        sort: asked['sort'] ?? [{ field: 'sla_due', dir: 'ASC', nulls: 'LAST' }],
        page: { size: QUEUE_SHOWN },
        count: 'exact',
      },
    });
  });

  // *Filter*: the reader's conditions joined to the queue's own; the queue's order stays.
  let isFiltering = $state(false);
  let narrowing = $state<ItemsQuery['filter']>(undefined);
  let filterCount = $state(0);
  function narrowed<T extends { query?: Record<string, unknown> }>(each: T): T {
    if (!narrowing || !each.query) return each;
    const own = each.query['filter'] as { op?: string; nodes?: unknown[] } | undefined;
    const nodes = own ? (own.op === 'AND' && own.nodes ? [...own.nodes, narrowing] : [own, narrowing]) : [narrowing];
    return { ...each, query: { ...each.query, filter: { op: 'AND', nodes } } };
  }

  // *Save as queue*: offered once the filter differs from the queue, to whoever shapes the hub.
  let isSaving = $state(false);
  let isNaming = $state(false);
  let newName = $state('');
  let failure = $state<string | undefined>(undefined);
  async function saveQueue() {
    const asked = only?.query;
    if (!asked || isSaving) return;
    if (newName.trim() === '') return;
    isSaving = true;
    failure = undefined;
    try {
      const made = await views.createQueue(hubId, {
        name: newName.trim(),
        layout: 'LIST_COLLAPSED',
        sharing: 'SCOPE',
        query: { scope_container_id: hubId, filter: asked['filter'], sort: asked['sort'] },
      }, crypto.randomUUID());
      isNaming = false;
      isFiltering = false;
      narrowing = undefined;
      // Known here before the hub's queues are read again, so its address is not taken for an
      // unknown one and sent to the first queue; the read brings its count.
      queues = [...(queues ?? []), { view_id: made.id, key: made.key, name: made.name, count: 0, at_risk: 0 }];
      onnavigate(queuePath(hubId, { viewId: made.id, key: made.key }));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  const headActions = $derived.by((): readonly [] | readonly [PageAction] | readonly [PageAction, PageAction] => {
    if (desks.length === 0) return [];
    const filter: PageAction = { label: filterCount > 0 ? t('app.hub_desk.filter_count', { count: filterCount }) : t('app.hub_desk.filter'), icon: 'funnel', tour: 'queue-filter', onclick: () => (isFiltering = !isFiltering) };
    if (!narrowing || !mayShape) return [filter];
    return [filter, { label: t('app.hub_desk.save_as_queue'), icon: 'plus', tour: 'queue-save', onclick: () => { newName = ''; failure = undefined; isNaming = true; } }];
  });

  // ⋯ → *Delete queue*: a saved queue, by whoever shapes the hub; the requests stay.
  let isDeleting = $state(false);
  async function deleteQueue() {
    if (!saved || isSaving) return;
    isSaving = true;
    failure = undefined;
    try {
      await views.removeQueue(hubId, saved.id, saved.version);
      isDeleting = false;
      onnavigate(`/hubs/${hubId}/desk`);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
  $effect(() => frame.entitle(hub ? t('app.hub_desk.title_of', { place: hub.name }) : queueName));

  $effect(() => {
    const at = hubId;
    return untrack(() => containers.openLevel(at));
  });
  $effect(() => untrack(() => entryTypes.open()));
  $effect(() => {
    const at = hubId;
    return untrack(() => people.open({ hubId: at }));
  });
  const mayShape = $derived(holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted');

  // A refused read of the types reads as no desk: what the reader cannot read offers no portal.
  const typesStatus = $derived(entryTypes.state?.status);
  const isReading = $derived(containers.isLevelLoading(hubId) || typesStatus === undefined || typesStatus === 'idle' || typesStatus === 'loading');
  const deskIds = $derived(isReading ? [] : desksOf(containers.collectionsOf(hubId), entryTypes.all));
  const desks = $derived(containers.collectionsOf(hubId).filter((collection) => deskIds.includes(collection.id)));
  // *New request* opens the desk's list, where an entry is captured (§5.2); each other desk in its list.
  const primary = $derived.by(() => {
    const first = desks[0];
    if (!first) return undefined;
    return {
      label: t('app.desk_home.new_request'),
      icon: 'plus' as const,
      onclick: () => onnavigate(`/collections/${first.id}`),
      menu: desks.length > 1
        ? { label: t('app.desk_home.new_request_in'), items: desks.map((desk) => ({ id: desk.id, label: t('app.desk_home.in_desk', { desk: desk.name }) })), onselect: (id: string) => onnavigate(`/collections/${id}`) }
        : undefined,
    };
  });
</script>

{#snippet queueStar()}{#if chosen?.view_id}<StarToggle kind="VIEW" id={chosen.view_id} name={queueName} />{/if}{/snippet}
{#snippet countFigure()}<span data-queue-count>{t('app.nav.queue_requests', { count: chosen?.count ?? 0 })}</span>{/snippet}
<WorkPage>
  {#snippet top()}
  <ToolHead {hubId} tool="queues" {onnavigate}
    title={queueName}
    figure={chosen && typeof chosen.count === 'number' ? countFigure : undefined}
    titleEnd={chosen?.view_id ? queueStar : undefined}
    {primary}
    secondary={headActions}
    menu={saved && mayShape ? { label: t('app.hub_desk.more'), items: [{ id: 'delete', label: t('app.hub_desk.delete_queue'), icon: 'trash' }], onselect: () => { failure = undefined; isDeleting = true; } } : undefined}
  />
  {/snippet}

  {#if isReading}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if desks.length === 0}
    <ProductEmpty
      kind="unused"
      icon="inbox"
      product="SERVICE_DESK"
      place={{ type: 'HUB', id: hubId }}
      title={t('app.desk_queues.none')}
      description={t('app.desk_queues.none_body')}
      create={mayShape
        ? { label: t('app.desk.itil_defaults'), onclick: () => onnavigate(`/hubs/${encodeURIComponent(hubId)}/settings/template?template=${ITIL_TEMPLATE}`) }
        : undefined}
    />
  {:else}
    <div class="filter" hidden={!isFiltering} data-queue-filter>
      <QueryPanel layout="LIST_COLLAPSED" onquery={(asked) => (narrowing = asked.filter)} oncount={(count) => (filterCount = count)} />
    </div>
    {#if only}
      {#key `${only.queue}:${chosen?.view_id ?? chosen?.key}`}
        <DeskQueues desks={{ [hubId]: desks.map((desk) => desk.id) }} {only} {onnavigate} />
      {/key}
    {:else}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {/if}
  {/if}
</WorkPage>

<Dialog size="s" bind:isOpen={isNaming} title={t('app.hub_desk.save_as_queue')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="150">
    <Input label={t('app.hub_desk.queue_name')} bind:value={newName} />
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isNaming = false)}>{t('app.workspace.cancel')}</Button>
    <Button tone="primary" isBusy={isSaving} busyLabel={t('app.hub_desk.saving')} disabledReason={newName.trim() === '' ? t('app.workspace.name_required') : undefined} data-save-queue onclick={saveQueue}>{t('app.hub_desk.save_as_queue')}</Button>
  {/snippet}
</Dialog>

<Dialog size="s" bind:isOpen={isDeleting} title={t('app.hub_desk.delete_title', { name: queueName })} dismissLabel={t('app.workspace.cancel')}>
  <p class="quiet">{t('app.hub_desk.delete_body')}</p>
  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isDeleting = false)}>{t('app.workspace.cancel')}</Button>
    <Button tone="danger" isBusy={isSaving} busyLabel={t('app.hub_desk.deleting')} data-delete-queue onclick={deleteQueue}>{t('app.hub_desk.delete_queue')}</Button>
  {/snippet}
</Dialog>

<style>
  .filter[hidden] { display: none; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
