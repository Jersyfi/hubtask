<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A workspace's address changes (UC-INS-19 checks 3 to 8, UC-INS-20 check 4; multi-tenancy.md
  // §3.1): the operator's from the installation's *Workspaces*, the owner's from the workspace's
  // *Address* - one dialog, so both say the same. Before anything is changed the dialog names the new address and what the
  // change ends, and who is told - the counts from the server's own dry run. Pressing *Change address*
  // asks the dry run again with the new address, so a refused name or a lockout is said here before a
  // proof is spent; the change itself is confirmed by a password or the provider, never a passkey,
  // which the step-up prompt says.
  //
  // With subdomains the field is the short name, the installation's host drawn after it; where the
  // installation assigns hosts it is the full host. Where the operator's list knows another
  // workspace already answers at the name or host, it is said at the field while it is typed, as on
  // *New workspace*; an owner sees no other workspace, and the server says it.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, Stack } from '@hubtask/design-system/components';
  import type { TenantAddressChange } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import type { AddressAnswer, AddressSubject } from './change.ts';

  interface Props {
    /** The dialog is open while it is set. */
    subject: AddressSubject | undefined;
    /** Whether another workspace already answers at the typed name or host, where the caller knows. */
    isTaken?: (typed: string) => boolean;
    /** Whether the installation assigns hosts (ADR-0095 §6). */
    isAssigned: boolean;
    /** The change, or with `dry_run` what it would be. */
    change: (body: TenantAddressChange) => Promise<AddressAnswer>;
    onclose: () => void;
    /** After the change went through. */
    onchanged?: (answer: AddressAnswer) => void;
  }

  const { subject, isTaken = () => false, isAssigned, change: send, onclose, onchanged }: Props = $props();
  const tenant = $derived(subject);

  let value = $state('');
  let counts = $state<AddressAnswer | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The lockout refusal: whose, and what is missing. */
  let lockout = $state<{ self: boolean; reason: string } | undefined>(undefined);

  $effect(() => {
    if (!tenant) return;
    value = '';
    counts = undefined;
    failure = undefined;
    lockout = undefined;
    const asked = tenant.id;
    // Untracked: what the call reads is not what this dialog follows.
    untrack(() => send({ dry_run: true }))
      .then((answer) => {
        if (tenant?.id === asked) counts = answer;
      })
      .catch((cause: unknown) => {
        if (tenant?.id === asked) failure = problem(cause);
      });
  });

  /** The main address now, from the dry run once it answered. */
  const from = $derived(counts?.from ?? tenant?.host ?? '');
  /** The installation's host a short name goes in front of: the main address without the name. */
  const base = $derived.by(() => {
    const prefix = `${tenant?.slug ?? ''}.`;
    return from.startsWith(prefix) ? from.slice(prefix.length) : from.split('.').slice(1).join('.');
  });
  const typed = $derived(value.trim().toLowerCase().replace(/\.$/, ''));
  const then = $derived(typed === '' ? '' : isAssigned ? typed : `${typed}.${base}`);
  const taken = $derived(typed !== '' && isTaken(typed));
  const fieldError = $derived(
    taken
      ? isAssigned
        ? t('app.instance.host_taken', { host: typed })
        : t('app.instance.slug_taken', { slug: typed })
      : failure?.fields.get(isAssigned ? '/host' : '/slug'),
  );

  function problem(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }

  async function change(): Promise<void> {
    if (!tenant || typed === '' || taken) return;
    isWorking = true;
    failure = undefined;
    lockout = undefined;
    const address = isAssigned ? { host: value.trim() } : { slug: value.trim() };
    try {
      await send({ ...address, dry_run: true });
      const answer = await send(address);
      onclose();
      onchanged?.(answer);
    } catch (cause) {
      const code = cause instanceof TransportError ? cause.detailCode : undefined;
      if (code === 'admin.address_lockout_self' || code === 'admin.address_lockout_owner') {
        const reason = cause instanceof TransportError ? cause.params?.reason : undefined;
        lockout = { self: code === 'admin.address_lockout_self', reason: reason === 'factor' ? 'factor' : 'way_in' };
      } else {
        failure = problem(cause);
      }
    } finally {
      isWorking = false;
    }
  }

  const who = $derived(lockout?.self ? 'self' : 'owner');
</script>

<Dialog
  title={t('app.instance.address_title', { workspace: tenant?.name ?? '' })}
  isOpen={tenant !== undefined}
  isWide
  dismissLabel={t('app.instance.cancel')}
  onClose={onclose}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={onclose}>{t('app.instance.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.instance.working')} onclick={() => void change()}>
      {t('app.instance.address_submit')}
    </Button>
  {/snippet}
  <Stack gap="200">
    {#if lockout}
      <!-- The refusal names what is missing, never the owner: the operator sees no account inside a
           workspace (P-01), and an owner is told no more than the operator. -->
      <Banner tone="danger" title={t(`app.instance.address_lockout_${who}_title`)}>
        {t(`app.instance.address_lockout_${who}_${lockout.reason}`)}
      </Banner>
    {:else if failure && !fieldError}
      <Banner tone="danger" title={failure.message} />
    {/if}
    <form
      id="change-address"
      onsubmit={(event) => {
        event.preventDefault();
        void change();
      }}
    >
      <Input
        label={t('app.instance.address_label')}
        hint={lockout ? undefined : isAssigned ? t('app.instance.host_hint') : t('app.instance.address_hint')}
        error={fieldError}
        face="code"
        suffix={isAssigned || base === '' ? undefined : `.${base}`}
        autocomplete="off"
        spellcheck={false}
        isRequired
        bind:value
      />
    </form>
    {#if !lockout}
      <dl class="fromto">
        <dt>{t('app.instance.address_now')}</dt>
        <dd class="old">{from}</dd>
        <dt>{t('app.instance.address_then')}</dt>
        <dd>{then}</dd>
      </dl>
      <Banner tone="warning" title={t('app.instance.address_costs')}>
        <ul class="costs">
          <li>{t('app.instance.address_cost_old', { host: from })}</li>
          <li>{t('app.instance.address_cost_sign_in')}</li>
          <li>{t('app.instance.address_cost_passkeys')}</li>
          <li>{t('app.instance.address_cost_links')}</li>
        </ul>
      </Banner>
      {#if counts}
        <Banner tone="info">
          {t('app.instance.address_told', {
            mailed: counts.members_mailed,
            unmailed: counts.members_told_at_sign_in,
          })}
        </Banner>
      {/if}
    {/if}
  </Stack>
</Dialog>

<style>
  form { margin: 0; }

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

  .costs { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }
</style>
