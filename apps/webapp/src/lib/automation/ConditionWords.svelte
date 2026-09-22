<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A condition's tree in words, read-only: the sentences plain, the modes as small chips, a
  // nested group in brackets (F8-18, decision 18). The gate and a branch draw it alike; the panel
  // edits it. An expression the composer did not write is said as "the expression holds", with
  // the expression under it, so nothing is hidden.

  import ConditionWords from './ConditionWords.svelte';
  import { isGroup, readNode, type Node } from './model.ts';
  import { sentenceWords, type Names } from './words.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The stored expression, or a node already read. */
    expr?: string;
    node?: Node;
    names: Names;
  }

  const { expr, node, names }: Props = $props();

  const words = { t, has: (code: string) => messages.has(code) };
  const tree = $derived(node ?? (expr !== undefined ? readNode(expr) : undefined));
</script>

{#if !tree}
  <span class="w">{t('app.flow.sentence_expression')}</span>
  {#if expr}<code class="expr">{expr}</code>{/if}
{:else if !isGroup(tree)}
  <span class="w">{sentenceWords(words, names, tree)}</span>
{:else}
  {#if tree.mode === 'none'}<span class="chip">{t('app.flow.chip_none')}</span>{/if}
  {#each tree.items as item, index (index)}
    {#if index > 0}<span class="chip">{tree.mode === 'all' ? t('app.flow.chip_and') : t('app.flow.chip_or')}</span>{/if}
    {#if isGroup(item)}
      <span class="group"><ConditionWords node={item} {names} /></span>
    {:else}
      <ConditionWords node={item} {names} />
    {/if}
  {/each}
{/if}

<style>
  .w { color: var(--text-primary); overflow-wrap: anywhere; }

  .chip { display: inline-block; padding: 0 var(--sp-050); border-radius: var(--r-xs); background: var(--label-amber-bg); color: var(--label-amber-fg); font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; }

  .group { display: inline; padding: 0 var(--sp-025); border-radius: var(--r-xs); outline: var(--bw-hairline) dashed var(--label-amber-fg); outline-offset: var(--sp-025); }

  .expr { display: block; font-family: var(--font-mono); font-size: var(--fs-050); color: var(--text-subtle); overflow-wrap: anywhere; }
</style>
