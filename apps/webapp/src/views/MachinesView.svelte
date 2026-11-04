<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What reaches this workspace without a person at a keyboard: its service accounts with their
  // tokens, and the third-party apps people may allow (ux-inventory §E8). One screen with two tabs,
  // because both answer "which machine may act here, and on whose behalf".
  //
  // **The head counts and adds** (user-experience.md §7.10): the figure says what is connected -
  // a service account without a role among it, since its tokens are refused by everything - and
  // the primary adds to the tab that is open.
  import { untrack } from 'svelte';

  import { Stack, PageHeader, Tabs } from '@hubtask/design-system/components';

  import { apps } from '../lib/data/apps.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import AppsView from './AppsView.svelte';
  import ServiceAccountsView from './ServiceAccountsView.svelte';

  interface Props {
    /** The tab the address names (`?tab=`); the service accounts where it names none or an unknown one. */
    tab?: string;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onnavigate?: (path: string) => void;
  }

  let { tab, onnavigate }: Props = $props();

  const TABS = [
    { id: 'service-accounts', code: 'app.service_accounts.title' },
    { id: 'apps', code: 'app.apps.title' },
  ] as const;
  const current = $derived(TABS.some((each) => each.id === tab) ? (tab as string) : 'service-accounts');

  const TENANT = { scopeType: 'TENANT' } as const;

  let isAddingAccount = $state(false);
  let isAddingApp = $state(false);

  $effect(() => untrack(() => serviceAccounts.open()));
  $effect(() => untrack(() => apps.open()));
  $effect(() => untrack(() => people.openScope(TENANT)));

  const mayManage = $derived(holdsHere('MANAGE_MEMBERS') === true);

  const figureText = $derived.by(() => {
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

  const primary = $derived(
    !mayManage
      ? undefined
      : current === 'apps'
        ? { label: t('app.apps.new_title'), icon: 'plus' as const, onclick: () => (isAddingApp = true) }
        : { label: t('app.service_accounts.new_title'), icon: 'plus' as const, onclick: () => (isAddingAccount = true) },
  );

  function select(next: string): void {
    onnavigate?.(next === 'service-accounts' ? '/administration/machines' : `/administration/machines?tab=${next}`);
  }

  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.machines.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.machines.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'machines', label: t('app.machines.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
    {primary}
  >
    {#snippet figure()}{#if figureText}<span class="figure" data-machines-figure>{figureText}</span>{/if}{/snippet}
  </PageHeader>

  <Tabs
    label={t('app.machines.title')}
    selected={current}
    onselect={select}
    tabs={TABS.map((each) => ({ id: each.id, label: t(each.code) }))}
  >
    {#if current === 'apps'}
      <AppsView bind:isAdding={isAddingApp} />
    {:else}
      <ServiceAccountsView bind:isAdding={isAddingAccount} />
    {/if}
  </Tabs>
</Stack>

<style>
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
</style>
