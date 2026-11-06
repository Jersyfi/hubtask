<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame, loaded once somebody is signed in: the doors (sign-in, setup, an invitation, a
  // reset) draw from the entry chunk alone and never wait for the navigation, the palette and the
  // work's screens. A frame that cannot be fetched is told apart as a screen is (LazyView).
  import type { Snippet } from 'svelte';
  import type { Resolution } from '../router.ts';
  import { settle } from './chunkfailure.ts';
  import { loadFrame } from './screens.ts';
  import ChunkFailed from './ChunkFailed.svelte';

  type Props = {
    route: Resolution;
    onnavigate: (path: string) => void;
    children: Snippet;
  };
  const { route, onnavigate, children }: Props = $props();

  const pending = loadFrame();
</script>

{#await pending then module}
  {@const Frame = module.default}
  <Frame {route} {onnavigate}>{@render children()}</Frame>
{:catch}
  {#await settle() then outcome}
    {#if outcome !== 'reload'}<ChunkFailed {outcome} />{/if}
  {/await}
{/await}
