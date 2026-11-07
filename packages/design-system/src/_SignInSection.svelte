<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The mockup's *Passwords* section, shared by SettingsSection's story and SettingsLayout's.
  import Input from './Input.svelte';
  import SaveBar from './SaveBar.svelte';
  import SettingRow from './SettingRow.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import SourceChip from './SourceChip.svelte';
  import Switch from './Switch.svelte';

  const { isLocked = false }: { isLocked?: boolean } = $props();

  let length = $state('14');
  let breaches = $state(true);
  const isChanged = $derived(length !== '12');
</script>

<SettingsSection
  title="Passwords"
  status="2 set here"
  description="What a new password must be. Existing passwords stay valid until they are changed."
  viewAsFile={{ label: 'View as file', onclick: () => {} }}
  advanced={{ label: 'Advanced · kinds of characters, repeats, earliest change', description: '5 settings, all default', rows: advancedRows }}
  save={isChanged ? saveBar : undefined}
>
  <SettingRow label="Minimum length" help="Length protects more than kinds of characters." groupLabel="Strength" fieldWidth="xs" unit="characters" {isChanged} reset={{ label: 'Reset', onclick: () => (length = '12') }} lockReason={isLocked ? 'Set by the installation' : undefined}>
    {#snippet source()}<SourceChip label="Set here" detailsLabel="Where this comes from" value="14 characters" source="Set in this workspace." />{/snippet}
    {#snippet control(nameId)}<Input label="Minimum length" isLabelHidden aria-labelledby={nameId} bind:value={length} inputmode="numeric" />{/snippet}
  </SettingRow>
  <SettingRow label="Refuse commonly used passwords" help="Adds a line to every password screen.">
    {#snippet source()}<SourceChip label="Default" detailsLabel="Where this comes from" value="On" source="Hubtask's default; nobody has changed it." />{/snippet}
    {#snippet control()}<Switch label="Refuse commonly used passwords" isLabelHidden checked />{/snippet}
  </SettingRow>
  <SettingRow label="Check against known breaches" help="Five characters of a hash leave this server." isChanged>
    {#snippet source()}<SourceChip label="Set here" detailsLabel="Where this comes from" value="On" source="Set in this workspace." />{/snippet}
    {#snippet control()}<Switch label="Check against known breaches" isLabelHidden bind:checked={breaches} />{/snippet}
  </SettingRow>
  <SettingRow label="Expires after" help="Empty means never — what is recommended." groupLabel="Change and reuse" fieldWidth="xs" unit="days">
    {#snippet source()}<SourceChip label="Default" detailsLabel="Where this comes from" value="Never" source="Hubtask's default; nobody has changed it." />{/snippet}
    {#snippet control(nameId)}<Input label="Expires after" isLabelHidden aria-labelledby={nameId} placeholder="—" />{/snippet}
  </SettingRow>
  <SettingRow label="Remember previous passwords" help="0 to 10." fieldWidth="xs">
    {#snippet source()}<SourceChip label="Default" detailsLabel="Where this comes from" value="0" source="Hubtask's default; nobody has changed it." />{/snippet}
    {#snippet control(nameId)}<Input label="Remember previous passwords" isLabelHidden aria-labelledby={nameId} value="0" />{/snippet}
  </SettingRow>
</SettingsSection>

{#snippet advancedRows()}
  <SettingRow label="Require a digit" help="Adds little once a password is long.">
    {#snippet control()}<Switch label="Require a digit" isLabelHidden />{/snippet}
  </SettingRow>
{/snippet}

{#snippet saveBar()}
  <SaveBar label="Unsaved changes" changesLabel="2 changes" reachLabel="Reach all 9 people at their next password change" discardLabel="Discard" saveLabel="Save" onDiscard={() => (length = '12')} />
{/snippet}
