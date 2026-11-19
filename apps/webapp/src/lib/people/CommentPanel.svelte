<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What was said about an entry, and saying something.
  //
  // **The capability decides whether the thread exists at all.** An activity carries no comments,
  // and the manifest is what says so: the entry page draws no tab for it, rather than an empty
  // panel that says "nobody has commented" about a type where nobody ever can.
  //
  // **Edit and delete are offered per comment**, from the two things this client already holds:
  // the account it is signed in as, and the roles the actor holds along the path. That is the
  // server's own rule mirrored, so a control the reader may never use is not drawn rather than
  // refused after they press it (P-05) — and when the mirror is wrong, the server's refusal is
  // what they read.
  //
  // **A body is plain text with its whitespace kept.** Rendering Markdown is a dependency and
  // therefore a proposal rather than a commit, and the notes are shown the same way.

  import { untrack } from 'svelte';

  import { Button, CommentThread, Composer, Stack } from '@hubtask/design-system/components';
  import type { ComposerMode, IconName, ThreadComment, ThreadEvent } from '@hubtask/design-system/components';
  import type { Comment, CommentPage, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { comments, commentsPath } from '../data/comments.svelte.ts';
  import { BODY_LIMIT, bodyLength, isRemoved, isWithinLimit, mayModerate, mayReplyTo, threadOf } from '../data/comments.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import MentionDialog from '../entries/MentionDialog.svelte';
  import ReferenceChip from '../entries/ReferenceChip.svelte';
  import { inserted, referencesIn, resolved } from '../data/links.svelte.ts';
  import { DeskTools } from '../servicedesk/desktools.svelte.ts';
  import EntryPresence from '../entries/EntryPresence.svelte';
  import { quietRowsInView } from './quietrows.ts';
  import RichText, { type Format } from '../entries/RichText.svelte';

  /**
   * `mayComment`: whether the reader may add to this thread, from their role on the entry - a
   * viewer may not, a guest may, a contributor only on what is assigned to them (UC-WRK-19 check 4).
   * Without it there is no field and no reply, rather than a field whose every send is refused;
   * `undefined` while the role is unknown draws neither.
   *
   * `isClosed`: the entry or its collection is archived, and the server refuses every comment,
   * change and removal there (AddComment, ChangeComment). The thread stays readable; nothing on it
   * is offered, and the page's notice says why once.
   */
  const {
    item,
    path,
    mayComment,
    isClosed = false,
    isDesk = false,
    show = 'comments',
    events = [],
    requester,
  }: {
    item: WorkItem;
    path: Path;
    mayComment: boolean | undefined;
    isClosed?: boolean;
    /** The service desk is on: an internal note, a canned reply and a macro are offered (UC-SVC-04, 06). */
    isDesk?: boolean;
    /**
     * Which posts the conversation shows (§6.9): *all* mixes them with `events` as quiet rows;
     * on a ticket *replies* and *notes* split the public answers from the internal notes.
     */
    show?: 'all' | 'comments' | 'replies' | 'notes';
    events?: readonly ThreadEvent[];
    /** Who a public reply on a ticket reaches, for the reply tab's words. */
    requester?: string;
  } = $props();

  /** Whether the next post is a note for the team (ET-7); a reply to an internal note is one anyway. */
  let mode = $state<ComposerMode>(untrack(() => (show === 'notes' ? 'note' : 'reply')));
  const isInternal = $derived(mode === 'note');
  const tools = untrack(() => (isDesk ? new DeskTools(item.id) : undefined));


  // The path is taken once, as every other subscription in this application takes one: the view
  // is rebuilt per entry, so a path that re-derived would resubscribe on every unrelated change.
  const page = resource<CommentPage>({ path: untrack(() => commentsPath(item.id)) });
  const rows = $derived(page.state.status === 'ready' ? (page.state.data.data ?? []) : []);
  const hasMore = $derived(page.state.status === 'ready' && (page.state.data.page?.has_more ?? false));
  /** Why the last press of *Older comments* brought nothing; cleared by the next press. */
  /** How many threads are drawn, newest first: five, then ten more at a time. */
  let shownCount = $state(3);
  /** The docked composer's height, kept free under the thread so the last post is not under it. */
  let dockHeight = $state(0);
  let olderFailed = $state<string | undefined>(undefined);

  async function older(): Promise<void> {
    olderFailed = undefined;
    try {
      await page.loadMore();
    } catch (error) {
      olderFailed = renderProblem(error as never, messages).message;
    }
  }

  const failure = $derived(
    page.state.status === 'failed' ? renderProblem(page.state.error, messages) : undefined,
  );

  /** The roles this reader holds along the entry's path, which is what moderation turns on. */
  const roles = $derived(
    people
      .along(path)
      .filter((membership) => membership.account_id === actor.account?.id)
      .map((membership) => membership.role as string),
  );

  let draft = $state('');
  let replyTo = $state<string | undefined>(undefined);
  let editing = $state<Comment | undefined>(undefined);
  let writeFailure = $state<string | undefined>(undefined);
  let isSending = $state(false);

  const tooLong = $derived(!isWithinLimit(draft));

  $effect(() => {
    accounts.resolve(rows.map((comment) => comment.author_id));
  });

  // What the conversation mentions (UC-PLN-01 check 7): the body stays plain text, and the entries
  // it points at are drawn under the thread, each resolved for this reader - one out of their sight
  // is "not available to you".
  const mentioned = $derived(referencesIn(rows.map((comment) => comment.body ?? '').join('\n')));
  const mentions = resolved(() => mentioned.map((found) => found.reference));
  let isMentionOpen = $state(false);

  function mention(text: string) {
    if (writer) writer.insert(text);
    else draft = inserted(draft, draft.length, draft.length, text).text;
  }

  // Written in the pages' editor (E-3, ET-5): the marks a page has, @ for people and entries, / to
  // insert, still stored as Markdown.
  let writer = $state<{ format(kind: Format): void; insert(markdown: string): void; reset(markdown: string): void; isAtLineStart(): boolean } | undefined>(undefined);
  let composer = $state<{ openSnippets(): void } | undefined>(undefined);
  const WRITE_FORMATS: readonly { id: Format; icon: IconName }[] = [
    { id: 'bold', icon: 'bold' }, { id: 'italic', icon: 'italic' }, { id: 'strike', icon: 'strikethrough' }, { id: 'code', icon: 'code' },
    { id: 'link', icon: 'link' }, { id: 'list', icon: 'list' }, { id: 'checklist', icon: 'list-checks' }, { id: 'quote', icon: 'quote' },
  ];
  /** What `/` offers where no snippets are: the blocks a comment takes, and a mention. */
  const BLOCKS: readonly Format[] = ['list', 'checklist', 'quote', 'code'];
  const insertItems = $derived([
    ...BLOCKS.map((id) => ({ id, label: t(`app.entry.format_${id}`) })),
    { id: 'mention', label: t('app.mentions.insert') },
  ]);
  function pressFormat(id: string) {
    if (id === 'mention') isMentionOpen = true;
    else writer?.format(id as Format);
  }
  // Caught before the editor, whose own Ctrl+Enter is a line break.
  function writingKey(event: KeyboardEvent) {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      send();
    } else if (event.key === '/' && (writer ? writer.isAtLineStart() : draft === '' || draft.endsWith('\n'))) {
      event.preventDefault();
      composer?.openSnippets();
    }
  }
  /** Starts the editor over with what `draft` now holds: one holding the focus takes no value from outside. */
  function rewrite() {
    writer?.reset(draft);
  }

  function threadRows(): readonly ThreadComment[] {
    const kept = rows.filter((comment) => (show === 'replies' ? !comment.internal : show === 'notes' ? comment.internal : true));
    return threadOf(kept).map(({ comment, replies }) => ({
      ...renderOne(comment),
      replies: replies.map(renderOne),
    }));
  }

  function renderOne(comment: Comment): Omit<ThreadComment, 'replies'> {
    const moderates = mayModerate(comment, actor.account?.id, roles);
    const removed = isRemoved(comment);
    return {
      id: comment.id,
      authorName: accounts.nameOf(comment.author_id) ?? t('app.people.unnamed'),
      authorSrc: accounts.pictureOf(comment.author_id),
      body: comment.body ?? undefined,
      when: formatDateTime(comment.created_at, messages.locale),
      at: comment.created_at,
      isRemoved: removed,
      editedLabel: comment.edited_at ? t('app.comments.edited') : undefined,
      internalLabel: comment.internal ? t('app.desk.internal_mark') : undefined,
      // Offered to the author and to an administrator, and to nobody else - not drawn off with a
      // sentence on every comment (UC-WRK-19 check 3, P-05) - and to nobody while it is closed.
      isChangeable: moderates && !isClosed,
    };
  }

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    writeFailure = undefined;
    isSending = true;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      writeFailure = renderProblem(error as never, messages).message;
    }
    isSending = false;
  }

  function send() {
    if (draft.trim() === '' || tooLong) return;
    void attempt(async () => {
      if (editing) await comments.edit(item.id, editing, draft);
      else await comments.add(item.id, draft, replyTo, crypto.randomUUID(), isDesk && isInternal);
      draft = '';
      replyTo = undefined;
      editing = undefined;
      rewrite();
      await page.refresh();
    }, editing ? t('app.comments.edited_announced') : t('app.comments.posted_announced'));
  }

  function startReply(id: string) {
    const target = rows.find((comment) => comment.id === id);
    // A removed comment cannot be replied to: the server refuses it, and offering the control
    // would be offering an action that never works.
    if (!target || !mayReplyTo(target)) {
      writeFailure = t('app.comments.reply_to_removed');
      return;
    }
    editing = undefined;
    replyTo = id;
    draft = '';
    rewrite();
  }

  function startEdit(id: string) {
    const target = rows.find((comment) => comment.id === id);
    if (!target) return;
    replyTo = undefined;
    editing = target;
    draft = target.body ?? '';
    rewrite();
  }

  function remove(id: string) {
    const target = rows.find((comment) => comment.id === id);
    if (!target) return;
    void attempt(async () => {
      await comments.remove(item.id, target);
      await page.refresh();
    }, t('app.comments.removed_announced'));
  }
