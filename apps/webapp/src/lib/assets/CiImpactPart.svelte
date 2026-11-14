<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An object's impact both ways (UC-AST-05 check 5, work-surface §6.10, scene `ci`): what suffers
  // when it fails, or what it depends on, up to eight steps - each object with its type and how many
  // steps away, its protection need where it is higher than normal, its owner at the end; services
  // marked. The way is a switch; the other way is read when it is chosen.
  import { Badge, FoldingSection, Icon, Spinner, ViewSwitcher } from '@hubtask/design-system/components';
  import type { CiClass, CiImpact, ConfigurationItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { classWord, cmdb, PROTECTION_LEVELS, type Words } from '../data/cmdb.svelte.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { highestProtection } from '../data/inventoryquery.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    ci: ConfigurationItem;
    classes: readonly CiClass[];
    words: Words;
    known: ReadonlyMap<string, ConfigurationItem>;
    /** The downstream read the page made already; the upstream one is read here when chosen. */
    downstream: CiImpact | undefined;
    downstreamRefusal?: string;
  }

  const { ci, classes, words, known, downstream, downstreamRefusal }: Props = $props();

  type Way = 'DOWNSTREAM' | 'UPSTREAM';
  let way = $state<Way>('DOWNSTREAM');
  let upstream = $state<CiImpact | undefined>(undefined);
  let upstreamRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    if (way !== 'UPSTREAM') return;
    const id = ci.id;
    void ci.version;
    upstream = undefined;
    upstreamRefusal = undefined;
    cmdb.impact(id, 'UPSTREAM').then(
      (read) => (upstream = read),
      (error) => (upstreamRefusal = renderProblem(error as never, messages).message),
    );
  });

  const impact = $derived(way === 'DOWNSTREAM' ? downstream : upstream);
  const refusal = $derived(way === 'DOWNSTREAM' ? downstreamRefusal : upstreamRefusal);
  const reached = $derived([...(impact?.reached ?? [])].sort((a, b) => a.depth - b.depth));
  const services = $derived(new Set(impact?.services ?? []));

  function ownerOf(id: string): string | undefined {
    const other = known.get(id);
    if (other?.owner_account_id) return accounts.nameOf(other.owner_account_id) ?? undefined;
    if (other?.owner_group_id) return groups.nameOf(other.owner_group_id) ?? undefined;
    return undefined;
  }
  /** The need said only where it is higher than normal. */
  function needOf(id: string): string | undefined {
    const other = known.get(id);
    const level = highestProtection(other?.effective_protection ?? other?.protection ?? {});
    return level && level !== PROTECTION_LEVELS[0] ? t(`app.assets.level.${level}`) : undefined;
  }
</script>

<FoldingSection id="ci-impact" title={t('app.object.mark_impact')} count={downstream?.reached.length ? String(downstream.reached.length) : undefined}>
  {#snippet children()}
    <div class="ways">
      <ViewSwitcher label={t('app.object.impact_way')} views={[{ id: 'DOWNSTREAM', label: t('app.assets.impact_downstream') }, { id: 'UPSTREAM', label: t('app.object.impact_upstream') }]}
        selected={way} onselect={(id) => (way = id === 'UPSTREAM' ? 'UPSTREAM' : 'DOWNSTREAM')} />
      <span class="meta">{t('app.object.impact_reach')}</span>
    </div>
    {#if refusal}
      <FailureNote problem={refusal} />
    {:else if !impact}
      <Spinner label={t('app.assets.loading')} />
    {:else if reached.length === 0}
      <p class="meta none">{t('app.assets.impact_none')}</p>
    {:else}
      <ul class="rows" data-impact={way}>
        {#each reached as each (each.ci_id)}
          {@const need = needOf(each.ci_id)}
          {@const owner = ownerOf(each.ci_id)}
          <li class="row" data-impact-row>
            <span class="what">
              <Icon name="shield" size="sm" />
              <a href={`/cis/${each.ci_id}`}>{each.name}</a>
              <span class="meta">{classWord(each.class, classes, words)} · {t('app.object.impact_steps', { depth: each.depth })}{#if need}{' · '}{need}{/if}</span>
              {#if services.has(each.ci_id)}<Badge size="sm" tone="info">{t('app.assets.impact_service')}</Badge>{/if}
            </span>
            {#if owner}<span class="owner">{owner}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</FoldingSection>

<style>
  .ways { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-150); margin-block-end: var(--sp-100); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); }
  .none { margin: 0; }
  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-050) var(--sp-150);
    min-block-size: var(--sp-500); padding-block: var(--sp-050); border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }
  .row:last-child { border-block-end: 0; }
  .what { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-100); min-inline-size: 0; color: var(--text-subtle); }
  .what a { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .what a:hover { text-decoration: underline; }
  .owner { margin-inline-start: auto; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
