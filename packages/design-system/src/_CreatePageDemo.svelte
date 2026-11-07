<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import CreatePage from './CreatePage.svelte';
  import FormLayout from './FormLayout.svelte';
  import FormSection from './FormSection.svelte';
  import Input from './Input.svelte';
  import Select from './Select.svelte';
  import SettingRow from './SettingRow.svelte';
  import Switch from './Switch.svelte';
  import Refused from './_RefusedDemo.svelte';

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' } = $props();
  const isEmpty = $derived(content === 'empty');

  const scopes = [
    { id: 'entries', label: 'Entries, hubs and collections', help: 'Read · create and change' },
    { id: 'people', label: 'People and roles', help: 'See who is here · invite and change' },
    { id: 'audit', label: 'Audit and privacy', help: 'Read the trail · export · requests' },
    { id: 'automation', label: 'Automation and agents', help: 'Rules, agents and their ledger' },
  ];
</script>

<CreatePage
  head={{
    title: 'New access token',
    subtitle: 'A token lets a script act as you, within what you may do and the scopes you give it.',
    breadcrumb: { trail: [{ id: 'you', label: 'Your settings', href: '#you' }, { id: 'tokens', label: 'Access tokens', href: '#tokens' }, { id: 'new', label: 'New' }], label: 'Where you are', expandLabel: 'Show every level' },
    primary: { label: 'Create token', onclick: () => {}, disabledReason: isEmpty ? 'Say what it is for first.' : undefined },
    secondary: [{ label: 'Cancel', onclick: () => {} }],
  }}
  close={{ label: 'Cancel', onclose: () => {} }}
  refused={content === 'refused' ? refused : undefined}
>
  <FormLayout place="page">
    <FormSection title="About it">
      <SettingRow label="What it is for" help="What you will read in a year." fieldWidth="md" isRequired>
        {#snippet control(nameId)}<Input label="What it is for" isLabelHidden aria-labelledby={nameId} value={isEmpty ? '' : 'Nightly export'} />{/snippet}
      </SettingRow>
      <SettingRow label="Expires" help="At most a year away." fieldWidth="sm" isRequired>
        {#snippet control(nameId)}<Select label="Expires" isLabelHidden aria-labelledby={nameId} options={[{ value: '90', label: 'In 90 days' }, { value: '365', label: 'In a year' }]} value="90" />{/snippet}
      </SettingRow>
    </FormSection>
    <FormSection title="What it may do" note={isEmpty ? 'none chosen' : '3 chosen'}>
      {#each scopes as scope (scope.id)}
        <SettingRow label={scope.label} help={scope.help} fieldWidth="sm">
          {#snippet control()}<Switch label={`${scope.label}: may change`} isLabelHidden checked={!isEmpty && scope.id !== 'automation'} />{/snippet}
        </SettingRow>
      {/each}
    </FormSection>
    <FormSection title="The configuration — 31 scopes by part" description="For tools that apply a configuration" isFolded>
      <SettingRow label="Read the configuration" fieldWidth="sm">
        {#snippet control()}<Switch label="Read the configuration" isLabelHidden />{/snippet}
      </SettingRow>
    </FormSection>
  </FormLayout>
</CreatePage>

{#snippet refused()}<Refused what="how to create a token" />{/snippet}
