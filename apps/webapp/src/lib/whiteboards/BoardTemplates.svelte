<script lang="ts" module>
  import { template, type Element, type TemplateName } from '@hubtask/whiteboard';

  /** A card of the gallery: one template, each retrospective its own (scene wnew), or blank. */
  export type BoardStart = 'BLANK' | TemplateName;

  /** In the order the new board's sheet draws them (scene wnew): blank, the three retrospectives, the rest. */
  export const STARTS: readonly BoardStart[] = [
    'BLANK', 'RETRO_START_STOP_CONTINUE', 'RETRO_FOUR_LS', 'RETRO_MAD_SAD_GLAD', 'KANBAN', 'BRAINSTORM', 'FLOWCHART', 'USER_STORY_MAP',
  ];

  /** The template a choice puts, or none for a blank board. */
  export const templateOf = (start: BoardStart): TemplateName | undefined => (start === 'BLANK' ? undefined : start);

  /** A template's objects centred on the board's origin, in the reader's words. */
  export function startElements(name: TemplateName, words: (key: string) => string, newId: () => string): Element[] {
    return template(name, { at: { x: 0, y: 0 }, words, newId });
  }
</script>

<script lang="ts">
  // The board templates (user-experience.md §5.2, §7.2) in two looks. As a gallery that is a choice
  // (`gallery`): pressing a card only chooses it, nothing lands until the dialog's own act (SC 3.2.2).
  // As tiles (`tiles`, scenes whome and wnew): each card is one button whose press is the act -
  // the sheet of a new board and the home's row, where choosing and starting are one thing.

  import { Button, TemplateCard, TemplateGallery, VisuallyHidden } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import BoardThumb from './BoardThumb.svelte';

  interface Props {
    chosen?: BoardStart;
    /** Whether *Blank* is offered: a new board, not a template put on one already drawn. */
    withBlank?: boolean;
    /** The cards offered, in this order, where not every one is (the home's row). */
    only?: readonly BoardStart[];
    look?: 'gallery' | 'tiles';
    /** A press starts from the card instead of choosing it in a dialog. */
    onpick?: (choice: BoardStart) => void;
  }

  let { chosen = $bindable('BLANK'), withBlank = true, only, look = 'gallery', onpick }: Props = $props();

  const offered = $derived(only ?? (withBlank ? STARTS : STARTS.filter((each) => each !== 'BLANK')));
  let counter = 0;
  const previewId = () => `preview-${++counter}`;
  const previews = $derived(
    new Map(offered.map((each) => {
      const name = templateOf(each);
      return [each, name ? startElements(name, (key) => t(`app.whiteboards.word_${key}`), previewId) : []] as const;
    })),
  );
  const nameOf = (each: BoardStart) => t(`app.whiteboards.start_${each.toLowerCase()}`);
  const hintOf = (each: BoardStart) => t(`app.whiteboards.start_${each.toLowerCase()}_hint`);
</script>

{#if look === 'tiles'}
  <ul class="tiles" aria-label={t('app.whiteboards.templates')}>
    {#each offered as each (each)}
      {@const objects = previews.get(each) ?? []}
      <li data-board-start={each}>
        <button type="button" class="tile" aria-pressed={onpick ? undefined : chosen === each} onclick={() => { chosen = each; onpick?.(each); }}>
          <span class="preview" aria-hidden="true">{#if objects.length > 0}<BoardThumb elements={objects} />{/if}</span>
          <span class="name">{nameOf(each)}</span>
          <span class="hint">{hintOf(each)}</span>
        </button>
      </li>
    {/each}
  </ul>
{:else}
<div class="templates">
  <TemplateGallery label={t('app.whiteboards.templates')} isLabelHidden>
    {#each offered as each (each)}
      {@const objects = previews.get(each) ?? []}
      <TemplateCard name={nameOf(each)} summary={hintOf(each)} isChosen={!onpick && chosen === each} isCompact={!onpick} data-board-start={each}>
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
</div>
{/if}

<style>
  .templates { display: grid; gap: var(--sp-150); }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1600) * 2.4), 1fr));
    gap: var(--sp-150);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .tile {
    display: grid;
    grid-template-rows: auto auto 1fr;
    align-content: start;
    gap: var(--sp-025);
    inline-size: 100%;
    block-size: 100%;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
    transition: border-color var(--motion-state-duration) var(--motion-state-easing), box-shadow var(--motion-state-duration) var(--motion-state-easing);
  }
  .tile:hover { border-color: var(--border-default); box-shadow: var(--shadow-raised); }
  .tile[aria-pressed='true'] { border-color: var(--accent-primary); box-shadow: 0 0 0 var(--bw-hairline) var(--accent-primary); }
  .tile:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-ring); }
  .preview {
    display: block;
    block-size: var(--sp-1000);
    margin-block-end: var(--sp-100);
    padding: var(--sp-050);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    overflow: hidden;
  }
  .name { font-weight: var(--fw-semibold); }
  .hint { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
