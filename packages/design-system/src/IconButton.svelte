<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A button whose label is not painted.
  //
  // `label` is required and is not decoration: a control with no visible text has no accessible
  // name unless something gives it one, and `VisuallyHidden` is in wave 0 for exactly this. It is
  // also the tooltip, so the two cannot say different things.

  import type { HTMLButtonAttributes } from 'svelte/elements';

  import Icon from './Icon.svelte';
  import Spinner from './_Spinner.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { IconName } from './icons/index.ts';
  import type { Busyable, ButtonTone, ControlSize, Disableable } from './control.ts';

  interface Props extends Omit<HTMLButtonAttributes, 'disabled'>, Disableable, Busyable {
    icon: IconName;
    /** What the control does, in words. Required: an icon is not an accessible name. */
    label: string;
    tone?: ButtonTone;
    size?: ControlSize;
  }

  const {
    icon,
    label,
    tone = 'subtle',
    size = 'md',
    isBusy = false,
    disabledReason,
    type = 'button',
    ...rest
  }: Props = $props();

  const unavailable = $derived(disabledReason !== undefined);
  const reasonId = $derived(unavailable ? `reason-${Math.random().toString(36).slice(2, 9)}` : undefined);
</script>

<button
  {type}
  class="icon-button"
  data-tone={tone}
  data-size={size}
  disabled={unavailable}
  aria-busy={isBusy ? 'true' : undefined}
  aria-describedby={reasonId}
  title={label}
  {...rest}
>
  {#if isBusy}
    <Spinner {size} />
  {:else}
    <Icon name={icon} size={size === 'sm' ? 'sm' : 'md'} />
  {/if}
  <VisuallyHidden>{label}</VisuallyHidden>
</button>
{#if unavailable}
  <span id={reasonId} class="reason">{disabledReason}</span>
{/if}

<style>
  .icon-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
    transition:
      transform var(--dur-fast) var(--ease-standard),
      opacity var(--dur-fast) var(--ease-standard);
  }

  /* Square, and big enough to hit. The target is the control, not the glyph. */
  .icon-button[data-size='md'] {
    width: var(--sp-500);
    height: var(--sp-500);
  }

  .icon-button[data-size='sm'] {
    width: var(--sp-400);
    height: var(--sp-400);
  }

  .icon-button:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .icon-button:hover:not(:disabled) { background: var(--bg-surface-hover); color: var(--text-primary); }
  .icon-button:active:not(:disabled) { background: var(--bg-surface-pressed); transform: scale(0.94); }

  .icon-button[data-tone='primary'] { background: var(--accent-primary); color: var(--text-inverse); }
  .icon-button[data-tone='primary']:hover:not(:disabled) { background: var(--accent-primary-hover); color: var(--text-inverse); }

  .icon-button[data-tone='secondary'] {
    background: var(--bg-surface);
    border-color: var(--border-default);
    color: var(--text-primary);
  }

  .icon-button[data-tone='danger'] { color: var(--text-danger); }

  .icon-button:disabled {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
    cursor: not-allowed;
  }

  .reason {
    display: block;
    max-width: 40ch;
    margin-top: var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  @media (prefers-reduced-motion: reduce) {
    .icon-button { transition-duration: var(--dur-instant); }
    .icon-button:active:not(:disabled) { transform: none; }
  }

  :global([data-motion='reduced']) .icon-button { transition-duration: var(--dur-instant); }
  :global([data-motion='reduced']) .icon-button:active:not(:disabled) { transform: none; }
</style>
