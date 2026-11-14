<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The whiteboards' *For you* (scene whome): the boards last opened on this device (`opened.ts`),
  // else the last changed, as drawings of their objects with who made and last changed them (W-7), a row of templates that each start a board in one
  // press, and the open comments that name the reader, each opening its board at the thread.
  //
  // The comments are read on the boards listed only: a thread on a board nobody has touched for a
  // long time is out of reach here, as it is out of the list.

  import { untrack } from 'svelte';
  import { Icon, ListRow, Stack } from '@hubtask/design-system/components';
  import type { Whiteboard, WhiteboardCommentList, WhiteboardRead } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { openToYou, type ToYou } from '../data/whiteboards.ts';
  import { boardCommentsPath, boardPath } from '../data/whiteboards.svelte.ts';
  import { formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import BoardCard from './BoardCard.svelte';
  import BoardTemplates, { STARTS, type BoardStart } from './BoardTemplates.svelte';

  type Comment = WhiteboardCommentList['data'][number];

  interface Props {
    /** The boards to read comments on, the last changed first. */
    boards: readonly Whiteboard[];
    /** The boards drawn first: those opened here last (`opened`), else the last changed. */
    drawn: readonly Whiteboard[];
    opened: boolean;
    hubName: (id: string) => string;
    /** Starts a board from a template card; absent where the reader may make none. */
    onstart?: (choice: BoardStart) => void;
    /** How many open comments name the reader, for the band. */
    oncomments?: (count: number) => void;
    onnavigate: (path: string) => void;
  }

  const { boards, drawn, opened, hubName, onstart, oncomments, onnavigate }: Props = $props();

  const ROW: readonly BoardStart[] = ['RETRO_START_STOP_CONTINUE', 'RETRO_FOUR_LS', 'RETRO_MAD_SAD_GLAD', 'BRAINSTORM', 'FLOWCHART'];

  let commentsAt = $state<Record<string, readonly Comment[]>>({});
  const watched = $derived(boards.map((board) => board.id).join(','));
  $effect(() => {
    const ids = watched === '' ? [] : watched.split(',');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<WhiteboardCommentList>({ path: boardCommentsPath(id) }, (next) => {
        if (next.status === 'ready') commentsAt = { ...commentsAt, [id]: next.data.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });

  const me = $derived(actor.account?.id ?? '');
  const toYou = $derived(
    me
      ? boards.flatMap((board) => openToYou(commentsAt[board.id] ?? [], me).map((each) => ({ ...each, board }))).sort((a, b) => (a.said.created_at < b.said.created_at ? 1 : -1))
      : ([] as (ToYou<Comment> & { board: Whiteboard })[]),
  );
  $effect(() => oncomments?.(toYou.length));
  $effect(() => {
    const ids = [...drawn.flatMap((board) => [board.created_by, board.updated_by]), ...toYou.map((each) => each.said.author_id)];
    untrack(() => accounts.resolve(ids));
  });

  /** The objects of the boards a comment names the reader on, for the words of the object it is pinned to. */
  let wordsAt = $state<Record<string, Record<string, { kind: string; text: string }>>>({});
  const named = $derived([...new Set(toYou.filter((each) => each.thread.element_id).map((each) => each.board.id))].join(','));
  $effect(() => {
    const ids = named === '' ? [] : named.split(',');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<WhiteboardRead>({ path: boardPath(id) }, (next) => {
        if (next.status !== 'ready') return;
        const objects = (next.data.elements ?? []) as readonly { id: string; kind: string; text?: string | null }[];
        wordsAt = { ...wordsAt, [id]: Object.fromEntries(objects.map((e) => [e.id, { kind: e.kind, text: e.text ?? '' }])) };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });

  const short = (words: string) => (words.length > 40 ? `${words.slice(0, 39).trimEnd()} …` : words);
  function whereOf(each: ToYou<Comment> & { board: Whiteboard }): string {
    const object = each.thread.element_id ? wordsAt[each.board.id]?.[each.thread.element_id] : undefined;
    if (!object?.text) return each.board.title;
    return t(object.kind === 'FRAME' ? 'app.product_home.boards_comment_in' : 'app.product_home.boards_comment_on', { board: each.board.title, words: short(object.text) });
  }
  const peopleOf = (board: Whiteboard) =>
    [...new Set([board.created_by, board.updated_by].filter((id): id is string => !!id))].map((id) => accounts.nameOf(id) ?? '').filter(Boolean);
  const at = (each: { board: Whiteboard; thread: Comment }) => `/whiteboards/${each.board.id}?comment=${each.thread.id}`;
</script>

{#if drawn.length > 0}
  <section aria-labelledby="boards-recent" data-boards-recent>
    <Stack gap="100">
      <h2 class="heading" id="boards-recent">{t(opened ? 'app.product_home.boards_opened' : 'app.product_home.boards_recent')}</h2>
      <ul class="cards">
        {#each drawn as board (board.id)}
          <li>
            <BoardCard {board} people={peopleOf(board)} meta={[hubName(board.hub_id), t('app.whiteboards.objects', { count: String(board.element_count) }), formatRelative(board.updated_at, messages.locale)].filter(Boolean).join(' · ')} onopen={onnavigate} />
          </li>
        {/each}
      </ul>
    </Stack>
  </section>
{/if}

{#if onstart}
  <section aria-labelledby="boards-start" data-boards-templates>
    <Stack gap="100">
      <div class="head">
        <h2 class="heading" id="boards-start">{t('app.product_home.boards_start')}</h2>
        <a class="more" href="/whiteboards?tab=templates" onclick={(event) => { event.preventDefault(); onnavigate('/whiteboards?tab=templates'); }}>
          {t('app.product_home.boards_all_templates', { count: STARTS.length })}<Icon name="chevron-right" size="sm" />
        </a>
      </div>
      <BoardTemplates look="tiles" only={ROW} onpick={onstart} />
    </Stack>
  </section>
{/if}

{#if toYou.length > 0}
  <section class="to-you" aria-labelledby="boards-comments" data-boards-comments>
    <h2 class="to-you-head" id="boards-comments"><span class="mark"><Icon name="message-circle" size="sm" /></span>{t('app.product_home.boards_comments', { count: toYou.length })}</h2>
    <ul class="rows">
      {#each toYou as each (each.said.id)}
        <li data-board-comment={each.thread.id}>
          <ListRow href={at(each)} onactivate={() => onnavigate(at(each))} label={t('app.product_home.boards_comment_open')}>
            {#snippet leading()}<span class="tile"><Icon name="layout-template" size="sm" /></span>{/snippet}
            <span class="said">{t('app.product_home.boards_comment_said', { name: accounts.nameOf(each.said.author_id) ?? t('app.people.unnamed'), words: each.words })}</span>
            <span class="where">{whereOf(each)}</span>
            {#snippet trailing()}<span class="open">{t('app.product_home.boards_comment_open')}</span>{/snippet}
          </ListRow>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .heading { margin: 0; color: var(--text-subtle); font-size: var(--fs-065); font-weight: var(--fw-semibold); text-transform: uppercase; }
  /* Three to a row where they fit, as wide as the row allows; one opened board keeps a third. */
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, max(var(--layout-gallery-card), calc((100% - 2 * var(--sp-200)) / 3))), 1fr)); gap: var(--sp-200); margin: 0; padding: 0; list-style: none; }
  .head { display: flex; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .more { display: inline-flex; align-items: center; gap: var(--sp-025); color: var(--text-brand); font-size: var(--fs-075); font-weight: var(--fw-semibold); text-decoration: none; }
  .more:hover { text-decoration: underline; }
  .to-you { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); overflow: hidden; }
  .to-you-head { display: flex; align-items: center; gap: var(--sp-100); margin: 0; padding: var(--sp-150) var(--sp-200); border-block-end: var(--bw-hairline) solid var(--border-subtle); font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .mark, .tile { display: inline-grid; place-items: center; inline-size: var(--sp-300); block-size: var(--sp-300); border-radius: var(--r-md); background: var(--label-amber-bg); color: var(--label-amber-fg); }
  .rows { margin: 0; padding: 0; list-style: none; }
  .rows li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .said { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .where { display: block; color: var(--text-secondary); font-size: var(--fs-075); }
  .open { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
</style>
