<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Goals* (M8): a period's goals, the portfolios and the dashboards, as tabs.
  //
  // **Goals** (UC-GOL-01 check 9): a period's goals as an alignment tree - each goal under the goal
  // it serves, with its place, status, confidence and progress; a period defined and closed by
  // whoever holds STRUCTURE at the workspace, a goal set at the workspace or in a hub.
  //
  // **Portfolios** (UC-GOL-03 check 8) and **Dashboards** (UC-GOL-04 check 8): the lists, each row
  // opening its own page.
  //
  // **The server computes every number**: progress is what it answers for this reader (P-05), and
  // a refusal is said where it happened, in its words.

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, Inline, Input, PageHeader, ProgressBar, Select, Skeleton, Stack, Table, Tabs, Textarea,
    type Column,
  } from '@hubtask/design-system/components';
  import type { DashboardList, Goal, GoalList, GoalPeriod, GoalPeriodList, PortfolioList } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    PERIODS, PORTFOLIOS, alignmentTree, dashboardsPath, goals, goalsPath, percentOf, percentText,
  } from '../lib/data/goals.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import SharingPicker, { type Sharing } from '../lib/goals/SharingPicker.svelte';
  import { goalTone } from '../lib/goals/tone.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the goals where it names none or an unknown one. */
    tab?: string;
    /** The period to open (`?period=`); the latest open one without. */
    period?: string;
    /** Only this hub's goals and dashboards (`?hub=`). */
    hub?: string;
    onnavigate: (path: string) => void;
  }

  const { tab, period, hub, onnavigate }: Props = $props();

  const TABS = [
    { id: 'goals', code: 'app.goals.tab_goals' },
    { id: 'portfolios', code: 'app.goals.tab_portfolios' },
    { id: 'dashboards', code: 'app.goals.tab_dashboards' },
  ] as const;
  const current = $derived(TABS.some((each) => each.id === tab) ? (tab as string) : 'goals');
  const hubName = $derived(hub ? containers.find(hub)?.name : undefined);
  function address(next: { tab?: string; period?: string }): string {
    const query = new URLSearchParams();
    const nextTab = next.tab ?? current;
    if (nextTab !== 'goals') query.set('tab', nextTab);
    const nextPeriod = next.period ?? period;
    if (nextPeriod && nextTab === 'goals') query.set('period', nextPeriod);
    if (hub) query.set('hub', hub);
    const text = query.toString();
    return text === '' ? '/goals' : `/goals?${text}`;
  }

  $effect(() => frame.entitle(hubName ? t('app.goals.title_of', { place: hubName }) : t('app.goals.title')));

  let failure = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done?: string): Promise<boolean> {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    }
  }

  /** Subscribes `path` while it is current; each value and failure goes to its setter. */
  function follow<T>(path: () => string | undefined, ready: (data: T | undefined) => void, failed: (message: string | undefined) => void) {
    $effect(() => {
      const at = path();
      ready(undefined);
      failed(undefined);
      if (!at) return;
      return untrack(() => engine.subscribe<T>({ path: at }, (next) => {
        if (next.status === 'ready') { ready(next.data); failed(undefined); }
        if (next.status === 'failed') failed(renderProblem(next.error, messages).message);
      }));
    });
  }

  const mayStructure = $derived(holdsHere('STRUCTURE') === true);
  const placeName = (hubId: string | null) => (hubId ? (containers.find(hubId)?.name ?? t('app.goals.hub_unread')) : t('app.goals.workspace'));
  const personName = (id: string | null) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : '');

  // --- Periods and goals ------------------------------------------------------------------------

  let periods = $state<GoalPeriod[] | undefined>(undefined);
  let periodsRefusal = $state<string | undefined>(undefined);
  follow<GoalPeriodList>(() => PERIODS, (data) => (periods = data ? [...data.data] : undefined), (m) => (periodsRefusal = m));
  /** The period asked for, else the open one that started last, else the last of all. */
  const chosen = $derived.by(() => {
    if (!periods || periods.length === 0) return undefined;
    const asked = periods.find((each) => each.id === period);
    if (asked) return asked;
    const sorted = [...periods].sort((a, b) => b.start_on.localeCompare(a.start_on));
    return sorted.find((each) => each.state === 'OPEN') ?? sorted[0];
  });

  let goalList = $state<Goal[] | undefined>(undefined);
  let goalsRefusal = $state<string | undefined>(undefined);
  follow<GoalList>(
    () => (current === 'goals' && chosen ? goalsPath(chosen.id, hub) : undefined),
    (data) => (goalList = data ? [...data.data] : undefined),
    (m) => (goalsRefusal = m),
  );
  const tree = $derived(goalList ? alignmentTree(goalList) : []);
  $effect(() => accounts.resolve((goalList ?? []).map((goal) => goal.owner_id)));

  const goalColumns = $derived<Column[]>([
    { id: 'goal', label: t('app.goals.col_goal') },
    { id: 'place', label: t('app.goals.col_place') },
    { id: 'owner', label: t('app.goals.col_owner') },
    { id: 'status', label: t('app.goals.col_status') },
    { id: 'confidence', label: t('app.goals.col_confidence'), align: 'end' },
    { id: 'progress', label: t('app.goals.col_progress') },
  ]);

  // The new period
  let isDefining = $state(false);
  const periodDraft = $state({ name: '', start: '', end: '' });
  async function definePeriod() {
    const ok = await act(async () => {
      const made = await goals.definePeriod({ name: periodDraft.name.trim(), start_on: periodDraft.start, end_on: periodDraft.end });
      onnavigate(address({ period: made.id }));
    }, t('app.goals.period_defined'));
    if (ok) isDefining = false;
  }

  // Closing it
  let isClosing = $state(false);
  async function closePeriod() {
    if (!chosen) return;
    const ok = await act(() => goals.closePeriod(chosen), t('app.goals.period_closed', { name: chosen.name }));
    if (ok) isClosing = false;
  }

  // The new goal
  let isSetting = $state(false);
  const goalDraft = $state({ title: '', place: '', parent: '' });
  const placeChoices = $derived([
    ...(holdsHere('WRITE_ITEMS') === true ? [{ value: '', label: t('app.goals.workspace') }] : []),
    ...containers.hubs.map((each) => ({ value: each.id, label: each.name })),
  ]);
  function openSetting() {
    goalDraft.title = '';
    goalDraft.place = hub ?? placeChoices[0]?.value ?? '';
    goalDraft.parent = '';
    isSetting = true;
  }
  async function setGoal() {
    if (!chosen) return;
    const ok = await act(async () => {
      const made = await goals.create({
        period_id: chosen.id,
        title: goalDraft.title.trim(),
        ...(goalDraft.place ? { hub_id: goalDraft.place } : {}),
        ...(goalDraft.parent ? { parent_id: goalDraft.parent } : {}),
      });
      onnavigate(`/goals/${made.id}`);
    }, t('app.goals.goal_set'));
    if (ok) isSetting = false;
  }

  // --- Portfolios -------------------------------------------------------------------------------

  let portfolioList = $state<PortfolioList['data'] | undefined>(undefined);
  let portfoliosRefusal = $state<string | undefined>(undefined);
  follow<PortfolioList>(() => (current === 'portfolios' ? PORTFOLIOS : undefined), (data) => (portfolioList = data?.data), (m) => (portfoliosRefusal = m));
  const portfolioName = (id: string | null) => (id ? (portfolioList?.find((each) => each.id === id)?.name ?? '') : '');
  const portfolioColumns = $derived<Column[]>([
    { id: 'name', label: t('app.portfolios.col_name') },
    { id: 'parent', label: t('app.portfolios.col_parent') },
    { id: 'owner', label: t('app.goals.col_owner') },
    { id: 'status', label: t('app.portfolios.col_health') },
    { id: 'projects', label: t('app.portfolios.col_projects'), align: 'end' },
  ]);
  $effect(() => accounts.resolve((portfolioList ?? []).map((each) => each.owner_id)));

  let isGathering = $state(false);
  const portfolioDraft = $state({ name: '', description: '', parent: '' });
  async function createPortfolio() {
    const ok = await act(async () => {
      const made = await goals.createPortfolio({
        name: portfolioDraft.name.trim(),
        description: portfolioDraft.description,
        ...(portfolioDraft.parent ? { parent_id: portfolioDraft.parent } : {}),
      });
      onnavigate(`/portfolios/${made.id}`);
    }, t('app.portfolios.created'));
    if (ok) isGathering = false;
  }

  // --- Dashboards -------------------------------------------------------------------------------

  let dashboardList = $state<DashboardList['data'] | undefined>(undefined);
  let dashboardsRefusal = $state<string | undefined>(undefined);
  follow<DashboardList>(() => (current === 'dashboards' ? dashboardsPath(hub) : undefined), (data) => (dashboardList = data?.data), (m) => (dashboardsRefusal = m));
  const dashboardColumns = $derived<Column[]>([
    { id: 'name', label: t('app.dashboards.col_name') },
    { id: 'place', label: t('app.goals.col_place') },
    { id: 'sharing', label: t('app.dashboards.col_sharing') },
    { id: 'widgets', label: t('app.dashboards.col_widgets'), align: 'end' },
  ]);

  let isBuilding = $state(false);
  const dashboardDraft = $state({ name: '', place: '', sharing: 'PRIVATE' as Sharing, groupIds: [] as string[] });
  async function createDashboard() {
    const ok = await act(async () => {
      const made = await goals.createDashboard({
        name: dashboardDraft.name.trim(),
        sharing: dashboardDraft.sharing,
        ...(dashboardDraft.sharing === 'GROUPS' ? { group_ids: dashboardDraft.groupIds } : {}),
        ...(dashboardDraft.place ? { hub_id: dashboardDraft.place } : {}),
      });
      onnavigate(`/dashboards/${made.id}`);
    }, t('app.dashboards.created'));
    if (ok) isBuilding = false;
  }

  const secondary = $derived.by(() => {
    if (current === 'goals' && chosen?.state === 'OPEN' && placeChoices.length > 0) return [{ label: t('app.goals.new_goal'), icon: 'plus' as const, onclick: openSetting }] as const;
    if (current === 'portfolios' && holdsHere('WRITE_ITEMS') === true) return [{ label: t('app.portfolios.new'), icon: 'plus' as const, onclick: () => { portfolioDraft.name = ''; portfolioDraft.description = ''; portfolioDraft.parent = ''; isGathering = true; } }] as const;
    if (current === 'dashboards') return [{ label: t('app.dashboards.new'), icon: 'plus' as const, onclick: () => { dashboardDraft.name = ''; dashboardDraft.place = hub ?? ''; dashboardDraft.sharing = 'PRIVATE'; dashboardDraft.groupIds = []; isBuilding = true; } }] as const;
    return [] as const;
  });
