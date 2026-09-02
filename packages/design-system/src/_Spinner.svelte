<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The turning part of a busy control. Underscored: `Spinner` proper is a component of wave 1
  // with its own sizes and its own accessible name, and this is the fragment a Button needs before
  // that exists. It has no name of its own - the button it sits in already has one.

  import Icon from './Icon.svelte';
  import type { ControlSize } from './control.ts';

  const { size = 'md' }: { size?: ControlSize } = $props();
</script>

<span class="spinner" data-size={size}><Icon name="loader-circle" size="sm" /></span>

<style>
  .spinner {
    display: inline-flex;
    /* Rule 6: transform only. A spinner that animated a width would move the label beside it. */
    animation: turn var(--dur-deliberate) linear infinite;
  }

  @keyframes turn {
    to { transform: rotate(1turn); }
  }

  /* Under a reduced-motion preference the turn stops; the control stays `aria-busy`, so the fact
     is still announced. A moving thing that cannot be stopped is the one rule 6 is written for. */
  @media (prefers-reduced-motion: reduce) {
    .spinner { animation: none; }
  }

  :global([data-motion='reduced']) .spinner { animation: none; }
</style>
