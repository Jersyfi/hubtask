<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *History & versions* as a log page (page-system §7.6, §9.3, configuration/ui.md §1): what waits
  // for a person's word - an agent's proposals (D-16), each a Dialog M with its plan, and the drafts
  // not yet published, each opening where it is published - first, then the configuration's
  // versions by day, each opening its record page with what it changed and its rollback. The
  // expert's parts are the head's: the configuration file as *View as file*, who applies an agent's
  // change and the sandbox in its ⋯ (user-experience.md §8.6).
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE nothing is offered.

  import { untrack } from 'svelte';
  import { Badge, DataTable, EmptyState, FilterBar, LogPage, Radio, Spinner, type DataColumn, type Filter } from '@hubtask/design-system/components';
  import type { ConfigDraftList, ConfigExport, ConfigProposal, ConfigProposalList, ConfigSandbox, ConfigTemplateList, ConfigVersionList, ResourceState } from '@hubtask/sync-engine';

  import { adminTrail } from '../lib/administration/trail.ts';
  import AgentApplyDialog from '../lib/changes/AgentApplyDialog.svelte';
  import FileDialog from '../lib/changes/FileDialog.svelte';
  import ProposalDialog from '../lib/changes/ProposalDialog.svelte';
  import SandboxDialog from '../lib/changes/SandboxDialog.svelte';
  import { countsOf, templateNameOf } from '../lib/changes/words.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { config } from '../lib/data/config.svelte.ts';
  import { draftRows, draftWords, type DraftRow } from '../lib/data/configdrafts.ts';
  import { versionByCode } from '../lib/data/configversion.ts';
  import { workflows } from '../lib/data/workflows.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDay, formatDateTime, formatTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';

  $effect(() => page.entitle(t('app.changes.title')));

  let versions = $state<ResourceState<ConfigVersionList>>();
  let waiting = $state<ResourceState<ConfigProposalList>>();
  let current = $state<ResourceState<ConfigExport>>();
  let catalogue = $state<ResourceState<ConfigTemplateList>>();
  let blockDrafts = $state<ResourceState<ConfigDraftList>>();
  let sandbox = $state<ResourceState<ConfigSandbox>>();
  $effect(() => untrack(() => config.versions((next) => (versions = next))));
  $effect(() => untrack(() => config.waiting((next) => (waiting = next))));
  $effect(() => untrack(() => config.document((next) => (current = next))));
  $effect(() => untrack(() => config.templates((next) => (catalogue = next))));
  $effect(() => untrack(() => config.drafts((next) => (blockDrafts = next))));
  $effect(() => untrack(() => config.sandbox((next) => (sandbox = next))));

  const mayChange = $derived(holdsHere('STRUCTURE') === true);
  const workflowsOn = $derived(manifest.value?.modules_on?.includes('WORKFLOWS') === true);
  $effect(() => {
    if (!workflowsOn) return;
    return untrack(() => workflows.open());
  });
  const drafts = $derived(draftRows(blockDrafts?.status === 'ready' ? blockDrafts.data : undefined, workflowsOn ? (workflows.list?.data ?? []) : []));
  const templateName = (provenance: string) => templateNameOf(catalogue, provenance);

  // What an agent's apply does (D-16): read from the document in force.
  const agentApply = $derived.by(() => {
    if (current?.status !== 'ready') return undefined;
    const settings = (current.data.document as { settings?: Record<string, { value?: unknown }> }).settings;
    const value = settings?.['config.agent_apply']?.value;
    return typeof value === 'string' ? value : undefined;
  });

  type Row =
    | { kind: 'proposal'; id: string; proposal: ConfigProposal }
    | { kind: 'draft'; id: string; draft: DraftRow }
    | { kind: 'version'; id: string; version: ConfigVersionList['data'][number] };

  const waitingRows = $derived<Row[]>([
    ...(waiting?.status === 'ready' ? waiting.data.data.map((proposal) => ({ kind: 'proposal' as const, id: `p-${proposal.id}`, proposal })) : []),
    ...drafts.map((draft) => ({ kind: 'draft' as const, id: `d-${draft.path}`, draft })),
  ]);
  const versionRows = $derived<Row[]>(
    versions?.status === 'ready' ? versions.data.data.map((version) => ({ kind: 'version' as const, id: `v-${version.version}`, version })) : [],
  );
  const nothingWaits = $derived(waiting?.status === 'ready' && waitingRows.length === 0);

  const whatOf = (row: Row): string => {
    if (row.kind === 'proposal') return t('app.changes.proposal', { at: formatDateTime(row.proposal.created_at, messages.locale) });
    if (row.kind === 'draft') {
      const said = draftWords(row.draft);
      return t(said.code, { section: t(said.section), key: said.key });
    }
    return t('app.changes.version', { version: row.version.version });
  };
  const whenOf = (row: Row): string | undefined =>
    row.kind === 'proposal' ? row.proposal.created_at : row.kind === 'draft' ? row.draft.savedAt : row.version.created_at;

  // Narrowing: the search over the words, the kind and the time range as chips.
  let search = $state('');
  let kind = $state('');
  let range = $state('');
  const SPANS: Readonly<Record<string, number>> = { week: 7 * 864e5, month: 30 * 864e5 };
  const rows = $derived(
    [...(kind === 'versions' ? [] : waitingRows), ...(kind === 'waiting' ? [] : versionRows)].filter((row) => {
      if (!matchesSearch(search, [whatOf(row)])) return false;
      const span = SPANS[range];
      const when = whenOf(row);
      return !span || row.kind !== 'version' || (when !== undefined && Date.now() - Date.parse(when) <= span);
    }),
  );
  const continued = new Continued(() => rows);
  $effect(() => {
    void search;
    void kind;
    void range;
    untrack(() => continued.restart());
  });

  const chips = $derived<Filter[]>([
    { id: 'when', label: range === '' ? t('app.changes.range_any') : t(`app.changes.range_${range}`), isActive: range !== '' },
    { id: 'kind', label: t('app.changes.chip_kind', { kind: kind === '' ? t('app.page.any') : t(`app.changes.kind_${kind}`) }), isActive: kind !== '' },
  ]);
  const activeFilters = $derived(chips.filter((chip) => chip.isActive).length);

  const columns = $derived<DataColumn[]>([
    { id: 'what', label: t('app.changes.what_column'), priority: 1 },
    { id: 'when', label: t('app.changes.when_column'), priority: 2 },
    { id: 'by', label: t('app.changes.by_column'), priority: 3 },
    { id: 'counts', label: t('app.changes.counts_column'), priority: 3 },
    { id: 'state', label: t('app.changes.state_column'), priority: 2, isStatus: true },
  ]);

  const refusal = $derived(
    versions?.status === 'failed' ? renderProblem(versions.error, messages) : waiting?.status === 'failed' ? renderProblem(waiting.error, messages) : undefined,
  );

  let opened = $state<ConfigProposal | undefined>(undefined);
  let isFileOpen = $state(false);
  let isAgentApplyOpen = $state(false);
  let isSandboxOpen = $state(false);
  // A sandbox that exists is running work: the head says so where its ⋯ is.
  const sandboxExists = $derived(sandbox?.status === 'ready' && sandbox.data.exists);

  const fileAct = $derived(viewAsFileAction(() => (isFileOpen = true)));
  const head = $derived({
    title: t('app.changes.title'),
    breadcrumb: adminTrail({ id: 'screen', label: t('app.changes.title') }),
    secondary: fileAct ? ([fileAct] as const) : ([] as const),
    menu: {
      label: t('app.page.more_for', { name: t('app.changes.title') }),
      items: [
        ...(agentApply !== undefined ? [{ id: 'agent-apply', label: t('app.changes.agent_apply_open') }] : []),
        { id: 'sandbox', label: sandboxExists ? t('app.changes.sandbox_open_running') : t('app.changes.sandbox_open') },
      ],
      onselect: (id: string) => {
        if (id === 'agent-apply') isAgentApplyOpen = true;
        else isSandboxOpen = true;
      },
    },
  });
