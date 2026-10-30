<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One portfolio (UC-GOL-03 check 8): its projects as a table - each hub or collection with its
  // dates, health, entries, how many are completed and its progress - and its status updates, the
  // latest first.
  //
  // **Writes** where the reader may write items at the workspace: the projects (added, dated,
  // given a health, taken out), its name and description, a status update whose health becomes
  // the portfolio's, deleting it. A project the reader may not read is not drawn, and the server
  // keeps it when the reader replaces the projects (check 3).

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, IconButton, Inline, Input, PageHeader, ProgressBar, Select, Skeleton, Stack, Table, Textarea,
    type Column,
  } from '@hubtask/design-system/components';
  import type { Portfolio, PortfolioList, PortfolioUpdateList, PortfolioWrite } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { PORTFOLIOS, csvOf, goals, percentOf, percentText, portfolioPath, portfolioWrite, updatesPath } from '../lib/data/goals.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { goalTone } from '../lib/goals/tone.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import { minutesText, moneyText } from '../lib/data/time.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    portfolioId: string;
    onnavigate: (path: string) => void;
  }

  const { portfolioId, onnavigate }: Props = $props();

  let portfolio = $state<Portfolio | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = portfolioPath(portfolioId);
    return untrack(() => engine.subscribe<Portfolio>({ path }, (next) => {
      if (next.status === 'ready') { portfolio = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  let updates = $state<PortfolioUpdateList['data']>([]);
  $effect(() => {
    const path = updatesPath(portfolioId);
    return untrack(() => engine.subscribe<PortfolioUpdateList>({ path }, (next) => { if (next.status === 'ready') updates = next.data.data; }));
  });
  let others = $state<PortfolioList['data']>([]);
  $effect(() => untrack(() => engine.subscribe<PortfolioList>({ path: PORTFOLIOS }, (next) => { if (next.status === 'ready') others = next.data.data; })));
  const parentName = $derived(portfolio?.parent_id ? others.find((each) => each.id === portfolio?.parent_id)?.name : undefined);
  const nested = $derived(others.filter((each) => each.parent_id === portfolioId));

  $effect(() => frame.entitle(portfolio ? portfolio.name : t('app.portfolios.portfolio')));
  $effect(() => accounts.resolve([portfolio?.owner_id, ...updates.map((each) => each.author_id)]));

  const mayWrite = $derived(holdsHere('WRITE_ITEMS') === true);
  const personName = (id: string | null | undefined) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : t('app.goals.nobody'));
  const locale = $derived(messages.locale);

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

  // The budgets are MANAGE_COSTS's (UC-GOL-03): the server sends them only to whoever manages costs.
  const hasBudgets = $derived(portfolio?.items.some((item) => item.budget) === true);
  const columns = $derived<Column[]>([
    { id: 'project', label: t('app.portfolios.col_project') },
    { id: 'start', label: t('app.portfolios.col_start') },
    { id: 'end', label: t('app.portfolios.col_end') },
    { id: 'health', label: t('app.portfolios.col_health') },
    { id: 'entries', label: t('app.portfolios.col_entries'), align: 'end' },
    { id: 'done', label: t('app.portfolios.col_done'), align: 'end' },
    { id: 'progress', label: t('app.goals.col_progress') },
    ...(hasBudgets ? [{ id: 'budget', label: t('app.portfolios.col_budget'), align: 'end' as const }] : []),
    ...(mayWrite ? [{ id: 'actions', label: t('app.goals.actions'), isLabelHidden: true }] : []),
  ]);
  const updateColumns = $derived<Column[]>([
    { id: 'when', label: t('app.goals.checkin_when') },
    { id: 'who', label: t('app.goals.checkin_who') },
    { id: 'health', label: t('app.portfolios.col_health') },
    { id: 'summary', label: t('app.portfolios.summary') },
  ]);
  type Burn = { budget_minutes: number; minutes: number; budget_amount_minor: number | null; amount_minor: number | null; currency: string };
  function burnText(burn: Burn): string {
    const time = t('app.portfolios.burn_time', { used: minutesText(burn.minutes), budget: minutesText(burn.budget_minutes) });
    if (!burn.currency || burn.budget_amount_minor === null || burn.amount_minor === null || burn.budget_amount_minor <= 0) return time;
    return t('app.portfolios.burn_both', { time, used: moneyText(burn.amount_minor, burn.currency, locale), budget: moneyText(burn.budget_amount_minor, burn.currency, locale) });
  }
  const projectPath = (kind: string, id: string) => (kind === 'HUB' ? `/hubs/${id}` : `/collections/${id}`);

  function exportTable() {
    if (!portfolio) return;
    const heads = [t('app.portfolios.col_project'), t('app.portfolios.col_start'), t('app.portfolios.col_end'), t('app.portfolios.col_health'), t('app.portfolios.col_entries'), t('app.portfolios.col_done'), t('app.goals.col_progress')];
    const rows = portfolio.items.map((item) => [item.name, item.start_on ?? '', item.end_on ?? '', item.status, item.entries ?? '', item.done ?? '', percentText(item.progress)]);
    platform.saveFile(new Blob([csvOf(heads, rows)], { type: 'text/csv;charset=utf-8' }), `portfolio-${portfolio.id}.csv`);
    toast.show({ text: t('app.portfolios.exported'), tone: 'success' });
  }

  // --- Writes -----------------------------------------------------------------------------------

  async function replace(fields: Partial<PortfolioWrite>, done: string): Promise<boolean> {
    const at = portfolio;
    if (!at) return false;
    return act(() => goals.updatePortfolio(at, portfolioWrite(at, fields)), done);
  }

  let isChanging = $state(false);
  const draft = $state({ name: '', description: '', parent: '' });
  async function saveChange() {
    const ok = await replace({ name: draft.name.trim(), description: draft.description, parent_id: draft.parent }, t('app.portfolios.saved'));
    if (ok) isChanging = false;
  }

  let isAdding = $state(false);
  const project = $state({ id: '', start: '', end: '', status: 'NONE' as 'NONE' | 'ON_TRACK' | 'AT_RISK' | 'OFF_TRACK', editing: false });
  $effect(() => {
    if (!isAdding) return;
    const ids = containers.hubs.map((hub) => hub.id);
    const opened = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => opened.forEach((close) => close());
  });
  const projectChoices = $derived.by(() => {
    const held = new Set(portfolio?.items.map((item) => item.container_id) ?? []);
    const choices: { value: string; label: string }[] = [];
    for (const hub of containers.hubs) {
      if (!held.has(hub.id)) choices.push({ value: hub.id, label: hub.name });
      for (const collection of containers.collectionsOf(hub.id)) {
        if (!held.has(collection.id)) choices.push({ value: collection.id, label: t('app.portfolios.collection_in', { name: collection.name, hub: hub.name }) });
      }
    }
    return choices;
  });
  function openProject(item?: Portfolio['items'][number]) {
    project.id = item?.container_id ?? '';
    project.start = item?.start_on ?? '';
    project.end = item?.end_on ?? '';
    project.status = item?.status ?? 'NONE';
    project.editing = item !== undefined;
    isAdding = true;
  }
  async function saveProject() {
    const at = portfolio;
    if (!at || project.id === '') return;
    const row = { container_id: project.id, status: project.status, ...(project.start ? { start_on: project.start } : {}), ...(project.end ? { end_on: project.end } : {}) };
    const items = portfolioWrite(at).items ?? [];
    const next = project.editing ? items.map((each) => (each.container_id === project.id ? row : each)) : [...items, row];
    const ok = await replace({ items: next }, t('app.portfolios.saved'));
    if (ok) isAdding = false;
  }
  async function takeOut(containerId: string, name: string) {
    const at = portfolio;
    if (!at) return;
    await replace({ items: (portfolioWrite(at).items ?? []).filter((each) => each.container_id !== containerId) }, t('app.portfolios.taken_out', { name }));
  }

  let isReporting = $state(false);
  const report = $state({ status: 'ON_TRACK' as 'ON_TRACK' | 'AT_RISK' | 'OFF_TRACK', summary: '' });
  async function saveReport() {
    const ok = await act(() => goals.reportStatus(portfolioId, { status: report.status, summary: report.summary.trim() }), t('app.portfolios.reported'));
    if (ok) isReporting = false;
  }

  let isDeleting = $state(false);
  async function remove() {
    const at = portfolio;
    if (!at) return;
    const ok = await act(() => goals.removePortfolio(at), t('app.portfolios.deleted', { name: at.name }));
    if (ok) { isDeleting = false; onnavigate('/goals?tab=portfolios'); }
  }

  const HEALTH = ['NONE', 'ON_TRACK', 'AT_RISK', 'OFF_TRACK'] as const;
</script>

<Stack gap="300">
  <PageHeader
    title={portfolio?.name ?? t('app.portfolios.portfolio')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [{ id: 'portfolios', label: t('app.goals.tab_portfolios'), href: '/goals?tab=portfolios' }, { id: 'portfolio', label: portfolio?.name ?? '' }],
      label: t('app.goals.trail'),
      expandLabel: t('app.goals.expand_trail'),
      onnavigate: () => onnavigate('/goals?tab=portfolios'),
    }}
    secondary={mayWrite ? [
      { label: t('app.portfolios.report'), icon: 'send', onclick: () => { report.status = portfolio?.status && portfolio.status !== 'NONE' ? portfolio.status : 'ON_TRACK'; report.summary = ''; isReporting = true; } },
      { label: t('app.portfolios.add_project'), icon: 'plus', onclick: () => openProject() },
    ] : []}
    menu={{
      label: t('app.goals.more'),
      items: [
        { id: 'export', label: t('app.portfolios.export'), icon: 'file-text' },
        { id: 'roadmap', label: t('app.roadmap.title'), icon: 'calendar' },
        ...(mayWrite ? [
          { id: 'change', label: t('app.portfolios.change'), icon: 'pencil' as const },
          { id: 'delete', label: t('app.portfolios.delete'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true },
        ] : []),
      ],
      onselect: (id: string) => {
        if (id === 'export') exportTable();
        if (id === 'roadmap') onnavigate(`/portfolios/${portfolioId}/roadmap`);
        if (id === 'change' && portfolio) { draft.name = portfolio.name; draft.description = portfolio.description; draft.parent = portfolio.parent_id ?? ''; isChanging = true; }
        if (id === 'delete') isDeleting = true;
      },
    }}
  />

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !portfolio}
    <div aria-busy="true"><Skeleton lines={8} /></div>
  {:else}
    <section class="summary" aria-label={t('app.goals.summary')}>
      <div class="facts">
        {#if portfolio.status !== 'NONE'}<Badge tone={goalTone(portfolio.status)}>{t(`app.goals.status_${portfolio.status}`)}</Badge>{/if}
        <span>{t('app.goals.owned_by', { name: personName(portfolio.owner_id) })}</span>
        {#if parentName && portfolio.parent_id}<span>{t('app.portfolios.inside')} <a href={`/portfolios/${portfolio.parent_id}`}>{parentName}</a></span>{/if}
      </div>
      {#if portfolio.progress !== null}
        <ProgressBar label={t('app.goals.progress_of', { title: portfolio.name })} value={percentOf(portfolio.progress)} valueLabel={t('app.goals.percent', { percent: percentText(portfolio.progress) })} />
      {/if}
      {#if portfolio.description}<p class="description">{portfolio.description}</p>{/if}
      {#if portfolio.budget}
        <p class="quiet">{t('app.portfolios.budget_total', { count: portfolio.budget.projects, burn: burnText(portfolio.budget) })}{#if portfolio.budget.mixed_currencies} {t('app.portfolios.mixed_currencies')}{/if}</p>
      {/if}
      {#if portfolio.latest_update}
        <Banner tone={goalTone(portfolio.latest_update.status) === 'neutral' ? 'info' : (goalTone(portfolio.latest_update.status) as 'success' | 'warning' | 'danger')}
          title={t('app.portfolios.latest', { when: formatDateTime(portfolio.latest_update.created_at, locale), name: personName(portfolio.latest_update.author_id) })}>
          {portfolio.latest_update.summary}
        </Banner>
      {/if}
    </section>

    <section class="block" aria-labelledby="portfolio-projects">
      <h2 id="portfolio-projects" class="heading">{t('app.portfolios.projects', { count: String(portfolio.items.length) })}</h2>
      <div class="scroll">
        <Table label={t('app.portfolios.projects', { count: String(portfolio.items.length) })} isLabelHidden {columns}>
          {#snippet empty()}{t('app.portfolios.no_projects')}{/snippet}
          {#each portfolio.items as item (item.container_id)}
            <tr>
              <th scope="row"><a href={projectPath(item.kind, item.container_id)}>{item.name}</a>{#if item.archived}<span class="note">{t('app.portfolios.archived')}</span>{/if}</th>
              <td>{item.start_on ?? ''}</td>
              <td>{item.end_on ?? ''}</td>
              <td>{#if item.status !== 'NONE'}<Badge size="sm" tone={goalTone(item.status)}>{t(`app.goals.status_${item.status}`)}</Badge>{/if}</td>
              <td class="number">{item.entries ?? ''}</td>
              <td class="number">{item.done ?? ''}</td>
              <td class="progress">{#if item.progress !== null}<ProgressBar size="sm" label={t('app.goals.progress_of', { title: item.name })} value={percentOf(item.progress)} valueLabel={t('app.goals.percent', { percent: percentText(item.progress) })} />{/if}</td>
              {#if hasBudgets}<td class="number" class:over={item.budget?.over_time || item.budget?.over_money}>{item.budget ? burnText(item.budget) : ''}</td>{/if}
              {#if mayWrite}
                <td>
                  <Inline gap="050">
                    <IconButton icon="pencil" size="sm" label={t('app.portfolios.change_project', { name: item.name })} onclick={() => openProject(item)} />
                    <IconButton icon="x" size="sm" label={t('app.portfolios.take_out', { name: item.name })} onclick={() => void takeOut(item.container_id, item.name)} />
                  </Inline>
                </td>
              {/if}
            </tr>
          {/each}
        </Table>
      </div>
    </section>

    {#if nested.length > 0}
      <section class="block" aria-labelledby="portfolio-nested">
        <h2 id="portfolio-nested" class="heading">{t('app.portfolios.nested')}</h2>
        <ul class="plain">{#each nested as each (each.id)}<li><a href={`/portfolios/${each.id}`}>{each.name}</a></li>{/each}</ul>
      </section>
    {/if}

    <section class="block" aria-labelledby="portfolio-updates">
      <h2 id="portfolio-updates" class="heading">{t('app.portfolios.updates')}</h2>
      <div class="scroll">
        <Table label={t('app.portfolios.updates')} isLabelHidden columns={updateColumns}>
          {#snippet empty()}{t('app.portfolios.no_updates')}{/snippet}
          {#each updates as each (each.id)}
            <tr>
              <td>{formatDateTime(each.created_at, locale)}</td>
              <th scope="row">{personName(each.author_id)}</th>
              <td><Badge size="sm" tone={goalTone(each.status)}>{t(`app.goals.status_${each.status}`)}</Badge></td>
              <td class="clip">{each.summary}</td>
            </tr>
          {/each}
        </Table>
      </div>
    </section>
  {/if}
</Stack>

{#if isChanging}
  <Dialog bind:isOpen={isChanging} title={t('app.portfolios.change')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.portfolios.col_name')} bind:value={draft.name} maxlength={200} />
      <Textarea label={t('app.portfolios.description')} bind:value={draft.description} maxlength={10000} />
      <Select label={t('app.portfolios.col_parent')} bind:value={draft.parent}
        options={[{ value: '', label: t('app.portfolios.no_parent') }, ...others.filter((each) => each.id !== portfolioId).map((each) => ({ value: each.id, label: each.name }))]} />
      <Inline justify="end"><Button disabledReason={draft.name.trim() === '' ? t('app.portfolios.name_needed') : undefined} onclick={() => void saveChange()}>{t('app.goals.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isAdding}
  <Dialog bind:isOpen={isAdding} title={project.editing ? t('app.portfolios.change_project_title') : t('app.portfolios.add_project')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      {#if !project.editing}
        <Select label={t('app.portfolios.col_project')} bind:value={project.id} placeholder={t('app.portfolios.pick_project')} options={projectChoices} />
      {/if}
      <Input label={t('app.portfolios.col_start')} type="date" bind:value={project.start} />
      <Input label={t('app.portfolios.col_end')} type="date" bind:value={project.end} />
      <Select label={t('app.portfolios.col_health')} bind:value={project.status} options={HEALTH.map((each) => ({ value: each, label: t(`app.goals.status_${each}`) }))} />
      <Inline justify="end"><Button disabledReason={project.id === '' ? t('app.portfolios.pick_project') : undefined} onclick={() => void saveProject()}>{t('app.goals.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isReporting}
  <Dialog bind:isOpen={isReporting} title={t('app.portfolios.report')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Select label={t('app.portfolios.col_health')} bind:value={report.status} options={HEALTH.filter((each) => each !== 'NONE').map((each) => ({ value: each, label: t(`app.goals.status_${each}`) }))} />
      <Textarea label={t('app.portfolios.summary')} hint={t('app.portfolios.summary_hint')} bind:value={report.summary} maxlength={10000} />
      <Inline justify="end"><Button disabledReason={report.summary.trim() === '' ? t('app.portfolios.summary_needed') : undefined} onclick={() => void saveReport()}>{t('app.portfolios.send_report')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isDeleting && portfolio}
  <Dialog bind:isOpen={isDeleting} title={t('app.portfolios.delete_title', { name: portfolio.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.portfolios.delete_hint')}</p>
      <Inline justify="end"><Button tone="danger" onclick={() => void remove()}>{t('app.portfolios.delete')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .summary, .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .facts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); color: var(--text-secondary); font-size: var(--fs-075); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .description { margin: 0; white-space: pre-wrap; }
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .over { color: var(--text-danger); }
  .progress { min-width: var(--layout-board-column); }
  .clip { max-inline-size: var(--layout-panel-width); white-space: pre-wrap; }
  .plain { margin: 0; padding-inline-start: var(--sp-200); }
</style>
