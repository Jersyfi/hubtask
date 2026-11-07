<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Goals* (M8, user-experience.md §7.7): a period's goals, the portfolios and the dashboards, as
  // tabs under one head that names the tab it is on. The current quarter is offered in one press
  // where no period is set yet (§4); each goal is a card with its key results as bars and numbers.
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

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Badge, Button, Dialog, Icon, Inline, Input, PageHeader, ViewBar, ProgressBar, Select, Skeleton, Stack, Table, Textarea,
    type Column, type ViewLink,
  } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { DashboardList, Goal, GoalList, GoalPeriod, GoalPeriodList, KeyResult, PortfolioList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    PERIODS, PORTFOLIOS, alignmentTree, dashboardsPath, goals, goalsPath, hubOf, percentOf, percentText,
  } from '../lib/data/goals.svelte.ts';
  import { currentQuarter, needingALook } from '../lib/data/goals.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import SharingPicker, { type Sharing } from '../lib/goals/SharingPicker.svelte';
  import { goalTone } from '../lib/goals/tone.ts';
  import PlaceViews from '../lib/workspace/PlaceViews.svelte';

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
    const text = query.toString();
    // A hub's goals are its tool *Goals* (the navigation concept §2.4): the goals and the dashboards.
    if (hub) {
      query.delete('tab');
      const rest = query.toString();
      return `/hubs/${hub}/goals${nextTab === 'dashboards' ? '/dashboards' : ''}${rest === '' ? '' : `?${rest}`}`;
    }
    return text === '' ? '/goals' : `/goals?${text}`;
  }

  const tabs = $derived<ViewLink[]>(TABS.map((each) => ({ id: each.id, label: t(each.code), href: address({ tab: each.id }) })));
  /** Each tab names itself (§7.7): *Dashboards*, never *Goals of …*. */
  const heading = $derived(
    hub ? t('app.nav.tool_goals')
      : current === 'portfolios' ? t('app.goals.tab_portfolios')
        : current === 'dashboards' ? t('app.goals.tab_dashboards')
          : t('app.goals.title'),
  );
  $effect(() => frame.entitle(hubName ? (current === 'dashboards' ? t('app.dashboards.title_of', { place: hubName }) : t('app.goals.title_of', { place: hubName })) : heading));

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
  const placeName = (place: { hub_id?: string | null }) => {
    const hub = hubOf(place);
    return hub ? (containers.find(hub)?.name ?? t('app.goals.hub_unread')) : t('app.goals.workspace');
  };
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


  const quarter = $derived(currentQuarter(todayIn(actor.zone)));
  const quarterName = $derived(t('app.goals.quarter_name', { year: String(quarter.year), quarter: String(quarter.quarter) }));
  async function startQuarter() {
    await act(async () => {
      const made = await goals.definePeriod({ name: quarterName, start_on: quarter.start_on, end_on: quarter.end_on });
      onnavigate(address({ period: made.id }));
    }, t('app.goals.period_defined'));
  }
  const figureText = $derived(
    current !== 'goals' || !chosen || !goalList
      ? ''
      : [
          t('app.goals.figure_goals', { count: goalList.length }),
          ...(needingALook(goalList) > 0 ? [t('app.goals.figure_look', { count: needingALook(goalList) })] : []),
          chosen.state === 'OPEN' ? t('app.goals.figure_until', { day: formatPlainDay(chosen.end_on, messages.locale) }) : t('app.goals.figure_closed'),
        ].join(' · '),
  );
  const resultValue = (result: KeyResult) =>
    result.kind === 'BINARY' || result.kind === 'MILESTONE' ? (result.progress >= 1 ? t('app.goals.kr_reached') : t('app.goals.kr_not_reached'))
      : result.kind === 'PERCENT' ? t('app.goals.kr_of_target', { value: t('app.goals.percent', { percent: String(result.current) }), target: t('app.goals.percent', { percent: String(result.target) }) })
        : t('app.goals.kr_of_target', { value: result.unit ? t('app.goals.kr_value_unit', { value: String(result.current), unit: result.unit }) : String(result.current), target: String(result.target) });

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

  const primary = $derived.by(() => {
    if (current === 'goals' && chosen?.state === 'OPEN' && placeChoices.length > 0) return { label: t('app.goals.new_goal'), icon: 'plus' as const, onclick: openSetting };
    if (current === 'portfolios' && holdsHere('WRITE_ITEMS') === true) return { label: t('app.portfolios.new'), icon: 'plus' as const, onclick: () => { portfolioDraft.name = ''; portfolioDraft.description = ''; portfolioDraft.parent = ''; isGathering = true; } };
    if (current === 'dashboards') return { label: t('app.dashboards.new'), icon: 'plus' as const, onclick: () => { dashboardDraft.name = ''; dashboardDraft.place = hub ?? ''; dashboardDraft.sharing = 'PRIVATE'; dashboardDraft.groupIds = []; isBuilding = true; } };
    return undefined;
  });
