<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Connected mailboxes (ADR-0094): the door for a workspace without a receiving domain - one
  // existing mailbox at Gmail, GMX, web.de or IONOS, or any IMAP or JMAP server. A provider is
  // chosen, the address and an (app) password typed, and Hubtask signs in once before it stores
  // anything: a mailbox that cannot be read is said here, in the form, and not in a log an hour
  // later. Only mail that arrives after connecting is taken, so connecting a personal mailbox
  // takes nothing that was already in it.
  //
  // Or without any password in Hubtask: through the provider's own sign-in (OAuth with PKCE). Google
  // and Microsoft are one button where the operator configured an app; otherwise, and for any other
  // provider, the administrator enters an app of their own. The provider sends the browser back to
  // `/administration/mail/oauth`, and this section hands the answer to the API.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, Radio, Select, Spinner, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import {
    authorizeMailbox,
    checkMailbox,
    completeMailboxAuthorization,
    connectMailbox,
    readMailboxes,
    removeMailbox,
    type Mailbox,
    type MailboxOAuthProvider,
    type MailboxPreset,
  } from '../data/mailintake.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** Told when the list changes, so the page can show addresses through the mailbox. */
    onchange?: (mailboxes: readonly Mailbox[]) => void;
    /** What the provider sent back, on `/administration/mail/oauth`. */
    providerAnswer?: Readonly<Record<string, string>>;
    /** Told once that answer is handed on. */
    onanswered?: () => void;
  }

  let { onchange, providerAnswer, onanswered }: Props = $props();

  let oauthProviders = $state<readonly MailboxOAuthProvider[]>([]);
  let redirectUrl = $state('');
  /** The provider whose own-app form is open: a preset without the installation's app, or "custom". */
  let oauthKind = $state<string | undefined>(undefined);
  let oauthAddress = $state('');
  let clientId = $state('');
  let clientSecret = $state('');
  let authorizeUrl = $state('');
  let tokenUrl = $state('');
  let scopes = $state('');
  let oauthHost = $state('');
  let oauthPort = $state('993');
  let isLeaving = $state(false);
  let isCompleting = $state(false);
  let oauthFailure = $state<string | undefined>(undefined);

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
          oauthProviders = read.oauth_providers ?? [];
          redirectUrl = read.oauth_redirect_url ?? '';
          take(read.mailboxes);
        })
        .catch((error) => (readFailure = renderProblem(error as never, messages).message))
        .finally(() => (isReading = false));
    }),
  );

  // Back from the provider: hand its answer on once, then drop the code and the state from the address.
  $effect(() => {
    const answer = providerAnswer;
    if (!answer?.['state']) return;
    untrack(() => void complete(answer));
  });

  async function complete(answer: Readonly<Record<string, string>>) {
    oauthFailure = undefined;
    isCompleting = true;
    try {
      const connected = await completeMailboxAuthorization({
        state: answer['state'] ?? '',
        ...(answer['code'] ? { code: answer['code'] } : {}),
        ...(answer['error'] ? { error: answer['error'] } : {}),
      });
      take([...mailboxes.filter((each) => each.id !== connected.id), connected]);
      announcer.say(t('app.mail.mailbox_connected', { address: connected.address }));
    } catch (error) {
      oauthFailure = renderProblem(error as never, messages).message;
    } finally {
      isCompleting = false;
      onanswered?.();
    }
  }

  /** One button where the installation brings the app; otherwise the form for one's own. */
  function choose(provider: MailboxOAuthProvider | 'custom') {
    oauthFailure = undefined;
    if (provider !== 'custom' && provider.installation) {
      void leave({ provider: provider.key });
      return;
    }
    oauthKind = provider === 'custom' ? 'custom' : provider.key;
  }

  async function leave(request: Parameters<typeof authorizeMailbox>[0]) {
    oauthFailure = undefined;
    isLeaving = true;
    try {
      window.location.assign(await authorizeMailbox(request));
    } catch (error) {
      oauthFailure = renderProblem(error as never, messages).message;
      isLeaving = false;
    }
  }

  function leaveWithOwnApp() {
    const custom = oauthKind === 'custom';
    void leave({
      provider: oauthKind ?? 'custom',
      client_id: clientId.trim(),
      ...(clientSecret ? { client_secret: clientSecret } : {}),
      ...(oauthAddress.trim() ? { address: oauthAddress.trim() } : {}),
      ...(custom
        ? {
            authorize_url: authorizeUrl.trim(),
            token_url: tokenUrl.trim(),
            scopes: scopes.split(/[\s,]+/).filter(Boolean),
            host: oauthHost.trim(),
            port: Number(oauthPort) || 993,
          }
        : {}),
    });
  }

  async function copyRedirect() {
    try {
      await navigator.clipboard.writeText(redirectUrl);
      announcer.say(t('app.mail.oauth_redirect_copied'));
    } catch {
      /* The address stays on screen to copy by hand. */
    }
  }

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
              <span class="quiet small">
                {box.protocol} · {box.host ?? box.session_url}{#if box.auth === 'OAUTH'} · {t('app.mail.via_oauth', { provider: t(`app.mail.oauth_provider.${box.oauth_provider ?? 'custom'}`) })}{/if}
              </span>
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

    {#if isCompleting}
      <p class="quiet" data-mailbox-completing><Spinner label={t('app.mail.oauth_completing')} /> {t('app.mail.oauth_completing')}</p>
    {/if}
    {#if oauthFailure}<div data-mailbox-oauth-failure><Banner tone="danger">{oauthFailure}</Banner></div>{/if}

    {#if oauthProviders.length > 0}
      <div class="connect" data-mailbox-oauth>
        <p class="quiet small">{t('app.mail.oauth_lead')}</p>
        <div class="actions">
          {#each oauthProviders as provider (provider.key)}
            <Button tone="secondary" isBusy={isLeaving && oauthKind === undefined} busyLabel={t('app.mail.oauth_going')} onclick={() => choose(provider)}>
              {t(`app.mail.oauth_connect.${provider.key}`)}
            </Button>
          {/each}
          <Button tone="secondary" onclick={() => choose('custom')}>{t('app.mail.oauth_connect.custom')}</Button>
        </div>
        {#if oauthKind}
          {@const preset = oauthProviders.find((each) => each.key === oauthKind)}
          <div class="fields" data-mailbox-own-app={oauthKind}>
            {#if preset}
              <p class="quiet small">{t(preset.hint)} {t('app.mail.oauth_own_app')}</p>
            {:else}
              <p class="quiet small">{t('app.mail.oauth_custom_hint')}</p>
            {/if}
            <div class="redirect">
              <Input label={t('app.mail.oauth_redirect')} value={redirectUrl} readonly />
              <Button size="sm" tone="secondary" onclick={() => void copyRedirect()}>{t('app.mail.oauth_redirect_copy')}</Button>
            </div>
            {#if oauthKind === 'custom'}
              <Input label={t('app.mail.oauth_authorize_url')} placeholder="https://…/authorize" bind:value={authorizeUrl} />
              <Input label={t('app.mail.oauth_token_url')} placeholder="https://…/token" bind:value={tokenUrl} />
              <Input label={t('app.mail.oauth_scopes')} hint={t('app.mail.oauth_scopes_hint')} bind:value={scopes} />
              <Input label={t('app.mail.mailbox_host')} placeholder="imap.example.com" bind:value={oauthHost} />
              <Input label={t('app.mail.mailbox_port')} inputmode="numeric" bind:value={oauthPort} />
            {/if}
            <Input label={t('app.mail.oauth_client_id')} autocomplete="off" bind:value={clientId} />
            <Input label={t('app.mail.oauth_client_secret')} hint={t('app.mail.oauth_client_secret_hint')} type="password" autocomplete="new-password" bind:value={clientSecret} />
            <Input label={t('app.mail.mailbox_address')} hint={t('app.mail.oauth_address_hint')} type="email" autocomplete="off" bind:value={oauthAddress} />
            <div class="actions">
              <Button
                tone="primary"
                isBusy={isLeaving}
                busyLabel={t('app.mail.oauth_going')}
                disabledReason={clientId.trim() ? undefined : t('app.mail.oauth_fill_first')}
                onclick={leaveWithOwnApp}
              >{t('app.mail.oauth_go')}</Button>
              <Button tone="secondary" onclick={() => (oauthKind = undefined)}>{t('app.mail.oauth_cancel')}</Button>
            </div>
          </div>
        {/if}
      </div>
      <p class="quiet small">{t('app.mail.password_lead')}</p>
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

  .fields p { margin: 0; }

  .redirect { display: grid; grid-template-columns: 1fr auto; gap: var(--sp-100); align-items: end; }
</style>
