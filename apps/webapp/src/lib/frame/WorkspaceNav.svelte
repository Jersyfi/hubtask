<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The navigation: the primary destinations, then the hubs and the collections of the hubs that
  // are open, then the trash - one `SideNav`, drawn pinned from `expanded` and in the `NavDrawer`
  // below it (ADR-0061 decision 1). On `compact` the destinations are in the bottom bar and the
  // drawer holds the tree alone, so no destination is drawn twice on any width.
  //
  // "Of the hubs that are open" is the API's shape rather than a lazy-loading flourish.
  // `ListContainers` reads **one level**: an empty `parent_id` is the hubs, a named one is that
  // hub's collections, and the two levels ask different permission questions — which is why there
  // is no read that answers both. So opening a hub is what fetches it, a collapsed hub costs
  // nothing, and the tree's expanded state is the thing that decides what is requested rather than
  // a decoration on top of a list already in memory.
  //
  // It holds no data of its own; `lib/data/containers.svelte.ts` does, and a sidebar that fetched
  // would be a second reader of the same levels.

  import { untrack } from 'svelte';

  import { Button, EmptyState, ErrorState, IconButton, SideNav, Skeleton, Stack } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import ReplicaMark from './ReplicaMark.svelte';

  import { containers } from '../data/containers.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { TRASH, primary } from '../navigation.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /**
     * The node the reader is on, so the tree can announce it as current: a container's id, or a
     * destination's - `workspace` on the workspace page, `trash` in the trash. The frame decides,
     * because the frame has the route; a collection marks the collection, not the workspace row.
     */
    current?: string;
    /** Whether the primary group is drawn above the tree - not on `compact`, where the bar has it. */
    hasDestinations?: boolean;
    /** Folded to its marks: the tree stays whole and draws its marks alone, as do the controls beside it. */
    isRail?: boolean;
    onnavigate: (path: string) => void;
  }

  const { current, hasDestinations = true, isRail = false, onnavigate }: Props = $props();

  /** The one list's primary group, as nodes of the same tree. */
  const destinationNodes = $derived(
    hasDestinations
      ? primary().map((destination) => ({ id: destination.id, label: t(destination.code), icon: destination.icon }))
      : [],
  );

  // A workspace has to be startable from inside the application, and a hub is where starting one
  // begins: nothing else can be created until one exists.
  let isCreatingHub = $state(false);

  let expanded = $state<string[]>([]);

  // One subscription per open hub, dropped when it closes. The effect is what ties the two
  // together: a hub that is opened, closed and opened again does not accumulate subscriptions, and
  // a level nobody is looking at stops being listened to.
  //
  // `untrack` is not a flourish. Subscribing delivers the current state at once and the listener
  // writes the store, and writing it reads it — so an effect that both subscribes and tracks that
  // read re-runs on its own first delivery and unsubscribes before the answer arrives. This effect
  // depends on `expanded` and on nothing else, which is what it is actually about.
  $effect(() => {
    const open = [...expanded];
    return untrack(() => {
      const stops = open.map((hubId) => containers.openLevel(hubId));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  // The hub the reader is inside is opened for them. Arriving at a collection by deep link and
  // finding its own hub collapsed would make the sidebar disagree with the address bar.
  $effect(() => {
    const container = current ? containers.find(current) : undefined;
    const hubId = container?.type === 'COLLECTION' ? container.parent_id : undefined;
    if (hubId && !expanded.includes(hubId)) expanded = [...expanded, hubId];
  });

  /**
   * The tree, with what this reader has lost taken out of it.
   *
   * `offline-sync.md` §6 and §9 rule 3: a client deletes what it holds for a container it lost. The
   * server will stop answering it on the next read, and the sidebar is what somebody is looking at
   * in the meantime — a row they can click and be refused by is worse than a row that has gone.
   */
  const hubNodes = $derived(
    containers.hubs
      .filter((hub) => !live.hasLost(hub.id))
      .map((hub) => ({
      id: hub.id,
      label: hub.name,
      icon: 'hub' as const,
      // Every hub is a branch, whether or not its collections are loaded — because whether it has
      // any is not known until it is opened, and a hub with no twist is a hub nobody can open to
      // find out. `isBranch` is what says so without inventing a placeholder child.
      isBranch: true,
      children: containers
        .collectionsOf(hub.id)
        .filter((collection) => !live.hasLost(collection.id))
        .map((collection) => ({
          id: collection.id,
          label: collection.name,
          icon: 'collection' as const,
        })),
    })),
  );

  // The trash is content of the workspace (arc42 F-09), so it is the tree's last node - under the
  // hubs, where somebody looks when something is missing - and it is there before the hubs have
  // loaded and when there are none.
  const nodes = $derived([
    ...destinationNodes,
    ...hubNodes,
    { id: TRASH.id, label: t(TRASH.code), icon: TRASH.icon },
  ]);

  const isTreeReady = $derived(containers.hubsState.status !== 'loading' && containers.hubsState.status !== 'idle');

  function navigate(id: string) {
    const destination = primary().find((each) => each.id === id);
    if (destination && destination.target.kind === 'route') return onnavigate(destination.target.path);
    if (id === TRASH.id) return onnavigate(TRASH.path);
    const container = containers.find(id);
    if (!container) return;
    onnavigate(container.type === 'HUB' ? `/hubs/${id}` : `/collections/${id}`);
  }

  const failure = $derived(
    containers.hubsState.status === 'failed'
      ? renderProblem(containers.hubsState.error, messages)
      : undefined,
  );
</script>

<Stack gap="100">
  <!-- Not in the rail: a sentence has no room in a column of marks, and the sync line under the
       bar says the same thing on every width. -->
  {#if isTreeReady && !failure && !isRail}
    <ReplicaMark state={containers.hubsState} />
  {/if}
  <SideNav label={t('app.workspace.title')} {nodes} {current} {isRail} flyoutLabel={(name) => t('app.nav.inside', { name })} bind:expanded onnavigate={navigate} />
  {#if !isTreeReady}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => containers.refresh()}
    />
  {:else if containers.hasNoHubs}
    <!-- `unused` and not `filtered`: nothing is filtering the sidebar, and voice-and-tone.md §4.2 is
         about a filter that excluded something. §4.1 is the other half of that rule - say what this
         place is for, and offer the one action. Before this the empty state was a dead end, and the
         one action was `hubctl`. -->
    <EmptyState kind="unused" title={t('app.workspace.no_hubs')} icon="hub">
      {#snippet action()}
        <Button icon="plus" onclick={() => (isCreatingHub = true)}>
          {t('app.workspace.create_hub')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else}
    <!-- In a block of its own so that the control keeps its width at the start of the line
         rather than stretching across the column with its label in the middle. -->
    <div>
      {#if isRail}
        <IconButton icon="plus" label={t('app.workspace.create_hub')} size="sm" onclick={() => (isCreatingHub = true)} />
      {:else}
        <Button tone="subtle" size="sm" icon="plus" onclick={() => (isCreatingHub = true)}>
          {t('app.workspace.create_hub')}
        </Button>
      {/if}
    </div>
  {/if}
</Stack>

<CreateContainerDialog
  bind:isOpen={isCreatingHub}
  type="HUB"
  oncreated={(id) => onnavigate(`/hubs/${id}`)}
/>