</script>

<LogPage {head}>
  {#snippet intro()}
    <p class="quiet">{t('app.changes.intro')}{#if nothingWaits} <span data-waiting>{t('app.changes.none_waiting')}</span>{/if}</p>
  {/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.changes.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(rows.length) })}
      clearLabel={t('app.page.clear')}
      onclear={() => { search = ''; kind = ''; range = ''; }}
      countLabel={versions?.status === 'ready' ? t('app.changes.found', { count: String(rows.length) }) : undefined}
    >
      {#snippet picker(chip: Filter)}
        {#if chip.id === 'when'}
          <Radio label={t('app.changes.when_column')} bind:value={range}
            options={['', 'week', 'month'].map((each) => ({ value: each, label: each === '' ? t('app.changes.range_any') : t(`app.changes.range_${each}`) }))} />
        {:else}
          <Radio label={t('app.changes.kind_label')} bind:value={kind}
            options={['', 'waiting', 'versions'].map((each) => ({ value: each, label: each === '' ? t('app.page.any') : t(`app.changes.kind_${each}`) }))} />
        {/if}
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if versions?.status !== 'ready'}
    <p class="quiet"><Spinner label={t('app.changes.reading')} /> <span>{t('app.changes.reading')}</span></p>
  {:else}
    <DataTable
      label={t('app.changes.history')}
      {columns}
      rows={continued.shown}
      key={(row: Row) => row.id}
      href={(row: Row) => (row.kind === 'version' ? `/administration/changes/${row.version.version}` : row.kind === 'draft' ? row.draft.href : undefined)}
      onopen={(row: Row) => { if (row.kind === 'proposal') opened = row.proposal; }}
      rowLabel={whatOf}
      group={(row: Row) => (row.kind === 'version' ? formatDay(row.version.created_at, messages.locale) : t('app.changes.waiting'))}
      shownLabel={rows.length === 0 ? undefined : continued.shownLabel}
      more={continued.more}
      perPage={continued.perPage}
    >
      {#snippet cell(row: Row, column: DataColumn)}
        {#if column.id === 'what'}
          <span class="name" data-proposal-row={row.kind === 'proposal' ? row.proposal.id : undefined} data-version={row.kind === 'version' ? row.version.version : undefined}>{whatOf(row)}</span>
        {:else if column.id === 'when'}
          <span class="quiet">{whenOf(row) ? (row.kind === 'version' ? formatTime(whenOf(row) ?? '', messages.locale) : formatDateTime(whenOf(row) ?? '', messages.locale)) : t('app.changes.draft_unpublished')}</span>
        {:else if column.id === 'by'}
          {#if row.kind === 'version'}
            <span class="quiet">{t(versionByCode(row.version))}{#if row.version.template} · {t('app.changes.from_template', { name: templateName(row.version.template) })}{/if}</span>
          {:else if row.kind === 'proposal'}
            <span class="quiet">{t('app.changes.by_agent')}</span>
          {/if}
        {:else if column.id === 'counts'}
          {#if row.kind !== 'draft'}<span class="quiet">{countsOf(row.kind === 'version' ? row.version : row.proposal)}</span>{/if}
        {:else if row.kind === 'proposal'}
          <Badge tone={row.proposal.hides_data || row.proposal.changes_permissions ? 'warning' : 'info'}>{row.proposal.hides_data ? t('app.changes.hides_data') : row.proposal.changes_permissions ? t('app.changes.changes_permissions') : t('app.changes.state_waiting')}</Badge>
        {:else if row.kind === 'draft'}
          <Badge tone="neutral">{t('app.changes.state_draft')}</Badge>
        {:else if row.version.rolled_back_to}
          <Badge tone="neutral">{t('app.changes.rollback_of', { version: row.version.rolled_back_to })}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        <EmptyState kind={search || kind || range ? 'filtered' : 'unused'} title={t('app.changes.no_versions')} />
      {/snippet}
    </DataTable>
  {/if}
</LogPage>

<ProposalDialog proposal={opened} {mayChange} {templateName} onClose={() => (opened = undefined)} />
<FileDialog bind:isOpen={isFileOpen} {current} />
<AgentApplyDialog bind:isOpen={isAgentApplyOpen} {agentApply} {mayChange} />
<SandboxDialog bind:isOpen={isSandboxOpen} {sandbox} {mayChange} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
</style>
