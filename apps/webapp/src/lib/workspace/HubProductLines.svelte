<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's overview says one line per product on here (user-experience.md §7.9): the plan's
  // running sprint or next release, the pages and boards changed this week, the desks' open and
  // breaching requests, the inventory, the reader's own time this week and the goals pursued. Each
  // line is what the product's own read answered; a line not answered yet is not drawn, rather
  // than a guess (P-11). The boards' list is drawn only where it came whole.

  import { untrack } from 'svelte';

  import { Icon, type IconName } from '@hubtask/design-system/components';

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

  interface Line { readonly id: string; readonly icon: IconName; readonly product: string; readonly text: string; readonly href: string }

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
        id: 'plan', icon: 'calendar', product: t('app.planning.title'),
        text: parts.length > 0 ? parts.join(' · ') : t('app.hub_overview.plan_quiet'),
        href: sprint ? `/collections/${sprint.collection_id}/sprints` : `/hubs/${hubId}/plan`,
      });
    }
    if (knowledge && pages !== undefined) {
      const count = pagesChangedSince(pages, weekFrom, (iso) => dateIn(iso, actor.zone));
      said.push({ id: 'pages', icon: 'book-open', product: t('app.pages.title'), text: t('app.hub_overview.pages_this_week', { count }), href: `/hubs/${hubId}/pages` });
    }
    if (boarding && boards !== undefined) {
      const count = boardsChangedSince(boards, weekFrom, (iso) => dateIn(iso, actor.zone));
      said.push({ id: 'whiteboards', icon: 'layout-template', product: t('app.whiteboards.title'), text: t('app.hub_overview.boards_this_week', { count }), href: `/hubs/${hubId}/whiteboards` });
    }
    if (deskIds.length > 0 && deskOpen !== undefined && deskBreaching !== undefined) {
      said.push({
        id: 'desk', icon: 'inbox', product: t('app.place_tabs.desk'),
        text: [
          t('app.desk_home.open', { count: String(deskOpen) }),
          ...(deskBreaching > 0 ? [t('app.desk_home.breaching', { count: String(deskBreaching) })] : []),
        ].join(' · '),
        href: `/hubs/${hubId}/desk`,
      });
    }
    if (assets && health !== undefined) {
      said.push({
        id: 'assets', icon: 'shield', product: t('app.assets.title'),
        text: [
          t('app.assets.figure_total', { count: health.total }),
          ...(health.problems.length > 0 ? [t('app.assets.figure_wanting', { count: health.problems.length })] : []),
        ].join(' · '),
        href: `/hubs/${hubId}/assets`,
      });
    }
    if (timing && minutes !== undefined) {
      said.push({ id: 'time', icon: 'clock', product: t('app.time.title'), text: t('app.hub_overview.time_this_week', { minutes: minutesText(minutes) }), href: `/hubs/${hubId}/actuals` });
    }
    if (aiming && goals !== undefined) {
      const standing = goalsStanding(goals);
      said.push({
        id: 'goals', icon: 'flag', product: t('app.goals.title'),
        text: [
          t('app.hub_overview.goals_open', { count: standing.open }),
          ...(standing.atRisk > 0 ? [t('app.hub_overview.goals_at_risk', { count: String(standing.atRisk) })] : []),
        ].join(' · '),
        href: `/goals?hub=${hubId}`,
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
          <Icon name={line.icon} size="sm" />
          <span class="product">{line.product}</span>
          <span class="text">{line.text}</span>
        </a>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .lines {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  a {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-050) var(--sp-100);
    padding-block: var(--sp-050);
    color: var(--text-secondary);
    text-decoration: none;
    font-size: var(--fs-075);
  }

  a:hover .text { color: var(--text-primary); }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }

  a :global(svg) { align-self: center; color: var(--text-subtle); }

  .product { color: var(--text-primary); font-weight: var(--fw-medium); }

  .text { min-inline-size: 0; overflow-wrap: anywhere; }
</style>
