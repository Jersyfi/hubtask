<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The main address moves to the own domain, or back to the subdomain (UC-INS-15 checks 6, 7 and
  // 8): before anything moves the dialog names both addresses, what the move ends and what stays,
  // and who is told - the counts from the server's own dry run, which also says a lockout before a
  // proof is spent. Confirming asks for a password or the provider, never a passkey, through the
  // step-up prompt, as a change of address does.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Stack } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import type { AddressAnswer } from './change.ts';
  import type { MainAddressMove } from './owndomain.ts';

  interface Props {
    /** The dialog is open while it is set. */
    move: MainAddressMove | undefined;
    /** The move, or with `dry_run` what it would be. */
    send: (body: { dry_run?: boolean }) => Promise<AddressAnswer>;
    onclose: () => void;
    /** After the move went through. */
    onmoved?: (answer: AddressAnswer) => void;
  }

  const { move, send, onclose, onmoved }: Props = $props();

  let counts = $state<AddressAnswer | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let lockout = $state<{ self: boolean; reason: string } | undefined>(undefined);

  $effect(() => {
    if (!move) return;
    counts = undefined;
    failure = undefined;
    lockout = undefined;
    const asked = move;
    untrack(() => send({ dry_run: true }))
      .then((answer) => {
        if (move === asked) counts = answer;
      })
      .catch((cause: unknown) => {
        if (move === asked) refused(cause);
      });
  });

  function refused(cause: unknown): void {
    const code = cause instanceof TransportError ? cause.detailCode : undefined;
    if (code === 'admin.address_lockout_self' || code === 'admin.address_lockout_owner') {
      const reason = cause instanceof TransportError ? cause.params?.reason : undefined;
      lockout = { self: code === 'admin.address_lockout_self', reason: reason === 'factor' ? 'factor' : 'way_in' };
      return;
    }
    failure = cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }

  async function confirm(): Promise<void> {
    if (!move || lockout) return;
    isWorking = true;
    failure = undefined;
    try {
      const answer = await send({});
      onclose();
      onmoved?.(answer);
    } catch (cause) {
      refused(cause);
    } finally {
      isWorking = false;
    }
  }

  const toOwn = $derived(move?.direction === 'own');
  const who = $derived(lockout?.self ? 'self' : 'owner');
</script>

<Dialog
  title={move ? t(toOwn ? 'app.address.main_move.title_own' : 'app.address.main_move.title_back', { host: move.to }) : ''}
  isOpen={move !== undefined}
  isWide
  dismissLabel={t('app.instance.cancel')}
  onClose={onclose}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={onclose}>{t('app.instance.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.instance.working')} onclick={() => void confirm()}>
      {t('app.address.main_move.submit')}
    </Button>
  {/snippet}
  {#if move}
    <Stack gap="200">
      {#if lockout}
        <Banner tone="danger" title={t(`app.instance.address_lockout_${who}_title`)}>
          {t(`app.instance.address_lockout_${who}_${lockout.reason}`)}
        </Banner>
      {:else if failure}
        <Banner tone="danger" title={failure.message} />
      {/if}
      <dl class="fromto">
        <dt>{t('app.instance.address_now')}</dt>
        <dd class="old">{move.from}</dd>
        <dt>{t('app.instance.address_then')}</dt>
        <dd>{move.to}</dd>
      </dl>
      <Banner tone="warning" title={t('app.address.main_move.ends')}>
        <ul class="list">
          <li>{t('app.address.main_move.sign_in', { host: move.to })}</li>
          <li>{t(toOwn ? 'app.address.main_move.passkeys_own' : 'app.address.main_move.passkeys_back', { host: move.from })}</li>
        </ul>
      </Banner>
      {#if toOwn}
        <Banner tone="info" title={t('app.address.main_move.stays')}>
          <ul class="list">
            <li>{t('app.address.main_move.stays_redirect', { from: move.from, to: move.to })}</li>
            <li>{t('app.address.main_move.stays_way_back')}</li>
          </ul>
        </Banner>
        <p class="note">
          {#if counts}{t('app.address.main_move.told', { mailed: counts.members_mailed, unmailed: counts.members_told_at_sign_in })}{/if}
          {t('app.address.main_move.links', { host: move.to })}
        </p>
      {:else}
        <p class="note">{t('app.address.main_move.back_note', { from: move.from, to: move.to })}</p>
      {/if}
    </Stack>
  {/if}
</Dialog>

<style>
  .fromto {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: var(--sp-050) var(--sp-150);
    align-items: baseline;
    margin: 0;
    padding: var(--sp-150);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
  }

  .fromto dt { font-size: var(--fs-075); color: var(--text-subtle); }

  .fromto dd { margin: 0; font-family: var(--font-mono); overflow-wrap: anywhere; }

  .fromto dd.old {
    color: var(--text-secondary);
    text-decoration: line-through;
    text-decoration-color: var(--border-default);
  }

  .list { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }

  .note { margin: 0; max-inline-size: none; font-size: var(--fs-075); color: var(--text-subtle); }
</style>
