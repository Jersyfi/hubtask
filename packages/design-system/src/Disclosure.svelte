<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What is needed later, folded on the same page (user-experience.md UX-4, A8): *Advanced* in a
  // settings section, *Details for support* under a refusal. A button that says whether it is
  // open (`aria-expanded`) and which region it opens, not a `<details>`: the content arrives through
  // the `reveal` role, and a closed section holds nothing a screen reader or a search would find.
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { reveal } from './motion.ts';

  interface Props {
    /** What is folded, as a person looks for it: "Advanced", "Details for support". */
    label: string;
    /** One quiet line beside the label: what is inside, so nobody has to open it to find out. */
    description?: string;
    isOpen?: boolean;
    /** `quiet` sits in running text (a refusal's details); `section` heads a settings block. */
    tone?: 'section' | 'quiet';
    children: Snippet;
  }

  let { label, description, isOpen = $bindable(false), tone = 'section', children }: Props = $props();

  const instanceId = $props.id();
  const regionId = `disclosure-${instanceId}`;
</script>

<div class="disclosure" data-tone={tone}>
  <button type="button" class="toggle" aria-expanded={isOpen} aria-controls={regionId} onclick={() => (isOpen = !isOpen)}>
    <span class="chevron" aria-hidden="true"><Icon name="chevron-right" /></span>
    <span class="label">{label}</span>
    {#if description}<span class="description">{description}</span>{/if}
  </button>
  <div id={regionId} class="region" role="region" aria-label={label} hidden={!isOpen}>
    {#if isOpen}<div class="content" in:reveal>{@render children()}</div>{/if}
  </div>
</div>

<style>
  .disclosure { display: flex; flex-direction: column; gap: var(--sp-100); }

  .toggle {
    display: inline-flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--sp-050);
    align-self: flex-start;
    padding: var(--sp-050) var(--sp-050);
    margin-inline-start: calc(-1 * var(--sp-050));
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-weight: var(--fw-medium);
    text-align: start;
    cursor: pointer;
  }

  .toggle:hover { background: var(--bg-surface-hover); }
  .toggle:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .chevron {
    display: inline-flex;
    align-self: center;
    color: var(--text-secondary);
    transition: transform var(--motion-state-duration) var(--motion-state-easing);
  }

  [aria-expanded='true'] .chevron { transform: rotate(90deg); }
  :global([dir='rtl']) [aria-expanded='true'] .chevron { transform: rotate(-90deg); }

  .description { color: var(--text-secondary); font-weight: var(--fw-regular); font-size: var(--fs-075); }

  [data-tone='quiet'] .toggle { color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-regular); }

  .content { display: flex; flex-direction: column; gap: var(--sp-150); }

  @media (prefers-reduced-motion: reduce) {
    .chevron { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .chevron { transition-duration: var(--dur-instant); }
</style>
