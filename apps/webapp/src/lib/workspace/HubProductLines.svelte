<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's overview shows its tools as tiles (work-surface concept §6.1, E-6, scene `hub`): each
  // with one figure, a line and a small picture - the week's bars for time, the goal's progress.
  // What each says (user-experience.md §7.9, the navigation concept §2.4): the plan's running sprint or next release, the desks' open and breaching
  // requests, the inventory, the reader's own time this week, the goals pursued, and the pages and
  // boards changed this week, each with the way to its tool's page or *All contents*. Each
  // line is what the product's own read answered; a line not answered yet is not drawn, rather
  // than a guess (P-11). The boards' list is drawn only where it came whole.

  import { untrack } from 'svelte';

  import { StateLozenge, Widget, type IconName } from '@hubtask/design-system/components';

  import { engine } from '../data/engine.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { healthPath } from '../data/cmdb.ts';
  import { goalsPath } from '../data/goals.svelte.ts';
  import { addDays, minutesText } from '../data/time.ts';
  import { goalState } from '../goals/tone.ts';
  import { boardsPath } from '../data/whiteboards.svelte.ts';
  import { queueQuery } from '../servicedesk/queues.ts';
  import { pagesPath } from '../data/pages.svelte.ts';
  import { releasesPath } from '../data/releases.svelte.ts';
  import { sprintsPath } from '../data/sprints.svelte.ts';
  import { weekOf } from '../data/due.ts';
  import { firstWeekdayOf } from '../i18n/week.ts';
  import { dateIn, todayIn } from '../i18n/zone.ts';
  import { formatShortDay, formatWeekdayShort } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { sprintDaysLeft } from '../today.ts';
  import { boardsChangedSince, goalsStanding, nextReleaseOf, pagesChangedSince, sprintsRunning } from '../product/home.ts';

  import type { CmdbHealth, GoalList, HubQueueList, ItemQueryResult, PageList, ReleaseList, SprintList, TimeReport, WhiteboardList } from '@hubtask/sync-engine';

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
  let deskIds = $state<string[]>([]);
  follow<HubQueueList>(() => (desking ? `/hubs/${hubId}/queues` : undefined), (data) => (deskIds = [...(data.desks ?? [])]));
  const deskKey = $derived(deskIds.join(','));
  let deskOpen = $state<number | undefined>(undefined);
  let deskBreaching = $state<number | undefined>(undefined);
  follow<ItemQueryResult>(() => (deskKey ? '/items:query' : undefined), (data) => (deskOpen = data.total ?? undefined), () => queueQuery('open', hubId, deskKey.split(','), 1));
  follow<ItemQueryResult>(() => (deskKey ? '/items:query' : undefined), (data) => (deskBreaching = data.total ?? undefined), () => queueQuery('breaching', hubId, deskKey.split(','), 1));

  // The reader's own time here this week: anybody else's is the approver's report, not a line.
  let minutes = $state<number | undefined>(undefined);
  let perDay = $state<Record<string, number>>({});
  follow<TimeReport>(() => (timing ? '/time-records:report' : undefined), (data) => {
    minutes = data.minutes;
    perDay = Object.fromEntries(data.groups.map((group) => [group.keys[0]?.value ?? '', group.minutes]));
  }, () => ({ from: weekFrom, to: today, mine: true, hub_id: hubId, group_by: ['DAY'] }));
  const weekDays = $derived(Array.from({ length: 7 }, (_, i) => addDays(weekFrom, i)));

  let goals = $state<GoalList['data'] | undefined>(undefined);
  follow<GoalList>(() => (aiming ? goalsPath(undefined, hubId) : undefined), (data) => (goals = data.data));

  interface Tile {
    readonly id: string; readonly icon: IconName; readonly color: string; readonly product: string; readonly href: string;
    /** The one figure and the word after it: *6 Tage* *bis Sprintende*. */
    readonly figure: string; readonly unit: string;
    readonly lines?: readonly string[];
    readonly picture?: 'week' | 'progress';
    /** The goal the progress bar is of, with its state's word (never colour alone). */
    readonly goal?: { readonly title: string; readonly progress: number; readonly status: string };
  }
  const tiles = $derived.by<Tile[]>(() => {
    const said: Tile[] = [];
    if (planning && planRead) {
      const sprint = sprintsRunning(sprintsAt, collectionIds, 1)[0];
      const next = nextReleaseOf({ [hubId]: releases ?? [] }, hubId);
      const release = next ? t('app.hub_overview.release_on', { name: next.name, day: formatShortDay(next.release_on, messages.locale) }) : undefined;
      said.push({
        id: 'plan', icon: 'calendar', color: 'orange', product: t('app.nav.tool_plan'), href: `/hubs/${hubId}/plan`,
        figure: sprint ? t('app.hub_overview.days', { count: String(sprintDaysLeft(sprint.end_on, today)) }) : '–',
        unit: sprint ? t('app.hub_overview.to_sprint_end') : t('app.hub_overview.no_sprint'),
        lines: [...(sprint ? [sprint.name] : []), ...(release ? [release] : []), ...(!sprint && !release ? [t('app.hub_overview.plan_quiet')] : [])],
      });
    }
    if (deskIds.length > 0 && deskOpen !== undefined && deskBreaching !== undefined) {
      said.push({
        id: 'desk', icon: 'inbox', color: 'magenta', product: t('app.nav.tool_queues'), href: `/hubs/${hubId}/desk`,
        figure: String(deskOpen), unit: t('app.hub_overview.unit_open'),
        lines: deskBreaching > 0 ? [t('app.desk_home.breaching', { count: String(deskBreaching) })] : [],
      });
    }
    if (timing && minutes !== undefined) {
      said.push({ id: 'time', icon: 'clock', color: 'lime', product: t('app.nav.tool_time'), href: `/hubs/${hubId}/time`, figure: minutesText(minutes), unit: t('app.hub_overview.this_week'), picture: 'week' });
    }
    if (aiming && goals !== undefined) {
      const standing = goalsStanding(goals);
      const worst = goals.find((goal) => goal.status === 'OFF_TRACK') ?? goals.find((goal) => goal.status === 'AT_RISK');
      said.push({
        id: 'goals', icon: 'flag', color: 'green', product: t('app.nav.tool_goals'), href: `/hubs/${hubId}/goals`,
        figure: String(standing.atRisk > 0 ? standing.atRisk : standing.open),
        unit: standing.atRisk > 0 ? t('app.hub_overview.unit_at_risk') : t('app.hub_overview.unit_goals_open'),
        ...(worst ? { picture: 'progress' as const, goal: { title: worst.title, progress: worst.progress ?? 0, status: worst.status } } : {}),
      });
    }
    if (assets && health !== undefined) {
      said.push({
        id: 'assets', icon: 'shield', color: 'slate', product: t('app.nav.tool_inventory'), href: `/hubs/${hubId}/inventory`,
        figure: String(health.total), unit: t('app.hub_overview.unit_items'),
        lines: health.total === 0 ? [t('app.hub_overview.assets_none')] : health.problems.length > 0 ? [t('app.assets.figure_wanting', { count: health.problems.length })] : [],
      });
    }
    // Pages and boards are one tree, so one tile: what changed in it this week, and *All contents*.
    const pagesRead = !knowledge || pages !== undefined;
    const boardsRead = !boarding || boards !== undefined;
    if ((knowledge || boarding) && pagesRead && boardsRead) {
      const changedPages = knowledge && pages ? pagesChangedSince(pages, weekFrom, (iso) => dateIn(iso, actor.zone)) : 0;
      const changedBoards = boarding && boards ? boardsChangedSince(boards, weekFrom, (iso) => dateIn(iso, actor.zone)) : 0;
      said.push({
        id: 'contents', icon: 'book-open', color: 'teal', product: t('app.nav.all_contents'), href: `/hubs/${hubId}/contents`,
        figure: String(changedPages + changedBoards), unit: t('app.hub_overview.unit_changed'),
        lines: [t('app.hub_overview.contents_this_week', { pages: changedPages, boards: changedBoards })],
      });
    }
    return said;
  });
  const dayPeak = $derived(Math.max(1, ...weekDays.map((day) => perDay[day] ?? 0)));
