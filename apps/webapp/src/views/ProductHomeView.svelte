<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's workspace-wide home (user-experience.md §2.2, §5.1): the one door the products band
  // opens. One anatomy for every product - the trail *Products › <product>*, the title with what
  // the product does, the views as one `ViewBar` where it has several, then the hubs the reader may
  // read where the product is on, each opening the hub's own screen of it. A hub narrows the
  // workspace's modules (UC-INS-21 check 3), so each hub is asked which are on there.
  import { untrack } from 'svelte';
  import { Badge, Button, Dialog, EmptyState, Icon, ListRow, PageHeader, ProductBand, Select, Skeleton, Stack, StateLozenge, ViewBar, Widget, WorkPage, type ViewLink } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import PlanImport from '../lib/planning/PlanImport.svelte';
  import { markOf } from '../lib/workspace/look.ts';
  import { holds, holdsHere, hubCreationHere, reachesAdministrationArea, roleOrder } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../lib/data/modules.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { productOf } from '../lib/navigation.ts';
  import { attentionAcross, continueReading, daysOverdue, isVerified, pagesFigures, verifiedBySpace, phasesOf, planCounts, createAt, createOf, homeTabs, placesByUse, writableHubs, isUnused, hubScreenOf, hubsWithAssetsFirst, inventoryByClass, inventorySummary, nextReleaseOf, pagesToReview, recentBoards, recentPages, releasesDue, sprintsRunning } from '../lib/product/home.ts';
  import CollectionFigure from '../lib/entries/CollectionFigure.svelte';
  import { releasePath, releasesPath } from '../lib/data/releases.svelte.ts';
  import { sprintPath, sprintsPath } from '../lib/data/sprints.svelte.ts';
  import SprintBurndown from '../lib/plans/SprintBurndown.svelte';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { sprintDaysLeft } from '../lib/today.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { boardsPath } from '../lib/data/whiteboards.svelte.ts';
  import BoardCard from '../lib/whiteboards/BoardCard.svelte';
  import { engine } from '../lib/data/engine.ts';
  import { formatPlainDay, formatRelative } from '../lib/i18n/datetime.ts';
  import type { CiClassList, CmdbHealth, ConfigTemplate, ConfigTemplateList, ItemAggregateResult, MySprintList, PageList, PlanSchedule, PortfolioList, ReleaseDetail, SprintDetail, ReleaseList, SprintList, WhiteboardList } from '@hubtask/sync-engine';
  import { PORTFOLIOS } from '../lib/data/goals.svelte.ts';
  import BoardTemplates, { type BoardStart } from '../lib/whiteboards/BoardTemplates.svelte';
  import BoardsForYou from '../lib/whiteboards/BoardsForYou.svelte';
  import { makeBoard } from '../lib/whiteboards/make.ts';
  import { boardsOpened, boardsToContinue } from '../lib/whiteboards/opened.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { config } from '../lib/data/config.svelte.ts';
  import { productTemplates } from '../lib/product/templates.ts';
  import { pages, pagesPath } from '../lib/data/pages.svelte.ts';
  import { pagesRead } from '../lib/pages/reading.ts';
  import { classesPath, classWord, cmdb, healthPath } from '../lib/data/cmdb.svelte.ts';
  import NotesView from './NotesView.svelte';
  import DeskHome from '../lib/servicedesk/DeskHome.svelte';

  interface Props {
    module: string;
    /** The view of the home, where it has several: the pages' *My notes*. */
    tab?: string;
    onnavigate: (path: string) => void;
  }
  const { module, tab, onnavigate }: Props = $props();

  const row = $derived(productOf(module));
  const name = $derived(row ? t(row.code) : '');
  $effect(() => page.entitle(name));

  // The assets' classes, sources and settings are the administration's screen at their home:
  // offered to whoever reaches the administration, as the door before them was (P-05). The plans'
  // roadmaps are the portfolios', which are the goals' product.
  const ADMINISTERED = ['classes', 'sources', 'settings'];
  const homeViews = $derived<ViewLink[]>(
    homeTabs(module)
      .filter((each) => !(module === 'ASSETS' && ADMINISTERED.includes(each.id)) || reachesAdministrationArea() === true)
      .filter((each) => each.id !== 'roadmaps' || (manifest.value?.modules_on ?? []).includes('GOALS'))
      .map((each) => ({ id: each.id, label: t(each.code), href: each.path })),
  );
  const currentTab = $derived(homeViews.some((each) => each.id === tab) ? (tab as string) : homeViews[0]?.id);
  /**
   * Where the home lists its hubs: the pages' *Spaces* (a hub is a space), the boards' *All*, else
   * the first tab; a home without a hub says so on any tab.
   */
  const showsHubs = $derived(module === 'KNOWLEDGE' ? currentTab === 'spaces' : module === 'WHITEBOARD' ? currentTab === 'all' : currentTab === homeViews[0]?.id);
  /** A tab that lists one kind whole, where *For you* shows its first few. */
  const isWhole = $derived(currentTab === 'sprints' || currentTab === 'releases' || currentTab === 'all');

  /** The hubs the tree draws, and for each the modules on there, once it has answered. */
  const hubs = $derived(containers.hubs.filter((hub) => !live.hasLost(hub.id)));
  let onAt = $state<Record<string, readonly string[]>>({});
  const hubKey = $derived(hubs.map((hub) => hub.id).join(','));
  $effect(() => {
    const ids = hubKey === '' ? [] : hubKey.split(',');
    return untrack(() => {
      const stops = ids.map((hubId) =>
        modules.here(hubId, (next) => {
          if (next.status === 'ready') onAt = { ...onAt, [hubId]: next.data.modules_on };
        }),
      );
      return () => {
        for (const stop of stops) stop();
      };
    });
  });
  const isReading = $derived(
    containers.hubsState.status === 'loading' || containers.hubsState.status === 'idle' || hubs.some((hub) => onAt[hub.id] === undefined),
  );
  const hubsWith = $derived(
    hubs.filter((hub) => onAt[hub.id] !== undefined && (modulesHere(manifest.value?.modules_on, onAt[hub.id]) as readonly string[]).includes(module)),
  );

  /**
   * One read per hub where the product is on, each hub's own as its tab reads it; a hub that
   * leaves the list stops its read. Called once per product at the component's start.
   */
  function eachHub<T>(forModule: string, pathOf: (hubId: string) => string, onready: (hubId: string, data: T) => void): void {
    const key = $derived(module === forModule ? hubsWith.map((hub) => hub.id).join(',') : '');
    $effect(() => {
      const ids = key === '' ? [] : key.split(',');
      return untrack(() => {
        const stops = ids.map((hubId) =>
          engine.subscribe<T>({ path: pathOf(hubId) }, (next) => {
            if (next.status === 'ready') onready(hubId, next.data);
          }),
        );
        return () => stops.forEach((stop) => stop());
      });
    });
  }

  /** The whiteboards' home lists the boards themselves (§2.2), recent first across the hubs. */
  const BOARDS_SHOWN = 12;
  const BOARDS_DRAWN = 3;
  let boardsAt = $state<Record<string, WhiteboardList['data']>>({});
  eachHub<WhiteboardList>('WHITEBOARD', boardsPath, (hubId, data) => (boardsAt = { ...boardsAt, [hubId]: data.data }));

  /** The pages' home: the reader's pages to review and the recently changed ones (§2.2). */
  const PAGES_SHOWN = 10;
  let pagesAt = $state<Record<string, PageList['data']>>({});
  eachHub<PageList>('KNOWLEDGE', pagesPath, (hubId, data) => (pagesAt = { ...pagesAt, [hubId]: data.data }));
  const knowledgeHubs = $derived(module === 'KNOWLEDGE' ? hubsWith.map((hub) => hub.id) : []);
  const toReview = $derived(pagesToReview(pagesAt, knowledgeHubs, actor.account?.id));
  const recent = $derived(recentPages(pagesAt, knowledgeHubs, PAGES_SHOWN));
  /** The pages' home (scene phome): what was read here last, and per space how much is verified. */
  const READING_SHOWN = 3;
  const now = new Date().toISOString();
  const reading = $derived(module === 'KNOWLEDGE' ? continueReading(pagesAt, knowledgeHubs, pagesRead(actor.account?.id), READING_SHOWN) : []);
  const spacesVerified = $derived(module === 'KNOWLEDGE' ? verifiedBySpace(pagesAt, knowledgeHubs, now) : []);
  const isPagesWhole = $derived(knowledgeHubs.every((hubId) => pagesAt[hubId] !== undefined));
  let isOpeningToday = $state(false);
  async function openToday() {
    isOpeningToday = true;
    try {
      const note = await pages.today();
      onnavigate(`/pages/${note.id}`);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isOpeningToday = false;
    }
  }

  /** The plans' home lists the releases still to go out (§2.2), the soonest first. */
  const RELEASES_SHOWN = 8;
  let releasesAt = $state<Record<string, ReleaseList['data']>>({});
  eachHub<ReleaseList>('PLANNING', releasesPath, (hubId, data) => (releasesAt = { ...releasesAt, [hubId]: data.data }));
  const upcoming = $derived(module === 'PLANNING' ? releasesDue(releasesAt, hubsWith.map((hub) => hub.id), isWhole ? Infinity : RELEASES_SHOWN) : []);

  /**
   * The plans late (E-1) and the work still open, each one read across the workspace by
   * collection: a hub's plan is late by the most days an entry's due date lies past its latest
   * baseline's (`plan.slip`), as a query reads it - nothing is written by reading it.
   */
  let slipAt = $state<Record<string, number>>({});
  let openAt = $state<Record<string, number>>({});
  $effect(() => {
    if (module !== 'PLANNING') return;
    return untrack(() => {
      const byCollection = (rows: ItemAggregateResult['data']): Record<string, number> =>
        Object.fromEntries(rows.filter((row) => row.group !== null).map((row) => [String(row.group), Number(row.value ?? row.count)]));
      const stops = [
        engine.subscribe<ItemAggregateResult>(
          { path: '/items:aggregate', body: { scope: { workspace: true }, q: 'plan.slip: >0', aggregate: { function: 'MAX', field: 'slip_days', group_by: 'collection_id' } } },
          (next) => { if (next.status === 'ready') slipAt = byCollection(next.data.data); },
        ),
        engine.subscribe<ItemAggregateResult>(
          { path: '/items:aggregate', body: { scope: { workspace: true }, q: 'is: open', aggregate: { function: 'COUNT', group_by: 'collection_id' } } },
          (next) => { if (next.status === 'ready') openAt = byCollection(next.data.data); },
        ),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const slipOf = (hubId: string) => Math.max(0, ...collectionsOfHub(hubId).map((id) => slipAt[id] ?? 0));
  const openOf = (hubId: string) => collectionsOfHub(hubId).reduce((sum, id) => sum + (openAt[id] ?? 0), 0);
  const plansLate = $derived(module === 'PLANNING' ? hubsWith.filter((hub) => slipOf(hub.id) > 0).length : 0);

  /** The plans' home lists the sprints running in the hubs' collections (§2.2), the soonest to end first. */
  const SPRINTS_SHOWN = 8;
  let sprintsAt = $state<Record<string, SprintList['data']>>({});
  const collectionsOfHub = (hubId: string) => containers.collectionsOf(hubId).map((collection) => collection.id);
  const sprintCollections = $derived(module === 'PLANNING' ? hubsWith.flatMap((hub) => collectionsOfHub(hub.id)) : []);
  const sprintKey = $derived(sprintCollections.join(','));
  $effect(() => {
    const ids = sprintKey === '' ? [] : sprintKey.split(',');
    return untrack(() => {
      const stops = ids.map((id) =>
        engine.subscribe<SprintList>({ path: `${sprintsPath(id)}?state=ACTIVE` }, (next) => {
          if (next.status === 'ready') sprintsAt = { ...sprintsAt, [id]: next.data.data };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });
  const running = $derived(sprintsRunning(sprintsAt, sprintCollections, isWhole ? Infinity : SPRINTS_SHOWN));
  const today = $derived(todayIn(actor.zone));
  /** *For you*'s sprint card: the running sprint that ends first, with its burndown. */
  const firstSprint = $derived(module === 'PLANNING' && currentTab === 'foryou' ? running[0] : undefined);
  let firstDetail = $state<SprintDetail | undefined>(undefined);
  $effect(() => {
    const id = firstSprint?.id;
    const zone = actor.zone;
    if (!id) return;
    return untrack(() => engine.subscribe<SprintDetail>({ path: sprintPath(id, zone) }, (next) => {
      if (next.status === 'ready' && next.data.sprint) firstDetail = next.data;
    }));
  });
  /** The reader's own entries in the sprint card's sprint: its second line says how many (scene homeplans). */
  let mySprints = $state<MySprintList['data']>([]);
  const isSprintCard = $derived(!!firstSprint);
  $effect(() => {
    if (!isSprintCard) return;
    return untrack(() => engine.subscribe<MySprintList>({ path: '/sprints' }, (next) => {
      if (next.status === 'ready') mySprints = next.data.data;
    }));
  });
  const myEntriesIn = (sprint: string) => mySprints.find((each) => each.sprint.id === sprint)?.my_entries ?? 0;
  /** Each hub's plan as the scheduler places it, for its phases. */
  let scheduleAt = $state<Record<string, PlanSchedule>>({});
  eachHub<PlanSchedule>('PLANNING', (hubId) => `/plans/${hubId}/schedule?zone=${encodeURIComponent(actor.zone)}`, (hubId, data) => { if (Array.isArray(data.entries)) scheduleAt = { ...scheduleAt, [hubId]: data }; });
  /** The hubs with a plan - dated work, a sprint running or a release to come - each with its phases. */
  const plansListed = $derived(
    module !== 'PLANNING' ? [] : hubsWith
      .map((hub) => ({ hub, phases: phasesOf(scheduleAt[hub.id]?.entries ?? [], today) }))
      .filter(({ hub, phases }) => phases.length > 0 || sprintsRunning(sprintsAt, collectionsOfHub(hub.id), 1).length > 0 || nextReleaseOf(releasesAt, hub.id) !== undefined),
  );
  const sprintLeft = (endOn: string) => t('app.sprints.days_left', { count: String(sprintDaysLeft(endOn, today)) });

  /** The assets' home opens with the inventory's health across hubs (§2.2). */
  let healthAt = $state<Record<string, CmdbHealth>>({});
  eachHub<CmdbHealth>('ASSETS', healthPath, (hubId, data) => (healthAt = { ...healthAt, [hubId]: data }));
  const assetHubs = $derived(module === 'ASSETS' ? hubsWith.map((hub) => hub.id) : []);
  const inventory = $derived(inventorySummary(healthAt, assetHubs));
  // The inventory itself across hubs, by class: a shipped class's word is the catalogue's, an own
  // class's its name, so the classes are read where the home lists them.
  const byClass = $derived(inventoryByClass(healthAt, assetHubs));
  let ownClasses = $state<CiClassList['data']>([]);
  const readsClasses = $derived(module === 'ASSETS' && assetHubs.length > 0);
  $effect(() => {
    if (!readsClasses) return;
    return untrack(() => engine.subscribe<CiClassList>({ path: classesPath }, (next) => {
      if (next.status === 'ready') ownClasses = next.data.data;
    }));
  });
  const classOf = (key: string) => classWord(key, ownClasses, (code) => (messages.has(code) ? t(code) : undefined));
  const heldIn = (held: (typeof byClass)[number]) =>
    held.hubs.map((each) => t('app.product_home.held_in', { count: String(each.total), hub: hubName(each.hubId) })).join(' · ');
  /** *For you* on the inventory: what needs attention across hubs, each item by its name. */
  const ATTENTION_SHOWN = 5;
  const attention = $derived(module === 'ASSETS' && currentTab === homeViews[0]?.id ? attentionAcross(healthAt, assetHubs, ATTENTION_SHOWN) : []);
  let ciNames = $state<Record<string, string>>({});
  $effect(() => {
    const unknown = attention.map((each) => each.ci_id).filter((id) => !(id in ciNames));
    untrack(() => {
      for (const id of unknown) void cmdb.read(id).then((ci) => { if (ci) ciNames = { ...ciNames, [id]: ci.name }; });
    });
  });

  /** The hubs the home lists: on the assets' home those that hold an inventory (§2.2). */
  const hubsListed = $derived(module === 'ASSETS' ? hubsWithAssetsFirst(hubsWith, healthAt) : hubsWith);
  /** What a hub's row says of it beside its name (§2.2): the plan's next release, the hub's inventory. */
  function hubLine(hubId: string): string | undefined {
    if (module === 'PLANNING') {
      const sprint = sprintsRunning(sprintsAt, collectionsOfHub(hubId), 1)[0];
      const next = nextReleaseOf(releasesAt, hubId);
      const said = [
        ...(sprint ? [t('app.product_home.sprint_line', { name: sprint.name, remaining: sprintLeft(sprint.end_on) })] : []),
        ...(next ? [t('app.product_home.next_release', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) })] : []),
      ];
      return said.length > 0 ? said.join(' · ') : undefined;
    }
    if (module === 'ASSETS') {
      const health = healthAt[hubId];
      if (!health) return undefined;
      return [
        t('app.assets.figure_total', { count: health.total }),
        ...(health.problems.length > 0 ? [t('app.assets.figure_wanting', { count: health.problems.length })] : []),
      ].join(' · ');
    }
    return undefined;
  }

  /**
   * A plan's line in *Plans in your hubs* (scene homeplans): late, the phase now and the days;
   * else its sprint and next release; else on plan with the entries still open.
   */
  function planLine(hubId: string, phase: string | undefined, slip: number): string {
    if (slip > 0) return [phase, t('app.product_home.plan_late', { count: slip })].filter(Boolean).join(' · ');
    const sprint = sprintsRunning(sprintsAt, collectionsOfHub(hubId), 1)[0];
    const next = nextReleaseOf(releasesAt, hubId);
    if (sprint || next) {
      return [sprint?.name, next ? t('app.product_home.release_on', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) }) : undefined].filter(Boolean).join(' · ');
    }
    return [t('app.product_home.plan_on_track'), t('app.product_home.plan_open', { count: openOf(hubId) })].join(' · ');
  }

  /** The releases card names what a release still waits for: the one entry open, else done of all. */
  let releaseAt = $state<Record<string, ReleaseDetail>>({});
  const cardReleases = $derived(module === 'PLANNING' && currentTab === 'foryou' ? upcoming.slice(0, 3).map((each) => each.id).join(',') : '');
  $effect(() => {
    const ids = cardReleases === '' ? [] : cardReleases.split(',');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<ReleaseDetail>({ path: releasePath(id) }, (next) => {
        if (next.status === 'ready') releaseAt = { ...releaseAt, [id]: next.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  function releaseLine(id: string): string | undefined {
    const detail = releaseAt[id];
    if (!detail) return undefined;
    const open = detail.entries.filter((entry) => !entry.done);
    if (open.length === 1) return t('app.product_home.release_one_open', { title: open[0]!.title });
    return t('app.product_home.release_done_of', { done: detail.done_count, total: detail.done_count + detail.open_count });
  }

  const boards = $derived(module === 'WHITEBOARD' ? recentBoards(boardsAt, hubsWith.map((hub) => hub.id), isWhole ? Infinity : BOARDS_SHOWN) : []);
  /** *Recently opened* (scene whome 2): the boards opened on this device, else the last changed. */
  const continued = $derived(
    module === 'WHITEBOARD'
      ? boardsToContinue(boardsOpened(actor.account?.id), recentBoards(boardsAt, hubsWith.map((hub) => hub.id), Infinity), boards, BOARDS_DRAWN)
      : { boards: [], opened: false },
  );

  /** The plans' *Roadmaps*: every portfolio's roadmap, read once its tab is open. */
  let portfolios = $state<PortfolioList['data'] | undefined>(undefined);
  $effect(() => {
    if (currentTab !== 'roadmaps') return;
    return untrack(() => engine.subscribe<PortfolioList>({ path: PORTFOLIOS }, (next) => {
      if (next.status === 'ready') portfolios = next.data.data;
      else if (next.status === 'failed') portfolios = [];
    }));
  });

  /** A board is made at once in the hub holding most of them (scene wnew): blank with its templates as a sheet, or from a card. */
  async function makeBoardIn(hubId: string, choice: BoardStart = 'BLANK'): Promise<void> {
    try {
      onnavigate(await makeBoard(hubId, choice));
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
  function startBoard(choice: BoardStart): void {
    const hub = createPlaces[0];
    if (hub) void makeBoardIn(hub.id, choice);
  }
  /** The open comments naming the reader on the boards listed, for the band (scene whome 1). */
  let boardComments = $state(0);
  const hubName = (id: string) => containers.find(id)?.name ?? '';

  const creation = $derived(hubCreationHere());
  const mayCreateHub = $derived(creation === 'shared' || creation === 'private');
  let isCreatingHub = $state(false);

  // The first visit's templates (user-experience.md §4): the catalogue's that switch this product
  // on, read only where the reader may make the hub one starts, and only while there is none.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  // The desk offers them while it has no desk, though hubs exist: a request type of the workspace
  // makes none (UC-SVC-05), and a service template's hub does.
  // A product switched on at the workspace is on in every hub at once, so the first visit is
  // decided by whether anything of it exists yet - where the home's reads answer that whole.
  const objectCounts = $derived.by((): Record<string, number> | undefined => {
    const count = <T,>(at: Record<string, T>, of: (data: T) => number) =>
      Object.fromEntries(Object.entries(at).map(([hubId, data]) => [hubId, of(data)]));
    if (module === 'KNOWLEDGE') return count(pagesAt, (data) => data.length);
    if (module === 'WHITEBOARD') return count(boardsAt, (data) => data.length);
    if (module === 'ASSETS') return count(healthAt, (data) => data.total);
    if (module === 'PLANNING') {
      const entries = Object.fromEntries(Object.entries(scheduleAt).map(([hubId, data]) => [hubId, data.entries]));
      return planCounts(hubsWith.map((hub) => hub.id), releasesAt, entries, sprintsAt, (id) => containers.find(id)?.parent_id ?? undefined);
    }
    return undefined;
  });
  const isUnusedHere = $derived(
    !isReading && hubsWith.length > 0 && objectCounts !== undefined && isUnused(objectCounts, hubsWith.map((hub) => hub.id)) === true,
  );
  const isFirstVisit = $derived(mayCreateHub && !isReading && (hubsWith.length === 0 || module === 'SERVICE_DESK' || isUnusedHere));
  // The catalogue is read by whoever holds STRUCTURE on the workspace; anybody else starts from the
  // hub dialog's presets, and asks nothing the server would refuse.
  $effect(() => {
    if (!isFirstVisit || holdsHere('STRUCTURE') !== true) return;
    return untrack(() => config.templates((next: { status: string; data?: ConfigTemplateList }) => {
      if (next.status === 'ready' && Array.isArray(next.data?.data)) catalogue = next.data.data;
    }));
  });
  /** The template the new hub is to take, chosen on the empty home. */
  let startingFrom = $state<string | undefined>(undefined);
  const templateName = (template: ConfigTemplate): string =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  const starts = $derived(isFirstVisit ? productTemplates(catalogue, module).map((template) => ({
    label: templateName(template),
    description: template.origin === 'HUBTASK' && messages.has(`app.templates.summary.${template.key.toUpperCase()}`) ? t(`app.templates.summary.${template.key.toUpperCase()}`) : undefined,
    icon: row?.icon,
    onclick: () => { startingFrom = template.key; isCreatingHub = true; },
  })) : []);
  const createHub = $derived(mayCreateHub ? { label: t('app.workspace.create_hub'), onclick: () => { startingFrom = undefined; isCreatingHub = true; } } : undefined);
  // The home's create act (UX-2): the product's own words, opening the create dialog on the place's
  // screen of it - the place holding most of the product, the others in its menu. Without a hub the empty state's *Create hub* is it.
  const createAct = $derived(createOf(module));
  // A create act at a collection reads the collections of the hubs where the product is on.
  const collectionHubs = $derived(createAct?.level === 'COLLECTION' ? hubsWith.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = collectionHubs ? collectionHubs.split(',') : [];
    const stops = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => {
      for (const stop of stops) stop();
    };
  });
  // A create act at a hub reads the reader's role in each, and offers only where they may write
  // (P-05): a viewer is offered no act the place's screen would not open.
  const writeHubs = $derived(createAct?.level === 'HUB' ? hubsWith.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = writeHubs ? writeHubs.split(',') : [];
    const stops = untrack(() => ids.map((hubId) => people.open({ hubId })));
    return () => {
      for (const stop of stops) stop();
    };
  });
  const mayWriteIn = (hubId: string): boolean | undefined => {
    const role = people.roleOf({ hubId }, actor.account?.id, roleOrder());
    return role === undefined ? undefined : holds(role ?? undefined, 'WRITE_ITEMS').status === 'permitted';
  };
  const createPlaces = $derived(
    createAct?.level === 'COLLECTION'
      ? hubsWith.flatMap((hub) => containers.collectionsOf(hub.id).map((collection) => ({ id: collection.id, label: `${hub.name} › ${collection.name}` })))
      : placesByUse(writableHubs(hubsWith, mayWriteIn).map((hub) => ({ id: hub.id, label: hub.name })), objectCounts),
  );
  const primary = $derived.by(() => {
    // The home's act stays on its setup tabs (§5.4): the head is the home's on every tab of it.
    if (!createAct || isReading || (currentTab !== homeViews[0]?.id && !ADMINISTERED.includes(currentTab ?? ''))) return undefined;
    const first = createPlaces[0];
    if (!first) return undefined;
    // The place's own create dialog opens there (§5.2): the act creates, it does not list.
    const go = (id: string) => (module === 'WHITEBOARD' ? void makeBoardIn(id) : onnavigate(createAt(module, id) ?? `/hubs/${id}`));
    return {
      label: t(createAct.code),
      icon: 'plus' as const,
      onclick: () => go(first.id),
      menu: createPlaces.length > 1 ? { label: t(createAct.inCode), items: createPlaces, onselect: go } : undefined,
    };
  });
  /** The first use's create act on the empty answer, where the head would otherwise carry it. */
  const firstCreate = $derived(primary ? { label: primary.label, onclick: primary.onclick } : undefined);
  // The plans' side way is a project file brought into a collection (scene homeplans 4): the plan's
  // own import (UC-PLN-09), offered where the create act leads, into the place it leads to first.
  let isImportingPlan = $state(false);
  let importInto = $state<string | undefined>(undefined);
  const importPlan = $derived(
    module === 'PLANNING' && firstCreate && createPlaces.length > 0
      ? { label: t('app.product_home.import_plan'), onclick: () => { importInto = createPlaces[0]?.id; isImportingPlan = true; } }
      : undefined,
  );
  const startEmpty = $derived(firstCreate && starts.length > 0 ? { label: t('app.product_empty.start_empty'), onclick: () => { startingFrom = undefined; isCreatingHub = true; } } : undefined);
  // The band of a product with its own colour (work-surface E-1): its tile and two or three figures,
  // each a word with its number, and none while the reads are out (P-11).
  const BANDS: Partial<Record<string, { product: 'plans' | 'assets' | 'pages' | 'whiteboards'; icon: 'calendar' | 'shield' | 'book-open' | 'layout-template' }>> = {
    PLANNING: { product: 'plans', icon: 'calendar' },
    KNOWLEDGE: { product: 'pages', icon: 'book-open' },
    ASSETS: { product: 'assets', icon: 'shield' },
    WHITEBOARD: { product: 'whiteboards', icon: 'layout-template' },
  };
  const band = $derived(BANDS[module]);
  const bandFigures = $derived.by((): string[] => {
    if (isReading || isUnusedHere) return [];
    if (module === 'PLANNING') {
      return [
        t('app.product_home.band_sprints', { count: running.length }),
        t('app.product_home.band_releases', { count: upcoming.length }),
        ...(plansLate > 0 ? [t('app.product_home.band_plans_late', { count: plansLate })] : []),
      ];
    }
    if (module === 'WHITEBOARD' && hubsWith.every((hub) => boardsAt[hub.id] !== undefined)) {
      const all = hubsWith.flatMap((hub) => boardsAt[hub.id] ?? []);
      const changedToday = all.filter((board) => todayIn(actor.zone, Date.parse(board.updated_at)) === today).length;
      return [
        t('app.product_home.band_boards', { count: all.length }),
        ...(changedToday > 0 ? [t('app.product_home.band_boards_today', { count: changedToday })] : []),
        ...(boardComments > 0 ? [t('app.product_home.band_boards_comments', { count: boardComments })] : []),
      ];
    }
    if (module === 'KNOWLEDGE' && isPagesWhole && knowledgeHubs.length > 0) {
      const figures = pagesFigures(pagesAt, knowledgeHubs, now, toReview.length);
      return [
        t('app.product_home.band_pages', { count: figures.pages }),
        t('app.product_home.band_spaces', { count: figures.spaces }),
        t('app.product_home.band_verified', { count: figures.verified }),
        ...(figures.toReview > 0 ? [t('app.product_home.band_to_review', { count: figures.toReview })] : []),
      ];
    }
    if (module === 'ASSETS' && inventory.isWhole && assetHubs.length > 0) {
      return [
        t('app.product_home.band_items', { count: inventory.total }),
        ...(inventory.wanting > 0 ? [t('app.product_home.band_attention', { count: inventory.wanting })] : []),
      ];
    }
    return [];
  });
  /** *For you* is never empty (E-4): where nothing runs and nothing is due, it says so and leads on. */
  const isPlanQuiet = $derived(module === 'PLANNING' && currentTab === 'foryou' && !isReading && !isUnusedHere && hubsWith.length > 0 && running.length === 0 && upcoming.length === 0);

  /** A hub made from a template opens where its main product is; any other on the product's screen. */
  function created(id: string, landing?: string): void {
    startingFrom = undefined;
    onnavigate(landing ?? hubScreenOf(module, id) ?? `/hubs/${id}`);
  }
</script>

{#snippet todayNote()}
  <Button tone="secondary" icon="calendar" isBusy={isOpeningToday} onclick={openToday} data-today-note>{t('app.notes.today')}</Button>
{/snippet}

{#if module === 'SERVICE_DESK'}
  <DeskHome hubs={hubsWith} {isReading} {tab} {onnavigate} {createHub} templates={starts} />
{:else}
<WorkPage>
  {#snippet top()}
  {#if band}
  <ProductBand
    product={band.product}
    icon={band.icon}
    {name}
    figures={bandFigures}
    primary={isUnusedHere ? undefined : primary}
    aside={module === 'KNOWLEDGE' ? todayNote : undefined}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: name },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />
  {:else}
  <PageHeader
    title={name}
    subtitle={t(`app.modules.hint_${module.toLowerCase()}`)}
    isTitleInBar={viewport.isCompact}
    primary={isUnusedHere ? undefined : primary}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: name },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {/if}
  {/snippet}
  {#snippet tabs()}
    {#if homeViews.length > 1}
      <ViewBar label={name} views={homeViews} isCompact={viewport.isCompact} closeLabel={t('app.sheet_close')} current={currentTab} onnavigate={(id) => onnavigate(homeViews.find((each) => each.id === id)?.href ?? '/')} />
    {/if}
  {/snippet}

  {#if currentTab === 'notes'}
    <NotesView {onnavigate} isEmbedded />
  {:else if currentTab === 'roadmaps'}
    <section aria-labelledby="product-roadmaps" aria-busy={portfolios === undefined}>
      <Stack gap="100">
        <h2 class="heading" id="product-roadmaps">{t('app.product_home.tab_roadmaps')}</h2>
        {#if portfolios === undefined}
          <Skeleton lines={3} />
        {:else if portfolios.length === 0}
          <EmptyState kind="unused" icon="calendar" title={t('app.product_home.roadmaps_none_title')} description={t('app.product_home.roadmaps_none')} />
        {:else}
          <ul class="hubs">
            {#each portfolios as each (each.id)}
              {@const at = `/portfolios/${each.id}/roadmap`}
              <li>
                <ListRow href={at} onactivate={() => onnavigate(at)} label={each.name}>
                  {#snippet leading()}<Icon name="calendar" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        {/if}
      </Stack>
    </section>
  {:else if currentTab === 'templates'}
    {#if createPlaces.length > 0}
      <BoardTemplates look="tiles" onpick={startBoard} />
    {:else if !isReading}
      <EmptyState kind="unused" icon="layout-template" title={t('app.product_home.templates_none_title')} description={t('app.product_home.templates_none')} />
    {/if}
  {:else}
    {#if isUnusedHere && currentTab === homeViews[0]?.id}
      <ProductEmpty product={module} place={{ type: 'TENANT' }}
        kind="unused"
        icon={row?.icon ?? 'hub'}
        title={module === 'PLANNING' || module === 'ASSETS' ? name : t('app.product_home.unused', { product: name })}
        description={messages.has(`app.modules.hint_${module.toLowerCase()}`) ? t(`app.modules.hint_${module.toLowerCase()}`) : t(starts.length > 0 ? 'app.product_home.unused_body_templates' : 'app.product_home.unused_body')}
        create={firstCreate}
        templates={starts}
        isProductHome
        templateShape={module === 'PLANNING' || module === 'ASSETS' ? 'row' : 'stacked'}
        side={module === 'PLANNING' ? importPlan : startEmpty}
      />
    {/if}
    {#if assetHubs.length > 0 && Object.keys(healthAt).length > 0 && !isUnusedHere}
      <section aria-labelledby="product-inventory" aria-busy={!inventory.isWhole}>
        <Stack gap="100">
          <h2 class="heading" id="product-inventory">{t('app.product_home.inventory')}</h2>
          <dl class="figures">
            <div><dt>{t('app.assets.health_total')}</dt><dd>{inventory.total}</dd></div>
            <div><dt>{t('app.assets.problems')}</dt><dd>{inventory.wanting}</dd></div>
            <div><dt>{t('app.assets.review_overdue')}</dt><dd>{inventory.reviewOverdue}</dd></div>
          </dl>
          {#if inventory.wanting > 0}<p class="why">{t('app.assets.attention_why')}</p>{/if}
        </Stack>
      </section>
      {#if attention.length > 0}
        <section aria-labelledby="product-attention" data-attention>
          <Stack gap="100">
            <h2 class="heading" id="product-attention">{t('app.product_home.attention')}</h2>
            <ul class="hubs">
              {#each attention as problem (problem.hubId + problem.ci_id)}
                <li data-attention-ci={problem.ci_id}>
                  <ListRow href={`/cis/${problem.ci_id}`} onactivate={() => onnavigate(`/cis/${problem.ci_id}`)} label={ciNames[problem.ci_id] ?? t('app.assets.unknown_ci')}>
                    {#snippet leading()}<Icon name="triangle-alert" size="sm" />{/snippet}
                    <span class="name">{ciNames[problem.ci_id] ?? t('app.assets.unknown_ci')}</span>
                    <span class="purpose">{problem.kinds.map((kind) => t(`app.assets.kind.${kind}`)).join(', ')}</span>
                    {#snippet trailing()}<span class="card-from">{hubName(problem.hubId)}</span>{/snippet}
                  </ListRow>
                </li>
              {/each}
            </ul>
          </Stack>
        </section>
      {/if}
      {#if byClass.length > 0}
        <section aria-labelledby="product-classes">
          <Stack gap="100">
            <h2 class="heading" id="product-classes">{t('app.product_home.inventory_by_class')}</h2>
            <ul class="hubs">
              {#each byClass as held (held.class)}
                {@const at = `/hubs/${held.hubs[0]?.hubId ?? ''}/assets`}
                <li data-class-held={held.class}>
                  <ListRow href={at} onactivate={() => onnavigate(at)} label={classOf(held.class)}>
                    {#snippet leading()}<Icon name="shield" size="sm" />{/snippet}
                    <span class="name" title={classOf(held.class)}>{classOf(held.class)} · {held.total}</span>
                    <span class="purpose">{heldIn(held)}</span>
                  </ListRow>
                </li>
              {/each}
            </ul>
          </Stack>
        </section>
      {/if}
    {/if}
    {#if isPlanQuiet}
      <p class="quiet" data-plan-quiet>{t('app.product_home.plans_quiet')}</p>
    {/if}
    {#if module === 'PLANNING' && currentTab === 'foryou' && (firstSprint || upcoming.length > 0)}
      <!-- *For you* (scene homeplans): the sprint that ends first with its burndown, the releases to come. -->
      <div class="plan-cards">
        {#if firstSprint}
          {@const where = containers.find(firstSprint.collection_id)}
          {@const last = firstDetail?.sprint.id === firstSprint.id ? firstDetail.burndown.at(-1) : undefined}
          <div class="plan-card" data-plan-card="sprint" data-sprint={firstSprint.id}>
            <Widget title={t('app.product_home.sprint_card', { name: firstSprint.name })} tile={{ icon: 'calendar', product: 'plans' }}
              figure={sprintLeft(firstSprint.end_on)}
              line={last ? t('app.sprints.done_of', { done: String(last.scope - last.remaining), scope: String(last.scope), unit: t(`app.sprints.unit_${firstSprint.unit}`) }) : undefined}
              act={{ label: t('app.today.card_open'), onclick: () => onnavigate(`/collections/${firstSprint.collection_id}/sprints`) }}>
              {#if firstDetail?.sprint.id === firstSprint.id}<SprintBurndown detail={firstDetail} />{/if}
              <span class="card-from" data-sprint-line>{[where?.name, t('app.today.sprint_mine', { count: myEntriesIn(firstSprint.id) })].filter(Boolean).join(' · ')}</span>
            </Widget>
          </div>
        {/if}
        {#if upcoming.length > 0}
          <div class="plan-card" data-plan-card="releases">
            <Widget title={t('app.product_home.releases_due')} tile={{ icon: 'flag', product: 'plans' }}>
              <ul class="card-rows">
                {#each upcoming.slice(0, 3) as each (each.id)}
                  <li>
                    <a href={`/hubs/${each.hub_id}/releases`}>{each.name}</a>
                    <span class="when">{formatPlainDay(each.release_on, messages.locale)}</span>
                    <span class="card-from" data-release-line>{releaseLine(each.id) ?? hubName(each.hub_id)}</span>
                  </li>
                {/each}
              </ul>
            </Widget>
          </div>
        {/if}
      </div>
    {/if}
    {#if module === 'PLANNING' && currentTab === 'foryou' && plansListed.length > 0}
      <section aria-labelledby="product-plans">
        <Stack gap="100">
          <h2 class="heading" id="product-plans">{t('app.product_home.plans_in_hubs')}</h2>
          <ul class="hubs">
            {#each plansListed as { hub, phases } (hub.id)}
              {@const path = hubScreenOf(module, hub.id) ?? `/hubs/${hub.id}`}
              {@const now = phases.find((each) => each.state === 'now')}
              {@const slip = slipOf(hub.id)}
              {@const line = planLine(hub.id, now?.title, slip)}
              <li data-plan={hub.id}>
                <ListRow href={path} onactivate={() => onnavigate(path)} label={hub.name}>
                  {#snippet leading()}<Icon name={markOf(hub)} size="sm" />{/snippet}
                  <span class="name" title={hub.name}>{hub.name}</span>
                  <span class="purpose" class:late={slip > 0} data-hub-line data-plan-late={slip > 0 ? '' : undefined}>{line}</span>
                  {#snippet trailing()}
                    {#if phases.length > 0}
                      <span class="phases" role="img" aria-label={t('app.product_home.phases_label', { phases: phases.map((each) => each.title).join(', ') })} data-phases>
                        {#each phases as phase (phase.id)}<span class="phase" data-phase={phase.state} data-late={phase.state === 'now' && slip > 0 ? '' : undefined} style:flex-grow={phase.days}></span>{/each}
                      </span>
                    {/if}
                  {/snippet}
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if running.length > 0 && currentTab === 'sprints'}
      <section aria-labelledby="product-sprints">
        <Stack gap="100">
          <h2 class="heading" id="product-sprints">{t('app.product_home.sprints_running')}</h2>
          <ul class="hubs">
            {#each running as each (each.id)}
              {@const where = containers.find(each.collection_id)}
              <li data-running-sprint={each.id}>
                <ListRow href={`/collections/${each.collection_id}/sprints`} onactivate={() => onnavigate(`/collections/${each.collection_id}/sprints`)} label={each.name}>
                  {#snippet leading()}<Icon name="timer" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                  <span class="purpose">{[where ? `${hubName(where.parent_id ?? '')} › ${where.name}` : '', sprintLeft(each.end_on)].filter(Boolean).join(' · ')}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if upcoming.length > 0 && currentTab === 'releases'}
      <section aria-labelledby="product-releases">
        <Stack gap="100">
          <h2 class="heading" id="product-releases">{t('app.product_home.releases_due')}</h2>
          <ul class="hubs">
            {#each upcoming as each (each.id)}
              <li>
                <ListRow href={`/hubs/${each.hub_id}/releases`} onactivate={() => onnavigate(`/hubs/${each.hub_id}/releases`)} label={each.name}>
                  {#snippet leading()}<Icon name="flag" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                  <span class="purpose">{[hubName(each.hub_id), t('app.releases.due_on', { day: formatPlainDay(each.release_on, messages.locale) })].filter(Boolean).join(' · ')}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if reading.length > 0 && currentTab === 'foryou'}
      <section aria-labelledby="product-reading" data-pages-reading>
        <Stack gap="100">
          <h2 class="eyebrow" id="product-reading">{t('app.product_home.pages_continue')}</h2>
          <ul class="read-cards">
            {#each reading as each (each.id)}
              <li>
                <a class="read-card" href={`/pages/${each.id}`} onclick={(event) => { event.preventDefault(); onnavigate(`/pages/${each.id}`); }}>
                  <span class="read-top">
                    <span class="read-icon" aria-hidden="true">{#if each.icon}{each.icon}{:else}<Icon name="book-open" size="md" />{/if}</span>
                    {#if isVerified(each, now)}<StateLozenge size="sm" tone="success" icon="shield" label={t('app.product_home.verified')} />{/if}
                  </span>
                  <span class="read-title">{each.title}</span>
                  <span class="purpose">{[hubName(each.hub_id), formatRelative(each.updated_at, messages.locale)].filter(Boolean).join(' · ')}</span>
                </a>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if module === 'KNOWLEDGE' && currentTab === 'foryou' && (toReview.length > 0 || spacesVerified.length > 0)}
      <div class="pages-pair">
        {#if toReview.length > 0}
          <section class="panel" aria-labelledby="product-review" data-pages-review>
            <div class="panel-head">
              <h2 class="heading" id="product-review"><Icon name="triangle-alert" size="sm" /> {t('app.product_home.pages_review_count', { count: toReview.length })}</h2>
              <span class="purpose">{t('app.product_home.pages_review_yours')}</span>
            </div>
            <ul class="review-rows">
              {#each toReview as each (each.id)}
                <li>
                  <a class="name" href={`/pages/${each.id}`} title={each.title} onclick={(event) => { event.preventDefault(); onnavigate(`/pages/${each.id}`); }}>{each.icon ? `${each.icon} ` : ''}{each.title}</a>
                  <span class="purpose">{hubName(each.hub_id)}</span>
                  {#if each.verified_until}<StateLozenge size="sm" tone="warning" icon="clock" label={t('app.product_home.review_since', { count: daysOverdue(each.verified_until, now) })} />{/if}
                  <Button size="sm" tone="secondary" icon="shield" onclick={() => onnavigate(`/pages/${each.id}`)}>{t('app.product_home.review')}</Button>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
        {#if spacesVerified.length > 0}
          <section class="panel" aria-labelledby="product-verified" data-pages-verified>
            <h2 class="eyebrow" id="product-verified">{t('app.product_home.spaces_verified')}</h2>
            <ul class="space-rows">
              {#each spacesVerified as space (space.hubId)}
                <li>
                  <a class="name" href={`/hubs/${space.hubId}/pages`} onclick={(event) => { event.preventDefault(); onnavigate(`/hubs/${space.hubId}/pages`); }}>{hubName(space.hubId)}</a>
                  <span class="purpose">{t('app.product_home.verified_of', { verified: space.verified, total: space.total })}</span>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      </div>
    {:else if toReview.length > 0 && currentTab === 'foryou'}
      <section aria-labelledby="product-review">
        <Stack gap="100">
          <h2 class="heading" id="product-review">{t('app.product_home.pages_review')}</h2>
          <ul class="hubs">
            {#each toReview as each (each.id)}
              <li>
                <ListRow href={`/pages/${each.id}`} onactivate={() => onnavigate(`/pages/${each.id}`)} label={each.title}>
                  {#snippet leading()}<Icon name="book-open" size="sm" />{/snippet}
                  <span class="name" title={each.title}>{each.title}</span>
                  <span class="purpose">{hubName(each.hub_id)}</span>
                  {#snippet trailing()}<Badge tone="warning" size="sm">{t('app.pages.due_for_review')}</Badge>{/snippet}
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if recent.length > 0 && currentTab === 'foryou'}
      <section aria-labelledby="product-pages">
        <Stack gap="100">
          <h2 class="heading" id="product-pages">{t('app.product_home.pages_recent')}</h2>
          <ul class="hubs">
            {#each recent as each (each.id)}
              <li>
                <ListRow href={`/pages/${each.id}`} onactivate={() => onnavigate(`/pages/${each.id}`)} label={each.title}>
                  {#snippet leading()}<Icon name="book-open" size="sm" />{/snippet}
                  <span class="name" title={each.title}>{each.title}</span>
                  <span class="purpose">{[hubName(each.hub_id), formatRelative(each.updated_at, messages.locale)].filter(Boolean).join(' · ')}</span>
                  {#snippet trailing()}
                    {#if each.due_for_review}<StateLozenge size="sm" tone="warning" icon="clock" label={t('app.pages.due_for_review')} />
                    {:else if isVerified(each, now)}<StateLozenge size="sm" tone="success" icon="shield" label={t('app.product_home.verified')} />{/if}
                  {/snippet}
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if module === 'WHITEBOARD' && currentTab === 'foryou' && !isUnusedHere}
      <BoardsForYou {boards} drawn={continued.boards} opened={continued.opened} {hubName} {onnavigate} onstart={createPlaces.length > 0 ? startBoard : undefined} oncomments={(count) => (boardComments = count)} />
    {/if}
    {#if boards.length > 0 && currentTab === 'all'}
      <section aria-labelledby="product-boards">
        <Stack gap="100">
          <h2 class="heading" id="product-boards">{t('app.product_home.boards_recent')}</h2>
          <ul class="cards">
            {#each boards as board (board.id)}
              <li>
                <BoardCard {board} meta={[hubName(board.hub_id), t('app.whiteboards.objects', { count: String(board.element_count) }), formatRelative(board.updated_at, messages.locale)].filter(Boolean).join(' · ')} onopen={onnavigate} />
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if hubScreenOf(module, '') !== undefined && ((showsHubs && !(module === 'PLANNING' && currentTab === 'foryou' && !isReading && hubsWith.length > 0)) || (!isReading && hubsWith.length === 0))}
      <section aria-labelledby="product-hubs">
        <Stack gap="100">
          <h2 class="heading" id="product-hubs">{t('app.product_home.hubs')}</h2>
          {#if isReading && hubsWith.length === 0}
            <div aria-busy="true"><Skeleton lines={3} /></div>
          {:else if hubsWith.length === 0}
            <ProductEmpty product={module} place={{ type: 'TENANT' }}
              kind="unused"
              icon={row?.icon ?? 'hub'}
              title={t('app.product_home.none', { product: name })}
              description={t(mayCreateHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')}
              create={createHub}
              templates={starts}
            />
          {:else}
            <ul class="hubs">
              {#each hubsListed as hub (hub.id)}
                {@const path = hubScreenOf(module, hub.id) ?? `/hubs/${hub.id}`}
                {@const line = hubLine(hub.id)}
                <li>
                  <ListRow href={path} onactivate={() => onnavigate(path)} label={hub.name}>
                    {#snippet leading()}<Icon name={markOf(hub)} size="sm" />{/snippet}
                    <span class="name" title={hub.name}>{hub.name}</span>
                    {#if line}<span class="purpose" data-hub-line>{line}</span>{/if}
                    {#snippet trailing()}
                      {#if hub.figure && !hub.figure.unavailable}<CollectionFigure figure={hub.figure} name={hub.name} />{/if}
                    {/snippet}
                  </ListRow>
                </li>
              {/each}
            </ul>
          {/if}
        </Stack>
      </section>
    {/if}
  {/if}
</WorkPage>
{/if}

{#if isImportingPlan && importInto}
  <Dialog bind:isOpen={isImportingPlan} title={t('app.planning.import')} dismissLabel={t('app.links.cancel')} size="m">
    {#if createPlaces.length > 1}
      <Select label={t('app.product_home.import_into')} value={importInto} options={createPlaces.map((place) => ({ value: place.id, label: place.label }))}
        onchange={(event: Event) => (importInto = (event.currentTarget as HTMLSelectElement).value)} />
    {/if}
    {#key importInto}
      <PlanImport collectionId={importInto} zone={actor.zone} ondone={() => { isImportingPlan = false; if (importInto) onnavigate(`/collections/${encodeURIComponent(importInto)}/timeline`); }} />
    {/key}
  </Dialog>
{/if}

{#if isCreatingHub && mayCreateHub}
  <CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" isPrivateOnly={creation === 'private'} template={startingFrom} oncreated={created} />
{/if}

<style>
  /* Boards are visual, so they are cards (§5.1), as many to a row as fit. */
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
  .purpose { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  /* *For you* on the plans: two cards side by side, one column where they do not fit. */
  .plan-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--sp-1600) * 2)), 1fr)); gap: var(--sp-150); }
  .card-from { color: var(--text-subtle); font-size: var(--fs-075); }
  .card-rows { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .card-rows li { display: grid; grid-template-columns: minmax(0, 1fr) auto; column-gap: var(--sp-100); }
  .card-rows a { overflow: hidden; color: var(--text-primary); font-weight: var(--fw-semibold); text-decoration: none; text-overflow: ellipsis; white-space: nowrap; }
  .card-rows a:hover { text-decoration: underline; }
  .card-rows .when { color: var(--text-secondary); font-size: var(--fs-075); font-variant-numeric: tabular-nums; }
  .late { color: var(--text-danger); }
  /* A plan's phases as one bar: done, now, later; the one now late says so in the line. */
  .phases { display: flex; gap: var(--sp-025); inline-size: var(--sp-1600); block-size: var(--sp-100); }
  .phase { flex-basis: 0; border-radius: var(--r-sm); background: var(--accent-primary-subtle); }
  .phase[data-phase='done'] { background: var(--accent-primary); }
  .phase[data-phase='now'] { background: var(--accent-primary); }
  .phase[data-late] { background: var(--status-danger-accent); }

  /* The same figures as a hub's health section draws (lib/assets/Health.svelte). */
  .figures { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1600)), 1fr)); gap: var(--sp-100); margin: 0; }
  .figures div {
    display: grid; gap: var(--sp-025); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .figures dt { color: var(--text-secondary); font-size: var(--fs-075); }
  .figures dd { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .why { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  /* The pages' home (scene phome): cards to continue reading, the reviews beside how verified each space is. */
  .eyebrow { margin: 0; color: var(--text-subtle); font-size: var(--fs-065); font-weight: var(--fw-semibold); text-transform: uppercase;
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.06em; }
  .read-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
  .read-card {
    display: grid; gap: var(--sp-100); align-content: start; block-size: 100%; padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-raised);
    color: var(--text-primary); text-decoration: none;
  }
  .read-card:hover { border-color: var(--border-default); }
  .read-card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .read-top { display: flex; justify-content: space-between; align-items: flex-start; gap: var(--sp-100); }
  .read-icon { font-size: var(--fs-300); line-height: var(--lh-tight); }
  .read-title { font-weight: var(--fw-semibold); }
  .pages-pair { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); align-items: start; min-inline-size: 0; }
  /* design-system-lint-ignore: `primitive.breakpoint.large` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .pages-pair { grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); }
    .pages-pair > [data-pages-verified] { grid-column: -2 / -1; }
  }
  .panel { display: grid; grid-template-columns: minmax(0, 1fr); min-inline-size: 0; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-raised); }
  .panel > .eyebrow { padding: var(--sp-150) var(--sp-200) var(--sp-050); }
  .panel-head { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-100); padding: var(--sp-150) var(--sp-200); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .panel-head .heading { display: inline-flex; gap: var(--sp-050); align-items: center; }
  .review-rows, .space-rows { margin: 0; padding: 0; list-style: none; }
  .review-rows li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); padding: var(--sp-100) var(--sp-200); }
  .review-rows li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .review-rows .name { flex: 1 1 var(--sp-1600); min-inline-size: 0; color: var(--text-primary); text-decoration: none; }
  .space-rows li .purpose { flex: none; }
  .space-rows li { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-100); padding: var(--sp-100) var(--sp-200); }
  .space-rows .name { flex: 1; min-inline-size: 0; color: var(--text-primary); text-decoration: none; }
  .review-rows .name:hover, .space-rows .name:hover { text-decoration: underline; }
  .hubs { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* One line with an ellipsis and the whole name as the tooltip (user-experience.md §2.5). */
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
