<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Inline from './Inline.svelte';
  import Logo from './Logo.svelte';
  import Stack from './Stack.svelte';
  import type { LogoSize } from './mark.ts';

  const { mode = 'lockup' }: { mode?: 'lockup' | 'steps' | 'mono' } = $props();

  const steps: LogoSize[] = ['sm', 'md', 'lg', 'xl'];
</script>

{#if mode === 'lockup'}
  <Stack gap="300">
    {#each steps as size (size)}
      <Logo {size} wordmark="Hubtask" />
    {/each}
  </Stack>
{:else if mode === 'steps'}
  <Inline gap="300" align="end">
    {#each steps as size (size)}
      <Logo {size} label="Hubtask" />
    {/each}
  </Inline>
{:else}
  <Stack gap="300">
    <Inline gap="300" align="end">
      {#each steps as size (size)}
        <Logo {size} variant="mono" label="Hubtask" />
      {/each}
    </Inline>
    <p class="on-accent">
      <Logo size="lg" variant="mono" wordmark="Hubtask" />
    </p>
  </Stack>
{/if}

<style>
  .on-accent {
    margin: 0;
    padding: var(--sp-200);
    border-radius: var(--r-lg);
    background: var(--accent-primary);
    color: var(--text-inverse);
  }
</style>
