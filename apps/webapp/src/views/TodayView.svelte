<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { platform } from '../lib/platform/index.ts';
  // *Today*: the page the app opens on (UC-WRK-34), in the overview's place (ADR-0063 decision 1),
  // arranged as the work-surface concept's scene `today` (§6.1): one row of equal cards, each with
  // its one act; two columns that end level, *your day* and *needs you* with the sprint under it,
  // eight rows each; then tiles over the full width. *Sections* hides and orders the cards and the
  // tiles for the person, stored on the account (E-17). Nobody else's day is shown - every read
  // here is the reader's own or narrowed to what they may see.
  //
  // **Composed of reads that already exist**: one wordless search for what is on the reader, the
  // collections of each hub with their figures (the same reads the tree makes), one entry per
  // project, the jumble's first page; what was opened last is this device's and is sent nowhere.
  // What the reads mean - the day's bands, which collection is a tile - is `lib/today.ts`.
  //
  // **Ticking off is the ordinary completion**: `items.setCompleted`, the moment if it is one, the
  // announcement - and queued like any other while the server cannot be reached. The row keeps its
  // place and shows the tick, so a mistaken one is undone where it was made.
  //
  // **Nothing here says what the person did not do** (check 6): what is past is listed plainly
  // under *Earlier*, with its date and without the overdue word.
  //
  // **Every panel says something when it is empty**, so "nothing here" is never mistaken for a
  // screen that has not finished loading.

  import { untrack } from 'svelte';

  import type { AgentList, AgentRequest, AgentRequestList, ApprovalList, ConfigTemplate, ConfigTemplateList, EntrySlaList, ItemQueryResult, MySprintList, SprintDetail, TimesheetWeek, TimeTimer, UnreadPostList, WorkItem } from '@hubtask/sync-engine';
  import { Button, Checkbox, Dialog, Disclosure, EmptyState, Icon, IconButton, Kbd, PageHeader, Skeleton, Stack, Tabs, TaskRow, UnreadDot, Widget, WidgetStrip, WorkPage } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';

  import { announcer } from '../lib/announce.svelte.ts';
  import { celebration } from '../lib/celebration.svelte.ts';
  import CelebrationSlot from '../lib/entries/CelebrationSlot.svelte';
  import CollectionFigure from '../lib/entries/CollectionFigure.svelte';
  import DueMark from '../lib/entries/DueMark.svelte';
  import EntryActions from '../lib/entries/EntryActions.svelte';
  import EntryMenu from '../lib/entries/EntryMenu.svelte';
  import { attachEntryGestures } from '../lib/entries/entryGestures.ts';
  import { entryMenu } from '../lib/entries/entryMenu.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, holdsHere, hubCreationHere, mayDo, reachesAdministrationArea, roleOrder, supports } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { postMoment, unreadPostsPath } from '../lib/data/posts.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { ticketKeyOf } from '../lib/data/ticket.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { jumble } from '../lib/data/jumble.svelte.ts';
  import { readMailSettings } from '../lib/data/mailintake.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import IntakeAddress from '../lib/mail/IntakeAddress.svelte';
  import { archivalOfItem } from '../lib/data/lifecycle.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { today } from '../lib/data/today.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatPlainDay, formatRelative, formatToday } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { recents } from '../lib/recents.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { TODAY_TILES, arrangeSections, bandsOf, breachedCount, capped, cardsOf, dayRows, deskAnswerQuery, greetingOf, heldActRows, hiddenSections, isListFigure, isProjectFigure, movedSection, nearestFirst, productBandsOf, runningDue, sprintDaysLeft, timedCount, timeTodayOf, type TodayCard, type TodaySection, type TodayTile } from '../lib/today.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { entrySlaPath } from '../lib/data/desk.svelte.ts';
  import { watchPageReviews, type ReviewRow } from '../lib/data/reviews.svelte.ts';
  import SprintBurndown from '../lib/plans/SprintBurndown.svelte';
  import { sprintPath } from '../lib/data/sprints.svelte.ts';
  import SlaRemaining from '../lib/servicedesk/SlaRemaining.svelte';
  import { agents, agentsPath, requestsPath } from '../lib/data/agents.svelte.ts';
  import { approvals, myApprovalsPath } from '../lib/data/approvals.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { TIMER, elapsedMinutes, minutesText, time, weekPath, weekStartOf } from '../lib/data/time.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { moreInHubtask } from '../lib/product/home.ts';
  import { products } from '../lib/navigation.ts';
  import { tryTemplateOf } from '../lib/product/templates.ts';
  import { config } from '../lib/data/config.svelte.ts';
  import { capture } from '../lib/capture/capture.svelte.ts';
  import { mayCaptureHere } from '../lib/capture/places.svelte.ts';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';
  import { createHubPath } from '../lib/workspace/createHub.ts';
  import StarterDialog from '../lib/workspace/StarterDialog.svelte';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import EntryPlace from '../lib/workspace/EntryPlace.svelte';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();
  // A requester holding nothing else never stands here: the frame sends them to their portal (PO-2).

  // The empty workspace's one action: a shared hub with STRUCTURE on the workspace, a private one
  // of one's own where the manifest offers it, nothing otherwise (UC-WRK-01 check 6, UC-ID-16
  // check 1). Not the head's: a workspace with hubs creates them in the navigation (ux-inventory
  // §E9).
  const creation = $derived(hubCreationHere());
  const createLabel = $derived(
    creation === 'shared' ? t('app.workspace.create_hub') : creation === 'private' ? t('app.workspace.create_private_hub') : undefined,
  );

  $effect(() => page.entitle(t('app.nav.today')));

  const zone = $derived(actor.zone);
  const now = $derived.by(() => Date.now());
  const dayLine = $derived(formatToday(messages.locale, zone, now));
  // The day starts with the person (design-system.md §11.7): a greeting by the hour where they are,
  // with the first word of their name where they gave one, beside the date.
  const firstName = $derived(actor.account?.display_name?.trim().split(/\s+/u)[0] || undefined);
  const greeting = $derived.by(() => {
    const code = `app.today.greeting_${greetingOf(zone, now)}`;
    return firstName ? t(`${code}_named`, { name: firstName }) : t(code);
  });

  // `untrack`, for the reason every store records: subscribing delivers the current state at once
  // and the listener writes the store, and writing it reads it - so an effect that both subscribes
  // and tracks that read re-runs on its own first delivery, for ever.
  $effect(() => untrack(() => today.open()));

  // The jumble's only for somebody who reads the workspace itself (UC-JUM-01 check 7): anybody else
  // would be refused the read, and the refusal recorded on every visit here.
  /** What waits: the undecided arrivals only - one sorted in or dismissed is decided. */
  const WAITING = 'NEW';
  const hasJumble = $derived(holdsHere('READ') === true);
  $effect(() => {
    if (!hasJumble) return;
    return untrack(() => jumble.open(WAITING));
  });

  // Every hub's collections, for their figures: the tree's own reads, so a hub the tree has open
  // costs nothing more. Keyed by the ids, so a hub arriving opens its level and nothing re-reads.
  const hubIds = $derived(containers.hubs.map((hub) => hub.id).join(' '));
  $effect(() => {
    const ids = hubIds ? hubIds.split(' ') : [];
    const stops = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => stops.forEach((stop) => stop());
  });

  const collections = $derived(containers.hubs.flatMap((hub) => containers.collectionsOf(hub.id)));
  /** Whether every hub's collections have answered: until then nobody knows which are lists. */
  const areCollectionsSettled = $derived(
    containers.hubsState.status === 'ready' && containers.hubs.every((hub) => !containers.isLevelLoading(hub.id)),
  );
  const lists = $derived(collections.filter(isListFigure));
  const projects = $derived(collections.filter(isProjectFigure));
  const listIds = $derived(new Set(lists.map((list) => list.id)));

  const projectIds = $derived(projects.map((project) => project.id).join(' '));
  $effect(() => {
    const ids = projectIds ? projectIds.split(' ') : [];
    const stops = untrack(() => ids.map((id) => today.openStep(id)));
    return () => stops.forEach((stop) => stop());
  });

  /** The bands, from the entries as the server answered them: a tick does not move a row. */
  const bands = $derived(bandsOf(today.mine, zone, now, (id) => listIds.has(id)));
  const isMineEmpty = $derived(bands.earlier.length + bands.today.length + bands.later.length === 0);

  const failure = $derived(today.state.status === 'failed' ? renderProblem(today.state.error, messages) : undefined);
  const isLoading = $derived(today.state.status === 'loading' || today.state.status === 'idle' || !areCollectionsSettled);

  const waiting = $derived(jumble.of(WAITING));
  // The workspace's mail intake address, for the card while nothing waits and for the first steps
  // (ADR-0094): read once where the operator's receiving domain makes one and the intake is on.
  // Only its administrators may read the settings; for everybody else nothing is drawn.
  let jumbleAddress = $state<string | undefined>(undefined);
  $effect(() => {
    if (!hasJumble || manifest.value?.features?.mail_domain !== true) return;
    untrack(() => void readMailSettings().then((read) => (jumbleAddress = read?.enabled ? (read.address ?? undefined) : undefined), () => undefined));
  });
  const isJumbleLoading = $derived(jumble.stateOf(WAITING).status === 'loading' || jumble.stateOf(WAITING).status === 'idle');

  // What this page ticked, by entry: the search is a walk already made and is not re-read for one
  // entry (as on the search page), so the row shows the answer the completion brought back.
  let ticked = $state<Record<string, boolean>>({});
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const isDone = (item: WorkItem) => ticked[item.id] ?? item.completion?.is_completed ?? false;

  async function toggle(item: WorkItem) {
    writeFailure = undefined;
    const wasDone = isDone(item);
    try {
      // Queued while the server is away, the answer is the copy's prediction - and nothing where
      // the copy does not hold the entry yet, which is still a completion on its way.
      const answered: WorkItem | undefined = await items.setCompleted(item.id, !wasDone, crypto.randomUUID());
      ticked = { ...ticked, [item.id]: answered?.completion?.is_completed ?? !wasDone };
      announcer.say(t(wasDone ? 'app.entries.reopened_announced' : 'app.entries.completed_announced', { title: item.title }));
      if (answered?.completion?.is_completed) void celebration.celebrate(answered);
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * A row's menu: the one every surface draws (UC-WRK-28 check 1). *Today* mixes collections and
   * holds no level, so, as on the search page, the role is read along each row's collection with
   * the rows and the moves wait for the level read when the menu opens.
   */
  const rowPath = (item: WorkItem) => ({
    hubId: containers.find(item.collection_id)?.parent_id ?? undefined,
    collectionId: item.collection_id,
  });
  const shown = $derived([
    ...bands.earlier,
    ...bands.today,
    ...bands.later,
    ...projects.map((project) => today.stepOf(project.id)).filter((step): step is WorkItem => step !== undefined),
  ]);
  $effect(() => {
    const paths = shown.map(rowPath);
    untrack(() => {
      for (const path of paths) people.open(path);
    });
  });

  function menuOf(item: WorkItem) {
    const role = people.roleOf(rowPath(item), actor.account?.id, roleOrder()) ?? undefined;
    const isMine = item.assignee_id != null && item.assignee_id === actor.account?.id;
    return entryMenu(
      {
        sendsLinks: platform.sendLink !== undefined,
        surface: 'today',
        item: { title: item.title, isCompleted: isDone(item), hasDue: Boolean(item.due_at) },
        archival: archivalOfItem(item, false),
        isCollectionReadOnly: containers.find(item.collection_id)?.effective_archived ?? false,
        mayChange: mayDo(role, 'change', isMine),
        mayCreate: mayDo(role, 'create', isMine) === true,
        mayShare: holds(role, 'MANAGE_MEMBERS').status === 'permitted',
        carries: (capability) => supports(item.type, capability).status === 'permitted',
        level: actions?.levelOf(item),
      },
      t,
    );
  }

  const menus = $state<Record<string, EntryMenu | undefined>>({});
  let actions = $state<EntryActions | undefined>(undefined);
  let panels = $state<HTMLElement | null>(null);

  function openMenu(id: string) {
    const item = shown.find((each) => each.id === id);
    if (item) actions?.readLevel(item);
    menus[id]?.open(0);
  }

  // A right click, the context-menu key and a long press open the same menu as its ⋯.
  $effect(() => {
    const node = panels;
    if (!node) return;
    return attachEntryGestures(node, {
      idAt: (target) => (target as Element | null)?.closest?.('[data-entry-row]')?.getAttribute('data-entry-row') ?? undefined,
      open: openMenu,
    });
  });

  /** *Your day*: eight rows across its bands, the rest behind "all n" (§6.1). */
  const shownBands = $derived(dayRows(bands));
  const mineTotal = $derived(bands.earlier.length + bands.today.length + bands.later.length);
  const mineHidden = $derived(shownBands.rest);

  /** The jumble's acts at a row: only for who may decide about an arrival. */
  const mayWriteJumble = $derived(holdsHere('WRITE_ITEMS') === true);
  const channelIcon = (channel: string) => (channel === 'EMAIL' ? 'mail' : channel === 'WEBHOOK' ? 'zap' : 'inbox');
  const subjectOf = (arrival: { raw_subject?: string | null }) => arrival.raw_subject?.trim() || t('app.jumble.no_subject');
  async function dismissArrival(id: string) {
    try {
      await jumble.dismiss(id);
      announcer.say(t('app.jumble.dismissed_announced'));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * A workspace with nothing in it yet for this reader: no entry on them, no list, no project,
   * nothing opened, nothing waiting. Instead of empty cards, the first steps (concept round 2, §1).
   */
  const looksNew = $derived(
    !isLoading && !failure && !(hasJumble && isJumbleLoading) && isMineEmpty && lists.length === 0 && projects.length === 0 && recents.rows.length === 0 && (!hasJumble || waiting.length === 0),
  );
  /**
   * Whether a hub holds an entry the reader can see, asked only while the page looks new: nothing
   * on the reader is no sign of a new workspace - an owner of a busy one holds nothing themselves,
   * and *First steps* there would tell them to create the hub that exists. One entry a hub.
   */
  let hubUse = $state<Record<string, boolean>>({});
  $effect(() => {
    if (!looksNew) return;
    const ids = hubIds ? hubIds.split(' ') : [];
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<ItemQueryResult>(
        { path: '/items:query', body: { scope: { container_id: id, include_descendants: true }, page: { size: 1 } } },
        (next) => {
          if (next.status === 'ready') hubUse = { ...hubUse, [id]: next.data.data.length > 0 };
          // A hub that cannot be read is not counted as used: the steps stay a newcomer's.
          if (next.status === 'failed') hubUse = { ...hubUse, [id]: false };
        },
      ));
      return () => stops.forEach((stop) => stop());
    });
  });
  const isWorkspaceUsed = $derived(containers.hubs.some((hub) => hubUse[hub.id] === true));
  const isUseSettled = $derived(containers.hubs.every((hub) => hubUse[hub.id] !== undefined));
  const isNewWorkspace = $derived(looksNew && isUseSettled && !isWorkspaceUsed);
  /** *In use here*: the hubs and the products of a workspace that holds work the reader has none of. */
  const inUse = $derived(looksNew && isWorkspaceUsed);
  const inUseProducts = $derived(products({ modulesOn: manifest.value?.modules_on }));
  const mayInvite = $derived(holdsHere('MANAGE_MEMBERS') === true);
  /** Today's *Choose what you use Hubtask for* opens the starter's dialog again (§3.3). */
  let isChoosing = $state(false);

  /**
   * *More in Hubtask* (user-experience.md §3.3): the products that are off, for whoever may switch
   * one on. *Try it* opens the product's home, which offers the switch and says what it does.
   * Dismissed for good on this device - a convenience kept in the browser, so a private window or
   * blocked storage shows it again rather than failing.
   */
  const MORE_KEY = 'hubtask.today.more';
  let isMoreDismissed = $state(false);
  $effect(() => {
    try {
      isMoreDismissed = localStorage.getItem(MORE_KEY) === 'dismissed';
    } catch {
      isMoreDismissed = false;
    }
  });
  function dismissMore() {
    isMoreDismissed = true;
    try {
      localStorage.setItem(MORE_KEY, 'dismissed');
    } catch {
      // Storage refused: dismissed for this visit only.
    }
  }
  const moreProducts = $derived(isMoreDismissed ? [] : moreInHubtask({ modulesOn: manifest.value?.modules_on, mayShape: holdsHere('STRUCTURE') === true }));
  // *Try it* takes the product's template through the hub dialog (user-experience.md §3.3): a hub
  // made from it opens on the template's plan, which switches the product on. A product no
  // template starts, or a reader who may make no hub, goes to its home and its switch.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  $effect(() => {
    if (moreProducts.length === 0 || (creation !== 'shared' && creation !== 'private')) return;
    return untrack(() => config.templates((next: { status: string; data?: ConfigTemplateList }) => {
      if (next.status === 'ready' && Array.isArray(next.data?.data)) catalogue = next.data.data;
    }));
  });
  function tryProduct(product: { readonly module: string; readonly path: string }): void {
    const template = creation === 'shared' || creation === 'private' ? tryTemplateOf(catalogue, product.module) : undefined;
    if (!template) {
      onnavigate?.(product.path);
      return;
    }
    onnavigate?.(createHubPath({ template: template.key }));
  }
  const hasCompany = $derived(people.along({}).filter((membership) => membership.scope_type === 'TENANT').length > 1);

  /**
   * The products' bands (user-experience.md §3.3, UC-WRK-34 check 8), each read only where its
   * module is on - a read of a module that is off is refused, and a refusal is not a day.
   */
  const productBands = $derived(productBandsOf(manifest.value?.modules_on));
  let waitingApprovals = $state<ApprovalList['data'] | undefined>(undefined);
  $effect(() => {
    if (!productBands.includes('approvals')) {
      waitingApprovals = undefined;
      return;
    }
    return untrack(() => engine.subscribe<ApprovalList>({ path: myApprovalsPath }, (next) => {
      if (next.status === 'ready') waitingApprovals = next.data.data.filter((approval) => approval.state === 'PENDING');
    }));
  });
  async function approve(approval: ApprovalList['data'][number]) {
    try {
      await approvals.decide(approval, 'APPROVE', '');
      toast.show({ text: t('app.approvals.approved_announced'), tone: 'success' });
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
  /**
   * The agents' acts held for the reader's yes (§3.3), where agents are on: `GET /agent-requests`
   * answers only those the reader may decide; the agents' names come from their own list.
   */
  let heldActs = $state<AgentRequestList['data'] | undefined>(undefined);
  let agentList = $state<AgentList['data']>([]);
  $effect(() => {
    if (!productBands.includes('agent_acts')) {
      heldActs = undefined;
      return;
    }
    return untrack(() => {
      const stops = [
        engine.subscribe<AgentRequestList>({ path: requestsPath('PENDING') }, (next) => { if (next.status === 'ready') heldActs = next.data.data; }),
        engine.subscribe<AgentList>({ path: agentsPath }, (next) => { if (next.status === 'ready') agentList = next.data.data; }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const heldRows = $derived(heldActRows(heldActs, agentList, actor.account?.id, reachesAdministrationArea() === true));
  async function decideAct(id: string, approve: boolean) {
    const request = heldActs?.find((each) => each.id === id);
    if (!request) return;
    try {
      await agents.decide(request as AgentRequest, approve);
      toast.show({ text: t(approve ? 'app.agents.request_approved' : 'app.agents.request_declined'), tone: 'success' });
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
  /** The pages waiting for the reader's review (§3.3), where pages are on at their hub. */
  let reviews = $state<ReviewRow[]>([]);
  $effect(() => {
    const ids = productBands.includes('reviews') && hubIds ? hubIds.split(' ') : [];
    const viewer = actor.account?.id;
    if (ids.length === 0) {
      reviews = [];
      return;
    }
    return untrack(() => watchPageReviews(ids, viewer, (rows) => (reviews = rows)));
  });
  const waitingCount = $derived((waitingApprovals?.length ?? 0) + heldRows.length + reviews.length);

  /**
   * *Needs your answer* (§3.3): the desk entries on the reader with an SLA target running, the one
   * about to breach first on top across the hubs.
   */
  const DESK_SHOWN = 8;
  let deskAt = $state<Record<string, WorkItem[]>>({});
  $effect(() => {
    const ids = productBands.includes('desk') && hubIds ? hubIds.split(' ') : [];
    return untrack(() => {
      const stops = ids.map((hubId) => engine.subscribe<ItemQueryResult>({ path: '/items:query', body: deskAnswerQuery(hubId, DESK_SHOWN) }, (next) => {
        if (next.status === 'ready') deskAt = { ...deskAt, [hubId]: next.data.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const deskCandidates = $derived(productBands.includes('desk') && hubIds ? hubIds.split(' ').flatMap((id) => deskAt[id] ?? []) : []);
  // Each hub answers its own nearest first; rows from more than one hub are merged by their due
  // moments, read from each entry's SLA (the rows shown read the same path again).
  const deskMerged = $derived(productBands.includes('desk') && hubIds ? hubIds.split(' ').filter((id) => (deskAt[id]?.length ?? 0) > 0).length > 1 : false);
  // Every candidate's due moment, not only where hubs merge: the reply card counts the breached.
  const deskCandidateIds = $derived(deskCandidates.map((entry) => entry.id).join(' '));
  let deskDue = $state<Record<string, number | undefined>>({});
  $effect(() => {
    const ids = deskCandidateIds ? deskCandidateIds.split(' ') : [];
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<EntrySlaList>({ path: entrySlaPath(id) }, (next) => {
        if (next.status === 'ready') deskDue = { ...deskDue, [id]: runningDue(next.data.data) };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const deskRows = $derived((deskMerged ? nearestFirst(deskCandidates, deskDue) : deskCandidates).slice(0, DESK_SHOWN));
  // The rows' time left is read against this moment; it moves on while the page stays open.
  let deskNow = $state(new Date());
  $effect(() => {
    if (deskRows.length === 0) return;
    const every = setInterval(() => (deskNow = new Date()), 60_000);
    return () => clearInterval(every);
  });

  /** *Sprint* (§3.3): the running sprints the reader holds entries in, each with its burndown. */
  const SPRINTS_SHOWN = 2;
  let mySprints = $state<MySprintList['data']>([]);
  $effect(() => {
    if (!productBands.includes('sprint')) {
      mySprints = [];
      return;
    }
    return untrack(() => engine.subscribe<MySprintList>({ path: '/sprints' }, (next) => {
      if (next.status === 'ready') mySprints = next.data.data;
    }));
  });
  let sprintDetails = $state<Record<string, SprintDetail>>({});
  const sprintIds = $derived(mySprints.slice(0, SPRINTS_SHOWN).map((each) => each.sprint.id).join(' '));
  $effect(() => {
    const ids = sprintIds ? sprintIds.split(' ') : [];
    const at = zone;
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<SprintDetail>({ path: sprintPath(id, at) }, (next) => {
        if (next.status === 'ready') sprintDetails = { ...sprintDetails, [id]: next.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const day = $derived(todayIn(zone));
  let week = $state<TimesheetWeek | undefined>(undefined);
  let timer = $state<TimeTimer | undefined>(undefined);
  $effect(() => {
    if (!productBands.includes('time')) {
      week = undefined;
      timer = undefined;
      return;
    }
    const path = weekPath(weekStartOf(day) ?? day);
    return untrack(() => {
      const stops = [
        engine.subscribe<TimesheetWeek>({ path }, (next) => { if (next.status === 'ready') week = next.data; }),
        engine.subscribe<TimeTimer>({ path: TIMER }, (next) => { if (next.status === 'ready') timer = next.data; }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  // A running timer counts on while the page is open; nothing ticks while none runs.
  let tick = $state(Date.now());
  $effect(() => {
    if (!timer?.running) return;
    tick = Date.now();
    const every = setInterval(() => (tick = Date.now()), 30_000);
    return () => clearInterval(every);
  });
  const timeToday = $derived.by(() => {
    const at = timeTodayOf(week, day, timer);
    return at?.running && timer?.started_at ? { ...at, running: { ...at.running, minutes: elapsedMinutes(timer.started_at, tick) } } : at;
  });


  const moment = $derived(celebration.current);

  const recentHref = (kind: string, id: string) => (kind === 'item' ? `/items/${id}` : kind === 'hub' ? `/hubs/${id}` : `/collections/${id}`);

  /**
   * *Sections* (E-17): the cards and tiles this person's roles and modules give, in the order they
   * gave them, the hidden left out - read from the account, so it follows them to every device.
   * A change is drawn at once and written whole; a refusal puts the account's own back.
   */
  let localOrder = $state<readonly string[] | undefined>(undefined);
  let localHidden = $state<readonly string[] | undefined>(undefined);
  const sectionOrder = $derived(localOrder ?? actor.account?.today_sections ?? []);
  const sectionHidden = $derived(localHidden ?? actor.account?.today_sections_hidden ?? []);
  const givenCards = $derived(cardsOf(manifest.value?.modules_on));
  // *New for you* (B-5): the published posts the reader has not seen, across every hub they read.
  const hasPosts = $derived(manifest.value?.modules_on?.includes('POSTS') ?? false);
  const newPosts = $derived(hasPosts ? resource<UnreadPostList>(unreadPostsPath) : undefined);
  const unreadPosts = $derived(newPosts?.state.status === 'ready' ? newPosts.state.data.data : []);
  $effect(() => accounts.resolve(unreadPosts.map((post) => post.created_by)));
  const givenTiles = $derived<TodayTile[]>(TODAY_TILES.filter((tile) => (tile !== 'INBOX' || hasJumble) && (tile !== 'NEWS' || hasPosts)));
  const cards = $derived(arrangeSections(givenCards, sectionOrder, sectionHidden));
  const tiles = $derived(arrangeSections(givenTiles, sectionOrder, sectionHidden));
  let isArranging = $state(false);

  async function writeSections(order: readonly string[], hidden: readonly string[]) {
    const id = actor.account?.id;
    if (!id) return;
    localOrder = order;
    localHidden = hidden;
    try {
      await preferences.setAccount(id, { today_sections: [...order] as TodaySection[], today_sections_hidden: [...hidden] as TodaySection[] });
      await actor.reread();
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    } finally {
      localOrder = undefined;
      localHidden = undefined;
    }
  }
  /** One group's move writes the whole order: this group's new order, the other group's kept. */
  function moveSection(isCard: boolean, key: TodaySection, by: -1 | 1) {
    const group: readonly TodaySection[] = isCard ? givenCards : givenTiles;
    const moved = movedSection(group, sectionOrder, key, by);
    const others = arrangeSections<TodaySection>(isCard ? givenTiles : givenCards, sectionOrder, undefined).shown;
    void writeSections(isCard ? [...moved, ...others] : [...others, ...moved], sectionHidden);
  }
  const sectionGroups = $derived([
    { code: 'app.today.sections_cards', isCard: true, ordered: arrangeSections<TodaySection>(givenCards, sectionOrder, undefined).shown },
    { code: 'app.today.sections_tiles', isCard: false, ordered: arrangeSections<TodaySection>(givenTiles, sectionOrder, undefined).shown },
  ]);
  function showSection(key: TodaySection, isShown: boolean) {
    void writeSections(sectionOrder, hiddenSections(sectionHidden, key, !isShown));
  }
  const sectionCode: Record<TodaySection, string> = {
    OVERDUE: 'app.today.card_overdue',
    DUE_TODAY: 'app.today.card_due_today',
    REPLY: 'app.today.desk',
    TIME: 'app.today.time',
    APPROVALS: 'app.today.approvals',
    COLLECTIONS: 'app.today.collections',
    NEWS: 'app.today.news',
    INBOX: 'app.today.jumble',
    RECENT: 'app.today.recent',
  };

  const searchHref = (f: string) => `/search?${new URLSearchParams({ f })}`;
  /** Whether the figures behind the cards are still on their way: a zero then would be unmeasured. */
  const isDayBusy = $derived(isLoading || failure !== undefined);
  const breached = $derived(breachedCount(deskCandidates.map((entry) => entry.id), deskDue, deskNow.getTime()));
  const oldestOverdue = $derived(bands.earlier.map((item) => item.due_at).filter((at): at is string => !!at).sort()[0]);
  const yesLine = $derived(
    [
      (waitingApprovals?.length ?? 0) > 0 ? t('app.today.yes_approvals', { count: waitingApprovals?.length ?? 0 }) : undefined,
      heldRows.length > 0 ? t('app.today.yes_acts', { count: heldRows.length }) : undefined,
      reviews.length > 0 ? t('app.today.yes_reviews', { count: reviews.length }) : undefined,
    ].filter(Boolean).join(', '),
  );
  async function stopTimer() {
    try {
      await time.stopTimer(zone, weekStartOf(day));
      toast.show({ text: t('app.today.timer_stopped'), tone: 'success' });
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * *Needs you*: what waits for this person's answer, one row and one act each - the desk's
   * entries nearest their deadline first, then the approvals, the agents' held acts and the pages
   * due for review. Eight at most; the rest is behind the column's "all".
   */
  type NeedRow =
    | { kind: 'desk'; id: string; entry: WorkItem }
    | { kind: 'approval'; id: string; approval: ApprovalList['data'][number] }
    | { kind: 'act'; id: string; act: (typeof heldRows)[number] }
    | { kind: 'review'; id: string; review: ReviewRow };
  const needRows = $derived<NeedRow[]>([
    ...deskRows.map((entry) => ({ kind: 'desk' as const, id: entry.id, entry })),
    ...(waitingApprovals ?? []).map((approval) => ({ kind: 'approval' as const, id: approval.id, approval })),
    ...heldRows.map((act) => ({ kind: 'act' as const, id: act.id, act })),
    ...reviews.map((review) => ({ kind: 'review' as const, id: review.id, review })),
  ]);
  const needs = $derived(capped(needRows));
  const hasNeeds = $derived(productBands.includes('desk') || productBands.includes('approvals') || productBands.includes('agent_acts') || productBands.includes('reviews') || productBands.includes('sprint'));

  /** On a phone *your day* and the overview are a switch (§7); everywhere else they stand together. */
  // Kept for this tab, so *back* from an entry returns to the half the reader was in (R-15).
  const VIEW_KEY = 'hubtask.today.view';
  let phoneView = $state<'day' | 'overview'>(readView());
  function readView(): 'day' | 'overview' {
    try {
      return sessionStorage.getItem(VIEW_KEY) === 'overview' ? 'overview' : 'day';
    } catch {
      return 'day';
    }
  }
  $effect(() => {
    const view = phoneView;
    try {
      sessionStorage.setItem(VIEW_KEY, view);
    } catch {
      // Storage refused: the switch is this visit's only.
    }
  });
  const hasTiles = $derived(tiles.shown.length > 0 || inUse || moreProducts.length > 0);
</script>

{#snippet entry(item: WorkItem, hasPlace: boolean)}
  {#if moment && moment.item.id === item.id}
    <CelebrationSlot current={moment} />
  {/if}
  <div data-entry-row={item.id}>
    <TaskRow
      type={item.type}
      title={item.title}
      href={`/items/${item.id}`}
      isCompleted={isDone(item)}
      completeLabel={t('app.entries.complete', { title: item.title })}
      hasTwist={false}
      entryKey={ticketKeyOf(containers.find(item.collection_id), item.number)}
      onToggleComplete={() => toggle(item)}
    >
      {#snippet trailing()}
        {#if hasPlace}<EntryPlace collectionId={item.collection_id} withHub />{/if}<DueMark {item} />
        <EntryMenu
          bind:this={menus[item.id]}
          title={item.title}
          items={menuOf(item)}
          onselect={(chosen) => void actions?.run(item, chosen)}
          onopening={() => actions?.readLevel(item)}
        />
      {/snippet}
    </TaskRow>
  </div>
{/snippet}

{#snippet band(code: string, rows: readonly WorkItem[], all: number, isLate = false)}
  {#if rows.length > 0}
    <div class="band-group">
      <!-- The late band's heading is the one red on the page, and it is a word (§3): never the tint alone. -->
      <h3 class="band" class:late={isLate}>{t(code)} <span class="band-count">{all}</span></h3>
      <Stack gap="050">
        {#each rows as item (item.id)}{@render entry(item, true)}{/each}
      </Stack>
    </div>
  {/if}
{/snippet}

<!-- One card of the row: its figure, one line, the whole card the way to it - a running timer's
     *Stop* a control of its own beside the link; absent where the role or module is. -->
{#snippet card(key: TodayCard)}
  <div class="card" data-card={key}>
    {#if key === 'OVERDUE'}
      <Widget title={t('app.today.card_overdue')} tile={{ icon: 'triangle-alert', color: 'red' }} figure={String(bands.earlier.length)} isBusy={isDayBusy}
        figureTone={bands.earlier.length > 0 ? { tone: 'danger', word: t('app.today.card_overdue_word') } : undefined}
        line={oldestOverdue ? t('app.today.card_overdue_line', { day: formatPlainDay(oldestOverdue.slice(0, 10), messages.locale) }) : undefined}
        href={searchHref('who:me is:open due:overdue')} />
    {:else if key === 'DUE_TODAY'}
      <Widget title={t('app.today.card_due_today')} tile={{ icon: 'calendar', color: 'blue' }} figure={String(bands.today.length)} isBusy={isDayBusy}
        line={timedCount(bands.today) > 0 ? t('app.today.card_timed', { count: timedCount(bands.today) }) : undefined}
        href={searchHref('who:me is:open due:today')} />
    {:else if key === 'REPLY'}
      <Widget title={t('app.today.desk')} tile={{ icon: 'inbox', product: 'desk' }} figure={String(deskCandidates.length)}
        figureTone={breached > 0 ? { tone: 'danger', word: t('app.today.card_breached_word') } : undefined}
        line={breached > 0 ? t('app.today.card_breached', { count: breached }) : undefined}
        href="/desk" />
    {:else if key === 'TIME'}
      <Widget title={t('app.today.time')} tile={{ icon: 'clock', product: 'time' }} figure={timeToday ? minutesText(timeToday.today) : undefined} isBusy={timeToday === undefined}
        line={timeToday ? [t('app.today.card_time_line', { week: minutesText(timeToday.week) }), timeToday.running ? t('app.today.card_timer_running') : undefined].filter(Boolean).join(' · ') : undefined}
        href="/time" act={timeToday?.running ? { label: t('app.today.card_stop'), onclick: () => void stopTimer() } : undefined} />
    {:else if key === 'APPROVALS'}
      <Widget title={t('app.today.approvals')} tile={{ icon: 'user-check', color: 'green' }} figure={String(waitingCount)} line={yesLine || undefined}
        href="/approvals" />
    {/if}
  </div>
{/snippet}

<!-- *Your day*: the capture field on top, then what is overdue, due today, and upcoming folded. -->
{#snippet yourDay()}
  <Widget title={isNewWorkspace ? t('app.today.first_steps') : t('app.today.day')}
    count={!isLoading && !failure && !isNewWorkspace && mineTotal > 0 ? t('app.today.day_count', { today: bands.today.length, overdue: bands.earlier.length }) : undefined}>
    <div class="day" data-day bind:this={panels}>
      {#if mayCaptureHere() && !isNewWorkspace}
        <!-- Capture is the one way to add an entry (§8, UC-WRK-26/7): the field opens it. -->
        <button type="button" class="capture" data-capture-field onclick={() => capture.summon()}>
          <Icon name="plus" size="sm" /><span class="capture-words">{t('app.today.capture')}</span><Kbd size="sm">Q</Kbd>
        </button>
      {/if}
      {#if writeFailure}
        <p class="failure" role="alert">{writeFailure.message}</p>
      {/if}
      {#if failure}
        <FailureState problem={failure} onRetry={() => today.open()()} />
      {:else if isLoading || (looksNew && !isUseSettled)}
        <div aria-busy="true"><Skeleton lines={3} /></div>
      {:else if isNewWorkspace}
        <!-- The first steps, each ticked where it is already done, each with its act. -->
        <ul class="steps">
          {#if workspace.workspace?.starter_skipped === true}
            <li data-step-choose>
              <span class="step-mark" aria-hidden="true"></span><span class="step-word">{t('app.today.step_choose')}</span>
              <Button size="sm" tone="secondary" onclick={() => (isChoosing = true)}>{t('app.today.step_choose_act')}</Button>
            </li>
          {/if}
          <li data-done=""><span class="step-mark" aria-hidden="true"><Icon name="check" size="sm" /></span><span class="step-word">{t('app.today.step_hub')}</span></li>
          <li>
            <span class="step-mark" aria-hidden="true"></span><span class="step-word">{t('app.today.step_capture')}</span>
            {#if mayCaptureHere()}<Button size="sm" tone="secondary" icon="plus" onclick={() => capture.summon()}>{t('app.capture.open_field')}</Button>{/if}
          </li>
          {#if mayInvite}
            <li data-done={hasCompany ? '' : undefined}>
              <span class="step-mark" aria-hidden="true">{#if hasCompany}<Icon name="check" size="sm" />{/if}</span><span class="step-word">{t('app.today.step_invite')}</span>
              {#if !hasCompany}<Button size="sm" tone="secondary" onclick={() => onnavigate?.('/administration/people')}>{t('app.today.step_invite_act')}</Button>{/if}
            </li>
          {/if}
          {#if jumbleAddress}
            <li data-step-mail>
              <span class="step-mark" aria-hidden="true"></span><span class="step-word">{t('app.today.step_mail')}</span>
              <IntakeAddress address={jumbleAddress} />
            </li>
          {/if}
        </ul>
      {:else if isMineEmpty}
        <p class="none">{t('app.today.mine_none')}</p>
      {:else}
        {@render band('app.today.earlier', shownBands.earlier, bands.earlier.length, true)}
        {@render band('app.today.today', shownBands.today, bands.today.length)}
        {#if shownBands.later.length > 0}
          <!-- Upcoming folded while something is due sooner; alone, it is the day and stands open. -->
          <div class="later" data-later>
            <Disclosure label={t('app.today.later')} description={t('app.today.later_folded', { count: bands.later.length })} isOpen={bands.earlier.length + bands.today.length === 0}>
              <Stack gap="050">
                {#each shownBands.later as item (item.id)}{@render entry(item, true)}{/each}
              </Stack>
            </Disclosure>
          </div>
        {/if}
        {#if mineHidden > 0 || today.hasMore}
          <!-- The rest is the search's: `has_more` as well as the rows held back, because a search
               answers what the caller may see, so a short page is not the end. -->
          <p class="more"><a href={searchHref('who:me is:open')}>{today.hasMore ? t('app.today.mine_more') : t('app.today.all', { count: mineTotal })}</a></p>
        {/if}
      {/if}
    </div>
  </Widget>
{/snippet}

<!-- *Needs you*: one row and one act each, eight at most; the sprint stands under it. -->
{#snippet needsYou()}
  <div class="needs-column">
    <Widget title={t('app.today.needs')} count={needRows.length > 0 ? String(needRows.length) : undefined}>
      {#if needRows.length === 0}
        <p class="none">{t('app.today.needs_none')}</p>
      {:else}
        <ul class="needs">
          {#each needs.shown as row (row.kind + row.id)}
            <li class="need" data-need={row.kind} data-desk-entry={row.kind === 'desk' ? row.id : undefined} data-held-act={row.kind === 'act' ? row.id : undefined} data-review={row.kind === 'review' ? row.id : undefined}>
              {#if row.kind === 'desk'}
                <span class="need-mark" data-product="desk" aria-hidden="true"><Icon name="inbox" size="sm" /></span>
                <a class="subject" href={`/items/${row.id}`}>{row.entry.title}</a>
                <span class="from">{#key row.id}<SlaRemaining itemId={row.id} now={deskNow} />{/key}</span>
                <span class="acts"><Button size="sm" tone="subtle" onclick={() => onnavigate?.(`/items/${row.id}`)}>{t('app.today.act_reply')}</Button></span>
              {:else if row.kind === 'approval'}
                {@const approval = row.approval}
                <span class="need-mark" data-product="approvals" aria-hidden="true"><Icon name="user-check" size="sm" /></span>
                <a class="subject" href={`/items/${approval.item_id}`}>{`${approval.item_number ? `#${approval.item_number} ` : ''}${approval.item_title ?? ''}`}</a>
                <span class="from">{[approval.step_name, formatRelative(approval.requested_at, messages.locale)].filter(Boolean).join(' · ')}</span>
                <span class="acts">
                  {#if approval.may_decide}
                    <Button size="sm" tone="subtle" onclick={() => void approve(approval)}>{t('app.approvals.approve')}</Button>
                  {:else}
                    <Button size="sm" tone="subtle" onclick={() => onnavigate?.(`/items/${approval.item_id}`)}>{t('app.today.card_open')}</Button>
                  {/if}
                </span>
              {:else if row.kind === 'act'}
                {@const act = row.act}
                {@const said = t('app.today.held_act', { agent: act.name || t('app.today.held_act_agent') })}
                <span class="need-mark" data-product="agents" aria-hidden="true"><Icon name="zap" size="sm" /></span>
                {#if act.path}<a class="subject" href={act.path}>{said}</a>{:else}<span class="subject">{said}</span>{/if}
                <span class="from">{formatRelative(act.at, messages.locale)}</span>
                <span class="acts">
                  {#if act.mayApproveHere}
                    <Button size="sm" tone="subtle" onclick={() => void decideAct(act.id, true)}>{t('app.agents.approve_act')}</Button>
                  {:else if act.path}
                    <Button size="sm" tone="subtle" onclick={() => onnavigate?.(act.path ?? '/agents')}>{t('app.today.card_open')}</Button>
                  {/if}
                  <IconButton icon="x" size="sm" label={t('app.today.decline_act', { agent: act.name || t('app.today.held_act_agent') })} onclick={() => void decideAct(act.id, false)} />
                </span>
              {:else}
                {@const review = row.review}
                <span class="need-mark" data-product="pages" aria-hidden="true"><Icon name="file-text" size="sm" /></span>
                <a class="subject" href={`/pages/${review.id}`}>{t('app.today.review_page', { title: review.title })}</a>
                <span class="from">{formatRelative(review.updated_at, messages.locale)}</span>
                <span class="acts"><Button size="sm" tone="subtle" onclick={() => onnavigate?.(`/pages/${review.id}`)}>{t('app.today.act_review')}</Button></span>
              {/if}
            </li>
          {/each}
        </ul>
        {#if needs.rest > 0}
          <p class="more"><a href={deskRows.length > 0 && waitingCount === 0 ? '/desk' : '/approvals'}>{t('app.today.all', { count: needRows.length })}</a></p>
        {/if}
      {/if}
    </Widget>
    {#each mySprints.slice(0, SPRINTS_SHOWN) as mine (mine.sprint.id)}
      {@const detail = sprintDetails[mine.sprint.id]}
      {@const unit = t(`app.sprints.unit_${mine.sprint.unit}`)}
      {@const last = detail?.burndown.at(-1)}
      <div class="sprint" data-band="sprint" data-sprint={mine.sprint.id}>
        <Widget title={mine.sprint.name} tile={{ icon: 'calendar', product: 'plans' }}
          line={[t('app.today.sprint_left', { count: sprintDaysLeft(mine.sprint.end_on, day) }), last ? t('app.sprints.done_of', { done: String(last.scope - last.remaining), scope: String(last.scope), unit }) : undefined].filter(Boolean).join(' · ')}
          href={`/collections/${mine.sprint.collection_id}/sprints`}>
          <span class="from">{t('app.today.sprint_mine', { count: mine.my_entries })}</span>
          {#if detail}<SprintBurndown {detail} />{/if}
        </Widget>
      </div>
    {/each}
  </div>
{/snippet}

<!-- A tile over the full width; absent while it has nothing to say. -->
{#snippet tile(key: TodayTile)}
  {#if key === 'COLLECTIONS' && (lists.length > 0 || projects.length > 0)}
    <!-- A list is one row with its figure, never its entries one by one (check 2); a project
         shows how far it is and the step due next (check 3). -->
    <div class="tile" data-tile={key} data-span={projects.length > 0 ? 'two' : undefined}>
      <Widget title={t('app.today.collections')}>
        <ul class="places">
          {#each lists as list (list.id)}
            <li class="place" data-list={list.id}>
              <a class="place-name" href={`/collections/${list.id}`}><ContainerMark look={list} size="md" /><span class="name" title={list.name}>{list.name}</span></a>
              {#if list.figure}<span class="place-figure"><CollectionFigure figure={list.figure} name={list.name} /></span>{/if}
            </li>
          {/each}
          {#each projects as project (project.id)}
            {@const step = today.stepOf(project.id)}
            <li class="place" data-project={project.id}>
              <a class="place-name" href={`/collections/${project.id}`}><ContainerMark look={project} size="md" /><span class="name" title={project.name}>{project.name}</span></a>
              {#if project.figure}<span class="place-figure"><CollectionFigure figure={project.figure} name={project.name} /></span>{/if}
              {#if step}
                <div class="step"><span class="step-word">{t('app.today.next_step')}</span>{@render entry(step, false)}</div>
              {:else if today.stepStateOf(project.id).status === 'ready'}
                <span class="none step">{t('app.today.step_none')}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </Widget>
    </div>
  {:else if key === 'NEWS' && unreadPosts.length > 0}
    <!-- What was posted for the reader and not read yet (B-5): a dot, the title, who and where. -->
    <div class="tile" data-tile={key}>
      <Widget title={t('app.today.news')} count={String(unreadPosts.length)}>
        <ul class="news">
          {#each unreadPosts.slice(0, 3) as post (post.page_id)}
            <li data-news={post.page_id}>
              <a class="news-title" href={`/posts/${post.page_id}`}><UnreadDot /><span class="name">{post.title}</span></a>
              <span class="from">{[
                t('app.today.news_by', { name: (post.created_by ? accounts.nameOf(post.created_by) : undefined) ?? t('app.people.unnamed') }),
                containers.find(post.hub_id)?.name,
                formatRelative(postMoment(post), messages.locale),
                post.confirm_by && !post.confirmed ? t('app.posts.confirm_asked') : undefined,
              ].filter(Boolean).join(' · ')}</span>
            </li>
          {/each}
        </ul>
      </Widget>
    </div>
  {:else if key === 'INBOX' && hasJumble && waiting.length > 0}
    <!-- What arrived and has not been put anywhere: where it came from, from whom and when, and
         the two decisions at the row. -->
    <div class="tile" data-tile={key}>
      <Widget title={t('app.today.jumble')} count={t(jumble.moreAfter(WAITING) ? 'app.today.jumble_waiting_more' : 'app.today.jumble_waiting', { count: waiting.length })}
        act={{ label: t('app.today.card_open'), onclick: () => onnavigate?.('/jumble') }}>
        <ul class="needs">
          {#each waiting.slice(0, 3) as arrival (arrival.id)}
            <li class="need">
              <span class="need-mark" data-channel={arrival.channel} aria-hidden="true"><Icon name={channelIcon(arrival.channel)} size="sm" /></span>
              <a class="subject" href="/jumble">{subjectOf(arrival)}</a>
              <span class="from">{[arrival.sender, arrival.received_at ? formatRelative(arrival.received_at, messages.locale) : undefined].filter(Boolean).join(' · ')}</span>
              {#if mayWriteJumble}
                <span class="acts">
                  <Button size="sm" tone="subtle" onclick={() => onnavigate?.('/jumble')}>{t('app.today.jumble_sort')}</Button>
                  <IconButton icon="x" size="sm" label={t('app.today.jumble_dismiss', { subject: subjectOf(arrival) })} onclick={() => void dismissArrival(arrival.id)} />
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      </Widget>
    </div>
  {:else if key === 'INBOX' && hasJumble && jumbleAddress && !isNewWorkspace}
    <!-- Nothing waits: the tile says where to send things instead of standing empty. -->
    <div class="tile" data-tile={key} data-jumble-address>
      <Widget title={t('app.today.jumble')}>
        <p class="none">{t('app.today.jumble_none')}</p>
        <IntakeAddress address={jumbleAddress} lead={t('app.mail.jumble_lead')} />
      </Widget>
    </div>
  {:else if key === 'RECENT' && recents.rows.length > 0}
    <!-- What this device opened last (check 5): kept here and sent nowhere. -->
    <div class="tile wide" data-tile={key}>
      <Widget title={t('app.today.recent')}>
        <ul class="recent">
          {#each recents.rows.slice(0, 6) as row (row.kind + row.id)}
            {@const self = row.kind === 'item' ? undefined : containers.find(row.id)}
            {@const place = row.placeId ? containers.find(row.placeId) : undefined}
            <li>
              <a class="chip" href={recentHref(row.kind, row.id)} title={[place?.name, row.at ? formatRelative(new Date(row.at).toISOString(), messages.locale) : undefined].filter(Boolean).join(' · ')}>
                <span class="rmark" aria-hidden="true">
                  {#if self}<ContainerMark look={self} size="sm" />{:else if place}<ContainerMark look={place} size="sm" />{:else}<Icon name="square-check" size="sm" />{/if}
                </span>
                <span class="rtitle">{row.title}</span>
              </a>
            </li>
          {/each}
        </ul>
      </Widget>
    </div>
  {/if}
{/snippet}

{#snippet tileRow()}
  {#if hasTiles}
    <div class="tiles">
      {#each tiles.shown as key (key)}{@render tile(key)}{/each}
      <!-- A workspace in use where nothing is on the reader yet: the way into what exists. -->
      {#if inUse}
        <div class="tile" data-band="in-use">
          <Widget title={t('app.today.in_use')}>
            <ul class="offers">
              {#each containers.hubs.filter((hub) => hubUse[hub.id]).slice(0, 5) as hub (hub.id)}
                <li>
                  <span class="offer-mark" aria-hidden="true"><ContainerMark look={hub} size="md" /></span>
                  <span class="offer-words">
                    <a class="offer-name" href={`/hubs/${hub.id}`}>{hub.name}</a>
                    <span class="offer-what">{t('app.today.in_use_collections', { count: containers.collectionsOf(hub.id).length })}</span>
                  </span>
                </li>
              {/each}
              {#each inUseProducts as product (product.id)}
                <li>
                  <span class="offer-mark" aria-hidden="true"><Icon name={product.icon} size="sm" /></span>
                  <span class="offer-words">
                    <a class="offer-name" href={product.path}>{t(product.code)}</a>
                    <span class="offer-what">{t(`app.modules.hint_${product.module.toLowerCase()}`)}</span>
                  </span>
                </li>
              {/each}
            </ul>
          </Widget>
        </div>
      {/if}
      {#if moreProducts.length > 0}
        <!-- Built as the tile beside it: a mark and words a row, the row itself the way in; the
             one way to hide the tile stands where a tile's act stands, at the head's end. -->
        <div class="tile" data-more>
          <Widget title={t('app.today.more')}>
            <ul class="offers">
              {#each moreProducts as product (product.id)}
                <li>
                  <button type="button" class="offer" onclick={() => tryProduct(product)}>
                    <span class="offer-mark" aria-hidden="true"><Icon name={product.icon} size="sm" /></span>
                    <span class="offer-words">
                      <span class="offer-name">{t(product.code)}</span>
                      <span class="offer-what">{t(`app.modules.hint_${product.module.toLowerCase()}`)}</span>
                    </span>
                    <span class="offer-try">{t('app.today.more_try')}</span>
                  </button>
                </li>
              {/each}
            </ul>
          </Widget>
          <span class="tile-dismiss"><IconButton icon="x" size="sm" label={t('app.today.more_dismiss')} onclick={dismissMore} /></span>
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet overview()}
  {#if hasNeeds}{@render needsYou()}{/if}
  {@render tileRow()}
{/snippet}

<WorkPage>
  {#snippet top()}
    <!-- The day starts with the person (§11.7): the greeting is the heading, the date under it,
         *Sections* at the end where the page has them. -->
    <PageHeader title={greeting} subtitle={dayLine}
      secondary={actor.account && !actor.hasNoAccess && !containers.hasNoHubs ? [{ label: t('app.today.sections'), icon: 'settings', onclick: () => (isArranging = true) }] : []} />
  {/snippet}

  {#if actor.hasNoAccess && actor.portals.length > 0}
    <!-- A requester holding nothing but portals: where they ask, not a notice about access. -->
    <EmptyState kind="unused" title={t('app.workspace.portals_title')} description={t('app.workspace.portals_detail')} icon="inbox">
      {#snippet action()}
        {#each actor.portals as portal (portal.collection_id)}
          <Button tone="primary" icon="inbox" onclick={() => onnavigate?.(`/portal/${portal.collection_id}`)}>{portal.name}</Button>
        {/each}
      {/snippet}
    </EmptyState>
  {:else if actor.hasNoAccess}
    <!-- Nobody gave this person anything yet: said in a sentence (UC-ID-08 check 2), and without
         *Create hub*, which the server refuses to an account holding no role anywhere. -->
    <EmptyState kind="unused" title={t('app.workspace.no_access')} description={t('app.workspace.no_access_detail')} icon="hub" />
  {:else if containers.hasNoHubs && containers.hubsState.status === 'ready'}
    <!-- A workspace with nothing in it has one thing to say and one thing to do. -->
    <EmptyState kind="unused" title={t('app.workspace.no_hubs')} icon="hub">
      {#snippet action()}
        {#if createLabel}
          <Button icon="plus" onclick={() => onnavigate?.(createHubPath())}>{createLabel}</Button>
        {/if}
      {/snippet}
    </EmptyState>
  {:else}
    <div class="today" data-today>
      {#if cards.shown.length > 0}
        <div class="cards">
          <WidgetStrip label={t('app.today.glance')}>
            {#each cards.shown as key (key)}{@render card(key)}{/each}
          </WidgetStrip>
        </div>
      {/if}
      {#if viewport.isCompact}
        <!-- A phone: *your day* or the overview, one at a time (§7). -->
        <Tabs label={t('app.today.switch')} tabs={[{ id: 'day', label: t('app.today.day') }, { id: 'overview', label: t('app.today.overview') }]} bind:selected={phoneView}>
          <div class="phone-panel">
            {#if phoneView === 'day'}{@render yourDay()}{:else}{@render overview()}{/if}
          </div>
        </Tabs>
      {:else}
        <div class="columns" data-alone={hasNeeds ? undefined : ''}>
          {@render yourDay()}
          {#if hasNeeds}{@render needsYou()}{/if}
        </div>
        {@render tileRow()}
      {/if}
    </div>
  {/if}
</WorkPage>

<!-- *Sections* (E-17): show, hide and order the cards and tiles; the account's, on every device. -->
<Dialog title={t('app.today.sections_title')} bind:isOpen={isArranging} size="m" dismissLabel={t('app.today.sections_close')}>
  <p class="none">{t('app.today.sections_lead')}</p>
  {#each sectionGroups as group (group.code)}
    {@const ordered = group.ordered}
    <h3 class="band">{t(group.code)}</h3>
    <ul class="arrange" data-arrange={group.code}>
      {#each ordered as key, at (key)}
        {@const name = t(sectionCode[key])}
        <li data-section={key}>
          <Checkbox label={name} checked={!sectionHidden.includes(key)} onchange={(event) => showSection(key, (event.currentTarget as HTMLInputElement).checked)} />
          <span class="arrange-acts">
            <IconButton icon="arrow-up" size="sm" label={t('app.today.section_up', { name })} disabledReason={at === 0 ? t('app.today.section_first') : undefined} onclick={() => moveSection(group.isCard, key, -1)} />
            <IconButton icon="arrow-down" size="sm" label={t('app.today.section_down', { name })} disabledReason={at === ordered.length - 1 ? t('app.today.section_last') : undefined} onclick={() => moveSection(group.isCard, key, 1)} />
          </span>
        </li>
      {/each}
    </ul>
  {/each}
</Dialog>

<EntryActions bind:this={actions} onopen={(id) => onnavigate?.(`/items/${id}`)} {onnavigate} />

<StarterDialog bind:isChoosing onstarted={(path) => onnavigate?.(path)} />

<style>
  .news { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-150); }
  .news li { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .news-title { display: flex; align-items: center; gap: var(--sp-100); color: var(--text-primary); text-decoration: none; font-weight: var(--fw-semibold); min-inline-size: 0; }
  .news-title .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .news-title:hover .name { text-decoration: underline; }
  .news .from { color: var(--text-subtle); font-size: var(--fs-075); }
  /* The scene `today` (work-surface concept §6.1): the cards' row, two columns that end level, the
     tiles over the full width. One column below `expanded`; on a phone the cards swipe and the
     columns are a switch (§7). */
  .today { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); min-inline-size: 0; }
  .cards { min-inline-size: 0; }

  .columns {
    display: grid;
    gap: var(--sp-200);
    grid-template-columns: minmax(0, 1fr);
    align-items: stretch;
  }
  /* The columns end level: the grid stretches each to the row; *needs you* grows above the sprint. */
  .needs-column { display: flex; flex-direction: column; gap: var(--sp-200); min-inline-size: 0; }
  .needs-column > :global(section:first-child) { flex: 1; }

  .tiles {
    display: grid;
    gap: var(--sp-200);
    grid-template-columns: minmax(0, 1fr);
  }
  .tile { display: flex; min-inline-size: 0; }
  .tile > :global(section) { flex: 1; }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .columns { grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); }
    .columns[data-alone] { grid-template-columns: minmax(0, 1fr); }
    /* Dense, so a tile two columns wide leaves no hole a narrower one after it could fill. */
    .tiles { grid-template-columns: repeat(3, minmax(0, 1fr)); grid-auto-flow: row dense; }
    /* Fewer than three tiles fill the row rather than leaving a column empty: one across it, two
       on the columns' own lines above them. */
    .tiles:has(> :only-child) { grid-template-columns: minmax(0, 1fr); }
    .tiles:has(> :nth-child(2):last-child) { grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); }
    .tiles:has(> :only-child) > .tile, .tiles:has(> :nth-child(2):last-child) > .tile { grid-column: auto; }
    .tile.wide { grid-column: 1 / -1; }
    /* Projects carry their next step as a row of its own: the tile takes two of the three. */
    .tile[data-span='two'] { grid-column: span 2; }
  }

  .phone-panel { display: grid; gap: var(--sp-200); padding-block-start: var(--sp-150); }
  .phone-panel .tiles { grid-template-columns: minmax(0, 1fr); }

  /* A tablet holds three cards to a row, so a card's words are not broken one to a line; the
     strip's own row stands from `expanded`, its swipe below `medium`. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` and `.expanded` (600px, 905px); a media query cannot read a custom property. */
  @media (width >= 600px) and (width < 905px) {
    .cards > :global(section) { grid-auto-flow: row; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  }
  .card { display: flex; min-inline-size: 0; }
  .card > :global(section) { flex: 1; }

  .day { display: flex; flex-direction: column; gap: var(--sp-150); min-inline-size: 0; }

  /* The capture field: looks like a field, opens the one capture (§8). */
  .capture {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--density-control-md-min);
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-subtle);
    font: inherit;
    text-align: start;
    cursor: text;
  }
  .capture:hover { border-color: var(--border-strong); }
  .capture:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .capture-words { flex: 1; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .band-group { display: grid; gap: var(--sp-050); }
  .band {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    text-transform: uppercase;
  }
  .band.late { color: var(--text-danger); }
  .band-count { color: var(--text-subtle); font-weight: var(--fw-regular); }

  .none { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-100); }
  .more { margin: 0; font-size: var(--fs-075); }

  a { color: var(--text-brand); }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-xs); }

  /* A row that needs the person: the product's mark, the subject, one line under it, one act. */
  .needs { display: grid; margin: 0; padding: 0; list-style: none; }
  .need {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    grid-template-areas: 'mark subject acts' 'mark from acts';
    align-items: center;
    gap: 0 var(--sp-100);
    padding-block: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
  .need:first-child { border-block-start: 0; }
  .need-mark {
    grid-area: mark;
    display: inline-grid;
    place-items: center;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-sm);
    background: var(--label-slate-bg);
    color: var(--label-slate-fg);
  }
  .need-mark[data-product='desk'] { background: var(--product-desk-surface); color: var(--product-desk-ink); }
  .need-mark[data-product='agents'] { background: var(--product-agents-surface); color: var(--product-agents-ink); }
  .need-mark[data-product='pages'] { background: var(--product-pages-surface); color: var(--product-pages-ink); }
  .need-mark[data-product='approvals'] { background: var(--label-green-bg); color: var(--label-green-fg); }
  .need-mark[data-channel='EMAIL'] { background: var(--label-blue-bg); color: var(--label-blue-fg); }
  .need-mark[data-channel='WEBHOOK'] { background: var(--label-green-bg); color: var(--label-green-fg); }
  .subject { grid-area: subject; overflow: hidden; color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; text-overflow: ellipsis; white-space: nowrap; }
  .subject:hover { text-decoration: underline; }
  .from { grid-area: from; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .acts { grid-area: acts; display: inline-flex; align-items: center; gap: var(--sp-050); }

  /* A title beside its hub and collection keeps room for a few words more than the row's least. */
  [data-entry-row] :global(.content) { min-inline-size: min(100%, 28ch); }


  /* *Your collections*: a row each, the mark, the name, the figure at the end. */
  .places { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .place { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-100); min-inline-size: 0; }
  .place-name { flex: 1 1 14ch; }
  .place-name { display: flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .place-name:hover .name { text-decoration: underline; }
  .place-figure { color: var(--text-secondary); font-size: var(--fs-075); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .step { flex-basis: 100%; display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .step-word { color: var(--text-secondary); font-size: var(--fs-075); }

  /* What was opened last: chips in a row, wrapping. */
  .recent { display: flex; flex-wrap: wrap; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    max-inline-size: 32ch;
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    color: var(--text-primary);
    font-size: var(--fs-075);
    text-decoration: none;
  }
  .chip:hover { background: var(--bg-surface-hover); }
  .rmark { display: inline-flex; color: var(--text-secondary); }
  .rtitle { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .offers { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .offers li { display: flex; align-items: center; gap: var(--sp-150); min-width: 0; }
  .offer-mark { display: inline-grid; place-items: center; inline-size: var(--sp-400); block-size: var(--sp-400); border-radius: var(--r-md); background: var(--accent-primary-subtle); color: var(--text-brand); flex: none; }
  .offer-words { display: flex; flex-direction: column; flex: 1; min-width: 0; }
  /* An offer is one row to press, as a place is in the tile beside it. */
  .offer {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    inline-size: 100%;
    margin: 0;
    padding: var(--sp-050);
    border: 0;
    border-radius: var(--r-md);
    background: none;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  .offer:hover { background: var(--bg-surface-hover); }
  .offer:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .offer-try { flex: none; color: var(--text-brand); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  /* The tile's one way out at its head's end, where another tile's act stands. */
  .tile[data-more] { position: relative; }
  .tile-dismiss { position: absolute; inset-block-start: var(--sp-150); inset-inline-end: var(--sp-150); }
  .offer-name { font-weight: var(--fw-medium); }
  .offer-what { color: var(--text-secondary); font-size: var(--fs-075); }

  /* The first steps of a new workspace: a checklist, each step's act at its end. */
  .steps { display: grid; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .steps li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-block-size: var(--density-control-sm-min); }
  .step-mark {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    border: var(--bw-thick) solid var(--border-strong);
    border-radius: var(--r-sm);
  }
  .steps li[data-done] .step-mark { border-color: var(--accent-primary); background: var(--accent-primary); color: var(--text-inverse); }
  .steps li[data-done] .step-word { color: var(--text-subtle); text-decoration: line-through; }
  .steps .step-word { flex: 1; min-width: 0; color: inherit; font-size: inherit; }

  /* *Sections*: a row each, its switch and its two moves. */
  .arrange { display: grid; gap: var(--sp-050); margin: 0 0 var(--sp-150); padding: 0; list-style: none; }
  .arrange li { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .arrange-acts { display: inline-flex; gap: var(--sp-025); }
</style>
