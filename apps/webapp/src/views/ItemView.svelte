<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry, at the address the board and the search results already linked to.
  //
  // **The history is the point of this screen** (F2-15), and the rule that shapes it is
  // `domain-model.md` §3.5: the server stores `item.completed` and sends
  // `activity.item_completed`, and the client renders it. Nothing here writes a verb; the
  // catalogue does, and a code this client has never heard of still reads as words because
  // `messages.t` humanises an unknown code rather than printing a key — which is the normal state
  // of a client one milestone behind its server rather than an error.
  //
  // The **actor** is the one place this screen refuses to guess. The contract says of an activity
  // actor that "the label is not here: the account is one request away" — and for anybody but the
  // signed-in account, the name is resolved through `GET /accounts/{accountId}` and cached by
  // `lib/data/accounts.svelte.ts`. The reader is still "You", and an actor whose name did not
  // resolve is named by its kind.

  import { untrack } from 'svelte';

  import {
    ActivityFeed,
    Badge,
    Button,
    EmptyState,
    ErrorState,
    Inline,
    Input,
    LoadMore,
    Skeleton,
    Stack,
    Textarea,
    type ActivityStep,
  } from '@hubtask/design-system/components';
  import type { ActivityEntry, ActivityPage, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { actorCodes, changesOf } from '../lib/data/activity.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { activityPath, itemPath } from '../lib/data/item.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
  }

  const { id }: Props = $props();

  // Read once, and `untrack` says the once is deliberate: `App.svelte` keys this view on the id,
  // so a different entry is a different component rather than the same one asking again. A
  // resource that followed the prop would be a second answer to which entry this screen is.
  const entry = resource<WorkItem>(untrack(() => itemPath(id)));
  const history = resource<ActivityPage>(untrack(() => activityPath(id)));

  const item = $derived(entry.state.status === 'ready' ? entry.state.data : undefined);
  const failure = $derived(
    entry.state.status === 'failed' ? renderProblem(entry.state.error, messages) : undefined,
  );

  /** What the history kept about one field, as one phrase. */
  function detailOf(change: ReturnType<typeof changesOf>[number]): string {
    // A field whose values the history does not keep says so and nothing else. A note is the
    // worked example, and looking for its text would be looking for what ADR-0017 kept out.
    if (change.isOpaque) return t('app.activity.changed');
    if (change.from !== undefined && change.to !== undefined) {
      return t('app.activity.from_to', { from: change.from, to: change.to });
    }
    if (change.to !== undefined) return t('app.activity.set_to', { to: change.to });
    if (change.from !== undefined) return t('app.activity.cleared_from', { from: change.from });
    return t('app.activity.no_detail');
  }

  function stepOf(step: ActivityEntry): ActivityStep {
    // The first code the catalogue knows, which for an actor kind this client has never heard of
    // is the sentence true of every actor. The same shape `problem.ts` uses for a problem's codes.
    const who = actorCodes(step, actor.account?.id).find((code) => messages.has(code));
    // A real name where one was resolved, and the sentence true of every actor where none was.
    // "You" wins over the reader's own name: somebody reading their own history is not a third
    // party to it. Reading the cache here rather than copying from it is what makes the sentences
    // rewrite themselves when the names arrive a moment after the page.
    const name = who === 'app.activity.actor_you' ? undefined : accounts.nameOf(step.actor?.id);
    return {
      id: step.id,
      // The verb is the server's code and the actor is a parameter of it. An unrecognised verb
      // renders as `humanise` makes of it — readable, never a key and never a blank.
      sentence: t(step.code, { actor: name ?? t(who ?? 'app.activity.actor_someone') }),
      when: formatDateTime(step.occurred_at, messages.locale),
      at: step.occurred_at,
      changes: changesOf(step.change_set as Record<string, unknown>).map((change) => ({
        field: change.field,
        detail: detailOf(change),
      })),
    };
  }

  const steps = $derived(
    history.state.status === 'ready'
      ? (history.state.data.data ?? []).map(stepOf)
      : ([] as ActivityStep[]),
  );

  // The names this feed needs, asked for after the page has arrived rather than with it: the
  // history is one read and the names are a handful more, and a screen that waited for all of them
  // would show nothing while it could already show the verbs and the times. Only the two kinds that
  // have an account row — an automation and the system have none, and their sentences name what
  // they are, which is the whole of what there is to say about them.
  $effect(() => {
    if (history.state.status !== 'ready') return;
    accounts.resolve(
      (history.state.data.data ?? [])
        .filter(
          (step) =>
            (step.actor?.type === 'USER' || step.actor?.type === 'SERVICE_ACCOUNT') &&
            step.actor.id !== actor.account?.id,
        )
        .map((step) => step.actor?.id),
    );
  });
  const hasMore = $derived(
    history.state.status === 'ready' && (history.state.data.page?.has_more ?? false),
  );
  const historyFailure = $derived(
    history.state.status === 'failed' ? renderProblem(history.state.error, messages) : undefined,
  );

  // Editing the title and the notes. `items.update` has carried both since F2-09, with the
  // `If-Match` and the version conflict handled, and no component called it - so the text that
  // `POST /search` searches and that the history says somebody changed could not be written here
  // at all. The dogfooding pass set a note with curl in order to search for a word in it.
  //
  // A form rather than an inline edit, for the reason `ContainerView`'s rename gives: a refusal
  // needs somewhere to land, and a sentence at the top of a screen is one the reader has to carry
  // back down to the field they were typing in.
  let isEditing = $state(false);
  let draftTitle = $state('');
  let draftNotes = $state('');
  let isSaving = $state(false);
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isTitleFailure = $state(false);

  // An archived entry is not editable, and the control says so rather than disappearing - the same
  // sentence the list uses for the same state.
  //
  // Only the entry's own archival is read here. An entry under an archived collection carries no
  // mark of its own, so that refusal comes from the server and lands in the sentence above the
  // buttons; the alternative would be this screen reading the whole trail to predict an answer it
  // is about to be given.
  const frozenReason = $derived(item?.archived_at ? t('app.entries.archived') : undefined);

  function startEditing() {
    if (!item) return;
    draftTitle = item.title;
    draftNotes = item.notes ?? '';
    writeFailure = undefined;
    isTitleFailure = false;
    isEditing = true;
  }

  async function save() {
    if (!item || draftTitle.trim() === '' || isSaving) return;
    isSaving = true;
    writeFailure = undefined;
    isTitleFailure = false;
    try {
      // Empty notes clear them rather than setting them to the empty string: the contract's null
      // is "there are none", and a note of zero characters is not a note somebody wrote.
      await items.update(
        item.id,
        { title: draftTitle.trim(), notes: draftNotes.trim() === '' ? null : draftNotes },
        item.version,
      );
      // Both the entry and its history come back on their own: the write invalidates `/items`, and
      // the engine matches by prefix — so `/items/{id}` and `/items/{id}/activity` are re-read
      // without either being asked for here. A refresh would be a second read of what is arriving.
      isEditing = false;
    } catch (error) {
      const problem = error as { detailCode?: string };
      writeFailure = renderProblem(error as never, messages);
      isTitleFailure = writeFailure.fields.has('/title') || problem.detailCode === 'items.title_empty';
    } finally {
      isSaving = false;
    }
  }
