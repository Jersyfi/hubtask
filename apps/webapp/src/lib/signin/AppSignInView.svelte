<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installed apps' sign-in (ADR-0096; UC-SYN-07 check 3): which workspace, its refusals at the
  // field, the wait while the system's browser signs in, and the two signed-out cards. The browser's
  // sign-in card offers every way in the workspace allows, a passkey included, at the workspace's
  // own host - which is why nothing of it is drawn here.

  import { Banner, Button, Input, Spinner, Stack } from '@hubtask/design-system/components';

  import { session } from '../session.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import AppWorkspaces from '../frame/AppWorkspaces.svelte';
  import { appWorkspaces } from '../data/appworkspaces.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import SignInCard from './SignInCard.svelte';
  import { appSignIn } from './appsignin.svelte.ts';

  const known = platform.browserSignIn?.workspace;
  let typed = $state('');
  let isSwitchOpen = $state(false);
  let isAddOpen = $state(false);

  /**
   * *Switch workspace* on the signed-out card: the app's list where it holds another workspace, to
   * open or sign in to; *Which workspace?* where it holds none.
   */
  function switchWorkspace(): void {
    if ((platform.appWorkspaces?.list().length ?? 0) > 1) isSwitchOpen = true;
    else appSignIn.askForAddress();
  }

  const showsSignedOut = $derived(known !== undefined && !appSignIn.isAsking && appSignIn.phase !== 'waiting');
  const host = $derived(appSignIn.address?.host ?? '');
  const name = known?.name ?? known?.host ?? '';

  const error = $derived(appSignIn.refusalText);

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    void appSignIn.continue(typed).then(() => {
      // Focus back to the field with its refusal, as every refusal at a field does.
      if (appSignIn.phase === 'ask') document.querySelector<HTMLInputElement>('#main input')?.focus();
    });
  }

  function openInBrowser(): void {
    const address = appSignIn.address;
    if (address !== undefined) void platform.browserSignIn?.openInBrowser(`${address.origin}/`);
  }
</script>

{#if appSignIn.phase === 'waiting'}
  <SignInCard
    title={t('app.app_sign_in.waiting_title')}
    lead={t('app.app_sign_in.waiting_lead', { host })}
    hostLine={t('app.sign_in.to_workspace', { host })}
  >
    {#snippet notice()}
      {#if appSignIn.isUnfinished}<Banner tone="warning">{t('app.app_sign_in.unfinished')}</Banner>{/if}
    {/snippet}
    <p class="row quiet"><Spinner label={t('app.app_sign_in.waiting')} /> <span>{t('app.app_sign_in.waiting')}</span></p>
    <div class="row">
      <Button tone="secondary" icon="external-link" onclick={() => void appSignIn.again()}>{t('app.app_sign_in.open_again')}</Button>
      <Button tone="subtle" onclick={() => appSignIn.cancel()}>{t('app.app_sign_in.another_address')}</Button>
    </div>
  </SignInCard>
{:else if showsSignedOut && known}
  <SignInCard
    title={t(session.endedNotice ? 'app.app_sign_in.ended_title' : 'app.app_sign_in.signed_out_title', { name })}
    lead={t(session.endedNotice ? 'app.app_sign_in.ended_lead' : 'app.app_sign_in.signed_out_lead', { name })}
    hostLine={known.host}
  >
    {#if error}<Banner tone="danger">{error}</Banner>{/if}
    <!-- Each a step of the card's own rhythm, as drawn: two answers, not a group of two. -->
    <Button
      tone="primary"
      isFull
      isBusy={appSignIn.phase === 'checking'}
      busyLabel={t('app.app_sign_in.checking')}
      onclick={() => void appSignIn.continue(known.host)}
    >
      {t('app.app_sign_in.sign_in_again')}
    </Button>
    <Button tone="subtle" isFull onclick={switchWorkspace}>{t('app.app_sign_in.switch_workspace')}</Button>
  </SignInCard>
{:else}
  <SignInCard title={t('app.app_sign_in.which_title')} lead={t('app.app_sign_in.which_lead')} hostLine={null}>
    <form onsubmit={submit}>
      <Stack gap="250">
        <Stack gap="100">
          <Input
            label={t('app.app_sign_in.address_label')}
            bind:value={typed}
            hint={t('app.app_sign_in.address_hint')}
            {error}
            face="code"
            autocomplete="url"
            spellcheck={false}
          />
          {#if appSignIn.refusal?.kind === 'older'}
            <div>
              <Button size="sm" tone="secondary" icon="external-link" onclick={openInBrowser}>
                {t('app.app_sign_in.open_in_browser', { host })}
              </Button>
            </div>
          {/if}
        </Stack>
        <Button type="submit" tone="primary" isFull isBusy={appSignIn.phase === 'checking'} busyLabel={t('app.app_sign_in.checking')}>
          {t('app.app_sign_in.continue')}
        </Button>
        <p class="quiet small">{t('app.app_sign_in.keystore_note')}</p>
      </Stack>
    </form>
  </SignInCard>
{/if}

{#if appWorkspaces.isHeld}
  <AppWorkspaces bind:isSwitchOpen bind:isAddOpen isSheet={viewport.isCompact} isSignedIn={false} />
{/if}

<style>
  .row { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }
</style>
