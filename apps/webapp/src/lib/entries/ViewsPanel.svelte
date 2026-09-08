<!-- SPDX-License-Identifier: BUSL-1.1
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
  } from '../data/views.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    collectionId: string;
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

  let { isOpen = $bindable(), collectionId, query, layout, role, onapply, onexport, onsubscribe }: Props =
    $props();

  const structure = $derived(holds(role, 'STRUCTURE'));
  const held = $derived(views.of(collectionId));

  const structureReason = $derived(
    structure.status === 'permitted'
      ? undefined
      : structure.status === 'refused'
        ? t(structure.code, structure.params)
        : t('app.views.deciding'),
  );

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
    name = '';
    scope = 'ACCOUNT';
    editing = undefined;
    sharing = 'PRIVATE';
    deleting = undefined;
    failure = undefined;
  }

  /** Saving anywhere but privately shapes the workspace, so it takes `STRUCTURE`. */
  const scopeReason = $derived(needsStructure(scope) ? structureReason : undefined);
  const shareRefusal = $derived(
    editing ? sharingRefusal(sharing, editing.scope_type as string) : undefined,
  );

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    isSaving = true;
    failure = undefined;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  function save() {
    void attempt(async () => {
      await views.create(
        {
          scope_type: scope as never,
          // `ACCOUNT` names the owner and `TENANT` names nobody; the two container scopes name
          // this collection, which is the only container this screen is.
          ...(scope === 'COLLECTION' || scope === 'HUB' ? { scope_id: collectionId } : {}),
          name,
          layout,
          query: queryDocumentOf(query),
          ...(query?.group ? { grouping: query.group as never } : {}),
        } as never,
        crypto.randomUUID(),
      );
      reset();
    });
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
    });
  }

  /** Replaces the view's query with what the editor currently holds. */
  function replaceQuery(view: SavedView) {
    void attempt(async () => {
      await views.update(
        view.id,
        {
          layout,
          query: queryDocumentOf(query) as never,
          ...(query?.group ? { grouping: query.group as never } : {}),
        },
        view.version,
      );
      reset();
    });
  }

  function decideSharing(view: SavedView) {
    if (shareRefusal) return;
    void attempt(async () => {
      await views.share(view.id, sharing, view.version, crypto.randomUUID());
      reset();
    });
  }

  function confirmDelete() {
    const target = deleting;
    if (!target) return;
    void attempt(async () => {
      await views.remove(target.id, target.version);
      deleting = undefined;
    });
  }
</script>

<Dialog bind:isOpen title={t('app.views.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    {#if held.length === 0}
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

        <Select
          label={t('app.views.sharing')}
          bind:value={sharing}
          options={SHARING.map((each) => ({ value: each, label: t(`app.views.sharing_${each}`) }))}
        />
        {#if shareRefusal}
          <!-- Shown and refused, with the server's own sentence. -->
          <p class="quiet">{t(shareRefusal)}</p>
        {/if}

        {#if failure && !failure.fields.get('/name')}<p class="failure">{failure.message}</p>{/if}

        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={() => rename(current)}>
            {t('app.views.rename')}
          </Button>
          <Button tone="secondary" onclick={() => replaceQuery(current)}>
            {t('app.views.update_query')}
          </Button>
          <Button
            tone="secondary"
            disabledReason={shareRefusal ? t(shareRefusal) : structureReason}
            onclick={() => decideSharing(current)}
          >
            {t('app.views.share')}
          </Button>
          <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
        </Inline>
      </Stack>
    {:else}
      <Stack gap="150">
        <Input label={t('app.views.name')} bind:value={name} error={failure?.fields.get('/name')} />
        <Select
          label={t('app.views.scope')}
          bind:value={scope}
          options={VIEW_SCOPES.map((each) => ({ value: each, label: t(`app.views.scope_${each}`) }))}
        />
        {#if failure && !failure.fields.get('/name')}<p class="failure">{failure.message}</p>{/if}
        <div>
          <Button
            isBusy={isSaving}
            busyLabel={t('app.workspace.saving')}
            disabledReason={name.trim() === '' ? t('app.views.name') : scopeReason}
            onclick={save}
          >
            {t('app.views.save_current')}
          </Button>
        </div>
      </Stack>
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
      {#if failure}<p class="failure">{failure.message}</p>{/if}
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
