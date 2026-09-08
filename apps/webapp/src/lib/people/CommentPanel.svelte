<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What was said about an entry, and saying something.
  //
  // **The capability decides whether the thread exists at all.** An activity carries no comments,
  // and the manifest is what says so — a gate with the reason, not an empty panel, because an
  // empty panel says "nobody has commented" about a type where nobody ever can.
  //
  // **Edit and delete are switched on per comment**, from the two things this client already
  // holds: the account it is signed in as, and the roles the actor holds along the path. That is
  // the server's own rule mirrored, so a control the reader cannot use is off with a reason rather
  // than refused after they press it — and when the mirror is wrong, the server's refusal is what
  // they read.
  //
  // **A body is plain text with its whitespace kept.** Rendering Markdown is a dependency and
  // therefore a proposal rather than a commit, and the notes F2-09 built are shown the same way.

  import { untrack } from 'svelte';

  import { Button, CapabilityGate, CommentThread, Stack, Textarea } from '@hubtask/design-system/components';
  import type { ThreadComment } from '@hubtask/design-system/components';
  import type { Comment, CommentPage, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { comments, commentsPath } from '../data/comments.svelte.ts';
  import { BODY_LIMIT, bodyLength, isRemoved, isWithinLimit, mayModerate, mayReplyTo, threadOf } from '../data/comments.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item, path }: { item: WorkItem; path: Path } = $props();

  const capability = $derived(supports(item.type, 'COMMENTS'));

  // The path is taken once, as every other subscription in this application takes one: the view
  // is rebuilt per entry, so a path that re-derived would resubscribe on every unrelated change.
  const page = resource<CommentPage>({ path: untrack(() => commentsPath(item.id)) });
  const rows = $derived(page.state.status === 'ready' ? (page.state.data.data ?? []) : []);
  const hasMore = $derived(page.state.status === 'ready' && (page.state.data.page?.has_more ?? false));
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

  function threadRows(): readonly ThreadComment[] {
    return threadOf(rows).map(({ comment, replies }) => ({
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
      body: comment.body ?? undefined,
      when: formatDateTime(comment.created_at, messages.locale),
      at: comment.created_at,
      isRemoved: removed,
      editedLabel: comment.edited_at ? t('app.comments.edited') : undefined,
      editDisabledReason: moderates ? undefined : t('app.comments.not_yours'),
      deleteDisabledReason: moderates ? undefined : t('app.comments.not_yours'),
    };
  }

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    writeFailure = undefined;
    isSending = true;
    try {
      await work();
    } catch (error) {
      writeFailure = renderProblem(error as never, messages).message;
    }
    isSending = false;
  }

  function send() {
    if (draft.trim() === '' || tooLong) return;
    void attempt(async () => {
      if (editing) await comments.edit(item.id, editing, draft);
      else await comments.add(item.id, draft, replyTo, crypto.randomUUID());
      draft = '';
      replyTo = undefined;
      editing = undefined;
      await page.refresh();
    });
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
  }

  function startEdit(id: string) {
    const target = rows.find((comment) => comment.id === id);
    if (!target) return;
    replyTo = undefined;
    editing = target;
    draft = target.body ?? '';
  }

  function remove(id: string) {
    const target = rows.find((comment) => comment.id === id);
    if (!target) return;
    void attempt(async () => {
      await comments.remove(item.id, target);
      await page.refresh();
    });
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.comments.deciding')}
>
  <Stack gap="200">
    {#if failure}
      <p class="failure">{failure.message}</p>
    {/if}

    <CommentThread
      label={t('app.comments.title')}
      comments={threadRows()}
      emptyLabel={t('app.comments.none')}
      removedLabel={t('app.comments.removed')}
      editLabel={t('app.comments.edit')}
      deleteLabel={t('app.comments.remove')}
      replyLabel={t('app.comments.reply')}
      {hasMore}
      loadMoreLabel={t('app.comments.older')}
      onLoadMore={() => void page.loadMore()}
      onEdit={startEdit}
      onDelete={remove}
      onReply={startReply}
    />

    <Stack gap="100">
      <Textarea
        label={editing
          ? t('app.comments.editing')
          : replyTo
            ? t('app.comments.replying')
            : t('app.comments.write')}
        bind:value={draft}
        rows={3}
        error={tooLong ? t('app.comments.too_long', { limit: String(BODY_LIMIT), length: String(bodyLength(draft)) }) : undefined}
      />
      {#if writeFailure}<p class="failure">{writeFailure}</p>{/if}
      <div class="actions">
        <Button isBusy={isSending} busyLabel={t('app.workspace.saving')} onclick={send}>
          {editing ? t('app.workspace.save') : t('app.comments.send')}
        </Button>
        {#if replyTo || editing}
          <Button
            tone="secondary"
            onclick={() => {
              replyTo = undefined;
              editing = undefined;
              draft = '';
            }}
          >
            {t('app.workspace.cancel')}
          </Button>
        {/if}
      </div>
    </Stack>
  </Stack>
</CapabilityGate>

<style>
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
