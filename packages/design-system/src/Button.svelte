<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The verb of what happens (voice-and-tone.md §2.1), drawn four ways.
  //
  // `tone` is the four purposes; everything a button *does* - hover, press, focus, disable - is a
  // CSS state below and never a prop, because §5 says a variant matrix that contains states
  // explodes and this is the component it would explode in first.

  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  import Reason from './_Reason.svelte';
  import Icon from './Icon.svelte';
  import Spinner from './Spinner.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { IconName } from './icons/index.ts';
  import type { Busyable, ButtonTone, ControlSize, Disableable } from './control.ts';

  interface Props extends Omit<HTMLButtonAttributes, 'disabled'>, Disableable, Busyable {
    tone?: ButtonTone;
    size?: ControlSize;
    /** An icon before the label. Decoration: the label already says what happens. */
    icon?: IconName;
    /**
     * What the button says while it is working. `voice-and-tone.md` §2.4: the present participle of
     * its own verb, and the button keeps its width - rule 6 forbids animating layout.
     */
    busyLabel?: string;
    /**
     * A mark before the label, drawn by the caller, pinned to the start edge.
     *
     * For the one case `icon` cannot serve: a third-party brand mark, which is not ours to recolour
     * and therefore not a member of the icon set (ADR-0041 governs *our* marks, in `currentColor`).
     * The label stays centred and the mark sits at the start, so a column of provider buttons has
     * its marks in one line and its labels on one axis - which is how every provider's own
     * guidelines draw the neutral form of their button.
     */
    lead?: Snippet;
    /**
     * Whether it takes the whole width of what it is in.
     *
     * A question, as §5 asks of a boolean we invent. It exists for the one shape a form has on a
     * phone - a column of full-width controls - rather than as a size: how prominent a button is
     * stays `tone`, and how tall it is stays `size`.
     */
    isFull?: boolean;
    children?: Snippet;
  }

  const {
    tone = 'secondary',
    size = 'md',
    icon,
    isBusy = false,
    busyLabel,
    disabledReason,
    lead,
    isFull = false,
    children,
    type = 'button',
    ...rest
  }: Props = $props();

  // A busy button is not unavailable - it is working, and keeps its focus and its name.
  //
  // An unavailable one is `aria-disabled`, never the native attribute: the native attribute takes
  // the button out of the tab order, and a keyboard reader would never reach the reason. What the
  // attribute used to stop - the click, `Enter`, a form submitting through it - the guard in
  // `_Reason.svelte` stops instead, before any handler of the caller's (reason.ts).
  const unavailable = $derived(disabledReason !== undefined);
  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const reasonId = $derived(unavailable ? `reason-${instanceId}` : undefined);
  let button = $state<HTMLButtonElement | null>(null);
</script>

<button
  {type}
  class="button"
  data-lead={lead ? '' : undefined}
  data-full={isFull ? '' : undefined}
  data-tone={tone}
  data-size={size}
  data-busy={isBusy && !icon ? '' : undefined}
  aria-disabled={unavailable ? 'true' : undefined}
  aria-busy={isBusy ? 'true' : undefined}
  aria-describedby={reasonId}
  bind:this={button}
  {...rest}
