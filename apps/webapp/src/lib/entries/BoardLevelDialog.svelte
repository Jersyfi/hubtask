<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The level switches of a collection's tasks (build-plan §5), each previewed before it is made:
  // *Als Board anzeigen* (L0 → L1: two columns, the done entries in the done one), *Zurück zur
  // Liste* (L1 → L0: the columns go, completion stays) and *Workflow aus diesen Spalten* (L1 → L2:
  // a status per column). The preview is the server's own dry run - counts, never a guess.
  import { untrack } from 'svelte';
  import { Dialog } from '@hubtask/design-system/components';
  import type { BoardSwitch, BoardWorkflow } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { boardLevels } from '../data/buckets.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import type { LevelSwitch } from './boardlevel.ts';

  interface Props {
    collectionId: string;
    asked: LevelSwitch;
    onclose: () => void;
    /** After the switch is made: the layout to draw now. */
    ondone?: (asked: LevelSwitch) => void;
  }

  const { collectionId, asked, onclose, ondone }: Props = $props();

  let preview = $state<BoardSwitch | BoardWorkflow | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isBusy = $state(false);

  async function ask(dryRun: boolean): Promise<BoardSwitch | BoardWorkflow> {
    return asked === 'workflow'
      ? boardLevels.makeWorkflow(collectionId, dryRun)
      : boardLevels.switchTo(collectionId, asked === 'board' ? 'COLUMNS' : 'NONE', dryRun);
  }

  $effect(() => {
    void asked;
    untrack(() => {
      preview = undefined;
      failure = undefined;
      ask(true).then((answer) => (preview = answer), (error) => (failure = renderProblem(error as never, messages).message));
    });
  });

  async function confirm() {
    isBusy = true;
    failure = undefined;
    try {
      await ask(false);
      announcer.say(t(`app.board.level.${asked}_done`));
      ondone?.(asked);
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  const switched = $derived(preview && 'entries_placed' in preview ? preview : undefined);
  const made = $derived(preview && 'statuses' in preview ? preview : undefined);
</script>

<Dialog
  size="m"
  isOpen={true}
  onClose={onclose}
  title={t(`app.board.level.${asked}_title`)}
  dismissLabel={t('app.workspace.cancel')}
  primary={{ label: t(`app.board.level.${asked}_confirm`), onclick: () => void confirm(), isBusy, disabledReason: preview ? undefined : failure ?? t('app.board.level.reading') }}
>
  <p class="lead">{t(`app.board.level.${asked}_lead`)}</p>
  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  {#if switched}
    <ul class="facts" data-level-preview>
      {#if switched.columns_created > 0}<li>{t('app.board.level.columns_created', { count: switched.columns_created })}</li>{/if}
      {#if switched.columns_removed > 0}<li>{t('app.board.level.columns_removed', { count: switched.columns_removed })}</li>{/if}
      <li>{t('app.board.level.entries_placed', { count: switched.entries_placed })}</li>
      {#if asked === 'list'}<li>{t('app.board.level.completion_kept')}</li>{/if}
    </ul>
  {:else if made}
    <ul class="mapping" data-level-preview>
      {#each made.statuses as status (status.key)}
        <li data-mapping-row><span class="name">{status.name}</span><span aria-hidden="true">→</span><span>{t(`app.board.level.category.${status.category}`)}</span><span class="count">{t('app.board.level.entries_count', { count: status.count })}</span></li>
      {/each}
    </ul>
  {/if}
</Dialog>

<style>
  .lead { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); }
  .facts { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }
  .mapping { display: grid; margin: 0; padding: 0; list-style: none; }
  .mapping li { display: flex; gap: var(--sp-100); align-items: baseline; padding-block: var(--sp-050); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .mapping .name { font-weight: var(--fw-medium); }
  .mapping .count { margin-inline-start: auto; color: var(--text-subtle); }
</style>
