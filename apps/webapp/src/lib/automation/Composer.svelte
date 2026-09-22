<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A condition composed as a tree of sentences and stored as CEL (F8-04 decision 3, F8-13
  // decision 15).
  //
  // The composer offers a bounded set of subjects with the operators each takes; sentences stand
  // alone or under *all of* / *any of* / *none of*, nested as deep as the writer likes; the whole
  // compiles to the expression the server stores, shown under the tree. *Edit as an expression*
  // switches to the raw text the server already accepts. An expression the composer did not write
  // is shown as an expression, with the offer to replace it by a sentence - never silently
  // rewritten, because it may say something the sentences cannot.

  import { Button, Checkbox, Stack, Textarea } from '@hubtask/design-system/components';

  import SentenceRow from './SentenceRow.svelte';
  import { compileNode, defaultSentence, isGroup, newGroup, readNode, type GroupMode, type Node } from './model.ts';
  import type { Choices } from './SentenceRow.svelte';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The expression as it stands. */
    expr: string;
    /** The server's refusal at this expression, where there is one. */
    error?: string;
    /** The values a subject may take: the item types, the buckets, the accounts. */
    choices: Choices;
    onchange: (expr: string) => void;
  }

  const { expr, error, choices, onchange }: Props = $props();

  /** Expert mode is the reader's choice, kept while the same condition is edited. */
  let expert = $state(false);

  const node = $derived(readNode(expr));
  const foreign = $derived(expr.trim() !== '' && node === undefined);
  const editingRaw = $derived(expert || foreign);

  /**
   * The root is always drawn as a group, whatever the expression is (decision 28). A lone
   * sentence used to offer only *Add another sentence*, so a group became possible only once a
   * second sentence existed and the reader could not say *any of* from the start; a group of one
   * compiles to the sentence itself, so nothing about the stored expression changes.
   */
  const root = $derived.by(() => {
    const current = node ?? defaultSentence();
    return isGroup(current) ? current : { mode: 'all' as GroupMode, items: [current] };
  });

  function emit(next: Node): void {
    onchange(compileNode(next));
  }

  /** The tree with the node at `at` (a path of indices from the root) replaced, or removed for undefined. */
  function patch(root: Node, at: readonly number[], next: Node | undefined): Node | undefined {
    if (at.length === 0) return next;
    if (!isGroup(root)) return root;
    const [head, ...rest] = at;
    const items = root.items.flatMap((item, index) => {
      if (index !== head) return [item];
      const changed = patch(item, rest, next);
      return changed ? [changed] : [];
    });
    return { ...root, items };
  }

  const MODES: readonly GroupMode[] = ['all', 'any', 'none'];
</script>

