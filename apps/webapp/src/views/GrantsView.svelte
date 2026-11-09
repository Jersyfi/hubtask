<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The apps that may act in this workspace as the reader.
  //
  // The same question the sessions ask, asked about apps: what is currently able to act as me, and
  // how do I stop it. Withdrawing takes effect on the app's next request, which the confirmation
  // says rather than implying that what is running now stops mid-sentence.
  //
  // A list page (page-system §9.5): a row is a mark, the name, when it was allowed and last used
  // (UC-SYN-07/3); withdrawing is the row's ⋯ and a Dialog S. The installed apps carry the app's
  // icon, their system and device and *Sign out*, because signing the app out is what withdrawing
  // their grant does (ADR-0096).

  import { untrack } from 'svelte';

  import { Button, DataTable, EmptyState, FilterBar, ListPage, Logo, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { consent, type Grant } from '../lib/data/consent.svelte.ts';
  import { grantOrder, grantUse, initialsOf } from '../lib/data/grants.ts';
  import { formatLongDay, formatTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem, type RenderedProblem } from '../lib/problem.ts';

  const account = $derived(actor.account);

  $effect(() => untrack(() => consent.openGrants()));

  let failure = $state<RenderedProblem | undefined>(undefined);

  const rows = $derived(grantOrder(consent.grants));
  let search = $state('');
  const found = $derived(rows.filter((grant) => matchesSearch(search, [nameOf(grant)])));

  const title = $derived(profileTitle('grants'));
  $effect(() => page.entitle(title));

  const columns = $derived<DataColumn[]>([
    { id: 'app', label: t('app.grants.app_column'), priority: 1 },
    { id: 'use', label: t('app.grants.use_column'), priority: 2 },
  ]);

  function rowMenu(grant: Grant): DataRowMenu {
    return {
      label: t('app.page.more_for', { name: nameOf(grant) }),
      items: [{ id: 'withdraw', label: t(grant.app_platform ? 'app.grants.sign_out_menu' : 'app.grants.withdraw_menu'), isDestructive: true }],
    };
  }

  /** An installed app's grant is named by its system and device (ADR-0096). */
  function nameOf(grant: Grant): string {
    return grant.app_platform
      ? t('app.grants.app_name', { platform: t(`app.app_platform.${grant.app_platform}`), device: grant.client_name })
      : grant.client_name;
  }

  /** "Allowed 28 October 2026 · Last used today, 14:32", in the reader's language. */
  function noteOf(grant: Grant): string {
    const day = formatLongDay(grant.created_at, messages.locale);
    const use = grantUse(grant.last_used_at, messages.locale);
    switch (use.kind) {
      case 'never':
        return t('app.grants.allowed_unused', { day });
      case 'today':
        return t('app.grants.allowed_used_today', { day, time: formatTime(use.at, messages.locale) });
      case 'days':
        return t('app.grants.allowed_used', { day, used: use.phrase });
      case 'day':
        return t('app.grants.allowed_used', { day, used: formatLongDay(use.at, messages.locale) });
    }
  }

  /** The app the reader is asked about: withdrawing is final - it has to be allowed again (§E0). */
  let asking = $state<{ id: string; name: string; isApp: boolean } | undefined>(undefined);

  async function withdraw(grantId: string): Promise<void> {
    failure = undefined;
    try {
      await consent.withdraw(grantId);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <ListPage head={profileHead('grants')}>
    {#snippet intro()}<p class="quiet">{t('app.grants.intro')}</p>{/snippet}
    {#snippet filters()}
      <FilterBar
        searchLabel={t('app.grants.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        countLabel={t('app.page.found', { shown: String(found.length), count: String(rows.length) })}
      />
    {/snippet}

    {#if failure}<FailureNote problem={failure} />{/if}

    <DataTable
      label={title}
      {columns}
      rows={found}
      key={(grant: Grant) => grant.id}
      rowLabel={nameOf}
      menu={rowMenu}
      onmenu={(grant: Grant) => (asking = { id: grant.id, name: nameOf(grant), isApp: !!grant.app_platform })}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={rows.length === 0 ? undefined : t('app.page.showing', { count: String(found.length) })}
    >
      {#snippet cell(grant: Grant, column: DataColumn)}
        {#if column.id === 'app'}
          <span class="app">
            {#if grant.app_platform}
              <!-- The installed app's own icon: the mark on the brand tile, as the launcher shows it. -->
              <span class="icon" aria-hidden="true"><Logo size="md" /></span>
            {:else}
              <span class="mark" aria-hidden="true">{initialsOf(grant.client_name)}</span>
            {/if}
            <span class="name">{nameOf(grant)}</span>
          </span>
        {:else}
          <span class="quiet">{noteOf(grant)}</span>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if rows.length === 0}
          <EmptyState kind="unused" title={t('app.grants.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.grants.filtered')}>
            {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  </ListPage>

  <FinalActDialog
    isOpen={asking !== undefined}
    title={t(asking?.isApp ? 'app.grants.sign_out_confirm_title' : 'app.grants.withdraw_confirm_title', { app: asking?.name ?? '' })}
    body={t(asking?.isApp ? 'app.grants.sign_out_confirm_body' : 'app.grants.withdraw_confirm_body', { app: asking?.name ?? '' })}
    confirmLabel={t(asking?.isApp ? 'app.grants.sign_out' : 'app.grants.withdraw')}
    busyLabel={t(asking?.isApp ? 'app.grants.signing_out' : 'app.grants.withdrawing')}
    onConfirm={async () => {
      if (asking) await withdraw(asking.id);
    }}
    onClose={() => (asking = undefined)}
  />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .app { display: inline-flex; align-items: center; gap: var(--sp-150); }

  .mark {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  /* The launcher's icon: the outline turns to the tile's own colour, as `app-icon.svg` draws it. */
  .icon {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-500);
    block-size: var(--sp-500);
    border-radius: var(--r-md);
    background: var(--brand-tile);
    --brand-outline: var(--brand-tile-outline);
  }

  .name { font-weight: var(--fw-medium); overflow-wrap: anywhere; }
</style>
