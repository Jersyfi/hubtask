<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The saved views that apply here: saving what is on screen, opening one, and deciding who sees
  // it.
  //
  // **Opening one applies its query *and* its layout.** That is what the `layout` field has been
  // for since `api-guidelines.md` §3 said the server would never interpret it: it is this client's
  // vocabulary, stored and echoed, and a client that applied the query alone would drop the half
  // the field exists for.
  //
  // **Where a view lives is decided once.** The scope is not in the update document — "where a view
  // lives is decided at creation" — so the control is on the save form and the edit form says so
  // rather than offering a move that would be refused.
  //
  // **Saving anywhere but privately is shaping the workspace**, so it asks `STRUCTURE`; sharing
  // asks the same. `PUBLIC_LINK` is shown and refused with the server's own sentence — omitting it
  // would disagree with the contract, offering it would promise a link this version has not built.

  import {
    Button,
    Dialog,
    Inline,
    Input,
    Select,
    Stack,
  } from '@hubtask/design-system/components';
  import type { SavedView } from '@hubtask/sync-engine';

  import { holds } from '../data/capability.svelte.ts';
  import type { ItemsQuery } from '../data/items.svelte.ts';
  import { views } from '../data/views.svelte.ts';
  import {
    SHARING,
    VIEW_SCOPES,
    needsStructure,
    queryDocumentOf,
    queryOf,
    sharingRefusal,
    type Sharing,
    type ViewScope,
    scopeIdOf,
  } from '../data/views.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { focusFirst } from '../forms/missing.ts';

  interface Props {
    isOpen: boolean;
    /**
     * Which act of the *Views* menu opened this: saving what is on screen, or renaming, sharing
     * and deleting the views that apply here. One act per opening, so the dialog says one thing.
     */
    intent?: 'save' | 'manage';
    collectionId: string;
    /** The hub the collection sits in: what a view saved at hub scope names. */
    hubId?: string;
    /**
     * Whether the reader may shape the workspace at the place a scope names - `STRUCTURE` there,
     * which saving a view anywhere but privately asks (`SavedView.go`). A scope is offered only
     * where it is `true`: the collection's administrator is not offered the hub.
     */
    mayShapeAt: (scope: ViewScope) => boolean | undefined;
    /** What the editor currently holds, which is what "save what is on screen" means. */
    query?: ItemsQuery;
    layout: string;
    role?: string;
    /** Applies a view: its query and its layout together. */
    onapply: (query: ItemsQuery, layout: string, name: string) => void;
    /** Opens the export or the feed dialog for one view. */
    onexport: (view: SavedView) => void;
    onsubscribe: (view: SavedView) => void;
  }

  let { isOpen = $bindable(), intent = 'manage', collectionId, hubId, mayShapeAt, query, layout, role, onapply, onexport, onsubscribe }: Props =
    $props();

  const structure = $derived(holds(role, 'STRUCTURE'));
  const held = $derived(views.of(collectionId));

  /**
   * Whether the reader may share a view here - `STRUCTURE` at the collection. Without it the
   * sharing is not offered at all rather than refused with a sentence (P-05, UC-WRK-22 check 4).
   */
  const mayShare = $derived(structure.status === 'permitted');
  /** The name was asked for and is empty: said at the field when the save is pressed. */
  let isNameMissing = $state(false);
  let saveForm = $state<HTMLElement | null>(null);

  let name = $state('');
  let scope = $state<ViewScope>('ACCOUNT');
  let editing = $state<SavedView | undefined>(undefined);
  let sharing = $state<Sharing>('PRIVATE');
  let deleting = $state<SavedView | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    if (!isOpen) return;
    reset();
  });

  function reset() {
    isNameMissing = false;
    name = '';
    scope = 'ACCOUNT';
    editing = undefined;
    sharing = 'PRIVATE';
    deleting = undefined;
    failure = undefined;
  }

  const shareRefusal = $derived(
    editing ? sharingRefusal(sharing, editing.scope_type as string) : undefined,
  );

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    isSaving = true;
    failure = undefined;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  function save() {
    // Never switched off for an empty name (design-system.md §4, case 3).
    if (name.trim() === '') {
      isNameMissing = true;
      queueMicrotask(() => focusFirst(saveForm));
      return;
    }
    const scopeId = scopeIdOf(scope, { collectionId, hubId });
    void attempt(async () => {
      await views.create(
        {
          scope_type: scope as never,
          // `ACCOUNT` names the owner and `TENANT` names nobody; `COLLECTION` names this collection
          // and `HUB` the hub it sits in.
          ...(scopeId ? { scope_id: scopeId } : {}),
          name,
          layout,
          query: queryDocumentOf(query, collectionId),
          ...(query?.group ? { grouping: query.group as never } : {}),
        } as never,
        crypto.randomUUID(),
      );
      // Saved: the view is now a row of the *Views* menu, which is where it is opened.
      isOpen = false;
    }, t('app.views.saved_announced'));
  }

  function apply(view: SavedView) {
    onapply(
      queryOf(view.query as Record<string, unknown>, view.grouping as Record<string, unknown>),
      view.layout,
      view.name,
    );
    isOpen = false;
  }

  function rename(view: SavedView) {
    void attempt(async () => {
      await views.update(view.id, { name }, view.version);
      reset();
    }, t('app.views.renamed_announced'));
  }

  /** Replaces the view's query with what the editor currently holds. */
  function replaceQuery(view: SavedView) {
    void attempt(async () => {
      await views.update(
        view.id,
        {
          layout,
          query: queryDocumentOf(query, collectionId) as never,
          ...(query?.group ? { grouping: query.group as never } : {}),
        },
        view.version,
      );
      reset();
    }, t('app.views.query_replaced_announced'));
  }

  function decideSharing(view: SavedView) {
    if (shareRefusal) return;
    void attempt(async () => {
      await views.share(view.id, sharing, view.version, crypto.randomUUID());
      reset();
    }, t('app.views.shared_announced'));
  }

  function confirmDelete() {
    const target = deleting;
    if (!target) return;
    void attempt(async () => {
      await views.remove(target.id, target.version);
      deleting = undefined;
    }, t('app.views.removed_announced'));
  }
