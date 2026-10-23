<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub or a collection, at its own address.
  //
  // One view for both, because the two differ in what they *hold* rather than in what they are:
  // a hub holds collections, a collection will hold entries. Everything above that line —
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
    Icon,
    IconButton,
    Menu,
    PageHeader,
    ReadOnlyNotice,
    Skeleton,
    Stack,
    type MenuItem,
    type PageAction,
  } from '@hubtask/design-system/components';

  import { untrack } from 'svelte';

  import Board from '../lib/entries/Board.svelte';
  import CollectionSummary from '../lib/entries/CollectionSummary.svelte';
  import CollectionFigure from '../lib/entries/CollectionFigure.svelte';
  import StructurePanel from '../lib/workspace/StructurePanel.svelte';
  import ContainerMilestone from '../lib/entries/ContainerMilestone.svelte';
  import { showsMilestone } from '../lib/data/kudos.ts';
  import { celebration } from '../lib/celebration.svelte.ts';
  import BulkBar from '../lib/entries/BulkBar.svelte';
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
  import EmergencyNotice from '../lib/workspace/EmergencyNotice.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { deviceStore, openingLayout, rememberLayout } from '../lib/entries/layoutmemory.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { changeRefusalCode, holds, mayDo, offerIn, roleOrder } from '../lib/data/capability.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { customFields } from '../lib/data/customfields.svelte.ts';
  import { templates } from '../lib/data/templates.svelte.ts';
  import { feeds, views as savedViews } from '../lib/data/views.svelte.ts';
  import { queryOf } from '../lib/data/views.ts';
  import { captureRulesOf } from '../lib/entries/capture.ts';
  import { queryFieldsFor } from '../lib/data/customfields.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { selection } from '../lib/data/selection.svelte.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { suggestions } from '../lib/data/suggestions.svelte.ts';
  import { isPending } from '../lib/data/suggestions.ts';
  import { byItem } from '../lib/data/bulk.ts';
  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import ContainerEditDialog from '../lib/workspace/ContainerEditDialog.svelte';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';
  import CollectionCards from '../lib/workspace/CollectionCards.svelte';
  import { ownsHub, privacyAction, shutOut } from '../lib/workspace/privacy.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import ItemView from './ItemView.svelte';

  import { announcer } from '../lib/announce.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { recents } from '../lib/recents.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { archivalOf } from '../lib/data/containers.ts';
  import { DRAWABLE, isListLayout, layoutsOffered } from '../lib/entries/layouts.ts';
  import { actionsOfHead, type Act } from '../lib/palette/actions.ts';
  import { commands } from '../lib/palette/commands.svelte.ts';
  import { mayCreateIn } from '../lib/palette/sources.ts';
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
   * The layout a person chose for a collection is kept for them on this device (UC-WRK-28 check 6,
   * design-system.md §11.10) and sent nowhere; saving a view is its own act, and a switch here
   * writes no `SavedView`.
   */
  let layout = $state('LIST_COLLAPSED');
  let query = $state<ItemsQuery>({});

  const container = $derived(containers.find(id));

  /** What this collection offers: the installation's answers, narrowed to its structure in force. */
  const offer = $derived(offerIn(container?.structure?.settings));

  /** The layouts that may be chosen here: the switch's and the palette's one answer. */
  const offeredLayouts = $derived(container ? layoutsOffered(container, manifest.value) : []);
  /** The list layout this collection offers, where the add form lives. */
  const listLayout = $derived(offeredLayouts.find(isListLayout) ?? 'LIST_COLLAPSED');

  /**
   * What the opening layout is chosen from: what may be chosen here, the list's both values where
   * the list is offered; everything this client draws while the manifest is not yet read.
   */
  const choosable = $derived(
    offeredLayouts.length === 0
      ? DRAWABLE
      : DRAWABLE.filter((each) => offeredLayouts.includes(each) || (isListLayout(each) && offeredLayouts.some(isListLayout))),
  );

  /** What the collection offers and opens with, as one key, so a re-read that changes neither leaves the layout alone. */
  const offeredViews = $derived(JSON.stringify({ views: container?.structure?.settings?.views ?? null, choosable }));

  // The layout a collection opens with: the person's remembered one where it is still offered,
  // else the collection's default, else the list.
  $effect(() => {
    const wanted = id;
    const account = actor.account?.id;
    const read = JSON.parse(offeredViews) as {
      views: { offered?: string[]; default?: string } | null;
      choosable: string[];
    };
    untrack(() => {
      layout = openingLayout({
        store: deviceStore(),
        accountId: account,
        collectionId: wanted,
        offered: read.views?.offered ?? [],
        preferred: read.views?.default,
        drawable: read.choosable,
      });
    });
  });

  /** A layout the person chose: drawn, and kept for them on this device. */
  function chooseLayout(chosen: string) {
    layout = chosen;
    rememberLayout(deviceStore(), actor.account?.id, id, chosen);
  }

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

  /**
   * The reader's effective role here - the strongest along the container's path, their own or a
   * group's - which is the role the server decides by (`people.roleOf`). `undefined` while it is
   * still being read; the shape controls and the dialogs then wait rather than guess.
   */
  const myRole = $derived(container ? people.roleOf(containerPath, actor.account?.id, roleOrder()) : undefined);
  const structureRole = $derived(myRole ?? undefined);
  /** The workspace's shape is the reader's to change: *Save as preset* then offers the workspace. */
  const mayShapeWorkspace = $derived(holds(people.roleOf({}, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted');
  /** Whether the reader may create entries in this collection, and why a row is not theirs to change. */
  const mayCreateHere = $derived(mayDo(myRole, 'create', false) === true);
  function changeRefusalOf(row: WorkItem): string | undefined {
    const code = changeRefusalCode(myRole, row.assignee_id != null && row.assignee_id === actor.account?.id);
    return code ? t(code) : undefined;
  }

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

  // `Escape` leaves the selection mode, from anywhere on the screen: a mode with no way out but a
  // small button is one somebody is stuck in (ADR-0063 decision 8). It is on the window rather
  // than on a node, because the reader may be anywhere — a filter panel, a dialog's trigger, the
  // page head — and a dialog that is open takes the key first, as its own overlay should.
  $effect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || !selection.isOn) return;
      if (document.querySelector('dialog[open], [role="dialog"], [role="menu"]')) return;
      selection.stop();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

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

  /** The ✦ at the figure: how this collection stands, asked of AI and drawn under the head. */
  async function askHowItStands() {
    if (!container) return;
    failure = undefined;
    try {
      await suggestions.askContainer(container.id);
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    }
  }
  const isReadOnly = $derived(archival !== 'active');

  /**
   * The level a kept summary becomes an entry of: the first top level that carries notes, where the
   * reader may add one and the collection is not read-only. None, and the summary is read only.
   */
  const summaryKeepType = $derived(
    mayCreateHere && !isReadOnly ? offer.rootTypes().find((type) => offer.supports(type, 'NOTES').status === 'permitted') : undefined,
  );
  /** Whether any level of this collection carries a capability - what its filter may ask about. */
  const carriedHere = $derived.by(() => {
    const levels = container?.structure?.settings?.levels;
    if (!levels) return undefined;
    return (capability: string) => levels.some((level) => offer.supports(level.type as string, capability).status === 'permitted');
  });
  const summaryAsk = $derived(container ? suggestions.askingOf(container.id) : undefined);


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
   * The keyboard path: WCAG 2.2 SC 2.5.7 wants a single-pointer alternative to every drag, and
   * the drag is built **against** this rather than the other way round. A rank change is a command before it is a gesture.
   *
   * A **hub** goes through `:reorder`, which is the only operation that can rank one — it sits in
   * nothing, so `:move`'s required `target_parent_id` has nothing to name. A collection
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
   * hubs have a `:reorder` of their own.
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
   * whole batch back (I-C2), so this is not an irreversible act — but a hub is
   * two hundred entries and "one deletion" is the thing worth saying before it happens rather than
   * afterwards. The sentence says what goes with it, which is what a person is actually deciding.
   */
  // The collection's labels: create, update and remove, so the set the picker offers can be made
  // inside the application.
  //
  // No subscription of its own: a collection renders either the list or the board, and both open
  // the level already. A third reader of one list is what the stores exist to avoid.
  /** *Edit*: its name, description, colour and symbol (UC-WRK-33), and a hub's preset. */
  let isEditing = $state(false);
  let isUsingTemplates = $state(false);
  let isManagingViews = $state(false);
  /** Which act of the *Views* menu opened the views dialog: saving, or changing one. */
  let viewsIntent = $state<'save' | 'manage'>('manage');
  /** The question of the view opened last, which the filter's controls take over. */
  let appliedView = $state<ItemsQuery | undefined>(undefined);
  /** The collection as it is filtered, being exported without a saved view. */
  let isExportingFiltered = $state(false);
  /**
   * The hub's or the collection's *Structure* panel (UC-WRK-23), and its open tab: the settings,
   * or one of the collection's parts - labels, fields, templates, automation, people
   * (ux-inventory.md §E4).
   */
  let isShaping = $state(false);
  let shapingTab = $state('settings');
  let isManagingFeeds = $state(false);
  /** The view an export or a subscription is about, if either is open. */
  let exporting = $state<SavedView | undefined>(undefined);
  let subscribing = $state<SavedView | undefined>(undefined);
  let isManagingMembers = $state(false);
  let isImporting = $state(false);
  /**
   * `STRUCTURE` is what an import takes: on the hub because it creates collections there, on a
   * collection because it may add the file's columns and labels.
   */
  const structure = $derived(holds(structureRole, 'STRUCTURE'));
  /**
   * A control that changes the shape of the workspace, or nothing where the reader's role may not
   * (UC-WRK-22 check 4, UC-WRK-01 check 6): absent rather than offered and refused (P-05).
   */
  const mayShape = $derived(structure.status === 'permitted');
  /**
   * Why nothing here can be changed - the notice's sentence, and the reason every control that
   * stays on the page gives, so the page says one thing (design-system.md §4, case 5). Under an
   * archived hub it names the hub, which is where *Unarchive* is; here there is none. Archived in
   * its own right, it tells a reader who may not unarchive whom to ask instead.
   */
  const readOnlyReason = $derived(
    archival === 'inherited'
      ? t('app.workspace.archived_above', { hub: trail[0]?.label ?? '' })
      : archival === 'archived'
        ? t(mayShape ? 'app.workspace.archived' : 'app.workspace.archived_ask')
        : undefined,
  );
  /** The notice's sentence, which the boxes that stay in the list name as their description. */
  const instanceId = $props.id();
  const readOnlyNoticeId = `container-${instanceId}-read-only`;
  function structureControl<T>(control: T): T | undefined {
    return mayShape ? control : undefined;
  }

  let isTrashing = $state(false);
  let isTrashingNow = $state(false);

  /**
   * Which privacy control the hub's menu draws, if any (UC-ID-16): only the one the server would
   * accept (P-05, `privacyAction`). The role on the workspace is read on its own path - along a
   * private hub it is not in force, and it is exactly what decides the right to create hubs.
   */
  $effect(() => untrack(() => workspace.open()));
  const workspaceRole = $derived(
    people
      .along({})
      .find((membership) => membership.account_id === actor.account?.id && membership.scope_type === 'TENANT')
      ?.role as string | undefined,
  );
  const privacyControl = $derived(
    container?.type === 'HUB'
      ? privacyAction({
          isPrivate: container.private === true,
          ownsHub: ownsHub(people.along(containerPath), actor.account?.id, container.id),
          mayCreateHubs: holds(workspaceRole, 'STRUCTURE').status === 'permitted',
          isOffered: workspace.workspace?.private_hubs_offered === true,
        })
      : undefined,
  );
  /** Who loses the hub when it turns private: read it through the workspace, hold nothing on it. */
  const losers = $derived(container?.type === 'HUB' ? shutOut(people.along(containerPath), container.id) : []);
  $effect(() => {
    accounts.resolve(losers);
  });
  let isChangingPrivacy = $state(false);
  let isChangingPrivacyNow = $state(false);

  async function changePrivacy() {
    if (!container) return;
    const name = container.name;
    const next = !container.private;
    isChangingPrivacyNow = true;
    failure = undefined;
    try {
      await containers.setPrivacy(container.id, next, container.version);
      isChangingPrivacy = false;
      announcer.say(t(next ? 'app.workspace.private_announced' : 'app.workspace.shared_announced', { name }));
    } catch (error) {
      isChangingPrivacy = false;
      failure = renderProblem(error as TransportError, messages);
    } finally {
      isChangingPrivacyNow = false;
    }
  }

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

  // Creating a collection in this hub. A hub that holds none is where a workspace is started, and
  // before this the only way to start one was `hubctl`.
  let isCreatingCollection = $state(false);

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // The bar carries the title on a phone (ADR-0061 decision 2); the head then reads its heading
  // rather than drawing it. Cleared when this screen leaves.
  $effect(() => page.entitle(container?.name));

  // What the overview's "what you had open" is built from, for the two container levels. Noted
  // once the level is known, because the mark on the row is what it says.
  $effect(() => {
    if (!container?.name) return;
    recents.note({ kind: container.type === 'HUB' ? 'hub' : 'collection', id: container.id, title: container.name });
  });

  /** The list, for the head's primary action: the form is the list's, the button is the head's. */
  let list = $state<EntryList | undefined>(undefined);

  /** The filter: open inline from `expanded`, in a drawer below it; the count is the panel's. */
  let isFilterOpen = $state(false);
  let filterCount = $state(0);

  /** Why an entry cannot be created here, or nothing. The gate is the list's; the words are shared. */
  const addDisabledReason = $derived(
    isReadOnly
      ? readOnlyReason
      : offer.rootTypes().length === 0
        ? t('app.entries.no_root_type')
        : undefined,
  );

  /** Creating from the head: on a board or the timeline, the list is where the form lives. */
  function addFromHead(several = false) {
    if (!container || addDisabledReason) return;
    if (layout === 'KANBAN' || layout === 'TIMELINE') layout = listLayout;
    // After the layout switch the list is mounted on the next tick.
    queueMicrotask(() => list?.addEntry(several));
  }

  /**
   * Whether the collection's capture line takes several lines as several entries (UC-WRK-25
   * check 1) - the rule every capture field reads, so the split offers *Several lines…* exactly
   * where the line would split them.
   */
  const splitsLines = $derived(captureRulesOf(container?.structure, '', []).splitsLines);

  /** Where the people dialog and the *People* tab grant and read roles: the container's own scope and path. */
  const memberScope = $derived(
    container?.type === 'HUB'
      ? { scopeType: 'HUB' as const, scopeId: container.id }
      : { scopeType: 'COLLECTION' as const, scopeId: container?.id ?? '' },
  );
  const memberPath = $derived(
    container?.type === 'HUB' ? { hubId: container.id } : { hubId: container?.parent_id ?? undefined, collectionId: container?.id },
  );
  /** Who holds a role here, for the faces in the head: the same dialog the *People* tab draws. */
  const memberIds = $derived([...new Set(people.along(containerPath).map((membership) => membership.account_id))]);
  $effect(() => {
    accounts.resolve(memberIds);
  });

  /**
   * The page menu, in the three groups of design-system.md §11.8: act on the object, set it up,
   * and trash - last and alone. A reason stays a reason: the rank at the top of its level and
   * the move of a hub are offered with why they cannot be used, not left out.
   */
  /** Whether the reader may change any entry here at all - what selecting entries is for. */
  const mayChangeSome = $derived(mayDo(myRole, 'change', true) === true);
  /** Whether the reader may put this container in the trash: `DELETE_CONTAINER`, the owner's alone. */
  const mayDelete = $derived(holds(structureRole, 'DELETE_CONTAINER').status === 'permitted');
  /** The menu rows that change the workspace's shape, or none where the reader's role may not. */
  function shaping<T>(rows: readonly T[]): readonly T[] {
    return mayShape ? rows : [];
  }

  /**
   * The *Structure* panel's row (UC-WRK-23 checks 1, 9): only where the container answers its
   * structure, and - through `shaping` - only for a reader who may change the shape. What counts
   * is one of its settings, no longer a row of its own.
   */
  const structureRow = $derived(
    container?.structure ? [{ id: 'structure', label: t('app.structure.menu'), icon: 'settings' as const, disabledReason: readOnlyReason }] : [],
  );

  const pageMenu = $derived<MenuItem[]>(
    container === undefined
      ? []
      : [
          // The way into the selection mode, where a screen's verbs are (ADR-0063 decision 8).
          // Only on a collection: a hub holds collections, and nothing acts on those in bulk.
          ...(container.type === 'COLLECTION' && mayChangeSome
            ? [{ id: 'select', label: t('app.bulk.select_mode'), icon: 'square-check' as const, hasSeparatorBefore: false }]
            : []),
          // What changes the shape of the workspace - its name, its place, its order, its archive -
          // only for a role that may (UC-WRK-01 check 6, P-05); `shaping` drops the rest.
          ...shaping<MenuItem>([
          // *Edit*: the name, the description, the colour and symbol, and a hub's preset.
          { id: 'edit', label: t('app.workspace.edit'), icon: 'pencil', disabledReason: readOnlyReason },
          // A hub sits in nothing, so only a collection moves to another hub (ux-inventory.md §E9).
          ...(container.type === 'COLLECTION' ? [{ id: 'move', label: t('app.move.to_hub'), disabledReason: readOnlyReason }] : []),
          ...(archival !== 'inherited'
            ? [{ id: 'archive', label: archival === 'archived' ? t('app.workspace.unarchive') : t('app.workspace.archive'), icon: 'archive' as const }]
            : []),
          { id: 'up', label: t('app.rank.up'), icon: 'chevron-up', disabledReason: canMoveUp ? undefined : t('app.rank.already_first') },
          { id: 'down', label: t('app.rank.down'), icon: 'chevron-down', disabledReason: canMoveDown ? undefined : t('app.rank.already_last') },
          ]),
          // The labels, the fields, the templates, the automation and the people are *Structure*'s
          // tabs (ux-inventory.md §E4), offered to whom the shape belongs; everybody reaches the
          // people by the faces in the head and a template by the add button's *From a template*
          // (UC-WRK-09 check 7). The views, the export and the subscriptions are the head's *Views*
          // menu, and only there (ux-inventory.md §E9).
          ...shaping(structureRow.map((row) => ({ ...row, hasSeparatorBefore: true }))),
          ...(privacyControl
            ? [
                {
                  id: 'privacy',
                  label: privacyControl === 'make_shared' ? t('app.workspace.make_shared') : t('app.workspace.make_private'),
                  icon: 'lock' as const,
                },
              ]
            : []),
          ...(mayDelete
            ? [{
                id: 'trash',
                label: t('app.workspace.trash'),
                icon: 'trash' as const,
                isDestructive: true,
                hasSeparatorBefore: true,
                disabledReason: readOnlyReason,
              }]
            : []),
        ],
  );

  /**
   * The *Views* menu (ux-inventory.md §E4): the saved views that apply here, each opened by its
   * row, then saving what is on screen, changing a view, exporting the list as it is filtered and
   * the calendar subscriptions.
   */
  const heldViews = $derived(container?.type === 'COLLECTION' ? savedViews.of(container.id) : []);
  const viewsMenu = $derived<MenuItem[]>(container?.type !== 'COLLECTION' ? [] : [
    ...heldViews.map((view) => ({ id: `open:${view.id}`, label: t('app.views.open_named', { name: view.name }), icon: 'saved-view' as const })),
    { id: 'save', label: t('app.views.menu_save'), icon: 'star' as const, hasSeparatorBefore: heldViews.length > 0 },
    ...(heldViews.length > 0 ? [{ id: 'manage', label: t('app.views.menu_manage'), icon: 'pencil' as const }] : []),
    { id: 'export', label: t('app.views.menu_export'), icon: 'file-text' as const, hasSeparatorBefore: true },
    { id: 'feeds', label: t('app.views.menu_feeds'), icon: 'calendar' as const },
  ]);

  function choseView(id: string) {
    if (id.startsWith('open:')) {
      const view = heldViews.find((each) => `open:${each.id}` === id);
      if (view) applyView(queryOf(view.query as Record<string, unknown>, view.grouping as Record<string, unknown>), view.layout, view.name);
    } else if (id === 'save' || id === 'manage') {
      viewsIntent = id;
      isManagingViews = true;
    } else if (id === 'export') isExportingFiltered = true;
    else if (id === 'feeds') {
      subscribing = undefined;
      isManagingFeeds = true;
    }
  }

  /** Opens a view: its query and its layout together - the half only a client knows what to do with is the layout. */
  function applyView(asked: ItemsQuery, appliedLayout: string, name: string) {
    query = asked;
    appliedView = asked;
    chooseLayout(appliedLayout);
    announcer.say(t('app.views.layout_applied', { name, layout: t(`app.view.${appliedLayout}`) }));
  }

  function choseFromMenu(id: string) {
    if (id === 'select') selection.start();
    else if (id === 'edit') isEditing = true;
    else if (id === 'move') isMovingHub = true;
    else if (id === 'archive') void toggleArchived();
    else if (id === 'up') void moveBy(-1);
    else if (id === 'down') void moveBy(1);
    else if (id === 'structure') openStructure('settings');
    else if (id === 'privacy') isChangingPrivacy = true;
    else if (id === 'trash') isTrashing = true;
  }

  /** The *Structure* panel, open on one of its tabs. */
  function openStructure(tab: string) {
    shapingTab = tab;
    isShaping = true;
  }

  /**
   * The head's three lists, built once and handed to the head and to the palette alike: the
   * palette lists what the head offers and nothing else (UC-WRK-27 check 2). The hub's primary is
   * what a hub is for - a collection - with import beside it; the collection's is an entry, with
   * the filter beside it and the templates as the verb's own list.
   */
  /** "Add an entry", or "Add item" where the one top level carries a name a person gave it (UC-WRK-23 check 6). */
  const addLabel = $derived.by(() => {
    const tops = offer.rootTypes();
    const only = tops.length === 1 ? offer.level(tops[0] ?? '') : undefined;
    return only?.name !== undefined ? only.add : t('app.entries.add');
  });
  const headPrimary = $derived<PageAction | undefined>(
    container === undefined
      ? undefined
      : container.type === 'HUB'
        ? structureControl({
            label: t('app.workspace.create_collection'),
            icon: 'plus',
            onclick: () => (isCreatingCollection = true),
            disabledReason: readOnlyReason,
            opener: 'add-collection',
          })
        : mayCreateHere
          ? {
              label: addLabel,
              icon: 'plus',
              onclick: () => addFromHead(),
              disabledReason: addDisabledReason,
              opener: 'add-entry',
              menu: {
                label: t('app.entries.add_ways'),
                items: [
                  { id: 'template', label: t('app.templates.from'), icon: 'layout-template', disabledReason: readOnlyReason },
                  ...(splitsLines ? [{ id: 'lines', label: t('app.entries.add_lines'), icon: 'menu' as const, disabledReason: addDisabledReason }] : []),
                  // Into this collection (ux-inventory.md §E4); absent where the reader may not shape it.
                  ...(mayShape ? [{ id: 'import', label: t('app.import.open'), icon: 'cloud-upload' as const, disabledReason: readOnlyReason }] : []),
                ],
                onselect: (way: string) => {
                  if (way === 'lines') addFromHead(true);
                  else if (way === 'import') isImporting = true;
                  else isUsingTemplates = true;
                },
              },
            }
          : undefined,
  );
  const headSecondary = $derived<readonly [] | readonly [PageAction]>(
    container?.type === 'HUB'
      ? mayShape
        ? [
            {
              label: t('app.import.open'),
              onclick: () => (isImporting = true),
              disabledReason: readOnlyReason,
            },
          ]
        : []
      : [
          {
            label: filterCount > 0 ? t('app.query.filter_count', { count: String(filterCount) }) : t('app.query.show'),
            icon: 'funnel',
            onclick: () => (isFilterOpen = !isFilterOpen),
            opener: 'filter',
          },
        ],
  );
  const headMenu = $derived({
    label: t('app.workspace.actions', { name: container?.name ?? '' }),
    items: pageMenu,
    onselect: choseFromMenu,
    opener: 'container-menu',
  });

  /** The other layouts this collection is shown in, as the palette's *Show as …*. */
  const layoutActs = $derived<readonly Act[]>(
    offeredLayouts
      .filter((each) => each !== layout && !(each.startsWith('LIST_') && layout.startsWith('LIST_')))
      .map((each) => ({
        id: `page:layout-${each}`,
        label: t('app.palette.show_as', { layout: each }),
        run: () => chooseLayout(each),
      })),
  );

  /** The *Views* menu's rows, as the palette lists them: the menu is the head's, so is the palette's. */
  const viewActs = $derived<readonly Act[]>(
    viewsMenu.map((item) => ({ id: `page:views-${item.id}`, label: item.label, icon: item.icon, run: () => choseView(item.id) })),
  );

  // What the palette may list here: the head's own lists, then the views, then the layouts.
  $effect(() => commands.offer('page', [...actionsOfHead('page', { primary: headPrimary, secondary: headSecondary, menu: headMenu }), ...viewActs, ...layoutActs]));

  // A wish the palette left for this collection - a layout, or the add form - taken once and acted
  // on only where this screen's own answers allow it. It wins over any layout this screen would
  // otherwise open with: it is what the reader just asked for.
  $effect(() => {
    if (!container || myRole === undefined || !commands.isWaitingFor(container.id)) return;
    const wish = commands.take(container.id);
    if (!wish) return;
    untrack(() => {
      if (wish.layout && offeredLayouts.includes(wish.layout)) chooseLayout(wish.layout);
      if (wish.add && mayCreateIn(container, myRole, manifest.value)) {
        if (layout === 'KANBAN' || layout === 'TIMELINE') layout = listLayout;
        isAddWished = true;
      }
    });
  });

  // The add form is the list's, and on arrival the list may not be drawn yet: the wish waits for
  // it rather than being spent on a list that is not there.
  let isAddWished = $state(false);
  $effect(() => {
    if (!isAddWished || !list) return;
    isAddWished = false;
    const drawn = list;
    untrack(() => drawn.addEntry());
  });
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
  <!-- What counts in the collection, counted by the server on its read (UC-WRK-29). Absent on a
       hub, offline, and where the collection counts nothing. Beside it the milestone of a
       collection or hub, while the reader celebrates (checks 4 and 7). -->
  {@const hasMilestone = showsMilestone(container.milestone, celebration.isOn)}
  {#snippet collectionFigure()}
    {#if container.type === 'COLLECTION' && hasAi}
      <!-- The ✦ where what counts is said (ux-inventory.md §E4): the question is about the figure. -->
      <IconButton
        icon="sparkles"
        size="sm"
        label={t('app.workspace.summarise')}
        disabledReason={isPending(summaryAsk) ? t('app.suggestions.pending') : undefined}
        data-opener="summarise"
        onclick={() => void askHowItStands()}
      />
    {/if}
    {#if container.figure}<CollectionFigure figure={container.figure} name={container.name} />{/if}
    {#if hasMilestone && container.milestone}<ContainerMilestone milestone={container.milestone} />{/if}
  {/snippet}
  {#snippet ownMark()}<ContainerMark look={container} size="md" />{/snippet}
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
      isMenuInBar={viewport.isCompact}
      onmenu={(offered) => page.offer(offered)}
      breadcrumb={{
        trail,
        label: t('app.workspace.trail'),
        expandLabel: t('app.workspace.expand_trail'),
        onnavigate: (crumbId) => {
          const crumb = containers.find(crumbId);
          if (crumb) onnavigate(crumb.type === 'HUB' ? `/hubs/${crumbId}` : `/collections/${crumbId}`);
        },
      }}
      primary={headPrimary}
      secondary={headSecondary}
      menu={headMenu}
      figure={(container.type === 'COLLECTION' && (container.figure || hasAi)) || hasMilestone ? collectionFigure : undefined}
      mark={ownMark}
    >
      {#snippet notices()}
        <!-- The two archive states say different things and offer different controls, which is the
             whole reason `archivalOf` distinguishes them. -->
        {#if container.type === 'HUB' && container.private}
          <p class="notice private"><Icon name="lock" size="sm" />{t('app.workspace.private_marker')}</p>
        {/if}
        {#if readOnlyReason}
          <!-- One notice for the whole page; the rows below say nothing about it. -->
          <ReadOnlyNotice text={readOnlyReason} textId={readOnlyNoticeId}>
            {#snippet action()}
              {#if archival === 'archived' && mayShape}
                <Button size="sm" tone="secondary" onclick={() => void toggleArchived()}>{t('app.workspace.unarchive')}</Button>
              {/if}
            {/snippet}
          </ReadOnlyNotice>
        {/if}
        {#if failure}
          <p class="failure" role="alert">{failure.message}</p>
        {/if}
      {/snippet}
      {#snippet views()}
        {#if container.type === 'COLLECTION'}
          <!-- The three layouts, with "show what is inside" within the list; beside them the one
               *Views* menu (design-system.md §11.8): the saved views, saving, the export and the
               calendar subscriptions. -->
          <LayoutSwitch {layout} offered={offeredLayouts} reported={offer.layouts} onlayout={chooseLayout} />
          <Menu label={t('app.views.menu')} items={viewsMenu} onselect={choseView}>
            {#snippet trigger(props)}
              <Button size="sm" tone="secondary" icon="saved-view" data-opener="views" {...props}>{t('app.views.menu')}</Button>
            {/snippet}
          </Menu>
        {/if}
        {#if memberIds.length === 0}
          <!-- Nobody to show yet: the way to the people stays, as their mark (ux-inventory.md §E9). -->
          <IconButton icon="users" label={t('app.people.title')} size="sm" data-opener="members" onclick={() => (isManagingMembers = true)} />
        {:else}
          <!-- Who holds a role here, as faces; pressing them opens the people, the same component
               as *Structure → People*. The faces are decoration on the button: its name is the word. -->
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

    {#if container.type === 'HUB' && container.private}
      <!-- Who opened this hub through the emergency access, and the opener's own hour (ADR-0073 §4). -->
      <EmergencyNotice hubId={container.id} />
    {/if}

    <!-- The collection's status, summarised where the manifest says AI is configured here,
         and absent - not gated - where it is not (design-system.md §4, CapabilityGate). -->
    {#if container.type === 'COLLECTION' && hasAi}
      <CollectionSummary containerId={container.id} part="summary" keepAs={summaryKeepType} />
    {/if}

    {#if container.type === 'HUB'}
      {#if collections.length === 0}
        <!-- §4.1: say what this place is for, and offer the one action. An archived hub keeps it in
             its place with the page's reason as a bubble, as the head's own actions do
             (design-system.md §4, case 2), so the page does not change shape with the archive. -->
        <EmptyState kind="unused" title={t('app.workspace.no_collections')} icon="collection">
          {#snippet action()}
            {#if mayShape}
              <Button
                icon="plus"
                disabledReason={readOnlyReason}
                onclick={() => (isCreatingCollection = true)}
              >
                {t('app.workspace.create_collection')}
              </Button>
            {/if}
          {/snippet}
        </EmptyState>
      {:else}
        <!-- The way to a new collection is the head's primary action now; the collections are
             cards with what counts in each (ux-inventory.md §E4). -->
        <CollectionCards {collections} />
      {/if}
    {:else}
      <!-- What the reader has asked of the entries. Everything offered comes from the manifest:
           `query_fields` decides what can be asked, and nothing here is a list written down. The
           panel stays mounted while it is hidden, because the conditions are its state; inline from
           `expanded`, a drawer from the bottom below it. -->
      {#if viewport.isBelowExpanded}
        <Drawer bind:isOpen={isFilterOpen} edge="block-end" title={t('app.query.title')} dismissLabel={t('app.dismiss')}>
          <QueryPanel {layout} applied={appliedView} onquery={(asked) => (query = asked)} oncount={(count) => (filterCount = count)} custom={customFieldFilters} carried={carriedHere} />
        </Drawer>
      {:else}
        <div class="filter" hidden={!isFilterOpen}>
          <QueryPanel {layout} applied={appliedView} onquery={(asked) => (query = asked)} oncount={(count) => (filterCount = count)} custom={customFieldFilters} carried={carriedHere} />
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
              {isReadOnly}
              changeRefusal={changeRefusalOf}
              onopen={(itemId) => (viewport.isLarge ? openBeside(itemId) : onnavigate(`/items/${itemId}`))}
              currentId={openItemId}
              {onnavigate}
              mayCreate={mayCreateHere}
              mayShare={holds(myRole ?? undefined, 'MANAGE_MEMBERS').status === 'permitted'}
            />
          {:else if layout === 'KANBAN'}
            <Board
              collectionId={container.id}
              isReadOnly={isReadOnly}
              {query}
              {lastResults}
              onopen={viewport.isLarge ? openBeside : undefined}
              currentId={openItemId}
              {onnavigate}
              mayShare={holds(myRole ?? undefined, 'MANAGE_MEMBERS').status === 'permitted'}
              changeRefusal={changeRefusalOf}
              mayCreate={mayCreateHere}
              {mayShape}
            />
          {:else}
            <!-- Read-only follows the container: an archived collection's entries are read-only with
                 it (I-C3), and the notice above says so once; the rows say nothing of their own.
                 From `large` a row opens beside the list and stays current. -->
            <EntryList
              bind:this={list}
              collectionId={container.id}
              isReadOnly={isReadOnly}
              {readOnlyReason}
              {readOnlyNoticeId}
              {query}
              isExpanded={layout === 'LIST_EXPANDED'}
              {lastResults}
              onopen={viewport.isLarge ? openBeside : undefined}
              currentId={openItemId}
              {onnavigate}
              mayShare={holds(myRole ?? undefined, 'MANAGE_MEMBERS').status === 'permitted'}
              changeRefusal={changeRefusalOf}
              mayCreate={mayCreateHere}
              isSelectable
              structure={container.structure ?? undefined}
              {mayShape}
              containerVersion={container.version}
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

{#if container?.type === 'COLLECTION' && container.parent_id && mayShape}
  <ImportDialog
    bind:isOpen={isImporting}
    hubId={container.parent_id}
    collection={{ id: container.id, name: container.name }}
    {onnavigate}
  />
{/if}

{#if container && mayShape}
  <ContainerEditDialog bind:isOpen={isEditing} {container} />
{/if}

{#if container?.structure && mayShape}
  {#snippet labelsTab()}
    <LabelsDialog isEmbedded isOpen={isShaping} collectionId={container.id} />
  {/snippet}
  {#snippet fieldsTab()}
    <CustomFieldsDialog isEmbedded isOpen={isShaping} collectionId={container.id} role={structureRole} />
  {/snippet}
  {#snippet templatesTab()}
    <TemplatesDialog
      isEmbedded
      isOpen={isShaping}
      collectionId={container.id}
      path={containerPath}
      role={structureRole}
      onopened={(itemId) => {
        isShaping = false;
        onnavigate(`/items/${itemId}`);
      }}
    />
  {/snippet}
  {#snippet automationTab()}
    <PoliciesDialog isEmbedded isOpen={isShaping} {container} path={containerPath} role={structureRole} />
  {/snippet}
  {#snippet peopleTab()}
    <MembersDialog isEmbedded isOpen={isShaping} title={t('app.people.title')} scope={memberScope} path={memberPath} />
  {/snippet}
  <StructurePanel
    bind:isOpen={isShaping}
    bind:tab={shapingTab}
    {container}
    {mayShapeWorkspace}
    tabs={container.type === 'COLLECTION'
      ? [
          { id: 'labels', label: t('app.structure.tab_labels'), body: labelsTab },
          { id: 'fields', label: t('app.structure.tab_fields'), body: fieldsTab },
          { id: 'templates', label: t('app.structure.tab_templates'), body: templatesTab },
          { id: 'automation', label: t('app.structure.tab_automation'), body: automationTab },
          { id: 'people', label: t('app.structure.tab_people'), body: peopleTab },
        ]
      : [{ id: 'people', label: t('app.structure.tab_people'), body: peopleTab }]}
  />
{/if}

{#if container?.type === 'COLLECTION'}
  <ViewsPanel
    bind:isOpen={isManagingViews}
    intent={viewsIntent}
    collectionId={container.id}
    hubId={container.parent_id ?? undefined}
    mayShapeAt={(scope) => {
      const at = scope === 'TENANT' ? {} : scope === 'HUB' ? { hubId: container.parent_id ?? undefined } : containerPath;
      const held = people.roleOf(at, actor.account?.id, roleOrder());
      return held === undefined ? undefined : holds(held ?? undefined, 'STRUCTURE').status === 'permitted';
    }}
    {query}
    {layout}
    role={structureRole}
    onapply={applyView}
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
  <ExportDialog
    filtered={isExportingFiltered ? { collectionId: container.id, name: container.name, query } : undefined}
    onclose={() => (isExportingFiltered = false)}
  />
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
  <MembersDialog bind:isOpen={isManagingMembers} title={t('app.people.title')} scope={memberScope} path={memberPath} />
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

{#if isChangingPrivacy && container}
  <!-- Making a hub private takes it away from people who read it today, so the confirmation names
       them; sharing it again takes nothing from anybody and says who gets it back. -->
  <Dialog
    bind:isOpen={isChangingPrivacy}
    title={container.private
      ? t('app.workspace.shared_confirm_title', { name: container.name })
      : t('app.workspace.private_confirm_title', { name: container.name })}
    dismissLabel={t('app.workspace.cancel')}
  >
    {#if container.private}
      <p class="notice">{t('app.workspace.shared_confirm_body')}</p>
    {:else}
      <p class="notice">{t('app.workspace.private_confirm_body')}</p>
      <p class="notice">
        {losers.length > 0
          ? t('app.workspace.private_confirm_who', {
              names: losers.map((each) => accounts.nameOf(each) ?? t('app.people.unnamed')).join(', '),
            })
          : t('app.workspace.private_confirm_anyone')}
      </p>
    {/if}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isChangingPrivacy = false)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button
        isBusy={isChangingPrivacyNow}
        busyLabel={container.private ? t('app.workspace.sharing') : t('app.workspace.making_private')}
        onclick={changePrivacy}
      >
        {container.private ? t('app.workspace.make_shared') : t('app.workspace.make_private')}
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
    box-shadow: var(--shadow-raised);
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
  /* Private is a fact about the hub, not a warning: said quietly, with the lock the tree shows. */
  .notice.private { display: flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

</style>
