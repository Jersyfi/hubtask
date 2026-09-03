<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Banner from './Banner.svelte';
  import Button from './Button.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'tones' }: { mode?: 'tones' | 'frame' } = $props();

  let dismissed = $state(false);
</script>

{#if mode === 'frame'}
  <Stack gap="200">
    <!-- The two the frame needs (F1-10). Both are this component with content: ADR-0035 §2's
         maturity banner, dismissible for the session, and §4's HealthBanner, which comes back
         while the degradation lasts. -->
    {#if !dismissed}
      <Banner
        title="Hubtask is experimental"
        dismissLabel="Dismiss"
        onDismiss={() => (dismissed = true)}
      >
        Screens appear, move and disappear without notice, and data may not survive an update.
      </Banner>
    {/if}
    <Banner tone="warning" title="Search is slower than usual">
      {#snippet action()}
        <Button size="sm">Open the status page</Button>
      {/snippet}
      Results are complete; they take longer to arrive while the index catches up.
    </Banner>
  </Stack>
{:else}
  <Stack gap="200">
    <Banner title="Two people are editing this task">Their changes are merged as they arrive.</Banner>
    <Banner tone="success" title="Import finished">All 240 tasks were created.</Banner>
    <Banner tone="warning" title="This collection is nearly full">
      12 tasks left before the plan's limit.
    </Banner>
    <Banner tone="danger" title="The last sync failed" dismissLabel="Dismiss">
      Nothing was lost. Hubtask will try again in a minute.
    </Banner>
  </Stack>
{/if}
