<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's own address in multi mode, signed out (UC-ID-25 checks 3, 4 and 6; the
  // approved CD-09 preview): the workspaces this browser remembers, or - with none - where a
  // workspace is reached. **No field**: the address takes no short name and no host (Decision 7),
  // because a list is what was decided, not a way to reach a workspace one was not invited to.
  //
  // Also the list's home for the redirects there and back: a sign-in's entry kept, a workspace
  // forgotten from its menu, one opened from another's. While one is carried out nothing is drawn.

  import { onMount } from 'svelte';

  import { deviceList } from '../lib/data/devicelist.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import SignInCard from '../lib/signin/SignInCard.svelte';
  import WorkspaceList from '../lib/frame/WorkspaceList.svelte';

  /** Whether the list is drawn: not while a redirect is carried out, and not before it was read. */
  let isShown = $state(false);

  onMount(() => {
    void (async () => {
      if (await deviceList.settle()) return;
      await deviceList.show();
      isShown = true;
    })();
  });
</script>

{#if isShown}
  {#if deviceList.rows.length > 0}
    <SignInCard title={t('app.workspaces.title')} lead={t('app.workspaces.lead')} isInstallation>
      <WorkspaceList
        rows={deviceList.rows}
        isHome
        onopen={(host) => deviceList.open(host)}
        onforget={(host) => void deviceList.forget(host)}
      />
      <p class="foot">{t('app.workspaces.foot')}</p>
    </SignInCard>
  {:else}
    <SignInCard title={t('app.workspaces.none_title')} lead={t('app.workspaces.none_lead')} isInstallation>
      {#snippet children()}{/snippet}
    </SignInCard>
  {/if}
{/if}

<style>
  .foot { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
