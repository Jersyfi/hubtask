<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The screen a controller answers a person's request from (`data-protection.md` §4, §9, §12), as
  // a list page (page-system §7.1, §9.3): the cases first, in one table, narrowed by who, kind and
  // whether closed ones are shown; *Record a request* the head's primary, a Dialog M; *Restrict
  // processing* and *Withdraw a consent* in its ⋯, a Dialog M each; a case opening its record page.
  // *Who keeps their content out of AI* is the strip's second tab.
  //
  // **The deadline is the column that matters**, so the list is ordered by what is closest to it
  // and marked the way the product marks a due date everywhere else: an icon *and* a word beside
  // the date, never colour alone (`design-system.md` §6, rule 3).
  //
  // **`INSTALLATION` scope is not offered** and the alternative is named: it crosses the tenant
  // boundary and is the operator's path.

  import { untrack } from 'svelte';

  import { Badge, Banner, DataTable, EmptyState, FilterBar, ListPage, Radio, Spinner, Stack, Tabs, type DataColumn, type Filter } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { consentRegister } from '../lib/data/consentregister.svelte.ts';
  import { KINDS, byDeadline, deadlinePhrase, privacy, standingOf, type Request } from '../lib/data/privacy.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import RecordRequestDialog from '../lib/privacy/RecordRequestDialog.svelte';
  import RestrictDialog from '../lib/privacy/RestrictDialog.svelte';
  import WithdrawDialog from '../lib/privacy/WithdrawDialog.svelte';
  import { kindWord, statusTone, statusWord, subjectOf, when } from '../lib/privacy/words.svelte.ts';
  import ConsentRegister from '../lib/workspace/ConsentRegister.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  interface Props {
    tab?: string;
    onnavigate: (path: string) => void;
  }

  const { tab, onnavigate }: Props = $props();

  $effect(() => page.entitle(t('app.privacy.title')));

  let includeClosed = $state(false);
  $effect(() => {
    const closed = includeClosed;
    return untrack(() => privacy.open(closed));
  });
  $effect(() => untrack(() => backup.open()));
  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => consentRegister.open()));

  /** The register is a tab only where it is readable and the workspace has others in it (P-10). */
  const hasRegister = $derived(consentRegister.isReadable && workspace.workspace?.several_people === true);
  const current = $derived(tab === 'consent' && hasRegister ? 'consent' : 'requests');

  const mayManage = $derived(holdsHere('MANAGE_MEMBERS') === true);

  /** What the last press of *Show more* brought; cleared when the closed cases are shown or hidden. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void includeClosed;
    arrived = undefined;
    moreFailed = undefined;
  });

  async function more(): Promise<void> {
    const closed = includeClosed;
    const before = privacy.of(closed).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await privacy.more(closed);
      arrived = t('app.privacy.arrived', { count: privacy.of(closed).length - before });
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  const reading = $derived(privacy.stateOf(includeClosed));
  const listed = $derived(byDeadline(privacy.of(includeClosed)));
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  // Read once per render rather than held: a timer that re-rendered every row every minute would
  // burn a battery to move one word.
  const now = $derived.by(() => Date.now());

  $effect(() => {
    const rows = listed;
    untrack(() => accounts.resolve([...rows.map((row) => row.subject_account_id), ...rows.map((row) => row.handled_by)]));
  });

  let search = $state('');
  let kindFilter = $state('');
  const shown = $derived(listed.filter((each) => matchesSearch(search, [subjectOf(each), kindWord(each.kind)]) && (kindFilter === '' || each.kind === kindFilter)));

  const chips = $derived<Filter[]>([
    { id: 'closed', label: includeClosed ? t('app.privacy.chip_with_closed') : t('app.privacy.chip_open'), isActive: includeClosed },
    { id: 'kind', label: t('app.privacy.chip_kind', { kind: kindFilter ? kindWord(kindFilter) : t('app.page.any') }), isActive: kindFilter !== '' },
  ]);
  const activeFilters = $derived(chips.filter((chip) => chip.isActive).length);

  const columns = $derived<DataColumn[]>([
    { id: 'subject', label: t('app.privacy.subject_column'), priority: 1 },
    { id: 'kind', label: t('app.privacy.kind'), priority: 2 },
    { id: 'deadline', label: t('app.privacy.deadline_column'), priority: 2 },
    { id: 'status', label: t('app.privacy.status_column'), priority: 3, isStatus: true },
  ]);

  let isRecording = $state(false);
  let isRestricting = $state(false);
  let isWithdrawing = $state(false);

  const head = $derived({
    title: t('app.privacy.title'),
    breadcrumb: adminTrail({ id: 'screen', label: t('app.privacy.title') }),
    primary: mayManage ? { label: t('app.privacy.record_open'), icon: 'plus' as const, opener: 'record-request', onclick: () => (isRecording = true) } : undefined,
    menu: mayManage
      ? {
          label: t('app.page.more_for', { name: t('app.privacy.title') }),
          items: [
            { id: 'restrict', label: t('app.privacy.restrict_open') },
            { id: 'withdraw', label: t('app.privacy.withdraw_open') },
          ],
          onselect: (id: string) => {
            if (id === 'restrict') isRestricting = true;
            else isWithdrawing = true;
          },
        }
      : undefined,
  });
</script>

<ListPage {head}>
  {#snippet strip()}
    {#if hasRegister}
      <Tabs
        label={t('app.privacy.title')}
        selected={current}
        onselect={(next) => onnavigate(next === 'consent' ? '/administration/privacy?tab=consent' : '/administration/privacy')}
        tabs={[
          { id: 'requests', label: t('app.privacy.cases_title'), count: reading.status === 'ready' ? listed.length : undefined },
          { id: 'consent', label: t('app.privacy.register_tab'), count: consentRegister.entries.length },
        ]}
      />
    {/if}
  {/snippet}
  {#snippet intro()}
    <Stack gap="150">
      <p class="quiet">{t('app.privacy.intro')}</p>
      {#if current === 'requests'}
        <!-- Stated rather than left as an absence: the alternative is somebody's to know. -->
        <Banner tone="info" title={t('app.privacy.installation_title')}>{t('app.privacy.installation_note')}</Banner>
        <!-- A reader without MANAGE_MEMBERS is told once why nothing here can be recorded (P-05). -->
        <WorkspaceGate permission="MANAGE_MEMBERS" />
      {/if}
    </Stack>
  {/snippet}
  {#snippet filters()}
    {#if current === 'requests'}
      <FilterBar
        searchLabel={t('app.privacy.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        filters={chips}
        moreLabel={t('app.page.more_filters')}
        filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
        sheetTitle={t('app.page.filters', { count: '0' })}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        showLabel={t('app.page.show_results', { count: String(shown.length) })}
        clearLabel={t('app.page.clear')}
        onclear={() => { search = ''; kindFilter = ''; includeClosed = false; }}
        countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(listed.length) }) : undefined}
      >
        {#snippet picker(chip: Filter)}
          {#if chip.id === 'closed'}
            <Radio label={t('app.privacy.cases_title')} bind:value={() => (includeClosed ? 'all' : 'open'), (next) => (includeClosed = next === 'all')}
              options={[{ value: 'open', label: t('app.privacy.chip_open') }, { value: 'all', label: t('app.privacy.chip_with_closed') }]} />
          {:else}
            <Radio label={t('app.privacy.kind')} bind:value={kindFilter}
              options={['', ...KINDS].map((each) => ({ value: each, label: each === '' ? t('app.page.any') : kindWord(each) }))} />
          {/if}
        {/snippet}
      </FilterBar>
    {/if}
  {/snippet}

  {#if current === 'consent'}
    <!-- Who keeps their own content out of AI: the register, here and nowhere else. -->
    <ConsentRegister />
  {:else if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.privacy.reading')} /> <span>{t('app.privacy.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={t('app.privacy.cases_title')}
      {columns}
      rows={shown}
      key={(each: Request) => each.id}
      href={(each: Request) => `/administration/privacy/${each.id}`}
      rowLabel={subjectOf}
      shownLabel={listed.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
      more={privacy.moreAfter(includeClosed) || arrived || moreFailed
        ? {
            label: t('app.privacy.more'),
            busyLabel: t('app.privacy.loading_more'),
            hasMore: Boolean(privacy.moreAfter(includeClosed)),
            isBusy: isLoadingMore,
            arrivedLabel: arrived,
            failedLabel: moreFailed,
            onLoadMore: () => void more(),
          }
        : undefined}
    >
      {#snippet cell(request: Request, column: DataColumn)}
        {#if column.id === 'subject'}
          <span class="name" data-case={request.id}>{subjectOf(request)}</span>
        {:else if column.id === 'kind'}
          <span class="quiet">{kindWord(request.kind)}</span>
        {:else if column.id === 'deadline'}
          {@const standing = standingOf(request, now)}
          {@const deadline = deadlinePhrase(request, (at) => when(at) ?? '')}
          <!-- Marked the way the product marks a due date: the icon and the word carry it. -->
          <span class="deadline">
            {#if standing === 'overdue'}
              <Badge tone="danger" icon="triangle-alert">{t('app.privacy.overdue')}</Badge>
            {:else if standing === 'soon'}
              <Badge tone="warning" icon="clock">{t('app.privacy.due_soon')}</Badge>
            {/if}
            <span class="quiet">{t(deadline.code, deadline.params)}</span>
          </span>
        {:else}
          <Badge tone={statusTone(request.status)}>{statusWord(request.status)}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        <EmptyState kind={search || kindFilter ? 'filtered' : 'unused'} title={t('app.privacy.none')} />
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#if mayManage}
  <RecordRequestDialog bind:isOpen={isRecording} />
  <RestrictDialog bind:isOpen={isRestricting} />
  <WithdrawDialog bind:isOpen={isWithdrawing} />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .deadline { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
