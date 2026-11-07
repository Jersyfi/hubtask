<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A person's own personal access tokens (`security.md` §5).
  //
  // **Profile configuration rather than administration**, which is ADR-0032's split rather than
  // convenience: these are this person's credentials, nobody else can list them, and an
  // administrator who could would learn exactly which of somebody's automations to attack.
  //
  // **The form is `MintTokenForm`**, shared with a service account's *New token* dialog, and keeps
  // the three rules a token's making must not soften.
  //
  // **The credential appears once, in `OneTimeSecret`, and this screen keeps no copy.** It is held
  // in one `$state` for as long as the panel is open and dropped when it closes; nothing writes it
  // to storage, to the address, or to a log.
  //
  // **The two ways a token stops working are told apart**, because "it ran out" and "somebody
  // pulled it" are different answers to the same question and the contract keeps them in separate
  // columns for exactly that reason.

  import { untrack } from 'svelte';

  import { Badge, Button, EmptyState, OneTimeSecret, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { tokens, type MintedToken } from '../lib/data/tokens.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import MintTokenForm from '../lib/tokens/MintTokenForm.svelte';
  import { scopeWord, standingOf } from '../lib/tokens/tokenwords.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    /** A service account's tokens, where the caller may mint for one. Absent means their own. */
    accountId?: string;
    /**
     * Whether the reader may mint for that service account: a token for it is held to the role
     * ceiling (identity.md §13), so a role it holds above the reader's own is a certain refusal
     * (P-05). One's own tokens are always minted.
     */
    mintable?: boolean;
  }

  const { accountId, mintable = true }: Props = $props();

  let minted = $state<MintedToken | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /**
   * The token the reader is asked about. Withdrawing is final - whatever used it stops, and a
   * withdrawn token cannot be restored, only a new one made - so it asks first (ux-inventory.md
   * §E0).
   */
  let withdrawing = $state<{ id: string; name: string } | undefined>(undefined);

  $effect(() => untrack(() => tokens.open(accountId)));

  // The credential dies with the screen. Not a courtesy: it is the one value here that is a
  // credential, and a panel that left it behind would leave it in memory for the tab's life.
  $effect(() => () => (minted = undefined));

  const reading = $derived(tokens.stateOf(accountId));
  // A service account's panel speaks about the service account: "You have no tokens" said to the
  // administrator looking at it was about the wrong person. So is the CalDAV note, which tells
  // somebody their own address is the user name.
  const readingLabel = $derived(accountId ? t('app.tokens.reading_service') : t('app.tokens.reading'));
  const held = $derived(tokens.of(accountId));

  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  // Scopes are not sortable: the cell is a list, and sorting a list of lists by its first entry
  // would be an order nobody asked for that looks like one somebody did.
  const columns = $derived([
    { id: 'name', label: t('app.tokens.name'), isSortable: true },
    { id: 'scopes', label: t('app.tokens.scopes') },
    { id: 'expiry', label: t('app.tokens.expires'), isSortable: true },
    { id: 'used', label: t('app.tokens.last_used'), isSortable: true },
    { id: 'state', label: t('app.tokens.state'), isSortable: true },
    { id: 'actions', label: t('app.tokens.actions'), isLabelHidden: true },
  ]);

  // Sorted here, because the whole list is here: `GET /auth/tokens` answers it in one response
  // for a personal account, and nothing on this screen is holding a cursor.
  const order = sorting({
    rows: () => held,
    read: (token, columnId) => {
      if (columnId === 'expiry') return token.expires_at;
      if (columnId === 'used') return token.last_used_at;
      // The state is a word the reader sees, so it sorts as that word rather than as a timestamp:
      // "every revoked one together" is the question somebody sorting this column is asking.
      if (columnId === 'state') return stateOf(token);
      return token.name;
    },
  });

  /** Which of the three words the state column draws. Read twice: once to sort, once to render. */
  const stateOf = (token: { revoked_at?: string | null; expires_at: string }): string => t(`app.tokens.${standingOf(token)}`);

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

</script>

<Stack gap="300">
  {#if minted}
    <!-- The only time this value exists anywhere but the server's hash. The acknowledgement is
         required, because a token scrolled past is a token nobody copied. -->
    <OneTimeSecret
      value={minted.token}
      label={t('app.tokens.minted_title')}
      hint={t('app.tokens.minted_hint')}
      revealLabel={t('app.tokens.reveal')}
      hideLabel={t('app.tokens.hide')}
      copyLabel={t('app.tokens.copy')}
      copiedLabel={t('app.tokens.copied')}
      acknowledgementLabel={t('app.tokens.kept')}
      notAcknowledgedReason={t('app.tokens.keep_first')}
      dismissLabel={t('app.tokens.done')}
      onDismiss={() => (minted = undefined)}
    />
    <!-- The one other thing a token is for, said here because no other screen says it: what a calendar
         client asks for as the password. -->
    {#if !accountId}<p class="quiet">{t('app.tokens.caldav_note')}</p>{/if}
  {/if}

  {#if failure}
    <FailureNote problem={failure} />
  {/if}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting"><Spinner label={readingLabel} /> <span>{readingLabel}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else if held.length === 0}
    <EmptyState kind="unused" title={accountId ? t('app.tokens.none_service') : t('app.tokens.none')} />
  {:else}
    <Table isStacked
      label={t('app.tokens.title')}
      isLabelHidden
      {columns}
      sort={order.sort}
      onSort={(next) => order.by(next)}
    >
      {#each order.rows as token (token.id)}
        <tr>
          <th scope="row" class="name">{token.name}</th>
          <td class="scopes">{token.scopes.map(scopeWord).join(', ')}</td>
          <td>{when(token.expires_at)}</td>
          <td>{when(token.last_used_at) ?? t('app.tokens.never_used')}</td>
          <td>
            {#if token.revoked_at}
              <!-- Somebody pulled it. A different fact from "it ran out", and the contract keeps
                   them in separate columns so that this screen can say which. -->
              <Badge tone="danger">{t('app.tokens.revoked')}</Badge>
            {:else if new Date(token.expires_at).getTime() <= Date.now()}
              <Badge tone="warning">{t('app.tokens.expired')}</Badge>
            {:else}
              <Badge tone="success">{t('app.tokens.live')}</Badge>
            {/if}
          </td>
          <td>
            {#if !token.revoked_at}
              <!-- `danger`: withdrawing a token stops whatever was using it, at once once the
                   dialog is confirmed. Every row action in these settings that takes access away now
                   carries the tone, because a reader should be able to tell the destructive
                   control from the harmless one without reading it. -->
              <Button
                size="sm"
                tone="danger"
                onclick={() => (withdrawing = { id: token.id, name: token.name })}
              >
                {t('app.tokens.revoke')}
              </Button>
            {/if}
          </td>
        </tr>
      {/each}
    </Table>

  {/if}

  {#if !mintable}
    <p class="quiet">{t('app.tokens.mint_above_own')}</p>
  {:else}
    <Stack gap="150">
      <h2 class="section">{t('app.tokens.mint_title')}</h2>
      <MintTokenForm {accountId} onminted={(next) => (minted = next)} />
    </Stack>
  {/if}
</Stack>

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
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .scopes { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }

</style>
