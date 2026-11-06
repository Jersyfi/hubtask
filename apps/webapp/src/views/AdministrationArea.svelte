<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The administration area's screens (ADR-0063 decision 7), by route. One module, so the Android
  // build can leave the whole area out and put the page that names the web app in its place
  // (ADR-0032, `platform/target.ts`); loaded on its first visit, like the views beside it.
  import AdministrationOverviewView from './AdministrationOverviewView.svelte';
  import AiSettingsView from './AiSettingsView.svelte';
  import AuditView from './AuditView.svelte';
  import BackupView from './BackupView.svelte';
  import ChangesView from './ChangesView.svelte';
  import EntryTypesView from './EntryTypesView.svelte';
  import LinkTypesView from './LinkTypesView.svelte';
  import MachinesView from './MachinesView.svelte';
  import MailView from './MailView.svelte';
  import ModulesView from './ModulesView.svelte';
  import PeopleView from './PeopleView.svelte';
  import PresetsView from './PresetsView.svelte';
  import PrivacyView from './PrivacyView.svelte';
  import PrivateHubsView from './PrivateHubsView.svelte';
  import QuotasView from './QuotasView.svelte';
  import RestoreView from './RestoreView.svelte';
  import RetentionView from './RetentionView.svelte';
  import RunsView from './RunsView.svelte';
  import SignInSettingsView from './SignInSettingsView.svelte';
  import VoiceView from './VoiceView.svelte';
  import WebhooksView from './WebhooksView.svelte';
  import WorkflowsView from './WorkflowsView.svelte';
  import WorkspaceAddressView from './WorkspaceAddressView.svelte';
  import WorkspaceSettingsView from './WorkspaceSettingsView.svelte';
  import LazyView from '../lib/frame/LazyView.svelte';
  import type { Resolution } from '../lib/router.ts';

  // The rules' screens are the profile's too (a person's own rules), which the Android build
  // ships: loaded on demand rather than bound into this area's module.
  const loadRules = () => import('./RulesView.svelte');
  const loadRuleEditor = () => import('./RuleEditorView.svelte');

  interface Props {
    route: Resolution;
    onnavigate: (path: string) => void;
    onreplace: (path: string) => void;
  }

  const { route, onnavigate, onreplace }: Props = $props();
</script>

{#if route.name === 'administration'}
  <AdministrationOverviewView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'workspace-settings'}
  <WorkspaceSettingsView />
{:else if route.name === 'workspace-address'}
  <WorkspaceAddressView />
{:else if route.name === 'presets'}
  <PresetsView template={route.query['template']} onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'people'}
  <PeopleView tab={route.query['tab']} person={route.query['person']} onnavigate={(path) => onreplace(path)} />
{:else if route.name === 'machines'}
  <MachinesView tab={route.query['tab']} onnavigate={(path) => onreplace(path)} />
{:else if route.name === 'rules'}
  <LazyView load={loadRules} props={{ tab: route.query['tab'], onnavigate: (path: string) => onnavigate(path) }} />
{:else if route.name === 'rule-new'}
  <LazyView load={loadRuleEditor} props={{ id: 'new', workflow: route.query['workflow'], step: route.query['step'], onnavigate: (path: string) => onnavigate(path) }} />
{:else if route.name === 'rule'}
  {#key route.params.id}
    <LazyView load={loadRuleEditor} props={{ id: route.params.id ?? '', onnavigate: (path: string) => onnavigate(path) }} />
  {/key}
{:else if route.name === 'runs'}
  <RunsView />
{:else if route.name === 'webhooks'}
  <WebhooksView />
{:else if route.name === 'quotas'}
  <QuotasView />
{:else if route.name === 'modules'}
  <ModulesView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'entry-types'}
  <EntryTypesView />
{:else if route.name === 'link-types'}
  <LinkTypesView />
{:else if route.name === 'changes'}
  <ChangesView />
{:else if route.name === 'workflows'}
  <WorkflowsView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'private-hubs'}
  <PrivateHubsView onnavigate={(path) => onnavigate(path)} />
{:else if route.name === 'backup'}
  <BackupView />
{:else if route.name === 'retention'}
  <RetentionView />
{:else if route.name === 'restore'}
  <RestoreView />
{:else if route.name === 'audit'}
  <AuditView />
{:else if route.name === 'privacy'}
  <PrivacyView />
{:else if route.name === 'sign-in-settings'}
  <SignInSettingsView tab={route.query['tab']} onnavigate={(path) => onreplace(path)} />
{:else if route.name === 'voice'}
  <VoiceView />
{:else if route.name === 'mail' || route.name === 'mail-oauth'}
  <MailView providerAnswer={route.name === 'mail-oauth' ? route.query : undefined} onanswered={() => onreplace('/administration/mail')} />
{:else if route.name === 'legal'}
  <SignInSettingsView part="legal" />
{:else if route.name === 'ai'}
  <AiSettingsView tab={route.query['tab']} onnavigate={(path) => onreplace(path)} />
{/if}
