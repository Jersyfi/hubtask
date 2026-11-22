<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';

  export interface ChoiceCard {
    readonly value: string;
    /** The card's title: a resolved message code (ADR-0011). */
    readonly label: string;
    /** One line of what choosing it means. */
    readonly description?: string;
    /** What it is, cut after two lines on the card; a reader hears all of it. */
    readonly summary?: string;
    /** Names drawn as neutral pills under the words - the products a template switches on. */
    readonly chips?: readonly string[];
    readonly icon?: IconName;
    /** The start from nothing among templates: its edge is dashed until it is chosen. */
    readonly isBlank?: boolean;
    readonly disabledReason?: string;
  }

  /** Cards under a heading: one family of templates. */
  export interface ChoiceCardGroup {
    readonly label: string;
    readonly options: readonly ChoiceCard[];
  }
</script>

<script lang="ts">
  // One of a few choices that each need a sentence to be understood, drawn as cards: a title, one
  // explaining line, an optional mark. `Radio` is the same question where a word per answer is
  // enough.
  //
  // Native radios of one name inside their cards' labels, so the whole card is pressed, the arrow
  // keys walk and choose as in any radio group - across the groups too, because the name is one -
  // and a reader hears "radio, 1 of 2" in a named group. The chosen card is marked by its edge,
  // its surface and the filled dot or tick - never by colour alone. Nothing is chosen unless the
  // caller chose it, and a value whose card the caller leaves out stays chosen: a filter hides a
  // tile, it does not answer the question.
  //
  // `grid` with `tick` is a gallery that is a choice (the start of a hub): tiles no narrower than
  // a gallery card, the tick in the top end corner. A gallery whose card carries its own act is
  // `TemplateCard`.

  import type { Disableable } from './control.ts';
  import Icon from './Icon.svelte';
  import { sharedReason } from './reason.ts';

  interface Props extends Disableable {
    /** The question the cards answer: the group's legend and accessible name. */
    label: string;
    /** Cards without a heading, drawn before the groups. */
    options?: readonly ChoiceCard[];
    /** Cards under headings, after `options`; a group of none is not drawn. */
    groups?: readonly ChoiceCardGroup[];
    /** Shared by every input in the group; generated when absent. */
    name?: string;
    value?: string;
    /** `dot` at the start of the card, or `tick` in its top end corner: an empty ring until chosen. */
    mark?: 'dot' | 'tick';
    /** `stack` in one column, or `grid` of tiles that fill the width; one per row on a phone. */
    layout?: 'stack' | 'grid';
  }

  const instanceId = $props.id();
  let {
    label,
    options = [],
    groups = [],
    disabledReason,
    name = `choice-${instanceId}`,
    value = $bindable(''),
    mark = 'dot',
    layout = 'stack',
  }: Props = $props();

  const uid = `choice-cards-${instanceId}`;
  const shared = sharedReason();
  const groupReasonId = $derived(shared(disabledReason));
  const reasonId = $derived(disabledReason ? (groupReasonId ?? `${uid}-reason`) : undefined);
</script>

