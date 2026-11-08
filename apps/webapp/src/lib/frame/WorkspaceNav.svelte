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
    Button,
    Dialog,
    EmptyState,
    Icon,
    Menu,
    NavFlyout,
    NavGroup,
    NavItem,
    NavLevel,
    ProductTile,
    Sheet,
    Skeleton,
    VisuallyHidden,
    ancestorsOf,
    applyPlacement,
    moveIntent,
    positionOf,
    type MenuItem,
    type MoveKey,
    type NavTreeNode,
    type RowAction,
    type TreePlacement,
  } from '@hubtask/design-system/components';
  import type { AgentRequestList, ApprovalList, Container, GoalList, HubQueueList, CmdbHealth, ItemQueryResult, PageList, ReleaseList, SprintList, Star, WhiteboardList } from '@hubtask/sync-engine';

  import PlacePicker, { type PickedPlace } from './PlacePicker.svelte';
  import { toast } from './toasts.svelte.ts';
  import { millisecondsOf } from './toasts.ts';
  import { carried, entryTarget } from './entryDrop.svelte.ts';
  import CollectionMoveDialog from '../workspace/CollectionMoveDialog.svelte';
  import PositionDialog from '../workspace/PositionDialog.svelte';
  import { pages as pagesStore } from '../data/pages.svelte.ts';
  import { whiteboards } from '../data/whiteboards.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { attachEntryGestures } from '../entries/entryGestures.ts';
  import { boardMoveBody, pageMoveBody, pickerPlaces, placementOf, reordered } from '../moving.ts';

  import FailureState from './FailureState.svelte';
  import ReplicaMark from './ReplicaMark.svelte';
  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import { DEFAULT_TILE, chosenSymbol, markOf } from '../workspace/look.ts';
  import { placeActions } from '../workspace/placeActions.ts';

  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { requestsPath } from '../data/agents.ts';
  import { myApprovalsPath } from '../data/approvals.svelte.ts';
  import { holds, holdsHere, hubCreationHere, reachesAdministrationArea, roleOrder } from '../data/capability.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
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
    SECTION_OPEN_AT_FIRST,
    levelOf,
    navHubs,
    navProducts,
    placedIndex,
    withHidden,
    queuePath,
    queueRows,
    viewStarPath,
    switchPath,
    toolOf,
    toolPath,
    type ContentNode,
    type ContentsMode,
    type HubTool,
    type HubsOrder,
    type NavLevel as Level,
    type NavSection,
  } from '../sidebar.ts';
  import { leftToWebApp, type ProductRow } from '../navigation.ts';
  import { device } from '../device.svelte.ts';
  import { contentsStep, hubStep, queuesStep, type PhoneStep } from '../phone.ts';
  import { page } from './page.svelte.ts';
  import { actsAsDesk } from '../portal/portal.ts';
  import { platform } from '../platform/index.ts';
  import { needsConnection, renderProblem } from '../problem.ts';
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
    /**
     * A step of the phone's Browse stack (§9) instead of the sidebar: the workspace level, a hub,
     * its queues or its contents (under a page), drawn as touch rows from the same reads.
     */
    step?: PhoneStep;
    onnavigate: (path: string) => void;
  }

  const { route, located, isRail = false, hasForYou = true, step, onnavigate }: Props = $props();

  const workspaceOn = $derived(manifest.value?.modules_on);
  const hasJumble = $derived(holdsHere('READ') === true);
  const topRows = $derived(forYou({ hasJumble, modulesOn: workspaceOn, besideBar: !hasForYou || step !== undefined }));
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
  const level = $derived<Level>(
    step ? (step.kind === 'workspace' ? { kind: 'workspace' } : { kind: 'hub', hubId: step.hubId }) : isBrowsingAll ? { kind: 'workspace' } : routeLevel,
  );
  const hubId = $derived(level.kind === 'hub' ? level.hubId : undefined);
  const hub = $derived(hubId ? containers.find(hubId) : undefined);
  const tool = $derived(toolOf(route));

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
  const baseContentTree = $derived(contentNodes.map(toTree));

  // A content move is drawn at once, marked *waiting* until the server answers (P-11); a refusal
  // springs it back with its reason. The drawn placement is let go when the reread arrives.
  let pendingContent = $state<{ id: string; placement: TreePlacement; isWaiting: boolean } | undefined>(undefined);
  const markWaiting = (nodes: readonly NavTreeNode[], id: string): NavTreeNode[] =>
    nodes.map((node) => ({
      ...node,
      ...(node.id === id ? { waitingLabel: t('app.nav.waiting') } : {}),
      ...(node.children ? { children: markWaiting(node.children, id) } : {}),
    }));
  const contentTree = $derived.by(() => {
    if (!pendingContent) return baseContentTree;
    const moved = applyPlacement(baseContentTree, pendingContent.id, pendingContent.placement);
    return pendingContent.isWaiting ? markWaiting(moved, pendingContent.id) : moved;
  });
  $effect(() => {
    void pageRows;
    void boardRows;
    untrack(() => {
      if (pendingContent && !pendingContent.isWaiting) pendingContent = undefined;
    });
  });

  /** A page or a board to a placement, by drag, key or *Move to…*: `movePage` or `moveWhiteboard`. */
  async function placeContent(id: string, placement: TreePlacement, isUndo = false): Promise<boolean> {
    const board = boardRows.find((each) => each.id === id);
    const pageRow = board ? undefined : pageRows.find((each) => each.id === id);
    const before = placementOf(contentTree, id);
    if ((!board && !pageRow) || !before) return false;
    pendingContent = { id, placement, isWaiting: true };
    // The branch it goes into opens, so the row is seen where it landed.
    if (placement.parentId && !expanded.includes(placement.parentId)) expanded = [...expanded, placement.parentId];
    rankFailure = undefined;
    try {
      if (board) await whiteboards.move(board, boardMoveBody(placement));
      else if (pageRow) await pagesStore.move(pageRow, pageMoveBody(placement));
    } catch (error) {
      pendingContent = undefined;
      failed(error);
      return false;
    }
    const after = applyPlacement(contentTree, id, placement);
    pendingContent = { id, placement, isWaiting: false };
    land(id);
    const at = positionOf(after, id);
    const said = at?.parent
      ? t('app.nav.moved', { position: at.position, count: at.of, parent: at.parent.label })
      : t('app.nav.moved_top', { position: at?.position ?? 1, count: at?.of ?? 1 });
    if (isUndo) announcer.say(said);
    else offerMoved(said, () => placeContent(id, before, true));
    return true;
  }
  const contentKey = (id: string, key: MoveKey) => {
    if (!mayWrite) return void announcer.say(t('app.nav.may_not_order'));
    const intent = moveIntent(key, contentTree, id, document.documentElement.dir === 'rtl' ? 'rtl' : 'ltr');
    if (!intent) return;
    if ('refusal' in intent) return void announcer.say(refusalLabel(intent.refusal));
    void placeContent(id, intent.placement);
  };

  // *Move to…* (`M` or ⋯): the hub's contents as places, what cannot take the row greyed (§5.4).
  let movingContent = $state<string | undefined>(undefined);
  const contentPlaces = $derived(movingContent ? pickerPlaces(contentTree, movingContent, t('app.nav.top_of_contents')) : []);
  const pickerRows = $derived<PickedPlace[]>(
    contentPlaces.map((place) => ({
      key: place.id ?? '',
      label: place.label,
      depth: place.id === null ? 0 : place.depth + 1,
      icon: place.id === null ? 'hub' : boardRows.some((each) => each.id === place.id) ? 'layout-template' : 'file-text',
      reason: place.refusal ? refusalLabel(place.refusal) : undefined,
    })),
  );
  const movingContentName = $derived(movingContent ? (pageRows.find((each) => each.id === movingContent)?.title ?? boardRows.find((each) => each.id === movingContent)?.title ?? '') : '');
  function pickContent(key: string) {
    const id = movingContent;
    const place = contentPlaces.find((each) => (each.id ?? '') === key);
    movingContent = undefined;
    if (id && place) void placeContent(id, place.placement);
  }
  // A page's ⋯ archives it or puts it in the trash, a board's only the trash: a board has no
  // archive (§5.1). Each offers *Undo*.
  const contentLifecycle = (id: string): MenuItem[] => [
    ...(pageRows.some((each) => each.id === id) ? [{ id: 'archive', label: t('app.workspace.archive'), icon: 'archive' as const, hasSeparatorBefore: true }] : []),
    { id: 'trash', label: t('app.workspace.trash'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: !pageRows.some((each) => each.id === id) },
  ];
  async function contentAct(id: string, act: string) {
    if (act === 'move') return void (movingContent = id);
    const pageRow = pageRows.find((each) => each.id === id);
    const board = boardRows.find((each) => each.id === id);
    try {
      if (pageRow && act === 'archive') {
        const archived = await pagesStore.change(pageRow, { archived: true });
        offerMoved(t('app.nav.archived', { title: pageRow.title }), () => pagesStore.change(archived, { archived: false }).then(() => true, () => false));
      } else if (pageRow && act === 'trash') {
        const trashed = await pagesStore.trash(pageRow);
        offerMoved(t('app.pages.trashed', { count: trashed.trashed }), () => pagesStore.restore(pageRow.hub_id, pageRow.id).then(() => true, () => false));
      } else if (board && act === 'trash') {
        await whiteboards.trash(board);
        offerMoved(t('app.whiteboards.trashed', { title: board.title }), () => whiteboards.restore(board).then(() => true, () => false));
      }
    } catch (error) {
      failed(error);
    }
  }
  const contentRowMenu = (id: string): RowAction | undefined =>
    mayWrite
      ? {
          label: t('app.nav.row_actions', { name: pageRows.find((each) => each.id === id)?.title ?? boardRows.find((each) => each.id === id)?.title ?? '' }),
          items: [{ id: 'move', label: t('app.nav.move_to'), icon: 'arrow-right-left' }, ...contentLifecycle(id)],
          onselect: (act) => void contentAct(id, act ?? 'move'),
        }
      : undefined;
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

  /** A hub's or collection's ⋯, the page menu's items (`placeActions`), run by the page itself. */
  function placeMenuOf(id: string, options: { readonly offersPosition?: boolean } = {}): RowAction | undefined {
    const container = containers.find(id);
    if (!container || isRail) return undefined;
    const actions = placeActions(id, {
      offersPosition: options.offersPosition,
      onnavigate,
      onmove: (collectionId) => (movingCollection = containers.find(collectionId)),
    });
    return actions ? { label: t('app.nav.row_actions', { name: container.name }), items: [...actions.items], onselect: (act) => actions.onselect(act ?? '') } : undefined;
  }

  /**
   * The hub head's ⋯ (§2.4 item 1): its link first, for every reader, then the place menu's items -
   * so the head always has one, while its settings stay only there.
   */
  function hubMenuOf(id: string): RowAction | undefined {
    const container = containers.find(id);
    if (!container || isRail) return undefined;
    const place = placeMenuOf(id, { offersPosition: false });
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

  // ---- Moving (§5.4): the landing, Undo for 8 s and Cmd/Ctrl+Z, the position said -------------
  let landed = $state<string | undefined>(undefined);
  let landTimer: ReturnType<typeof setTimeout> | undefined;
  /** The row that landed is marked for the settle and the highlight, then let go. */
  function land(id: string) {
    const css = getComputedStyle(document.documentElement);
    const ms = (millisecondsOf(css.getPropertyValue('--dur-settle')) ?? 0) + (millisecondsOf(css.getPropertyValue('--dur-highlight')) ?? 0);
    landed = id;
    clearTimeout(landTimer);
    landTimer = setTimeout(() => (landed = undefined), ms);
  }
  /** Says where the row stands now and offers the way back; the way back offers none. */
  function offerMoved(text: string, back: () => Promise<boolean>) {
    toast.show({
      text,
      undo: async () => {
        if (await back()) toast.show({ text: t('app.undo.done'), tone: 'success' });
      },
    });
  }
  const failed = (error: unknown) => {
    // Offline a move is refused rather than queued (the push frame carries no move of a page, a
    // board or a container), so the sidebar says that, not only that the server is away.
    rankFailure = needsConnection(error as never) ? t('app.nav.move_offline') : renderProblem(error as never, messages).message;
    announcer.say(rankFailure);
  };

  /** `Alt+↑/↓` or a drag among hubs or a hub's collections: the writes the page menu sends. */
  async function rank(id: string, siblings: readonly Container[], to: number, isUndo = false): Promise<boolean> {
    const container = containers.find(id);
    if (!container || to < 0 || to >= siblings.length) return false;
    const from = siblings.findIndex((each) => each.id === id);
    rankFailure = undefined;
    try {
      if (container.type === 'HUB') await containers.reorder(id, siblings, to, crypto.randomUUID());
      else if (container.parent_id) await containers.move(id, container.parent_id, anchorFor(siblings, id, to), crypto.randomUUID());
    } catch (error) {
      failed(error);
      return false;
    }
    land(id);
    const said = t('app.nav.moved_top', { position: to + 1, count: siblings.length });
    if (isUndo || from < 0) announcer.say(said);
    else {
      const parent = container.parent_id;
      offerMoved(said, () => rank(id, parent ? containers.collectionsOf(parent) : containers.hubs.filter((each) => !live.hasLost(each.id)), from, true));
    }
    return true;
  }
  const place = (siblings: readonly Container[]) => (id: string, placement: TreePlacement) =>
    void rank(id, siblings, placedIndex(siblings.map((each) => each.id), id, placement.afterId));
  /** A rank key moves only what the reader may order (P-05): the row draws no key where it may not. */
  const moveKey = (siblings: readonly Container[], nodes: readonly NavTreeNode[], mayOrder: (id: string) => boolean) => (id: string, key: MoveKey) => {
    if (!mayOrder(id)) return void announcer.say(t('app.nav.may_not_order'));
    const intent = moveIntent(key, nodes, id);
    if (intent && 'placement' in intent) place(siblings)(id, intent.placement);
  };

  // ---- Workspace level ------------------------------------------------------------------------
  /** A section's fold, or its *More*'s, as this device left it (R-12). */
  const sectionOpen = (key: NavSection) => ({
    get isOpen() {
      return device.isSectionOpen(key, SECTION_OPEN_AT_FIRST[key]);
    },
    set isOpen(open: boolean) {
      device.setSectionOpen(key, open);
    },
  });
  const sections = {
    starred: sectionOpen('starred'),
    products: sectionOpen('products'),
    productsMore: sectionOpen('products-more'),
    hubs: sectionOpen('hubs'),
    hubsMore: sectionOpen('hubs-more'),
    collections: sectionOpen('collections'),
    contents: sectionOpen('contents'),
  };

  const productSets = $derived(navProducts({ modulesOn: workspaceOn, order: actor.account?.nav_products, hidden: actor.account?.nav_products_hidden }));
  // The products' order is the person's (§7.5 `nav_products`): dragged or `Alt+↑/↓` among the
  // shown rows; a hidden one stands behind *More* and is shown again from its ⋯ (R-3).
  const productNode = (row: ProductRow): NavTreeNode => ({
    id: row.module, label: t(row.code), icon: row.icon, color: row.color, href: row.path, part: 'products', canHoldChildren: false,
  });
  const productNodes = $derived(productSets.shown.map(productNode));
  const hiddenProductNodes = $derived(productSets.more.map(productNode));
  const productPaths = $derived(Object.fromEntries(productSets.all.map((row) => [row.module, row.path])));
  const currentProduct = $derived(productSets.all.find((row) => productCurrent(row.routes))?.module);
  async function placeProduct(module: string, afterId: string | null, isUndo = false): Promise<boolean> {
    const before: string[] = productSets.all.map((row) => row.module);
    const at = before.indexOf(module);
    const next = reordered(before, module, afterId);
    const id = actor.account?.id;
    if (!id) return false;
    try {
      // The modules as the account's preference names them (§7.5).
      await preferences.setAccount(id, { nav_products: next as never });
      await actor.reread();
    } catch (error) {
      failed(error);
      return false;
    }
    land(module);
    const said = t('app.nav.moved_top', { position: next.indexOf(module) + 1, count: next.length });
    if (isUndo) announcer.say(said);
    else offerMoved(said, () => placeProduct(module, at > 0 ? (before[at - 1] ?? null) : null, true));
    return true;
  }
  const productKey = (id: string, key: MoveKey) => {
    const intent = moveIntent(key, productNodes, id);
    if (intent && 'placement' in intent) void placeProduct(id, intent.placement.afterId);
  };
  async function writePreference(change: Record<string, unknown>): Promise<boolean> {
    const id = actor.account?.id;
    if (!id) return false;
    try {
      await preferences.setAccount(id, change);
      void actor.reread();
      return true;
    } catch (error) {
      announcer.say(renderProblem(error as never, messages).message);
      return false;
    }
  }

  /**
   * Hides a product or a hub behind its section's *More*, or shows it again (R-3, R-4): the
   * account's whole list written, so another device draws the same; said, with the way back.
   */
  async function setHidden(field: 'nav_products_hidden' | 'nav_hubs_hidden', id: string, name: string, isHidden: boolean, isUndo = false): Promise<boolean> {
    const list: readonly string[] | null | undefined = actor.account?.[field];
    if (!(await writePreference({ [field]: withHidden(list, id, isHidden) }))) return false;
    const said = t(isHidden ? 'app.nav.row_hidden' : 'app.nav.row_shown', { name });
    if (isUndo) announcer.say(said);
    else toast.show({ text: said, undo: async () => void (await setHidden(field, id, name, !isHidden, true)) });
    return true;
  }
  /** A product row's ⋯ - and its long press on touch: *Hide* where shown, *Show* behind *More*. */
  function productMenuOf(module: string): RowAction | undefined {
    const row = productSets.all.find((each) => each.module === module);
    if (!row || isRail || !actor.account) return undefined;
    const isHidden = productSets.more.includes(row);
    return {
      label: t('app.nav.row_actions', { name: t(row.code) }),
      items: [hideItem(isHidden)],
      onselect: (act) => act && void setHidden('nav_products_hidden', module, t(row.code), act === 'hide'),
    };
  }
  const hideItem = (isHidden: boolean): MenuItem =>
    isHidden ? { id: 'show', label: t('app.nav.show_in_sidebar'), icon: 'eye' } : { id: 'hide', label: t('app.nav.hide_from_sidebar'), icon: 'eye-off' };

  const hubsOrder = $derived<HubsOrder>((actor.account?.nav_hubs_order as HubsOrder | null | undefined) ?? 'MANUAL');
  const visitedHubs = $derived([...new Set(recents.rows.map((row) => (row.kind === 'hub' ? row.id : row.kind === 'collection' ? row.placeId : undefined)).filter((id): id is string => !!id))]);
  /** Every hub in sight, in the shared order - the siblings a hub is ranked among. */
  const shownHubs = $derived(containers.hubs.filter((each) => !live.hasLost(each.id)));
  const hubSets = $derived(navHubs(shownHubs, { order: hubsOrder, visited: visitedHubs, hidden: actor.account?.nav_hubs_hidden, locale: messages.locale }));
  const orderedHubs = $derived(hubSets.shown);
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
  const hiddenHubNodes = $derived(hubSets.more.map((each) => ({ ...hubNode(each), figure: undefined, figureLabel: undefined })));
  const hubsMenu = $derived<RowAction>({
    label: t('app.nav.order_hubs'),
    items: (['MANUAL', 'RECENT', 'NAME'] as const).map((mode) => ({ id: mode, label: t(`app.nav.hubs_${mode.toLowerCase()}`), ...(mode === hubsOrder ? { icon: 'check' as const } : {}) })),
    onselect: (mode) => mode && void writePreference({ nav_hubs_order: mode === 'MANUAL' ? null : mode }),
  });
  /**
   * A hub row's ⋯ at the workspace level: the place's own items with *Change position* - only here,
   * where every hub is listed - and *Hide* or *Show* (R-4).
   */
  function hubRowMenuOf(id: string): RowAction | undefined {
    const container = containers.find(id);
    if (!container || isRail || !actor.account) return undefined;
    const isHidden = hubSets.more.includes(container);
    const place = placeMenuOf(id, { offersPosition: !isHidden });
    // The person's own act first; the place's items after it, the trash still last and alone.
    return {
      label: t('app.nav.row_actions', { name: container.name }),
      items: [hideItem(isHidden), ...(place?.items ?? []).map((item, index) => (index === 0 ? { ...item, hasSeparatorBefore: true } : item))],
      onselect: (act) => {
        if (act === 'hide' || act === 'show') return void setHidden('nav_hubs_hidden', id, container.name, act === 'hide');
        if (act === 'position') return void toSharedOrder().then((isShared) => isShared && (positioningHub = container));
        place?.onselect(act);
      },
    };
  }
  /**
   * Hubs move in the shared order (§5.4). Moved while the person reads them by name or by visit,
   * the list turns to the shared order first and says so (R-4): a drag lands after the hub it was
   * dropped under; `Alt+↑/↓` only turns the order, and the next press moves.
   */
  async function toSharedOrder(): Promise<boolean> {
    if (hubsOrder === 'MANUAL') return true;
    if (!(await writePreference({ nav_hubs_order: null }))) return false;
    announcer.say(t('app.nav.hubs_now_shared'));
    return true;
  }
  const mayMoveHub = (id: string) => mayRankIn(id);
  /** The hub *Change position* was chosen for: its dialog, over the shared order. */
  let positioningHub = $state<Container | undefined>(undefined);
  const moveHubBy = (id: string, offset: number) => {
    const from = shownHubs.findIndex((each) => each.id === id);
    if (from >= 0) void rank(id, shownHubs, from + offset);
  };
  async function placeHub(id: string, placement: TreePlacement) {
    if (!(await toSharedOrder())) return;
    await rank(id, shownHubs, placedIndex(shownHubs.map((each) => each.id), id, placement.afterId));
  }
  const hubKey = (id: string, key: MoveKey) => {
    if (!mayMoveHub(id)) return void announcer.say(t('app.nav.may_not_order'));
    if (hubsOrder !== 'MANUAL') return void toSharedOrder();
    const intent = moveIntent(key, hubNodes, id);
    if (intent && 'placement' in intent) void placeHub(id, intent.placement);
  };

  let isCreatingHub = $state(false);
  const creation = $derived(hubCreationHere());
  const createHubLabel = $derived(creation === 'shared' ? t('app.workspace.create_hub') : creation === 'private' ? t('app.workspace.create_private_hub') : undefined);

  /** A star as a row: its name from what is already read, or from its own read. */
  let starNames = $state<Record<string, string>>({});
  /** A starred view's place (there is no screen of a view by itself) and, for a queue, its hub. */
  let starViews = $state<Record<string, { href: string | undefined; hubId: string | undefined }>>({});
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
          if (kind === 'VIEW') {
            const view = data as { id: string; scope_type: string; scope_id: string | null; queue?: boolean };
            starViews = { ...starViews, [key]: { href: viewStarPath(view), hubId: view.scope_type === 'HUB' && view.queue ? (view.scope_id ?? undefined) : undefined } };
          }
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
  // A starred queue's count and its danger surface, from its hub's queues (§7.4) - one read per
  // hub, the same the hub level makes.
  let starQueues = $state<Record<string, HubQueueList['data']>>({});
  const starQueueHubs = $derived([...new Set(Object.values(starViews).map((each) => each.hubId).filter((each) => each !== undefined))].join(','));
  $effect(() => {
    const ids = starQueueHubs === '' ? [] : starQueueHubs.split(',');
    return untrack(() => {
      const stops = ids.map((at) => engine.subscribe<HubQueueList>({ path: `/hubs/${at}/queues` }, (next) => {
        if (next.status === 'ready') starQueues = { ...starQueues, [at]: next.data.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const starRows = $derived(
    stars.all
      .map((star) => {
        const container = star.kind === 'HUB' || star.kind === 'COLLECTION' ? containers.find(star.id) : undefined;
        const key = `${star.kind}:${star.id}`;
        const name = container?.name ?? starNames[key];
        const view = star.kind === 'VIEW' ? starViews[key] : undefined;
        if (!name || (star.kind === 'VIEW' && !view?.href)) return undefined;
        const queueHub = view?.hubId ? containers.find(view.hubId) : undefined;
        const queue = view?.hubId ? starQueues[view.hubId]?.find((each) => each.view_id === star.id) : undefined;
        const figure = container?.type === 'COLLECTION' ? figureOf(container)
          : queue && queue.count !== null ? { text: String(queue.count), isUrgent: (queue.at_risk ?? 0) > 0 } : undefined;
        return {
          key,
          // A queue's name says little without its hub: *About to breach · IT operations*.
          label: queueHub ? `${name} · ${queueHub.name}` : name,
          icon: container ? markOf(container) : STAR_ICONS[star.kind],
          color: container ? (container.color_token ?? DEFAULT_TILE) : undefined,
          href: view?.href ?? STAR_PATHS[star.kind](star.id),
          figure: figure?.text,
          isUrgent: figure?.isUrgent ?? false,
        };
      })
      .filter((row) => row !== undefined),
  );

  // Stars ordered by drag or `Alt+↑/↓` (UC-WRK-37 check 6): personal, without asking.
  const starNodes = $derived(
    starRows.map((row): NavTreeNode => ({
      id: row.key, label: row.label, icon: row.icon, color: row.color, href: row.href, figure: row.figure, isFigureUrgent: row.isUrgent, part: 'stars', canHoldChildren: false,
      // A starred collection takes an entry dropped on it (UC-WRK-06 check 8).
      acceptsEntries: row.key.startsWith('COLLECTION:'),
    })),
  );
  const starKeys = $derived(stars.all.map((each) => `${each.kind}:${each.id}`));
  async function placeStar(key: string, afterId: string | null, isUndo = false): Promise<boolean> {
    const before = starKeys;
    const at = before.indexOf(key);
    const next = reordered(before, key, afterId);
    try {
      await stars.reorder(next.map((each) => stars.all.find((star) => `${star.kind}:${star.id}` === each)!).filter(Boolean));
    } catch (error) {
      failed(error);
      return false;
    }
    land(key);
    const said = t('app.nav.moved_top', { position: next.indexOf(key) + 1, count: next.length });
    if (isUndo) announcer.say(said);
    else offerMoved(said, () => placeStar(key, at > 0 ? (before[at - 1] ?? null) : null, true));
    return true;
  }
  const starKey = (id: string, key: MoveKey) => {
    const intent = moveIntent(key, starNodes, id);
    if (intent && 'placement' in intent) void placeStar(id, intent.placement.afterId);
  };

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
  // An entry dropped onto a collection (UC-WRK-06 check 8): into the same hub at once with *Undo*;
  // into another hub's (reached through *Starred*) after the dialog with its warning. A collection
  // the reader may not write in takes no drop, and says why.
  const mayWriteIn = (collection: Container) =>
    holds(people.roleOf({ hubId: collection.parent_id ?? '', collectionId: collection.id }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted';
  // A collection's own memberships are read once an entry is carried, so its outline and its
  // refusal judge the reader's role there rather than a path nobody has opened yet.
  $effect(() => {
    if (!carried.id) return;
    const starred = stars.all.filter((star) => star.kind === 'COLLECTION').map((star) => containers.find(star.id)).filter((each) => each !== undefined);
    const shown = [...collections, ...starred];
    untrack(() => shown.forEach((each) => people.open({ hubId: each.parent_id ?? '', collectionId: each.id })));
  });
  const collectionOfRow = (rowId: string) => {
    const found = containers.find(rowId.startsWith('COLLECTION:') ? rowId.slice('COLLECTION:'.length) : rowId);
    return found?.type === 'COLLECTION' ? found : undefined;
  };
  const entryOver = $derived.by(() => {
    if (!carried.id || !carried.over) return undefined;
    const collection = collectionOfRow(carried.over);
    return collection && mayWriteIn(collection) ? carried.over : undefined;
  });
  let entryMove = $state<{ entryId: string; title: string; collection: Container } | undefined>(undefined);
  async function moveEntry(entryId: string, collection: Container, back?: { collectionId: string; parentId: string | null }): Promise<boolean> {
    const item = items.find(entryId);
    if (!item) return false;
    const from = { collectionId: item.collection_id, parentId: item.parent_id ?? null };
    try {
      const result = await items.move(entryId, back ?? { parentId: null, collectionId: collection.id }, item.version, crypto.randomUUID());
      const lost = result.dropped_references.length;
      const said = t('app.nav.entry_moved', { title: item.title, collection: collection.name }) + (lost > 0 ? ` ${t('app.move.dropped', { title: item.title })} ${lost}` : '');
      if (back) announcer.say(said);
      else {
        const home = containers.find(from.collectionId);
        offerMoved(said, () => (home ? moveEntry(entryId, home, from) : Promise.resolve(false)));
      }
      return true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return false;
    }
  }
  $effect(() =>
    entryTarget.set((rowId, entryId) => {
      const collection = collectionOfRow(rowId);
      const item = items.find(entryId);
      if (!collection || !item) return false;
      if (!mayWriteIn(collection)) {
        announcer.say(t('app.nav.entry_no_write', { collection: collection.name }));
        return true;
      }
      if (item.collection_id === collection.id) return true;
      const fromHub = containers.find(item.collection_id)?.parent_id;
      if (fromHub && fromHub === collection.parent_id) void moveEntry(entryId, collection);
      else entryMove = { entryId, title: item.title, collection };
      return true;
    }),
  );

  /** *Move to…* (`M` or ⋯) for a collection: the pre-check dialog (§5.3). */
  let movingCollection = $state<Container | undefined>(undefined);
  const moveCollection = (id: string) => {
    if (!mayStructure) return void announcer.say(t('app.nav.may_not_order'));
    movingCollection = containers.find(id);
  };
  let isCreatingCollection = $state(false);
  let collectionHubId = $state<string | undefined>(undefined);

  // ＋ on *Contents* creates at the top, ＋ on a page under it (§5.2): *Page · From a template… ·
  // Whiteboard*; a whiteboard has no children and so no ＋.
  const createItems = $derived([
    ...(isOn('KNOWLEDGE') ? [{ id: 'page', label: t('app.nav.create_page'), icon: 'file-text' as const }, { id: 'template', label: t('app.nav.create_from_template'), icon: 'copy' as const }] : []),
    ...(isOn('WHITEBOARD') ? [{ id: 'whiteboard', label: t('app.nav.create_whiteboard'), icon: 'layout-template' as const }] : []),
  ]);
  const createPath = (at: string, kind: string, under?: string) =>
    kind === 'whiteboard'
      ? `/hubs/${at}/contents?new=whiteboard${under ? `&parent=${under}` : ''}`
      : under ? `/pages/${under}?new=under` : `/hubs/${at}/pages?new=1`;
  const contentsAdd = $derived<RowAction | undefined>(
    hubId && mayWrite
      ? { label: t('app.nav.create_in_contents'), items: createItems, onselect: (kind) => hubId && onnavigate(createPath(hubId, kind ?? 'page')) }
      : undefined,
  );
  const contentRowAdd = (id: string): RowAction | undefined => {
    if (!hubId || !mayWrite || boardRows.some((each) => each.id === id)) return undefined;
    const title = pageRows.find((each) => each.id === id)?.title ?? '';
    return { label: t('app.nav.create_under', { name: title }), heading: t('app.nav.create_under', { name: title }), items: createItems, onselect: (kind) => hubId && onnavigate(createPath(hubId, kind ?? 'page', id)) };
  };
  const contentsMenu = $derived<RowAction>({
    label: t('app.nav.contents_menu'),
    items: [
      ...(['TREE', 'RECENT', 'NAME'] as const).map((mode) => ({ id: mode, label: t(`app.nav.mode_${mode.toLowerCase()}`), ...(mode === contentsMode ? { icon: 'check' as const } : {}) })),
      { id: 'all', label: t('app.nav.all_contents') },
    ],
    onselect: (choice) => {
      if (choice === 'all' && hubId) return onnavigate(`/hubs/${hubId}/contents`);
      if (choice) void writePreference({ nav_contents_mode: choice === 'TREE' ? null : choice });
    },
  });

  // The quick switch (§2.4 item 1): the same tool at the other hub where it has it.
  let switchSearch = $state('');
  let isSwitchOpen = $state(false);
  // The quick switch drops from the hub's head, under it and as wide as the sidebar (§2.4, ▾).
  const SWITCH_PLACEMENT = { side: 'block-end', align: 'start' } as const;
  const switchHubs = $derived(shownHubs.filter((each) => each.id !== hubId && each.name.toLocaleLowerCase(messages.locale).includes(switchSearch.trim().toLocaleLowerCase(messages.locale))));
  const starredHubs = $derived(switchHubs.filter((each) => isStarred(stars.all, 'HUB', each.id)));
  const visitedSwitch = $derived(visitedHubs.map((id) => switchHubs.find((each) => each.id === id)).filter((each): each is Container => !!each && !isStarred(stars.all, 'HUB', each.id)).slice(0, 5));
  /**
   * The rest under *Hubs*: what the search finds, hidden hubs included - and without a search the
   * sidebar's own hubs in its order, so the panel never offers *All hubs* alone to someone who has
   * starred and visited nothing yet.
   */
  const matchedHubs = $derived(
    (switchSearch.trim() ? switchHubs : orderedHubs.filter((each) => switchHubs.includes(each)))
      .filter((each) => !starredHubs.includes(each) && !visitedSwitch.includes(each)),
  );
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

  // ---- The phone's Browse steps (§9) --------------------------------------------------------
  /** A content node and the nodes above it, found in the tree. */
  function findContent(nodes: readonly ContentNode[], id: string, above: readonly ContentNode[] = []): { node: ContentNode; above: readonly ContentNode[] } | undefined {
    for (const node of nodes) {
      if (node.id === id) return { node, above };
      const found = findContent(node.children, id, [...above, node]);
      if (found) return found;
    }
    return undefined;
  }
  const under = $derived(step?.kind === 'contents' && step.under ? findContent(contentNodes, step.under) : undefined);
  const flatContents = (nodes: readonly ContentNode[]): ContentNode[] => nodes.flatMap((node) => [node, ...flatContents(node.children)]);
  let stepFilter = $state('');
  let isStepFiltering = $state(false);
  let stepKind = $state<'all' | 'page' | 'whiteboard'>('all');
  const stepRows = $derived.by(() => {
    const words = stepFilter.trim().toLocaleLowerCase(messages.locale);
    const base = words ? flatContents(contentNodes).filter((node) => node.title.toLocaleLowerCase(messages.locale).includes(words)) : under ? under.node.children : contentNodes;
    return stepKind === 'all' ? base : base.filter((node) => node.kind === stepKind);
  });
  const contentPath = (node: ContentNode) => (node.kind === 'whiteboard' ? `/whiteboards/${node.id}` : `/pages/${node.id}`);
  const stepTitle = $derived(
    !step ? undefined
      : step.kind === 'workspace' ? t('app.nav.browse')
        : step.kind === 'queues' ? t('app.nav.queues_step')
          : step.kind === 'contents' ? (under?.node.title ?? t('app.nav.contents'))
            : hub?.name,
  );
  $effect(() => {
    if (!step) return;
    return page.entitle(stepTitle);
  });
  // Under a page the way up is the page above, which only the tree knows.
  $effect(() => {
    if (step?.kind !== 'contents' || !step.under || !under) return;
    const parent = under.above[under.above.length - 1];
    return page.ascend(parent ? { label: parent.title, path: contentsStep(step.hubId, parent.id) } : { label: t('app.nav.contents'), path: contentsStep(step.hubId) });
  });
  // *Queues*' figure on the phone (§9): what asks for action - the urgent queues' count on the
  // danger surface - else the total.
  const queuesUrgent = $derived(queueList.some((row) => row.isUrgent));
  const queuesTotal = $derived(queueList.filter((row) => !queuesUrgent || row.isUrgent).reduce((sum, row) => sum + (row.count ?? 0), 0));
  // A hub's step carries the hub's ⋯ in the bar, as its head does in the sidebar.
  $effect(() => {
    if (step?.kind !== 'hub' || !hub) return;
    const menu = hubMenuOf(hub.id);
    if (!menu?.items) return;
    page.offer({ label: menu.label, items: menu.items, onselect: (id) => menu.onselect(id) });
    return () => page.offer(undefined);
  });
  // A long press on a row (ADR-0088's hold) opens its context sheet - the items ⋯ has (§9, phone
  // screen 7); *Change order* opens reorder mode, the rows dragged by the grip at their end and
  // `Alt+↑/↓`, *Done* ends it (screen 8); *Move to…* the place picker as a sheet (screen 9).
  let phoneElement = $state<HTMLElement | null>(null);
  let contextRow = $state<{ readonly id: string; readonly kind: 'product' | 'hub' | 'collection' | 'page' | 'whiteboard'; readonly name: string } | undefined>(undefined);
  let reordering = $state<'collections' | 'contents' | undefined>(undefined);
  $effect(() => {
    const at = phoneElement;
    if (!at) return;
    return attachEntryGestures(at, {
      idAt: (target) => (reordering ? undefined : ((target as Element | null)?.closest?.<HTMLElement>('[data-press-row] li[data-id]')?.dataset['id'] ?? undefined)),
      open: (id) => {
        const product = step?.kind === 'workspace' ? productSets.all.find((row) => row.module === id) : undefined;
        if (product) return void (contextRow = { id, kind: 'product', name: t(product.code) });
        const collection = containers.find(id);
        if (collection?.type === 'HUB' && step?.kind === 'workspace') return void (contextRow = { id, kind: 'hub', name: collection.name });
        if (collection?.type === 'COLLECTION') return void (contextRow = { id, kind: 'collection', name: collection.name });
        const board = boardRows.find((each) => each.id === id);
        const pageRow = pageRows.find((each) => each.id === id);
        if (board) contextRow = { id, kind: 'whiteboard', name: board.title };
        else if (pageRow) contextRow = { id, kind: 'page', name: pageRow.title };
      },
    });
  });
  const STAR_KIND = { hub: 'HUB', collection: 'COLLECTION', page: 'PAGE', whiteboard: 'WHITEBOARD' } as const;
  const contextItems = $derived.by((): readonly MenuItem[] => {
    const row = contextRow;
    if (!row) return [];
    if (row.kind === 'product') return [hideItem(productSets.more.some((each) => each.module === row.id))];
    if (row.kind === 'hub') {
      const starred = isStarred(stars.all, 'HUB', row.id);
      return [
        { id: 'star', label: t(starred ? 'app.nav.sheet_unstar' : 'app.nav.sheet_star'), icon: 'star' },
        hideItem(hubSets.more.some((each) => each.id === row.id)),
      ];
    }
    const starred = isStarred(stars.all, STAR_KIND[row.kind], row.id);
    const may = row.kind === 'collection' ? mayStructure : mayWrite;
    const kept = row.kind === 'collection' ? (placeMenuOf(row.id)?.items ?? []).filter((item) => item.id === 'archive' || item.id === 'trash') : mayWrite ? contentLifecycle(row.id) : [];
    return [
      { id: 'star', label: t(starred ? 'app.nav.sheet_unstar' : 'app.nav.sheet_star'), icon: 'star' as const },
      ...(row.kind === 'page' && mayWrite ? [{ id: 'beneath', label: t('app.nav.create_beneath'), icon: 'plus' as const }] : []),
      ...(may ? [{ id: 'move', label: t('app.nav.move_to'), icon: 'arrow-right-left' as const }, { id: 'order', label: t('app.nav.change_order'), icon: 'grip-vertical' as const }] : []),
      ...kept.map((item) => ({ id: item.id, label: item.label, icon: item.icon })),
    ];
  });
  function chooseContext(act: string) {
    const row = contextRow;
    contextRow = undefined;
    if (!row) return;
    if (act === 'hide' || act === 'show') return void setHidden(row.kind === 'product' ? 'nav_products_hidden' : 'nav_hubs_hidden', row.id, row.name, act === 'hide');
    if (row.kind === 'product') return;
    if (act === 'star') return void stars.toggle(STAR_KIND[row.kind], row.id);
    if (row.kind === 'hub' || !hub) return;
    if (act === 'beneath') return onnavigate(createPath(hub.id, 'page', row.id));
    if (act === 'move') return row.kind === 'collection' ? moveCollection(row.id) : void (movingContent = row.id);
    if (act === 'order') return void (reordering = row.kind === 'collection' ? 'collections' : 'contents');
    if (row.kind === 'collection') placeMenuOf(row.id)?.onselect(act);
    else void contentAct(row.id, act);
  }
  $effect(() => {
    if (!reordering) return;
    return page.entitle(t('app.nav.order_title'));
  });
  let reorderDrag = $state<{ readonly id: string; y: number } | undefined>(undefined);
  /** A grip pressed in reorder mode carries its row; the list's midpoints decide where it lands. */
  function beginReorder(event: PointerEvent, id: string) {
    if (!reordering || !(event.target as Element | null)?.closest?.('.reorder')) return;
    event.preventDefault();
    reorderDrag = { id, y: event.clientY };
    const follow = (move: PointerEvent) => {
      if (reorderDrag) reorderDrag.y = move.clientY;
    };
    const end = (up: PointerEvent) => {
      window.removeEventListener('pointermove', follow);
      window.removeEventListener('pointerup', end);
      const done = reorderDrag;
      reorderDrag = undefined;
      if (done) finishReorder(done.id, up.clientY);
    };
    window.addEventListener('pointermove', follow);
    window.addEventListener('pointerup', end);
  }
  function finishReorder(id: string, y: number) {
    const rows = [...(phoneElement?.querySelectorAll<HTMLElement>('[data-reordering] li[data-id]') ?? [])];
    const ids = rows.map((row) => row.dataset['id'] ?? '');
    const others = rows.filter((row) => row.dataset['id'] !== id);
    let at = others.length;
    for (let index = 0; index < others.length; index++) {
      const box = others[index]!.getBoundingClientRect();
      if (y < box.top + box.height / 2) {
        at = index;
        break;
      }
    }
    if (ids.indexOf(id) === at) return;
    if (reordering === 'collections') return void rank(id, collections, at);
    const afterId = at > 0 ? (others[at - 1]!.dataset['id'] ?? null) : null;
    void placeContent(id, { parentId: under?.node.id ?? null, afterId });
  }
  const reorderKey = (id: string, key: MoveKey) => {
    if (reordering === 'collections') return moveKey(collections, collectionNodes, () => mayStructure)(id, key);
    if (key.key === 'ArrowUp' || key.key === 'ArrowDown') contentKey(id, key);
  };
  const phoneDrop = (id: string) => (reorderDrag?.id === id ? 'lifted' : landed === id ? 'landed' : undefined);

  let isAllHubsShown = $state(false);
  let isNewOpen = $state(false);
  const RECENT_KINDS = { item: 'app.nav.recent_kind_item', collection: 'app.nav.recent_kind_collection', hub: 'app.nav.recent_kind_hub' } as const;
  const recentColour = (row: { kind: 'item' | 'collection' | 'hub'; id: string; placeId?: string }) => {
    const container = containers.find(row.kind === 'item' ? (row.placeId ?? '') : row.id);
    return container ? (container.color_token ?? DEFAULT_TILE) : DEFAULT_TILE;
  };

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

{#if step}
<div class="phone" data-browse-step={step.kind} bind:this={phoneElement}>
  <!-- The bar shows the step's title; the screen's one heading is read here (as `PageHeader`'s). -->
  <VisuallyHidden as="h1">{stepTitle ?? ''}</VisuallyHidden>
  {#if !isTreeReady}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else if failure}
    <FailureState problem={failure} onRetry={() => containers.refresh()} />
  {:else if step.kind === 'workspace'}
    {#if topRows.length > 0}
      <NavGroup label={t('app.nav.for_you')} size="touch">
        {#each topRows as row (row.id)}
          <NavItem label={t(row.code)} icon={row.icon} href={row.target.kind === 'route' ? row.target.path : undefined} figure={topFigure(row.id)} isFigureUrgent={row.id === 'approvals'} size="touch" isDrillDown
            onnavigate={() => row.target.kind === 'route' && onnavigate(row.target.path)} />
        {/each}
      </NavGroup>
    {/if}
    {#if recents.rows.length > 0}
      <section class="recent" aria-labelledby="browse-recent">
        <h3 class="gl" id="browse-recent">{t('app.nav.recent')}</h3>
        <ul class="cards">
          {#each recents.rows.slice(0, 3) as row (row.kind + row.id)}
            <li>
              <a class="card" href={RECENT_PATHS[row.kind](row.id)} onclick={(event) => { event.preventDefault(); onnavigate(RECENT_PATHS[row.kind](row.id)); }}>
                <span aria-hidden="true"><ProductTile icon={RECENT_ICONS[row.kind]} color={recentColour(row)} /></span>
                <span class="card-name">{row.title}</span>
                <small class="card-kind">{t(RECENT_KINDS[row.kind])}</small>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
    {#if starRows.length > 0}
      <NavGroup label={t('app.nav.starred')} size="touch">
        {#each starRows as row (row.key)}
          <NavItem label={row.label} icon={row.icon} color={row.color} href={row.href} figure={row.figure} isFigureUrgent={row.isUrgent} size="touch" onnavigate={() => onnavigate(row.href)} />
        {/each}
      </NavGroup>
    {/if}
    <!-- A long press on a product or a hub opens its sheet: *Hide*, or *Show* behind *More* (R-3, R-4). -->
    {#if productSets.all.length > 0}
      <div data-press-row>
      <NavGroup label={t('app.nav.products')} size="touch" hiddenRows={hiddenProductNodes} hiddenLabel={t('app.nav.more_hidden')} bind:isHiddenOpen={sections.productsMore.isOpen}
        onnavigate={(module) => onnavigate(productPaths[module] ?? '/')}>
        {#each productSets.shown as row (row.id)}
          <NavItem id={row.module} label={t(row.code)} icon={row.icon} color={row.color} href={row.path} size="touch" isDrillDown onnavigate={() => onnavigate(row.path)} />
        {/each}
      </NavGroup>
      </div>
    {/if}
    <div data-press-row>
    <NavGroup label={t('app.nav.hubs')} size="touch" hiddenRows={hiddenHubNodes} hiddenLabel={t('app.nav.more_hidden')} bind:isHiddenOpen={sections.hubsMore.isOpen}
      onnavigate={(id) => onnavigate(hubStep(id))}>
      {#each isAllHubsShown ? orderedHubs : orderedHubs.slice(0, HUBS_SHOWN) as each (each.id)}
        <NavItem id={each.id} label={each.name} icon={each.private && !chosenSymbol(each) ? 'lock' : markOf(each)} color={each.color_token ?? DEFAULT_TILE} href={hubStep(each.id)}
          badge={each.private ? { icon: 'lock', label: t('app.nav.private') } : undefined} size="touch" isDrillDown onnavigate={() => onnavigate(hubStep(each.id))} />
      {/each}
      {#if !isAllHubsShown && orderedHubs.length > HUBS_SHOWN}
        <NavItem label={t('app.nav.more_count', { count: orderedHubs.length - HUBS_SHOWN })} icon="ellipsis" size="touch" isDrillDown onnavigate={() => (isAllHubsShown = true)} />
      {/if}
      {#if orderedHubs.length === 0 && createHubLabel}
        <NavItem label={createHubLabel} icon="plus" tone="link" size="touch" onnavigate={() => (isCreatingHub = true)} />
      {/if}
    </NavGroup>
    </div>
    <ul class="card-list" aria-label={t('app.nav.keeping')}>
      {#each footRows as row (row.id)}
        <NavItem label={t(row.code)} icon={row.icon} href={row.path} figure={footFigure(row.id)} size="touch" isDrillDown onnavigate={() => goFoot(row.id, row.path)} />
      {/each}
    </ul>
  {:else if hub && step.kind === 'hub'}
    <!-- Reorder mode shows the one part being ordered, as its own screen would (§9). -->
    {#if !reordering}
    <ul class="card-list" aria-label={t('app.nav.tools_of', { hub: hub.name })}>
      <NavItem label={t('app.nav.overview')} icon="hub" href={`/hubs/${hub.id}`} size="touch" isDrillDown onnavigate={() => onnavigate(`/hubs/${hub.id}`)} />
      {#each hubToolRows as each (each.id)}
        {#if each.id === 'queues'}
          <NavItem label={t(each.code)} icon={each.icon} href={queuesStep(hub.id)} figure={queueList.length > 0 ? String(queuesTotal) : undefined} isFigureUrgent={queuesUrgent}
            size="touch" isDrillDown onnavigate={() => onnavigate(queuesStep(hub.id))} />
        {:else}
          <NavItem label={t(each.code)} icon={each.icon} href={toolPath(hub.id, each.id)} figure={toolFigure(each.id)} isFigureUrgent={each.id === 'inventory'} size="touch" isDrillDown onnavigate={() => onnavigate(toolPath(hub.id, each.id))} />
        {/if}
      {/each}
    </ul>
    {/if}
    {#if reordering}
      <div class="order-head">
        <p class="order-hint">{t('app.nav.order_hint', { part: t(reordering === 'collections' ? 'app.nav.collections' : 'app.nav.contents'), place: hub.name })}</p>
        <Button tone="primary" size="sm" onclick={() => (reordering = undefined)}>{t('app.nav.order_done')}</Button>
      </div>
    {/if}
    {#if collectionNodes.length > 0 && reordering !== 'contents'}
      <div class="drop-part" data-press-row data-reordering={reordering === 'collections' ? '' : undefined}>
      <NavGroup label={t('app.nav.collections')} size="touch">
        {#each collectionNodes as node (node.id)}
          <NavItem id={node.id} label={node.label} icon={node.icon} color={node.color} href={`/collections/${node.id}`} figure={node.figure} isFigureUrgent={node.isFigureUrgent} badge={node.badge} size="touch"
            isReordering={reordering === 'collections'} dropState={phoneDrop(node.id)} onpointerdown={(event) => beginReorder(event, node.id)} onmovekey={reordering ? (key) => reorderKey(node.id, key) : undefined}
            onnavigate={() => !reordering && onnavigate(`/collections/${node.id}`)} />
        {/each}
      </NavGroup>
      </div>
    {:else if mayStructure && !reordering}
      <NavGroup label={t('app.nav.collections')} size="touch">
        <NavItem label={t('app.nav.create_first_collection')} icon="plus" tone="link" size="touch" onnavigate={() => { collectionHubId = hub.id; isCreatingCollection = true; }} />
      </NavGroup>
    {/if}
    {#if hasContents && contentNodes.length > 0 && reordering !== 'collections'}
      <div class="drop-part" data-press-row data-reordering={reordering === 'contents' ? '' : undefined}>
      <NavGroup label={t('app.nav.contents')} size="touch" headLink={reordering ? undefined : { label: t('app.nav.kind_all'), onselect: () => onnavigate(contentsStep(hub.id)) }}>
        {#each contentTree.slice(0, CONTENTS_SHOWN) as node (node.id)}
          {@const path = node.kind === 'whiteboard' ? `/whiteboards/${node.id}` : `/pages/${node.id}`}
          {@const below = node.children?.length ?? 0}
          <NavItem id={node.id} label={node.label} icon={node.kind === 'page' ? 'file-text' : 'layout-template'} href={path} size="touch" waitingLabel={node.waitingLabel}
            isReordering={reordering === 'contents'} dropState={phoneDrop(node.id)} onpointerdown={(event) => beginReorder(event, node.id)} onmovekey={reordering ? (key) => reorderKey(node.id, key) : undefined}
            onnavigate={() => !reordering && onnavigate(path)}
            descendLabel={below > 0 && !reordering ? t('app.nav.descend', { name: node.label }) : undefined}
            ondescend={below > 0 && !reordering ? () => onnavigate(contentsStep(hub.id, node.id)) : undefined} />
        {/each}
        {#if contentNodes.length > CONTENTS_SHOWN}
          <NavItem label={t('app.nav.more_count', { count: contentNodes.length - CONTENTS_SHOWN })} icon="ellipsis" size="touch" isDrillDown onnavigate={() => onnavigate(contentsStep(hub.id))} />
        {/if}
      </NavGroup>
      </div>
    {:else if hasContents && contentsAdd && !reordering}
      <NavGroup label={t('app.nav.contents')} size="touch">
        <NavItem label={t(isOn('KNOWLEDGE') ? 'app.nav.create_first_page' : 'app.nav.create_first_whiteboard')} icon="plus" tone="link" size="touch" onnavigate={() => hubId && onnavigate(createPath(hubId, isOn('KNOWLEDGE') ? 'page' : 'whiteboard'))} />
      </NavGroup>
    {/if}
  {:else if hub && step.kind === 'queues'}
    <ul class="card-list" aria-label={t('app.nav.queues_of', { hub: hub.name })}>
      {#each queueList as row (row.id)}
        <NavItem label={queueName(row)} icon={row.isStarred ? 'star' : 'columns-3'} href={queuePath(hub.id, row)} figure={row.count === null ? undefined : String(row.count)} isFigureUrgent={row.isUrgent}
          figureLabel={row.count === null ? undefined : t(row.isUrgent ? 'app.nav.queue_count_urgent' : 'app.nav.queue_count', { count: row.count })}
          size="touch" isDrillDown onnavigate={() => onnavigate(queuePath(hub.id, row))} />
      {/each}
    </ul>
  {:else if hub && step.kind === 'contents'}
    <div class="chips">
      <button type="button" class="chip" aria-pressed={isStepFiltering} onclick={() => { isStepFiltering = !isStepFiltering; if (!isStepFiltering) stepFilter = ''; }}>
        <span class="pill"><Icon name="search" size="sm" />{t('app.nav.filter')}</span>
      </button>
      {#if isOn('KNOWLEDGE') && isOn('WHITEBOARD')}
        <Menu label={t('app.nav.kind_label')} items={(['all', 'page', 'whiteboard'] as const).map((id) => ({ id, label: t(id === 'all' ? 'app.nav.kind_all' : id === 'page' ? 'app.nav.kind_pages' : 'app.nav.kind_whiteboards'), ...(id === stepKind ? { icon: 'check' as const } : {}) }))}
          onselect={(id) => (stepKind = id as typeof stepKind)}>
          {#snippet trigger(props)}
            <button type="button" class="chip" {...props}><span class="pill">{t(stepKind === 'all' ? 'app.nav.kind_all' : stepKind === 'page' ? 'app.nav.kind_pages' : 'app.nav.kind_whiteboards')}<Icon name="chevron-down" size="sm" /></span></button>
          {/snippet}
        </Menu>
      {/if}
      <Menu label={contentsMenu.label} items={contentsMenu.items ?? []} onselect={(id) => contentsMenu.onselect(id)}>
        {#snippet trigger(props)}
          <button type="button" class="chip" {...props}><span class="pill">{t(`app.nav.mode_${contentsMode.toLowerCase()}`)}<Icon name="chevron-down" size="sm" /></span></button>
        {/snippet}
      </Menu>
    </div>
    {#if isStepFiltering}
      <label class="step-filter">
        <Icon name="search" size="sm" />
        <input type="search" bind:value={stepFilter} aria-label={t('app.nav.filter_contents')} placeholder={t('app.nav.filter_contents')} />
      </label>
    {/if}
    {#if reordering === 'contents'}
      <div class="order-head">
        <p class="order-hint">{t('app.nav.order_hint', { part: t('app.nav.contents'), place: under?.node.title ?? hub.name })}</p>
        <Button tone="primary" size="sm" onclick={() => (reordering = undefined)}>{t('app.nav.order_done')}</Button>
      </div>
    {/if}
    <ul class="card-list" aria-label={t('app.nav.contents_of', { hub: under?.node.title ?? hub.name })} data-press-row data-reordering={reordering === 'contents' ? '' : undefined}>
      {#each stepRows as node (node.id)}
        <NavItem id={node.id} label={node.title} icon={node.kind === 'page' ? 'file-text' : 'layout-template'} href={contentPath(node)} size="touch" onnavigate={() => !reordering && onnavigate(contentPath(node))}
          isReordering={reordering === 'contents'} dropState={phoneDrop(node.id)} onpointerdown={(event) => beginReorder(event, node.id)} onmovekey={reordering ? (key) => reorderKey(node.id, key) : undefined}
          waitingLabel={pendingContent?.isWaiting && pendingContent.id === node.id ? t('app.nav.waiting') : undefined}
          descendLabel={node.children.length > 0 && !reordering ? t('app.nav.descend', { name: node.title }) : undefined}
          ondescend={node.children.length > 0 && !reordering ? () => onnavigate(contentsStep(hub.id, node.id)) : undefined} />
      {/each}
    </ul>
    {#if mayWrite && createItems.length > 0}
      <span class="fab"><Button tone="primary" icon="plus" onclick={() => (isNewOpen = true)}>{t('app.nav.new')}</Button></span>
      <Sheet title={under ? t('app.nav.create_under', { name: under.node.title }) : t('app.nav.new_in_place', { name: hub.name })} closeLabel={t('app.sheet_close')} bind:isOpen={isNewOpen}>
        <ul>
          {#each createItems as item (item.id)}
            <NavItem label={item.id === 'template' ? t('app.nav.create_page_from_template') : item.label} icon={item.icon} size="touch"
              onnavigate={() => { isNewOpen = false; onnavigate(createPath(hub.id, item.id, under?.node.id)); }} />
          {/each}
        </ul>
        {#if mayStructure && !under}
          <ul>
            <NavItem label={t('app.nav.create_collection_kind')} icon="collection" size="touch" onnavigate={() => { isNewOpen = false; collectionHubId = hub.id; isCreatingCollection = true; }} />
          </ul>
        {/if}
      </Sheet>
    {/if}
  {/if}
  {#if contextRow}
    {@const row = contextRow}
    <Sheet title={row.name} icon={row.kind === 'product' ? (productSets.all.find((each) => each.module === row.id)?.icon ?? 'ellipsis') : row.kind === 'hub' ? 'hub' : row.kind === 'collection' ? 'collection' : row.kind === 'whiteboard' ? 'layout-template' : 'file-text'} closeLabel={t('app.sheet_close')} bind:isOpen={() => contextRow !== undefined, (open) => { if (!open) contextRow = undefined; }}>
      <ul>
        {#each contextItems as item (item.id)}
          <NavItem label={item.label} icon={item.icon} size="touch" onnavigate={() => chooseContext(item.id)} />
        {/each}
      </ul>
    </Sheet>
  {/if}
</div>
{:else}
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
      {#snippet switcher(head)}
        <NavFlyout label={t('app.nav.switch_hub')} trigger="icon" icon="chevrons-up-down" triggerLabel={t('app.nav.switch_hub')} searchLabel={t('app.nav.find_hub')} anchor={head} placement={SWITCH_PLACEMENT} bind:search={switchSearch} bind:isOpen={isSwitchOpen}>
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
        <div class="drop-part" data-entry-target>
        <NavGroup
          label={t('app.nav.collections')}
          collapseLabel={t('app.nav.fold', { name: t('app.nav.collections') })}
          bind:isOpen={sections.collections.isOpen}
          add={mayStructure ? { label: t('app.nav.new_collection_short'), onselect: () => { collectionHubId = hub.id; isCreatingCollection = true; } } : undefined}
          tree={collectionNodes}
          treeLabel={t('app.nav.collections_of', { hub: hub.name })}
          current={currentCollection}
          rowMenu={placeMenuOf}
          canOrder={() => mayStructure}
          onnavigate={(id) => onnavigate(`/collections/${id}`)}
          onplace={place(collections)}
          onmovekey={moveKey(collections, collectionNodes, () => mayStructure)}
          onmoveto={moveCollection}
          {refusalLabel}
          {landed}
          dropInto={entryOver}
          oncancel={() => announcer.say(t('app.nav.cancelled'))}
          limit={COLLECTIONS_SHOWN}
          moreLabel={(hidden) => t('app.nav.more_count', { count: hidden })}
          filterFrom={COLLECTIONS_FILTER_FROM}
          filterLabel={t('app.nav.filter_collections')}
          bind:filter={collectionFilter}
        />
        </div>
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
          bind:isOpen={sections.contents.isOpen}
          add={contentsAdd}
          rowAdd={contentRowAdd}
          menu={contentsMenu}
          tree={contentTree}
          treeLabel={t('app.nav.contents_of', { hub: hub.name })}
          bind:expanded
          current={currentContent}
          {twistLabel}
          onnavigate={(id) => onnavigate(boardRows.some((each) => each.id === id) ? `/whiteboards/${id}` : `/pages/${id}`)}
          rowMenu={contentRowMenu}
          canOrder={() => mayWrite && contentsMode === 'TREE'}
          onplace={(id, placement) => void placeContent(id, placement)}
          onmovekey={contentKey}
          onmoveto={(id) => (mayWrite ? (movingContent = id) : announcer.say(t('app.nav.may_not_order')))}
          {refusalLabel}
          {landed}
          oncancel={() => announcer.say(t('app.nav.cancelled'))}
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
          <NavItem label={t(isOn('KNOWLEDGE') ? 'app.nav.create_first_page' : 'app.nav.create_first_whiteboard')} icon="plus" tone="link" onnavigate={() => hubId && onnavigate(createPath(hubId, isOn('KNOWLEDGE') ? 'page' : 'whiteboard'))} />
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
        <div class="drop-part" data-entry-target>
          <NavGroup
            label={t('app.nav.starred')}
            collapseLabel={t('app.nav.fold', { name: t('app.nav.starred') })}
            bind:isOpen={sections.starred.isOpen}
            tree={starNodes}
            current={starRows.find((row) => route.path === row.href)?.key}
            canOrder={() => true}
            onnavigate={(key) => { const row = starRows.find((each) => each.key === key); if (row) onnavigate(row.href); }}
            onplace={(key, placement) => void placeStar(key, placement.afterId)}
            onmovekey={starKey}
            {refusalLabel}
            {landed}
            dropInto={entryOver}
            oncancel={() => announcer.say(t('app.nav.cancelled'))}
          />
        </div>
      {/if}
      {#if productSets.all.length > 0}
        <NavGroup
          label={t('app.nav.products')}
          collapseLabel={t('app.nav.fold', { name: t('app.nav.products') })}
          bind:isOpen={sections.products.isOpen}
          tree={productNodes}
          current={currentProduct}
          rowMenu={productMenuOf}
          canOrder={() => !!actor.account}
          onnavigate={(module) => onnavigate(productPaths[module] ?? '/')}
          onplace={(module, placement) => void placeProduct(module, placement.afterId)}
          onmovekey={productKey}
          {refusalLabel}
          {landed}
          oncancel={() => announcer.say(t('app.nav.cancelled'))}
          hiddenRows={hiddenProductNodes}
          hiddenLabel={t('app.nav.more_hidden')}
          bind:isHiddenOpen={sections.productsMore.isOpen}
        />
      {/if}
      <NavGroup
        label={t('app.nav.hubs')}
        collapseLabel={t('app.nav.fold', { name: t('app.nav.hubs') })}
        bind:isOpen={sections.hubs.isOpen}
        add={createHubLabel ? { label: createHubLabel, onselect: () => (isCreatingHub = true) } : undefined}
        menu={hubsMenu}
        tree={hubNodes}
        treeLabel={t('app.nav.hubs')}
        rowMenu={hubRowMenuOf}
        canOrder={mayMoveHub}
        onnavigate={(id) => onnavigate(`/hubs/${id}`)}
        onplace={(id, placement) => void placeHub(id, placement)}
        onmovekey={hubKey}
        hiddenRows={hiddenHubNodes}
        hiddenLabel={t('app.nav.more_hidden')}
        bind:isHiddenOpen={sections.hubsMore.isOpen}
        {refusalLabel}
        {landed}
        oncancel={() => announcer.say(t('app.nav.cancelled'))}
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
{/if}

{#if positioningHub}
  <PositionDialog
    bind:isOpen={() => positioningHub !== undefined, (open) => { if (!open) positioningHub = undefined; }}
    id={positioningHub.id}
    name={positioningHub.name}
    siblings={shownHubs.map((each) => ({ id: each.id, name: each.name }))}
    onmove={moveHubBy}
  />
{/if}

<CreateContainerDialog
  bind:isOpen={isCreatingHub}
  type="HUB"
  isPrivateOnly={creation === 'private'}
  oncreated={(id, landing) => onnavigate(landing ?? `/hubs/${id}`)}
/>
{#if movingContent}
  <PlacePicker title={t('app.nav.move_row_to', { name: movingContentName })} places={pickerRows} bind:isOpen={() => movingContent !== undefined, (open) => { if (!open) movingContent = undefined; }} onpick={pickContent} />
{/if}
{#if movingCollection}
  <CollectionMoveDialog container={movingCollection} bind:isOpen={() => movingCollection !== undefined, (open) => { if (!open) movingCollection = undefined; }} {onnavigate} />
{/if}
{#if entryMove}
  {@const move = entryMove}
  <Dialog title={t('app.nav.entry_move_title', { title: move.title, collection: move.collection.name })} isOpen dismissLabel={t('app.workspace.cancel')} onClose={() => (entryMove = undefined)}>
    <p>{t('app.nav.entry_move_hub', { collection: move.collection.name, hub: containers.find(move.collection.parent_id ?? '')?.name ?? '' })}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (entryMove = undefined)}>{t('app.workspace.cancel')}</Button>
      <!-- `move` is derived from `entryMove`: read before it is cleared. -->
      <Button tone="primary" onclick={() => { const { entryId, collection } = move; entryMove = undefined; void moveEntry(entryId, collection); }}>{t('app.move.confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}
{#if collectionHubId}
  <CreateContainerDialog bind:isOpen={isCreatingCollection} type="COLLECTION" parentId={collectionHubId} oncreated={(id, landing) => onnavigate(landing ?? `/collections/${id}`)} />
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
  .drop-part { display: contents; }
  .order-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); margin: 0 var(--sp-050) var(--sp-100); }
  .order-hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  [data-reordering] :global(.reorder) { touch-action: none; cursor: grab; }
  .foryou { border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .foot { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .grow { flex: 1; min-block-size: 0; overflow-y: auto; }
  .pad { padding: var(--sp-150); }
  .flyout { list-style: none; }
  .quiet { list-style: none; padding: var(--sp-050) var(--sp-100); color: var(--text-subtle); font-size: var(--fs-075); }
  .failure { margin: 0; padding-inline: var(--sp-100); color: var(--text-danger); font-size: var(--fs-075); }

  /* ---- The phone's Browse steps (§9): cards of 44 px rows, as the mockup's phone screens. ---- */
  .phone { display: flex; flex-direction: column; padding-block-end: calc(var(--density-control-md-min) + var(--sp-200)); }
  .card-list {
    margin: var(--sp-150) 0;
    padding: 0;
    overflow: hidden;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .card-list:first-child { margin-block-start: 0; }
  .gl {
    margin: var(--sp-150) var(--sp-050) calc(var(--sp-050) + var(--sp-025));
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-065);
    font-weight: var(--fw-medium);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .recent .gl { margin-block-start: 0; }
  .cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .card {
    display: grid;
    gap: var(--sp-025);
    min-block-size: var(--layout-nav-touchrow);
    padding: calc(var(--sp-100) + var(--sp-025));
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    text-decoration: none;
  }
  .card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .card-name { overflow: hidden; margin-block-start: var(--sp-050); font-weight: var(--fw-semibold); text-overflow: ellipsis; white-space: nowrap; }
  .card-kind { color: var(--text-subtle); font-size: var(--fs-065); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  /* A chip's pill is small, its target a thumb's height (§9: 44 px targets). */
  .chip {
    display: inline-flex;
    align-items: center;
    min-block-size: var(--layout-nav-touchrow);
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-050) calc(var(--sp-100) + var(--sp-025));
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }
  .chip[aria-pressed='true'] .pill { border-color: var(--accent-primary); }
  .chip:focus-visible .pill { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .step-filter {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--layout-nav-touchrow);
    margin-block-start: var(--sp-100);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-strong);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .step-filter input { flex: 1; min-inline-size: 0; border: 0; background: none; color: var(--text-primary); font: inherit; outline: none; }
  .step-filter:focus-within { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  /* The one main action, floating above the bottom bar (§9). */
  .fab {
    position: fixed;
    inset-inline-end: var(--sp-200);
    inset-block-end: calc(var(--layout-bottombar-height) + var(--sp-200) + env(safe-area-inset-bottom, 0));
    z-index: var(--z-raised);
    border-radius: var(--r-xl);
    box-shadow: var(--shadow-overlay);
  }
</style>