</script>

<Dialog bind:isOpen title={intent === 'save' ? t('app.views.save_title') : t('app.views.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    {#if intent === 'save'}
      <!-- Saving only: the views already held are the menu's own rows. -->
    {:else if held.length === 0}
      <p class="quiet">{t('app.views.none')}</p>
    {:else}
      <ul class="list">
        {#each held as view (view.id)}
          <li>
            <div class="row">
              <div>
                <span class="name">{view.name}</span>
                <span class="meta">
                  {t(`app.views.scope_${view.scope_type}`)} · {t(`app.view.${view.layout}`)}
                </span>
              </div>
              <Inline gap="050">
                <Button size="sm" onclick={() => apply(view)}>{t('app.views.open')}</Button>
                <Button size="sm" tone="secondary" onclick={() => onexport(view)}>
                  {t('app.export.title')}
                </Button>
                <Button size="sm" tone="secondary" onclick={() => onsubscribe(view)}>
                  {t('app.feeds.create')}
                </Button>
                <Button
                  size="sm"
                  tone="secondary"
                  onclick={() => {
                    editing = view;
                    name = view.name;
                    sharing = (view.sharing as Sharing) ?? 'PRIVATE';
                  }}
                >
                  {t('app.entries.edit')}
                </Button>
                <Button size="sm" tone="secondary" onclick={() => (deleting = view)}>
                  {t('app.views.delete')}
                </Button>
              </Inline>
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    {#if editing}
      {@const current = editing}
      <Stack gap="150">
        <Input label={t('app.views.name')} bind:value={name} error={failure?.fields.get('/name')} />
        <!-- The scope is not in the update document, so this says so rather than offering a move
             the server would refuse. -->
        <p class="quiet">{t('app.views.scope_fixed')}</p>

        {#if mayShare}
          <!-- A way of sharing this view cannot take is an option that says why, in the list
               itself; nothing is printed under the field. -->
          <Select
            label={t('app.views.sharing')}
            bind:value={sharing}
            options={SHARING.map((each) => {
              const refusal = sharingRefusal(each, current.scope_type as string);
              return { value: each, label: t(`app.views.sharing_${each}`), disabledReason: refusal ? t(refusal) : undefined };
            })}
            describeUnavailable={(label, reason) => t('app.select.unavailable_option', { label, reason })}
          />
        {/if}

        {#if failure && !failure.fields.get('/name')}<p class="failure" role="alert">{failure.message}</p>{/if}

        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={() => rename(current)}>
            {t('app.views.rename')}
          </Button>
          <Button tone="secondary" onclick={() => replaceQuery(current)}>
            {t('app.views.update_query')}
          </Button>
          {#if mayShare}
            <Button tone="secondary" disabledReason={shareRefusal ? t(shareRefusal) : undefined} onclick={() => decideSharing(current)}>
              {t('app.views.share')}
            </Button>
          {/if}
          <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
        </Inline>
      </Stack>
    {:else if intent === 'save'}
      <div bind:this={saveForm}>
      <Stack gap="150">
        <Input
          label={t('app.views.name')}
          bind:value={name}
          oninput={() => (isNameMissing = false)}
          error={isNameMissing ? t('views.name_empty') : failure?.fields.get('/name')}
        />
        <Select
          label={t('app.views.scope')}
          bind:value={scope}
          options={VIEW_SCOPES.filter((each) => scopeIdOf(each, { collectionId, hubId }) !== undefined && (!needsStructure(each) || mayShapeAt(each) === true)).map((each) => ({ value: each, label: t(`app.views.scope_${each}`) }))}
        />
        {#if failure && !failure.fields.get('/name')}<p class="failure" role="alert">{failure.message}</p>{/if}
        <div>
          <Button
            isBusy={isSaving}
            busyLabel={t('app.workspace.saving')}
            onclick={save}
          >
            {t('app.views.save_current')}
          </Button>
        </div>
      </Stack>
      </div>
    {/if}
  </Stack>
</Dialog>

{#if deleting}
  <Dialog
    isOpen={true}
    title={t('app.views.delete_title', { name: deleting.name })}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (deleting = undefined)}
  >
    <Stack gap="150">
      <p class="quiet">{t('app.views.delete_explains')}</p>
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
      <Inline gap="100">
        <Button tone="danger" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={confirmDelete}>
          {t('app.views.delete')}
        </Button>
        <Button tone="secondary" onclick={() => (deleting = undefined)}>{t('app.workspace.cancel')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .list { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-100); }

  .row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .name { display: block; font-weight: var(--fw-semibold); }

  .meta { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
