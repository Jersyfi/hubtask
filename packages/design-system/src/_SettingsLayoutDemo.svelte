<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Input from './Input.svelte';
  import SettingRow from './SettingRow.svelte';
  import SettingsLayout from './SettingsLayout.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import SourceChip from './SourceChip.svelte';
  import SignInSection from './_SignInSection.svelte';
  import Refused from './_RefusedDemo.svelte';
  import { signInSections } from './_pageFixtures.ts';

  const { content = 'full', start = 'list' }: { content?: 'full' | 'empty' | 'refused'; start?: 'list' | 'detail' } = $props();

  let opened = $state<string | undefined>(undefined);
  const view = $derived<'list' | 'detail'>(opened !== undefined ? 'detail' : start);
  const current = $derived(opened ?? 'passwords');
  const sections = $derived(content === 'empty' ? signInSections.map((section) => ({ ...section, status: 'Default', statusTone: undefined })) : signInSections);
</script>

<SettingsLayout
  head={{
    title: 'Sign-in',
    subtitle: content === 'empty' ? 'How people prove who they are · everything as Hubtask sets it' : 'How people prove who they are · 3 set here · 1 locked by the installation',
    breadcrumb: { trail: [{ id: 'admin', label: 'Administration', href: '#admin' }, { id: 'signin', label: 'Sign-in' }], label: 'Where you are', expandLabel: 'Show every level' },
    secondary: [{ label: 'View as file', onclick: () => {} }],
    menu: { label: 'More for Sign-in', items: [{ id: 'force', label: 'Require a new password from everyone…', isDestructive: true, hasSeparatorBefore: true }], onselect: () => {} },
  }}
  nav={{ label: 'Sections of Sign-in', sections, current, onselect: (id) => (opened = id) }}
  {view}
  back={{ label: 'Sign-in', onback: () => (opened = undefined) }}
  refused={content === 'refused' ? refused : undefined}
>
  {#if content === 'empty'}
    <SettingsSection title="Passwords" status="Default" description="What a new password must be. Nothing is set here: every value is Hubtask's default.">
      <SettingRow label="Minimum length" help="Length protects more than kinds of characters." fieldWidth="xs" unit="characters">
        {#snippet source()}<SourceChip label="Default" detailsLabel="Where this comes from" value="12 characters" source="Hubtask's default; nobody has changed it." />{/snippet}
        {#snippet control(nameId)}<Input label="Minimum length" isLabelHidden aria-labelledby={nameId} value="12" />{/snippet}
      </SettingRow>
    </SettingsSection>
  {:else}
    <SignInSection />
  {/if}
</SettingsLayout>

{#snippet refused()}<Refused what="the sign-in settings" />{/snippet}
