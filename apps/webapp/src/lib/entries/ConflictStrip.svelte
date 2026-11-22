<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  /**
   * A text of the entry somebody else changed while the reader was writing it (UC-SYN-02
   * check 10): nothing is written yet, and the reader chooses. `failure` is the resolved sentence
   * of a choice that did not go through.
   */
  export interface Meanwhile {
    readonly field: 'title' | 'notes';
    readonly mine: string;
    readonly theirs: string;
    readonly failure?: string;
  }
</script>

<script lang="ts">
  // The entry's own word about a text two people changed at once (offline-sync.md §5), from
  // either source: a conflict the push answered while this device was away - theirs is in place,
  // mine already a comment - or one met here while the reader was writing, where nothing is
  // decided yet. Either way a warning banner that opens the same resolver with both versions, and
  // whichever version loses is kept as a comment on the entry. Nothing without a conflict, which
  // is almost always.
  //
  // The offline dialog's link to the kept version (`?comment=`) closes it without deciding: closing
  // it otherwise keeps theirs, and following the link is looking, not choosing.

  import { Banner, Button, ConflictResolver } from '@hubtask/design-system/components';

  import { commentAddress } from '../data/comments.ts';
  import { items } from '../data/items.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const {
    itemId,
    meanwhile,
    kept,
    onkeep,
    onwrite,
    onshowcomment,
    onclosekept,
  }: {
    itemId: string;
    /** A text changed meanwhile, held by the screen that writes it. */
    meanwhile?: Meanwhile;
    /** The version a choice just kept as a comment: the reader's own, or the one replaced. */
    kept?: 'MINE' | 'REPLACED';
    onkeep?: () => void;
    onwrite?: () => void;
    onshowcomment?: () => void;
    onclosekept?: () => void;
  } = $props();

  const conflict = $derived(queue.conflictOn(itemId));
  let isOpen = $state(false);
  let rewriteFailure = $state<string | undefined>(undefined);
  let resolver = $state<HTMLElement | undefined>(undefined);
  let isFollowing = false;

  $effect(() => {
    const node = resolver;
    if (!node) return;
    const follow = (event: MouseEvent) => {
      if (!(event.target as Element | null)?.closest('a[href]')) return;
      isFollowing = true;
      isOpen = false;
    };
    node.addEventListener('click', follow, true);
    return () => node.removeEventListener('click', follow, true);
  });

  function closed(): void {
    if (isFollowing) isFollowing = false;
    else void keep();
  }

  async function keep(): Promise<void> {
    isOpen = false;
    if (conflict) await queue.dismissConflict(conflict.id);
  }

  async function rewrite(): Promise<void> {
    if (!conflict) return;
    const { id, mine } = conflict;
    isOpen = false;
    rewriteFailure = undefined;
    try {
      await items.rewriteNotes(itemId, String(mine ?? ''));
    } catch (error) {
      // Theirs could not be kept, or mine not written: nothing changed, and the choice stays.
      rewriteFailure = renderProblem(error as never, messages).message;
      return;
    }
    await queue.dismissConflict(id);
  }

  function choose(choice: (() => void) | undefined): void {
    isOpen = false;
    choice?.();
  }
</script>

{#if meanwhile}
  <div data-meanwhile={meanwhile.field}>
    <Banner tone="warning" title={meanwhile.field === 'title' ? t('app.sync.meanwhile.title_title') : t('app.sync.meanwhile.title_notes')}>
      {meanwhile.field === 'title' ? t('app.sync.meanwhile.text_title') : t('app.sync.meanwhile.text_notes')}
      {#if meanwhile.failure}<span class="failure" data-meanwhile-failure>{meanwhile.failure}</span>{/if}
      {#snippet action()}
        <Button size="sm" tone="primary" onclick={() => (isOpen = true)}>{t('app.sync.meanwhile.compare')}</Button>
      {/snippet}
    </Banner>
  </div>
  <ConflictResolver
    bind:isOpen
    title={meanwhile.field === 'title' ? t('app.sync.meanwhile.dialog_title') : t('app.sync.meanwhile.dialog_notes')}
    fieldLabel={t('app.sync.meanwhile.field')}
    theirs={meanwhile.theirs}
    theirsLabel={t('app.sync.conflict.theirs')}
    mine={meanwhile.mine}
    mineLabel={t('app.sync.meanwhile.mine')}
    keepLabel={t('app.sync.conflict.keep')}
    rewriteLabel={t('app.sync.meanwhile.write')}
    dismissLabel={t('app.workspace.cancel')}
    onKeep={() => choose(onkeep)}
    onRewrite={() => choose(onwrite)}
    onDismiss={() => (isOpen = false)}
  />
{:else if kept}
  <div data-meanwhile-kept={kept}>
    <Banner tone="success" title={kept === 'MINE' ? t('app.sync.meanwhile.kept_mine') : t('app.sync.meanwhile.kept_theirs')} dismissLabel={t('app.sync.dismiss')} onDismiss={onclosekept}>
      {#snippet action()}
        <Button size="sm" tone="secondary" onclick={onshowcomment}>{t('app.sync.meanwhile.show_comment')}</Button>
      {/snippet}
    </Banner>
  </div>
{:else if conflict}
  <Banner tone="warning" title={t('app.sync.conflict.title')}>
    {t('app.sync.conflict.strip')}
    {#if rewriteFailure}<span class="failure">{rewriteFailure}</span>{/if}
    {#snippet action()}
      <Button size="sm" onclick={() => (isOpen = true)}>{t('app.sync.conflict.show')}</Button>
    {/snippet}
  </Banner>
  <div class="resolver" bind:this={resolver}>
  <ConflictResolver
    bind:isOpen
    title={t('app.sync.conflict.title')}
    fieldLabel={t('app.sync.conflict.field')}
    theirs={String(conflict.theirs ?? '')}
    theirsLabel={t('app.sync.conflict.theirs')}
    mine={String(conflict.mine ?? '')}
    mineLabel={t('app.sync.conflict.mine')}
    preservedHref={conflict.preservedCommentId ? commentAddress(itemId, conflict.preservedCommentId) : undefined}
    preservedLabel={conflict.preservedCommentId ? t('app.sync.conflict.preserved') : undefined}
    keepLabel={t('app.sync.conflict.keep')}
    rewriteLabel={t('app.sync.conflict.rewrite')}
    dismissLabel={t('app.workspace.cancel')}
    onKeep={closed}
    onRewrite={rewrite}
  />
  </div>
{/if}

<style>
  .resolver { display: contents; }

  /* The reason a choice did not go through, on its own line under the banner's sentence. */
  .failure {
    display: block;
    margin-block-start: var(--sp-050);
  }
</style>
