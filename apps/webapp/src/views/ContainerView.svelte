<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub or a collection, at its own address.
  //
  // One view for both, because the two differ in what they *hold* rather than in what they are:
  // a hub holds collections, a collection will hold entries (F2-09). Everything above that line —
  // the trail, the name, the archive state, the controls — is the same screen, and two files would
  // be two places to fix the same thing.
  //
  // The breadcrumb is built from the **route** rather than from a remembered click, which is what
  // makes a deep link land with a correct trail: ADR-0028's `index.html` fallback exists so a deep
  // link survives a reload, and a trail assembled from navigation history would be empty after one.

  import {
    Breadcrumb,
    Button,
    EmptyState,
    IconButton,
    Inline,
    Input,
    ListRow,
    Skeleton,
    Stack,
    Tabs,
    Toolbar,
  } from '@hubtask/design-system/components';

  import { untrack } from 'svelte';

  import Board from '../lib/entries/Board.svelte';
  import EntryList from '../lib/entries/EntryList.svelte';
  import MoveDialog from '../lib/entries/MoveDialog.svelte';

  import { announcer } from '../lib/announce.svelte.ts';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { archivalOf } from '../lib/data/containers.ts';
  import { anchorFor } from '../lib/data/rank.ts';
  import type { TransportError } from '@hubtask/sync-engine';

  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  // Its own read as well as the levels. A deep link to a collection may be the first thing this
  // client ever asks for, and its hub is then not loaded either — looking only in the levels would
  // say "not in this workspace" about a container that is simply not fetched yet.
  // `untrack` for the reason `WorkspaceNav` records: the listener writes the store, writing it
  // reads it, and an effect tracking that read cancels its own subscription before the answer
  // lands. This one depends on `id`.
  $effect(() => {
    const wanted = id;
    return untrack(() => containers.openSingle(wanted));
  });

  /** Which of the two the reader is looking at. Kept on the device; saved views are F3's. */
  let layout = $state('list');

  const container = $derived(containers.find(id));
  // The hub above a collection, for the trail. Read on its own for the same reason.
  $effect(() => {
    const parent = container?.type === 'COLLECTION' ? container.parent_id : undefined;
    if (!parent) return;
    return untrack(() => containers.openSingle(parent));
  });
  const hub = $derived(
    container?.type === 'COLLECTION' && container.parent_id
      ? containers.find(container.parent_id)
      : undefined,
  );
  const archival = $derived(container ? archivalOf(container) : 'active');
  const isReadOnly = $derived(archival !== 'active');

  // Built from the route rather than from a remembered click, which is what makes a deep link land
  // with a correct trail: a trail assembled from navigation history would be empty after a reload,
  // and ADR-0028's `index.html` fallback exists precisely so a reload works.
  const trail = $derived([
    ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
    // The last one is where the reader is, so it has no href — `Breadcrumb` reads that from the
    // absent href rather than from a flag.
    ...(container ? [{ id: container.id, label: container.name }] : []),
  ]);

  // A hub's collections are their own level, so opening this screen is what reads them.
  $effect(() => {
    if (container?.type !== 'HUB') return;
    const wanted = id;
    return untrack(() => containers.openLevel(wanted));
  });
  const collections = $derived(container?.type === 'HUB' ? containers.collectionsOf(id) : []);

  /**
   * The level this container is ranked within, and where it currently sits in it.
   *
   * A hub's level is the tenant's hubs; a collection's is its hub's collections. Both are already
   * loaded by the time the controls are usable — the sidebar reads the hubs, and this view reads
   * the level of the hub it is in.
   */
  const siblings = $derived(
    container?.type === 'HUB'
      ? containers.hubs
      : container?.parent_id
        ? containers.collectionsOf(container.parent_id)
        : [],
  );
  const position = $derived(siblings.findIndex((entry) => entry.id === id));
  const canMoveUp = $derived(position > 0);
  const canMoveDown = $derived(position >= 0 && position < siblings.length - 1);

  /**
   * Ranks this container one place up or down.
   *
   * The keyboard path, and for now the only one: WCAG 2.2 SC 2.5.7 wants a single-pointer
   * alternative to every drag, and F2-12 builds the drag **against** this rather than the other way
   * round. A rank change is a command before it is a gesture.
   *
   * A **hub** goes through `:reorder`, which is the only operation that can rank one — it sits in
   * nothing, so `:move`'s required `target_parent_id` has nothing to name (F2-04). A collection
   * goes through `:move` naming the hub it is already in, which the operation documents as a
   * reorder.
   */
  async function moveBy(offset: number) {
    if (!container || position < 0) return;
    const target = position + offset;
    if (target < 0 || target >= siblings.length) return;

    failure = undefined;
    try {
      if (container.type === 'HUB') {
        await containers.reorder(container.id, siblings, target, crypto.randomUUID());
      } else if (container.parent_id) {
        await containers.move(
          container.id,
          container.parent_id,
          anchorFor(siblings, container.id, target),
          crypto.randomUUID(),
        );
      }
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    }
  }

  /**
   * Moving a collection into another hub, which is the one placement no position can express.
   *
   * Up and down rank it where it already is; this changes which hub holds it, and `:move`'s
   * `target_parent_id` is required precisely because that is the question it answers. A hub is
   * offered none of this: it sits in nothing, so there is no destination to name — the same reason
   * F2-04 gave hubs a `:reorder` of their own.
   *
   * Nothing is lost by it. A collection carries its own labels and its own board, so the losses
   * `MoveResult` reports for an entry (I-W6) have no counterpart here, and the dialog is shown
   * without a warning rather than with an invented one.
   */
  let isMovingHub = $state(false);
  let isMovingNow = $state(false);

  const hubs = $derived(
    containers.hubs
      .filter((each) => each.id !== container?.parent_id && !each.effective_archived)
      .map((each) => ({ value: each.id, label: each.name })),
  );

  async function moveToHub(hubId: string) {
    if (!container) return;
    // Both names are read **before** the write, and neither is available afterwards. The move
    // invalidates `/containers`, so `container` is briefly undefined while the level reloads, and
    // the destination has stopped being a destination — it is the hub this collection is in now,
    // so `hubs` no longer offers it. Reading either afterwards throws, and a throw in here would
    // reach the catch below and be rendered as a transport failure it is not.
    const moving = container.name;
    const destination = hubs.find((each) => each.value === hubId)?.label ?? '';

    isMovingNow = true;
    failure = undefined;
    try {
      await containers.move(container.id, hubId, null, crypto.randomUUID());
      isMovingHub = false;
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
      return;
    } finally {
      isMovingNow = false;
    }
    // Outside the try, for the same reason: `renderProblem` reads a problem document, and handing
    // it anything else fails on `fieldErrors` — which turns a rendering mistake into a sentence
    // about the server.
    announcer.say(t('app.move.container_announced', { name: moving, hub: destination }));
  }

  // Renaming, and the whole reason it is a form rather than an inline edit: a name collision is a
  // field error on the name (`containers.name_taken`), and a field error needs a field to land on.
  let isRenaming = $state(false);
  let draft = $state('');
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /**
   * Whether the failure belongs under the name field rather than above the form.
   *
   * A taken name arrives as a **409 with `containers.name_taken`**, not as a `field_errors[]`
   * entry — it is a conflict rather than a validation failure, and the server is right about that.
   * But the reader's next action is to type a different name, and a sentence at the top of a form
   * is a sentence they have to carry back down to the field. So this one code is placed, and the
   * placement is the client's judgement rather than a claim about the document.
   */
  let isNameFailure = $state(false);

  function startRename() {
    draft = container?.name ?? '';
    failure = undefined;
    isNameFailure = false;
    isRenaming = true;
  }

  async function save() {
    if (!container) return;
    isSaving = true;
    failure = undefined;
    isNameFailure = false;
    try {
      // The version the reader had when they started typing. A rename that lost a race is refused
      // rather than winning by being second (ADR-0025), and `version_conflict` is what says so.
      await containers.update(container.id, { name: draft }, container.version);
      isRenaming = false;
    } catch (error) {
      const problem = error as TransportError;
      failure = renderProblem(problem, messages);
      isNameFailure =
        problem.detailCode === 'containers.name_taken' || failure.fields.has('/name');
    } finally {
      isSaving = false;
    }
  }
