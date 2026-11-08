<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's own record (audit.md §6).
  //
  // **Evidence of acts whose per-tenant trail cannot hold them.** After a hard delete the
  // workspace's own audit chain is gone by design, which is the reason this record exists — and
  // why its rows name a workspace by a bare identifier and a slug rather than by a reference.
  //
  // **Identifiers, slugs, counts and moments. Never content.** A journal carrying a title or a URL
  // would be a journal carrying somebody's data into a place nothing ever deletes from, so the
  // details are drawn as the counts and moments they are and this screen has no way to ask for
  // more.
  //
  // The action is a code and is rendered here (ADR-0011): the server holds no display text, and an
  // action this build has no word for is shown as the code rather than as a blank.
  //
  // **Narrowed at the server, a page at a time** (ux-inventory §E8): what happened - a kind of act,
  // as the action's prefix - and the workspace it was about, by the address the journal kept, which
  // is also how a workspace that is gone is found. *Show more* walks on under the same narrowing;
  // narrowing what one page held would hide entries the next page has. A log page (page-system
  // §7.6, §9.4): the kind a chip, the address the search, an entry a Dialog M.
  import { untrack } from 'svelte';

  import { Button, DataTable, EmptyState, FilterBar, LogPage, Radio, Spinner, type DataColumn, type Filter } from '@hubtask/design-system/components';
  import type { InstanceJournalEntry } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance, type JournalFilter } from '../lib/data/instance.svelte.ts';
  import { formatDateTime, formatDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import LogRecordDialog from '../lib/instance/LogRecordDialog.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  /** The kinds of act, each the prefix its actions share; a single installation has no plans. */
  const KINDS = [
    { value: 'tenant.', code: 'app.instance.journal_kind_workspaces' },
    { value: 'plan.', code: 'app.instance.journal_kind_plans' },
    { value: 'instance.', code: 'app.instance.journal_kind_installation' },
  ] as const;
  let kind = $state('');
  let workspace = $state('');
  // The search is the workspace's address, read at the server; it waits for the typing to pause,
  // so a word half typed asks for nothing.
  let search = $state('');
  $effect(() => {
    const typed = search;
    const timer = setTimeout(() => (workspace = typed), SEARCH_PAUSE);
    return () => clearTimeout(timer);
  });
  const filter = $derived<JournalFilter>({ action: kind || undefined, workspace: workspace.trim() || undefined });

  $effect(() => {
    if (!instance.isElevated) return;
    const current = filter;
    return untrack(() => instance.openJournalOf(current));
  });
  const reading = $derived(instance.journalStateOf(filter));
  const entries = $derived(instance.journalOf(filter));
  const isNarrowed = $derived(Boolean(filter.action || filter.workspace));

  /** What the last press of *Show more* brought, said by `LoadMore`'s live region; cleared by a new narrowing. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void filter;
    arrived = undefined;
    moreFailed = undefined;
  });

  async function more(): Promise<void> {
    const current = filter;
    const before = instance.journalOf(current).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await instance.moreJournal(current);
      arrived = t('app.instance.journal_arrived', { count: instance.journalOf(current).length - before });
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = $derived<DataColumn[]>([
    { id: 'action', label: t('app.instance.column_action'), priority: 1 },
    { id: 'occurred', label: t('app.instance.column_when'), priority: 2 },
    ...(manifest.isSingleTenant ? [] : [{ id: 'workspace', label: t('app.instance.column_workspace'), priority: 2 as const }]),
    { id: 'actor', label: t('app.instance.column_actor'), priority: 3 },
  ]);

  const kinds = $derived(KINDS.filter((kind) => !(manifest.isSingleTenant && kind.value === 'plan.')));
  const chips = $derived<Filter[]>([
    { id: 'kind', label: kind ? t(KINDS.find((each) => each.value === kind)?.code ?? 'app.instance.journal_kind_all') : t('app.instance.journal_kind_all'), isActive: kind !== '' },
  ]);

  let opened = $state<InstanceJournalEntry | undefined>(undefined);
  const openedFacts = $derived(
    opened
      ? [
          { label: t('app.instance.column_when'), value: formatDateTime(opened.occurred_at, messages.locale) },
          ...(opened.tenant_slug ? [{ label: t('app.instance.column_workspace'), value: opened.tenant_slug }] : []),
          ...(opened.actor_label ? [{ label: t('app.instance.column_actor'), value: opened.actor_label }] : []),
          ...Object.entries(opened.details ?? {}).map(([key, value]) => ({ label: key, value: shorten(value) })),
        ]
      : [],
  );
  const openedSupport = $derived(
    opened
      ? [
          { label: t('app.instance.journal_support_action'), value: opened.action },
          ...(opened.tenant_id ? [{ label: t('app.instance.journal_support_workspace'), value: opened.tenant_id }] : []),
        ]
      : [],
  );

  /**
   * The word for an action, or the code where this build has none.
   *
   * The catalogue is asked rather than a table here: an installation running ahead of this client
   * records actions it has no word for, and a blank cell would hide an act rather than name it.
   */
  function wordFor(action: string): string {
    const code = `app.instance.action.${action.replaceAll('.', '_')}`;
    const rendered = messages.t(code, {});
    return rendered === code ? action : rendered;
  }

  /**
   * The details, as the one line of counts and moments they are.
   *
   * An identifier is shortened to its first group. The whole of it is in the database for anybody
   * who has to correlate a row; on a screen it is twelve columns of hex that push the sentence
   * beside it off the table, and the journal's value is the sentence.
   */
  function summarise(details: Record<string, unknown> | undefined): string {
    if (!details) return '';
    return Object.entries(details)
      .map(([key, value]) => `${key}: ${shorten(value)}`)
      .join(' · ');
  }

  const SEARCH_PAUSE = 400;
  const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

  function shorten(value: unknown): string {
    const text = Array.isArray(value) ? value.join(', ') : String(value);
    return UUID.test(text) ? `${text.slice(0, 8)}…` : text;
  }

  $effect(() => page.entitle(t('app.instance.journal')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <LogPage head={{ title: t('app.instance.journal'), breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.journal') }) }} refused={door}>
      {#snippet intro()}<p class="quiet">{t(manifest.isSingleTenant ? 'app.instance.journal_intro_single' : 'app.instance.journal_intro')}</p>{/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.journal_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search
          filters={chips}
          moreLabel={t('app.page.more_filters')}
          filtersLabel={t('app.page.filters', { count: String(kind ? 1 : 0) })}
          sheetTitle={t('app.page.filters', { count: '0' })}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          showLabel={t('app.page.show_results', { count: String(entries.length) })}
          clearLabel={t('app.page.clear')}
          onclear={() => { kind = ''; search = ''; }}
          countLabel={reading.status === 'ready' ? t('app.instance.journal_found', { count: String(entries.length) }) : undefined}
        >
          {#snippet picker()}
            <Radio
              label={t('app.instance.journal_kind')}
              bind:value={kind}
              options={[{ value: '', label: t('app.instance.journal_kind_all') }, ...kinds.map((each) => ({ value: each.value, label: t(each.code) }))]}
            />
          {/snippet}
        </FilterBar>
      {/snippet}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if unreadable}
        <FailureNote problem={unreadable} />
      {:else}
        <DataTable
          label={t('app.instance.journal')}
          {columns}
          rows={entries}
          key={(entry: InstanceJournalEntry) => entry.id}
          onopen={(entry: InstanceJournalEntry) => (opened = entry)}
          rowLabel={(entry: InstanceJournalEntry) => wordFor(entry.action)}
          group={(entry: InstanceJournalEntry) => formatDay(entry.occurred_at, messages.locale)}
          shownLabel={entries.length === 0 ? undefined : t('app.page.showing', { count: String(entries.length) })}
          more={instance.journalMoreAfter(filter) || arrived || moreFailed
            ? {
                label: t('app.instance.journal_more'),
                busyLabel: t('app.instance.journal_loading_more'),
                hasMore: Boolean(instance.journalMoreAfter(filter)),
                isBusy: isLoadingMore,
                arrivedLabel: arrived,
                failedLabel: moreFailed,
                onLoadMore: () => void more(),
              }
            : undefined}
        >
          {#snippet cell(entry: InstanceJournalEntry, column: DataColumn)}
            {#if column.id === 'action'}
              <span class="action">{wordFor(entry.action)}</span>
              {#if entry.details}<span class="quiet-cell">{summarise(entry.details)}</span>{/if}
            {:else if column.id === 'occurred'}
              <span class="quiet">{formatDateTime(entry.occurred_at, messages.locale)}</span>
            {:else if column.id === 'workspace'}
              <!-- The slug a person recognises; a workspace that is gone has only its identifier,
                   which is under *Details for support*. -->
              <span>{entry.tenant_slug ?? ''}</span>
            {:else}
              <span class="quiet">{entry.actor_label ?? ''}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if isNarrowed}
              <EmptyState kind="filtered" title={t('app.instance.journal_none_narrowed')}>
                {#snippet action()}<Button onclick={() => { kind = ''; search = ''; }}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {:else}
              <EmptyState kind="unused" title={t('app.instance.journal_empty')} />
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </LogPage>
  {/snippet}
</InstanceGate>

<LogRecordDialog isOpen={opened !== undefined} title={opened ? wordFor(opened.action) : ''} facts={openedFacts} support={openedSupport} onClose={() => (opened = undefined)} />

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { display: block; color: var(--text-secondary); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .action { display: block; font-weight: var(--fw-medium); }
</style>
