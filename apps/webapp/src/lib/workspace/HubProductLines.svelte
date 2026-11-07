<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's overview says one line per tool on here (user-experience.md §7.9, the navigation
  // concept §2.4): the plan's running sprint or next release, the desks' open and breaching
  // requests, the inventory, the reader's own time this week, the goals pursued, and the pages and
  // boards changed this week, each with the way to its tool's page or *All contents*. Each
  // line is what the product's own read answered; a line not answered yet is not drawn, rather
  // than a guess (P-11). The boards' list is drawn only where it came whole.

  import { untrack } from 'svelte';

  import { Icon, ProductTile, type IconName } from '@hubtask/design-system/components';

  import { engine } from '../data/engine.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { healthPath } from '../data/cmdb.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { goalsPath } from '../data/goals.svelte.ts';
  import { minutesText } from '../data/time.ts';
  import { boardsPath } from '../data/whiteboards.svelte.ts';
  import { desksOf, queueQuery } from '../servicedesk/queues.ts';
  import { pagesPath } from '../data/pages.svelte.ts';
  import { releasesPath } from '../data/releases.svelte.ts';
  import { sprintsPath } from '../data/sprints.svelte.ts';
  import { weekOf } from '../data/due.ts';
  import { firstWeekdayOf } from '../i18n/week.ts';
  import { dateIn, todayIn } from '../i18n/zone.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { sprintDaysLeft } from '../today.ts';
  import { boardsChangedSince, goalsStanding, nextReleaseOf, pagesChangedSince, sprintsRunning } from '../product/home.ts';

  import type { CmdbHealth, GoalList, ItemQueryResult, PageList, ReleaseList, SprintList, TimeReport, WhiteboardList } from '@hubtask/sync-engine';

  interface Props {
    hubId: string;
    /** The modules on at this hub: the workspace's, narrowed by the hub. */
    modulesOn: readonly string[];
    collectionIds: readonly string[];
  }

  const { hubId, modulesOn, collectionIds }: Props = $props();

  const planning = $derived(modulesOn.includes('PLANNING'));
  const knowledge = $derived(modulesOn.includes('KNOWLEDGE'));
  const assets = $derived(modulesOn.includes('ASSETS'));
  const boarding = $derived(modulesOn.includes('WHITEBOARD'));
  const desking = $derived(modulesOn.includes('SERVICE_DESK'));
  const timing = $derived(modulesOn.includes('TIME'));
  const aiming = $derived(modulesOn.includes('GOALS'));

  function follow<T>(path: () => string | undefined, onready: (data: T) => void, body?: () => unknown): void {
    $effect(() => {
      const at = path();
      if (at === undefined) return;
      const sent = body?.();
      return untrack(() => engine.subscribe<T>({ path: at, ...(sent === undefined ? {} : { body: sent }) }, (next) => {
        if (next.status === 'ready') onready(next.data);
      }));
    });
  }

  let pages = $state<PageList['data'] | undefined>(undefined);
  follow<PageList>(() => (knowledge ? pagesPath(hubId) : undefined), (data) => (pages = data.data));
  let health = $state<CmdbHealth | undefined>(undefined);
  follow<CmdbHealth>(() => (assets ? healthPath(hubId) : undefined), (data) => (health = data));
  let releases = $state<ReleaseList['data'] | undefined>(undefined);
  follow<ReleaseList>(() => (planning ? releasesPath(hubId) : undefined), (data) => (releases = data.data));

  let sprintsAt = $state<Record<string, SprintList['data']>>({});
  const sprintKey = $derived(planning ? collectionIds.join(',') : '');
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
  // The plan has answered once every collection's sprints and the releases are read.
  const planRead = $derived(releases !== undefined && collectionIds.every((id) => sprintsAt[id] !== undefined));

  const today = $derived(todayIn(actor.zone));
  const weekFrom = $derived(weekOf(today, firstWeekdayOf(actor.weekStart, messages.locale, manifest.supportedLocales, workspace.weekStartAbove)).from);

  // A list that says it goes on is not counted: its count would be one page's.
  let boards = $state<WhiteboardList['data'] | undefined>(undefined);
  follow<WhiteboardList>(() => (boarding ? boardsPath(hubId) : undefined), (data) => (boards = data.page?.next_cursor ? undefined : data.data));

  // The desks are the hub's collections that take requests; their queues count as the desk's do.
  $effect(() => (desking ? untrack(() => entryTypes.open()) : undefined));
  const typesRead = $derived(entryTypes.state?.status === 'ready');
  const deskIds = $derived(desking && typesRead ? desksOf(containers.collectionsOf(hubId), entryTypes.all) : []);
  const deskKey = $derived(deskIds.join(','));
  let deskOpen = $state<number | undefined>(undefined);
  let deskBreaching = $state<number | undefined>(undefined);
  follow<ItemQueryResult>(() => (deskKey ? '/items:query' : undefined), (data) => (deskOpen = data.total ?? undefined), () => queueQuery('open', hubId, deskKey.split(','), 1));
  follow<ItemQueryResult>(() => (deskKey ? '/items:query' : undefined), (data) => (deskBreaching = data.total ?? undefined), () => queueQuery('breaching', hubId, deskKey.split(','), 1));

  // The reader's own time here this week: anybody else's is the approver's report, not a line.
  let minutes = $state<number | undefined>(undefined);
  follow<TimeReport>(() => (timing ? '/time-records:report' : undefined), (data) => (minutes = data.minutes), () => ({ from: weekFrom, to: today, mine: true, hub_id: hubId }));

  let goals = $state<GoalList['data'] | undefined>(undefined);
  follow<GoalList>(() => (aiming ? goalsPath(undefined, hubId) : undefined), (data) => (goals = data.data));

  interface Line { readonly id: string; readonly icon: IconName; readonly color: string; readonly product: string; readonly text: string; readonly href: string }

  const lines = $derived.by<Line[]>(() => {
    const said: Line[] = [];
    if (planning && planRead) {
      const sprint = sprintsRunning(sprintsAt, collectionIds, 1)[0];
      const next = nextReleaseOf({ [hubId]: releases ?? [] }, hubId);
      const parts = [
        ...(sprint ? [t('app.product_home.sprint_line', { name: sprint.name, remaining: t('app.sprints.days_left', { count: String(sprintDaysLeft(sprint.end_on, today)) }) })] : []),
        ...(next ? [t('app.product_home.next_release', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) })] : []),
      ];
      said.push({
        id: 'plan', icon: 'calendar', color: 'orange', product: t('app.nav.tool_plan'),
        text: parts.length > 0 ? parts.join(' · ') : t('app.hub_overview.plan_quiet'),
        href: `/hubs/${hubId}/plan`,
      });
    }
    if (deskIds.length > 0 && deskOpen !== undefined && deskBreaching !== undefined) {
      said.push({
        id: 'desk', icon: 'inbox', color: 'magenta', product: t('app.nav.tool_queues'),
        text: [
          t('app.desk_home.open', { count: String(deskOpen) }),
          ...(deskBreaching > 0 ? [t('app.desk_home.breaching', { count: String(deskBreaching) })] : []),
        ].join(' · '),
        href: `/hubs/${hubId}/desk`,
      });
    }
    if (assets && health !== undefined) {
      said.push({
        id: 'assets', icon: 'shield', color: 'slate', product: t('app.nav.tool_inventory'),
        text: [
          t('app.assets.figure_total', { count: health.total }),
          ...(health.problems.length > 0 ? [t('app.assets.figure_wanting', { count: health.problems.length })] : []),
        ].join(' · '),
        href: `/hubs/${hubId}/inventory`,
      });
    }
    if (timing && minutes !== undefined) {
      said.push({ id: 'time', icon: 'clock', color: 'lime', product: t('app.nav.tool_time'), text: t('app.hub_overview.time_this_week', { minutes: minutesText(minutes) }), href: `/hubs/${hubId}/time` });
    }
    if (aiming && goals !== undefined) {
      const standing = goalsStanding(goals);
      said.push({
        id: 'goals', icon: 'flag', color: 'green', product: t('app.nav.tool_goals'),
        text: [
          t('app.hub_overview.goals_open', { count: standing.open }),
          ...(standing.atRisk > 0 ? [t('app.hub_overview.goals_at_risk', { count: String(standing.atRisk) })] : []),
        ].join(' · '),
        href: `/hubs/${hubId}/goals`,
      });
    }
    // Pages and boards are one tree, so one line: what changed in it this week, and *All contents*.
    const pagesRead = !knowledge || pages !== undefined;
    const boardsRead = !boarding || boards !== undefined;
    if ((knowledge || boarding) && pagesRead && boardsRead) {
      const changedPages = knowledge && pages ? pagesChangedSince(pages, weekFrom, (iso) => dateIn(iso, actor.zone)) : 0;
      const changedBoards = boarding && boards ? boardsChangedSince(boards, weekFrom, (iso) => dateIn(iso, actor.zone)) : 0;
      said.push({
        id: 'contents', icon: 'book-open', color: 'teal', product: t('app.nav.all_contents'),
        text: t('app.hub_overview.contents_this_week', { pages: changedPages, boards: changedBoards }),
        href: `/hubs/${hubId}/contents`,
      });
    }
    return said;
  });
</script>

{#if lines.length > 0}
  <ul class="lines" aria-label={t('app.hub_overview.products')}>
    {#each lines as line (line.id)}
      <li data-product-line={line.id}>
        <a href={line.href}>
          <ProductTile icon={line.icon} color={line.color} size="sm" />
          <span class="text">{line.text}</span>
          <span class="product">{line.product} <Icon name="chevron-right" size="sm" /></span>
        </a>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .lines {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* A boxed row per tool, as the mockup's overview draws it: the tile, what it says, and the tool
     it leads to at the end. */
  a {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-raised);
    color: var(--text-primary);
    text-decoration: none;
    font-size: var(--fs-075);
  }

  a:hover { border-color: var(--border-default); }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .text { flex: 1 1 auto; min-inline-size: 0; overflow-wrap: anywhere; }

  .product { display: inline-flex; align-items: center; flex: none; color: var(--text-secondary); }
  a:hover .product { color: var(--text-primary); }
</style>
