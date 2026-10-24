<!-- SPDX-License-Identifier: Apache-2.0
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

  import { Button, EmptyState, ErrorState, IconButton, Menu, SideNav, Skeleton, type MenuItem, type RowMenu } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import ReplicaMark from './ReplicaMark.svelte';
  import { chosenSymbol, markOf } from '../workspace/look.ts';

  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { holds, holdsHere, hubCreationHere, roleOrder } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { anchorFor } from '../data/rank.ts';
  import { people } from '../data/people.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { rowMessage } from '../data/figure.ts';
  import { KEEPING, primary } from '../navigation.ts';
  import { commands } from '../palette/commands.svelte.ts';
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

  /** Whether the jumble is the reader's, which decides whether its row is drawn. */
  const hasJumble = $derived(holdsHere('READ') === true);

  /** The one list's primary group, as nodes of the same tree. */
  const destinationNodes = $derived(
    hasDestinations
      ? primary({ hasJumble }).map((destination) => ({ id: destination.id, label: t(destination.code), icon: destination.icon }))
      : [],
  );

  // A workspace has to be startable from inside the application, and a hub is where starting one
  // begins: nothing else can be created until one exists.
  let isCreatingHub = $state(false);
  // Which hub the reader may create, as the overview's head offers it: a shared one with STRUCTURE,
  // a private one of one's own where the manifest offers it, none otherwise or while unknown.
  const creation = $derived(hubCreationHere());
  const createLabel = $derived(
    creation === 'shared' ? t('app.workspace.create_hub') : creation === 'private' ? t('app.workspace.create_private_hub') : undefined,
  );

  /**
   * *＋ New* (ux-inventory.md §E1): a hub, or a collection in the hub the reader stands in, each
   * with its preset in the one create dialog. The collection is offered only where the reader's
   * role on that hub holds `STRUCTURE`, the condition the hub's own *Create collection* asks;
   * elsewhere the control stays the hub's alone (P-05).
   */
  const hereHub = $derived.by(() => {
    const container = current ? containers.find(current) : undefined;
    const hubId = container?.type === 'HUB' ? container.id : container?.type === 'COLLECTION' ? container.parent_id : undefined;
    return hubId ? containers.find(hubId) : undefined;
  });
  $effect(() => {
    const hubId = hereHub?.id;
    if (!hubId) return;
    return untrack(() => people.open({ hubId }));
  });
  const mayCreateCollection = $derived(
    hereHub !== undefined &&
      holds(people.roleOf({ hubId: hereHub.id }, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted',
  );
  let isCreatingCollection = $state(false);
  /** The collection's hub, held while its dialog is open so that moving on does not change it. */
  let collectionHubId = $state<string | undefined>(undefined);
  const newItems = $derived([
    ...(createLabel ? [{ id: 'hub', label: createLabel, icon: 'hub' as const }] : []),
    ...(mayCreateCollection && hereHub ? [{ id: 'collection', label: t('app.nav.new_collection', { hub: hereHub.name }), icon: 'collection' as const }] : []),
  ]);
  function chooseNew(id: string) {
    if (id === 'hub') isCreatingHub = true;
    else if (id === 'collection' && hereHub) {
      collectionHubId = hereHub.id;
      isCreatingCollection = true;
    }
  }

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

  /** The hubs and a hub's collections as the tree draws them: the levels a rank position is in. */
  const shownHubs = $derived(containers.hubs.filter((hub) => !live.hasLost(hub.id)));
  const shownCollectionsOf = (hubId: string) => containers.collectionsOf(hubId).filter((collection) => !live.hasLost(collection.id));

  // Each hub's people, so the tree knows where the reader may rank: `:reorder` and `:move` ask
  // `STRUCTURE` on the container, as the page menu's *Move up/down* does.
  $effect(() => {
    const hubIds = shownHubs.map((hub) => hub.id);
    untrack(() => {
      for (const hubId of hubIds) people.open({ hubId });
    });
  });

  /**
   * Whether the reader may rank in this hub - the hub itself, or one of its collections. A grip
   * where the role may not is absent, not refused (P-05); while the role is unread it is absent
   * too. A collection is judged by its hub's role, so a grant on one collection alone keeps that
   * collection's rank to its page menu.
   */
  const mayRankIn = (hubId: string) =>
    holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted';

  /**
   * A hub's or collection's own menu in the tree (design-system.md §11.3, §13 G7): the place is
   * managed where it is listed. Its items are the page menu's own, by id, and choosing one leaves
   * the page the wish (`commands.intend`) and goes there: the page runs it with its own handler
   * and its own answers, so there is one implementation of each act. Offered where the reader may
   * change the hub's shape - the condition the page asks for these acts - and absent otherwise.
   */
  function rowMenuOf(id: string): RowMenu | undefined {
    const container = containers.find(id);
    if (!container || isRail) return undefined;
    const hubId = container.type === 'HUB' ? container.id : container.parent_id;
    if (!hubId || !mayRankIn(hubId)) return undefined;
    const items: MenuItem[] = [
      { id: 'edit', label: t('app.workspace.edit'), icon: 'pencil' },
      ...(container.type === 'COLLECTION' ? [{ id: 'move', label: t('app.move.to_hub'), icon: 'arrow-right-left' as const }] : []),
      { id: 'settings', label: t('app.settings.title'), icon: 'settings' },
      { id: 'archive', label: t('app.workspace.archive'), icon: 'archive', hasSeparatorBefore: true },
      { id: 'trash', label: t('app.workspace.trash'), icon: 'trash', isDestructive: true, hasSeparatorBefore: true },
    ];
    return {
      label: t('app.nav.row_actions', { name: container.name }),
      items,
      onselect: (act) => {
        const place = container.type === 'HUB' ? `/hubs/${container.id}` : `/collections/${container.id}`;
        // The settings are a page of their own (design-system.md §11.8); the rest the place's page runs.
        if (act === 'settings') return onnavigate(`${place}/settings`);
        commands.intend({ containerId: container.id, act });
        onnavigate(place);
      },
    };
  }

  let rankFailure = $state<string | undefined>(undefined);

  /**
   * A drag or `Alt+↑/↓` in the tree: the same write the page menu's *Move up/down* sends - a hub
   * by `:reorder`, a collection by `:move` into the hub it is already in - and the same sentence
   * after it, because the tree rearranged itself and nothing focused changed its name.
   */
  async function rank(id: string, to: number) {
    const container = containers.find(id);
    const siblings = container?.type === 'HUB' ? shownHubs : container?.parent_id ? shownCollectionsOf(container.parent_id) : [];
    if (!container || to < 0 || to >= siblings.length) return;
    rankFailure = undefined;
    try {
      if (container.type === 'HUB') await containers.reorder(id, siblings, to, crypto.randomUUID());
      else if (container.parent_id) await containers.move(id, container.parent_id, anchorFor(siblings, id, to), crypto.randomUUID());
      announcer.say(t('app.rank.announced', { title: container.name, position: to + 1, count: siblings.length }));
    } catch (error) {
      rankFailure = renderProblem(error as never, messages).message;
      announcer.say(rankFailure);
    }
  }

  /**
   * The tree, with what this reader has lost taken out of it.
   *
   * `offline-sync.md` §6 and §9 rule 3: a client deletes what it holds for a container it lost. The
   * server will stop answering it on the next read, and the sidebar is what somebody is looking at
   * in the meantime — a row they can click and be refused by is worse than a row that has gone.
   */
  const hubNodes = $derived(
    shownHubs
      .map((hub) => ({
      id: hub.id,
      label: hub.name,
      // Its own symbol and colour where its people chose them (UC-WRK-33). A private hub nobody
      // chose a symbol for wears the lock its own page explains (UC-ID-16); one with a symbol says
      // it in words at the end of the row, so the choice does not cost the fact.
      icon: hub.private && !chosenSymbol(hub) ? ('lock' as const) : markOf(hub),
      colorToken: hub.color_token ?? undefined,
      meta: hub.private && chosenSymbol(hub) ? t('app.nav.private') : undefined,
      isMovable: mayRankIn(hub.id),
      // A hub is a branch until its level says otherwise. Whether it has collections is not known
      // before it is opened, and a hub with no twist is a hub nobody can open to find out - so it
      // carries one, and **loses it once the level has been read and is empty**: a
      // twist that opens nothing is a promise the navigation cannot keep. The row still goes to
      // the hub, which is where a collection is made.
      isBranch:
        containers.isLevelLoading(hub.id) || shownCollectionsOf(hub.id).length > 0,
      children: shownCollectionsOf(hub.id)
        .map((collection) => {
          // What counts in it, short (UC-WRK-29): nothing where it counts nothing or nothing yet.
          const figure = collection.figure ? rowMessage(collection.figure, messages.locale) : undefined;
          return {
            id: collection.id,
            label: collection.name,
            icon: markOf(collection),
            colorToken: collection.color_token ?? undefined,
            meta: figure ? t(figure.code, figure.params) : undefined,
            isMovable: mayRankIn(hub.id),
          };
        }),
    })),
  );

  /**
   * The three bands, in the one order (ADR-0063 decision 1).
   *
   * `places` first, then the workspace's own structure under its caption, then `keeping` — the
   * trash and the archive — pinned to the foot of the column. The
   * band is a mark on the first node of each, so the tree stays one list with one keyboard walk
   * and the drawing still gets the separation: the trash under the last hub would
   * otherwise read as one more hub.
   *
   * The `keeping` band is there before the hubs have loaded and when there are none.
   */
  const nodes = $derived([
    ...destinationNodes,
    ...hubNodes.map((hub, index) => (index === 0 ? { ...hub, band: { caption: t('app.nav.hubs') } } : hub)),
  ]);

  /**
   * The `keeping` band, drawn as its own tree at the foot of the column.
   *
   * Its own, because it is pinned there: a band inside the tree above could only be pushed down by
   * making that tree fill the column, and then nothing — the control that makes a hub least of all
   * — could stand between the hubs and it. Two trees in one column are not two navigations: the
   * list is still `navigation.ts`'s, and the rows are the same rows. What it costs is that the
   * arrows do not carry from the last hub into the trash, which `Tab` does instead.
   */
  const keepingNodes = $derived(KEEPING.map((each) => ({ id: each.id, label: t(each.code), icon: each.icon })));

  const isTreeReady = $derived(containers.hubsState.status !== 'loading' && containers.hubsState.status !== 'idle');

  function navigate(id: string) {
    const destination = primary({ hasJumble }).find((each) => each.id === id);
    if (destination && destination.target.kind === 'route') return onnavigate(destination.target.path);
    const keeping = KEEPING.find((each) => each.id === id);
    if (keeping) return onnavigate(keeping.path);
    const container = containers.find(id);
    if (!container) return;
    onnavigate(container.type === 'HUB' ? `/hubs/${id}` : `/collections/${id}`);
  }

  const failure = $derived(
    containers.hubsState.status === 'failed'
      ? renderProblem(containers.hubsState.error, messages)
      : undefined,
  );

  // While nobody has given this person anything, coming back to the window asks again: an
  // administrator granting access meanwhile shows without a reload. Only then - a person with
  // access is not re-read on every focus.
  $effect(() => {
    if (!actor.hasNoAccess) return;
    const askAgain = () => {
      void actor.reread();
      void containers.refresh();
    };
    window.addEventListener('focus', askAgain);
    return () => window.removeEventListener('focus', askAgain);
  });
</script>

<!-- A column rather than a `Stack`, because the tree has to fill it: the `keeping` band is
     pinned to the foot of the navigation, and `auto` on a margin needs a height to push against. -->
<div class="column">
  <!-- Not in the rail: a sentence has no room in a column of marks, and the sync line under the
       bar says the same thing on every width. -->
  {#if isTreeReady && !failure && !isRail}
    <ReplicaMark state={containers.hubsState} />
  {/if}
  <!-- `branchLabel` is what makes a hub a place: with a word for the twist, pressing the row goes
       to the hub and the twist at its end opens and closes it. Without one the row would only
       fold, and the hub's own screen would be reachable solely through a collection and back up
       the breadcrumb. -->
  <SideNav
    label={t('app.workspace.title')}
    {nodes}
    {current}
    {isRail}
    flyoutLabel={(name) => t('app.nav.inside', { name })}
    branchLabel={(name, isExpanded) => t(isExpanded ? 'app.nav.close_branch' : 'app.nav.open_branch', { name })}
    bind:expanded
    onnavigate={navigate}
    onreorder={(id, to) => void rank(id, to)}
    rowMenu={rowMenuOf}
  />
  {#if rankFailure && !isRail}<p class="failure">{rankFailure}</p>{/if}
  <!-- Folded, a sentence has nowhere to be drawn: what is waiting, what failed and what an empty
       workspace should do next are all words, and a column of marks has room for none of them
       (ADR-0063 decision 2). The rail draws the marks and the one control; the reader unfolds it
       to be told anything. -->
  {#if !isTreeReady}
    {#if !isRail}<div aria-busy="true"><Skeleton lines={4} /></div>{/if}
  {:else if failure && !isRail}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => containers.refresh()}
    />
  {:else if actor.hasNoAccess}
    {#if !isRail}
      <!-- Nobody gave this person anything yet (UC-ID-08 check 2): a sentence, and no *Create hub*,
           which the server refuses to an account holding no role anywhere. -->
      <EmptyState kind="unused" title={t('app.workspace.no_access')} icon="hub" />
    {/if}
  {:else if containers.hasNoHubs && !isRail}
    <!-- `unused` and not `filtered`: nothing is filtering the sidebar, and voice-and-tone.md §4.2 is
         about a filter that excluded something. §4.1 is the other half of that rule - say what this
         place is for, and offer the one action. Before this the empty state was a dead end, and the
         one action was `hubctl`. -->
    <EmptyState kind="unused" title={t('app.workspace.no_hubs')} icon="hub">
      {#snippet action()}
        {#if createLabel}
          <Button icon="plus" onclick={() => (isCreatingHub = true)}>
            {createLabel}
          </Button>
        {/if}
      {/snippet}
    </EmptyState>
  {:else if newItems.length > 0}
    <!-- *＋ New* lists what can be made here, the hub first - one name on every page, even where
         the list holds one row: a control that read "Create hub" on one page and "New" on the next
         would be two controls. In a block of its own so that it keeps its width at the start of
         the line; folded, it stands in the rail's one column with the marks above it. -->
    <div class="create" data-rail={isRail ? '' : undefined} data-new>
      <Menu label={t('app.nav.new')} items={newItems} onselect={chooseNew}>
        {#snippet trigger(props)}
          {#if isRail}
            <IconButton icon="plus" label={t('app.nav.new')} size="sm" {...props} />
          {:else}
            <Button tone="subtle" size="sm" icon="plus" {...props}>{t('app.nav.new')}</Button>
          {/if}
        {/snippet}
      </Menu>
    </div>
  {/if}

  <!-- Where a reader goes when something is missing: at the foot of the column, on every width,
       never mixed in with the hubs above it (ADR-0063 decision 1). -->
  <div class="keeping">
    <SideNav label={t('app.nav.keeping')} nodes={keepingNodes} {current} {isRail} onnavigate={navigate} />
  </div>
</div>

<CreateContainerDialog
  bind:isOpen={isCreatingHub}
  type="HUB"
  isPrivateOnly={creation === 'private'}
  oncreated={(id) => onnavigate(`/hubs/${id}`)}
/>
{#if collectionHubId}
  <CreateContainerDialog
    bind:isOpen={isCreatingCollection}
    type="COLLECTION"
    parentId={collectionHubId}
    oncreated={(id) => onnavigate(`/collections/${id}`)}
  />
{/if}

<style>
  .column {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-block-size: 100%;
  }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  /* Folded, the control is in the rail's column like every mark above it. */
  .create[data-rail] { display: flex; justify-content: center; }

  /* Pushed to the bottom of the column, with the hairline that says a band begins. */
  .keeping {
    margin-block-start: auto;
    padding-block-start: var(--sp-200);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
</style>
