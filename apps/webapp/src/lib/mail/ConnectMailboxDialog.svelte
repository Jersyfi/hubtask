<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Connect a mailbox* as a Dialog L (page-system §9.3): first the provider, then the sign-in.
  //
  // **Through the provider's own sign-in where there is one** (OAuth with PKCE): where the operator
  // configured an app, choosing the provider goes there at once; otherwise, and for any other
  // provider, the administrator enters an app of their own, with the redirect URI to register. The
  // provider sends the browser back to `/administration/mail/oauth`, which the page hands on.
  //
  // **Or with a password**: Hubtask signs in once before it stores anything, so a mailbox that
  // cannot be read is said here, in the dialog, and not in a log an hour later. The password leaves
  // the component's state with the request that carried it.
  import { Button, Dialog, Input, Radio, Stack, type DialogAction } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import {
    authorizeMailbox,
    connectMailbox,
    type Mailbox,
    type MailboxOAuthProvider,
    type MailboxPreset,
  } from '../data/mailintake.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    presets: readonly MailboxPreset[];
    oauthProviders: readonly MailboxOAuthProvider[];
    redirectUrl: string;
    /** A refusal of the provider's answer, said where the person will look for it. */
    oauthFailure?: string;
    onconnected: (box: Mailbox) => void;
  }

  let { isOpen = $bindable(false), presets, oauthProviders, redirectUrl, oauthFailure, onconnected }: Props = $props();

  /** `oauth:<provider>` or `password:<preset>`; `oauth:custom` and `password:other` the free ones. */
  let chosenWay = $state('');
  // A way is always chosen, so *Continue* has somewhere to go: Gmail, as the form always began.
  const way = $derived(
    chosenWay ||
      (presets.some((each) => each.key === 'gmail')
        ? 'password:gmail'
        : oauthProviders[0] ? `oauth:${oauthProviders[0].key}` : `password:${presets[0]?.key ?? 'other'}`),
  );
  let step = $state<'provider' | 'sign-in'>('provider');

  const preset = $derived(way.startsWith('password:') ? way.slice('password:'.length) : '');
  const oauthKind = $derived(way.startsWith('oauth:') ? way.slice('oauth:'.length) : undefined);
  const chosenOAuth = $derived(oauthProviders.find((each) => each.key === oauthKind));
  /** The installation brings the app: choosing the provider is the whole of it. */
  const goesAtOnce = $derived(chosenOAuth?.installation === true);
  const isOther = $derived(preset === 'other');
  const chosen = $derived(presets.find((each) => each.key === preset));

  let protocol = $state<'IMAP' | 'JMAP'>('IMAP');
  let address = $state('');
  let password = $state('');
  let host = $state('');
  let port = $state('993');
  let sessionUrl = $state('');
  let username = $state('');
  let clientId = $state('');
  let clientSecret = $state('');
  let oauthAddress = $state('');
  let authorizeUrl = $state('');
  let tokenUrl = $state('');
  let scopes = $state('');
  let oauthHost = $state('');
  let oauthPort = $state('993');
  let isWorking = $state(false);
  let failure = $state<string | undefined>(undefined);

  const oauthWays = $derived([
    ...oauthProviders.map((each) => ({ value: `oauth:${each.key}`, label: t(`app.mail.oauth_connect.${each.key}`) })),
    ...(oauthProviders.length > 0 ? [{ value: 'oauth:custom', label: t('app.mail.oauth_connect.custom') }] : []),
  ]);
  const passwordWays = $derived([
    ...presets.map((each) => ({ value: `password:${each.key}`, label: t(`app.mail.provider.${each.key}`) })),
    { value: 'password:other', label: t('app.mail.provider.other') },
  ]);



  function reset(): void {
    step = 'provider';
    chosenWay = '';
    address = '';
    password = '';
    clientSecret = '';
    failure = undefined;
    isWorking = false;
  }

  async function leave(request: Parameters<typeof authorizeMailbox>[0]): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      window.location.assign(await authorizeMailbox(request));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      isWorking = false;
    }
  }

  function leaveWithOwnApp(): void {
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

  async function copyRedirect(): Promise<void> {
    try {
      await navigator.clipboard.writeText(redirectUrl);
      announcer.say(t('app.mail.oauth_redirect_copied'));
    } catch {
      /* The address stays on screen to copy by hand. */
    }
  }

  async function connect(): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      const connected = await connectMailbox({
        ...(isOther ? { protocol } : { preset }),
        address: address.trim(),
        secret: password,
        ...(isOther && protocol === 'IMAP' ? { host: host.trim(), port: Number(port) || 993 } : {}),
        ...(isOther && protocol === 'JMAP' ? { session_url: sessionUrl.trim() } : {}),
        ...(username.trim() ? { username: username.trim() } : {}),
      });
      onconnected(connected);
      announcer.say(t('app.mail.mailbox_connected', { address: connected.address }));
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
      password = '';
    }
  }

  const primary = $derived.by((): DialogAction => {
    if (step === 'provider') {
      return goesAtOnce
        ? { label: t('app.mail.oauth_go'), isBusy: isWorking, busyLabel: t('app.mail.oauth_going'), onclick: () => void leave({ provider: oauthKind ?? '' }) }
        : { label: t('app.mail.connect_next'), onclick: () => { step = 'sign-in'; } };
    }
    if (oauthKind) {
      return {
        label: t('app.mail.oauth_go'),
        isBusy: isWorking,
        busyLabel: t('app.mail.oauth_going'),
        disabledReason: clientId.trim() ? undefined : t('app.mail.oauth_fill_first'),
        onclick: leaveWithOwnApp,
      };
    }
    return {
      label: t('app.mail.mailbox_connect'),
      isBusy: isWorking,
      busyLabel: t('app.mail.connecting'),
      disabledReason: address.trim() && password ? undefined : t('app.mail.mailbox_fill_first'),
      onclick: () => void connect(),
    };
  });
