<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What reaches this workspace without a person at a keyboard: its service accounts with their
  // tokens, and the third-party apps people may allow (ux-inventory §E8). Two sibling list pages
  // under one strip (page-system §7.1), because both answer "which machine may act here, and on
  // whose behalf"; each draws its own head, whose primary adds to the list that is open.
  import AppsView from './AppsView.svelte';
  import ServiceAccountsView from './ServiceAccountsView.svelte';
  import { MACHINE_TABS } from '../lib/machines/machinespage.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the service accounts where it names none or an unknown one. */
    tab?: string;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onnavigate?: (path: string) => void;
  }

  const { tab, onnavigate }: Props = $props();

  const current = $derived(MACHINE_TABS.some((each) => each.id === tab) ? (tab as string) : 'service-accounts');
</script>

{#if current === 'apps'}
  <AppsView {onnavigate} />
{:else}
  <ServiceAccountsView {onnavigate} />
{/if}
