<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A view or a whiteboard living in a page (S-4): the `LiveBlock` head - tile, what it is, *Live*
  // and the way to the original - over the board drawn live. Its title follows the references'
  // resolver, so a renamed view reads as it is called now; a reader who may not see the original
  // gets no way to it (P-19).

  import { LiveBlock } from '@hubtask/design-system/components';
  import type { ReferenceAttrs, ReferenceDrawing } from '@hubtask/editor';

  import BoardView from '../whiteboards/BoardView.svelte';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    ref: ReferenceAttrs;
    drawing: ReferenceDrawing;
    /** Whether the reader may open the original. */
    canOpen: () => boolean;
    onnavigate: (path: string) => void;
  }

  const { ref, drawing, canOpen, onnavigate }: Props = $props();

  const isBoard = $derived(ref.kind.toUpperCase() === 'WHITEBOARD');
  let title = $state<string | undefined>(undefined);
  let mayOpen = $state(false);
  $effect(() => {
    const read = () => {
      title = drawing.label(ref);
      mayOpen = canOpen();
    };
    read();
    return drawing.subscribe(read);
  });
</script>

<LiveBlock
  title={title ?? t('app.pages.reference_loading')}
  icon={isBoard ? 'layout-template' : 'square-check'}
  product={isBoard ? 'whiteboards' : undefined}
  liveLabel={t('app.pages.live')}
  openLabel={mayOpen ? t(isBoard ? 'app.pages.live_open_board' : 'app.pages.live_open_view') : undefined}
  onopen={() => drawing.open?.(ref)}
>
  {#if isBoard}
    <BoardView boardId={ref.id} frame={ref.anchor} onopen={onnavigate} isUnderHead />
  {/if}
</LiveBlock>
