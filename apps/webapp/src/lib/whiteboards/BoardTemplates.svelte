<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import { template, type Element, type TemplateName } from '@hubtask/whiteboard';

  /** A card of the gallery: one template, the retrospectives as one card with a format, or blank. */
  export type BoardStart = 'BLANK' | 'RETRO' | Exclude<TemplateName, `RETRO_${string}`>;
  export type RetroFormat = Extract<TemplateName, `RETRO_${string}`>;

  export const RETRO_FORMATS: readonly RetroFormat[] = ['RETRO_START_STOP_CONTINUE', 'RETRO_FOUR_LS', 'RETRO_MAD_SAD_GLAD'];
  /** In the order user-experience.md §4 offers them; the flowchart last, as the one not named there. */
  export const STARTS: readonly BoardStart[] = ['BLANK', 'RETRO', 'BRAINSTORM', 'USER_STORY_MAP', 'KANBAN', 'FLOWCHART'];

  /** The template a choice puts, or none for a blank board. */
  export const templateOf = (start: BoardStart, format: RetroFormat): TemplateName | undefined =>
    start === 'BLANK' ? undefined : start === 'RETRO' ? format : start;

  /** A template's objects centred on the board's origin, in the reader's words. */
  export function startElements(name: TemplateName, words: (key: string) => string, newId: () => string): Element[] {
    return template(name, { at: { x: 0, y: 0 }, words, newId });
  }
</script>

<script lang="ts">
  // The board templates as a gallery that is a choice (user-experience.md §5.2, §7.2): each card
  // shows a drawing of what it puts, pressing a card only chooses it - nothing lands on a board
  // until the dialog's own act (SC 3.2.2) - and the retrospectives are one card with a format.

  import { Button, Select, TemplateCard, TemplateGallery, VisuallyHidden } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import BoardThumb from './BoardThumb.svelte';

  interface Props {
    chosen: BoardStart;
    format: RetroFormat;
    /** Whether *Blank* is offered: a new board, not a template put on one already drawn. */
    withBlank?: boolean;
    /** One press starts from the card (an empty hub's gallery) instead of choosing it in a dialog. */
    onpick?: (choice: BoardStart) => void;
  }

  let { chosen = $bindable(), format = $bindable(), withBlank = true, onpick }: Props = $props();

  const offered = $derived(withBlank ? STARTS : STARTS.filter((each) => each !== 'BLANK'));
  let counter = 0;
  const previewId = () => `preview-${++counter}`;
  const previews = $derived(
    new Map(offered.map((each) => {
      const name = templateOf(each, format);
      return [each, name ? startElements(name, (key) => t(`app.whiteboards.word_${key}`), previewId) : []] as const;
    })),
  );
  const nameOf = (each: BoardStart) => t(`app.whiteboards.start_${each.toLowerCase()}`);
</script>

<div class="templates">
  <TemplateGallery label={t('app.whiteboards.templates')} isLabelHidden>
    {#each offered as each (each)}
      {@const objects = previews.get(each) ?? []}
      <TemplateCard name={nameOf(each)} summary={t(`app.whiteboards.start_${each.toLowerCase()}_hint`)} isChosen={!onpick && chosen === each} isCompact={!onpick} data-board-start={each}>
        {#snippet preview()}
          {#if objects.length > 0}<BoardThumb elements={objects} />{/if}
        {/snippet}
        {#snippet action()}
          {#if onpick}
            <Button size="sm" tone={each === 'BLANK' ? 'secondary' : 'primary'} onclick={() => onpick(each)}>
              {t('app.whiteboards.start_use')}<VisuallyHidden>{` ${nameOf(each)}`}</VisuallyHidden>
            </Button>
          {:else}
            <Button size="sm" tone={chosen === each ? 'primary' : 'secondary'} icon={chosen === each ? 'check' : undefined} aria-pressed={chosen === each} onclick={() => (chosen = each)}>
              {t('app.whiteboards.start_pick')}<VisuallyHidden>{` ${nameOf(each)}`}</VisuallyHidden>
            </Button>
          {/if}
        {/snippet}
      </TemplateCard>
    {/each}
  </TemplateGallery>
  {#if !onpick && chosen === 'RETRO'}
    <Select
      label={t('app.whiteboards.retro_format')}
      bind:value={format}
      options={RETRO_FORMATS.map((each) => ({ value: each, label: t(`app.whiteboards.template_${each.toLowerCase()}`) }))}
    />
  {/if}
</div>

<style>
  .templates { display: grid; gap: var(--sp-150); }
</style>
