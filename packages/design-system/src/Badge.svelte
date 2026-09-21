<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A state, said in one word.
  //
  // Rule 3 is the constraint that shapes it: colour never stands alone. A badge that is red and
  // says "Failed" survives greyscale, print, and colour vision deficiency; one that is only red
  // says nothing to a third of the ways it will be read. So a tone that carries meaning carries an
  // icon too, and the icon is chosen by the tone rather than by the caller - two badges of the same
  // tone with different marks would be two vocabularies.
  //
  // It is not a `LabelChip`. That one is wave 3, it takes one of the ten label colours, and it
  // belongs to data the tenant owns; this is the system's own vocabulary about the system's own
  // states.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { STATUS_ICON, type StatusTone } from './control.ts';
  import type { IconName } from './icons/index.ts';

  /**
   * How loudly the badge says it (ADR-0061, F9-04). `subtle` is a tinted surface with the tone's
   * text - the form for a list, where every row has one. `bold` is the tone's accent with
   * inverse text, for the one badge on a screen that must be seen first: a failed run, a lost
   * connection. A screen with three bold badges has misread the rule.
   */
  export type BadgeEmphasis = 'subtle' | 'bold';

  interface Props {
    /** `neutral` is a count or a category: it carries no icon because it means nothing. */
    tone?: StatusTone | 'neutral';
    emphasis?: BadgeEmphasis;
    /** The mark rule 3 asks for, when the tone's own is not the right one. */
    icon?: IconName;
    children?: Snippet;
  }

  const { tone = 'neutral', emphasis = 'subtle', icon, children }: Props = $props();

  const mark = $derived<IconName | undefined>(icon ?? (tone === 'neutral' ? undefined : STATUS_ICON[tone]));
</script>

<span class="badge" data-tone={tone} data-emphasis={emphasis}>
  {#if mark}
    <Icon name={mark} size="sm" />
  {/if}
  <span class="text">{@render children?.()}</span>
</span>

<style>
  /* The tone's four roles (ADR-0061): the surface the word sits on, the border that carries the
     boundary beside the colour (rule 3), the text, and - bold - the accent under inverse text.
     The tone sets the three custom properties and the two emphases read them, so there is one
     rule per role rather than one per tone and emphasis. */
  .badge {
    --badge-surface: var(--status-neutral-surface);
    --badge-border: var(--status-neutral-border);
    --badge-text: var(--status-neutral-text);
    --badge-accent: var(--status-neutral-accent);
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--badge-border);
    border-radius: var(--r-full);
    background: var(--badge-surface);
    color: var(--badge-text);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    /* Rule 4: a badge holds a word that is three times longer in Finnish. It wraps rather than
       clipping, and nothing here fixes its width. */
    text-align: start;
    overflow-wrap: anywhere;
  }

  .badge[data-tone='info'] {
    --badge-surface: var(--status-info-surface);
    --badge-border: var(--status-info-border);
    --badge-text: var(--status-info-text);
    --badge-accent: var(--status-info-accent);
  }

  .badge[data-tone='success'] {
    --badge-surface: var(--status-success-surface);
    --badge-border: var(--status-success-border);
    --badge-text: var(--status-success-text);
    --badge-accent: var(--status-success-accent);
  }

  .badge[data-tone='warning'] {
    --badge-surface: var(--status-warning-surface);
    --badge-border: var(--status-warning-border);
    --badge-text: var(--status-warning-text);
    --badge-accent: var(--status-warning-accent);
  }

  .badge[data-tone='danger'] {
    --badge-surface: var(--status-danger-surface);
    --badge-border: var(--status-danger-border);
    --badge-text: var(--status-danger-text);
    --badge-accent: var(--status-danger-accent);
  }

  .badge[data-emphasis='bold'] {
    background: var(--badge-accent);
    border-color: var(--badge-accent);
    color: var(--text-inverse);
    font-weight: var(--fw-semibold);
  }
</style>
