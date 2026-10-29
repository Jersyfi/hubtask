<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A run's decisions row by row (UC-AST-02 check 7, UC-AST-04 check 5): each item's outcome, its
  // CI, the rule that matched, the attributes taken and those kept with the source that kept them,
  // a duplicate's candidates and a hold's or refusal's reason - by name, never by value.

  import { Badge } from '@hubtask/design-system/components';
  import type { CiClass, CiDecision } from '@hubtask/sync-engine';

  import { classWord, outcomeTone, type Words } from '../data/cmdb.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    decisions: readonly CiDecision[];
    classes: readonly CiClass[];
    words: Words;
  }

  const { decisions, classes, words }: Props = $props();
  const word = (key: string) => (messages.has(`app.assets.attribute.${key}`) ? t(`app.assets.attribute.${key}`) : key);
</script>

<table class="decisions">
  <caption>{t('app.assets.decisions')}</caption>
  <thead>
    <tr>
      <th scope="col">{t('app.assets.row')}</th>
      <th scope="col">{t('app.assets.class_label')}</th>
      <th scope="col">{t('app.assets.decisions')}</th>
    </tr>
  </thead>
  <tbody>
    {#each decisions as decision, index (`${decision.ref}-${index}`)}
      {@const tone = outcomeTone(decision.outcome)}
      <tr>
        <td>
          {#if decision.ci_id}<a href={`/cis/${decision.ci_id}`}>{decision.ref}</a>{:else}{decision.ref}{/if}
        </td>
        <td>{decision.class ? classWord(decision.class, classes, words) : '—'}</td>
        <td class="what">
          <Badge size="sm" tone={tone}>{t(`app.assets.outcome.${decision.outcome}`)}</Badge>
          {#if decision.rule}<span>{t('app.assets.decision_rule', { rule: decision.rule.split('+').map(word).join(' + ') })}</span>{/if}
          {#if decision.taken?.length}<span>{t('app.assets.decision_taken', { keys: decision.taken.map(word).join(', ') })}</span>{/if}
          {#each decision.kept ?? [] as kept (kept.attribute)}
            <span>{t('app.assets.decision_kept', { key: word(kept.attribute), by: kept.by })}</span>
          {/each}
          {#if decision.candidates?.length}<span>{t('app.assets.decision_candidates', { count: decision.candidates.length })}</span>{/if}
          {#if decision.reason}<span class="reason">{messages.has(decision.reason) ? t(decision.reason) : decision.reason}</span>{/if}
        </td>
      </tr>
    {/each}
  </tbody>
</table>

<style>
  .decisions { border-collapse: collapse; inline-size: 100%; font-size: var(--fs-075); }
  .decisions caption { text-align: start; color: var(--text-secondary); padding-block-end: var(--sp-050); }
  .decisions th, .decisions td { text-align: start; vertical-align: baseline; padding: var(--sp-050) var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .what { display: flex; flex-wrap: wrap; gap: var(--sp-050) var(--sp-100); align-items: baseline; }
  .reason { color: var(--text-secondary); }
  a { color: var(--text-primary); }
</style>
