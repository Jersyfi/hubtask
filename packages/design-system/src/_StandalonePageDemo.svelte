<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Banner from './Banner.svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Input from './Input.svelte';
  import Logo from './Logo.svelte';
  import StandalonePage from './StandalonePage.svelte';
  import Stack from './Stack.svelte';

  const { variant = 'card', content = 'full' }: { variant?: 'card' | 'public'; content?: 'full' | 'empty' | 'refused' } = $props();
  let address = $state('');
</script>

{#if variant === 'card'}
  <StandalonePage>
    <Logo size="md" wordmark="Hubtask" />
    <Stack gap="250">
      {#if content === 'refused'}<Banner tone="danger">That address and password do not match.</Banner>{/if}
      <h1>Sign in</h1>
      {#if content !== 'empty'}<Input label="Email address" type="email" bind:value={address} />{/if}
      <Button tone="primary" isFull>Continue</Button>
    </Stack>
    {#snippet foot()}<a href="#imprint">Imprint</a><a href="#privacy">Privacy</a>{/snippet}
  </StandalonePage>
{:else}
  <StandalonePage variant="public" title="Roadmap 2027">
    {#if content === 'refused'}
      <Banner tone="warning">There is no roadmap at this link.</Banner>
    {:else if content === 'empty'}
      <EmptyState kind="unused" title="Nothing on the roadmap yet" description="Its projects appear here once they have dates." />
    {:else}
      <p>Pilot · 13 Nov – 28 Nov</p>
      <p>Go-live · 1 Dec</p>
    {/if}
  </StandalonePage>
{/if}
