<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's queue (the navigation concept §2.4, UC-SVC-03 check 10): the sidebar's *Queues* opens
  // one queue as a table over every desk of the hub, named by `?queue=` - a saved view marked as a
  // queue, or a standard queue's key. Without one the address takes the hub's first queue. A hub
  // with no desk says how a collection becomes one and offers whoever shapes the hub the ITIL
  // desk's plan at its *Products*; anyone else is told whom to ask (UX-5, §5.5).
  import { untrack } from 'svelte';
  import type { HubQueueList, SavedView } from '@hubtask/sync-engine';
  import { engine } from '../lib/data/engine.ts';
  import { DESK_QUEUES, QUEUE_SHOWN, type DeskQueue } from '../lib/servicedesk/queues.ts';
  import { queuePath } from '../lib/sidebar.ts';
  import { PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { ITIL_TEMPLATE } from '../lib/servicedesk/desk.ts';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { entryTypes } from '../lib/data/entrytypes.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import DeskQueues from '../lib/servicedesk/DeskQueues.svelte';
  import { desksOf } from '../lib/servicedesk/queues.ts';

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
    if (!id || STANDARD[chosen?.key ?? '']) return;
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
      return { queue: fallback, name: t(`app.desk_queues.${fallback}`) };
    }
    if (!chosen) return undefined;
    const standard = STANDARD[chosen.key];
    if (standard) return { queue: standard, name: queueName };
    if (!saved) return undefined;
    const asked = saved.query as Record<string, unknown>;
    return {
      queue: 'open' as const,
      name: queueName,
      query: {
        scope: { container_id: (asked['scope_container_id'] as string | undefined) ?? hubId, include_descendants: true },
        ...(asked['filter'] ? { filter: asked['filter'] } : {}),
        sort: asked['sort'] ?? [{ field: 'sla_due', dir: 'ASC', nulls: 'LAST' }],
        page: { size: QUEUE_SHOWN },
        count: 'exact',
      },
    };
  });
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

<Stack gap="300">
  <PageHeader
    title={queueName}
    subtitle={chosen && typeof chosen.count === 'number' ? t('app.nav.queue_requests', { count: chosen.count }) : undefined}
    isTitleInBar={viewport.isCompact}
    {primary}
    breadcrumb={hub
      ? {
          trail: [{ id: 'place', label: hub.name, href: `/hubs/${hubId}` }, { id: 'desk', label: t('app.nav.tool_queues') }],
          label: t('app.hub_desk.trail'),
          expandLabel: t('app.admin.expand_trail'),
          onnavigate: () => onnavigate(`/hubs/${hubId}`),
        }
      : undefined}
  />

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
        ? { label: t('app.desk.itil_defaults'), onclick: () => onnavigate(`/hubs/${encodeURIComponent(hubId)}/settings/modules?template=${ITIL_TEMPLATE}`) }
        : undefined}
    />
  {:else}
    {#if only}
      {#key `${only.queue}:${chosen?.view_id ?? chosen?.key}`}
        <DeskQueues desks={{ [hubId]: desks.map((desk) => desk.id) }} {only} {onnavigate} />
      {/key}
    {:else}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {/if}
  {/if}
</Stack>
