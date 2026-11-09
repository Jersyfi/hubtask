<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A page outside the app frame (page-system §7.9): nobody is signed in, or the visitor is not a
  // member - there is nowhere to go, so there is no bar and no navigation. Two variants of one
  // kind: `card` - one centred card on the canvas with the two ambient washes, the only brand
  // moment (sign-in, setup, a reset, an invitation, a consent, a rating, a public form); `public` -
  // the content at the window's full width under its title (a shared roadmap).
  import type { Snippet } from 'svelte';

  interface Props {
    variant?: 'card' | 'public';
    /** The heading of a `public` page; a card draws its own. */
    title?: string;
    /** Under the card or the content: the operator's legal links. */
    foot?: Snippet;
    children: Snippet;
  }

  const { variant = 'card', title, foot, children }: Props = $props();
</script>

<div class="canvas" data-page-kind="standalone" data-variant={variant}>
  <main id="main" class="main" tabindex="-1">
    {#if variant === 'card'}
      <section class="card">{@render children()}</section>
    {:else}
      {#if title}<h1 class="title">{title}</h1>{/if}
      {@render children()}
    {/if}
  </main>
  {#if foot}<footer class="foot">{@render foot()}</footer>{/if}
</div>

<style>
  .canvas {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .canvas[data-variant='card'] {
    min-height: 100vh;
    /* The geometry is layout and stays here; the colours are the two ambient tokens. */
    background:
      radial-gradient(ellipse 70% 60% at 12% -10%, var(--ambient-signature), transparent 70%),
      radial-gradient(ellipse 80% 70% at 100% 100%, var(--ambient-primary), transparent 70%),
      var(--bg-canvas);
  }

  .main {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .main:focus { outline: none; }

  [data-variant='card'] .main {
    align-items: center;
    justify-content: center;
    padding: var(--sp-400) var(--sp-200);
  }

  [data-variant='public'] .main {
    gap: var(--sp-150);
    padding: var(--sp-300) var(--sp-150);
  }

  .card {
    /* A width and padding together: without it the card is its own padding wider than its room. */
    box-sizing: border-box;
    /* In characters, so a language that needs more room takes it. */
    inline-size: min(100%, 52ch);
    display: grid;
    gap: var(--sp-250);
    padding: var(--sp-300);
    background: var(--bg-surface);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    /* The one focal card of the page (PS-16). */
    box-shadow: var(--shadow-raised);
  }

  .title { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .foot {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: var(--sp-100) var(--sp-250);
    padding: var(--sp-200);
    font-size: var(--fs-075);
  }

  .foot :global(a) { color: var(--text-subtle); }
</style>
