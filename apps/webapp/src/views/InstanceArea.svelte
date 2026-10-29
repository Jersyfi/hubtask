<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The instance area's screens (ADR-0070 §5), by route. One module, so the Android build can leave the whole area out and put the page that
  // names the web app in its place (ADR-0032, `platform/target.ts`).
  import InstanceAiView from './InstanceAiView.svelte';
  import InstanceEncryptionView from './InstanceEncryptionView.svelte';
  import InstanceJournalView from './InstanceJournalView.svelte';
  import InstanceOperatorsView from './InstanceOperatorsView.svelte';
  import InstanceOverviewView from './InstanceOverviewView.svelte';
  import InstancePlanView from './InstancePlanView.svelte';
  import InstancePlansView from './InstancePlansView.svelte';
  import InstancePlatformView from './InstancePlatformView.svelte';
  import InstanceProvidersView from './InstanceProvidersView.svelte';
  import InstanceSettingsView from './InstanceSettingsView.svelte';
  import InstanceTargetsView from './InstanceTargetsView.svelte';
  import InstanceWorkspacesView from './InstanceWorkspacesView.svelte';
  import type { Resolution } from '../lib/router.ts';

  interface Props {
    route: Resolution;
    onnavigate: (path: string) => void;
    onreplace: (path: string) => void;
  }

  const { route, onnavigate, onreplace }: Props = $props();
</script>

{#if route.name === 'instance'}
  <!-- The level above the workspaces (ADR-0070 §5). Its own area, and its own door: every screen
       here is drawn only while the session is raised, and the hour's end returns the reader to
       the application rather than leaving them looking at numbers nothing will refresh. -->
  <InstanceOverviewView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-workspaces'}
  <InstanceWorkspacesView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-plans'}
  <InstancePlansView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-plan'}
  {#key route.params.name}
    <InstancePlanView name={route.params.name ?? ''} onnavigate={(path) => onnavigate(path)} />
  {/key}
{:else if route.name === 'instance-settings'}
  <InstanceSettingsView tab={route.query['tab']} onnavigate={(path) => onnavigate(path)} onreplace={(path) => onreplace(path)} />
{:else if route.name === 'instance-providers'}
  <InstanceProvidersView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-ai'}
  <InstanceAiView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-targets'}
  <InstanceTargetsView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-platform'}
  <InstancePlatformView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-encryption'}
  <InstanceEncryptionView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-operators'}
  <InstanceOperatorsView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'instance-journal'}
  <InstanceJournalView onnavigate={(path) => onnavigate(path)} />
{/if}
