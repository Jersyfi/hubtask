<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installed apps' workspaces (UC-ID-25 check 7, UC-SYN-07 check 4, the approved
  // preview): *Switch workspace* with the workspaces the app holds, grouped under each
  // installation's host - a sheet from the bottom on a phone, as the browser's is -, *Add a workspace* by
  // address, and *Forget this workspace* with its confirmation. One dialog at a time: choosing *Add*
  // or *Forget* in the list closes it first.
  //
  // *Open* starts the window on that workspace from its own copy, with no browser; *Sign in* goes
  // through the browser (ADR-0096) - in *Add a workspace*'s dialog while the window is signed in to
  // another one, which stays open behind it until the new one is signed in.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Drawer, Input, Spinner, Stack } from '@hubtask/design-system/components';

  import { appWorkspaces } from '../data/appworkspaces.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { platform, type AppWorkspace } from '../platform/index.ts';
  import { addressOf } from '../signin/appSignIn.ts';
  import { appSignIn } from '../signin/appsignin.svelte.ts';
  import WorkspaceList from './WorkspaceList.svelte';

  interface Props {
    isSwitchOpen: boolean;
    isAddOpen: boolean;
    /** A sheet from the bottom on `compact`; a dialog above it. */
    isSheet: boolean;
    /** Whether this window is signed in: otherwise a sign-in is the window's own card. */
    isSignedIn: boolean;
  }

  let { isSwitchOpen = $bindable(false), isAddOpen = $bindable(false), isSheet, isSignedIn }: Props = $props();

  /** The workspace whose forgetting is being confirmed, and the changes that would go with it. */
  let forgetting = $state<{ readonly workspace: AppWorkspace; readonly waiting: number } | undefined>(undefined);
  let isForgetting = $state(false);
  let typed = $state('');
  /** The workspace already in the list at the typed address: the field's own refusal. */
  let already = $state<AppWorkspace | undefined>(undefined);

  const here = $derived(isSignedIn ? appWorkspaces.current?.host : undefined);
  const isWaiting = $derived(isAddOpen && appSignIn.phase === 'waiting');
  const waitingHost = $derived(appSignIn.address?.host ?? '');
  const fieldError = $derived(
    already !== undefined
      ? t('app.app_workspaces.already', { name: already.name ?? already.host, host: already.host })
      : appSignIn.refusalText,
  );

  $effect(() => {
    if (isSwitchOpen) untrack(() => void appWorkspaces.show());
  });

  function rowsOf(workspaces: readonly (AppWorkspace & { readonly isSignedIn: boolean })[]) {
    return workspaces.map((each) => ({
      entry: each.server,
      host: each.host,
      name: each.name ?? each.host,
      isSignedOut: !each.isSignedIn,
    }));
  }

  const serverAt = (host: string) => appWorkspaces.heldAt(host)?.server ?? '';

  function openAdd(): void {
    isSwitchOpen = false;
    typed = '';
    already = undefined;
    appSignIn.reset();
    isAddOpen = true;
  }

  function signIn(host: string): void {
    isSwitchOpen = false;
    if (isSignedIn) {
      appSignIn.reset();
      typed = host;
      already = undefined;
      isAddOpen = true;
    }
    void appSignIn.continue(host);
  }

  async function askForget(host: string): Promise<void> {
    const workspace = appWorkspaces.heldAt(host);
    if (workspace === undefined) return;
    isSwitchOpen = false;
    forgetting = { workspace, waiting: await appWorkspaces.waiting(workspace.server) };
  }

  async function forget(): Promise<void> {
    const asked = forgetting;
    if (asked === undefined) return;
    isForgetting = true;
    try {
      await appWorkspaces.forget(asked.workspace.server);
    } finally {
      isForgetting = false;
      forgetting = undefined;
    }
  }

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    const address = addressOf(typed);
    const held = address === undefined ? undefined : appWorkspaces.heldAt(address.host);
    already = held;
    if (held !== undefined) {
      document.querySelector<HTMLInputElement>('#new-workspace input')?.focus();
      return;
    }
    void appSignIn.continue(typed).then(() => {
      if (appSignIn.phase === 'ask') document.querySelector<HTMLInputElement>('#new-workspace input')?.focus();
    });
  }

  function closeAdd(): void {
    appSignIn.stop();
    isAddOpen = false;
  }

  function openInBrowser(): void {
    const address = appSignIn.address;
    if (address !== undefined) void platform.browserSignIn?.openInBrowser(`${address.origin}/`);
  }
</script>

