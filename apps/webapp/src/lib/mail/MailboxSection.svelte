<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Connected mailboxes (ADR-0094): the door for a workspace without a receiving domain - one
  // existing mailbox at Gmail, GMX, web.de or IONOS, or any IMAP or JMAP server. A provider is
  // chosen, the address and an (app) password typed, and Hubtask signs in once before it stores
  // anything: a mailbox that cannot be read is said here, in the form, and not in a log an hour
  // later. Only mail that arrives after connecting is taken, so connecting a personal mailbox
  // takes nothing that was already in it.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, Radio, Select, Spinner, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import {
    checkMailbox,
    connectMailbox,
    readMailboxes,
    removeMailbox,
    type Mailbox,
    type MailboxPreset,
  } from '../data/mailintake.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** Told when the list changes, so the page can show addresses through the mailbox. */
    onchange?: (mailboxes: readonly Mailbox[]) => void;
  }

  let { onchange }: Props = $props();

  let mailboxes = $state<readonly Mailbox[]>([]);
  let presets = $state<readonly MailboxPreset[]>([]);
  let isReading = $state(true);
  let readFailure = $state<string | undefined>(undefined);

  let preset = $state('gmail');
  let protocol = $state<'IMAP' | 'JMAP'>('IMAP');
  let address = $state('');
  let password = $state('');
  let host = $state('');
  let port = $state('993');
  let sessionUrl = $state('');
  let username = $state('');
  let isConnecting = $state(false);
  let connectFailure = $state<string | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let rowFailure = $state<string | undefined>(undefined);

  const isOther = $derived(preset === 'other');
  const chosen = $derived(presets.find((each) => each.key === preset));
  const providers = $derived([
    ...presets.map((each) => ({ value: each.key, label: t(`app.mail.provider.${each.key}`) })),
    { value: 'other', label: t('app.mail.provider.other') },
  ]);

  function take(next: readonly Mailbox[]) {
    mailboxes = next;
    onchange?.(next);
  }

  $effect(() =>
    untrack(() => {
      void readMailboxes()
        .then((read) => {
          presets = read.presets;
          take(read.mailboxes);
        })
        .catch((error) => (readFailure = renderProblem(error as never, messages).message))
        .finally(() => (isReading = false));
    }),
  );

  async function connect() {
    connectFailure = undefined;
    isConnecting = true;
    try {
      const connected = await connectMailbox({
        ...(isOther ? { protocol } : { preset }),
        address: address.trim(),
        secret: password,
        ...(isOther && protocol === 'IMAP' ? { host: host.trim(), port: Number(port) || 993 } : {}),
        ...(isOther && protocol === 'JMAP' ? { session_url: sessionUrl.trim() } : {}),
        ...(username.trim() ? { username: username.trim() } : {}),
      });
      take([...mailboxes, connected]);
      address = '';
      password = '';
      announcer.say(t('app.mail.mailbox_connected', { address: connected.address }));
    } catch (error) {
      connectFailure = renderProblem(error as never, messages).message;
    } finally {
      isConnecting = false;
      password = '';
    }
  }

  async function check(box: Mailbox) {
    rowFailure = undefined;
    working = box.id;
    try {
      const checked = await checkMailbox(box.id);
      take(mailboxes.map((each) => (each.id === box.id ? checked : each)));
      announcer.say(checked.failure ? t(checked.failure) : t('app.mail.mailbox_ok', { address: box.address }));
    } catch (error) {
      rowFailure = renderProblem(error as never, messages).message;
    } finally {
      working = undefined;
    }
  }

  async function remove(box: Mailbox) {
    rowFailure = undefined;
    working = box.id;
    try {
      await removeMailbox(box.id);
      take(mailboxes.filter((each) => each.id !== box.id));
      announcer.say(t('app.mail.mailbox_removed', { address: box.address }));
    } catch (error) {
      rowFailure = renderProblem(error as never, messages).message;
    } finally {
      working = undefined;
    }
  }
</script>

