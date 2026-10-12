<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a private hub says about its emergency accesses (ADR-0073 §4): to the owner who opened it,
  // while the hour runs, that its members have been told and when it closes; to its members, every
  // access of the last thirty days - who, when, until when and why. The hub's own record, so a
  // member who was not mailed reads it here.

  import { untrack } from 'svelte';

  import { Banner, Stack } from '@hubtask/design-system/components';
  import type { PrivateHubAccess, ResourceState } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { isRunning, privateHubs, recentAccesses } from '../data/privatehubs.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    hubId: string;
  }

  const { hubId }: Props = $props();

  let state = $state<ResourceState<readonly PrivateHubAccess[]>>({ status: 'idle' });

  $effect(() => {
    const id = hubId;
    return untrack(() => privateHubs.accessesOf(id, (next) => (state = next)));
  });

  const accesses = $derived(state.status === 'ready' && Array.isArray(state.data) ? recentAccesses(state.data) : []);
  const me = $derived(actor.account?.id);
  const mine = $derived(accesses.find((each) => each.opened_by === me && isRunning(each)));
  const told = $derived(accesses.filter((each) => each !== mine));

  $effect(() => {
    accounts.resolve(accesses.map((each) => each.opened_by));
  });

  const at = (iso: string) => formatDateTime(iso, messages.locale);
  const nameOf = (id: string | null | undefined) => accounts.nameOf(id) ?? t('app.private_hubs.somebody');
</script>

{#if mine || told.length > 0}
  <Stack gap="150">
    {#if mine}
      <div data-emergency-notice="opener">
        <Banner tone="warning">{t('app.private_hubs.notice_opener', { until: at(mine.ends_at) })}</Banner>
      </div>
    {/if}
    {#each told as access (access.id)}
      <div data-emergency-notice="member">
        <Banner tone="warning">
          {t('app.private_hubs.notice_member', {
            name: nameOf(access.opened_by), when: at(access.opened_at), until: at(access.ends_at), reason: access.reason,
          })}
        </Banner>
      </div>
    {/each}
  </Stack>
{/if}
