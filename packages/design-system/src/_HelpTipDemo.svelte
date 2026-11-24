<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import HelpTip from './HelpTip.svelte';
  import Input from './Input.svelte';
  import SettingRow from './SettingRow.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import Stack from './Stack.svelte';
  import Switch from './Switch.svelte';
  import { setHelpName } from './media.ts';

  const { mode = 'label' }: { mode?: 'label' | 'heading' | 'long' | 'rtl' | 'phone' } = $props();

  // What the application sets once in its language: the name of every ⓘ.
  $effect(() => {
    setHelpName((subject) => `More about ${subject}`);
    return () => setHelpName(undefined);
  });

  let expires = $state('');
  let isInviting = $state(false);

  const why = 'There is no default on purpose: a short one gets worked around, and a long one is the token that never expires, which this rule is there to prevent.';
  const long =
    'A guest sees only the hubs they are invited to, and never the people or the hubs around them. Their invitation runs out after fourteen days unless they accept it; an owner can send it again at any time. Guests count towards the workspace’s seats only once they have signed in for the first time.';
</script>

{#if mode === 'label' || mode === 'phone'}
  <div class="frame">
    <Input label="Expires" hint="At most a year from today." more={why} bind:value={expires} isRequired />
  </div>
{:else if mode === 'heading'}
  <SettingsSection title="Guests" description="Who outside the workspace may be invited to a hub." more={long}>
    <SettingRow label="Invite guests" help="Owners of a hub may invite people from outside." more={why}>
      {#snippet control(nameId)}<Switch label="Invite guests" isLabelHidden aria-labelledby={nameId} bind:checked={isInviting} />{/snippet}
    </SettingRow>
  </SettingsSection>
{:else if mode === 'long'}
  <Stack gap="200">
    <p class="line">Guests <HelpTip subject="Guests" text={long} /></p>
    <p class="line end">At the end of the line <HelpTip subject="Guests at the end" text={long} /></p>
  </Stack>
{:else}
  <div dir="rtl" class="frame">
    <Input label="Expires" hint="At most a year from today." more={why} bind:value={expires} />
  </div>
{/if}

<style>
  .frame { max-inline-size: var(--layout-field-md); }
  .line { display: flex; align-items: center; gap: var(--sp-050); margin: 0; }
  .line.end { justify-content: flex-end; }
</style>
