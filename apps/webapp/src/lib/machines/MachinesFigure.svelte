<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The head's figure (user-experience.md §7.10): what is connected - a service account without a
  // role among it, since its tokens are refused by everything - and the apps.
  import { apps } from '../data/apps.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { serviceAccounts } from '../data/serviceaccounts.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const TENANT = { scopeType: 'TENANT' } as const;

  const text = $derived.by(() => {
    const accounts = serviceAccounts.all;
    const registered = apps.all;
    if (accounts.length === 0 && registered.length === 0) return '';
    // Counted only once every membership is read: a page still out would count everyone.
    const holders = new Set(people.holders({}, TENANT).map((holder) => holder.accountId));
    const roleless = people.isCompleteAt(TENANT) ? accounts.filter((account) => !holders.has(account.id)).length : 0;
    return [
      t('app.machines.figure_accounts', { count: String(accounts.length) }),
      ...(roleless > 0 ? [t('app.machines.figure_roleless', { count: String(roleless) })] : []),
      t('app.machines.figure_apps', { count: String(registered.length) }),
    ].join(' · ');
  });
</script>

{#if text}<span class="figure" data-machines-figure>{text}</span>{/if}

<style>
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
</style>
