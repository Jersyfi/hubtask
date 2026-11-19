<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A run's decisions as a list, for a dialog: the dialog ladder holds no table there
  // (page-system's dialog rule); `Decisions` keeps the table on the runs page.

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

<ul class="list" aria-label={t('app.assets.decisions')} data-decision-list>
  {#each decisions as decision, index (`${decision.ref}-${index}`)}
    <li>
      <span class="head">
        {#if decision.ci_id}<a href={`/cis/${decision.ci_id}`}>{decision.ref}</a>{:else}{decision.ref}{/if}
        {#if decision.class}<span class="reason">{classWord(decision.class, classes, words)}</span>{/if}
        <Badge size="sm" tone={outcomeTone(decision.outcome)}>{t(`app.assets.outcome.${decision.outcome}`)}</Badge>
      </span>
      <span class="what">
        {#if decision.rule}<span>{t('app.assets.decision_rule', { rule: decision.rule.split('+').map(word).join(' + ') })}</span>{/if}
        {#if decision.taken?.length}<span>{t('app.assets.decision_taken', { keys: decision.taken.map(word).join(', ') })}</span>{/if}
        {#each decision.kept ?? [] as kept (kept.attribute)}
          <span>{t('app.assets.decision_kept', { key: word(kept.attribute), by: kept.by })}</span>
        {/each}
        {#if decision.candidates?.length}<span>{t('app.assets.decision_candidates', { count: decision.candidates.length })}</span>{/if}
        {#if decision.reason}<span class="reason">{messages.has(decision.reason) ? t(decision.reason) : decision.reason}</span>{/if}
      </span>
    </li>
  {/each}
</ul>

<style>
  .list { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  li { display: flex; flex-direction: column; gap: var(--sp-050); padding-block-end: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .head, .what { display: flex; flex-wrap: wrap; gap: var(--sp-050) var(--sp-100); align-items: baseline; }
  .reason { color: var(--text-secondary); }
  a { color: var(--text-primary); }
</style>