{#snippet cards(list: readonly ChoiceCard[])}
  <div class="cards">
    {#each list as option (option.value)}
      {@const optionId = `${uid}-${option.value}`}
      {@const off = disabledReason !== undefined || option.disabledReason !== undefined}
      {@const hasChips = (option.chips?.length ?? 0) > 0}
      {@const described = [
        option.description ? `${optionId}-line` : undefined,
        option.summary ? `${optionId}-summary` : undefined,
        hasChips ? `${optionId}-chips` : undefined,
        option.disabledReason ? `${optionId}-reason` : undefined,
      ].filter(Boolean).join(' ')}
      <label class="card" data-icon={option.icon ? '' : undefined} data-blank={option.isBlank ? '' : undefined}>
        <span class="control">
          <input
            id={optionId}
            type="radio"
            class="native"
            {name}
            value={option.value}
            bind:group={value}
            disabled={off}
            aria-describedby={described || undefined}
          />
          {#if mark === 'tick'}
            <span class="tick" aria-hidden="true"><Icon name="check" size="sm" /></span>
          {:else}
            <span class="dot" aria-hidden="true"></span>
          {/if}
        </span>
        {#if option.icon}<span class="mark" aria-hidden="true"><Icon name={option.icon} /></span>{/if}
        <span class="words">
          <span class="title">{option.label}</span>
          {#if option.description}<span id={`${optionId}-line`} class="line">{option.description}</span>{/if}
          {#if option.summary}<span id={`${optionId}-summary`} class="summary">{option.summary}</span>{/if}
          {#if hasChips}
            <span id={`${optionId}-chips`} class="chips">
              {#each option.chips ?? [] as chip (chip)}<span class="chip">{chip}</span>{/each}
            </span>
          {/if}
          {#if option.disabledReason}<span id={`${optionId}-reason`} class="reason">{option.disabledReason}</span>{/if}
        </span>
      </label>
    {/each}
  </div>
{/snippet}

<fieldset class="choices" role="radiogroup" aria-describedby={reasonId} data-mark={mark} data-layout={layout}>
  <legend class="legend">{label}</legend>

  {#if options.length > 0}{@render cards(options)}{/if}

  {#each groups as group (group.label)}
    {#if group.options.length > 0}
      <!-- A family inside the one radio group: a reader hears its legend on entering it. -->
      <fieldset class="group">
        <legend class="heading">{group.label}</legend>
        {@render cards(group.options)}
      </fieldset>
    {/if}
  {/each}

  {#if disabledReason && !groupReasonId}
    <p id={reasonId} class="reason">{disabledReason}</p>
  {/if}
</fieldset>

<style>
  .choices {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-inline-size: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }

  .legend {
    padding: 0;
    margin-block-end: var(--sp-100);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
    text-align: start;
  }

  .group {
    min-inline-size: 0;
    margin: var(--sp-100) 0 0;
    padding: 0;
    border: 0;
  }

  .heading {
    padding: 0;
    margin-block-end: var(--sp-100);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    text-align: start;
  }

  /* The grid sits on an element of its own, not on a fieldset: engines have laid out a fieldset's
     grid differently. */
  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-inline-size: 0;
  }

  .choices[data-layout='grid'] .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr));
    gap: var(--sp-150);
  }

  /* The whole card is the label: a press anywhere on it chooses. */
  .card {
    position: relative;
    display: grid;
    grid-template-columns: var(--sp-250) minmax(0, 1fr);
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    cursor: pointer;
    transition:
      background-color var(--motion-state-duration) var(--motion-state-easing),
      border-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .card[data-icon] { grid-template-columns: var(--sp-250) auto minmax(0, 1fr); }

  .card:hover { border-color: var(--border-default); }

  .card:has(.native:checked) {
    border-color: var(--accent-primary);
    background: var(--accent-primary-subtle);
  }

  .card:has(.native:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .card:has(.native:disabled) { cursor: not-allowed; }
  .card:has(.native:disabled):hover { border-color: var(--border-subtle); }
  .card:has(.native:disabled) .title { color: var(--text-subtle); }

  /* The start from nothing is dashed until chosen: a choice that sets up nothing reads as one. */
  .card[data-blank]:not(:has(.native:checked)) { border-style: dashed; }

  /* The dot sits on the title's first line, as `Radio`'s does. */
  .control {
    position: relative;
    display: inline-flex;
    margin-block-start: calc((1lh - var(--sp-250)) / 2);
  }

  /* In the tree and focusable, drawn by the dot or the tick beside it. */
  .native {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: inherit;
  }

  .dot {
    pointer-events: none;
    display: inline-block;
    width: var(--sp-250);
    height: var(--sp-250);
    border: var(--bw-thick) solid var(--border-strong);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    transition: box-shadow var(--motion-state-duration) var(--motion-state-easing);
  }

  .native:checked + .dot {
    border-color: var(--accent-primary);
    box-shadow: inset 0 0 0 var(--sp-050) var(--accent-primary);
  }

  .native:disabled + .dot { background: var(--bg-surface-sunken); border-color: var(--border-subtle); }

  /* The tick: in the top end corner, out of the words' columns. */
  .choices[data-mark='tick'] .card {
    grid-template-columns: minmax(0, 1fr);
    padding-inline-end: var(--sp-400);
  }

  .choices[data-mark='tick'] .card[data-icon] { grid-template-columns: auto minmax(0, 1fr); }

  .choices[data-mark='tick'] .card:hover { border-color: var(--border-strong); }
  .choices[data-mark='tick'] .card:has(.native:disabled):hover { border-color: var(--border-subtle); }

  /* Chosen, the edge doubles inward: a heavier line, not only another hue (SC 1.4.1). */
  .choices[data-mark='tick'] .card:has(.native:checked) { box-shadow: inset 0 0 0 var(--bw-hairline) var(--accent-primary); }

  .choices[data-mark='tick'] .control {
    position: absolute;
    inset-block-start: var(--sp-100);
    inset-inline-end: var(--sp-100);
    margin: 0;
  }

  /* An empty ring until chosen, then filled with the tick: a shape, not a hue. */
  .tick {
    pointer-events: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--sp-250);
    height: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: transparent;
    transition:
      background-color var(--motion-state-duration) var(--motion-state-easing),
      border-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .native:checked + .tick {
    border-color: var(--accent-primary);
    background: var(--accent-primary);
    color: var(--text-inverse);
  }

  .native:disabled + .tick { background: var(--bg-surface-sunken); border-color: var(--border-subtle); }

  .mark {
    display: inline-flex;
    margin-block-start: calc((1lh - var(--sp-250)) / 2);
    color: var(--text-secondary);
  }

  .words { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }

  .title { font-weight: var(--fw-semibold); overflow-wrap: anywhere; }

  .line { color: var(--text-secondary); font-size: var(--fs-090); overflow-wrap: anywhere; }

  .summary {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    overflow-wrap: anywhere;
  }

  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin-block-start: var(--sp-050); }

  /* The space scale has no step as small as the pill's block padding: it is the hairline's. */
  .chip {
    padding: var(--bw-hairline) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--status-neutral-surface);
    color: var(--status-neutral-text);
    font-size: var(--fs-050);
  }

  .reason { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  /* The tile: a gallery card's corner and height, the mark a square at the start. */
  .choices[data-layout='grid'] .card {
    align-content: start;
    column-gap: var(--sp-150);
    min-block-size: calc(var(--sp-1000) + var(--sp-150));
    border-radius: var(--r-lg);
  }

  /* The glyphs a size below Icon's own, as the tile draws them: the tick in the caption's size,
     the mark in the body's. */
  .tick :global(.icon) { inline-size: var(--fs-075); block-size: var(--fs-075); }
  .choices[data-layout='grid'] .mark :global(.icon) { inline-size: var(--fs-200); block-size: var(--fs-200); }

  .choices[data-layout='grid'] .mark {
    align-items: center;
    justify-content: center;
    align-self: start;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    margin: 0;
    border-radius: var(--r-md);
    background: var(--status-neutral-surface);
    color: var(--status-neutral-text);
  }

  .choices[data-layout='grid'] .title { font-size: var(--fs-100); line-height: var(--lh-snug); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .choices[data-layout='grid'] .cards { grid-template-columns: minmax(0, 1fr); gap: var(--sp-100); }
    .choices[data-layout='grid'] .card { min-block-size: var(--sp-800); }
  }

  @media (prefers-reduced-motion: reduce) {
    .card, .dot, .tick { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) :is(.card, .dot, .tick) { transition-duration: var(--dur-instant); }
</style>