</script>

{#if !container && !containers.isSettled(id)}
  <div aria-busy="true"><Skeleton lines={3} /></div>
{:else if !container}
  <!-- Not an error state: the read succeeded and this address is simply not in the workspace. §4.4
       is about a *failure* rendered as an empty list; this is the other way round. -->
  <EmptyState kind="filtered" title={t('app.workspace.not_loaded')} />
{:else}
  <Stack gap="300">
    <Breadcrumb
      label={t('app.workspace.trail')}
      {trail}
      expandLabel={t('app.workspace.expand_trail')}
      onnavigate={(crumbId) => {
        const crumb = containers.find(crumbId);
        if (crumb) onnavigate(crumb.type === 'HUB' ? `/hubs/${crumbId}` : `/collections/${crumbId}`);
      }}
    />

    {#if isRenaming}
      <Stack gap="150">
        <Input
          label={container.type === 'HUB' ? t('app.workspace.hub_name') : t('app.workspace.collection_name')}
          bind:value={draft}
          error={isNameFailure ? failure?.message : undefined}
        />
        <!-- Everything else is a sentence above the buttons rather than a field error, and a
             version conflict is the ordinary case: nothing about the name is wrong, the row moved
             underneath the reader. -->
        {#if failure && !isNameFailure}
          <p class="failure">{failure.message}</p>
        {/if}
        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>
            {t('app.workspace.save')}
          </Button>
          <Button tone="secondary" onclick={() => (isRenaming = false)}>
            {t('app.workspace.cancel')}
          </Button>
        </Inline>
      </Stack>
    {:else}
      <Stack gap="150">
        <h1 class="name">{container.name}</h1>
        {#if container.description}<p class="description">{container.description}</p>{/if}

        <!-- The two archive states say different things and offer different controls, which is the
             whole reason `archivalOf` distinguishes them. -->
        {#if archival === 'archived'}
          <p class="notice">{t('app.workspace.archived')}</p>
        {:else if archival === 'inherited'}
          <p class="notice">
            {t('app.workspace.archived_above', { hub: trail[0]?.label ?? '' })}
          </p>
        {/if}

        {#if failure && !isRenaming}
          <p class="failure">{failure.message}</p>
        {/if}

        <Toolbar label={t('app.workspace.title')}>
          <Button
            size="sm"
            tone="secondary"
            onclick={startRename}
            disabledReason={isReadOnly
              ? (archival === 'archived' ? t('app.workspace.archived') : t('app.workspace.archived_above', { hub: trail[0]?.label ?? '' }))
              : undefined}
          >
            {t('app.workspace.rename')}
          </Button>
          {#if archival !== 'inherited'}
            <Button
              size="sm"
              tone="secondary"
              onclick={() => containers.setArchived(container.id, archival === 'active', crypto.randomUUID())}
            >
              {archival === 'archived' ? t('app.workspace.unarchive') : t('app.workspace.archive')}
            </Button>
          {/if}
          <!-- The rank, as a command. There is no `disabled` boolean: at the top of a level there
               is nowhere up to go, and the reason says so rather than the control going grey for
               no stated cause. -->
          <IconButton
            icon="chevron-up"
            label={t('app.rank.up')}
            size="sm"
            onclick={() => moveBy(-1)}
            disabledReason={canMoveUp ? undefined : t('app.rank.already_first')}
          />
          <IconButton
            icon="chevron-down"
            label={t('app.rank.down')}
            size="sm"
            onclick={() => moveBy(1)}
            disabledReason={canMoveDown ? undefined : t('app.rank.already_last')}
          />
          <!-- The placement no position can express. A hub is offered it with the reason it cannot
               be used rather than not at all: it sits in nothing, so there is nowhere to move it
               to, and a control that quietly disappeared would leave the reader wondering. -->
          <Button
            size="sm"
            tone="secondary"
            onclick={() => (isMovingHub = true)}
            disabledReason={container.type === 'HUB'
              ? t('app.move.hub_only')
              : isReadOnly
                ? t('app.workspace.archived')
                : undefined}
          >
            {t('app.move.to_hub')}
          </Button>
        </Toolbar>
      </Stack>
    {/if}

    {#if container.type === 'HUB'}
      {#if collections.length === 0}
        <EmptyState kind="unused" title={t('app.workspace.no_collections')} icon="collection" />
      {:else}
        <Stack gap="050">
          {#each collections as collection (collection.id)}
            <ListRow href={`/collections/${collection.id}`}>{collection.name}</ListRow>
          {/each}
        </Stack>
      {/if}
    {:else}
      <!-- Two ways to look at the same entries. `ViewSwitcher` and the layouts the manifest reports
           are F2-13's; this is the pair F2-11 built, and the choice is kept on the device because
           saved views are F3's and writing one here would be building half of that milestone
           badly. -->
      <Tabs
        label={t('app.workspace.title')}
        tabs={[
          { id: 'list', label: t('app.board.show_list') },
          { id: 'board', label: t('app.board.show_board') },
        ]}
        bind:selected={layout}
      >
        {#if layout === 'board'}
          <Board collectionId={container.id} isReadOnly={isReadOnly} />
        {:else}
          <!-- Read-only follows the container: an archived collection's entries are archived with
               it (I-C3), and the reason travels with the controls rather than the controls
               disappearing. -->
          <EntryList collectionId={container.id} isReadOnly={isReadOnly} />
        {/if}
      </Tabs>
    {/if}
  </Stack>
{/if}

{#if isMovingHub && container}
  <MoveDialog
    bind:isOpen={isMovingHub}
    title={t('app.rank.actions', { title: container.name })}
    label={t('app.move.hub')}
    placeholder={t('app.move.choose_hub')}
    options={hubs}
    emptyLabel={t('app.move.no_hub')}
    confirmLabel={t('app.move.confirm')}
    busyLabel={t('app.move.moving')}
    cancelLabel={t('app.workspace.cancel')}
    chooseFirstLabel={t('app.move.choose_hub_first')}
    isBusy={isMovingNow}
    onmove={(hubId) => moveToHub(hubId)}
  />
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

  .description { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .notice { margin: 0; color: var(--text-warning); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

</style>