</script>

{#if tiles.length > 0}
  <ul class="tiles" aria-label={t('app.hub_overview.products')}>
    {#each tiles as tile (tile.id)}
      <li data-product-line={tile.id}>
        <!-- Every tool's tile is Today's widget (WB-5): one figure size, the head, the way to the tool. -->
        <Widget title={tile.product} tile={{ icon: tile.icon, color: tile.color }} figure={tile.figure} unit={tile.unit} href={tile.href}>
          {#each tile.lines ?? [] as line, index (index)}<span class="line">{line}</span>{/each}
          {#if tile.picture === 'week'}
            <span class="week" aria-hidden="true">
              {#each weekDays as day (day)}
                <span class="day"><span class="bar" data-today={day === today ? '' : undefined} style:block-size={`${Math.max(8, Math.round(((perDay[day] ?? 0) / dayPeak) * 100))}%`}></span><span class="weekday">{formatWeekdayShort(day, messages.locale)}</span></span>
              {/each}
            </span>
          {:else if tile.picture === 'progress' && tile.goal}
            <span class="goal"><span class="goal-title">{tile.goal.title}</span><StateLozenge vocabulary="goal" state={goalState(tile.goal.status)} label={t(`app.goals.status_${tile.goal.status}`)} /></span>
            <span class="progress" aria-hidden="true"><span style:inline-size={`${Math.round(Math.min(1, Math.max(0, tile.goal.progress > 1 ? tile.goal.progress / 100 : tile.goal.progress)) * 100)}%`}></span></span>
          {/if}
        </Widget>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, calc(var(--sp-500) * 5)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
  li { display: grid; }
  .line { color: var(--text-secondary); font-size: var(--fs-090); }
  .line { overflow-wrap: anywhere; }
  .week { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: var(--sp-050); align-items: end; block-size: var(--sp-500); margin-block-start: auto; }
  .day { display: grid; grid-template-rows: minmax(0, 1fr) auto; gap: var(--sp-025); block-size: 100%; align-items: end; justify-items: stretch; }
  .bar { display: block; border-radius: var(--r-sm); background: var(--border-default); }
  .bar[data-today] { background: var(--text-secondary); }
  .weekday { color: var(--text-subtle); font-size: var(--fs-075); text-align: center; }
  .goal { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); min-inline-size: 0; }
  .goal-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .progress { display: block; block-size: var(--sp-050); border-radius: var(--r-full); background: var(--bg-surface-sunken); overflow: hidden; }
  .progress > span { display: block; block-size: 100%; background: var(--text-secondary); }
</style>