{#snippet list()}
  <Stack gap="200">
    <p class="lead">{t('app.app_workspaces.switch_lead')}</p>
    {#each appWorkspaces.groups as group (group.installation)}
      <section class="group" aria-label={t('app.app_workspaces.group', { host: group.installation })}>
        <p class="cap">{group.installation}</p>
        <WorkspaceList
          rows={rowsOf(group.workspaces)}
          {here}
          forgetLabel={t('app.app_workspaces.forget')}
          onopen={(host) => appWorkspaces.open(serverAt(host))}
          onsignin={signIn}
          onforget={(host) => void askForget(host)}
        />
      </section>
    {/each}
    <div><Button tone="subtle" icon="plus" onclick={openAdd}>{t('app.app_workspaces.add')}</Button></div>
  </Stack>
{/snippet}

{#snippet addActions()}
  <Button tone="subtle" onclick={closeAdd}>{t('app.app_workspaces.cancel')}</Button>
  <Button type="submit" form="new-workspace" tone="primary" isBusy={appSignIn.phase === 'checking'} busyLabel={t('app.app_sign_in.checking')}>
    {t('app.app_sign_in.continue')}
  </Button>
{/snippet}

{#if isSwitchOpen}
  {#if isSheet}
    <Drawer bind:isOpen={isSwitchOpen} edge="block-end" title={t('app.workspaces.switch')} dismissLabel={t('app.dismiss')}>
      {@render list()}
    </Drawer>
  {:else}
    <Dialog bind:isOpen={isSwitchOpen} title={t('app.workspaces.switch')} dismissLabel={t('app.dismiss')}>
      {@render list()}
    </Dialog>
  {/if}
{/if}

{#if isAddOpen}
  <!-- One dialog for the address and the wait: a second one in its place would close the first. -->
  <Dialog
    isOpen
    title={isWaiting ? t('app.app_sign_in.waiting_title') : t('app.app_workspaces.add')}
    dismissLabel={t('app.dismiss')}
    onClose={closeAdd}
    actions={isWaiting ? undefined : addActions}
  >
    {#if isWaiting}
      <Stack gap="200">
        <p class="prose">{t('app.app_sign_in.waiting_lead', { host: waitingHost })}</p>
        {#if appSignIn.isUnfinished}<Banner tone="warning">{t('app.app_sign_in.unfinished')}</Banner>{/if}
        <p class="row prose"><Spinner label={t('app.app_sign_in.waiting')} /> <span>{t('app.app_sign_in.waiting')}</span></p>
        <div class="row">
          <Button tone="secondary" icon="external-link" onclick={() => void appSignIn.again()}>{t('app.app_sign_in.open_again')}</Button>
          <Button tone="subtle" onclick={() => appSignIn.stop()}>{t('app.app_sign_in.another_address')}</Button>
        </div>
      </Stack>
    {:else}
      <form id="new-workspace" onsubmit={submit}>
        <Stack gap="200">
          <p class="prose">{t('app.app_workspaces.add_lead')}</p>
          <Input
            label={t('app.app_sign_in.address_label')}
            bind:value={typed}
            error={fieldError}
            face="code"
            autocomplete="url"
            spellcheck={false}
            oninput={() => (already = undefined)}
          />
          {#if appSignIn.refusal?.kind === 'older'}
            <div>
              <Button size="sm" tone="secondary" icon="external-link" onclick={openInBrowser}>
                {t('app.app_sign_in.open_in_browser', { host: waitingHost })}
              </Button>
            </div>
          {/if}
        </Stack>
      </form>
    {/if}
  </Dialog>
{/if}

{#if forgetting}
  {@const name = forgetting.workspace.name ?? forgetting.workspace.host}
  <Dialog isOpen title={t('app.app_workspaces.forget_title', { name })} dismissLabel={t('app.dismiss')} onClose={() => (forgetting = undefined)}>
    <Stack gap="200">
      <p class="prose">{t('app.app_workspaces.forget_body', { name })}</p>
      {#if forgetting.waiting > 0}
        <Banner tone="warning" title={t('app.app_workspaces.waiting_title', { count: forgetting.waiting })}>
          {t('app.app_workspaces.waiting_body')}
        </Banner>
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (forgetting = undefined)}>{t('app.app_workspaces.cancel')}</Button>
      <Button tone="danger" isBusy={isForgetting} busyLabel={t('app.app_workspaces.forgetting')} onclick={() => void forget()}>
        {t('app.app_workspaces.forget')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .lead { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .prose { margin: 0; color: var(--text-secondary); }

  .group { display: flex; flex-direction: column; gap: var(--sp-200); }

  .cap { margin: 0 0 var(--sp-050); font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-subtle); }

  .row { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
