<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The overview's first steps (user-experience.md §8.5): each step a sentence, a tick or the one
  // press to its screen, and *n of m done*. What is shown is `firstSteps`' decision.
  import { Button, Icon, ProgressBar } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { FirstSteps } from './firststeps.ts';

  interface Props {
    steps: FirstSteps;
    onhide: () => void;
  }

  const { steps, onhide }: Props = $props();
  const total = $derived(steps.steps.length);
</script>

<section class="first" aria-labelledby="first-steps" data-testid="first-steps">
  <div class="head">
    <h2 id="first-steps">{t('app.first_steps.title')}</h2>
    <span class="quiet">{t('app.first_steps.count', { done: steps.done, total })}</span>
  </div>
  <ProgressBar value={total === 0 ? 0 : Math.round((steps.done / total) * 100)} label={t('app.first_steps.title')} size="sm" />
  <ul class="steps">
    {#each steps.steps as step (step.id)}
      <li data-step={step.id} data-done={step.isDone ? '' : undefined}>
        <span class="tick" aria-hidden="true">{#if step.isDone}<Icon name="check" size="sm" />{/if}</span>
        <span class="text">{t(`app.first_steps.${step.label}`)}</span>
        {#if step.isDone}
          <span class="quiet">{t('app.first_steps.done')}</span>
        {:else}
          <a href={step.page}>{t(`app.first_steps.${step.label}_act`)}</a>
        {/if}
      </li>
    {/each}
  </ul>
  <div><Button size="sm" onclick={onhide}>{t('app.first_steps.hide')}</Button></div>
</section>

<style>
  /* The overview's focal card draws the surface; the steps are its content (page-system §4.2). */
  .first { display: grid; gap: var(--sp-150); }

  .head { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); align-items: baseline; justify-content: space-between; }

  h2 { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { color: var(--text-secondary); font-size: var(--fs-100); }

  .steps { margin: 0; padding: 0; list-style: none; display: grid; }

  /* The act stays beside its sentence at the end; a long sentence wraps in its own column. */
  .steps li {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: var(--sp-150);
    align-items: center;
    padding: var(--sp-100) 0;
    border-bottom: var(--bw-hairline) solid var(--border-subtle);
  }

  .tick {
    display: inline-grid;
    place-items: center;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-full);
    border: var(--bw-thick) solid var(--border-default);
  }

  li[data-done] .tick {
    background: var(--status-success-surface);
    border-color: var(--status-success-border);
    color: var(--status-success-text);
  }

  li[data-done] .text { color: var(--text-secondary); }

  a { color: var(--text-brand); }
</style>