</script>

<Stack gap="300">
  <PageHeader
    title={heading}
    subtitle={hub ? undefined : t('app.modules.hint_goals')}
    isTitleInBar={viewport.isCompact}
    {primary}
    breadcrumb={hub ? (hubName ? {
      trail: [{ id: 'place', label: hubName, href: `/hubs/${hub}` }, { id: 'goals', label: t('app.nav.tool_goals') }],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
      onnavigate: () => onnavigate(`/hubs/${hub}`),
    } : undefined) : {
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: t('app.nav.goals') },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  >
    {#snippet figure()}{#if figureText}<span class="figure" data-goals-figure>{figureText}</span>{/if}{/snippet}
  </PageHeader>

  {#if hub}
    <PlaceViews placeId={hub} tool="goals" {current} {onnavigate} />
  {:else}
    <ViewBar label={t('app.goals.title')} views={tabs} isCompact={viewport.isCompact} closeLabel={t('app.nav.close')} {current} onnavigate={(next) => onnavigate(address({ tab: next }))} />
  {/if}

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if current === 'goals'}
    {#if periodsRefusal}
      <FailureNote problem={periodsRefusal} />
    {:else if !periods}
      <div aria-busy="true"><Skeleton lines={5} /></div>
    {:else if periods.length === 0}
      <ProductEmpty product="GOALS" kind="unused" icon="flag" title={t('app.goals.no_periods_title')} description={t('app.goals.no_periods')}
        create={mayStructure ? { label: t('app.goals.start_quarter', { name: quarterName }), onclick: () => void startQuarter() } : undefined}
        templates={mayStructure ? [{ label: t('app.goals.another_period'), onclick: () => { periodDraft.name = ''; periodDraft.start = ''; periodDraft.end = ''; isDefining = true; } }] : []}
        whoCan={t('app.goals.who_starts_periods')} />
    {:else}
      <div class="controls">
        <Select label={t('app.goals.period')} value={chosen?.id ?? ''}
          options={[...periods].sort((a, b) => b.start_on.localeCompare(a.start_on)).map((each) => ({ value: each.id, label: each.state === 'CLOSED' ? t('app.goals.period_closed_label', { name: each.name }) : each.name }))}
          onchange={(event: Event) => onnavigate(address({ period: (event.currentTarget as HTMLSelectElement).value }))} />
        {#if chosen}<span class="quiet">{t('app.goals.period_span', { from: formatPlainDay(chosen.start_on, messages.locale), to: formatPlainDay(chosen.end_on, messages.locale) })}</span>{/if}
        {#if mayStructure}
          <Button size="sm" tone="secondary" icon="plus" onclick={() => { periodDraft.name = ''; periodDraft.start = ''; periodDraft.end = ''; isDefining = true; }}>{t('app.goals.new_period')}</Button>
          {#if chosen?.state === 'OPEN'}<Button size="sm" tone="subtle" icon="lock" onclick={() => (isClosing = true)}>{t('app.goals.close_period')}</Button>{/if}
        {/if}
      </div>

      {#if goalsRefusal}
        <FailureNote problem={goalsRefusal} />
      {:else if !goalList}
        <div aria-busy="true"><Skeleton lines={6} /></div>
      {:else if tree.length === 0}
        <ProductEmpty product="GOALS" place={{ type: 'TENANT' }} kind={chosen?.state === 'OPEN' ? 'unused' : 'settled'} icon="flag" title={t('app.goals.empty')}
          description={chosen?.state === 'OPEN' ? t('app.goals.empty_body') : undefined} create={primary} />
      {:else}
        <ul class="goals" aria-label={t('app.goals.tree', { period: chosen?.name ?? '' })}>
          {#each tree as { goal, depth } (goal.id)}
            <li class="goal" style:--depth={Math.min(depth, 4)} data-depth={depth > 0 ? '' : undefined}>
              <div class="goal-head">
                <a class="goal-title" href={`/goals/${goal.id}`}>{goal.title}</a>
                {#if goal.status !== 'NONE'}<Badge size="sm" tone={goalTone(goal.status)}>{t(`app.goals.status_${goal.status}`)}</Badge>{/if}
              </div>
              <p class="meta">
                <span>{placeName(goal)}</span>
                {#if goal.owner_id}<span>{t('app.goals.owned_by', { name: personName(goal.owner_id) })}</span>{/if}
                {#if goal.confidence !== null}<span>{t('app.goals.confidence_line', { value: String(goal.confidence) })}</span>{/if}
                {#if goal.child_count > 0}<span>{t('app.goals.children', { count: goal.child_count })}</span>{/if}
              </p>
              <div class="overall">
                {#if goal.grade !== null}
                  <span>{t('app.goals.graded', { percent: percentText(goal.grade) })}</span>
                {:else if goal.progress === null}
                  <span class="quiet">{t('app.goals.no_progress_hint')}</span>
                {:else}
                  <ProgressBar label={t('app.goals.progress_of', { title: goal.title })} value={percentOf(goal.progress)} valueLabel={t('app.goals.percent', { percent: percentText(goal.progress) })} />
                {/if}
              </div>
              {#if goal.key_results.length > 0}
                <ul class="results" aria-label={t('app.goals.key_results_of', { title: goal.title })}>
                  {#each goal.key_results as result (result.id)}
                    <li class="result">
                      <span class="result-title">{result.title}</span>
                      <ProgressBar size="sm" label={t('app.goals.progress_of', { title: result.title })} value={percentOf(result.progress)} valueLabel={resultValue(result)} />
                    </li>
                  {/each}
                </ul>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    {/if}

  {:else if current === 'portfolios'}
    {#if portfoliosRefusal}
      <FailureNote problem={portfoliosRefusal} />
    {:else if !portfolioList}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if portfolioList.length === 0}
      <ProductEmpty product="GOALS" kind="unused" icon="briefcase" title={t('app.portfolios.empty')} description={t('app.portfolios.empty_body')} create={primary}
        whoCan={t('app.portfolios.who_can')} />
    {:else}
      <Table isStacked label={t('app.goals.tab_portfolios')} isLabelHidden columns={portfolioColumns}>
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
    {/if}

  {:else}
    {#if dashboardsRefusal}
      <FailureNote problem={dashboardsRefusal} />
    {:else if !dashboardList}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if dashboardList.length === 0}
      <ProductEmpty product="GOALS" place={{ type: 'TENANT' }} kind="unused" icon="gauge" title={t('app.dashboards.empty')} description={t('app.dashboards.empty_body')} create={primary} />
    {:else}
      <ul class="cards" aria-label={heading}>
        {#each dashboardList as each (each.id)}
          <li>
            <a class="card" href={`/dashboards/${each.id}`}>
              <span class="card-mark" aria-hidden="true"><Icon name="gauge" size="md" /></span>
              <span class="card-name">{each.name}</span>
              <span class="meta">
                <span>{placeName(each)}</span>
                <span>{t(`app.dashboards.sharing_${each.sharing}`)}</span>
                <span>{t('app.dashboards.widget_count', { count: each.widgets.length })}</span>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
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
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .goals, .results, .cards { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .goal {
    display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0;
    margin-inline-start: calc(var(--depth) * var(--sp-300)); padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface);
  }
  .goal[data-depth] { border-inline-start: var(--bw-thick) solid var(--border-strong); }
  .goal-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .goal-title { min-width: 0; color: var(--text-brand); font-size: var(--fs-200); font-weight: var(--fw-semibold); overflow-wrap: anywhere; text-decoration: none; }
  .goal-title:hover { text-decoration: underline; }
  .meta { display: flex; flex-wrap: wrap; gap: var(--sp-050) var(--sp-150); margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .results { gap: var(--sp-100); padding-block-start: var(--sp-100); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .result { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr); align-items: center; gap: var(--sp-150); }
  .result-title { overflow-wrap: anywhere; font-size: var(--fs-075); }
  .cards { grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-board-column)), 1fr)); }
  .card {
    display: flex; flex-direction: column; gap: var(--sp-100); block-size: 100%; padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface);
    color: var(--text-primary); text-decoration: none;
  }
  .card:hover { border-color: var(--border-strong); }
  .card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-ring); }
  .card-mark { color: var(--accent-primary); }
  .card-name { font-weight: var(--fw-semibold); overflow-wrap: anywhere; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .goal { margin-inline-start: calc(var(--depth) * var(--sp-100)); padding: var(--sp-150); }
    .result { grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); }
  }
</style>
