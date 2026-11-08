<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A person's own personal access tokens (`security.md` §5), a list page (page-system §9.5).
  //
  // **Profile configuration rather than administration**, which is ADR-0032's split rather than
  // convenience: these are this person's credentials, nobody else can list them, and an
  // administrator who could would learn exactly which of somebody's automations to attack.
  //
  // *New token* is the head's primary and a create page of its own (scopes by area); a row opens
  // the token's record page; revoking is the row's ⋯ and a Dialog S. **The two ways a token stops
  // working are told apart**, because "it ran out" and "somebody pulled it" are different answers
  // and the contract keeps them in separate columns for exactly that reason.

  import { untrack } from 'svelte';

  import { Badge, Button, DataTable, EmptyState, FilterBar, ListPage, Spinner, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';
  import type { AccessToken } from '@hubtask/sync-engine';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { tokens } from '../lib/data/tokens.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import { scopeWord, standingOf } from '../lib/tokens/tokenwords.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  $effect(() => untrack(() => tokens.open()));
  const title = $derived(profileTitle('tokens'));
  $effect(() => page.entitle(title));

  const reading = $derived(tokens.stateOf());
  const held = $derived(tokens.of());
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /**
   * The token the reader is asked about. Revoking is final - whatever used it stops, and a revoked
   * token cannot be restored, only a new one made - so it asks first (ux-inventory.md §E0).
   */
  let withdrawing = $state<{ id: string; name: string } | undefined>(undefined);

  let search = $state('');
  const found = $derived(held.filter((token) => matchesSearch(search, [token.name, ...token.scopes.map(scopeWord)])));

  // Scopes are not sortable: the cell is a list, and sorting a list of lists by its first entry
  // would be an order nobody asked for that looks like one somebody did.
  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.tokens.name'), isSortable: true, priority: 1 },
    { id: 'scopes', label: t('app.tokens.scopes'), priority: 3 },
    { id: 'expiry', label: t('app.tokens.expires'), isSortable: true, priority: 2 },
    { id: 'used', label: t('app.tokens.last_used'), isSortable: true, priority: 3 },
    { id: 'state', label: t('app.tokens.state'), isSortable: true, priority: 1, isStatus: true },
  ]);

  /** Which of the three words the state column draws. Read twice: once to sort, once to render. */
  const stateOf = (token: AccessToken): string => t(`app.tokens.${standingOf(token)}`);

  const order = sorting({
    rows: () => found,
    read: (token, columnId) => {
      if (columnId === 'expiry') return token.expires_at;
      if (columnId === 'used') return token.last_used_at;
      // The state sorts as the word the reader sees: "every revoked one together".
      if (columnId === 'state') return stateOf(token);
      return token.name;
    },
  });
  const continued = new Continued(() => order.rows);
  $effect(() => {
    void search;
    untrack(() => continued.restart());
  });

  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);
  const recordOf = (token: AccessToken) => `/profile/tokens/${token.id}`;

  function rowMenu(token: AccessToken): DataRowMenu {
    return {
      label: t('app.page.more_for', { name: token.name }),
      items: [
        { id: 'open', label: t('app.page.open') },
        ...(token.revoked_at ? [] : [{ id: 'revoke', label: t('app.tokens.revoke_menu'), isDestructive: true, hasSeparatorBefore: true }]),
      ],
    };
  }
</script>

<ListPage
  head={profileHead('tokens', {
    primary: { label: t('app.tokens.new'), icon: 'plus', opener: 'new-token', onclick: () => onnavigate?.('/profile/tokens/new') },
  })}
>
  {#snippet intro()}<p class="quiet">{t('app.tokens.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.tokens.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(found.length), count: String(held.length) }) : undefined}
    />
  {/snippet}

  {#if failure}<FailureNote problem={failure} />{/if}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting"><Spinner label={t('app.tokens.reading')} /> <span>{t('app.tokens.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={title}
      {columns}
      rows={continued.shown}
      key={(token: AccessToken) => token.id}
      href={recordOf}
      rowLabel={(token: AccessToken) => token.name}
      menu={rowMenu}
      onmenu={(token: AccessToken, id: string) => {
        if (id === 'open') onnavigate?.(recordOf(token));
        else withdrawing = { id: token.id, name: token.name };
      }}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      shownLabel={held.length === 0 ? undefined : continued.shownLabel}
      more={continued.more}
      perPage={continued.perPage}
    >
      {#snippet cell(token: AccessToken, column: DataColumn)}
        {#if column.id === 'name'}
          {token.name}
        {:else if column.id === 'scopes'}
          <span class="quiet">{token.scopes.map(scopeWord).join(', ')}</span>
        {:else if column.id === 'expiry'}
          {when(token.expires_at)}
        {:else if column.id === 'used'}
          {when(token.last_used_at) ?? t('app.tokens.never_used')}
        {:else if token.revoked_at}
          <Badge tone="danger">{t('app.tokens.revoked')}</Badge>
        {:else if standingOf(token) === 'expired'}
          <Badge tone="warning">{t('app.tokens.expired')}</Badge>
        {:else}
          <Badge tone="success">{t('app.tokens.live')}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if held.length === 0}
          <EmptyState kind="unused" title={t('app.tokens.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.tokens.filtered')}>
            {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

<FinalActDialog
  isOpen={withdrawing !== undefined}
  title={t('app.tokens.revoke_confirm_title', { name: withdrawing?.name ?? '' })}
  body={t('app.tokens.revoke_confirm_body', { name: withdrawing?.name ?? '' })}
  confirmLabel={t('app.tokens.revoke')}
  busyLabel={t('app.tokens.working')}
  onConfirm={async () => {
    if (!withdrawing) return;
    try {
      await tokens.revoke(withdrawing.id);
      announcer.say(t('app.tokens.revoked_announced'));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    }
  }}
  onClose={() => (withdrawing = undefined)}
/>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
</style>