<Stack gap="150">
  <p class="quiet small">{t('app.mail.mailboxes_hint')}</p>
  {#if isReading}
    <p class="quiet"><Spinner label={t('app.mail.reading')} /> {t('app.mail.reading')}</p>
  {:else if readFailure}
    <Banner tone="danger">{readFailure}</Banner>
  {:else}
    {#if mailboxes.length > 0}
      <ul class="boxes">
        {#each mailboxes as box (box.id)}
          <li data-mailbox={box.address}>
            <div class="box-head">
              <strong>{box.address}</strong>
              <span class="quiet small">{box.protocol} · {box.host ?? box.session_url}</span>
            </div>
            <p class="small" class:danger={box.failure} data-mailbox-state>
              {#if box.failure}
                {t(box.failure)}
              {:else if box.checked_at}
                {t('app.mail.mailbox_read_at', { when: formatDateTime(box.checked_at, messages.locale) })}
              {:else}
                {t('app.mail.mailbox_waiting')}
              {/if}
            </p>
            <div class="actions">
              <Button size="sm" tone="secondary" isBusy={working === box.id} busyLabel={t('app.mail.checking')} onclick={() => void check(box)}>{t('app.mail.mailbox_check')}</Button>
              <Button size="sm" tone="secondary" onclick={() => void remove(box)}>{t('app.mail.mailbox_remove')}</Button>
            </div>
          </li>
        {/each}
      </ul>
      {#if rowFailure}<Banner tone="danger">{rowFailure}</Banner>{/if}
    {/if}

    <div class="connect" data-mailbox-connect>
      <Select label={t('app.mail.provider')} options={providers} bind:value={preset} />
      {#if chosen}<p class="quiet small" data-mailbox-hint>{t(chosen.hint)}</p>{/if}
      {#if isOther}
        <Radio
          label={t('app.mail.protocol')}
          options={[
            { value: 'IMAP', label: 'IMAP', hint: t('app.mail.protocol_imap_hint') },
            { value: 'JMAP', label: 'JMAP', hint: t('app.mail.protocol_jmap_hint') },
          ]}
          bind:value={() => protocol, (next) => (protocol = next as 'IMAP' | 'JMAP')}
        />
      {/if}
      <div class="fields">
        <Input label={t('app.mail.mailbox_address')} type="email" autocomplete="off" bind:value={address} />
        <Input
          label={isOther && protocol === 'JMAP' || preset === 'fastmail' ? t('app.mail.mailbox_token') : t('app.mail.mailbox_password')}
          type="password"
          autocomplete="new-password"
          bind:value={password}
        />
        {#if isOther && protocol === 'IMAP'}
          <Input label={t('app.mail.mailbox_host')} placeholder="imap.example.com" bind:value={host} />
          <Input label={t('app.mail.mailbox_port')} inputmode="numeric" bind:value={port} />
        {/if}
        {#if isOther && protocol === 'JMAP'}
          <Input label={t('app.mail.mailbox_session_url')} placeholder="https://…/jmap/session" bind:value={sessionUrl} />
        {/if}
        {#if isOther}
          <Input label={t('app.mail.mailbox_username')} hint={t('app.mail.mailbox_username_hint')} bind:value={username} />
        {/if}
      </div>
      {#if connectFailure}<Banner tone="danger">{connectFailure}</Banner>{/if}
      <div>
        <Button
          tone="primary"
          isBusy={isConnecting}
          busyLabel={t('app.mail.connecting')}
          disabledReason={address.trim() && password ? undefined : t('app.mail.mailbox_fill_first')}
          onclick={() => void connect()}
        >{t('app.mail.mailbox_connect')}</Button>
      </div>
    </div>
  {/if}
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .danger { color: var(--text-danger); }

  .boxes { display: grid; gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }

  .boxes li {
    display: grid;
    gap: var(--sp-050);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
  }

  .boxes li p { margin: 0; }

  .box-head { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: baseline; }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .connect { display: grid; gap: var(--sp-150); }

  .fields { display: grid; gap: var(--sp-100); max-inline-size: calc(var(--sp-1000) * 6); }
</style>
