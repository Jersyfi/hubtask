<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where this reader is signed in.
  //
  // **A table, and the newest first** (ADR-0065 decision 3). It was a list of rows, each carrying
  // four facts as a sentence — which is readable at three sessions and unreadable at thirty, and
  // thirty is what a year of phones, laptops and browsers looks like. A column per fact is scanned
  // down rather than read across, and the row somebody is looking for is the one that appeared
  // while they were not looking, so it is at the top.
  //
  // **Ending the current session is a sign-out and is treated as one**: the list would otherwise
  // be re-read with a credential that has just stopped working, and the reader would meet a
  // refusal instead of the sign-in screen.

  import { untrack } from 'svelte';

  import { Button, DataTable, EmptyState, FilterBar, ListPage, Skeleton, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';

  import { sorting } from '../lib/sorting.svelte.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { sessions } from '../lib/data/sessions.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { session } from '../lib/session.svelte.ts';

  const account = $derived(actor.account);

  // The sessions are the account's own and take no parameter, so the read starts with the screen
  // rather than with an identifier arriving.
  $effect(() => untrack(() => sessions.open()));

  const held = $derived(sessions.state);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /**
   * What the reader is being asked to end: one session, or every other one. A session ended is
   * gone on the server and nothing brings it back, so ending is a final act and asks first
   * (ux-inventory.md §E0).
   */
  let asking = $state<{ id: string; isCurrent: boolean; client: string } | 'others' | undefined>(undefined);

  // `$derived`, because the headings are words: a language chosen on the screen beside this one
  // changes them, and a list built once would keep the language it was built in.
  const columns = $derived<DataColumn[]>([
    { id: 'client', label: t('app.sessions.client'), isSortable: true, priority: 1 },
    { id: 'created', label: t('app.sessions.created'), isSortable: true, priority: 3 },
    // How it was opened (UC-ID-06 check 2): the way in a person recognises a session by.
    { id: 'method', label: t('app.sessions.method'), isSortable: true, priority: 3 },
    { id: 'used', label: t('app.sessions.last_used'), isSortable: true, priority: 2 },
    { id: 'network', label: t('app.sessions.network'), isSortable: true, priority: 3 },
  ]);

  let search = $state('');

  /**
   * The newest first, and this device first of all.
   *
   * The server answers them in its own order; which one a reader looks for is the one they do not
   * recognise, and that is the one that arrived last.
   *
   * This is the list's own order and not a column, which is why the third press of a heading
   * matters here: it is the only way back to it.
   */
  const ordered = $derived(
    [...sessions.all].sort((a, b) => {
      if (a.current !== b.current) return a.current ? -1 : 1;
      return String(b.created_at).localeCompare(String(a.created_at));
    }),
  );

  // Sorted in the client, because the whole list is already here: `/auth/sessions` answers it in
  // one response and holds no cursor. A year of phones, laptops and browsers is thirty rows, and
  // what makes thirty rows readable is the column somebody can sort by - not a page under them,
  // which this product does not have and will not grow one of.
  const found = $derived(ordered.filter((row) => matchesSearch(search, [row.user_agent ?? undefined, row.ip_class ?? undefined, methodOf(row)])));
  const order = sorting({
    rows: () => found,
    read: (row, columnId) => {
      if (columnId === 'created') return row.created_at;
      if (columnId === 'used') return row.last_used_at;
      if (columnId === 'network') return row.ip_class;
      if (columnId === 'method') return methodOf(row);
      return row.user_agent;
    },
  });

  const continued = new Continued(() => order.rows);
  $effect(() => {
    void search;
    untrack(() => continued.restart());
  });

  const title = $derived(profileTitle('sessions'));
  $effect(() => page.entitle(title));

  /**
   * How a session was opened, in words: the provider by its name where one opened it, and nothing
   * at all for a session from before this was recorded - a word nobody can act on is not printed.
   */
  function methodOf(row: { signed_in_with?: string | null; signed_in_provider?: string | null }): string {
    const method = row.signed_in_with;
    if (!method) return '';
    if (method === 'OIDC') return row.signed_in_provider || t('app.sessions.method_OIDC');
    return t(`app.sessions.method_${method}`);
  }

  /** An instant as this reader reads one: their locale, their clock (`i18n-l10n.md` §4). */
  function when(at: string | null | undefined): string {
    return at ? formatDateTime(at, messages.locale) : t('app.sessions.never_used');
  }

  async function endOne(id: string, isCurrent: boolean): Promise<void> {
    failure = undefined;
    try {
      await sessions.end(id);
      // This session is over at the server, so nothing can be sent under it any more: what is
      // waiting is kept for this account's next sign-in rather than discarded unasked.
      if (isCurrent) await session.signOut({ keepUnsent: true });
      else announcer.say(t('app.sessions.ended_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  /** Ends every other session; this one stays, and the list shows that it did. */
  async function endElsewhere(): Promise<void> {
    failure = undefined;
    try {
      await sessions.endOthers();
      announcer.say(t('app.sessions.ended_all_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  /** Whether there is anything else to end: one session is this one, and nothing is elsewhere. */
  const elsewhere = $derived(sessions.all.some((row) => !row.current));

  const clientOf = (row: { user_agent?: string | null }) => row.user_agent || t('app.sessions.unknown_client');

  // Ending is destructive, so it is a row's ⋯ and the head's ⋯, each confirmed by a Dialog S (PS-6).
  function rowMenu(row: (typeof sessions.all)[number]): DataRowMenu {
    return {
      label: t('app.page.more_for', { name: clientOf(row) }),
      items: [{ id: 'end', label: row.current ? t('app.sessions.end_this') : t('app.sessions.end_menu'), isDestructive: true }],
    };
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <ListPage
    head={profileHead('sessions', {
      menu: elsewhere ? { label: t('app.page.more_for', { name: title }), items: [{ id: 'others', label: t('app.sign_out.everywhere_else'), isDestructive: true }], onselect: () => (asking = 'others') } : undefined,
    })}
  >
    {#snippet intro()}<p class="quiet">{t('app.sessions.intro')}{#if elsewhere}{' '}{t('app.sign_out.everywhere_else_note')}{/if}</p>{/snippet}
    {#snippet filters()}
      <FilterBar
        searchLabel={t('app.sessions.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        countLabel={held?.status === 'ready' ? t('app.page.found', { shown: String(found.length), count: String(sessions.all.length) }) : undefined}
      />
    {/snippet}

    {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

    {#if held === undefined || held.status === 'loading' || held.status === 'idle'}
      <div aria-busy="true"><Skeleton lines={2} /></div>
    {:else if held.status === 'failed'}
      <FailureState problem={renderProblem(held.error, messages)} onRetry={() => sessions.open()} />
    {:else}
      <DataTable
        label={title}
        {columns}
        rows={continued.shown}
        key={(row) => row.id}
        rowLabel={clientOf}
        menu={rowMenu}
        onmenu={(row) => (asking = { id: row.id, isCurrent: row.current, client: clientOf(row) })}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        sort={order.sort}
        onSort={(next) => order.by(next)}
        shownLabel={sessions.all.length === 0 ? undefined : continued.shownLabel}
        more={continued.more}
        perPage={continued.perPage}
      >
        {#snippet cell(row, column: DataColumn)}
          {#if column.id === 'client'}
            <!-- The whole string in `title`: a user agent is 150 characters of version numbers and
                 the cell shows the two lines that identify the browser. -->
            <span class="who">
              <span class="client" title={row.user_agent || undefined}>{clientOf(row)}</span>
              {#if row.current}<span class="here">{t('app.sessions.this_device')}</span>{/if}
            </span>
          {:else if column.id === 'created'}
            {when(row.created_at)}
          {:else if column.id === 'method'}
            {methodOf(row)}
          {:else if column.id === 'used'}
            {when(row.last_used_at)}
          {:else}
            {row.ip_class ?? '—'}
          {/if}
        {/snippet}
        {#snippet empty()}
          {#if sessions.all.length === 0}
            <EmptyState kind="unused" title={t('app.sessions.none')} />
          {:else}
            <EmptyState kind="filtered" title={t('app.sessions.filtered')}>
              {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
            </EmptyState>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </ListPage>

  <FinalActDialog
    isOpen={asking !== undefined}
    title={asking === 'others'
      ? t('app.sessions.others_confirm_title')
      : asking?.isCurrent
        ? t('app.sessions.this_confirm_title')
        : t('app.sessions.end_confirm_title')}
    body={asking === 'others'
      ? t('app.sessions.others_confirm_body')
      : asking?.isCurrent
        ? t('app.sessions.this_confirm_body')
        : t('app.sessions.end_confirm_body', { client: asking?.client ?? '' })}
    confirmLabel={asking === 'others'
      ? t('app.sign_out.everywhere_else')
      : asking?.isCurrent
        ? t('app.sessions.end_this')
        : t('app.sessions.end')}
    busyLabel={t('app.sessions.ending')}
    onConfirm={async () => {
      const chosen = asking;
      if (chosen === 'others') await endElsewhere();
      else if (chosen) await endOne(chosen.id, chosen.isCurrent);
    }}
    onClose={() => (asking = undefined)}
  />
{/if}

<style>
  /* One line and then an ellipsis, with the whole string in `title`: a user agent is the longest
     value here by an order of magnitude, and a second line doubled every row of a long list.
     The measure is in `ch` because what is being limited is a run of text, not a box. */
  .client {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 1;
    line-clamp: 1;
    overflow: hidden;
    max-inline-size: 44ch; /* design-system-lint-ignore: a text measure in the font's own units; no token can express it. */
    overflow-wrap: anywhere;
  }

  /* Which row is this device, beside its name rather than as a column of its own: it is true of
     exactly one row, and a column would be empty in every other. */
  /* The badge beside the name where the row has room, so this device's row is as tall as the rest. */
  .who { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-100); }

  .here {
    display: inline-block;
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--bg-surface-pressed);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-regular);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