</script>

<!-- No gate here: the entry page draws this panel only where the type carries the capability
     (`carries`, ItemView), so a refusal could never reach it - a refused capability is an absent
     row, not a dead one (design-system.md §11.8). -->
  {#snippet writing()}
    <div class="writing" role="presentation" onkeydowncapture={writingKey}>
        <RichText bind:this={writer} value={draft} label={editing ? t('app.comments.editing') : replyTo ? t('app.comments.replying') : t('app.comments.write')} placeholder={t('app.comments.write')} editable onchange={(markdown) => (draft = markdown)} />
    </div>
  {/snippet}

  <Stack gap="200">
    <EntryPresence itemId={item.id} replying={draft.trim() !== ''} />
    {#if failure}
      <p class="failure" role="alert">{failure.message}</p>
    {/if}

    <!-- Writing first, the newest three under it (ET-3). One composer for a comment, a reply and
         an internal note (ET-7): on a ticket two tabs, the note yellow with a lock, the button
         saying what it does; snippets and macros as its menus (UC-SVC-06). -->
    {#if mayComment}
      <!-- On a phone the reply is docked at the screen's foot (the scene's *Comment …*), the thread above it. -->
      <!-- Docked, it is fixed on the bottom bar and leaves its height as room under the thread: a
           sticky one stood behind the bar mid-thread and a page's padding above it at the end. -->
      {#if viewport.isCompact}<div class="dock-room" aria-hidden="true" style:block-size={`${dockHeight}px`}></div>{/if}
      <div data-composer data-docked={viewport.isCompact ? '' : undefined} bind:clientHeight={dockHeight}>
        <Composer
          bind:this={composer}
          editor={writing}
          label={editing ? t('app.comments.editing') : replyTo ? t('app.comments.replying') : t('app.comments.write')}
          bind:value={draft}
          bind:mode
          modes={isDesk && !editing ? { reply: requester ? t('app.desk.reply_to', { name: requester }) : t('app.desk.reply_tab'), note: t('app.desk.note_tab'), label: t('app.desk.modes') } : undefined}
          audience={isDesk && requester ? t('app.desk.reply_audience', { name: requester }) : undefined}
          noteAudience={isDesk ? t('app.desk.note_audience') : undefined}
          sendLabel={editing ? t('app.workspace.save') : isDesk ? t('app.desk.send_reply') : t('app.comments.send')}
          sendTone={isDesk && !editing ? 'secondary' : 'primary'}
          noteSendLabel={t('app.desk.save_note')}
          hint={t('app.comments.send_hint')}
          isBusy={isSending || (tools?.isBusy ?? false)}
          busyLabel={t('app.workspace.saving')}
          onsend={() => send()}
          format={{ label: t('app.comments.format'), items: [...WRITE_FORMATS.map((each) => ({ id: each.id, label: t(`app.entry.format_${each.id}`), icon: each.icon })), { id: 'mention', label: t('app.mentions.insert'), icon: 'at-sign' }], onformat: pressFormat }}
          snippets={tools && tools.snippets.length > 0
            ? { label: t('app.desk.snippets'), items: tools.snippets, onselect: (key) => void tools.insert(key, (text) => (writer ? writer.insert(text) : (draft = draft.trim() === '' ? text : `${draft}\n\n${text}`))) }
            : { label: t('app.comments.insert'), items: insertItems, onselect: pressFormat }}
          macros={tools && tools.macros.length > 0 ? { label: t('app.desk.macros'), title: t('app.desk.macros_title'), items: tools.macros, onselect: (id) => void tools.press(id) } : undefined}
        />
        {#if tooLong}<p class="failure" role="alert">{t('app.comments.too_long', { limit: String(BODY_LIMIT), length: String(bodyLength(draft)) })}</p>{/if}
        {#if tools?.refusal}<p class="failure" role="alert">{tools.refusal}</p>{/if}
        {#if writeFailure}<p class="failure" role="alert">{writeFailure}</p>{/if}
        {#if replyTo || editing}
          <Button size="sm" tone="secondary" onclick={() => { replyTo = undefined; editing = undefined; draft = ''; rewrite(); }}>{t('app.workspace.cancel')}</Button>
        {/if}
      </div>
    {/if}
    <CommentThread
      label={t('app.comments.title')}
      comments={threadRows().slice(0, shownCount)}
      body={formatted}
      events={show === 'all' ? quietRowsInView(events, threadRows().slice(0, shownCount)) : []}
      emptyLabel={t(mayComment === false ? 'app.comments.none_closed' : 'app.comments.none')}
      removedLabel={t('app.comments.removed')}
      editLabel={t('app.comments.edit')}
      deleteLabel={t('app.comments.remove')}
      replyLabel={t('app.comments.reply')}
      moreLabel={t('app.comments.more')}
      hasMore={hasMore || threadRows().length > shownCount}
      loadMoreLabel={t('app.comments.older')}
      loadMoreFailedLabel={olderFailed}
      onLoadMore={() => {
        // What is held here first, ten at a time; then the server's older page.
        if (threadRows().length > shownCount) shownCount += 10;
        else void older();
      }}
      onEdit={startEdit}
      onDelete={remove}
      onReply={mayComment ? startReply : undefined}
    />
    {#if mentioned.length > 0}
      <div class="mentions" data-comment-mentions>
        <h3 class="mentions-title">{t('app.mentions.title')}</h3>
        <ul class="mentions-list">
          {#each mentioned as found, index (`${found.reference.kind}/${found.reference.id}#${found.reference.anchor ?? ''}`)}
            <li><ReferenceChip target={mentions.targets[index]} strength={found.strength} /></li>
          {/each}
        </ul>
      </div>
    {/if}
  </Stack>
  {#if mayComment}<MentionDialog bind:isOpen={isMentionOpen} selfId={item.id} oninsert={mention} />{/if}

{#snippet formatted(markdown: string)}<RichText value={markdown} label={t('app.comments.title')} />{/snippet}

<style>
  [data-composer] { display: flex; flex-direction: column; gap: var(--sp-100); }
  [data-composer][data-docked] {
    position: fixed;
    inset-inline: 0;
    inset-block-end: calc(var(--layout-bottombar-height) + env(safe-area-inset-bottom, 0));
    z-index: var(--z-sticky);
    padding: var(--sp-100) var(--sp-200);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-canvas);
  }
  /* The composer's own phone dock is this one's: nested, the two offsets added up. */
  [data-composer][data-docked] :global(.composer) { position: static; box-shadow: none; }
  .dock-room { order: 1; flex: none; }

  .failure { margin: 0; color: var(--text-danger); }

  .mentions { display: flex; flex-direction: column; gap: var(--sp-050); }
  .mentions-title { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .mentions-list { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
</style>
