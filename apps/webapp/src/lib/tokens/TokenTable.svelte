<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A holder's tokens as one region (page-system PS-4): what each is for, what it may do in words,
  // when it ends, when it was last used and its standing. Withdrawing is in the row's menu behind a
  // Dialog S (PS-6): whatever used the token stops, and it cannot be restored.
  import { untrack } from 'svelte';

  import { Badge, DataTable, EmptyState, Spinner, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';
  import type { AccessToken } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import FinalActDialog from '../frame/FinalActDialog.svelte';
  import { tokens } from '../data/tokens.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { sorting } from '../sorting.svelte.ts';
  import { scopeWord, standingOf } from './tokenwords.ts';

  type Token = AccessToken;

  /** The service account whose tokens these are. */
  const { accountId, label }: { accountId: string; label: string } = $props();

  $effect(() => untrack(() => tokens.open(accountId)));

  const reading = $derived(tokens.stateOf(accountId));
  const held = $derived(tokens.of(accountId));
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let withdrawing = $state<{ id: string; name: string } | undefined>(undefined);

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.tokens.name'), isSortable: true, priority: 1 },
    { id: 'scopes', label: t('app.tokens.scopes'), priority: 2 },
    { id: 'expiry', label: t('app.tokens.expires'), isSortable: true, priority: 3 },
    { id: 'used', label: t('app.tokens.last_used'), isSortable: true, priority: 3 },
    { id: 'state', label: t('app.tokens.state'), isSortable: true, isStatus: true },
  ]);

  const stateOf = (token: Token): string => t(`app.tokens.${standingOf(token)}`);

  // Sorted here, because the whole list is here: `GET /auth/tokens` answers it in one response.
  const order = sorting({
    rows: () => held,
    read: (token, columnId) => {
      if (columnId === 'expiry') return token.expires_at;
      if (columnId === 'used') return token.last_used_at ?? undefined;
      if (columnId === 'state') return stateOf(token);
      return token.name;
    },
  });

  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);

  function menuOf(token: Token): DataRowMenu | undefined {
    if (token.revoked_at) return undefined;
    return { label: t('app.page.more_for', { name: token.name }), items: [{ id: 'withdraw', label: t('app.tokens.revoke_menu'), isDestructive: true }] };
  }
</script>

{#if failure}<FailureNote problem={failure} />{/if}

{#if reading.status === 'loading' || reading.status === 'idle'}
  <p class="waiting"><Spinner label={t('app.tokens.reading_service')} /> <span>{t('app.tokens.reading_service')}</span></p>
{:else if refusal}
  <FailureNote problem={refusal} />
{:else}
  <DataTable
    {label}
    {columns}
    rows={order.rows}
    key={(token: Token) => token.id}
    rowLabel={(token: Token) => token.name}
    menu={menuOf}
    onmenu={(token: Token) => (withdrawing = { id: token.id, name: token.name })}
    actionsLabel={t('app.page.row_actions')}
    sheetDismissLabel={t('app.page.sheet_close')}
    sheetGrabLabel={t('app.page.sheet_grab')}
    sort={order.sort}
    onSort={(next) => order.by(next)}
  >
    {#snippet cell(token: Token, column: DataColumn)}
      {#if column.id === 'name'}
        <span class="name">{token.name}</span>
      {:else if column.id === 'scopes'}
        <span class="quiet">{token.scopes.map(scopeWord).join(', ')}</span>
      {:else if column.id === 'expiry'}
        {when(token.expires_at)}
      {:else if column.id === 'used'}
        {when(token.last_used_at) ?? t('app.tokens.never_used')}
      {:else}
        <Badge tone={standingOf(token) === 'revoked' ? 'danger' : standingOf(token) === 'expired' ? 'warning' : 'success'}>{stateOf(token)}</Badge>
      {/if}
    {/snippet}
    {#snippet empty()}
      <EmptyState kind="unused" title={t('app.tokens.none_service')} />
    {/snippet}
  </DataTable>
{/if}

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
  .quiet { color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
</style>
