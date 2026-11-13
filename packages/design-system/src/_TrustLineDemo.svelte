<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import LabelChip from './LabelChip.svelte';
  import TrustLine from './TrustLine.svelte';

  interface Props {
    mode?: 'verified' | 'due' | 'ownerless';
  }

  const { mode = 'verified' }: Props = $props();
</script>

<TrustLine
  owner={mode === 'ownerless' ? undefined : 'Mehmet Yılmaz'}
  ownerRole="answers for this page"
  ownerless="No one answers for this page"
  verification={mode === 'verified' ? { state: 'verified', label: 'Verified until 15 Jan 2027' } : mode === 'due' ? { state: 'due', label: 'Review due' } : undefined}
  changed="Changed 22 min. ago by Amelia"
>
  {#snippet labels()}
    <LabelChip name="architecture" colorToken="teal" />
    <LabelChip name="customer-app" colorToken="blue" />
  {/snippet}
</TrustLine>
