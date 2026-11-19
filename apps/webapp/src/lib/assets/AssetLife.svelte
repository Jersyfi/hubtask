<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A device's life from requested to disposed (work-surface §6.10, scenes `ci` and `invreg`): every
  // step as a line of a checklist, each reached one with its day and what it held - the stockroom,
  // the person, the request -, the disposal's method and certificate once disposed, and *proof
  // needed* at the last step until then. The object's page and the register's tab draw the same.
  import { untrack } from 'svelte';
  import { Icon, VisuallyHidden } from '@hubtask/design-system/components';
  import type { CiAsset } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { formatDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { lifeSteps } from './objectpage.ts';

  interface Props {
    asset: Pick<CiAsset, 'state' | 'moves' | 'request_id' | 'disposal'>;
    /** An object's name by its id: the stockroom a step was in. */
    nameOf: (id: string) => string | undefined;
  }

  const { asset, nameOf }: Props = $props();

  const steps = $derived(lifeSteps(asset));
  $effect(() => {
    const ids = steps.flatMap((step) => [step.holder, step.by]).filter((each): each is string => Boolean(each));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });
  const day = (at: string) => formatDay(at, messages.locale);
  const person = (id: string) => accounts.nameOf(id) ?? t('app.people.unnamed');
</script>

<ol class="life" aria-label={t('app.object.life')} data-asset-life>
  {#each steps as step (step.state)}
    <li class="step" data-reached={step.isReached ? '' : undefined} data-current={step.isCurrent ? '' : undefined} data-life-step={step.state}>
      <span class="mark" aria-hidden="true">
        {#if step.isReached && !step.isCurrent}<Icon name="check" size="sm" />{:else if step.isCurrent}<span class="dot"></span>{/if}
      </span>
      <span class="what">
        <span class="word">{t(`app.register.state.${step.state}`)}</span>
        {#if step.isReached && !step.isCurrent}<VisuallyHidden>{t('app.object.step_done')}</VisuallyHidden>{/if}
        {#if step.isCurrent}<VisuallyHidden>{t('app.object.step_current')}</VisuallyHidden>{/if}
        <span class="detail">
          {#if step.at}<span>{day(step.at)}</span>{/if}
          {#if step.state === 'REQUESTED' && asset.request_id}<a href={`/items/${asset.request_id}`}>{t('app.register.request_open')}</a>{/if}
          {#if step.stockroom}<span>{t('app.object.life_in', { place: nameOf(step.stockroom) ?? t('app.assets.unknown_ci') })}</span>{/if}
          {#if step.holder}<span>{t('app.object.life_with', { person: person(step.holder) })}</span>{/if}
          {#if step.state === 'DISPOSED'}
            {#if asset.disposal}
              <span data-disposal>{t(`app.register.method.${asset.disposal.method}`)} · {t('app.register.certificate_value', { reference: asset.disposal.certificate })}</span>
            {:else}
              <span class="proof">{t('app.object.life_proof')}</span>
            {/if}
          {/if}
        </span>
      </span>
    </li>
  {/each}
</ol>

<style>
  .life { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }
  .step { position: relative; display: flex; gap: var(--sp-100); padding-block-end: var(--sp-150); }
  .step:last-child { padding-block-end: 0; }
  /* The thread between the marks: done up to where it stands, quiet after. */
  .step:not(:last-child)::before {
    content: '';
    position: absolute;
    inset-block: var(--sp-250) 0;
    inset-inline-start: calc(var(--sp-100) - var(--bw-hairline));
    border-inline-start: var(--bw-thick) solid var(--border-subtle);
  }
  .step[data-reached]:not([data-current])::before { border-inline-start-color: var(--border-strong); }
  .mark {
    display: inline-flex; flex: none; align-items: center; justify-content: center;
    inline-size: var(--sp-200); block-size: var(--sp-200);
    border: var(--bw-thick) solid var(--border-default); border-radius: var(--r-full);
    background: var(--bg-surface); color: var(--text-success);
  }
  .step[data-reached] .mark { border-color: var(--status-success-border); background: var(--status-success-surface); }
  .step[data-current] .mark { border-color: var(--accent-primary); background: var(--bg-surface); }
  .dot { inline-size: var(--sp-100); block-size: var(--sp-100); border-radius: var(--r-full); background: var(--accent-primary); }
  .what { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .word { font-weight: var(--fw-medium); color: var(--text-subtle); }
  .step[data-reached] .word { color: var(--text-primary); }
  .step[data-current] .word { font-weight: var(--fw-semibold); }
  .detail { display: flex; flex-wrap: wrap; gap: var(--sp-025) var(--sp-100); color: var(--text-secondary); font-size: var(--fs-075); }
  .detail a { color: var(--text-brand); }
  .proof { color: var(--text-warning); font-weight: var(--fw-medium); }
</style>
