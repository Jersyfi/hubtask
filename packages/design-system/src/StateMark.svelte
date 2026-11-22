<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one press of an entry's state control in a row and on a card: a small round mark in the
  // same place on every level (design-system.md §4, wave 7). Empty where one press completes, a
  // tick once done, an arrow where the press takes a step that does not complete the entry.
  //
  // Where the press completes or reopens it is the APG Checkbox pattern - `role="checkbox"` with
  // `aria-checked` on a button, because the press is a write the caller makes, not a value a form
  // submits; where it takes a step it is a plain button named by the step (`statemark.ts`).
  // The drawn mark stays small; its hit area reaches `density.control.sm` around it, and
  // `layout.target` where the pointer is coarse or the window a phone's (§7).

  import type { HTMLButtonAttributes } from 'svelte/elements';

  import Reason from './_Reason.svelte';
  import Icon from './Icon.svelte';
  import Spinner from './Spinner.svelte';
  import type { Busyable, Disableable } from './control.ts';
  import { guard, sharedReason } from './reason.ts';
  import { markLook, type MarkForm } from './statemark.ts';

  interface Props extends Omit<HTMLButtonAttributes, 'disabled' | 'role' | 'onclick'>, Disableable, Busyable {
    /** What the press does, with the entry's title: *Mark "Order tiles" as done*. Required: the mark draws no word. */
    label: string;
    /** `check` where the press completes or reopens, `step` where it moves on without completing. */
    form?: MarkForm;
    /** Done: the mark is filled and ticked. Only the `check` form reads it. */
    checked?: boolean;
    /** What it is doing while busy, said to a reader: the present participle. */
    busyLabel?: string;
    /** The press. A promise it returns ends the press's own drawing when it settles. */
    onclick?: () => void | Promise<unknown>;
  }

  const {
    label,
    form = 'check',
    checked = false,
    disabledReason,
    isBusy = false,
    busyLabel,
    onclick,
    type = 'button',
    ...rest
  }: Props = $props();

  // The press is drawn at once, as a native box ticks before its write is answered; the caller's
  // next `checked` replaces it, and so does the end of a write that changed nothing.
  let pressed = $state<boolean | undefined>(undefined);
  $effect.pre(() => {
    void checked;
    pressed = undefined;
  });
  const look = $derived(markLook(form, pressed ?? checked));
  const unavailable = $derived(disabledReason !== undefined);
  const instanceId = $props.id();
  const shared = sharedReason();
  const groupReasonId = $derived(shared(disabledReason));
  const reasonId = $derived(unavailable ? (groupReasonId ?? `reason-${instanceId}`) : undefined);
  let button = $state<HTMLButtonElement | null>(null);

  // Off for a reason a group states once, or busy, the mark draws no bubble but still refuses the
  // press and its keys here, so neither reaches a handler around the row (as `Checkbox` does).
  $effect(() => {
    if (!(unavailable && groupReasonId) && !isBusy) return;
    return guard([button], () => {});
  });

  function press() {
    if (unavailable || isBusy) return;
    if (form === 'check') pressed = !(pressed ?? checked);
    const answer = onclick?.();
    if (answer instanceof Promise) void answer.finally(() => (pressed = undefined));
  }
</script>

<button
  {type}
  class="state-mark"
  data-form={form}
  data-on={look.ariaChecked === 'true' ? '' : undefined}
  role={look.role}
  aria-checked={look.ariaChecked}
  aria-label={label}
  aria-disabled={unavailable ? 'true' : undefined}
  aria-busy={isBusy ? 'true' : undefined}
  aria-describedby={reasonId}
  title={unavailable ? undefined : label}
  bind:this={button}
  onclick={press}
  {...rest}
>
  {#if isBusy}
    <Spinner size="sm" label={busyLabel} />
  {:else}
    <Icon name={look.icon} size="sm" />
  {/if}
</button>
{#if unavailable && reasonId && !groupReasonId}
  <Reason id={reasonId} text={disabledReason ?? ''} control={button} />
{/if}

<style>
  /* Twenty-two pixels across: the drawn mark sits on a row's first line beside a small lozenge of
     the same height, and is not a control size of its own. */
  .state-mark {
    position: relative;
    display: inline-grid;
    flex: none;
    place-items: center;
    inline-size: calc(var(--sp-250) + var(--sp-025));
    block-size: calc(var(--sp-250) + var(--sp-025));
    padding: 0;
    border: var(--bw-thick) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: transparent;
    font: inherit;
    cursor: pointer;
    transition:
      background-color var(--motion-state-duration) var(--motion-state-easing),
      border-color var(--motion-state-duration) var(--motion-state-easing),
      color var(--motion-state-duration) var(--motion-state-easing);
  }

  /* The glyph at the lozenge's type size, so mark and chip read as one line. */
  .state-mark :global(.icon) { inline-size: var(--fs-075); block-size: var(--fs-075); }

  /* The hit area beyond the drawn circle: a small control's on a desk, a thumb's on a phone. */
  .state-mark::after {
    content: '';
    position: absolute;
    inset: calc((100% - var(--density-control-sm-min)) / 2);
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .state-mark::after { inset: calc((100% - var(--layout-target)) / 2); }
  }

  .state-mark[data-form='step'] { color: var(--text-secondary); }

  .state-mark:hover:not([aria-disabled='true']) { border-color: var(--accent-primary); color: var(--accent-primary); }

  .state-mark[data-on] {
    border-color: var(--status-success-accent);
    background: var(--status-success-accent);
    color: var(--text-inverse);
  }

  .state-mark[data-on]:hover:not([aria-disabled='true']) { border-color: var(--status-success-accent); color: var(--text-inverse); }

  .state-mark:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* Not pressable now: the edge breaks, so it reads as unavailable without the colour. */
  .state-mark[aria-disabled='true'] { border-style: dashed; cursor: not-allowed; }

  .state-mark[aria-busy='true'] { color: var(--text-secondary); cursor: progress; }

  @media (prefers-reduced-motion: reduce) {
    .state-mark { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .state-mark { transition-duration: var(--dur-instant); }
</style>
