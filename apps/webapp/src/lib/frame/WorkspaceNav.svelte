<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The sidebar: hubs, and the collections of the hubs that are open.
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

  import { Button, EmptyState, ErrorState, SideNav, Skeleton, Stack } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';

  import { containers } from '../data/containers.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The container the reader is on, so the tree can announce it as current. */
    currentId?: string;
    onnavigate: (path: string) => void;
  }

  const { currentId, onnavigate }: Props = $props();

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
    const current = currentId ? containers.find(currentId) : undefined;
    const hubId = current?.type === 'COLLECTION' ? current.parent_id : undefined;
    if (hubId && !expanded.includes(hubId)) expanded = [...expanded, hubId];
  });

  const nodes = $derived(
    containers.hubs.map((hub) => ({
      id: hub.id,
      label: hub.name,
      icon: 'hub' as const,
      // Every hub is a branch, whether or not its collections are loaded — because whether it has
      // any is not known until it is opened, and a hub with no twist is a hub nobody can open to
      // find out. `isBranch` is what says so without inventing a placeholder child.
      isBranch: true,
      children: containers.collectionsOf(hub.id).map((collection) => ({
        id: collection.id,
        label: collection.name,
        icon: 'collection' as const,
      })),
    })),
  );

  const failure = $derived(
    containers.hubsState.status === 'failed'
      ? renderProblem(containers.hubsState.error, messages)
      : undefined,
  );
</script>

{#if containers.hubsState.status === 'loading' || containers.hubsState.status === 'idle'}
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
  <Stack gap="100">
    <SideNav
      label={t('app.workspace.title')}
      {nodes}
      current={currentId}
      bind:expanded
      onnavigate={(id) => {
        const container = containers.find(id);
        if (!container) return;
        onnavigate(container.type === 'HUB' ? `/hubs/${id}` : `/collections/${id}`);
      }}
    />
    <Button tone="subtle" size="sm" icon="plus" onclick={() => (isCreatingHub = true)}>
      {t('app.workspace.create_hub')}
    </Button>
  </Stack>
{/if}

<CreateContainerDialog
  bind:isOpen={isCreatingHub}
  type="HUB"
  oncreated={(id) => onnavigate(`/hubs/${id}`)}
/>
