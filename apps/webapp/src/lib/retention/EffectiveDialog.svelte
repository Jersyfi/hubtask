<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *What is in force where* as a Dialog M from the head's ⋯: the rules that would actually act
  // inside one hub, inheritance included - a question asked of the rules, not a section of them.
  import { untrack } from 'svelte';

  import { Badge, Button, Dialog, Select, Stack } from '@hubtask/design-system/components';

  import { policies, type Policy } from '../data/policies.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { actionWord, kindWord } from './words.svelte.ts';

  interface Props {
    isOpen: boolean;
    hubs: readonly { value: string; label: string }[];
    /** Where a rule comes from, in words. */
    scopeWord: (policy: Policy) => string;
  }

  let { isOpen = $bindable(false), hubs, scopeWord }: Props = $props();

  let where = $state('');

  $effect(() => {
    const chosen = where;
    if (!chosen) return;
    return untrack(() => policies.open({ containerId: chosen, effective: true }));
  });

  const effective = $derived(where ? policies.of({ containerId: where, effective: true }) : []);
</script>

<Dialog
  size="m"
  title={t('app.retention.effective_title')}
  bind:isOpen
  dismissLabel={t('app.dismiss')}
  onClose={() => (where = '')}
>
  <Stack gap="150">
    <p class="quiet">{t('app.retention.effective_intro')}</p>
    <Select label={t('app.retention.effective_where')} bind:value={where} placeholder={t('app.retention.choose_container')} options={hubs} />
    {#if where}
      <ul class="lines">
        {#each effective as policy (policy.id)}
          <li class="line">
            <Badge tone={policy.in_force ? 'success' : 'neutral'}>
              {policy.in_force ? t('app.retention.in_force') : t('app.retention.overridden')}
            </Badge>
            <span>{kindWord(policy.data_kind)}</span>
            <span class="quiet">
              {actionWord(policy.action)} · {t('app.retention.after_days', { days: String(policy.retain_days) })} · {t('app.retention.comes_from', { scope: scopeWord(policy) })}
            </span>
          </li>
        {:else}
          <li class="quiet">{t('app.retention.no_effective')}</li>
        {/each}
      </ul>
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .lines { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