</script>

<Dialog size="l" bind:isOpen title={t('app.mail.mailbox_add')} dismissLabel={t('app.mail.oauth_cancel')} onClose={reset} {primary}>
  <Stack gap="200">
    {#if failure ?? oauthFailure}<div data-mailbox-oauth-failure><FailureNote problem={failure ?? oauthFailure ?? ''} /></div>{/if}
    {#if step === 'provider'}
      <p class="quiet">{t('app.mail.mailboxes_hint')}</p>
      {#if oauthWays.length > 0}
        <div data-mailbox-oauth>
          <Radio label={t('app.mail.oauth_lead')} options={oauthWays} bind:value={() => way, (next) => (chosenWay = next)} />
        </div>
      {/if}
      <div data-mailbox-ways>
        <Radio label={oauthWays.length > 0 ? t('app.mail.connect_by_password') : t('app.mail.provider')} options={passwordWays} bind:value={() => way, (next) => (chosenWay = next)} />
      </div>
      {#if goesAtOnce}<p class="quiet">{t('app.mail.connect_installation')}</p>{/if}
    {:else if oauthKind}
      <div class="fields" data-mailbox-own-app={oauthKind}>
        {#if chosenOAuth}
          <p class="quiet">{t(chosenOAuth.hint)} {t('app.mail.oauth_own_app')}</p>
        {:else}
          <p class="quiet">{t('app.mail.oauth_custom_hint')}</p>
        {/if}
        <div class="redirect">
          <Input label={t('app.mail.oauth_redirect')} value={redirectUrl} readonly />
          <Button size="sm" tone="secondary" onclick={() => void copyRedirect()}>{t('app.mail.oauth_redirect_copy')}</Button>
        </div>
        {#if oauthKind === 'custom'}
          <div class="pair">
            <Input label={t('app.mail.oauth_authorize_url')} placeholder="https://…/authorize" bind:value={authorizeUrl} />
            <Input label={t('app.mail.oauth_token_url')} placeholder="https://…/token" bind:value={tokenUrl} />
          </div>
          <Input label={t('app.mail.oauth_scopes')} hint={t('app.mail.oauth_scopes_hint')} bind:value={scopes} />
          <div class="pair">
            <Input label={t('app.mail.mailbox_host')} placeholder="imap.example.com" bind:value={oauthHost} />
            <Input label={t('app.mail.mailbox_port')} inputmode="numeric" bind:value={oauthPort} />
          </div>
        {/if}
        <div class="pair">
          <Input label={t('app.mail.oauth_client_id')} autocomplete="off" bind:value={clientId} />
          <Input label={t('app.mail.oauth_client_secret')} hint={t('app.mail.oauth_client_secret_hint')} type="password" autocomplete="new-password" bind:value={clientSecret} />
        </div>
        <Input label={t('app.mail.mailbox_address')} hint={t('app.mail.oauth_address_hint')} type="email" autocomplete="off" bind:value={oauthAddress} />
      </div>
    {:else}
      <div class="fields" data-mailbox-connect>
        {#if chosen}<p class="quiet" data-mailbox-hint>{t(chosen.hint)}</p>{/if}
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
        <div class="pair">
          <Input label={t('app.mail.mailbox_address')} type="email" autocomplete="off" bind:value={address} />
          <Input
            label={isOther && protocol === 'JMAP' || preset === 'fastmail' ? t('app.mail.mailbox_token') : t('app.mail.mailbox_password')}
            type="password"
            autocomplete="new-password"
            bind:value={password}
          />
        </div>
        {#if isOther && protocol === 'IMAP'}
          <div class="pair">
            <Input label={t('app.mail.mailbox_host')} placeholder="imap.example.com" bind:value={host} />
            <Input label={t('app.mail.mailbox_port')} inputmode="numeric" bind:value={port} />
          </div>
        {/if}
        {#if isOther && protocol === 'JMAP'}
          <Input label={t('app.mail.mailbox_session_url')} placeholder="https://…/jmap/session" bind:value={sessionUrl} />
        {/if}
        {#if isOther}
          <Input label={t('app.mail.mailbox_username')} hint={t('app.mail.mailbox_username_hint')} bind:value={username} />
        {/if}
      </div>
    {/if}
  </Stack>
  {#snippet actions()}
    <!-- One way out: back to the providers from the sign-in, closed from the providers. -->
    <Button
      tone="subtle"
      onclick={() => {
        failure = undefined;
        if (step === 'sign-in') step = 'provider';
        else isOpen = false;
      }}
    >{step === 'sign-in' ? t('app.mail.connect_back') : t('app.mail.oauth_cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .fields { display: grid; gap: var(--sp-150); }

  /* Short fields share a line, so the dialog holds its fields without scrolling (page-system §5.1). */
  .pair { display: grid; grid-template-columns: repeat(auto-fit, minmax(calc(var(--sp-1000) * 2.5), 1fr)); gap: var(--sp-150); align-items: start; }

  .redirect { display: grid; grid-template-columns: 1fr auto; gap: var(--sp-100); align-items: end; }
</style>
