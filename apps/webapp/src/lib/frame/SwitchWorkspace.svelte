<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Switch workspace* (UC-ID-25 checks 2 and 4, the approved CD-09 preview): the workspaces this
  // browser remembers, in a dialog - a sheet from the bottom on a phone, as the account menu is.
  // Choosing one opens its address, where it signs in on its own; forgetting one takes it off the
  // list at once. The list is checked when it is shown, so an entry whose address changed says so.

  import { untrack } from 'svelte';

  import { Dialog, Drawer, Stack } from '@hubtask/design-system/components';

  import { deviceList } from '../data/devicelist.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import WorkspaceList from './WorkspaceList.svelte';

  interface Props {
    isOpen: boolean;
    /** A sheet from the bottom on `compact`; a dialog above it. */
    isSheet: boolean;
  }

  let { isOpen = $bindable(false), isSheet }: Props = $props();

  $effect(() => {
    if (isOpen) untrack(() => void deviceList.show());
  });
</script>

{#snippet content()}
  <Stack gap="200">
    <p class="lead">{t('app.workspaces.switch_lead')}</p>
    {#if deviceList.rows.length > 0}
      <WorkspaceList
        rows={deviceList.rows}
        here={location.hostname}
        onopen={(host) => deviceList.open(host)}
        onforget={(host) => void deviceList.forget(host)}
      />
    {/if}
    <p class="foot">{t('app.workspaces.switch_foot')}</p>
  </Stack>
{/snippet}

{#if isSheet}
  <Drawer bind:isOpen edge="block-end" title={t('app.workspaces.switch')} dismissLabel={t('app.dismiss')}>
    {@render content()}
  </Drawer>
{:else}
  <Dialog bind:isOpen title={t('app.workspaces.switch')} dismissLabel={t('app.dismiss')}>
    {@render content()}
  </Dialog>
{/if}

<style>
  .lead { margin: 0; color: var(--text-secondary); }

  .foot { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
