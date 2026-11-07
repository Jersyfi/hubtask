<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's SLA policies in its settings, beside its working hours (UC-SVC-02 check 13): the
  // workspace's shared policies read-only with *Make a copy here*, and the hub's own, which
  // measure the hub's entries before the workspace's. Copying needs STRUCTURE on the hub.

  import { untrack } from 'svelte';
  import { Badge, Button, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { SlaPolicyList } from '@hubtask/sync-engine';

  import { desk, policiesPath } from '../data/desk.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { policiesAtHub, policyCopyKey } from './desk.ts';

  interface Props {
    hubId: string;
    mayShape: boolean;
  }

  let { hubId, mayShape }: Props = $props();

  const policies = untrack(() => resource<SlaPolicyList>(policiesPath));
  const all = $derived(policies.state.status === 'ready' ? policies.state.data.data : undefined);
  const atHub = $derived(all ? policiesAtHub(all, hubId) : undefined);

  let copying = $state<string | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function copyHere(key: string) {
    if (copying !== undefined || !all) return;
    copying = key;
    refusal = undefined;
    try {
      await desk.copyPolicyHere(key, hubId, policyCopyKey(all, key));
      toast.show({ text: t('app.desk.hub_policy_copied'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      copying = undefined;
    }
  }
</script>

<Stack gap="150">
  <p class="quiet">{t('app.desk.hub_policies_hint')}</p>
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  {#if !atHub}<Skeleton lines={2} />{:else}
    <ul class="rows" data-hub-policies>
      {#each atHub.shared as policy (policy.key)}
        <li>
          <span class="name">{policy.name}</span>
          <Badge size="sm">{t('app.desk.hub_workspace')}</Badge>
          {#if mayShape}<Button size="sm" tone="subtle" isBusy={copying === policy.key} busyLabel={t('app.structure.hub_types_copying')} onclick={() => void copyHere(policy.key)}>
            {t('app.structure.hub_types_copy')}
          </Button>{/if}
        </li>
      {/each}
      {#each atHub.own as policy (policy.key)}
        <li>
          <span class="name">{policy.name}</span>
          <Badge size="sm" tone="info">{policy.copied_from ? t('app.desk.hub_policies_copied', { from: policy.copied_from }) : t('app.desk.hub_own')}</Badge>
        </li>
      {/each}
    </ul>
  {/if}
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .rows li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }
  .name { margin-inline-end: auto; }
</style>
