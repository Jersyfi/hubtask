<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What counts in a collection, in its head (UC-WRK-29 check 1): the sentence, a bar for progress
  // alone (check 2), and for the week's rhythm one mark per day with a completion and nothing for
  // the others (check 3); for a flow, the columns over their limit as a warning beside the figure
  // (UC-WRK-31 check 3). The words are figure.ts's; this draws them.

  import type { CollectionFigure } from '@hubtask/sync-engine';
  import { ProgressBar } from '@hubtask/design-system/components';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { barShare, headMessage, overLimitMessage, rhythmMarks } from '../data/figure.ts';

  interface Props {
    figure: CollectionFigure;
    /** The collection's name, which the bar's name carries. */
    name: string;
  }

  const { figure, name }: Props = $props();

  const message = $derived(headMessage(figure, messages.locale));
  const share = $derived(barShare(figure));
  const marks = $derived(rhythmMarks(figure));
  const overLimit = $derived(overLimitMessage(figure));
</script>

{#if message}
  <span class="figure" data-figure={figure.kind} data-unavailable={figure.unavailable ? '' : undefined}>
    {#if share !== undefined}
      <span class="bar"><ProgressBar label={t('app.figure.progress_label', { name })} value={share} size="sm" /></span>
    {/if}
    {#if marks.length > 0}
      <span class="marks" aria-hidden="true">
        {#each marks as day (day)}<span class="mark" data-day={day}></span>{/each}
      </span>
    {/if}
    <span class="text">{t(message.code, message.params)}</span>
    {#if overLimit}
      <span class="over" data-over-limit>{t(overLimit.code, overLimit.params)}</span>
    {/if}
  </span>
{/if}

<style>
  .figure {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
  }

  .bar { inline-size: 12ch; }

  .marks {
    display: inline-flex;
    gap: var(--sp-050);
  }

  /* A day with a completion: a filled mark. A day without one draws nothing, so no day is named
     as missed (UC-WRK-29 check 3). */
  .mark {
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--status-success-accent);
  }

  .figure[data-unavailable] .text { font-style: italic; }

  /* A column over its limit: advice, so a tinted mark rather than an error. */
  .over {
    padding-inline: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--status-warning-surface);
    color: var(--status-warning-text);
  }
</style>