</script>

{#if entry.state.status === 'loading' || entry.state.status === 'idle'}
  <div aria-busy="true"><Skeleton lines={3} /></div>
{:else if failure}
  <ErrorState
    title={failure.message}
    reference={failure.reference}
    referenceLabel={t('app.reference')}
    retryLabel={t('app.retry')}
    onRetry={() => entry.refresh()}
  />
{:else if !item}
  <EmptyState kind="filtered" title={t('app.item.not_found')} />
{:else}
  <Stack gap="300">
    {#if isEditing}
      <Stack gap="150">
        <Input
          label={t('app.entries.new_title')}
          bind:value={draftTitle}
          error={isTitleFailure ? writeFailure?.message : undefined}
        />
        <Textarea label={t('app.entries.notes')} bind:value={draftNotes} rows={6} />
        <!-- Everything that is not about the title is a sentence above the buttons: a version
             conflict is the ordinary case here, and nothing about the title is wrong when the
             entry moved underneath the reader. -->
        {#if writeFailure && !isTitleFailure}
          <p class="failure">{writeFailure.message}</p>
        {/if}
        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>
            {t('app.workspace.save')}
          </Button>
          <Button tone="secondary" onclick={() => (isEditing = false)}>
            {t('app.workspace.cancel')}
          </Button>
        </Inline>
      </Stack>
    {:else}
      <Stack gap="150">
        <h1 class="name">{item.title}</h1>
        <div class="marks">
          <Badge>{item.type}</Badge>
          {#if item.archived_at}
            <Badge icon="archive">{t('app.entries.archived_label')}</Badge>
          {/if}
          {#if item.completion?.is_completed}
            <Badge tone="success">{t('app.entries.complete', { title: item.title })}</Badge>
          {/if}
        </div>
        {#if item.notes}<p class="notes">{item.notes}</p>{/if}
        <div>
          <!-- Offered with its reason rather than hidden when the entry is archived, which is what
               every other refused control in this application does. -->
          <Button size="sm" tone="secondary" icon="pencil" disabledReason={frozenReason} onclick={startEditing}>
            {t('app.entries.edit')}
          </Button>
        </div>
      </Stack>
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.activity.title')}</h2>

      {#if historyFailure}
        <ErrorState
          title={historyFailure.message}
          reference={historyFailure.reference}
          referenceLabel={t('app.reference')}
          retryLabel={t('app.retry')}
          onRetry={() => history.refresh()}
        />
      {:else if history.state.status === 'loading' || history.state.status === 'idle'}
        <div aria-busy="true"><Skeleton lines={3} /></div>
      {:else}
        <ActivityFeed
          label={t('app.activity.label')}
          {steps}
          emptyLabel={t('app.activity.none')}
        />
        <!-- Cursor pagination, never a page number: the API has none, so no component may imply
             one. What arrived is announced, because pressing a button and being told nothing is
             the case a live region is for. -->
        {#if hasMore}
          <LoadMore
            label={t('app.activity.more')}
            arrivedLabel={t('app.activity.arrived', { count: steps.length })}
            onLoadMore={() => history.loadMore()}
          />
        {/if}
      {/if}
    </Stack>
  </Stack>
{/if}

<style>
  .name {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  .marks { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .notes { margin: 0; max-width: 64ch; color: var(--text-secondary); white-space: pre-wrap; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
