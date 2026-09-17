<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entry's own word about a conflict on its notes (F6-06, offline-sync.md §5): a banner
  // that says somebody else changed them while this device was away, and opens the resolver -
  // the same resolver the frame's list opens, over the same conflict record. Nothing without a
  // conflict, which is almost always.

  import { Banner, Button, ConflictResolver } from '@hubtask/design-system/components';

  import { items } from '../data/items.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const { itemId }: { itemId: string } = $props();

  const conflict = $derived(queue.conflictOn(itemId));
  let isOpen = $state(false);

  async function keep(): Promise<void> {
    isOpen = false;
    if (conflict) await queue.dismissConflict(conflict.id);
  }

  async function rewrite(): Promise<void> {
    if (!conflict) return;
    const { id, mine } = conflict;
    isOpen = false;
    await items.rewriteNotes(itemId, String(mine ?? ''));
    await queue.dismissConflict(id);
  }
</script>

{#if conflict}
  <Banner tone="warning" title={t('app.sync.conflict.title')}>
    {t('app.sync.conflict.strip')}
    {#snippet action()}
      <Button size="sm" onclick={() => (isOpen = true)}>{t('app.sync.conflict.show')}</Button>
    {/snippet}
  </Banner>
  <ConflictResolver
    bind:isOpen
    title={t('app.sync.conflict.title')}
    fieldLabel={t('app.sync.conflict.field')}
    theirs={String(conflict.theirs ?? '')}
    theirsLabel={t('app.sync.conflict.theirs')}
    mine={String(conflict.mine ?? '')}
    mineLabel={t('app.sync.conflict.mine')}
    preservedHref={conflict.preservedCommentId ? `#comment-${conflict.preservedCommentId}` : undefined}
    preservedLabel={conflict.preservedCommentId ? t('app.sync.conflict.preserved') : undefined}
    keepLabel={t('app.sync.conflict.keep')}
    rewriteLabel={t('app.sync.conflict.rewrite')}
    dismissLabel={t('app.workspace.cancel')}
    onKeep={keep}
    onRewrite={rewrite}
  />
{/if}
