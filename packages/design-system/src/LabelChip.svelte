<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A label, in one of ten colours and no others.
  //
  // §4 states the constraint in five words — "ten `colorToken` values, nothing else" — and
  // `domain-model.md` §3.5 gives the reason: "the colour is a token (not hex) → theming is possible
  // in the frontend". A chip that took a hex would be a chip that is unreadable in one of the two
  // themes, because the pair behind each token is `bg` **and** `fg`, measured together by F1-02 for
  // contrast in both. One value cannot carry that.
  //
  // The token is validated by the backend against the same ten names (`LabelTokens.go`, generated
  // from `tokens.json`), so a chip and a row never disagree about which colours exist.

  import type { Snippet } from 'svelte';

  import { labelTokens, type LabelToken } from '../dist/tokens.ts';

  interface Props {
    /** The label's own words. Content, not a message code. */
    name: string;
    /** One of the ten. A name that is not one of them falls back rather than painting nothing. */
    colorToken?: string;
    /** What the colour means to somebody who did not choose it. Announced, not drawn. */
    description?: string | null;
    /** The name of the control that takes it off. Absent means the chip is not removable. */
    removeLabel?: string;
    onRemove?: () => void;
    children?: Snippet;
  }

  const { name, colorToken, description, removeLabel, onRemove }: Props = $props();

  /**
   * The token to paint with, and the fallback.
   *
   * `slate` rather than nothing: a label whose colour this client does not recognise is still a
   * label, and drawing it unstyled would make it look like a defect rather than like a label from
   * an installation that has one more colour than this build knows.
   */
  const token = $derived(
    colorToken && (labelTokens as readonly string[]).includes(colorToken)
      ? (colorToken as LabelToken)
      : 'slate',
  );
</script>

<span class="chip" data-token={token} title={description ?? undefined}>
  <span class="name">{name}</span>
  {#if removeLabel}
    <button type="button" class="remove" aria-label={removeLabel} onclick={() => onRemove?.()}>
      <!-- A cross drawn in CSS rather than an icon, because the chip is `fs-075` and a 16 px glyph
           beside 12 px text is a control that outweighs the thing it belongs to. -->
      <span aria-hidden="true">×</span>
    </button>
  {/if}
</span>

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-full);
    font-size: var(--fs-075);
    /* Both halves of the pair, always together. The background alone would be a chip whose text is
       the page's colour on a tint that was measured against a different one. */
    background: var(--label-bg);
    color: var(--label-fg);
    max-width: 24ch;
  }

  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  /* The ten, and nothing else. Written out rather than composed, because a custom property name
     cannot be built from a value in CSS — and a `style="--label-bg: …"` would be the inline style
     ADR-0028's policy refuses. */
  .chip[data-token='slate'] { --label-bg: var(--label-slate-bg); --label-fg: var(--label-slate-fg); }
  .chip[data-token='blue'] { --label-bg: var(--label-blue-bg); --label-fg: var(--label-blue-fg); }
  .chip[data-token='teal'] { --label-bg: var(--label-teal-bg); --label-fg: var(--label-teal-fg); }
  .chip[data-token='green'] { --label-bg: var(--label-green-bg); --label-fg: var(--label-green-fg); }
  .chip[data-token='lime'] { --label-bg: var(--label-lime-bg); --label-fg: var(--label-lime-fg); }
  .chip[data-token='amber'] { --label-bg: var(--label-amber-bg); --label-fg: var(--label-amber-fg); }
  .chip[data-token='orange'] { --label-bg: var(--label-orange-bg); --label-fg: var(--label-orange-fg); }
  .chip[data-token='red'] { --label-bg: var(--label-red-bg); --label-fg: var(--label-red-fg); }
  .chip[data-token='magenta'] { --label-bg: var(--label-magenta-bg); --label-fg: var(--label-magenta-fg); }
  .chip[data-token='violet'] { --label-bg: var(--label-violet-bg); --label-fg: var(--label-violet-fg); }

  .remove {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    /* The floor WCAG 2.2 SC 2.5.8 sets, even inside a chip this small: the target is bigger than
       the glyph, which is what the negative margin buys back from the layout. */
    min-inline-size: var(--density-control-sm-min);
    min-block-size: var(--density-control-sm-min);
    margin-inline: calc(var(--sp-100) * -1);
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    line-height: 1;
    cursor: pointer;
  }

  .remove:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-full);
  }
</style>