{#snippet tree(current: Node, at: readonly number[], root: Node, siblings: number)}
  {#if isGroup(current)}
    <div class="group" data-group={at.join('/') || 'root'} data-depth={at.length}>
      <div class="ghead">
        <!-- The mode is what holds several things together, so it appears when there are several:
             *all of* over one sentence says nothing and reads as a setting nobody made. -->
        {#if at.length > 0 || current.items.length > 1}
          <select class="mode" aria-label={t('app.flow.composer_mode')} value={current.mode} onchange={(event: Event) => emit(patch(root, at, { ...current, mode: (event.currentTarget as HTMLSelectElement).value as GroupMode }) ?? current)}>
            {#each MODES as mode (mode)}<option value={mode}>{t(`app.flow.composer_mode_${mode}`)}</option>{/each}
          </select>
        {/if}
        {#if at.length > 0}
          <button class="tool" type="button" aria-label={t('app.flow.composer_remove_group')} onclick={() => emit(patch(root, at, undefined) ?? defaultSentence())}>{t('app.flow.composer_remove')}</button>
        {/if}
      </div>
      <div class="rows">
        {#each current.items as item, index (index)}
          <div class="row" class:nested={isGroup(item)}>
            {@render tree(item, [...at, index], root, current.items.length)}
          </div>
        {/each}
      </div>
      <div class="adds">
        <Button size="sm" tone="subtle" icon="plus" onclick={() => emit(patch(root, at, { ...current, items: [...current.items, defaultSentence()] }) ?? current)}>{t(current.items.length > 1 ? 'app.flow.composer_add_sentence' : 'app.flow.composer_add_another')}</Button>
        <Button size="sm" tone="subtle" icon="plus" onclick={() => emit(patch(root, at, { ...current, items: [...current.items, newGroup(current.mode === 'any' ? 'all' : 'any')] }) ?? current)}>{t('app.flow.composer_add_group')}</Button>
      </div>
    </div>
  {:else}
    <div class="sentence" data-sentence={at.join('/') || 'root'}>
      <SentenceRow sentence={current} {choices} onchange={(next) => emit(patch(root, at, next) ?? next)} />
      <!-- The only sentence of the whole condition has nothing to be removed to: a condition is
           an expression, and an empty one is not one. -->
      {#if at.length > 0 && (at.length > 1 || siblings > 1)}
        <button class="tool" type="button" aria-label={t('app.flow.composer_remove_sentence')} onclick={() => emit(patch(root, at, undefined) ?? defaultSentence())}>{t('app.flow.composer_remove')}</button>
      {/if}
    </div>
  {/if}
{/snippet}

<Stack gap="150">
  {#if editingRaw}
    <Textarea
      label={t('app.rules.condition')}
      hint={t('app.flow.composer_expert_hint')}
      {error}
      rows={3}
      value={expr}
      spellcheck={false}
      oninput={(event: Event) => onchange((event.currentTarget as HTMLTextAreaElement).value)}
    />
    {#if foreign}
      <p class="foreign">{t('app.flow.composer_foreign')}</p>
      <button class="link" type="button" onclick={() => emit(defaultSentence())}>{t('app.flow.composer_replace')}</button>
    {:else}
      <Checkbox label={t('app.flow.composer_expert')} checked={expert} onchange={() => (expert = !expert)} />
    {/if}
  {:else}
    {@render tree(root, [], root, 1)}
    <div>
      <span class="label">{t('app.flow.composer_compiled')}</span>
      <code class="compiled" class:refused={Boolean(error)}>{expr || compileNode(root)}</code>
      {#if error}<span class="error">{error}</span>{/if}
    </div>
    <Checkbox label={t('app.flow.composer_expert')} checked={expert} onchange={() => (expert = !expert)} />
  {/if}
</Stack>

<style>
  .group { display: flex; flex-direction: column; gap: var(--sp-100); padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }

  .group[data-depth='0'] { padding: 0; border: 0; background: transparent; }

  .ghead { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  /* The mode reads as the group's heading - "all of these hold" - so it is a plain select in the
     heading's own type rather than a labelled field. */
  .mode { padding: var(--sp-025) var(--sp-050); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-sm); background: var(--bg-surface); color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .rows { display: flex; flex-direction: column; gap: var(--sp-100); }

  .row.nested { padding-inline-start: var(--sp-100); border-inline-start: var(--bw-ring) solid var(--border-subtle); }

  .sentence { display: flex; flex-direction: column; gap: var(--sp-050); }

  .adds { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .tool { align-self: flex-end; padding: 0; border: 0; background: none; color: var(--text-subtle); font-size: var(--fs-050); cursor: pointer; }

  .tool:hover { color: var(--text-danger); }

  .tool:focus-visible, .mode:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .label { display: block; margin-block-end: var(--sp-050); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .compiled {
    display: block;
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    overflow-wrap: anywhere;
  }

  .compiled.refused { outline: var(--bw-hairline) solid var(--danger-500); }

  .error { display: block; margin-block-start: var(--sp-050); font-size: var(--fs-075); color: var(--text-danger); }

  .foreign { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }

  .link { align-self: flex-start; padding: 0; border: 0; background: none; color: var(--text-brand); font-size: var(--fs-075); font-weight: var(--fw-medium); cursor: pointer; }

  .link:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-xs); }
</style>
