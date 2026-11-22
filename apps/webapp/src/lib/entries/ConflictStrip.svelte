<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entry's own word about a conflict on its notes (offline-sync.md §5): a banner
  // that says somebody else changed them while this device was away, and opens the resolver -
  // the same resolver the frame's list opens, over the same conflict record. Nothing without a
  // conflict, which is almost always.
  //
  // The dialog's link to the kept version (`?comment=`) closes it without deciding: closing it
  // otherwise keeps theirs, and following the link is looking, not choosing. *Write mine again*
  // closes it too, and keeps the conflict until the write went through.

  import { Banner, Button, ConflictResolver } from '@hubtask/design-system/components';

  import { commentAddress } from '../data/comments.ts';
  import { items } from '../data/items.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { itemId }: { itemId: string } = $props();

  const conflict = $derived(queue.conflictOn(itemId));
  let isOpen = $state(false);
  let rewriteFailure = $state<string | undefined>(undefined);
  let resolver = $state<HTMLElement | undefined>(undefined);
  let isFollowing = false;
  let isRewriting = false;

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
    if (isFollowing || isRewriting) {
      isFollowing = false;
      isRewriting = false;
    } else void keep();
  }

  async function keep(): Promise<void> {
    isOpen = false;
    if (conflict) await queue.dismissConflict(conflict.id);
  }

  async function rewrite(): Promise<void> {
    if (!conflict) return;
    const { id, mine } = conflict;
    isRewriting = true;
    isOpen = false;
    rewriteFailure = undefined;
    try {
      await items.rewriteNotes(itemId, String(mine ?? ''));
    } catch (error) {
      // Mine was not written: nothing changed, and the choice stays.
      rewriteFailure = renderProblem(error as never, messages).message;
      return;
    }
    await queue.dismissConflict(id);
  }
</script>

{#if conflict}
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
