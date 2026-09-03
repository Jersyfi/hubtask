<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Something is happening and it is not finished.
  //
  // The whole design of this component is the accessible name, because that is where a spinner
  // goes wrong. Two cases, and they are opposites. Inside a control that is already `aria-busy`,
  // the spinner is decoration and must say nothing - a second announcement of the same fact is
  // noise. Standing on its own, while a region loads, it is the only thing that knows, and silence
  // there means a screen reader user waits at an empty page with no idea that anything is coming.
  //
  // So `label` decides: with one, the spinner is a live region that says it; without one, it is
  // hidden from the accessibility tree. There is no default, because a default would be wrong in
  // one of the two cases and this component cannot know which.

  import Icon from './Icon.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { ControlSize } from './control.ts';

  interface Props {
    size?: ControlSize;
    /**
     * What is being waited for, in words. Resolved text, never a sentence written here (ADR-0011),
     * and `voice-and-tone.md` §2.4's present participle: "Loading tasks", not "Please wait".
     */
    label?: string;
  }

  const { size = 'md', label }: Props = $props();
</script>

<span
  class="spinner"
  data-size={size}
  role={label ? 'status' : undefined}
  aria-hidden={label ? undefined : 'true'}
>
  <Icon name="loader-circle" size={size === 'sm' ? 'sm' : 'md'} />
  {#if label}
    <VisuallyHidden>{label}</VisuallyHidden>
  {/if}
</span>

<style>
  .spinner {
    display: inline-flex;
    align-items: center;
    /* No colour of its own: it takes the one it sits in, the way the icon inside it does. A muted
       grey looked right standing on a surface and was unreadable inside a primary button, which is
       where this component spends most of its life. */
    /* Rule 6: transform only. A spinner that animated a width would move the label beside it. */
    animation: turn var(--dur-deliberate) linear infinite;
  }

  @keyframes turn {
    to { transform: rotate(1turn); }
  }

  /* Under a reduced-motion preference the turn stops. The fact is still announced - `role=status`
     and the caller's `aria-busy` do not move - which is rule 6's floor: a moving thing that cannot
     be stopped is what the rule is written for, not the information it carries. */
  @media (prefers-reduced-motion: reduce) {
    .spinner { animation: none; }
  }

  :global([data-motion='reduced']) .spinner { animation: none; }
</style>
