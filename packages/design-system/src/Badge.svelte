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

  interface Props {
    /** `neutral` is a count or a category: it carries no icon because it means nothing. */
    tone?: StatusTone | 'neutral';
    /** The mark rule 3 asks for, when the tone's own is not the right one. */
    icon?: IconName;
    children?: Snippet;
  }

  const { tone = 'neutral', icon, children }: Props = $props();

  const mark = $derived<IconName | undefined>(icon ?? (tone === 'neutral' ? undefined : STATUS_ICON[tone]));
</script>

<span class="badge" data-tone={tone}>
  {#if mark}
    <Icon name={mark} size="sm" />
  {/if}
  <span class="text">{@render children?.()}</span>
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    /* Rule 4: a badge holds a word that is three times longer in Finnish. It wraps rather than
       clipping, and nothing here fixes its width. */
    text-align: start;
    overflow-wrap: anywhere;
  }

  .badge[data-tone='neutral'] { color: var(--text-secondary); }
  .badge[data-tone='info'] { color: var(--text-brand); }
  .badge[data-tone='success'] { color: var(--text-success); }
  .badge[data-tone='warning'] { color: var(--text-warning); }
  .badge[data-tone='danger'] { color: var(--text-danger); }
</style>
