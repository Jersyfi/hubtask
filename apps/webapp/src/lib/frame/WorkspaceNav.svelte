<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The sidebar of the navigation concept (user-experience.md §2, UX-1): a fixed top (*For you*), a
  // scrolling middle that is the workspace level or one hub's level, and a fixed foot. What each
  // part holds is answered by `navigation.ts`; this component reads the data and draws the answer
  // with the wave-5 pieces. It reads only what the level it draws needs: a hub's modules, queues,
  // collections, pages and boards while that hub's level is shown.

  import { untrack } from 'svelte';

  import {
    EmptyState,
    NavFlyout,
    NavGroup,
    NavItem,
    NavLevel,
    Skeleton,
    ancestorsOf,
    moveIntent,
    type MoveKey,
    type NavTreeNode,
    type RowAction,
    type TreePlacement,
  } from '@hubtask/design-system/components';
  import type { AgentRequestList, ApprovalList, Container, GoalList, HubQueueList, CmdbHealth, ItemQueryResult, PageList, ReleaseList, SprintList, Star, WhiteboardList } from '@hubtask/sync-engine';

  import FailureState from './FailureState.svelte';
  import ReplicaMark from './ReplicaMark.svelte';
  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import { chosenSymbol, markOf } from '../workspace/look.ts';
  import { placeMenu } from '../workspace/placeMenu.ts';
  import { ownsHub, privacyAction } from '../workspace/privacy.ts';

  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { requestsPath } from '../data/agents.ts';
  import { myApprovalsPath } from '../data/approvals.svelte.ts';
  import { holds, holdsHere, hubCreationHere, reachesAdministrationArea, roleOrder } from '../data/capability.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { archivalOf } from '../data/containers.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { entryTypesOffered } from '../data/entrytypes.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { rowMessage } from '../data/figure.ts';
  import { live } from '../data/live.svelte.ts';
  import { modules, modulesHere } from '../data/modules.svelte.ts';
  import { pagesPath } from '../data/pages.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { preferences } from '../data/preferences.svelte.ts';
  import { anchorFor } from '../data/rank.ts';
  import { watchPageReviews } from '../data/reviews.svelte.ts';
  import { sprintsPath } from '../data/sprints.svelte.ts';
  import { goalsPath } from '../data/goals.svelte.ts';
  import { healthPath } from '../data/cmdb.ts';
  import { jumble } from '../data/jumble.svelte.ts';
  import { releasesPath } from '../data/releases.svelte.ts';
  import { today as todayStore } from '../data/today.svelte.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { goalsStanding, nextReleaseOf, sprintsRunning } from '../product/home.ts';
  import { queueQuery } from '../servicedesk/queues.ts';
  import { stars } from '../data/stars.svelte.ts';
  import { isStarred } from '../data/stars.ts';
  import { boardsPath } from '../data/whiteboards.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import {
    COLLECTIONS_FILTER_FROM,
    COLLECTIONS_SHOWN,
    CONTENTS_FILTER_FROM,
    CONTENTS_SHOWN,
    HUBS_SHOWN,
    QUEUES_SHOWN,
    contentsCount,
    contentsTree,
    figureCount,
    foot,
    forYou,
    hubTools,
    levelOf,
    navProducts,
    orderHubs,
    placedIndex,
    queuePath,
    queueRows,
    switchPath,
    toolOf,
    toolPath,
    type ContentNode,
    type ContentsMode,
    type HubTool,
    type HubsOrder,
    type NavLevel as Level,
  } from '../sidebar.ts';
  import { leftToWebApp } from '../navigation.ts';
  import { commands } from '../palette/commands.svelte.ts';
  import { actsAsDesk } from '../portal/portal.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';
  import { recents } from '../recents.svelte.ts';
  import { isDueByTonight, productBandsOf, sprintDaysLeft, todayRowCountOf } from '../today.ts';
  import { webAppAddress, webAppHost } from '../webApp.ts';
  import type { Resolution } from '../router.ts';

  interface Props {
    route: Resolution;
    /** The container the screen named where the route does not (`page.locate`). */
    located?: string;
    /** Folded to the rail: the marks alone. */
    isRail?: boolean;
    /** Whether *For you* is drawn whole - not on `compact`, where the bottom bar carries most of it. */
    hasForYou?: boolean;
    onnavigate: (path: string) => void;
  }

  const { route, located, isRail = false, hasForYou = true, onnavigate }: Props = $props();

  const workspaceOn = $derived(manifest.value?.modules_on);
  const hasJumble = $derived(holdsHere('READ') === true);
  const topRows = $derived(forYou({ hasJumble, modulesOn: workspaceOn, besideBar: !hasForYou }));
  const footRows = $derived(foot({ isAdministrationReachable: reachesAdministrationArea() === true }));

  // What waits for the reader's yes - approvals, held agent acts, page reviews (§2.2).
  let waiting = $state<ApprovalList['data'] | undefined>(undefined);
  let waitingHasMore = $state(false);
  let heldActs = $state<AgentRequestList['data'] | undefined>(undefined);
  let reviews = $state(0);
  const todayBands = $derived(productBandsOf(workspaceOn));
  const countsApprovals = $derived(todayBands.includes('approvals'));
  const countsActs = $derived(todayBands.includes('agent_acts'));
  const countsReviews = $derived(todayBands.includes('reviews'));
  $effect(() => {
    if (!countsApprovals) return void (waiting = undefined);
    return untrack(() => engine.subscribe<ApprovalList>({ path: myApprovalsPath }, (next) => {
      if (next.status === 'ready') {
        waiting = next.data.data;
        waitingHasMore = next.data.page?.has_more === true;
      }
    }));
  });
  $effect(() => {
    if (!countsActs) return void (heldActs = undefined);
    return untrack(() => engine.subscribe<AgentRequestList>({ path: requestsPath('PENDING') }, (next) => {
      if (next.status === 'ready') heldActs = next.data.data;
    }));
  });
  const reviewHubs = $derived(containers.hubs.map((each) => each.id).join(' '));
  $effect(() => {
    const ids = countsReviews && reviewHubs ? reviewHubs.split(' ') : [];
    const viewer = actor.account?.id;
    if (ids.length === 0) return void (reviews = 0);
    return untrack(() => watchPageReviews(ids, viewer, (rows) => (reviews = rows.length)));
  });
  const waitingCount = $derived(todayRowCountOf(waiting, heldActs, waitingHasMore, reviews));

  // *Today*'s count: the reader's open entries due by the end of their day, overdue among them -
  // the read the Today page draws from. *Inbox*'s: the arrivals still undecided.
  $effect(() => untrack(() => todayStore.open()));
  const todayCount = $derived(figureCount(todayStore.mine.filter((item) => isDueByTonight(item, actor.zone, Date.now())).length, todayStore.hasMore));
  $effect(() => {
    if (!hasJumble) return;
    return untrack(() => jumble.open('NEW'));
  });
  const inboxCount = $derived(hasJumble ? figureCount(jumble.of('NEW').length, jumble.moreAfter('NEW') !== undefined) : undefined);
  const topFigure = (id: string): string | undefined =>
    id === 'workspace' ? todayCount : id === 'jumble' ? inboxCount : id === 'approvals' && waitingCount ? String(waitingCount) : undefined;

  // The person's stars, one list for every ☆ (§6.2).
  $effect(() => {
    const id = actor.account?.id;
    if (!id) return;
    return untrack(() => stars.open(id));
  });

  // ---- The level ----------------------------------------------------------------------------
  const hubOf = (id: string) => {
    const container = containers.find(id);
    return container ? (container.type === 'HUB' ? container.id : (container.parent_id ?? undefined)) : undefined;
  };
  const routeLevel = $derived<Level>(levelOf(route, hubOf, located));
  /** *‹ All hubs* shows the workspace level without leaving the page; the next address ends it. */
  let isBrowsingAll = $state(false);
  $effect(() => {
    void route.path;
    isBrowsingAll = false;
  });
  const level = $derived<Level>(isBrowsingAll ? { kind: 'workspace' } : routeLevel);
  const hubId = $derived(level.kind === 'hub' ? level.hubId : undefined);
  const hub = $derived(hubId ? containers.find(hubId) : undefined);
  const tool = $derived(toolOf(route));
  /** A place without a chosen colour still stands on a tile, as the mockup draws every row. */
  const DEFAULT_TILE = 'blue';

  // Every hub's level is read, as the tree always did: a collection's hub is then known on a deep
  // link, and a starred collection has its name.
  const hubIds = $derived(containers.hubs.map((each) => each.id).join(','));
  $effect(() => {
    const ids = hubIds === '' ? [] : hubIds.split(',');
    return untrack(() => {
      const stops = ids.map((id) => containers.openLevel(id));
      return () => stops.forEach((stop) => stop());
    });
  });
  $effect(() => {
    const ids = hubIds === '' ? [] : hubIds.split(',');
    untrack(() => ids.forEach((id) => people.open({ hubId: id })));
  });

  // The hub's own modules: the workspace's narrowed by the hub (UC-INS-21 check 11).
  let hubModules = $state<readonly string[] | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    hubModules = undefined;
    if (!at) return;
    return untrack(() => modules.here(at, (next) => {
      if (next.status === 'ready') hubModules = next.data.modules_on;
    }));
  });
  const hereOn = $derived(hubModules === undefined ? [] : modulesHere(workspaceOn, hubModules));
  const isOn = (module: string) => (hereOn as readonly string[]).includes(module);

  const collections = $derived(hubId ? containers.collectionsOf(hubId).filter((each) => !live.hasLost(each.id)) : []);
  $effect(() => {
    if (!isOn('SERVICE_DESK')) return;
    return untrack(() => entryTypes.open());
  });
  const isDesk = (collection: Container) =>
    isOn('SERVICE_DESK') && actsAsDesk(entryTypes.all, entryTypesOffered(collection.structure?.settings, 'TASK'), collection.parent_id);
  const hasDesk = $derived(collections.some(isDesk));
  const hubToolRows = $derived<readonly HubTool[]>(hubTools({ modulesOn: hereOn, hasDesk }));

  // Which collections run sprints and their running sprint: one read each, only where planning is
  // on at the hub. The same read answers the sprint mark, the collection's days and *Plan*'s figure.
  let sprinting = $state<Record<string, boolean>>({});
  let sprintsAt = $state<Record<string, SprintList['data']>>({});
  const sprintIds = $derived(isOn('PLANNING') ? collections.map((each) => each.id).join(',') : '');
  $effect(() => {
    const ids = sprintIds === '' ? [] : sprintIds.split(',');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<SprintList>({ path: sprintsPath(id) }, (next) => {
        if (next.status !== 'ready') return;
        sprinting = { ...sprinting, [id]: next.data.runs_sprints };
        sprintsAt = { ...sprintsAt, [id]: next.data.data ?? [] };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const today = $derived(todayIn(actor.zone));
  const runningSprintOf = (collectionId: string) => sprintsRunning(sprintsAt, [collectionId], 1)[0];

  // *Plan*'s figure (§2.4): the running sprint that ends first, else the next release.
  let releases = $state<ReleaseList['data']>([]);
  $effect(() => {
    const at = isOn('PLANNING') ? hubId : undefined;
    releases = [];
    if (!at) return;
    return untrack(() => engine.subscribe<ReleaseList>({ path: releasesPath(at) }, (next) => {
      if (next.status === 'ready') releases = next.data.data;
    }));
  });
  const planFigure = $derived.by(() => {
    if (!hubId) return undefined;
    const sprint = sprintsRunning(sprintsAt, collections.map((each) => each.id), 1)[0];
    if (sprint) return t('app.nav.plan_sprint', { name: sprint.name, count: sprintDaysLeft(sprint.end_on, today) });
    const next = nextReleaseOf({ [hubId]: releases }, hubId);
    return next ? t('app.nav.plan_release', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) }) : undefined;
  });

  // *Goals*' figure: the hub's goals at risk or off track, nothing while none is.
  let hubGoals = $state<GoalList['data']>([]);
  $effect(() => {
    const at = isOn('GOALS') ? hubId : undefined;
    hubGoals = [];
    if (!at) return;
    return untrack(() => engine.subscribe<GoalList>({ path: goalsPath(undefined, at) }, (next) => {
      if (next.status === 'ready') hubGoals = next.data.data;
    }));
  });
  const goalsAtRisk = $derived(goalsStanding(hubGoals).atRisk);

  // *Inventory*'s figure: what in the hub's inventory needs attention.
  let inventoryProblems = $state(0);
  $effect(() => {
    const at = isOn('ASSETS') ? hubId : undefined;
    inventoryProblems = 0;
    if (!at) return;
    return untrack(() => engine.subscribe<CmdbHealth>({ path: healthPath(at) }, (next) => {
      if (next.status === 'ready') inventoryProblems = next.data.problems.length;
    }));
  });
  const toolFigure = (id: string): string | undefined =>
    id === 'plan' ? planFigure
      : id === 'goals' && goalsAtRisk > 0 ? t('app.nav.goals_at_risk', { count: goalsAtRisk })
        : id === 'inventory' && inventoryProblems > 0 ? t('app.nav.inventory_attention', { count: inventoryProblems })
          : undefined;

  // The hub's queues with their counts, read once per hub level shown (§7.4).
  let queues = $state<HubQueueList['data']>([]);
  $effect(() => {
    const at = hasDesk && hubToolRows.some((each) => each.id === 'queues') ? hubId : undefined;
    queues = [];
    if (!at) return;
    return untrack(() => engine.subscribe<HubQueueList>({ path: `/hubs/${at}/queues` }, (next) => {
      if (next.status === 'ready') queues = next.data.data;
    }));
  });
  // A desk's figure (§2.4 part 3): its requests about to breach or breached, one count per desk -
  // no list read answers it per desk.
  let deskBreaching = $state<Record<string, number>>({});
  const deskKey = $derived(hubId ? collections.filter(isDesk).map((each) => each.id).join(',') : '');
  $effect(() => {
    const ids = deskKey === '' ? [] : deskKey.split(',');
    const at = hubId;
    deskBreaching = {};
    if (!at) return;
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<ItemQueryResult>({ path: '/items:query', body: queueQuery('breaching', at, [id], 1) }, (next) => {
        if (next.status === 'ready' && typeof next.data.total === 'number') deskBreaching = { ...deskBreaching, [id]: next.data.total };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const starredViews = $derived(stars.all.filter((each) => each.kind === 'VIEW').map((each) => each.id));
  const queueList = $derived(queueRows(queues, starredViews));
  let isQueuesOpen = $state(true);
  let isAllQueues = $state(false);
  const queueName = (row: { name: string | null; key: string }) => row.name ?? t(`app.desk_queues.${row.key}`);
  const currentQueue = $derived(route.name === 'hub-desk' ? route.query['queue'] : undefined);

  // The content tree: the hub's pages and boards, one read each (§2.4 part 3, the limits).
  let pageRows = $state<PageList['data']>([]);
  let boardRows = $state<WhiteboardList['data']>([]);
  const hasContents = $derived(isOn('KNOWLEDGE') || isOn('WHITEBOARD'));
  $effect(() => {
    const at = isOn('KNOWLEDGE') ? hubId : undefined;
    pageRows = [];
    if (!at) return;
    return untrack(() => engine.subscribe<PageList>({ path: pagesPath(at) }, (next) => {
      if (next.status === 'ready') pageRows = next.data.data;
    }));
  });
  $effect(() => {
    const at = isOn('WHITEBOARD') ? hubId : undefined;
    boardRows = [];
    if (!at) return;
    return untrack(() => engine.subscribe<WhiteboardList>({ path: `${boardsPath(at)}?order=TREE` }, (next) => {
      if (next.status === 'ready') boardRows = next.data.data;
    }));
  });
  const contentsMode = $derived<ContentsMode>((actor.account?.nav_contents_mode as ContentsMode | null | undefined) ?? 'TREE');
  const contentNodes = $derived(contentsTree(pageRows, boardRows, contentsMode, messages.locale));
  const toTree = (node: ContentNode): NavTreeNode => ({
    id: node.id,
    label: node.title,
    icon: node.kind === 'page' ? 'file-text' : 'layout-template',
    kind: node.kind,
    part: 'contents',
    canHoldChildren: node.kind === 'page',
    children: node.children.length > 0 ? node.children.map(toTree) : undefined,
  });
  const contentTree = $derived(contentNodes.map(toTree));
  const currentContent = $derived(route.name === 'page' || route.name === 'whiteboard' ? route.params['id'] : undefined);
  let expanded = $state<string[]>([]);
  // The current page's ancestors open (§2.6).
  $effect(() => {
    const id = currentContent;
    if (!id) return;
    const above = ancestorsOf(contentTree, id);
    untrack(() => {
      const missing = above.filter((each) => !expanded.includes(each));
      if (missing.length > 0) expanded = [...expanded, ...missing];
    });
  });
  let contentsFilter = $state('');
  let contentsKind = $state('all');

  // ---- Rows, figures, menus -----------------------------------------------------------------
  function figureOf(container: Container): { text?: string; isUrgent: boolean } {
    const figure = container.figure;
    const words = figure ? rowMessage(figure, messages.locale) : undefined;
    const isUrgent = figure?.kind === 'DUE' ? (figure.due ?? 0) > 0 : figure?.kind === 'FLOW' ? (figure.over_limit ?? 0) > 0 : false;
    return { text: words ? t(words.code, words.params) : undefined, isUrgent };
  }

  const roleIn = (id: string) => people.roleOf({ hubId: id }, actor.account?.id, roleOrder()) ?? undefined;
  const mayRankIn = (id: string) => holds(roleIn(id), 'STRUCTURE').status === 'permitted';

  /** A hub's or collection's ⋯, the page menu's items (`placeMenu`), run by the page itself. */
  function placeMenuOf(id: string): RowAction | undefined {
    const container = containers.find(id);
    if (!container || isRail) return undefined;
    const at = container.type === 'HUB' ? container.id : container.parent_id;
    if (!at) return undefined;
    const path = container.type === 'HUB' ? { hubId: at } : { hubId: at, collectionId: container.id };
    const role = people.roleOf(path, actor.account?.id, roleOrder()) ?? undefined;
    const workspaceRole = people.along({}).find((each) => each.account_id === actor.account?.id && each.scope_type === 'TENANT')?.role;
    const items = placeMenu(
      {
        type: container.type === 'HUB' ? 'HUB' : 'COLLECTION',
        mayShape: holds(role, 'STRUCTURE').status === 'permitted',
        mayDelete: holds(role, 'DELETE_CONTAINER').status === 'permitted',
        archival: archivalOf(container),
        privacy:
          container.type === 'HUB'
            ? privacyAction({
                isPrivate: container.private === true,
                ownsHub: ownsHub(people.along(path), actor.account?.id, container.id),
                mayCreateHubs: holds(workspaceRole as string | undefined, 'STRUCTURE').status === 'permitted',
                isOffered: workspace.workspace?.private_hubs_offered === true,
              })
            : undefined,
      },
      (code) => t(code),
    );
    if (items.length === 0) return undefined;
    return {
      label: t('app.nav.row_actions', { name: container.name }),
      items,
      onselect: (act) => {
        const place = container.type === 'HUB' ? `/hubs/${container.id}` : `/collections/${container.id}`;
        if (act === 'settings') return onnavigate(`${place}/settings`);
        if (act) commands.intend({ containerId: container.id, act });
        onnavigate(place);
      },
    };
  }

  /**
   * The hub head's ⋯ (§2.4 item 1): its link first, for every reader, then the place menu's items -
   * so the head always has one, while its settings stay only there.
   */
  function hubMenuOf(id: string): RowAction | undefined {
    const container = containers.find(id);
    if (!container || isRail) return undefined;
    const place = placeMenuOf(id);
    const rest = (place?.items ?? []).map((item, index) => (index === 0 ? { ...item, hasSeparatorBefore: true } : item));
    return {
      label: t('app.nav.row_actions', { name: container.name }),
      items: [{ id: 'copy-link', label: t('app.nav.copy_link'), icon: 'link' }, ...rest],
      onselect: (act) => {
        if (act !== 'copy-link') return place?.onselect(act);
        void navigator.clipboard.writeText(`${platform.webOrigin}/hubs/${container.id}`).then(
          () => announcer.say(t('app.nav.link_copied')),
          () => undefined,
        );
      },
    };
  }

  let rankFailure = $state<string | undefined>(undefined);
  /** `Alt+↑/↓` or a drag among hubs or a hub's collections: the writes the page menu sends. */
  async function rank(id: string, siblings: readonly Container[], to: number) {
    const container = containers.find(id);
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
  const place = (siblings: readonly Container[]) => (id: string, placement: TreePlacement) =>
    void rank(id, siblings, placedIndex(siblings.map((each) => each.id), id, placement.afterId));
  /** A rank key moves only what the reader may order (P-05): the row draws no key where it may not. */
  const moveKey = (siblings: readonly Container[], nodes: readonly NavTreeNode[], mayOrder: (id: string) => boolean) => (id: string, key: MoveKey) => {
    if (!mayOrder(id)) return;
    const intent = moveIntent(key, nodes, id);
    if (intent && 'placement' in intent) place(siblings)(id, intent.placement);
  };

  // ---- Workspace level ------------------------------------------------------------------------
  const productSets = $derived(navProducts({ modulesOn: workspaceOn, order: actor.account?.nav_products, hidden: actor.account?.nav_products_hidden }));
  let isAllProducts = $state(false);
  async function writePreference(change: Record<string, unknown>) {
    const id = actor.account?.id;
    if (!id) return;
    try {
      await preferences.setAccount(id, change);
      void actor.reread();
    } catch (error) {
      announcer.say(renderProblem(error as never, messages).message);
    }
  }
  const customise = $derived<RowAction>({
    label: t('app.nav.customise'),
    items: [...productSets.shown, ...productSets.more].map((row) => ({
      id: row.module,
      label: t(productSets.more.includes(row) ? 'app.nav.show_product' : 'app.nav.hide_product', { name: t(row.code) }),
    })),
    onselect: (module) => {
      if (!module) return;
      const hidden = actor.account?.nav_products_hidden ?? [];
      const next = hidden.includes(module as never) ? hidden.filter((each) => each !== module) : [...hidden, module];
      void writePreference({ nav_products_hidden: next });
    },
  });

  const shownHubs = $derived(containers.hubs.filter((each) => !live.hasLost(each.id)));
  const hubsOrder = $derived<HubsOrder>((actor.account?.nav_hubs_order as HubsOrder | null | undefined) ?? 'MANUAL');
  const visitedHubs = $derived([...new Set(recents.rows.map((row) => (row.kind === 'hub' ? row.id : row.kind === 'collection' ? row.placeId : undefined)).filter((id): id is string => !!id))]);
  const orderedHubs = $derived(orderHubs(shownHubs, hubsOrder, visitedHubs, messages.locale));
  const hubNode = (each: Container): NavTreeNode => ({
    id: each.id,
    label: each.name,
    icon: each.private && !chosenSymbol(each) ? 'lock' : markOf(each),
    color: each.color_token ?? DEFAULT_TILE,
    part: 'hubs',
    canHoldChildren: false,
    // A private hub says so in words, whatever symbol it wears (UC-ID-16).
    badge: each.private ? { icon: 'lock', label: t('app.nav.private') } : undefined,
    figure: '›',
    figureLabel: t('app.nav.open_level', { name: each.name }),
  });
  const hubNodes = $derived(orderedHubs.map(hubNode));
  const hubsMenu = $derived<RowAction>({
    label: t('app.nav.order_hubs'),
    items: (['MANUAL', 'RECENT', 'NAME'] as const).map((mode) => ({ id: mode, label: t(`app.nav.hubs_${mode.toLowerCase()}`), ...(mode === hubsOrder ? { icon: 'check' as const } : {}) })),
    onselect: (mode) => mode && void writePreference({ nav_hubs_order: mode === 'MANUAL' ? null : mode }),
  });

  let isCreatingHub = $state(false);
  const creation = $derived(hubCreationHere());
  const createHubLabel = $derived(creation === 'shared' ? t('app.workspace.create_hub') : creation === 'private' ? t('app.workspace.create_private_hub') : undefined);

  /** A star as a row: its name from what is already read, or from its own read. */
  let starNames = $state<Record<string, string>>({});
  const READS: Partial<Record<Star['kind'], (id: string) => string>> = {
    PAGE: (id) => `/pages/${id}`,
    WHITEBOARD: (id) => `/whiteboards/${id}`,
    DASHBOARD: (id) => `/dashboards/${id}`,
    PORTFOLIO: (id) => `/portfolios/${id}`,
    VIEW: (id) => `/views/${id}`,
  };
  const READ_STARS = $derived(stars.all.filter((each) => READS[each.kind]).map((each) => `${each.kind}:${each.id}`).join(','));
  $effect(() => {
    const keys = READ_STARS === '' ? [] : READ_STARS.split(',');
    return untrack(() => {
      const stops = keys.map((key) => {
        const [kind, id] = key.split(':') as [Star['kind'], string];
        return engine.subscribe<Record<string, unknown>>({ path: READS[kind]!(id) }, (next) => {
          if (next.status !== 'ready') return;
          const data = next.data;
          const inner = (data['page'] ?? data['whiteboard']) as Record<string, unknown> | undefined;
          const name = data['title'] ?? data['name'] ?? inner?.['title'];
          if (typeof name === 'string') starNames = { ...starNames, [key]: name };
        });
      });
      return () => stops.forEach((stop) => stop());
    });
  });
  const STAR_PATHS: Record<Star['kind'], (id: string) => string> = {
    HUB: (id) => `/hubs/${id}`,
    COLLECTION: (id) => `/collections/${id}`,
    VIEW: (id) => `/views/${id}`,
    PAGE: (id) => `/pages/${id}`,
    WHITEBOARD: (id) => `/whiteboards/${id}`,
    DASHBOARD: (id) => `/dashboards/${id}`,
    PORTFOLIO: (id) => `/portfolios/${id}`,
  };
  const STAR_ICONS = { HUB: 'hub', COLLECTION: 'collection', VIEW: 'columns-3', PAGE: 'file-text', WHITEBOARD: 'layout-template', DASHBOARD: 'gauge', PORTFOLIO: 'flag' } as const;
  const starRows = $derived(
    stars.all
      .map((star) => {
        const container = star.kind === 'HUB' || star.kind === 'COLLECTION' ? containers.find(star.id) : undefined;
        const label = container?.name ?? starNames[`${star.kind}:${star.id}`];
        if (!label) return undefined;
        const figure = container?.type === 'COLLECTION' ? figureOf(container) : undefined;
        return {
          key: `${star.kind}:${star.id}`,
          label,
          icon: container ? markOf(container) : STAR_ICONS[star.kind],
          color: container?.color_token ?? undefined,
          href: STAR_PATHS[star.kind](star.id),
          figure: figure?.text,
          isUrgent: figure?.isUrgent ?? false,
        };
      })
      .filter((row) => row !== undefined),
  );

  const RECENT_PATHS = { item: (id: string) => `/items/${id}`, collection: (id: string) => `/collections/${id}`, hub: (id: string) => `/hubs/${id}` } as const;
  const RECENT_ICONS = { item: 'task', collection: 'collection', hub: 'hub' } as const;

  // ---- Hub level ------------------------------------------------------------------------------
  const isHubStarred = $derived(hub ? isStarred(stars.all, 'HUB', hub.id) : false);
  const mayStructure = $derived(hubId ? mayRankIn(hubId) : false);
  const mayWrite = $derived(hubId ? holds(roleIn(hubId), 'WRITE_ITEMS').status === 'permitted' : false);

  const collectionNodes = $derived(
    collections.map((each): NavTreeNode => {
      const desk = isDesk(each);
      const sprint = sprinting[each.id] ? runningSprintOf(each.id) : undefined;
      const breaching = desk ? (deskBreaching[each.id] ?? 0) : 0;
      const figure = sprint
        ? { text: t('app.nav.sprint_days', { count: sprintDaysLeft(sprint.end_on, today) }), isUrgent: false }
        : breaching > 0
          ? { text: t('app.nav.desk_sla', { count: breaching }), isUrgent: true }
          : figureOf(each);
      return {
        id: each.id,
        label: each.name,
        icon: desk && !chosenSymbol(each) ? 'inbox' : markOf(each),
        color: each.color_token ?? DEFAULT_TILE,
        part: 'collections',
        canHoldChildren: false,
        figure: figure.text,
        isFigureUrgent: figure.isUrgent,
        badge: sprinting[each.id] ? { icon: 'repeat', label: t('app.nav.runs_sprints') } : desk ? { icon: 'inbox', label: t('app.nav.desk_mark') } : undefined,
      };
    }),
  );
  const currentCollection = $derived.by(() => {
    const name = route.name ?? '';
    if (name.startsWith('collection')) return route.params['id'];
    if (name === 'item') return located;
    return undefined;
  });
  let collectionFilter = $state('');
  let isCreatingCollection = $state(false);
  let collectionHubId = $state<string | undefined>(undefined);

  const contentsAdd = $derived<RowAction | undefined>(
    hubId && mayWrite
      ? {
          label: t('app.nav.create_in_contents'),
          items: [
            ...(isOn('KNOWLEDGE') ? [{ id: 'page', label: t('app.nav.create_page') }, { id: 'template', label: t('app.nav.create_from_template') }] : []),
            ...(isOn('WHITEBOARD') ? [{ id: 'whiteboard', label: t('app.nav.create_whiteboard') }] : []),
          ],
          onselect: (kind) => {
            if (!hubId) return;
            onnavigate(kind === 'whiteboard' ? `/hubs/${hubId}/whiteboards?new=1` : `/hubs/${hubId}/pages?new=1`);
          },
        }
      : undefined,
  );
  const contentsMenu = $derived<RowAction>({
    label: t('app.nav.contents_menu'),
    items: [
      ...(['TREE', 'RECENT', 'NAME'] as const).map((mode) => ({ id: mode, label: t(`app.nav.mode_${mode.toLowerCase()}`), ...(mode === contentsMode ? { icon: 'check' as const } : {}) })),
      ...(isOn('KNOWLEDGE') ? [{ id: 'all', label: t('app.nav.all_contents') }] : []),
    ],
    onselect: (choice) => {
      if (choice === 'all' && hubId) return onnavigate(`/hubs/${hubId}/pages`);
      if (choice) void writePreference({ nav_contents_mode: choice === 'TREE' ? null : choice });
    },
  });

  // The quick switch (§2.4 item 1): the same tool at the other hub where it has it.
  let switchSearch = $state('');
  let isSwitchOpen = $state(false);
  const switchHubs = $derived(shownHubs.filter((each) => each.id !== hubId && each.name.toLocaleLowerCase(messages.locale).includes(switchSearch.trim().toLocaleLowerCase(messages.locale))));
  const starredHubs = $derived(switchHubs.filter((each) => isStarred(stars.all, 'HUB', each.id)));
  const visitedSwitch = $derived(visitedHubs.map((id) => switchHubs.find((each) => each.id === id)).filter((each): each is Container => !!each && !isStarred(stars.all, 'HUB', each.id)).slice(0, 5));
  const matchedHubs = $derived(switchSearch.trim() ? switchHubs.filter((each) => !starredHubs.includes(each) && !visitedSwitch.includes(each)) : []);
  function switchTo(target: string) {
    isSwitchOpen = false;
    switchSearch = '';
    const keep = tool;
    if (!keep || keep === 'overview') return onnavigate(`/hubs/${target}`);
    // The target's own modules decide whether it has the tool; asked once, then let go.
    let stop: (() => void) | undefined;
    let isDone = false;
    stop = modules.here(target, (next) => {
      if (isDone || next.status === 'loading' || next.status === 'idle') return;
      isDone = true;
      const on = next.status === 'ready' ? modulesHere(workspaceOn, next.data.modules_on) : [];
      onnavigate(switchPath(target, keep, hubTools({ modulesOn: on, hasDesk: true })));
      queueMicrotask(() => stop?.());
    });
    if (isDone) stop();
  }

  // ---- Navigation, failures -----------------------------------------------------------------
  const webAppOrigins = $derived({ workspace: platform.webOrigin, installation: manifest.value?.installation_url });
  function goFoot(id: string, path: string) {
    const area = leftToWebApp(id, platform.webApp?.areas);
    if (area !== undefined) {
      void platform.webApp?.open(webAppAddress(area, path, webAppOrigins).href).catch(() => {});
      return;
    }
    onnavigate(path);
  }
  const footFigure = (id: string) => {
    const area = leftToWebApp(id, platform.webApp?.areas);
    return area === undefined ? undefined : t('app.web_app.at', { host: webAppHost(area, webAppOrigins) });
  };

  const failure = $derived(containers.hubsState.status === 'failed' ? renderProblem(containers.hubsState.error, messages) : undefined);
  const isTreeReady = $derived(containers.hubsState.status !== 'loading' && containers.hubsState.status !== 'idle');
  const isCurrentTop = (id: string) =>
    level.kind === 'workspace' && (id === 'workspace' ? route.name === 'home' : id === 'jumble' ? route.name === 'jumble' : route.name === id);
  const productCurrent = (routes: readonly string[]) => routes.includes(route.name ?? '');

  // While nobody has given this person anything, coming back to the window asks again.
  $effect(() => {
    if (!actor.hasNoAccess) return;
    const askAgain = () => {
      void actor.reread();
      void containers.refresh();
    };
    window.addEventListener('focus', askAgain);
    return () => window.removeEventListener('focus', askAgain);
  });

  const twistLabel = (name: string, isOpen: boolean) => t(isOpen ? 'app.nav.close_branch' : 'app.nav.open_branch', { name });
  const refusalLabel = (refusal: 'self' | 'no-children' | 'other-part' | 'too-deep') => t(`app.nav.refused_${refusal.replace('-', '_')}`);
</script>

<div class="column" data-rail={isRail ? '' : undefined}>
  {#if topRows.length > 0}
  <ul class="foryou" aria-label={t('app.nav.for_you')}>
    {#each topRows as row (row.id)}
      <NavItem
        label={t(row.code)}
        icon={row.icon}
        href={row.target.kind === 'route' ? row.target.path : undefined}
        isCurrent={isCurrentTop(row.id)}
        figure={topFigure(row.id)}
        isFigureUrgent={row.id === 'approvals'}
        {isRail}
        onnavigate={() => row.target.kind === 'route' && onnavigate(row.target.path)}
      />
    {/each}
  </ul>
  {/if}

  {#if !isTreeReady}
    <div class="grow">{#if !isRail}<div aria-busy="true" class="pad"><Skeleton lines={4} /></div>{/if}</div>
  {:else if failure}
    <div class="grow pad">{#if !isRail}<FailureState problem={failure} onRetry={() => containers.refresh()} />{/if}</div>
  {:else if actor.hasNoAccess}
    <div class="grow pad">
      {#if actor.portals.length > 0}
        <ul class="plain" aria-label={t('app.nav.portals')}>
          {#each actor.portals as each (each.collection_id)}
            <NavItem label={each.name} icon="inbox" href={`/portal/${each.collection_id}`} {isRail} onnavigate={() => onnavigate(`/portal/${each.collection_id}`)} />
          {/each}
        </ul>
      {:else if !isRail}
        <EmptyState kind="unused" title={t('app.workspace.no_access')} icon="hub" />
      {/if}
    </div>
  {:else if isRail}
    <ul class="grow rail" aria-label={t('app.workspace.title')}>
      {#if level.kind === 'hub' && hub}
        <NavItem label={hub.name} icon={markOf(hub)} color={hub.color_token ?? undefined} href={`/hubs/${hub.id}`} isCurrent={tool === 'overview'} isRail onnavigate={() => onnavigate(`/hubs/${hub.id}`)} />
        {#each hubToolRows as each (each.id)}
          <NavItem label={t(each.code)} icon={each.icon} href={toolPath(hub.id, each.id)} isCurrent={tool === each.id} isRail onnavigate={() => onnavigate(toolPath(hub.id, each.id))} />
        {/each}
      {:else}
        {#each productSets.shown as row (row.id)}
          <NavItem label={t(row.code)} icon={row.icon} color={row.color} href={row.path} isCurrent={productCurrent(row.routes)} isRail onnavigate={() => onnavigate(row.path)} />
        {/each}
        {#each orderedHubs.slice(0, HUBS_SHOWN) as each (each.id)}
          <NavItem label={each.name} icon={markOf(each)} color={each.color_token ?? undefined} href={`/hubs/${each.id}`} isRail onnavigate={() => onnavigate(`/hubs/${each.id}`)} />
        {/each}
      {/if}
    </ul>
  {:else if level.kind === 'hub' && hub}
    <NavLevel
      levelKey={hub.id}
      label={hub.name}
      hub={{
        name: hub.name,
        icon: hub.private && !chosenSymbol(hub) ? 'lock' : markOf(hub),
        color: hub.color_token ?? DEFAULT_TILE,
        backLabel: t('app.nav.all_hubs'),
        onback: () => (isBrowsingAll = true),
        starLabel: t(isHubStarred ? 'app.nav.unstar' : 'app.nav.star', { name: hub.name }),
        isStarred: isHubStarred,
        onstar: () => void stars.toggle('HUB', hub.id),
        menu: hubMenuOf(hub.id),
      }}
    >
      {#snippet switcher()}
        <NavFlyout label={t('app.nav.switch_hub')} trigger="icon" icon="chevrons-up-down" triggerLabel={t('app.nav.switch_hub')} searchLabel={t('app.nav.find_hub')} bind:search={switchSearch} bind:isOpen={isSwitchOpen}>
          {#if starredHubs.length > 0}
            <NavGroup label={t('app.nav.starred')}>
              {#each starredHubs as each (each.id)}
                <NavItem label={each.name} icon={markOf(each)} color={each.color_token ?? undefined} href={`/hubs/${each.id}`} onnavigate={() => switchTo(each.id)} />
              {/each}
            </NavGroup>
          {/if}
          {#if visitedSwitch.length > 0}
            <NavGroup label={t('app.nav.recently_visited')}>
              {#each visitedSwitch as each (each.id)}
                <NavItem label={each.name} icon={markOf(each)} color={each.color_token ?? undefined} href={`/hubs/${each.id}`} onnavigate={() => switchTo(each.id)} />
              {/each}
            </NavGroup>
          {/if}
          {#if matchedHubs.length > 0}
            <NavGroup label={t('app.nav.hubs')}>
              {#each matchedHubs as each (each.id)}
                <NavItem label={each.name} icon={markOf(each)} color={each.color_token ?? undefined} href={`/hubs/${each.id}`} onnavigate={() => switchTo(each.id)} />
              {/each}
            </NavGroup>
          {/if}
          <ul class="plain">
            <NavItem label={t('app.nav.all_hubs')} icon="hub" tone="link" onnavigate={() => { isSwitchOpen = false; isBrowsingAll = true; }} />
          </ul>
        </NavFlyout>
      {/snippet}
      {#snippet tools()}
        <NavItem label={t('app.nav.overview')} icon="hub" href={`/hubs/${hub.id}`} isCurrent={tool === 'overview'} onnavigate={() => onnavigate(`/hubs/${hub.id}`)} />
        {#each hubToolRows as each (each.id)}
          {#if each.id === 'queues'}
            <NavItem
              label={t(each.code)}
              icon={each.icon}
              href={toolPath(hub.id, 'queues')}
              twist={queueList.length > 0 ? (isQueuesOpen ? 'expanded' : 'collapsed') : undefined}
              twistLabel={twistLabel(t(each.code), isQueuesOpen)}
              isCurrent={tool === 'queues' && !currentQueue}
              ontoggle={(open) => (isQueuesOpen = open)}
              onnavigate={() => onnavigate(toolPath(hub.id, 'queues'))}
            />
            {#if isQueuesOpen}
              {#each isAllQueues ? queueList : queueList.slice(0, QUEUES_SHOWN) as row (row.id)}
                <NavItem
                  label={queueName(row)}
                  icon={row.isStarred ? 'star' : 'columns-3'}
                  level={1}
                  href={queuePath(hub.id, row)}
                  figure={row.count === null ? undefined : String(row.count)}
                  isFigureUrgent={row.isUrgent}
                  figureLabel={row.count === null ? undefined : t(row.isUrgent ? 'app.nav.queue_count_urgent' : 'app.nav.queue_count', { count: row.count })}
                  isCurrent={currentQueue === (row.viewId ?? row.key)}
                  onnavigate={() => onnavigate(queuePath(hub.id, row))}
                />
              {/each}
              {#if !isAllQueues && queueList.length > QUEUES_SHOWN}
                <NavItem label={t('app.nav.more_count', { count: queueList.length - QUEUES_SHOWN })} icon="ellipsis" tone="more" level={1} onnavigate={() => (isAllQueues = true)} />
              {/if}
            {/if}
          {:else}
            <NavItem
              label={t(each.code)}
              icon={each.icon}
              href={toolPath(hub.id, each.id)}
              isCurrent={tool === each.id}
              figure={toolFigure(each.id)}
              isFigureUrgent={each.id === 'inventory'}
              onnavigate={() => onnavigate(toolPath(hub.id, each.id))}
            />
          {/if}
        {/each}
      {/snippet}

      {#if collectionNodes.length > 0}
        <NavGroup
          label={t('app.nav.collections')}
          collapseLabel={t('app.nav.fold', { name: t('app.nav.collections') })}
          add={mayStructure ? { label: t('app.nav.new_collection_short'), onselect: () => { collectionHubId = hub.id; isCreatingCollection = true; } } : undefined}
          tree={collectionNodes}
          treeLabel={t('app.nav.collections_of', { hub: hub.name })}
          current={currentCollection}
          rowMenu={placeMenuOf}
          canOrder={() => mayStructure}
          onnavigate={(id) => onnavigate(`/collections/${id}`)}
          onplace={place(collections)}
          onmovekey={moveKey(collections, collectionNodes, () => mayStructure)}
          {refusalLabel}
          limit={COLLECTIONS_SHOWN}
          moreLabel={(hidden) => t('app.nav.more_count', { count: hidden })}
          filterFrom={COLLECTIONS_FILTER_FROM}
          filterLabel={t('app.nav.filter_collections')}
          bind:filter={collectionFilter}
        />
      {:else if mayStructure}
        <NavGroup label={t('app.nav.collections')} add={{ label: t('app.nav.new_collection_short'), onselect: () => { collectionHubId = hub.id; isCreatingCollection = true; } }}>
          <NavItem label={t('app.nav.create_first_collection')} icon="plus" tone="link" onnavigate={() => { collectionHubId = hub.id; isCreatingCollection = true; }} />
        </NavGroup>
      {/if}
      {#if rankFailure}<p class="failure">{rankFailure}</p>{/if}

      {#if hasContents && contentTree.length > 0}
        <NavGroup
          label={t('app.nav.contents')}
          count={String(contentsCount(contentNodes))}
          collapseLabel={t('app.nav.fold', { name: t('app.nav.contents') })}
          add={contentsAdd}
          menu={contentsMenu}
          tree={contentTree}
          treeLabel={t('app.nav.contents_of', { hub: hub.name })}
          bind:expanded
          current={currentContent}
          {twistLabel}
          onnavigate={(id) => onnavigate(boardRows.some((each) => each.id === id) ? `/whiteboards/${id}` : `/pages/${id}`)}
          {refusalLabel}
          limit={CONTENTS_SHOWN}
          moreLabel={(hidden) => t('app.nav.more_count', { count: hidden })}
          filterFrom={CONTENTS_FILTER_FROM}
          filterLabel={t('app.nav.filter_contents')}
          bind:filter={contentsFilter}
          kinds={isOn('KNOWLEDGE') && isOn('WHITEBOARD') ? [{ id: 'all', label: t('app.nav.kind_all') }, { id: 'page', label: t('app.nav.kind_pages') }, { id: 'whiteboard', label: t('app.nav.kind_whiteboards') }] : undefined}
          bind:kind={contentsKind}
          kindLabel={t('app.nav.kind_label')}
        />
      {:else if hasContents && contentsAdd}
        <NavGroup label={t('app.nav.contents')} add={contentsAdd}>
          <NavItem label={t(isOn('KNOWLEDGE') ? 'app.nav.create_first_page' : 'app.nav.create_first_whiteboard')} icon="plus" tone="link" onnavigate={() => hubId && onnavigate(isOn('KNOWLEDGE') ? `/hubs/${hubId}/pages?new=1` : `/hubs/${hubId}/whiteboards?new=1`)} />
        </NavGroup>
      {/if}
    </NavLevel>
  {:else}
    <NavLevel levelKey="workspace" label={t('app.nav.workspace_band')}>
      <ReplicaMark state={containers.hubsState} />
      {#if recents.rows.length > 0}
        <ul class="plain">
          <li class="flyout">
            <NavFlyout label={t('app.nav.recent')} trigger="row" icon="rotate-ccw" triggerLabel={t('app.nav.recent')}>
              <ul class="plain">
                {#each recents.rows as row (row.kind + row.id)}
                  <NavItem label={row.title} icon={RECENT_ICONS[row.kind]} href={RECENT_PATHS[row.kind](row.id)} onnavigate={() => onnavigate(RECENT_PATHS[row.kind](row.id))} />
                {/each}
              </ul>
            </NavFlyout>
          </li>
        </ul>
      {/if}
      {#if starRows.length > 0}
        <NavGroup label={t('app.nav.starred')}>
          {#each starRows as row (row.key)}
            <NavItem label={row.label} icon={row.icon} color={row.color} href={row.href} figure={row.figure} isFigureUrgent={row.isUrgent} isCurrent={route.path === row.href} onnavigate={() => onnavigate(row.href)} />
          {/each}
        </NavGroup>
      {/if}
      {#if productSets.shown.length + productSets.more.length > 0}
        <NavGroup label={t('app.nav.products')} menu={customise}>
          {#each isAllProducts ? [...productSets.shown, ...productSets.more] : productSets.shown as row (row.id)}
            <NavItem label={t(row.code)} icon={row.icon} color={row.color} href={row.path} isCurrent={productCurrent(row.routes)} onnavigate={() => onnavigate(row.path)} />
          {/each}
          {#if !isAllProducts && productSets.more.length > 0}
            <NavItem label={t('app.nav.more_count', { count: productSets.more.length })} icon="ellipsis" tone="more" onnavigate={() => (isAllProducts = true)} />
          {/if}
        </NavGroup>
      {/if}
      <NavGroup
        label={t('app.nav.hubs')}
        add={createHubLabel ? { label: createHubLabel, onselect: () => (isCreatingHub = true) } : undefined}
        menu={hubsMenu}
        tree={hubNodes}
        treeLabel={t('app.nav.hubs')}
        rowMenu={placeMenuOf}
        canOrder={(id) => hubsOrder === 'MANUAL' && mayRankIn(id)}
        onnavigate={(id) => onnavigate(`/hubs/${id}`)}
        onplace={place(shownHubs)}
        onmovekey={moveKey(shownHubs, hubNodes, (id) => hubsOrder === 'MANUAL' && mayRankIn(id))}
        {refusalLabel}
        limit={HUBS_SHOWN}
        moreLabel={(hidden) => t('app.nav.more_hubs', { count: hidden })}
      >
        {#if hubNodes.length === 0}
          {#if createHubLabel}
            <NavItem label={createHubLabel} icon="plus" tone="link" onnavigate={() => (isCreatingHub = true)} />
          {:else}
            <li class="quiet">{t('app.workspace.no_hubs')}</li>
          {/if}
        {/if}
      </NavGroup>
      {#if rankFailure}<p class="failure">{rankFailure}</p>{/if}
    </NavLevel>
  {/if}

  <ul class="foot" aria-label={t('app.nav.keeping')}>
    {#each footRows as row (row.id)}
      <NavItem
        label={t(row.code)}
        icon={row.icon}
        href={row.path}
        figure={footFigure(row.id)}
        isCurrent={row.routes.includes(route.name ?? '') || (row.id === 'administration' && route.area === 'administration')}
        {isRail}
        onnavigate={() => goFoot(row.id, row.path)}
      />
    {/each}
  </ul>
</div>

<CreateContainerDialog
  bind:isOpen={isCreatingHub}
  type="HUB"
  isPrivateOnly={creation === 'private'}
  oncreated={(id, landing) => onnavigate(landing ?? `/hubs/${id}`)}
/>
{#if collectionHubId}
  <CreateContainerDialog bind:isOpen={isCreatingCollection} type="COLLECTION" parentId={collectionHubId} oncreated={(id) => onnavigate(`/collections/${id}`)} />
{/if}

<style>
  .column {
    display: flex;
    flex-direction: column;
    block-size: 100%;
    min-block-size: 0;
  }

  .foryou,
  .foot,
  .plain,
  .rail {
    margin: 0;
    padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-100);
    list-style: none;
  }

  .plain { padding: 0; }
  .foryou { border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .foot { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .grow { flex: 1; min-block-size: 0; overflow-y: auto; }
  .pad { padding: var(--sp-150); }
  .flyout { list-style: none; }
  .quiet { list-style: none; padding: var(--sp-050) var(--sp-100); color: var(--text-subtle); font-size: var(--fs-075); }
  .failure { margin: 0; padding-inline: var(--sp-100); color: var(--text-danger); font-size: var(--fs-075); }
</style>
