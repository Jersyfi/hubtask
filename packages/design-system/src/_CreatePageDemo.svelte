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

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' | 'filled' } = $props();
  const isEmpty = $derived(content === 'empty');

  const scopes = [
    { id: 'entries', label: 'Entries, hubs and collections', help: 'Read · create and change' },
    { id: 'people', label: 'People and roles', help: 'See who is here · invite and change' },
    { id: 'audit', label: 'Audit and privacy', help: 'Read the trail · export · requests' },
    { id: 'automation', label: 'Automation and agents', help: 'Rules, agents and their ledger' },
  ];
</script>

{#if content === 'filled'}
  <!-- The region the frame gives a filled page has a height: the demo gives it one. -->
  <div class="region">
    <CreatePage
      isFilled
      head={{
        title: 'Create hub',
        breadcrumb: { trail: [{ id: 'hubs', label: 'Hubs', href: '#hubs' }, { id: 'new', label: 'Create hub' }], label: 'Where you are', expandLabel: 'Show every level' },
        primary: { label: 'Create hub', onclick: () => {} },
        secondary: [{ label: 'Cancel', isQuiet: true, onclick: () => {} }],
      }}
      close={{ label: 'Cancel', onclose: () => {} }}
    >
      <div class="panes">
        <section class="pane details" aria-label="Details">
          {#each Array.from({ length: 12 }, (_, at) => at + 1) as at (at)}<Input label={`Detail ${at}`} />{/each}
        </section>
        <section class="pane start" aria-label="Start">
          {#each Array.from({ length: 30 }, (_, at) => at + 1) as at (at)}<p>Template {at}</p>{/each}
        </section>
      </div>
    </CreatePage>
  </div>
{:else}
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

{/if}

{#snippet refused()}<Refused what="how to create a token" />{/snippet}

<style>
  .region { display: flex; flex-direction: column; block-size: calc(var(--layout-dialog-l) - var(--sp-1000)); border: var(--bw-hairline) solid var(--border-subtle); }
  .panes { display: grid; grid-template-columns: var(--layout-pane-width) minmax(0, 1fr); min-block-size: 0; block-size: 100%; }
  .pane { display: grid; align-content: start; gap: var(--sp-200); min-block-size: 0; overflow: auto; padding: var(--sp-300); }
  .details { background: var(--bg-surface); border-inline-end: var(--bw-hairline) solid var(--border-subtle); }
  .start p { margin: 0; }
</style>
