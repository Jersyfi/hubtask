<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The Hubtask mark, with or without the wordmark (design-system.md §9).
  //
  // **Not an `Icon`.** An icon is `currentColor` and nothing else (§12); the mark has four parts in
  // three colours, each a `brand.*` token, so it is a component of its own. Its mono variant is the
  // one place it takes `currentColor`.
  //
  // **The form follows the size step**, never a measurement: `sm` draws the reduced mark, every
  // larger step the full one (`mark.ts`).
  //
  // **The wordmark is the caller's resolved text**, the name as it is written in prose, and only
  // the stylesheet lowers it: copying it, searching the page for it and a screen reader all meet
  // the name, not a styling of it. It is never translated: it is a name. The mark stays at the
  // inline start and is never mirrored; the spark is not a direction.
  //
  // **Named once.** With a wordmark the text is the name and the drawing is decoration; without
  // one, `label` names the drawing; with neither, the mark is decorative and its context names it.

  import { formFor, knockedOutPlane, MARK_GRID, squarePath, type LogoSize } from './mark.ts';

  interface Props {
    size?: LogoSize;
    /** `color` draws the brand tokens; `mono` takes the colour of the text around it. */
    variant?: 'color' | 'mono';
    /** The product's name as resolved text, shown beside the mark. */
    wordmark?: string;
    /** The accessible name of the mark alone. Ignored beside a wordmark, which names it. */
    label?: string;
  }

  const { size = 'md', variant = 'color', wordmark, label }: Props = $props();

  const form = $derived(formFor(size));
  const isNamed = $derived(!wordmark && !!label);
</script>

<span class="logo" data-size={size} data-variant={variant}>
  <svg
    class="mark"
    viewBox="0 0 {MARK_GRID} {MARK_GRID}"
    role={isNamed ? 'img' : undefined}
    aria-label={isNamed ? label : undefined}
    aria-hidden={isNamed ? undefined : 'true'}
    focusable="false"
  >
    <path class="outline" d={form.outline.d} stroke-width={form.outline.stroke} />
    {#if variant === 'mono'}
      <path class="plane" d={knockedOutPlane(form)} fill-rule="evenodd" />
    {:else}
      <path class="plane" d={squarePath(form.plane)} />
      {#if form.core}
        <path class="core" d={squarePath(form.core)} />
      {/if}
    {/if}
    <circle class="spark" cx={form.spark.cx} cy={form.spark.cy} r={form.spark.r} />
  </svg>
  {#if wordmark}
    <span class="wordmark" translate="no">{wordmark}</span>
  {/if}
</span>

<style>
  .logo {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-primary);
  }

  .mark {
    flex: none;
    overflow: visible;
  }

  .outline {
    fill: none;
    stroke: var(--brand-outline);
    stroke-linecap: round;
  }

  .plane { fill: var(--brand-plane); }
  .core { fill: var(--brand-core); }
  .spark { fill: var(--brand-spark); }

  .logo[data-variant='mono'] .outline { stroke: currentColor; }
  .logo[data-variant='mono'] .plane,
  .logo[data-variant='mono'] .spark { fill: currentColor; }

  .wordmark {
    font-family: var(--font-display);
    font-weight: var(--fw-bold);
    line-height: 1;
    /* A name in a script with case is lowered; one without case is left as it is written. */
    text-transform: lowercase;
    white-space: nowrap;
  }

  .logo[data-size='sm'] { gap: var(--sp-050); }
  .logo[data-size='sm'] .mark { inline-size: var(--sp-200); block-size: var(--sp-200); }
  .logo[data-size='sm'] .wordmark { font-size: var(--fs-200); }
  .logo[data-size='md'] .mark { inline-size: var(--sp-300); block-size: var(--sp-300); }
  .logo[data-size='md'] .wordmark { font-size: var(--fs-400); }
  .logo[data-size='lg'] .mark { inline-size: var(--sp-400); block-size: var(--sp-400); }
  .logo[data-size='lg'] .wordmark { font-size: var(--fs-600); }
  .logo[data-size='xl'] { gap: var(--sp-150); }
  .logo[data-size='xl'] .mark { inline-size: var(--sp-600); block-size: var(--sp-600); }
  .logo[data-size='xl'] .wordmark { font-size: var(--fs-700); }
</style>