</script>

<Stack gap="300">
  <PageHeader title={hubName ? t('app.goals.title_of', { place: hubName }) : t('app.goals.title')} isTitleInBar={viewport.isCompact} {secondary} />

  <Tabs label={t('app.goals.title')} selected={current} onselect={(next) => onnavigate(address({ tab: next }))} tabs={TABS.map((each) => ({ id: each.id, label: t(each.code) }))}>
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if current === 'goals'}
      {#if periodsRefusal}
        <Banner tone="danger">{periodsRefusal}</Banner>
      {:else if !periods}
        <div aria-busy="true"><Skeleton lines={5} /></div>
      {:else if periods.length === 0}
        <p class="quiet">{t('app.goals.no_periods')}</p>
        {#if mayStructure}<Inline><Button icon="plus" onclick={() => (isDefining = true)}>{t('app.goals.new_period')}</Button></Inline>{/if}
      {:else}
        <div class="controls">
          <Select label={t('app.goals.period')} value={chosen?.id ?? ''}
            options={[...periods].sort((a, b) => b.start_on.localeCompare(a.start_on)).map((each) => ({ value: each.id, label: each.state === 'CLOSED' ? t('app.goals.period_closed_label', { name: each.name }) : each.name }))}
            onchange={(event: Event) => onnavigate(address({ period: (event.currentTarget as HTMLSelectElement).value }))} />
          {#if chosen}<span class="quiet">{t('app.goals.period_span', { from: chosen.start_on, to: chosen.end_on })}</span>{/if}
          {#if mayStructure}
            <Button size="sm" tone="secondary" icon="plus" onclick={() => { periodDraft.name = ''; periodDraft.start = ''; periodDraft.end = ''; isDefining = true; }}>{t('app.goals.new_period')}</Button>
            {#if chosen?.state === 'OPEN'}<Button size="sm" tone="subtle" icon="lock" onclick={() => (isClosing = true)}>{t('app.goals.close_period')}</Button>{/if}
          {/if}
        </div>

        {#if goalsRefusal}
          <Banner tone="danger">{goalsRefusal}</Banner>
        {:else if !goalList}
          <div aria-busy="true"><Skeleton lines={6} /></div>
        {:else}
          <div class="scroll">
            <Table label={t('app.goals.tree', { period: chosen?.name ?? '' })} columns={goalColumns}>
              {#snippet empty()}{t('app.goals.empty')}{/snippet}
              {#each tree as { goal, depth } (goal.id)}
                <tr>
                  <th scope="row">
                    <span class="indent" style:--depth={depth}>
                      {#if depth > 0}<span class="branch" aria-hidden="true"></span>{/if}
                      <a href={`/goals/${goal.id}`}>{goal.title}</a>
                    </span>
                    {#if goal.key_results.length > 0}<span class="note">{t('app.goals.key_results_count', { count: String(goal.key_results.length) })}</span>{/if}
                  </th>
                  <td>{placeName(goal.hub_id)}</td>
                  <td>{personName(goal.owner_id)}</td>
                  <td>{#if goal.status !== 'NONE'}<Badge size="sm" tone={goalTone(goal.status)}>{t(`app.goals.status_${goal.status}`)}</Badge>{/if}</td>
                  <td class="number">{goal.confidence === null ? '' : t('app.goals.confidence_of', { value: String(goal.confidence) })}</td>
                  <td class="progress">
                    {#if goal.grade !== null}
                      <span>{t('app.goals.graded', { percent: percentText(goal.grade) })}</span>
                    {:else if goal.progress === null}
                      <span class="quiet">{t('app.goals.no_progress')}</span>
                    {:else}
                      <ProgressBar size="sm" label={t('app.goals.progress_of', { title: goal.title })} value={percentOf(goal.progress)} valueLabel={t('app.goals.percent', { percent: percentText(goal.progress) })} />
                    {/if}
                  </td>
                </tr>
              {/each}
            </Table>
          </div>
        {/if}
      {/if}

    {:else if current === 'portfolios'}
      {#if portfoliosRefusal}
        <Banner tone="danger">{portfoliosRefusal}</Banner>
      {:else if !portfolioList}
        <div aria-busy="true"><Skeleton lines={4} /></div>
      {:else}
        <div class="scroll">
          <Table label={t('app.goals.tab_portfolios')} isLabelHidden columns={portfolioColumns}>
            {#snippet empty()}{t('app.portfolios.empty')}{/snippet}
            {#each portfolioList as each (each.id)}
              <tr>
                <th scope="row"><a href={`/portfolios/${each.id}`}>{each.name}</a></th>
                <td>{portfolioName(each.parent_id)}</td>
                <td>{personName(each.owner_id)}</td>
                <td>{#if each.status !== 'NONE'}<Badge size="sm" tone={goalTone(each.status)}>{t(`app.goals.status_${each.status}`)}</Badge>{/if}</td>
                <td class="number">{each.items.length}</td>
              </tr>
            {/each}
          </Table>
        </div>
      {/if}

    {:else}
      {#if dashboardsRefusal}
        <Banner tone="danger">{dashboardsRefusal}</Banner>
      {:else if !dashboardList}
        <div aria-busy="true"><Skeleton lines={4} /></div>
      {:else}
        <div class="scroll">
          <Table label={t('app.goals.tab_dashboards')} isLabelHidden columns={dashboardColumns}>
            {#snippet empty()}{t('app.dashboards.empty')}{/snippet}
            {#each dashboardList as each (each.id)}
              <tr>
                <th scope="row"><a href={`/dashboards/${each.id}`}>{each.name}</a></th>
                <td>{placeName(each.hub_id)}</td>
                <td>{t(`app.dashboards.sharing_${each.sharing}`)}</td>
                <td class="number">{each.widgets.length}</td>
              </tr>
            {/each}
          </Table>
        </div>
      {/if}
    {/if}
  </Tabs>
</Stack>

{#if isDefining}
  <Dialog bind:isOpen={isDefining} title={t('app.goals.new_period')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.goals.period_name')} hint={t('app.goals.period_name_hint')} bind:value={periodDraft.name} maxlength={200} />
      <Input label={t('app.goals.period_start')} type="date" bind:value={periodDraft.start} />
      <Input label={t('app.goals.period_end')} type="date" bind:value={periodDraft.end} />
      <Inline justify="end">
        <Button disabledReason={periodDraft.name.trim() === '' || periodDraft.start === '' || periodDraft.end === '' ? t('app.goals.period_incomplete') : undefined}
          onclick={() => void definePeriod()}>{t('app.goals.define')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if isClosing && chosen}
  <Dialog bind:isOpen={isClosing} title={t('app.goals.close_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.goals.close_hint')}</p>
      <Inline justify="end"><Button tone="danger" onclick={() => void closePeriod()}>{t('app.goals.close_period')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isSetting && chosen}
  <Dialog bind:isOpen={isSetting} title={t('app.goals.new_goal')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.goals.goal_title')} bind:value={goalDraft.title} maxlength={200} />
      <Select label={t('app.goals.col_place')} bind:value={goalDraft.place} options={placeChoices} />
      <Select label={t('app.goals.parent')} hint={t('app.goals.parent_hint')} bind:value={goalDraft.parent}
        options={[{ value: '', label: t('app.goals.no_parent') }, ...tree.map(({ goal }) => ({ value: goal.id, label: goal.title }))]} />
      <Inline justify="end">
        <Button disabledReason={goalDraft.title.trim() === '' ? t('app.goals.title_needed') : undefined} onclick={() => void setGoal()}>{t('app.goals.set')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if isGathering}
  <Dialog bind:isOpen={isGathering} title={t('app.portfolios.new')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.portfolios.col_name')} bind:value={portfolioDraft.name} maxlength={200} />
      <Textarea label={t('app.portfolios.description')} bind:value={portfolioDraft.description} maxlength={10000} />
      <Select label={t('app.portfolios.col_parent')} bind:value={portfolioDraft.parent}
        options={[{ value: '', label: t('app.portfolios.no_parent') }, ...(portfolioList ?? []).map((each) => ({ value: each.id, label: each.name }))]} />
      <Inline justify="end">
        <Button disabledReason={portfolioDraft.name.trim() === '' ? t('app.portfolios.name_needed') : undefined} onclick={() => void createPortfolio()}>{t('app.portfolios.create')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if isBuilding}
  <Dialog bind:isOpen={isBuilding} title={t('app.dashboards.new')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.dashboards.col_name')} bind:value={dashboardDraft.name} maxlength={200} />
      <Select label={t('app.goals.col_place')} bind:value={dashboardDraft.place}
        options={[{ value: '', label: t('app.goals.workspace') }, ...containers.hubs.map((each) => ({ value: each.id, label: each.name }))]} />
      <SharingPicker bind:sharing={dashboardDraft.sharing} bind:groupIds={dashboardDraft.groupIds} />
      <Inline justify="end">
        <Button disabledReason={dashboardDraft.name.trim() === '' ? t('app.dashboards.name_needed') : dashboardDraft.sharing === 'GROUPS' && dashboardDraft.groupIds.length === 0 ? t('app.dashboards.groups_needed') : undefined} onclick={() => void createDashboard()}>{t('app.dashboards.create')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .progress { min-width: var(--layout-board-column); }
  .indent { display: inline-flex; align-items: center; gap: var(--sp-050); padding-inline-start: calc(var(--depth) * var(--sp-200)); }
  .branch { display: inline-block; inline-size: var(--sp-100); block-size: var(--sp-100); border-inline-start: var(--bw-hairline) solid var(--border-strong); border-block-end: var(--bw-hairline) solid var(--border-strong); }
</style>
