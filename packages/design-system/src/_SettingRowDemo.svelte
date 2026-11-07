<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Input from './Input.svelte';
  import Select from './Select.svelte';
  import SettingRow from './SettingRow.svelte';
  import SourceChip from './SourceChip.svelte';
  import Switch from './Switch.svelte';
</script>

<div class="region">
  <SettingRow label="Minimum length" help="Length protects more than kinds of characters." groupLabel="Field widths by content" fieldWidth="xs" unit="characters" isChanged reset={{ label: 'Reset', onclick: () => {} }}>
    {#snippet source()}<SourceChip label="Set here" detailsLabel="Where this comes from" value="14 characters" source="Set in this workspace." />{/snippet}
    {#snippet control(nameId)}<Input label="Minimum length" isLabelHidden aria-labelledby={nameId} value="14" />{/snippet}
  </SettingRow>
  <SettingRow label="Expires" help="At most a year away." fieldWidth="sm" isRequired>
    {#snippet control(nameId)}<Select label="Expires" isLabelHidden aria-labelledby={nameId} options={[{ value: '90', label: 'In 90 days' }, { value: '365', label: 'In a year' }]} value="90" />{/snippet}
  </SettingRow>
  <SettingRow label="What it is for" help="What you will read in a year." fieldWidth="md" isRequired>
    {#snippet control(nameId)}<Input label="What it is for" isLabelHidden aria-labelledby={nameId} value="Nightly export" />{/snippet}
  </SettingRow>
  <SettingRow label="Address of the help page" help="Shown under *Help* to everyone." fieldWidth="lg">
    {#snippet control(nameId)}<Input label="Address of the help page" isLabelHidden aria-labelledby={nameId} value="https://help.nordlicht.example.org/hubtask" />{/snippet}
  </SettingRow>
  <SettingRow label="Second factor for everyone" help="Asked at every sign-in on a new device." groupLabel="Locked" lockReason="Set by the installation; ask its operator.">
    {#snippet source()}<SourceChip label="Locked" isLocked detailsLabel="Where this comes from" value="On" source="Set by the installation." whomToAsk="The installation's operator" />{/snippet}
    {#snippet control()}<Switch label="Second factor for everyone" isLabelHidden checked disabledReason="Set by the installation" />{/snippet}
  </SettingRow>
</div>

<style>
  .region {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: clip;
  }
</style>