>
  {#if isBusy && !icon}
    <!-- Over the label rather than beside it: a button without an icon has no slot for the
         spinner, and adding one would widen it (voice-and-tone.md §2.4 keeps its width). -->
    <span class="busy"><Spinner size="sm" /></span>
  {:else if isBusy}
    <Spinner size="sm" />
  {:else if icon}
    <Icon name={icon} size="sm" />
  {/if}
  {#if lead && !isBusy}
    <span class="lead">{@render lead()}</span>
  {/if}
  <span class="label">{@render children?.()}</span>
  {#if isBusy && busyLabel}
    <VisuallyHidden>{busyLabel}</VisuallyHidden>
  {/if}
</button>
{#if unavailable && reasonId}
  <Reason id={reasonId} text={disabledReason ?? ''} control={button} />
{/if}

<style>
  .button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    font: inherit;
    font-weight: var(--fw-medium);
    /* `start`/`end`, never left/right: the direction axis exists to catch the other spelling. */
    text-align: center;
    cursor: pointer;
    /* Rule 6: opacity and transform only. Layout is never animated. */
    transition:
      transform var(--motion-state-duration) var(--motion-state-easing),
      opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  /* The mark is taken out of the flow so the label is centred in the whole button rather than in
     what is left of it. The padding on both ends keeps a long label from running under the mark -
     symmetric, because a label centred between asymmetric padding is not centred. */
  .button[data-full] { width: 100%; }
  .button[data-lead] { position: relative; }
  .button[data-lead][data-size='md'] { padding-inline: var(--sp-500); }
  .button[data-lead][data-size='sm'] { padding-inline: var(--sp-400); }

  .lead {
    position: absolute;
    inset-inline-start: var(--sp-150);
    display: inline-flex;
    align-items: center;
  }

  .button[data-size='md'] {
    padding: var(--density-control-md-block) var(--sp-200);
    min-height: var(--density-control-md-min);
    font-size: var(--fs-100);
  }

  .button[data-size='sm'] {
    padding: var(--density-control-sm-block) var(--sp-150);
    min-height: var(--density-control-sm-min);
    font-size: var(--fs-075);
  }

  /* Rule 5, on every control in this wave: 2 px ring, 2 px offset, --focus-ring. */
  .button:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .button[data-tone='primary'] {
    background: var(--accent-primary);
    color: var(--text-inverse);
  }
  .button[data-tone='primary']:hover:not([aria-disabled='true']) { background: var(--accent-primary-hover); }
  .button[data-tone='primary']:active:not([aria-disabled='true']) { background: var(--accent-primary-pressed); }

  .button[data-tone='secondary'] {
    background: var(--bg-surface);
    border-color: var(--border-default);
    color: var(--text-primary);
  }
  .button[data-tone='secondary']:hover:not([aria-disabled='true']) { background: var(--bg-surface-hover); }
  .button[data-tone='secondary']:active:not([aria-disabled='true']) { background: var(--bg-surface-pressed); }

  .button[data-tone='subtle'] {
    background: transparent;
    color: var(--text-secondary);
  }
  .button[data-tone='subtle']:hover:not([aria-disabled='true']) { background: var(--bg-surface-hover); color: var(--text-primary); }
  .button[data-tone='subtle']:active:not([aria-disabled='true']) { background: var(--bg-surface-pressed); }

  /* Rule 3: colour never stands alone. A danger button is red *and* says what it destroys - the
     label is the caller's, and voice-and-tone.md §2.6 is where that rule lives. */
  .button[data-tone='danger'] {
    background: var(--bg-surface);
    border-color: var(--border-default);
    color: var(--text-danger);
  }
  .button[data-tone='danger']:hover:not([aria-disabled='true']) { background: var(--bg-surface-hover); }
  .button[data-tone='danger']:active:not([aria-disabled='true']) { background: var(--bg-surface-pressed); }

  .button:active:not([aria-disabled='true']) { transform: scale(0.98); }

  .button[aria-disabled='true'] {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
    cursor: not-allowed;
  }

  .label:empty { display: none; }

  /* The spinner laid over a label that keeps its place, unseen: the width stays the label's. Not
     `visibility: hidden`, which would take the label out of the button's accessible name too. */
  .button[data-busy] { position: relative; }
  .button[data-busy] .label { opacity: 0; }
  .busy {
    position: absolute;
    inset: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  @media (prefers-reduced-motion: reduce) {
    .button { transition-duration: var(--dur-instant); }
    .button:active:not([aria-disabled='true']) { transform: none; }
  }

  :global([data-motion='reduced']) .button { transition-duration: var(--dur-instant); }
  :global([data-motion='reduced']) .button:active:not([aria-disabled='true']) { transform: none; }
</style>
