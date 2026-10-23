<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What reaches this workspace without a person at a keyboard: its service accounts with their
  // tokens, and the third-party apps people may allow (ux-inventory §E8). One screen with two tabs,
  // because both answer "which machine may act here, and on whose behalf".
  import { Stack, PageHeader, Tabs } from '@hubtask/design-system/components';

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
  />

  <Tabs
    label={t('app.machines.title')}
    selected={current}
    onselect={select}
    tabs={TABS.map((each) => ({ id: each.id, label: t(each.code) }))}
  >
    {#if current === 'apps'}
      <AppsView />
    {:else}
      <ServiceAccountsView />
    {/if}
  </Tabs>
</Stack>
