<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import CheckboxGroup from './CheckboxGroup.svelte';
  import Dialog from './Dialog.svelte';
  import FormLayout from './FormLayout.svelte';
  import FormSection from './FormSection.svelte';
  import Input from './Input.svelte';
  import Select from './Select.svelte';
  import SettingRow from './SettingRow.svelte';
  import Switch from './Switch.svelte';

  const { place = 'dialog' }: { place?: 'dialog' | 'page' } = $props();

  let isOpen = $state(false);
  const scopes = [
    { id: 'entries', label: 'Entries, hubs and collections', help: 'Read · create and change' },
    { id: 'people', label: 'People and roles', help: 'See who is here · invite and change' },
    { id: 'audit', label: 'Audit and privacy', help: 'Read the trail · export · requests' },
    { id: 'automation', label: 'Automation and agents', help: 'Rules, agents and their ledger' },
  ];
</script>

{#if place === 'page'}
  <FormLayout place="page">
    <FormSection title="About it">
      <SettingRow label="What it is for" help="What you will read in a year." fieldWidth="md" isRequired>
        {#snippet control(nameId)}<Input label="What it is for" isLabelHidden aria-labelledby={nameId} value="Nightly export" />{/snippet}
      </SettingRow>
      <SettingRow label="Expires" help="At most a year away." fieldWidth="sm" isRequired>
        {#snippet control(nameId)}<Select label="Expires" isLabelHidden aria-labelledby={nameId} options={[{ value: '90', label: 'In 90 days' }]} value="90" />{/snippet}
      </SettingRow>
    </FormSection>
    <FormSection title="What it may do" note="3 chosen">
      {#each scopes as scope (scope.id)}
        <SettingRow label={scope.label} help={scope.help} fieldWidth="sm">
          {#snippet control()}<Switch label={`${scope.label}: write`} checked={scope.id !== 'automation'} />{/snippet}
        </SettingRow>
      {/each}
    </FormSection>
    <FormSection title="The configuration" description="31 scopes by part, for tools that apply a configuration" isFolded>
      <SettingRow label="Read the configuration" fieldWidth="sm">
        {#snippet control()}<Switch label="Read the configuration" isLabelHidden />{/snippet}
      </SettingRow>
    </FormSection>
  </FormLayout>
{:else}
  <Button tone="primary" onclick={() => (isOpen = true)}>Edit step “Finish”</Button>
  <Dialog bind:isOpen size="l" title="Step “Finish”" dismissLabel="Close" primary={{ label: 'Done', onclick: () => (isOpen = false) }}>
    <FormLayout place="dialog">
      <FormSection title="The step">
        <Input label="Name" value="Finish" isRequired />
        <Select label="Leads to" options={[{ value: 'done', label: 'Done' }]} value="done" />
        <CheckboxGroup label="Leaves from" hint="None chosen: from any other status." options={[{ value: 'progress', label: 'In progress' }, { value: 'todo', label: 'To do' }, { value: 'backlog', label: 'Backlog' }]} selected={['progress']} />
      </FormSection>
      <FormSection title="Who may take it">
        <CheckboxGroup label="Roles" options={[{ value: 'owner', label: 'Owner' }, { value: 'member', label: 'Member' }, { value: 'cab', label: 'Change Advisory Board' }]} selected={['owner', 'member']} />
        <Switch label="Wait for people's yes" hint="The entry moves once the last level approves." checked />
      </FormSection>
      <FormSection title="Advanced" description="Conditions, fields that must be filled, checks, rules on this step" isFolded>
        <Input label="Condition" placeholder="Priority is high" />
      </FormSection>
    </FormLayout>
    {#snippet actions()}<Button tone="subtle" onclick={() => (isOpen = false)}>Cancel</Button>{/snippet}
  </Dialog>
{/if}
