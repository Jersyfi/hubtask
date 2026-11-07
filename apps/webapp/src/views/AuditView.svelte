<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The trail as a log page (page-system §7.6, §9.3, `audit.md` §3-§5): the time range the first
  // chip of the filter bar, who, the kind of thing, the outcome, what was done and the reference
  // the others - on a phone behind *Filters (n)* -, the entries grouped by day in words, each
  // opening its Dialog M (RecordDialog). *Export…* in the head; checking the chain and where it is
  // anchored in the head's ⋯, each a Dialog M. No form is open on arrival (PS-3).
  //
  // **An entry carries no user content, by design (ADR-0017)**, so an actor is an identifier plus
  // the label that was valid at the time. The screen resolves the identifier through the accounts
  // store and falls back to the stored label - which is precisely what survives an erasure.

  import { untrack } from 'svelte';

  import {
    Badge, DataTable, EmptyState, FilterBar, Input, LogPage, Radio, Select, Spinner, Stack,
    type DataColumn, type Filter,
  } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import AnchoringDialog from '../lib/audit/AnchoringDialog.svelte';
  import ChainDialog from '../lib/audit/ChainDialog.svelte';
  import ExportDialog from '../lib/audit/ExportDialog.svelte';
  import RecordDialog from '../lib/audit/RecordDialog.svelte';
  import { TARGET_TYPES, actionWords, thingWord } from '../lib/audit/words.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { audit, type Entry, type Query } from '../lib/data/audit.svelte.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDay, formatTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import PersonPicker from '../lib/people/PersonPicker.svelte';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => page.entitle(t('app.audit.title')));

  const OUTCOMES = ['SUCCESS', 'DENIED', 'FAILED'];
  const words = { t, has: (code: string) => messages.has(code) };

  // The time range: a span back from when it was chosen, or a window typed by hand.
  const SPANS: Readonly<Record<string, number>> = { day: 864e5, week: 7 * 864e5, month: 30 * 864e5 };
  let range = $state('');
  let since = $state('');
  let from = $state('');
  let to = $state('');
  let action = $state('');
  let actorId = $state('');
  let targetType = $state('');
  let targetId = $state('');
  let outcome = $state('');
  let requestId = $state('');

  function chooseRange(next: string): void {
    range = next;
    from = '';
    to = '';
    const span = SPANS[next];
    since = span ? new Date(Date.now() - span).toISOString() : '';
  }

  const asked = $derived<Query>({
    ...(range !== 'custom' && since ? { from: since } : {}),
    ...(range === 'custom' && from ? { from: new Date(from).toISOString() } : {}),
    ...(range === 'custom' && to ? { to: new Date(to).toISOString() } : {}),
    ...(action.trim() ? { action: action.trim() } : {}),
    ...(actorId.trim() ? { actorId: actorId.trim() } : {}),
    ...(targetType ? { targetType } : {}),
    ...(targetId.trim() ? { targetId: targetId.trim() } : {}),
    ...(outcome ? { outcome } : {}),
    ...(requestId.trim() ? { requestId: requestId.trim() } : {}),
  });

  // The query the listing reads follows the filters a moment later, so typing into a filter's field
  // does not send a request per keystroke.
  let applied = $state<Query>({});
  $effect(() => {
    const next = asked;
    const timer = setTimeout(() => {
      if (JSON.stringify(next) !== JSON.stringify(untrack(() => applied))) applied = next;
    }, 350);
    return () => clearTimeout(timer);
  });

  /** What the last press of *Show more* brought, said by `LoadMore`'s live region; cleared by a new query. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void applied;
    arrived = undefined;
    moreFailed = undefined;
  });

  /** The next page, appended, and what arrived said once it has (P-13). */
  async function more(): Promise<void> {
    const query = applied;
    const before = audit.of(query).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await audit.more(query);
      arrived = t('app.audit.arrived', { count: audit.of(query).length - before });
    } catch (error) {
      // Beside the button that asked: the trail already shown is still right.
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  $effect(() => {
    const query = applied;
    return untrack(() => audit.open(query));
  });
  $effect(() => untrack(() => backup.open()));
  // Where the chain's end is anchored, read from the workspace - the one place it is read back from.
  $effect(() => untrack(() => workspace.open()));
  const anchorTargetId = $derived(workspace.workspace?.audit_anchor_target_id ?? null);

  const reading = $derived(audit.stateOf(applied));
  const entries = $derived(audit.of(applied));
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  // The names behind the identifiers, asked for once each.
  $effect(() => {
    const listed = entries;
    untrack(() => accounts.resolve(listed.map((entry) => entry.actor?.id)));
  });

  /**
   * Who acted: the name the accounts store resolved, the label the entry stored, then the sentence
   * true of every actor. An erased account's name is the same "former user" for everybody erased;
   * the entry's label is the stable pseudonym that tells them apart (UC-AUD-01 check 3).
   */
  function actorOf(entry: Entry): string {
    const kind = entry.actor?.type ?? 'SYSTEM';
    if (kind === 'SYSTEM') return t('app.audit.actor_system');
    if (accounts.statusOf(entry.actor?.id) === 'ANONYMIZED' && entry.actor?.label) return entry.actor.label;
    return accounts.nameOf(entry.actor?.id) ?? entry.actor?.label ?? t('app.audit.actor_gone');
  }

  const whatOf = (entry: Entry) => actionWords(entry.action, entry.target?.type, words);
  const aboutOf = (entry: Entry) => entry.target?.label ?? '';

  // The search narrows the entries loaded by their words, who and what about.
  let search = $state('');
  const shown = $derived(entries.filter((entry) => matchesSearch(search, [whatOf(entry), actorOf(entry), aboutOf(entry)])));
  // A page the server answered is shown at the device's rows per page first; the next page is read
  // once those are all on show (PS-8).
  const continued = new Continued(() => shown);
  $effect(() => {
    void search;
    void applied;
    untrack(() => continued.restart());
  });

  const outcomeTone = (entry: Entry) => (entry.outcome === 'SUCCESS' ? 'success' : entry.outcome === 'DENIED' ? 'warning' : 'danger');
  const any = () => t('app.page.any');
  const rangeWords = $derived(range === '' ? t('app.audit.range_any') : t(`app.audit.range_${range}`));
  const chips = $derived<Filter[]>([
    { id: 'when', label: rangeWords, isActive: range !== '' },
    { id: 'who', label: t('app.audit.chip_who', { who: actorId ? (accounts.nameOf(actorId) ?? t('app.audit.chip_set')) : any() }), isActive: actorId !== '' },
    { id: 'thing', label: t('app.audit.chip_thing', { thing: targetType ? thingWord(targetType, '', words) : any() }), isActive: targetType !== '' || targetId.trim() !== '' },
    { id: 'outcome', label: t('app.audit.chip_outcome', { outcome: outcome ? t(`app.audit.outcome_${outcome.toLowerCase()}`) : any() }), isActive: outcome !== '' },
    { id: 'action', label: t('app.audit.chip_action', { action: action.trim() || any() }), isActive: action.trim() !== '' },
    { id: 'reference', label: t('app.audit.chip_reference', { reference: requestId.trim() ? t('app.audit.chip_set') : any() }), isActive: requestId.trim() !== '' },
  ]);
  const activeFilters = $derived(chips.filter((chip) => chip.isActive).length);

  function clearFilters(): void {
    chooseRange('');
    action = '';
    actorId = '';
    targetType = '';
    targetId = '';
    outcome = '';
    requestId = '';
    search = '';
  }

  const columns = $derived<DataColumn[]>([
    { id: 'what', label: t('app.audit.what_column'), priority: 1 },
    { id: 'when', label: t('app.audit.when'), priority: 2 },
    { id: 'who', label: t('app.audit.actor'), priority: 3 },
    { id: 'about', label: t('app.audit.about_column'), priority: 3 },
    { id: 'outcome', label: t('app.audit.outcome'), priority: 2, isStatus: true },
  ]);

  const thingOptions = $derived(
    TARGET_TYPES.map((type) => ({ value: type, label: thingWord(type, '', words) })).sort((a, b) => a.label.localeCompare(b.label, messages.locale)),
  );

  let opened = $state<Entry | undefined>(undefined);
  let isExporting = $state(false);
  let isChecking = $state(false);
  let isAnchoring = $state(false);

  const head = $derived({
    title: t('app.audit.title'),
    breadcrumb: adminTrail({ id: 'screen', label: t('app.audit.title') }),
    secondary: [{ label: t('app.audit.export_open'), opener: 'audit-export', onclick: () => (isExporting = true) }] as const,
    menu: {
      label: t('app.page.more_for', { name: t('app.audit.title') }),
      items: [
        { id: 'verify', label: t('app.audit.verify_open') },
        { id: 'anchoring', label: t('app.audit.anchoring_open') },
      ],
      onselect: (id: string) => {
        if (id === 'verify') isChecking = true;
        else isAnchoring = true;
      },
    },
  });
</script>

<LogPage {head}>
  {#snippet intro()}
    <p class="quiet">{t('app.audit.intro')} {t('app.audit.reading_is_not_recorded')}</p>
  {/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.audit.search')}
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
      onclear={clearFilters}
      countLabel={reading.status === 'ready' ? t('app.audit.found', { count: String(shown.length) }) : undefined}
    >
      {#snippet picker(chip: Filter)}
        {#if chip.id === 'when'}
          <Stack gap="150">
            <Radio
              label={t('app.audit.when')}
              bind:value={() => (range === 'custom' ? '' : range), (next) => chooseRange(next)}
              options={['', 'day', 'week', 'month'].map((each) => ({ value: each, label: each === '' ? t('app.audit.range_any') : t(`app.audit.range_${each}`) }))}
            />
            <Input label={t('app.audit.from')} type="datetime-local" bind:value={from} oninput={() => (range = 'custom')} />
            <Input label={t('app.audit.to')} type="datetime-local" bind:value={to} oninput={() => (range = 'custom')} />
          </Stack>
        {:else if chip.id === 'who'}
          <PersonPicker
            label={t('app.audit.actor')}
            hint={t('app.audit.actor_hint')}
            noneLabel={t('app.person_picker.anyone')}
            known={entries.map((entry) => entry.actor?.id ?? '')}
            bind:value={actorId}
          />
        {:else if chip.id === 'thing'}
          <Stack gap="150">
            <Select label={t('app.audit.target_type')} bind:value={targetType} placeholder={t('app.audit.any_thing')} options={thingOptions} />
            <Input label={t('app.audit.target_id')} bind:value={targetId} />
          </Stack>
        {:else if chip.id === 'outcome'}
          <Select
            label={t('app.audit.outcome')}
            bind:value={outcome}
            placeholder={t('app.audit.any_outcome')}
            options={OUTCOMES.map((value) => ({ value, label: t(`app.audit.outcome_${value.toLowerCase()}`) }))}
          />
        {:else if chip.id === 'action'}
          <Input label={t('app.audit.action')} hint={t('app.audit.action_hint')} bind:value={action} />
        {:else}
          <Input label={t('app.audit.request_id')} hint={t('app.audit.request_id_hint')} bind:value={requestId} />
        {/if}
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet waiting"><Spinner label={t('app.audit.reading')} /> <span>{t('app.audit.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={t('app.audit.title')}
      {columns}
      rows={continued.shown}
      key={(entry: Entry) => entry.id}
      onopen={(entry: Entry) => (opened = entry)}
      rowLabel={whatOf}
      group={(entry: Entry) => formatDay(entry.occurred_at, messages.locale)}
      shownLabel={entries.length === 0 ? undefined : continued.shownLabel}
      perPage={continued.perPage}
      more={continued.more ?? (entries.length > 0
        ? {
            label: t('app.audit.more'),
            busyLabel: t('app.audit.loading_more'),
            hasMore: Boolean(audit.moreAfter(applied)),
            isBusy: isLoadingMore,
            arrivedLabel: arrived,
            failedLabel: moreFailed,
            endLabel: t('app.audit.end'),
            onLoadMore: () => void more(),
          }
        : undefined)}
    >
      {#snippet cell(entry: Entry, column: DataColumn)}
        {#if column.id === 'what'}
          <span class="name" data-entry={entry.id}>{whatOf(entry)}</span>
        {:else if column.id === 'when'}
          <span class="quiet">{formatTime(entry.occurred_at, messages.locale)}</span>
        {:else if column.id === 'who'}
          <span class="quiet">{actorOf(entry)}</span>
        {:else if column.id === 'about'}
          <span class="quiet">{aboutOf(entry)}</span>
        {:else}
          <Badge tone={outcomeTone(entry)}>{t(`app.audit.outcome_${entry.outcome.toLowerCase()}`)}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}<EmptyState kind="filtered" title={t('app.audit.none')} />{/snippet}
    </DataTable>
  {/if}
</LogPage>

<RecordDialog entry={opened} {actorOf} onClose={() => (opened = undefined)} />
<ExportDialog bind:isOpen={isExporting} />
<ChainDialog bind:isOpen={isChecking} {anchorTargetId} />
<AnchoringDialog bind:isOpen={isAnchoring} {anchorTargetId} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .waiting { display: flex; align-items: center; gap: var(--sp-100); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
</style>
