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
    AvatarGroup,
    Button,
    DetailPane,
    Dialog,
    Drawer,
    EmptyState,
    focusFirst,
    IconButton,
    Inline,
    Input,
    ListRow,
    PageHeader,
    Skeleton,
    Stack,
    type MenuItem,
  } from '@hubtask/design-system/components';

  import { untrack } from 'svelte';

  import Board from '../lib/entries/Board.svelte';
  import CollectionSummary from '../lib/entries/CollectionSummary.svelte';
  import BulkBar from '../lib/entries/BulkBar.svelte';
  import DuplicateDialog from '../lib/entries/DuplicateDialog.svelte';
  import TemplatesDialog from '../lib/entries/TemplatesDialog.svelte';
  import ViewsPanel from '../lib/entries/ViewsPanel.svelte';
  import ExportDialog from '../lib/entries/ExportDialog.svelte';
  import ImportDialog from '../lib/entries/ImportDialog.svelte';
  import FeedsDialog from '../lib/entries/FeedsDialog.svelte';
  import TimelineView from '../lib/entries/TimelineView.svelte';
  import CustomFieldsDialog from '../lib/entries/CustomFieldsDialog.svelte';
  import LabelsDialog from '../lib/entries/LabelsDialog.svelte';
  import EntryList from '../lib/entries/EntryList.svelte';
  import LayoutSwitch from '../lib/entries/LayoutSwitch.svelte';
  import MoveDialog from '../lib/entries/MoveDialog.svelte';
  import PoliciesDialog from '../lib/workspace/PoliciesDialog.svelte';
  import QueryPanel from '../lib/entries/QueryPanel.svelte';
  import MembersDialog from '../lib/people/MembersDialog.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { holds, rootTypes } from '../lib/data/capability.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { customFields } from '../lib/data/customfields.svelte.ts';
  import { templates } from '../lib/data/templates.svelte.ts';
  import { feeds, views as savedViews } from '../lib/data/views.svelte.ts';
  import { queryFieldsFor } from '../lib/data/customfields.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { selection } from '../lib/data/selection.svelte.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { byItem } from '../lib/data/bulk.ts';
  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import ItemView from './ItemView.svelte';

  import { announcer } from '../lib/announce.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { archivalOf } from '../lib/data/containers.ts';
  import { anchorFor } from '../lib/data/rank.ts';
  import type {
    BulkOperation,
    BulkResult,
    SavedView,
    TransportError,
    WorkItem,
  } from '@hubtask/sync-engine';

  import type { ItemsQuery } from '../lib/data/items.svelte.ts';

  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { humanise } from '../lib/i18n/messages.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
    /**
     * The entry open beside the list (ADR-0061 decision 4): `?item=` on the collection's address,
     * from `large` up. The frame's router resolved it and `App.svelte` redirected below `large`,
     * so here it is only ever a pane. Opening from a row sets it; closing clears it; both are
     * navigations, so the address, a reload and the back button agree.
     */
    openItemId?: string;
    onnavigate: (path: string) => void;
  }

  const { id, openItemId, onnavigate }: Props = $props();

  /** The open entry, for the pane's head: read by the view inside; this only needs its words. */
  const openItem = $derived(openItemId ? items.find(openItemId) : undefined);

  function openBeside(itemId: string) {
    onnavigate(`/collections/${id}?item=${encodeURIComponent(itemId)}`);
  }

  /**
   * Closing the pane puts the focus back on the row it came from - the reader who opened it from
   * the list is still in the list, which is the point of a pane. The row is found after the
   * navigation has redrawn, when nothing has focus but the body.
   */
  function closePane() {
    const rowId = openItemId;
    onnavigate(`/collections/${id}`);
    queueMicrotask(() => {
      if (document.activeElement && document.activeElement !== document.body) return;
      document.querySelector<HTMLElement>(`[data-row="${rowId}"] a`)?.focus();
    });
  }

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

  /**
   * Which layout the reader is looking at, and what they have asked of the entries.
   *
   * Kept on the device, in memory. Saved views are F3's, and writing a `SavedView` here would be
   * building half of that milestone badly — the choice is the reader's for as long as the screen
   * is open, and no further claim is made about it.
   */
  let layout = $state('LIST_COLLAPSED');
  let query = $state<ItemsQuery>({});

  /** The layouts this client can actually draw. `TIMELINE` needs F3's time work and is not here. */
  const DRAWABLE = ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'];

  const container = $derived(containers.find(id));

  /**
   * The path this container sits on, and the role this reader holds along it.
   *
   * `STRUCTURE` is what defining a custom field takes, and a role is held at a scope rather than
   * globally — so it is composed the same way the members dialog composes its own path. Nothing
   * here decides anything: the gate is a prediction, and the server's refusal is what a reader
   * meets when it is wrong.
   */
  const containerPath = $derived(
    container?.type === 'HUB'
      ? { hubId: container.id }
      : { hubId: container?.parent_id ?? undefined, collectionId: container?.id },
  );

  $effect(() => {
    if (!container) return;
    people.open(containerPath);
  });

  const structureRole = $derived(
    people
      .along(containerPath)
      .find((membership) => membership.account_id === actor.account?.id)?.role as string | undefined,
  );

  /**
   * The definitions in force here, as fields the query editor can offer.
   *
   * Only for a collection: a hub's screen lists containers rather than entries, and a definition
   * belongs to a collection or to the workspace — so there is no one set in force above one.
   */
  const customFieldFilters = $derived(
    container?.type === 'COLLECTION' ? queryFieldsFor(customFields.of(container.id)) : [],
  );

  /**
   * What the last bulk did, per entry, until the reader dismisses it.
   *
   * Kept here rather than in the bar because it belongs to the rows: a refusal is about an entry,
   * and the place a reader looks for it is the row it happened to. Cleared when the selection is,
   * and when the screen changes.
   */
  let lastResults = $state<ReadonlyMap<string, BulkResult>>(new Map());

  /** The entry being copied, if one is. The dialog is open exactly while this is set. */
  let duplicating = $state<WorkItem | undefined>(undefined);

  // A selection is about what is in front of somebody, so it does not survive the screen.
  $effect(() => {
    void id;
    return () => {
      selection.clear();
      lastResults = new Map();
    };
  });

  // The views that apply here, and this reader's own feeds. Both read once for their dialogs.
  $effect(() => {
    if (container?.type !== 'COLLECTION') return;
    const wanted = container.id;
    return untrack(() => savedViews.open(wanted));
  });

  $effect(() => untrack(() => feeds.open()));

  // The templates that apply here, read once for the dialog.
  $effect(() => {
    if (container?.type !== 'COLLECTION') return;
    const wanted = container.id;
    return untrack(() => templates.open(wanted));
  });

  // The definitions in force here, read once for the dialog and for the filter editor below.
  $effect(() => {
    if (container?.type !== 'COLLECTION') return;
    const wanted = container.id;
    return untrack(() => customFields.open(wanted));
  });
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
  const hasAi = $derived(manifest.value?.features?.ai_suggestions === true);
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
      // Said out loud, as an entry's rank change is: the control kept focus and its name, and
      // the list quietly rearranged itself (4.1.3).
      announcer.say(
        t('app.rank.announced', { title: container.name, position: target + 1, count: siblings.length }),
      );
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    }
  }

  /** Archives or reactivates, and says so: the button keeps focus and only its label changes. */
  async function toggleArchived() {
    if (!container) return;
    failure = undefined;
    const archiving = archival === 'active';
    try {
      await containers.setArchived(container.id, archiving, crypto.randomUUID());
      announcer.say(
        t(archiving ? 'app.workspace.archived_announced' : 'app.workspace.unarchived_announced', {
          name: container.name,
        }),
      );
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

  /**
   * Moving a container and everything under it to the trash.
   *
   * **Confirmed, although it is reversible.** The trash is a soft delete and a restore brings the
   * whole batch back (I-C2), so this is not the irreversible act of the milestone — but a hub is
   * two hundred entries and "one deletion" is the thing worth saying before it happens rather than
   * afterwards. The sentence says what goes with it, which is what a person is actually deciding.
   */
  // The collection's labels. `labels` has had create, update and remove since F2-10 and no
  // caller, so the set the picker offers could only be made outside the application.
  //
  // No subscription of its own: a collection renders either the list or the board, and both open
  // the level already. A third reader of one list is what the stores exist to avoid.
  let isManagingLabels = $state(false);
  let isManagingFields = $state(false);
  let isUsingTemplates = $state(false);
  let isManagingViews = $state(false);
  /** The collection's policies (issue 773): how it works, as opposed to what it is called. */
  let isManagingPolicies = $state(false);
  let isManagingFeeds = $state(false);
  /** The view an export or a subscription is about, if either is open. */
  let exporting = $state<SavedView | undefined>(undefined);
  let subscribing = $state<SavedView | undefined>(undefined);
  let isManagingMembers = $state(false);
  let isImporting = $state(false);
  /** `STRUCTURE` on the hub is what an import takes, because it creates collections there. */
  const structure = $derived(holds(structureRole, 'STRUCTURE'));

  let isTrashing = $state(false);
  let isTrashingNow = $state(false);

  async function moveToTrash() {
    if (!container) return;
    const name = container.name;
    isTrashingNow = true;
    failure = undefined;
    try {
      await containers.trash(container.id, container.version);
      isTrashing = false;
      announcer.say(t('app.workspace.trashed_announced', { name }));
      // The container is gone from this address, so the reader goes somewhere that still exists.
      onnavigate(hub ? `/hubs/${hub.id}` : '/');
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    } finally {
      isTrashingNow = false;
    }
  }

  // Renaming, and the whole reason it is a form rather than an inline edit: a name collision is a
  // field error on the name (`containers.name_taken`), and a field error needs a field to land on.
  // Creating a collection in this hub. A hub that holds none is where a workspace is started, and
  // before this the only way to start one was `hubctl`.
  let isCreatingCollection = $state(false);

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

  // The bar carries the title on a phone (ADR-0061 decision 2); the head then reads its heading
  // rather than drawing it. Cleared when this screen leaves.
  $effect(() => page.entitle(container?.name));

  /** The list, for the head's primary action: the form is the list's, the button is the head's. */
  let list = $state<EntryList | undefined>(undefined);

  /** The filter: open inline from `expanded`, in a drawer below it; the count is the panel's. */
  let isFilterOpen = $state(false);
  let filterCount = $state(0);

  /** Why an entry cannot be created here, or nothing. The gate is the list's; the words are shared. */
  const addDisabledReason = $derived(
    isReadOnly
      ? t('app.workspace.archived')
      : rootTypes().length === 0
        ? t('items.capability_not_supported', { type: '', capability: 'CREATE' })
        : undefined,
  );

  /** Creating from the head: on a board or the timeline, the list is where the form lives. */
  function addFromHead() {
    if (!container || addDisabledReason) return;
    if (layout === 'KANBAN' || layout === 'TIMELINE') layout = 'LIST_COLLAPSED';
    // After the layout switch the list is mounted on the next tick.
    queueMicrotask(() => list?.addEntry());
  }

  /** Who holds a role here, for the faces in the head: the same dialog the menu opens. */
  const memberIds = $derived([...new Set(people.along(containerPath).map((membership) => membership.account_id))]);
  $effect(() => {
    accounts.resolve(memberIds);
  });

  /**
   * The page menu, in the three groups backlog decision 5 fixed: act on the object, set it up,
   * and trash - last and alone. A reason stays a reason: the rank at the top of its level and
   * the move of a hub are offered with why they cannot be used, not left out.
   */
  const pageMenu = $derived<MenuItem[]>(
    container === undefined
      ? []
      : [
          {
            id: 'rename',
            label: t('app.workspace.rename'),
            icon: 'pencil',
            disabledReason: isReadOnly
              ? archival === 'archived'
                ? t('app.workspace.archived')
                : t('app.workspace.archived_above', { hub: trail[0]?.label ?? '' })
              : undefined,
          },
          {
            id: 'move',
            label: t('app.move.to_hub'),
            disabledReason: container.type === 'HUB' ? t('app.move.hub_only') : isReadOnly ? t('app.workspace.archived') : undefined,
          },
          ...(archival !== 'inherited'
            ? [{ id: 'archive', label: archival === 'archived' ? t('app.workspace.unarchive') : t('app.workspace.archive'), icon: 'archive' as const }]
            : []),
          { id: 'up', label: t('app.rank.up'), icon: 'chevron-up', disabledReason: canMoveUp ? undefined : t('app.rank.already_first') },
          { id: 'down', label: t('app.rank.down'), icon: 'chevron-down', disabledReason: canMoveDown ? undefined : t('app.rank.already_last') },
          ...(container.type === 'COLLECTION'
            ? [
                { id: 'labels', label: t('app.labels.choose'), icon: 'tag' as const, hasSeparatorBefore: true, disabledReason: isReadOnly ? t('app.workspace.archived') : undefined },
                { id: 'fields', label: t('app.fields.title'), disabledReason: isReadOnly ? t('app.workspace.archived') : undefined },
                { id: 'views', label: t('app.views.title'), icon: 'star' as const },
                { id: 'templates', label: t('app.templates.title'), icon: 'layout-template' as const, disabledReason: isReadOnly ? t('app.workspace.archived') : undefined },
                { id: 'policies', label: t('app.policies.title'), disabledReason: isReadOnly ? t('app.workspace.archived') : undefined },
                { id: 'people', label: t('app.people.title'), icon: 'users' as const },
              ]
            : [{ id: 'people', label: t('app.people.title'), icon: 'users' as const, hasSeparatorBefore: true }]),
          {
            id: 'trash',
            label: t('app.workspace.trash'),
            icon: 'trash',
            isDestructive: true,
            hasSeparatorBefore: true,
            disabledReason: isReadOnly ? t('app.workspace.archived') : undefined,
          },
        ],
  );

  function choseFromMenu(id: string) {
    if (id === 'rename') startRename();
    else if (id === 'move') isMovingHub = true;
    else if (id === 'archive') void toggleArchived();
    else if (id === 'up') void moveBy(-1);
    else if (id === 'down') void moveBy(1);
    else if (id === 'labels') isManagingLabels = true;
    else if (id === 'fields') isManagingFields = true;
    else if (id === 'views') isManagingViews = true;
    else if (id === 'templates') isUsingTemplates = true;
    else if (id === 'policies') isManagingPolicies = true;
    else if (id === 'people') isManagingMembers = true;
    else if (id === 'trash') isTrashing = true;
  }

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
      announcer.say(t('app.workspace.renamed_announced', { name: draft }));
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

{#if live.hasLost(id)}
  <!-- The stream said this reader lost access while they were looking at it. `offline-sync.md` §6
       binds a client to drop what it holds for a container it lost, and leaving them on a page
       drawn from that dropped cache would be showing them what they may no longer read. -->
  <EmptyState kind="filtered" title={t('app.live.access_revoked')} />
{:else if !container && !containers.isSettled(id)}
  <div aria-busy="true"><Skeleton lines={3} /></div>
{:else if !container}
  <!-- Not an error state: the read succeeded and this address is simply not in the workspace. §4.4
       is about a *failure* rendered as an empty list; this is the other way round. -->
  <EmptyState kind="filtered" title={t('app.workspace.not_loaded')} />
{:else}
  <Stack gap="300">
    <!-- The head (ADR-0061 decision 4): where this is, what it is called, the one thing one does
         here most, and the rest behind a menu in three groups. The hub's primary is what a hub is
         for - a collection - with import beside it; the collection's is an entry, with the filter
         beside it and the templates as the verb's own list. The title is read rather than drawn
         where the bar shows it. -->
    <PageHeader
      title={container.name}
      subtitle={container.description ?? undefined}
      isTitleInBar={viewport.isCompact}
      breadcrumb={{
        trail,
        label: t('app.workspace.trail'),
        expandLabel: t('app.workspace.expand_trail'),
        onnavigate: (crumbId) => {
          const crumb = containers.find(crumbId);
          if (crumb) onnavigate(crumb.type === 'HUB' ? `/hubs/${crumbId}` : `/collections/${crumbId}`);
        },
      }}
      primary={container.type === 'HUB'
        ? {
            label: t('app.workspace.create_collection'),
            icon: 'plus',
            onclick: () => (isCreatingCollection = true),
            disabledReason: isReadOnly ? t('app.workspace.archived') : undefined,
            opener: 'add-collection',
          }
        : {
            label: t('app.entries.add'),
            icon: 'plus',
            onclick: addFromHead,
            disabledReason: addDisabledReason,
            opener: 'add-entry',
            menu: {
              label: t('app.entries.add_ways'),
              items: [{ id: 'template', label: t('app.templates.from'), icon: 'layout-template', disabledReason: isReadOnly ? t('app.workspace.archived') : undefined }],
              onselect: () => (isUsingTemplates = true),
            },
          }}
      secondary={container.type === 'HUB'
        ? [
            {
              label: t('app.import.open'),
              onclick: () => (isImporting = true),
              disabledReason: isReadOnly
                ? t('app.workspace.archived')
                : structure.status === 'permitted'
                  ? undefined
                  : structure.status === 'refused'
                    ? t(structure.code, structure.params)
                    : t('app.import.deciding'),
            },
          ]
        : [
            {
              label: filterCount > 0 ? t('app.query.filter_count', { count: String(filterCount) }) : t('app.query.show'),
              icon: 'funnel',
              onclick: () => (isFilterOpen = !isFilterOpen),
              opener: 'filter',
            },
          ]}
      menu={{ label: t('app.workspace.actions', { name: container.name }), items: pageMenu, onselect: choseFromMenu, opener: 'container-menu' }}
    >
      {#snippet notices()}
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
          <p class="failure" role="alert">{failure.message}</p>
        {/if}
      {/snippet}
      {#snippet views()}
        {#if container.type === 'COLLECTION'}
          <!-- The three layouts, with "show what is inside" within the list; the star beside them
               is the saved views where they are used daily (decision 5), the same panel the menu
               opens. -->
          <LayoutSwitch {layout} drawable={DRAWABLE} onlayout={(id) => (layout = id)} />
          <IconButton icon="star" label={t('app.views.title')} size="sm" data-opener="views" onclick={() => (isManagingViews = true)} />
        {/if}
        {#if memberIds.length > 0}
          <!-- Who holds a role here, as faces; pressing them opens the same dialog the menu's
               "People" does. The faces are decoration on the button: its name is the word. -->
          <button type="button" class="members" aria-label={t('app.people.title')} data-opener="members" onclick={() => (isManagingMembers = true)}>
            <span aria-hidden="true">
              <AvatarGroup
                people={memberIds.map((accountId) => ({ name: accounts.nameOf(accountId) ?? t('app.people.unnamed') }))}
                size="sm"
                max={4}
                overflowLabel={t('app.people.more_members', { count: String(Math.max(memberIds.length - 4, 0)) })}
              />
            </span>
          </button>
        {/if}
      {/snippet}
    </PageHeader>

    {#if isRenaming}
      <!-- The field takes the place of the control that opened it, so it takes the focus too (2.4.3). -->
      <Stack gap="150" {@attach focusFirst({ returnTo: '[data-opener="container-menu"]' })}>
        <Input
          label={container.type === 'HUB' ? t('app.workspace.hub_name') : t('app.workspace.collection_name')}
          bind:value={draft}
          error={isNameFailure ? failure?.message : undefined}
        />
        <!-- Everything else is a sentence above the buttons rather than a field error, and a
             version conflict is the ordinary case: nothing about the name is wrong, the row moved
             underneath the reader. -->
        {#if failure && !isNameFailure}
          <p class="failure" role="alert">{failure.message}</p>
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
    {/if}

    <!-- The collection's status, summarised where the manifest says AI is configured here (K-05,
         F5-04), and absent - not gated - where it is not (milestone-F5.md decision 4). -->
    {#if container.type === 'COLLECTION' && hasAi}
      <CollectionSummary containerId={container.id} />
    {/if}

    {#if container.type === 'HUB'}
      {#if collections.length === 0}
        <!-- §4.1: say what this place is for, and offer the one action. An archived hub offers it
             with the reason rather than not at all, which is what every other control here does. -->
        <EmptyState kind="unused" title={t('app.workspace.no_collections')} icon="collection">
          {#snippet action()}
            <Button
              icon="plus"
              disabledReason={isReadOnly ? t('app.workspace.archived') : undefined}
              onclick={() => (isCreatingCollection = true)}
            >
              {t('app.workspace.create_collection')}
            </Button>
          {/snippet}
        </EmptyState>
      {:else}
        <!-- The way to a new collection is the head's primary action now; the list is the list. -->
        <Stack gap="050">
          {#each collections as collection (collection.id)}
            <ListRow href={`/collections/${collection.id}`}>{collection.name}</ListRow>
          {/each}
        </Stack>
      {/if}
    {:else}
      <!-- What the reader has asked of the entries. Everything offered comes from the manifest:
           `query_fields` decides what can be asked, and nothing here is a list written down. The
           panel stays mounted while it is hidden, because the conditions are its state; inline from
           `expanded`, a drawer from the bottom below it. -->
      {#if viewport.isBelowExpanded}
        <Drawer bind:isOpen={isFilterOpen} edge="block-end" title={t('app.query.title')} dismissLabel={t('app.dismiss')}>
          <QueryPanel {layout} onquery={(asked) => (query = asked)} oncount={(count) => (filterCount = count)} custom={customFieldFilters} />
        </Drawer>
      {:else}
        <div class="filter" hidden={!isFilterOpen}>
          <QueryPanel {layout} onquery={(asked) => (query = asked)} oncount={(count) => (filterCount = count)} custom={customFieldFilters} />
        </div>
      {/if}

      <!-- Above the entries, because it is about the ones below it. It draws itself only when
           something is picked, so a reader who never selects anything never sees it. -->
      <BulkBar
        collectionId={container.id}
        path={containerPath}
        onresults={(operations: readonly BulkOperation[], results: readonly BulkResult[]) =>
          (lastResults = byItem(operations, results))}
      />

      <!-- The list, and from `large` the pane beside it (ADR-0061 decision 4): the entry the
           address names, in the same form it takes on a phone. The pane is a place, not a
           feature - `/items/:id` is still the entry's address and this only draws it here. -->
      <div class="split" data-pane={openItemId ? '' : undefined}>
        <div class="entries">
          {#if layout === 'TIMELINE'}
            <TimelineView
              collectionId={container.id}
              {query}
              onopen={(itemId) => onnavigate(`/items/${itemId}`)}
            />
          {:else if layout === 'KANBAN'}
            <Board
              collectionId={container.id}
              isReadOnly={isReadOnly}
              {query}
              {lastResults}
              onduplicate={(item) => (duplicating = item)}
            />
          {:else}
            <!-- Read-only follows the container: an archived collection's entries are archived with
                 it (I-C3), and the reason travels with the controls rather than the controls
                 disappearing. From `large` a row opens beside the list and stays current. -->
            <EntryList
              bind:this={list}
              collectionId={container.id}
              isReadOnly={isReadOnly}
              {query}
              isExpanded={layout === 'LIST_EXPANDED'}
              {lastResults}
              onopen={viewport.isLarge ? openBeside : undefined}
              currentId={openItemId}
              onduplicate={(item) => (duplicating = item)}
            />
          {/if}
        </div>
        {#if openItemId}
          {#key openItemId}
            <DetailPane
              title={openItem?.title ?? ''}
              kind={openItem ? humanise(openItem.type.toLowerCase()) : undefined}
              dismissLabel={t('app.pane.close')}
              pageLabel={t('app.pane.open_page')}
              pageHref={`/items/${openItemId}`}
              onOpenPage={() => onnavigate(`/items/${openItemId}`)}
              onClose={closePane}
            >
              <ItemView id={openItemId} {onnavigate} isInPane />
            </DetailPane>
          {/key}
        {/if}
      </div>
    {/if}
  </Stack>
{/if}

{#if container?.type === 'HUB'}
  <CreateContainerDialog
    bind:isOpen={isCreatingCollection}
    type="COLLECTION"
    parentId={container.id}
    oncreated={(collectionId) => onnavigate(`/collections/${collectionId}`)}
  />
  <ImportDialog bind:isOpen={isImporting} hubId={container.id} {onnavigate} />
{/if}

<DuplicateDialog
  item={duplicating}
  hubId={container?.type === 'COLLECTION' ? (container.parent_id ?? undefined) : container?.id}
  onclose={() => (duplicating = undefined)}
  onopened={(itemId) => {
    duplicating = undefined;
    onnavigate(`/items/${itemId}`);
  }}
/>

{#if container?.type === 'COLLECTION'}
  <LabelsDialog bind:isOpen={isManagingLabels} collectionId={container.id} />
  <CustomFieldsDialog
    bind:isOpen={isManagingFields}
    collectionId={container.id}
    role={structureRole}
  />
  <PoliciesDialog
    bind:isOpen={isManagingPolicies}
    {container}
    path={containerPath}
    role={structureRole}
  />
  <ViewsPanel
    bind:isOpen={isManagingViews}
    collectionId={container.id}
    {query}
    {layout}
    role={structureRole}
    onapply={(asked, appliedLayout, name) => {
      // Both halves: the query the server validated, and the layout only a client knows what to do
      // with — which is what the `layout` field has been for since it was declared uninterpreted.
      query = asked;
      layout = appliedLayout;
      announcer.say(t('app.views.layout_applied', { name, layout: t(`app.view.${appliedLayout}`) }));
    }}
    onexport={(view) => {
      isManagingViews = false;
      exporting = view;
    }}
    onsubscribe={(view) => {
      isManagingViews = false;
      subscribing = view;
      isManagingFeeds = true;
    }}
  />
  <ExportDialog view={exporting} onclose={() => (exporting = undefined)} />
  <FeedsDialog bind:isOpen={isManagingFeeds} view={subscribing} collectionId={container.id} />
  <TemplatesDialog
    bind:isOpen={isUsingTemplates}
    collectionId={container.id}
    path={containerPath}
    role={structureRole}
    onopened={(itemId) => {
      isUsingTemplates = false;
      onnavigate(`/items/${itemId}`);
    }}
  />
{/if}

{#if container}
  <!-- The path is the container's own: a collection composes the workspace, its hub and itself,
       and a hub composes the workspace and itself. -->
  <MembersDialog
    bind:isOpen={isManagingMembers}
    title={t('app.people.title')}
    scope={container.type === 'HUB'
      ? { scopeType: 'HUB', scopeId: container.id }
      : { scopeType: 'COLLECTION', scopeId: container.id }}
    path={container.type === 'HUB'
      ? { hubId: container.id }
      : { hubId: container.parent_id ?? undefined, collectionId: container.id }}
  />
{/if}

{#if isTrashing && container}
  <!-- Reversible, and confirmed anyway: a hub is two hundred entries, and "one deletion" is worth
       saying before it happens rather than afterwards. The primary action is the caller's to name
       and to place last — and it is named for what it does, not "OK". -->
  <Dialog
    bind:isOpen={isTrashing}
    title={t('app.workspace.trash_confirm_title', { name: container.name })}
    dismissLabel={t('app.workspace.cancel')}
  >
    <p class="notice">
      {container.type === 'HUB'
        ? t('app.workspace.trash_confirm_hub')
        : t('app.workspace.trash_confirm_collection')}
    </p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isTrashing = false)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button tone="danger" isBusy={isTrashingNow} busyLabel={t('app.workspace.saving')} onclick={moveToTrash}>
        {t('app.workspace.confirm')}
      </Button>
    {/snippet}
  </Dialog>
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
  .split { display: flex; align-items: flex-start; gap: var(--sp-300); min-width: 0; }

  .entries { flex: 1; min-width: 0; }

  /* The pane keeps to the top while the list scrolls under it, and scrolls inside itself. */
  .split[data-pane] > :global(aside) {
    position: sticky;
    inset-block-start: calc(var(--layout-appbar-height) + var(--sp-200));
    max-block-size: calc(100vh - var(--layout-appbar-height) - var(--sp-400));
    overflow: auto;
  }

  .filter[hidden] { display: none; }

  .filter {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .members {
    display: inline-flex;
    align-items: center;
    padding: var(--sp-025);
    border: 0;
    border-radius: var(--r-full);
    background: transparent;
    cursor: pointer;
  }

  .members:hover { background: var(--bg-surface-hover); }

  .members:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .notice { margin: 0; color: var(--text-warning); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

</style>
