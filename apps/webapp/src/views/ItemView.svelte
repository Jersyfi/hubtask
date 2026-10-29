<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry, at the address the board and the search results already linked to: its head, the
  // whole subtree under it, its details beside its text, and its history (ADR-0061 decision 4).
  //
  // The head is the entry itself - the completion checkbox, the title and the notes edited in
  // place, the set values as chips. In place means an input that looks like text until it has
  // focus, the same `PATCH`, the same announcement and the same conflict path. **There is one
  // way to edit and it is where the field is shown** (ADR-0063 decision 9): the head's three
  // fields in place, every other field a details row that opens its own editor, and no "edit"
  // form writing the same three a second time. The subtree is `EntryList` with a root
  // (design-system.md §11.8); comments and activity are tabs. Two columns from `expanded`, one below.
  //
  // **The history is the point of this screen**, and the rule that shapes it is
  // `domain-model.md` §3.5: the server stores `item.completed` and sends
  // `activity.item_completed`, and the client renders it. Nothing here writes a verb; the
  // catalogue does, and a code this client has never heard of still reads as words because
  // `messages.t` humanises an unknown code rather than printing a key — which is the normal state
  // of a client one release behind its server rather than an error.
  //
  // The **actor** is the one place this screen refuses to guess. The contract says of an activity
  // actor that "the label is not here: the account is one request away" — and for anybody but the
  // signed-in account, the name is resolved through `GET /accounts/{accountId}` and cached by
  // `lib/data/accounts.svelte.ts`. The reader is still "You", and an actor whose name did not
  // resolve is named by its kind.

  import LazyView from '../lib/frame/LazyView.svelte';
  import { untrack } from 'svelte';

  import {
    ActivityFeed,
    Badge,
    Button,
    Checkbox,
    EmptyState,
    ErrorState,
    Icon,
    IconButton,
    LoadMore,
    Menu,
    PageHeader,
    provideReasonGroup,
    ReadOnlyNotice,
    Select,
    Skeleton,
    Stack,
    Tabs,
    ViewSwitcher,
    type ActivityStep,
    type MenuItem,
  } from '@hubtask/design-system/components';
  import type { ActivityEntry, ActivityPage, CommentPage, MediaPage, ResourceState, Suggestion, SuggestionPage, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import {
    accountsNamedBy,
    actorCodes,
    changesOf,
    mediaNamedBy,
    namesInstant,
    namesMedia,
    namesPeople,
  } from '../lib/data/activity.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { changeRefusalCode, declaresType, holds, mayDo, offerIn, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { archivalOf } from '../lib/data/containers.ts';
  import { customFields } from '../lib/data/customfields.svelte.ts';
  import { definitionsFor, valueText } from '../lib/data/customfields.ts';
  import { hasForm, panelRows } from '../lib/data/entryforms.ts';
  import { entryTypes, offeredTypes, typeOf, typesShown } from '../lib/data/entrytypes.svelte.ts';
  import type { EntryFormState } from '@hubtask/sync-engine';
  import { formatDecimal } from '../lib/i18n/number.ts';
  import { formatDuration } from '../lib/i18n/duration.ts';
  import { commentsPath } from '../lib/data/comments.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { entryEditOf } from '../lib/data/edits.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { labels } from '../lib/data/labels.svelte.ts';
  import { attachmentsPath, media } from '../lib/data/media.svelte.ts';
  import { coverImageIdOf } from '../lib/data/media.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { reminders, series } from '../lib/data/reminders.svelte.ts';
  import { celebration } from '../lib/celebration.svelte.ts';
  import CelebrationSlot from '../lib/entries/CelebrationSlot.svelte';
  import EntryKudos from '../lib/entries/EntryKudos.svelte';
  import { kudosOffer } from '../lib/data/kudos.ts';
  import CustomFieldPanel from '../lib/entries/CustomFieldPanel.svelte';
  import DetailRow from '../lib/entries/DetailRow.svelte';
  import AmountPanel from '../lib/entries/AmountPanel.svelte';
  import EntryList from '../lib/entries/EntryList.svelte';
  import LabelsPanel from '../lib/entries/LabelsPanel.svelte';
  import LinksSection from '../lib/entries/LinksSection.svelte';
  import StepControl from '../lib/entries/StepControl.svelte';
  import MentionDialog from '../lib/entries/MentionDialog.svelte';
  import { inserted } from '../lib/data/links.ts';
  import ReplicaMark from '../lib/frame/ReplicaMark.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { recents } from '../lib/recents.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { device } from '../lib/device.svelte.ts';
  import DuePanel from '../lib/entries/DuePanel.svelte';
  import LanguagePicker from '../lib/entries/LanguagePicker.svelte';
  import ConflictStrip from '../lib/entries/ConflictStrip.svelte';
  import AiAsk from '../lib/entries/AiAsk.svelte';
  import AiPlace from '../lib/entries/AiPlace.svelte';
  import TranslatePanel from '../lib/entries/TranslatePanel.svelte';
  import { askOf, placeOf, type Ask, type Place } from '../lib/data/aiplaces.ts';
  import { buckets } from '../lib/data/buckets.svelte.ts';
  import { entryAsks } from '../lib/data/entryasks.svelte.ts';
  import { hasChoices, suggestionsPath } from '../lib/data/suggestions.ts';
  import AttachmentPanel from '../lib/media/AttachmentPanel.svelte';
  import AssigneePanel from '../lib/people/AssigneePanel.svelte';
  import CommentPanel from '../lib/people/CommentPanel.svelte';
  import EntryActions from '../lib/entries/EntryActions.svelte';
  import { entryMenu as menuOfEntry } from '../lib/entries/entryMenu.ts';
  import { archivalOfItem } from '../lib/data/lifecycle.ts';
  import { activityPath, itemPath } from '../lib/data/item.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { actor as signedIn } from '../lib/data/account.svelte.ts';
  import { formatDateTime, formatDue } from '../lib/i18n/datetime.ts';
  import { textLanguages } from '../lib/data/query.ts';
  import { ticketKeyOf } from '../lib/data/ticket.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { actionsOfHead } from '../lib/palette/actions.ts';
  import { commands } from '../lib/palette/commands.svelte.ts';

  // The editors an entry opens on demand are chunks of their own (F-10): an entry's first paint
  // carries none of them.
  const entryParts = {
    RecurrencePanel: () => import('../lib/entries/RecurrencePanel.svelte'),
    ReminderPanel: () => import('../lib/entries/ReminderPanel.svelte'),
    CoverPanel: () => import('../lib/media/CoverPanel.svelte'),
  };

  interface Props {
    id: string;
    /** Where the entry's parents lead. The view does not own the router. */
    onnavigate?: (path: string) => void;
    /**
     * Inside the detail pane beside a collection (ADR-0061 decision 4): the pane's head names
     * the entry, so no page head, no trail and no title for the bar; one column; the entry's
     * menu stands on its own. Everything else is the page's, unchanged.
     */
    isInPane?: boolean;
  }

  const { id, onnavigate, isInPane = false }: Props = $props();

  /**
   * Whether the details' fold is open: open from `medium` up, folded on a phone - and opened by a
   * proposal standing at a detail. Bound rather than recomputed, so the person's own press holds
   * while anything else on the page changes.
   */
  let isDetailsOpen = $state(!viewport.isCompact);
  $effect(() => {
    const isCompact = viewport.isCompact;
    untrack(() => (isDetailsOpen = !isCompact || hasDetailProposal));
  });
  $effect(() => {
    if (hasDetailProposal) isDetailsOpen = true;
  });

  /** Whether the details show the fields with nothing set too; folded by default. */
  let isFieldsOpen = $state(false);
  let detailRowsElement = $state<HTMLElement | null>(null);
  /** How many detail rows hold nothing, and how many hold something: read off the rows drawn. */
  let emptyFieldCount = $state(0);
  let setFieldCount = $state(0);
  $effect(() => {
    void item;
    const rows = detailRowsElement;
    if (!rows) return;
    const count = () => {
      emptyFieldCount = rows.querySelectorAll(':scope > .detail[data-empty]').length;
      setFieldCount = rows.querySelectorAll(':scope > .detail:not([data-empty])').length;
    };
    count();
    const observer = new MutationObserver(count);
    observer.observe(rows, { childList: true, subtree: true, attributes: true, attributeFilter: ['data-empty'] });
    return () => observer.disconnect();
  });


  // Read once, and `untrack` says the once is deliberate: `App.svelte` keys this view on the id,
  // so a different entry is a different component rather than the same one asking again. A
  // resource that followed the prop would be a second answer to which entry this screen is.
  const entry = resource<WorkItem>(untrack(() => itemPath(id)));
  const history = resource<ActivityPage>(untrack(() => activityPath(id)));

  const item = $derived(entry.state.status === 'ready' ? entry.state.data : undefined);


  // The bar carries the title on a phone (ADR-0061 decision 2); on every width the head's `h1`
  // is read rather than drawn, because the title the reader sees is the field they edit it in.
  $effect(() => (isInPane ? undefined : page.entitle(item?.title)));
  $effect(() => (isInPane ? undefined : page.locate(item?.collection_id)));

  // What the overview's "what you had open" is built from. Noted when the entry has a title rather
  // than when the route resolved: a row naming an entry nothing could name yet would be a blank.
  // Not in the pane, where the reader is on the list beside it rather than on this.
  $effect(() => {
    if (isInPane || !item?.title) return;
    recents.note({ kind: 'item', id: item.id, title: item.title, placeId: item.collection_id });
  });

  /**
   * The entries above this one, nearest first, for the breadcrumb through the levels: a work
   * package's trail is hub › collection › task › work package, and from the bottom level the
   * trail is the way up. Each parent is one read, and the chain follows `parent_id` until it
   * meets the collection - three levels at most in this schema, and one more per level arc42
   * Q-03 adds, with no change here.
   */
  let ancestors = $state<WorkItem[]>([]);
  $effect(() => {
    const start = item?.parent_id ?? undefined;
    if (!start) {
      ancestors = [];
      return;
    }
    const stops: (() => void)[] = [];
    const chain: WorkItem[] = [];
    const followed = new Set<string>();
    const follow = (parentId: string) => {
      if (followed.has(parentId)) return;
      followed.add(parentId);
      stops.push(
        engine.subscribe<WorkItem>({ path: itemPath(parentId) }, (next) => {
          if (next.status !== 'ready') return;
          const at = chain.findIndex((each) => each.id === parentId);
          if (at >= 0) chain[at] = next.data;
          else chain.push(next.data);
          ancestors = [...chain];
          if (next.data.parent_id) follow(next.data.parent_id);
        }),
      );
    };
    untrack(() => follow(start));
    return () => {
      for (const stop of stops) stop();
    };
  });

  // The collection and its hub, read for the trail the way `ContainerView` reads its own: a deep
  // link to an entry may be the first thing this client asks for, and the levels are then not
  // loaded. `untrack` for the reason `WorkspaceNav` records.
  $effect(() => {
    const wanted = item?.collection_id;
    if (!wanted) return;
    return untrack(() => containers.openSingle(wanted));
  });
  // The hub's id as a derived string, so the effect below depends on the value and not on the
  // store: the read it starts writes the store, and an effect tracking that store would start
  // the read again on its own answer.
  const hubIdOfTrail = $derived(item ? containers.find(item.collection_id)?.parent_id ?? undefined : undefined);
  $effect(() => {
    const wanted = hubIdOfTrail;
    if (!wanted) return;
    return untrack(() => containers.openSingle(wanted));
  });
  const collection = $derived(item ? containers.find(item.collection_id) : undefined);
  /**
   * What the collection offers this entry (UC-WRK-23 check 4): its structure in force narrows the
   * installation's answers - a field it does not offer has no row, a level it does not offer no
   * subtree. What it hides stays stored (domain-model.md §2).
   */
  const offer = $derived(offerIn(collection?.structure?.settings));
  /**
   * Whether the collection, or the hub above it, is archived: then nothing in it can be changed
   * (I-C3). The entry's own archive freezes the entry alone, never its children.
   */
  const isCollectionArchived = $derived(collection !== undefined && archivalOf(collection) !== 'active');
  const hub = $derived(collection?.parent_id ? containers.find(collection.parent_id) : undefined);
  /** The entry's ticket key, `EINKAUF-42` (ADR-0094): what a mail subject quotes to reach it. */
  const ticketKey = $derived(ticketKeyOf(collection?.handle, item?.number));

  async function copyTicketKey(key: string) {
    await navigator.clipboard.writeText(key);
    announcer.say(t('app.entries.ticket_key_copied', { key }));
  }
  /** Whether the collection's structure in force gives this entry an amount (UC-WRK-25 check 2). */
  const offersAmount = $derived(
    item ? (collection?.structure?.settings?.fields?.[item.type]?.capabilities ?? []).includes('AMOUNT') : false,
  );
  const trail = $derived([
    ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
    ...(collection ? [{ id: collection.id, label: collection.name, href: `/collections/${collection.id}` }] : []),
    ...[...ancestors].reverse().map((each) => ({ id: each.id, label: each.title, href: `/items/${each.id}` })),
    ...(item ? [{ id: item.id, label: item.title }] : []),
  ]);
  function goTo(crumbId: string) {
    const crumb = trail.find((each) => each.id === crumbId);
    if (crumb?.href) onnavigate?.(crumb.href);
  }

  // What the chips and the rows say: the labels of the collection, the reminders, the series,
  // the attachments and the comments, each read once here for the count and again by its panel
  // - the engine shares one entry per path, so the second is a listener and not a request.
  $effect(() => {
    const wanted = item?.collection_id;
    if (!wanted) return;
    return untrack(() => labels.open(wanted));
  });
  $effect(() => untrack(() => reminders.open(id)));
  // The series is asked for only when the row says there is one; the effect follows
  // `recurrence_rule_id` so that setting a series starts the read and removing it ends it.
  $effect(() => {
    const row = item ? { id: item.id, recurrence_rule_id: item.recurrence_rule_id } : undefined;
    if (!row) return;
    return untrack(() => series.open(row));
  });
  const attachments = resource<MediaPage>({ path: untrack(() => attachmentsPath(id)) });
  const thread = resource<CommentPage>({ path: untrack(() => commentsPath(id)) });
  const attachmentCount = $derived(attachments.state.status === 'ready' ? (attachments.state.data.data ?? []).length : undefined);
  const commentCount = $derived(thread.state.status === 'ready' ? (thread.state.data.data ?? []).length : undefined);
  const reminderCount = $derived(reminders.of(id).length);
  const rule = $derived(series.of(id));
  /** The board column the entry stands in, by its name: what it *is* beside its level. */
  /** The collection's columns, where it has any: what the column mark offers. */
  const columnsHere = $derived.by(() => {
    if (!item) return [];
    // Only where the collection is a board at all: a list's entries have no column to change.
    const offered = collection?.structure?.settings?.views?.offered as readonly string[] | undefined;
    if (offered && !offered.includes('KANBAN')) return [];
    return buckets.of(item.collection_id);
  });
  const columnItems = $derived<MenuItem[]>([
    ...columnsHere.map((column) => ({ id: column.id, label: column.name, icon: column.id === item?.bucket_id ? ('check' as const) : undefined })),
    { id: 'none', label: t('app.board.unbucketed'), icon: item?.bucket_id ? undefined : ('check' as const), hasSeparatorBefore: true },
  ]);

  /** Puts the entry in a column, as the board's move does - the done column completes it too. */
  async function setColumn(bucketId: string | null) {
    if (!item || (item.bucket_id ?? null) === bucketId) return;
    const target = columnsHere.find((column) => column.id === bucketId);
    try {
      let moved = await items.setBucket(item.id, bucketId, item.version);
      if (target?.is_done_bucket && !moved.completion?.is_completed) {
        moved = await items.setCompleted(moved.id, true, crypto.randomUUID());
        void celebration.celebrate(moved);
      }
      announcer.say(t('app.board.moved', { title: item.title, name: target?.name ?? t('app.board.unbucketed'), limit: '' }));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  const columnName = $derived(item?.bucket_id ? buckets.of(item.collection_id).find((each) => each.id === item?.bucket_id)?.name : undefined);
  const carriedLabels = $derived(
    item ? (item.label_ids ?? []).map((labelId) => labels.of(item.collection_id).find((each) => each.id === labelId)).filter((each) => each !== undefined) : [],
  );
  const definitions = $derived(item ? definitionsFor(customFields.of(item.collection_id), item.type as string) : []);

  // The entry's type's form, in edit mode (UC-FLW-01 check 2): its custom fields in the form's
  // sections and order, a hidden one left out, a read-only one opening nothing. The server answers
  // the conditions for the entry as it is stored, again whenever its version moves.
  $effect(() => {
    if (!typesShown(manifest.value)) return;
    return untrack(() => entryTypes.open());
  });
  const itemType = $derived(
    item && typesShown(manifest.value) ? typeOf(entryTypes.all, item.type as string, item.entry_type ?? undefined) : undefined,
  );
  const itemForm = $derived(itemType && hasForm(itemType.form) ? itemType.form : undefined);
  // The entry's type, changed in place (UC-FLW-01 check 4): the types of its level still offered,
  // the one it has kept even when archived. A row only where there is a choice.
  const typeChoices = $derived.by(() => {
    if (!item || !itemType) return [];
    const offered = offeredTypes(entryTypes.all, item.type as string);
    return offered.some((each) => each.key === itemType.key) ? offered : [itemType, ...offered];
  });
  const entryTypeName = (type: { name?: string | null; level: string }) => type.name ?? t('app.entry_types.level', { level: type.level });
  let typeDraft = $state('');
  $effect(() => {
    typeDraft = itemType?.key ?? '';
  });
  async function saveType() {
    if (!item || !itemType || typeDraft === '' || typeDraft === itemType.key) return;
    writeFailure = undefined;
    try {
      await items.setEntryType(item.id, typeDraft, item.version);
      announcer.say(t('app.entries.saved_announced'));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  let formAnswer = $state<EntryFormState | undefined>(undefined);
  $effect(() => {
    const key = itemForm && itemType ? itemType.key : undefined;
    const itemId = item?.id;
    void item?.version;
    if (!key || !itemId) {
      formAnswer = undefined;
      return;
    }
    let isCurrent = true;
    entryTypes.evaluate(key, { item_id: itemId }).then(
      (answer) => { if (isCurrent) formAnswer = answer; },
      () => { if (isCurrent) formAnswer = undefined; },
    );
    return () => { isCurrent = false; };
  });
  const fieldRows = $derived(
    panelRows(itemForm, formAnswer, definitions.map((definition) => definition.key)).flatMap((row) => {
      const definition = definitions.find((each) => each.key === row.key);
      return definition ? [{ ...row, definition }] : [];
    }),
  );

  /** The due date as the row says it - the same words `DueMark` draws. */
  const dueValue = $derived(
    item?.due_at
      ? formatDue(item.due_at, messages.locale, item.due_time_zone ?? signedIn.zone, { allDay: item.due_date_only ?? false, showZone: (item.due_time_zone ?? signedIn.zone) !== signedIn.zone })
      : undefined,
  );
  const startValue = $derived(item?.start_at ? formatDateTime(item.start_at, messages.locale) : undefined);
  /**
   * The two dates as one value, because they are one editor.
   *
   * `DuePanel` writes the start and the due together, so two rows opening it showed the same
   * panel twice and the reader met the other date wherever they pressed. One row,
   * whose value reads the start, the due, or the span between them.
   */
  const datesValue = $derived(
    startValue && dueValue
      ? t('app.due.span', { start: startValue, due: dueValue })
      : (dueValue ?? startValue),
  );
  /**
   * What this entry's **type** carries, from `/meta/capabilities` and from nowhere else
   * (`domain-model.md` §2). A row is drawn only where the answer is `permitted`: a field the
   * profile refuses is answered with 422 by the server, and offering it is offering a refusal.
   */
  const carries = $derived((capability: string) => (item ? offer.supports(item.type, capability).status === 'permitted' : false));
  /**
   * Whether `carries` is answering at all — and the reason this screen asks separately.
   *
   * `supports` has three answers and `carries` collapses them into two. `refused` and
   * `undetermined` both come back as `false`, and for `refused` that is right: the profile says
   * no and a control nobody can use is a control nobody is offered. For `undetermined` it is a
   * lie the screen told silently. Until the manifest is read **nothing about any type is known**,
   * so every row went missing at once and the entry read as a title with a language under it —
   * with no error, no hint and no pending state to say why. The manifest is read at
   * boot, so this is a moment on an ordinary load and the whole screen while the read failed.
   */
  const isTypeKnown = $derived(manifest.isRead);
  /**
   * Why it is not known, where the read failed rather than merely not landed yet.
   *
   * The difference decides what is drawn: a skeleton that never resolves is the same silence with
   * a pulse on it. The retry is not here — it is the one mark in the bar, which is on every
   * screen (ADR-0063 decision 5), and a second one on this page would be the second answer to
   * "where do I ask again".
   */
  const typeFailure = $derived(manifest.failure ? renderProblem(manifest.failure, messages) : undefined);
  /**
   * Whether the installation declares this entry's type at all.
   *
   * The other way the column empties, and the one that survives a reload: a manifest that answers
   * and declares no profile for `TASK` refuses all fourteen capabilities, one by one, and the
   * screen drew fourteen nothings. The refusal has one cause and the server has one sentence for
   * it — `items.type_unsupported`, which this client uses rather than inventing a second.
   */
  const isTypeOffered = $derived(item ? declaresType(item.type) !== false : true);
  const repeatValue = $derived.by(() => {
    if (!rule) return undefined;
    const frequency = /FREQ=([A-Z]+)/.exec(rule.rrule ?? '')?.[1];
    return frequency ? t(`app.recurrence.freq_${frequency}`) : t('app.recurrence.title');
  });

  /** Completing the entry from its own page - the one addition of two (ADR-0061). */
  let completionFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  async function toggleCompleted() {
    if (!item) return;
    completionFailure = undefined;
    const wasDone = item.completion?.is_completed ?? false;
    try {
      const answered = await items.setCompleted(item.id, !wasDone, crypto.randomUUID());
      announcer.say(t(wasDone ? 'app.entries.reopened_announced' : 'app.entries.completed_announced', { title: item.title }));
      if (answered.completion?.is_completed) void celebration.celebrate(answered);
    } catch (error) {
      completionFailure = renderProblem(error as never, messages);
    }
  }
  const moment = $derived(celebration.current);
  $effect(() => () => celebration.dismiss());

  /**
   * The subtree's head: the child type as the manifest names it where the entry holds one kind, and
   * "Under this entry" where it holds several - a task with work packages and activities side by
   * side is headed by neither name alone. Beside it, how much of it is done, counting every kind.
   */
  const heldTypes = $derived(item ? offer.childTypes(item.type) : []);
  /** A level as a name standing on its own - a heading, a badge - by the name the collection gave it. */
  const typeName = (type: string) => offer.level(type).label;
  const subtreeHeading = $derived(heldTypes.length === 1 ? typeName(heldTypes[0] ?? '') : t('app.item.children'));
  let subtree = $state<EntryList | undefined>(undefined);
  let isSubtreeOpen = $state(true);

  /**
   * The entry's own menu in its head: the one every place that shows an entry draws (UC-WRK-28
   * check 2), without *Open*. *Share* where the reader may grant a role here; otherwise *People*,
   * the same dialog, to read who has access - the offer to grant is absent rather than refused
   * inside it (UC-WRK-21 check 7). What it does is `EntryActions`', as on every surface.
   */
  const entryMenu = $derived.by((): MenuItem[] =>
    item
      ? menuOfEntry(
          {
            surface: 'page',
            item: { title: item.title, isCompleted: item.completion?.is_completed ?? false, hasDue: Boolean(item.due_at) },
            archival: archivalOfItem(item, isCollectionArchived),
            isCollectionReadOnly: isCollectionArchived,
            mayChange: mayChangeHere,
            mayCreate: mayCreateHere,
            mayShare: mayShare(),
            readsPeople: true,
            carries,
            level: actions?.levelOf(item),
          },
          t,
        )
      : [],
  );
  let actions = $state<EntryActions | undefined>(undefined);
  // The moves need the entry's level, which a page does not otherwise hold. It is read when the
  // reader reaches for the menu - its trigger in the head, the pane or the bar - as search reads a
  // hit's (D6), not with the page: held open, every write here would read the level again.
  $effect(() => {
    const current = item;
    const held = actions;
    if (!current || !held) return;
    const reach = (event: Event) => {
      if (event.target instanceof Element && event.target.closest('[data-opener="entry-menu"]')) held.readLevel(current);
    };
    document.addEventListener('pointerdown', reach, true);
    document.addEventListener('focusin', reach, true);
    return () => {
      document.removeEventListener('pointerdown', reach, true);
      document.removeEventListener('focusin', reach, true);
    };
  });
  /**
   * Whether the reader may grant a role on this entry: the server's own answer of what they may
   * give there (`grantable_roles`), the one the members dialog offers from. Unknown reads as no.
   */
  function mayShare(): boolean {
    if (!item) return false;
    if (mayShareHere) return true;
    return (people.grantableAt({ scopeType: 'ITEM', scopeId: item.id })?.length ?? 0) > 0;
  }

  let activeTab = $state('comments');
  /** The tab that is actually shown: the history, where this type has no conversation to open. */
  const shownTab = $derived(activeTab === 'comments' && !carries('COMMENTS') ? 'activity' : activeTab);

  function chooseFromMenu(chosen: string) {
    if (item) void actions?.run(item, chosen);
  }

  // What the palette may list for this entry: the head's menu, as the page's own or, in the detail
  // pane, beside the collection's (UC-WRK-27 check 2).
  $effect(() => commands.offer(isInPane ? 'pane' : 'page', actionsOfHead(isInPane ? 'pane' : 'page', { menu: { items: entryMenu, onselect: chooseFromMenu } })));

  /** The language row's draft, written when the reader says so - a picker that wrote on every keystroke of a tag would send "d", "de". */
  let languageDraft = $state('');
  $effect(() => {
    languageDraft = item?.content_language ?? '';
  });
  async function saveLanguage() {
    if (!item) return;
    const body = entryEditOf(
      { title: item.title, notes: item.notes ?? '', language: item.content_language ?? '' },
      { title: item.title, notes: item.notes ?? '', language: languageDraft },
    );
    if (Object.keys(body).length === 0) return;
    writeFailure = undefined;
    try {
      await items.update(item.id, body, item.version);
      announcer.say(t('app.entries.saved_announced'));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * The path this entry sits on, which is what the memberships are composed along.
   *
   * The collection is on the entry; the hub is the collection's parent and comes from the
   * container tree the frame already holds. A hub that has not arrived yet simply contributes no
   * scope — the picker grows when it does, rather than blocking on a second request.
   */
  const peoplePath = $derived({
    hubId: item ? containers.find(item.collection_id)?.parent_id ?? undefined : undefined,
    collectionId: item?.collection_id,
    itemId: item?.id,
  });

  /**
   * What the reader's role lets them do to this entry (UC-WRK-22 checks 1-3): their effective role
   * along its path, the role the server decides the entry by, and the matrix's per-entry cell -
   * `ASSIGNED` against whether it is theirs. Each `undefined` while the role is still being read;
   * then nothing that writes is drawn, and no reason either.
   */
  const role = $derived(item ? people.roleOf(peoplePath, actor.account?.id, roleOrder()) : undefined);
  const isMine = $derived(item?.assignee_id != null && item.assignee_id === actor.account?.id);
  const mayChangeHere = $derived(mayDo(role, 'change', isMine));
  const mayCreateHere = $derived(mayDo(role, 'create', isMine) === true);
  const mayCommentHere = $derived(mayDo(role, 'comment', isMine));
  /** Whether the reader may share an entry of the subtree: granting a role asks `MANAGE_MEMBERS` (D5). */
  const mayShareHere = $derived(holds(role ?? undefined, 'MANAGE_MEMBERS').status === 'permitted');
  /**
   * The one line that says why nothing here can be changed (UC-WRK-22 check 2): the role reads and
   * does not change, or changes only what is assigned to the reader. Not while the role is unknown.
   */
  const roleReason = $derived.by(() => {
    if (mayChangeHere !== false) return undefined;
    const code = changeRefusalCode(role, isMine);
    return code ? t(code) : undefined;
  });
  /** Whether the controls that write this entry are drawn: only where the role is known to allow it. */
  const isChangeable = $derived(mayChangeHere === true);
  /** What a row of the subtree may not have changed, by the same rule, for each row's own assignee. */
  function changeRefusalOf(row: WorkItem): string | undefined {
    const code = changeRefusalCode(role, row.assignee_id != null && row.assignee_id === actor.account?.id);
    return code ? t(code) : undefined;
  }

  // The scopes are opened once the entry has told us where it sits. `people.open` is idempotent
  // per scope, so a re-render adds nothing.
  $effect(() => {
    if (item) people.open(peoplePath);
  });

  // The custom fields in force where this entry sits. Read here rather than passed down, because
  // the entry is what says which collection that is, and the collection is only known once it has
  // arrived.
  $effect(() => {
    const wanted = item?.collection_id;
    if (!wanted) return;
    return untrack(() => customFields.open(wanted));
  });
  const failure = $derived(
    entry.state.status === 'failed' ? renderProblem(entry.state.error, messages) : undefined,
  );

  /** What the history kept about one field, as one phrase. */
  /**
   * An identifier that names a person, as their name.
   *
   * A UUID in front of somebody asking who took the entry over is no answer, and the name is one
   * request away — the same one every actor's name comes through. Until it lands, the sentence
   * that is true of anybody stands in.
   */
  function personOf(value: string | undefined, field: string): string | undefined {
    if (value === undefined) return undefined;
    if (!namesPeople(field)) return value;
    return accounts.nameOf(value) ?? t('app.people.unnamed');
  }

  /**
   * An identifier that names a file, as its name.
   *
   * The same courtesy `personOf` extends: `item.attachment_added` carries a media identifier, and
   * a UUID answers nobody asking which file was attached. A record that was refused or is already
   * gone — a detached object the reconciliation took — falls back to the sentence true of any
   * unnamed file.
   */
  function fileOf(value: string | undefined, field: string): string | undefined {
    if (value === undefined) return undefined;
    if (!namesMedia(field)) return value;
    return media.fileNameOf(value) ?? t('app.media.unnamed');
  }

  /**
   * An instant, drawn as the date it is.
   *
   * A due date carries its own zone, and the same change set carries it — so a move that crossed
   * zones reads in the zone it was set in rather than in the reader's, which is the whole point of
   * storing the three fields together. Everything else is a moment and is read on the reader's own
   * clock.
   */
  function instantOf(value: string | undefined, field: string, zone: string | undefined): string | undefined {
    if (value === undefined) return undefined;
    if (!namesInstant(field)) return value;
    return field === 'due_at'
      ? formatDue(value, messages.locale, zone ?? signedIn.zone, { showZone: true })
      : formatDateTime(value, messages.locale);
  }

  function detailOf(
    change: ReturnType<typeof changesOf>[number],
    zone: string | undefined = undefined,
  ): string {
    // A field whose values the history does not keep says so and nothing else. A note is the
    // worked example, and looking for its text would be looking for what ADR-0017 kept out.
    if (change.isOpaque) return t('app.activity.changed');
    const from = instantOf(fileOf(personOf(change.from, change.field), change.field), change.field, zone);
    const to = instantOf(fileOf(personOf(change.to, change.field), change.field), change.field, zone);
    // Handing an entry from one person to another is one step with both sides, which is what the
    // model says a hand-over is (domain-model.md §3.5) rather than an unassignment and an
    // assignment that happen to be adjacent.
    if (from !== undefined && to !== undefined) {
      return t('app.activity.from_to', { from, to });
    }
    if (to !== undefined) return t('app.activity.set_to', { to });
    if (from !== undefined) return t('app.activity.cleared_from', { from });
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
      changes: changesOf(step.change_set as Record<string, unknown>).map((change, _, all) => ({
        field: change.field,
        // The zone the step itself recorded, where it recorded one. A due date that moved from
        // Berlin to São Paulo says so on both sides, because both sides are in this one step.
        detail: detailOf(change, all.find((each) => each.field === 'due_time_zone')?.to),
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

    // …and the people the change sets name, which is a different list: the person who took an
    // entry over is not the person who recorded that they did.
    accounts.resolve(
      (history.state.data.data ?? []).flatMap((step) =>
        accountsNamedBy(changesOf(step.change_set as Record<string, unknown>)),
      ),
    );

    // …and the files they name, which is the same courtesy for a different kind of identifier.
    media.resolve(
      (history.state.data.data ?? []).flatMap((step) =>
        mediaNamedBy(changesOf(step.change_set as Record<string, unknown>)),
      ),
    );
  });
  const hasMore = $derived(
    history.state.status === 'ready' && (history.state.data.page?.has_more ?? false),
  );
  /**
   * What the last press of *Show more history* brought, for `LoadMore`'s live region. Empty until
   * the reader asks: the first page arrives with the tab, and a region filled at mount announces a
   * count nobody asked for.
   */
  let historyArrived = $state<string | undefined>(undefined);

  /** Why the last press of *Show more history* brought nothing; cleared by the next press. */
  let historyMoreFailed = $state<string | undefined>(undefined);

  async function moreHistory(): Promise<void> {
    historyMoreFailed = undefined;
    try {
      await history.loadMore();
      historyArrived = t('app.activity.arrived', { count: steps.length });
    } catch (error) {
      // The pages already shown stay (the engine keeps them); what is missing is said beside the
      // button that asked for it.
      historyMoreFailed = renderProblem(error as never, messages).message;
    }
  }
  const historyFailure = $derived(
    history.state.status === 'failed' ? renderProblem(history.state.error, messages) : undefined,
  );

  // Editing the title and the notes. `items.update` carries both, with the `If-Match` and the
  // version conflict handled - without this caller the text that `POST /search` searches and that
  // the history says somebody changed could not be written here
  // at all. The dogfooding pass set a note with curl in order to search for a word in it.
  //
  // It was built twice: in place here, and as a form behind "edit" in the menu, both writing the
  // same three fields. The form is gone (ADR-0063 decision 9). What it was for - a refusal needing
  // somewhere to land - is answered without it: the sentence is drawn at the field it is about,
  // the title's under the title and everything else under the notes.
  const languages = $derived(textLanguages(manifest.value));

  // The language the entry is written in, where it differs from the page's, so that a screen
  // reader switches voice on the title and the notes (design-system.md §10, 3.1.2). The same
  // language as the document says nothing - `lang` there would be noise.
  const entryLang = $derived(
    item?.content_language && item.content_language !== messages.locale ? item.content_language : undefined,
  );
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isTitleFailure = $state(false);

  // Why nothing on this entry can be changed because of an archive, or nothing: the collection's
  // first - unarchiving the entry would be refused while the collection is archived (I-C3) - then
  // the entry's own, worded for whether the reader may unarchive it or must ask - so not before the
  // role is known, as the role's own sentence (P-11); nothing is writable meanwhile, since writing
  // needs the role. The entry's archive is its own: its children are not read-only with it.
  const frozenReason = $derived(
    isCollectionArchived
      ? t('app.entries.read_only')
      : item?.archived_at && mayChangeHere !== undefined
        ? t(isChangeable ? 'app.entries.archived' : 'app.entries.archived_ask')
        : undefined,
  );
  /**
   * The page's one notice (design-system.md §4, case 5): the role's sentence where the role reads
   * only - it outlasts any unarchive - else the archive's. The controls that stay, the completion
   * box, name it as their description rather than repeating it.
   */
  const readOnlyReason = $derived(roleReason ?? frozenReason);
  /** Whether anything on this entry may be written: the role allows it and nothing is archived. */
  const isWritable = $derived(isChangeable && frozenReason === undefined);
  const instanceId = $props.id();
  const readOnlyNoticeId = `item-${instanceId}-read-only`;
  provideReasonGroup(() => ({ id: readOnlyNoticeId, reason: readOnlyReason }));

  async function unarchive() {
    if (!item) return;
    try {
      await items.setArchived(item.id, false, crypto.randomUUID());
      announcer.say(t('app.entries.unarchived_announced', { title: item.title }));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  // AI where its subject is (UC-AI-04 check 6): a ✦ beside each subject and what stands about it
  // under it - and nothing at all when the manifest announces no AI for this workspace, not
  // disabled with a reason, not rendered empty (ai-first.md §2, absence is absence).
  const hasAi = $derived(manifest.value?.features?.ai_suggestions === true);

  // What stands about this entry, read once - the view is keyed on the id. A reader reads it too:
  // a summary of the discussion is a reading aid (check 8).
  // Read only where the manifest announces AI: an installation without it reads nothing of it.
  let listingState = $state<ResourceState<SuggestionPage>>({ status: 'idle' });
  $effect(() => {
    if (!hasAi) return;
    return untrack(() => engine.subscribe<SuggestionPage>({ path: suggestionsPath(id) }, (next) => (listingState = next)));
  });
  const listing = { get state() { return listingState; } };
  /** Proposals a reader closed on this page: hidden here, unchanged for everybody else. */
  let closed = $state<ReadonlySet<string>>(new Set());
  const standing = $derived(
    hasAi && listing.state.status === 'ready'
      ? (listing.state.data.items ?? []).filter((suggestion) => suggestion.status === 'PROPOSED' && !closed.has(suggestion.id))
      : [],
  );
  const proposalsAt = (place: Place): readonly Suggestion[] => standing.filter((suggestion) => placeOf(suggestion) === place);
  /** The questions whose answer already stands at a place, so its ✦ shows it rather than asking again. */
  const standingAt = (place: Place): Readonly<Partial<Record<Ask, string>>> => {
    const found: Partial<Record<Ask, string>> = {};
    for (const suggestion of proposalsAt(place)) {
      const ask = askOf(suggestion);
      if (ask && !found[ask]) found[ask] = suggestion.id;
    }
    return found;
  };
  const closeProposal = (suggestionId: string) => (closed = new Set([...closed, suggestionId]));
  /**
   * Whether the details card holds at least one row (P-05, P-11): a value that is set, or a row the
   * reader may fill in. A read-only reader on an entry with nothing set would otherwise meet an
   * empty frame that reads as something that failed to load. Each line mirrors when `detailRows`
   * draws its row - `DetailRow` draws a read-only row only with a value. While the type is unknown
   * or not offered the card stays: it says why there are no rows.
   */
  /** A field's value as its row shows it, the reader's way (`valueText`). */
  const fieldReading = $derived({
    number: (value: number) => formatDecimal(value, messages.locale),
    amount: (value: number, currency: string) =>
      t('app.fields.money_value', { amount: formatDecimal(value, messages.locale), currency }),
    duration: formatDuration,
    nameOf: (id: string) => accounts.nameOf(id),
    titleOf: (id: string) => items.find(id)?.title,
    unknown: t('app.fields.value_unnamed'),
  });

  const holdsDetails = $derived.by(() => {
    if (!item) return false;
    if (!(isTypeKnown && isTypeOffered)) return true;
    if (proposalsAt('due').length + proposalsAt('labels').length > 0) return true;
    const shown = (isSet: boolean, isReadOnly: boolean) => isSet || !isReadOnly;
    const held = item.custom_fields as Record<string, unknown> | undefined;
    return (
      ((carries('ASSIGNMENT') || carries('MEMBERS')) && shown(Boolean(item.assignee_id), !isWritable)) ||
      (carries('DUE_DATE') && shown(datesValue !== undefined, !isWritable)) ||
      (offersAmount && shown(item.amount != null, !isChangeable)) ||
      (carries('LABELS') && shown(carriedLabels.length > 0, !isWritable)) ||
      (carries('REMINDER') && shown(reminderCount > 0, !isWritable)) ||
      (carries('RECURRENCE') && shown(repeatValue !== undefined, !isWritable)) ||
      shown(Boolean(item.content_language), !isWritable) ||
      (carries('COVER') && shown(Boolean(item.cover), !isWritable)) ||
      (carries('ATTACHMENTS') && (isWritable || Boolean(attachmentCount))) ||
      (carries('CUSTOM_FIELDS') &&
        definitions.some((definition) => {
          const value = held?.[definition.key];
          return shown(value !== undefined && value !== null && value !== '', !isWritable);
        }))
    );
  });
  /** The translation, open under the head once *Read in another language* was chosen. */
  let isTranslating = $state(false);
  $effect(() => () => untrack(() => entryAsks.forget(id)));
  $effect(() => {
    if (item?.collection_id) untrack(() => buckets.open(item.collection_id));
  });
  // What a classification could choose from: the same three lists the server reads before it
  // queues one, so the ✦ offers it exactly where it can succeed (P-05).
  const aiChoices = $derived(
    item
      ? hasChoices(item, labels.stateOf(item.collection_id), buckets.stateOf(item.collection_id), customFields.stateOf(item.collection_id))
      : 'reading',
  );
  const hasComments = $derived((commentCount ?? 0) > 0);

  /** A proposal waits at a detail: the fields are never folded over it. */
  const hasDetailProposal = $derived(item ? proposalsAt('due').length + proposalsAt('labels').length > 0 : false);

  // What sits under this entry, for the kinds that hold anything: an accepted breakdown
  // creates children, and a screen that showed the proposal but not what it made would leave
  // the reader to find them in the collection. The subtree is the one read the section below
  // makes, and this counts its first level; the acceptance's invalidation of the
  // lists brings them here without being asked.
  const takesChildren = $derived(heldTypes.length > 0);
  $effect(() => {
    if (!takesChildren) return;
    return untrack(() => items.openSubtree(id));
  });
  const children = $derived(takesChildren ? items.childrenOf(id) : []);
  /**
   * The title and the notes edited in place: drawn as text until they have focus, saved when the
   * reader leaves them or presses Enter in the title, restored by Escape. Only what moved is
   * written (offline-sync.md §4.2) - a field repeated unchanged would be queued with a
   * fresh clock and win a merge it never entered.
   *
   * Both the entry and its history come back on their own: the write invalidates `/items`, and the
   * engine matches by prefix - so `/items/{id}` and `/items/{id}/activity` are re-read without
   * either being asked for here. A refresh would be a second read of what is arriving.
   */
  let inlineTitle = $state('');
  let inlineNotes = $state('');
  let isInlineDirty = $state(false);
  $effect(() => {
    if (!item || isInlineDirty) return;
    inlineTitle = item.title;
    inlineNotes = item.notes ?? '';
  });

  async function commitInline() {
    if (!item || !isInlineDirty) return;
    if (inlineTitle.trim() === '') {
      writeFailure = renderProblem({ status: 422, code: 'items.title_empty', detailCode: 'items.title_empty', fieldErrors: [] } as never, messages);
      isTitleFailure = true;
      return;
    }
    const body = entryEditOf(
      { title: item.title, notes: item.notes ?? '', language: item.content_language ?? '' },
      { title: inlineTitle, notes: inlineNotes, language: item.content_language ?? '' },
    );
    isInlineDirty = false;
    if (Object.keys(body).length === 0) return;
    writeFailure = undefined;
    isTitleFailure = false;
    try {
      await items.update(item.id, body, item.version);
      announcer.say(t('app.entries.saved_announced'));
    } catch (error) {
      const problem = error as { detailCode?: string };
      writeFailure = renderProblem(error as never, messages);
      isTitleFailure = writeFailure.fields.has('/title') || problem.detailCode === 'items.title_empty';
    }
  }

  function revertInline() {
    if (!item) return;
    inlineTitle = item.title;
    inlineNotes = item.notes ?? '';
    isInlineDirty = false;
    writeFailure = undefined;
    isTitleFailure = false;
  }

  function onTitleKey(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      (event.currentTarget as HTMLTextAreaElement).blur();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      revertInline();
      (event.currentTarget as HTMLTextAreaElement).blur();
    }
  }

  /**
   * A mention or a card put into the notes where the caret was when the reader left them, and
   * saved as any other change of the notes is (UC-PLN-01 check 7).
   */
  let notesSelection = { start: 0, end: 0 };
  let isMentionOpen = $state(false);

  function rememberSelection(event: FocusEvent) {
    const field = event.currentTarget as HTMLTextAreaElement;
    notesSelection = { start: field.selectionStart, end: field.selectionEnd };
  }

  function insertMention(text: string) {
    const at = Math.min(notesSelection.start, inlineNotes.length);
    const result = inserted(inlineNotes, at, Math.max(at, Math.min(notesSelection.end, inlineNotes.length)), text);
    inlineNotes = result.text;
    notesSelection = { start: result.caret, end: result.caret };
    isInlineDirty = true;
    void commitInline();
  }

  function onNotesKey(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    event.preventDefault();
    revertInline();
    (event.currentTarget as HTMLTextAreaElement).blur();
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
    <ReplicaMark state={entry.state} />
    <ConflictStrip itemId={item.id} />

    <!-- The head (ADR-0061 decision 4). The heading is read and not drawn on every width: the
         title the reader sees is the field they edit it in, and a screen holds one `h1`. -->
    {#if !isInPane}
      <PageHeader
        title={item.title}
        isTitleInBar={true}
        isMenuInBar={viewport.isCompact}
        onmenu={(offered) => page.offer(offered)}
        breadcrumb={{ trail, label: t('app.workspace.trail'), expandLabel: t('app.workspace.expand_trail'), onnavigate: goTo }}
        menu={{ label: t('app.workspace.actions', { name: item.title }), items: entryMenu, opener: 'entry-menu', onselect: chooseFromMenu }}
      >
        {#snippet aside()}
          <!-- The panel's switch, the way back: where an entry could open beside its list, *Side
               panel* opens this one there and every one after it (design-system.md §11.8). -->
          {#if viewport.isLarge && item.collection_id}
            {@const collectionId = item.collection_id}
            <ViewSwitcher
              label={t('app.pane.view')}
              isIconOnly
              views={[
                { id: 'panel', label: t('app.pane.view_panel'), icon: 'panel-right' },
                { id: 'page', label: t('app.pane.view_page'), icon: 'maximize-2' },
              ]}
              selected="page"
              onselect={(id) => {
                if (id !== 'panel') return;
                device.setOpenMode('panel');
                onnavigate?.(`/collections/${collectionId}?item=${item.id}`);
              }}
            />
          {/if}
        {/snippet}
      </PageHeader>
    {/if}

    <!-- `data-tour`: where the tour points for "what an entry carries". -->
    <div class="head" data-tour="entry" data-celebrating={moment && moment.item.id === item.id ? '' : undefined}>
      {#if moment && moment.item.id === item.id}
        <CelebrationSlot current={moment} />
      {/if}
      <!-- The cover, where one is set: the stripe or the picture above the title, as on a card. -->
      {#if !carries('COVER')}
        <!-- A cover the type does not carry here stays stored and is not drawn. -->
      {:else if item.cover?.kind === 'IMAGE' && media.coverUrl(coverImageIdOf(item.cover), Date.now())}
        <img class="cover-image" src={media.coverUrl(coverImageIdOf(item.cover), Date.now())} alt="" />
      {:else if item.cover?.kind === 'COLOR' && item.cover.color_token}
        <div class="cover-stripe" data-token={item.cover.color_token} aria-hidden="true"></div>
      {/if}
      <div class="title-row">
        <!-- Completing the entry from its own page: the same control the row has, the same write. -->
        {#if isChangeable}
          <Checkbox
            label={t(item.completion?.is_completed ? 'app.entries.reopen' : 'app.entries.complete', { title: item.title })}
            isLabelHidden
            checked={item.completion?.is_completed ?? false}
            disabledReason={frozenReason}
            onchange={() => void toggleCompleted()}
          />
        {/if}
        <!-- The title, in place: text until it has focus. Enter saves, Escape restores, leaving
             saves; the label is announced and not drawn, because it is the title. -->
        <textarea
          class="title-field"
          class:done={item.completion?.is_completed}
          lang={entryLang}
          aria-label={t('app.entries.new_title')}
          aria-invalid={isTitleFailure ? 'true' : undefined}
          rows="1"
          bind:value={inlineTitle}
          readonly={!isWritable}
          oninput={() => (isInlineDirty = true)}
          onblur={() => void commitInline()}
          onkeydown={onTitleKey}
        ></textarea>
        <!-- The ticket key at the end of the title's line, not above it: no room is taken from the
             head where no cover is set. -->
        {#if ticketKey}
          {@const key = ticketKey}
          <button type="button" class="ticket-key" aria-label={t('app.entries.ticket_key', { key })} onclick={() => void copyTicketKey(key)}>{key}</button>
        {/if}
        {#if hasAi}
          <AiAsk {item} place="title" mayChange={isWritable} standing={standingAt('title')} ontranslate={() => (isTranslating = true)} />
        {/if}
        {#if isInPane}
          <!-- The pane's own head names the entry and leads to its page; its menu is at the end of
               the title's line, not on a line of its own above an empty band. -->
          <Menu label={t('app.workspace.actions', { name: item.title })} items={entryMenu} placement={{ side: 'block-end', align: 'end' }} onselect={chooseFromMenu}>
            {#snippet trigger(props)}
              <IconButton icon="ellipsis" label={t('app.workspace.actions', { name: item.title })} size="sm" data-opener="entry-menu" {...props} />
            {/snippet}
          </Menu>
        {/if}
      </div>
      {#if writeFailure && isTitleFailure}
        <p class="failure" role="alert">{writeFailure.message}</p>
      {/if}
      {#if hasAi}
        <AiPlace {item} place="title" proposals={proposalsAt('title')} mayChange={isWritable} onclose={closeProposal} />
        {#if isTranslating}
          <TranslatePanel {item} {languages} onclose={() => (isTranslating = false)} />
        {/if}
      {/if}
      <!-- What is set, as chips (design-system.md §11.8: set before empty). Each is also a row below.
           Per the effective profile (UC-WRK-23 check 4): a value the type no longer carries here
           stays stored and is not drawn, as its row is not. -->
      <!-- Beside the title only what the entry *is*: its level, its column, archived. Who, when and
           which labels are the details' (concept round 2, §4), not a second row of the same facts. -->
      <div class="marks">
        <Badge size="sm">{typeName(item.type)}</Badge>
        {#if isWritable && columnsHere.length > 0}
          <!-- The column is changed here as well as on the board (concept round 3): the mark is
               the menu of the collection's columns. -->
          <Menu label={t('app.item.column_menu')} items={columnItems} onselect={(id) => void setColumn(id === 'none' ? null : id)}>
            {#snippet trigger(props)}
              <button type="button" class="column-mark" data-column-mark {...props}>
                <Icon name="columns-3" size="sm" /><span>{columnName ?? t('app.board.unbucketed')}</span><Icon name="chevron-down" size="sm" />
              </button>
            {/snippet}
          </Menu>
        {:else if columnName}<Badge size="sm" icon="columns-3">{columnName}</Badge>{/if}
        {#if item.archived_at}
          <Badge size="sm" icon="archive">{t('app.entries.archived_label')}</Badge>
        {/if}
      </div>
      <!-- Kudos on a finished entry (UC-WRK-29 check 5): nothing is read or drawn while the
           celebrations are off (check 7), or while the entry is open. -->
      {#if kudosOffer(item, signedIn.account?.id, celebration.isOn) !== 'none'}
        {#key `${item.id}:${item.completion?.completed_by}`}<EntryKudos {item} />{/key}
      {/if}
      <!-- The notes, in place, for the same reasons; empty, the field says what it is for. An
           activity carries none (`domain-model.md` §2), and a field whose every save is refused
           is not a field. While the manifest has not been read, which type this is decides
           nothing yet, so the field is pending rather than absent. -->
      {#if writeFailure && !isTitleFailure}
        <p class="failure" role="alert">{writeFailure.message}</p>
      {/if}
      {#if completionFailure}
        <p class="failure" role="alert">{completionFailure.message}</p>
      {/if}
      {#if readOnlyReason}
        <!-- Why there is nothing to press: said once, not on every control (UC-WRK-22 check 2,
             UC-WRK-32 check 4). An entry archived in its own right offers the way back to whoever
             may take it. -->
        <ReadOnlyNotice text={readOnlyReason} textId={readOnlyNoticeId} icon={roleReason ? 'eye' : 'lock'}>
          {#snippet action()}
            {#if !roleReason && !isCollectionArchived && item.archived_at && isChangeable}
              <Button size="sm" tone="secondary" onclick={() => void unarchive()}>{t('app.entries.unarchive')}</Button>
            {/if}
          {/snippet}
        </ReadOnlyNotice>
      {/if}
    </div>

    <div class="columns" data-pane={isInPane ? '' : undefined}>
      <!-- The details before the text in the document: after the head they are what the entry
           is, so the reading order and the tab order meet them there on every width; from
           `expanded` they are drawn beside the text, at the end of the line. Absent where they
           would hold no row (`holdsDetails`). -->
      {#if holdsDetails}
        <aside class="details" aria-label={t('app.item.details')}>
          <!-- Open on a phone too while a proposal stands at a detail: it is never hidden in a fold. -->
          <details class="details-fold" bind:open={isDetailsOpen}>
            <summary class="details-summary">{t('app.item.details')}</summary>
            <div class="rows" data-folded={isFieldsOpen || hasDetailProposal ? undefined : ''} bind:this={detailRowsElement}>
              {#if isTypeKnown && isTypeOffered}
                {@render detailRows()}
              {:else if isTypeKnown}
                <!-- The manifest answered and does not declare this type. Its own sentence, once,
                     rather than fourteen refusals drawn as an empty column. -->
                <p class="unread" role="status">{offer.refusal({ code: 'items.type_unsupported', params: { item_type: item.type } })}</p>
              {:else if typeFailure}
                <!-- §4.4: a failure rendered as "there is nothing here" is a lie the reader acts
                     on. The rows are missing because the installation could not be read, and that
                     is what it says — with the sentence the server gave, and where to ask again. -->
                <p class="unread" role="status">{t('app.item.type_unread')}</p>
                <p class="unread-detail">{typeFailure.message}</p>
                {#if typeFailure.reference}
                  <p class="unread-detail">{t('app.error_reference', { request_id: typeFailure.reference })}</p>
                {/if}
              {:else}
                <div class="pending" aria-busy="true"><Skeleton lines={6} /></div>
              {/if}
            </div>
            <!-- The fields with nothing set wait behind one press: a short entry stays short, a
                 long one stays ordered (concept round 2, §4). -->
            {#if isTypeKnown && isTypeOffered && emptyFieldCount > 0 && !hasDetailProposal}
              <button type="button" class="more-fields" aria-expanded={isFieldsOpen} onclick={() => (isFieldsOpen = !isFieldsOpen)}>
                <Icon name={isFieldsOpen ? 'chevron-down' : 'chevron-right'} size="sm" />
                {isFieldsOpen ? t('app.item.fields_fewer') : setFieldCount === 0 ? t('app.item.fields_add') : t('app.item.fields_more', { count: String(emptyFieldCount) })}
              </button>
            {/if}
          </details>
        </aside>
      {/if}

      <div class="main-column">
        <!-- The description first in the main column, as the concept draws it: a box that reads
             as text, one quiet line while empty (concept round 2, §4). -->
          {#if !isTypeKnown}
            {#if !typeFailure}<div class="pending" aria-busy="true"><Skeleton lines={3} /></div>{/if}
          {:else if carries('NOTES') && (isWritable || inlineNotes.trim() !== '')}
          <textarea
            class="notes-field"
            lang={entryLang}
            aria-label={t('app.entries.notes')}
            placeholder={t('app.item.notes_placeholder')}
            rows={inlineNotes.trim() === '' ? 1 : 3}
            bind:value={inlineNotes}
            readonly={!isWritable}
            oninput={() => (isInlineDirty = true)}
            onblur={(event) => { rememberSelection(event); void commitInline(); }}
            onkeydown={onNotesKey}
          ></textarea>
          {#if isWritable}
            <div class="notes-tools">
              <Button size="sm" tone="subtle" icon="link" data-mention-entry onclick={() => (isMentionOpen = true)}>{t('app.mentions.insert')}</Button>
            </div>
            <MentionDialog bind:isOpen={isMentionOpen} selfId={item.id} oninsert={insertMention} />
          {/if}
          {/if}
          {#if hasAi && isTypeKnown && carries('NOTES')}
            {#if isChangeable}
              <div class="field-ai"><AiAsk {item} place="notes" mayChange={isWritable} standing={standingAt('notes')} /></div>
            {/if}
            <AiPlace {item} place="notes" proposals={proposalsAt('notes')} mayChange={isWritable} onclose={closeProposal} />
          {/if}

        {#if !isTypeKnown}
          <!-- What a type holds is the manifest's answer too, so "nothing sits under this" is
               not something this screen may say before it has one. -->
          {#if !typeFailure}<div class="pending" aria-busy="true"><Skeleton lines={4} /></div>{/if}
        {:else if takesChildren}
          <!-- The whole subtree (decisions 6 and 7): the same tree the expanded list draws, one
               level down, headed by what it holds and how much of it is done. -->
          <div class="parts"><Stack gap="150">
            <div class="section-head" data-tour="levels">
              <h2 class="section">
                {subtreeHeading}
                {#if children.length > 0}
                  <span class="section-count">{children.filter((child) => child.completion?.is_completed).length}/{children.length}</span>
                {/if}
              </h2>
              {#if children.length > 0}
                <Button size="sm" tone="subtle" onclick={() => { isSubtreeOpen = !isSubtreeOpen; subtree?.expandAll(isSubtreeOpen); }}>
                  {isSubtreeOpen ? t('app.item.collapse_all') : t('app.item.expand_all')}
                </Button>
              {/if}
              {#if hasAi}
                <AiAsk {item} place="steps" mayChange={isChangeable && !isCollectionArchived} standing={standingAt('steps')} />
              {/if}
            </div>
            {#if hasAi}
              <AiPlace {item} place="steps" proposals={proposalsAt('steps')} mayChange={isChangeable && !isCollectionArchived} onclose={closeProposal} />
            {/if}
            <EntryList bind:this={subtree} collectionId={item.collection_id} root={item} isReadOnly={isCollectionArchived} readOnlyReason={isCollectionArchived ? readOnlyReason : undefined} readOnlyNoticeId={readOnlyNoticeId} {onnavigate} mayShare={mayShareHere} changeRefusal={changeRefusalOf} mayCreate={mayCreateHere} />
          </Stack></div>
        {/if}

        <!-- The entry's status and the steps the reader may take (UC-FLW-03). -->
        {#key item.id}<StepControl {item} />{/key}
        <!-- Links from both sides, backlinks and what the notes mention (UC-PLN-01). Keyed, so
             each entry reads its own. -->
        {#key item.id}<LinksSection {item} mayChange={isWritable} />{/key}

        <Tabs
          label={t('app.item.tabs')}
          tabs={[
            // The conversation where the type has one. An activity carries no `COMMENTS`, and a
            // tab with a gate inside it saying so would be a whole panel spent on a refusal.
            ...(carries('COMMENTS')
              ? [{ id: 'comments', label: commentCount === undefined ? t('app.comments.title') : t('app.item.tab_with_count', { title: t('app.comments.title'), count: String(commentCount) }) }]
              : []),
            { id: 'activity', label: t('app.activity.title') },
          ]}
          selected={shownTab}
          onselect={(chosen) => (activeTab = chosen)}
        >
          {#if shownTab === 'comments'}
            {#if hasAi}
              <!-- What the discussion came to: a reading aid, asked and read by everybody who may
                   read the entry (UC-AI-04 check 8). -->
              <div class="field-ai"><AiAsk {item} place="comments" mayChange={isWritable} {hasComments} standing={standingAt('comments')} /></div>
              <AiPlace {item} place="comments" proposals={proposalsAt('comments')} mayChange={isWritable} onclose={closeProposal} />
            {/if}
            <CommentPanel {item} path={peoplePath} mayComment={frozenReason === undefined ? mayCommentHere : false} isClosed={frozenReason !== undefined} />
          {:else}
            <Stack gap="150">
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
                <!-- Cursor pagination, never a page number: the API has none, so no component may
                     imply one. What arrived is announced, because pressing a button and being told
                     nothing is the case a live region is for. -->
                <!-- Drawn whole even after the last page: the live region goes with the
                     component, and one removed with the button is one whose sentence about the
                     last page nobody hears. Without a further page it draws no button. -->
                {#if hasMore || historyArrived}
                  <LoadMore
                    label={t('app.activity.more')}
                    {hasMore}
                    arrivedLabel={historyArrived}
                    failedLabel={historyMoreFailed}
                    onLoadMore={() => void moreHistory()}
                  />
                {/if}
              {/if}
            </Stack>
          {/if}
        </Tabs>
      </div>

    </div>
  </Stack>
{/if}

{#snippet dueAsk()}
  {#if item}<AiAsk {item} place="due" mayChange={isWritable} standing={standingAt('due')} />{/if}
{/snippet}
{#snippet dueBelow()}
  {#if item}<AiPlace {item} place="due" proposals={proposalsAt('due')} mayChange={isWritable} onclose={closeProposal} />{/if}
{/snippet}
{#snippet labelsAsk()}
  {#if item}<AiAsk {item} place="labels" mayChange={isWritable} choices={aiChoices} standing={standingAt('labels')} />{/if}
{/snippet}
{#snippet labelsBelow()}
  {#if item}<AiPlace {item} place="labels" proposals={proposalsAt('labels')} mayChange={isWritable} onclose={closeProposal} />{/if}
{/snippet}

{#snippet detailRows()}
  {#if item}
            {#if carries('ASSIGNMENT') || carries('MEMBERS')}
              <DetailRow id="assignee" label={t('app.people.assignee')} value={item.assignee_id ? (accounts.nameOf(item.assignee_id) ?? t('app.people.unnamed')) : undefined} isReadOnly={!isWritable}>
                <AssigneePanel {item} path={peoplePath} />
              </DetailRow>
            {/if}
            {#if carries('DUE_DATE')}
              <!-- One row, because `DuePanel` is one editor: it writes the start and the due
                   together, and two rows opening it showed the reader the other date wherever
                   they pressed. -->
              <DetailRow id="due" label={t('app.due.title')} value={datesValue} isReadOnly={!isWritable} ask={hasAi ? dueAsk : undefined} below={hasAi ? dueBelow : undefined}>
                <DuePanel {item} />
              </DetailRow>
            {/if}
            {#if offersAmount}
              <DetailRow id="amount" label={t('app.entries.amount_label')} value={item.amount ?? undefined} isReadOnly={!isChangeable}>
                <AmountPanel {item} disabledReason={frozenReason} />
              </DetailRow>
            {/if}
            {#if carries('LABELS')}
              <DetailRow id="labels" label={t('app.labels.choose')} value={carriedLabels.length > 0 ? carriedLabels.map((label) => label.name).join(', ') : undefined} isReadOnly={!isWritable} ask={hasAi ? labelsAsk : undefined} below={hasAi ? labelsBelow : undefined}>
                <LabelsPanel {item} />
              </DetailRow>
            {/if}
            {#if carries('REMINDER')}
              <DetailRow id="reminders" label={t('app.reminders.title')} value={reminderCount > 0 ? t('app.item.reminders_count', { count: String(reminderCount) }) : undefined} isReadOnly={!isWritable}>
                <LazyView load={entryParts.ReminderPanel} props={{ item, path: peoplePath }} />
              </DetailRow>
            {/if}
            {#if carries('RECURRENCE')}
              <DetailRow id="recurrence" label={t('app.recurrence.title')} value={repeatValue} isReadOnly={!isWritable}>
                <LazyView load={entryParts.RecurrencePanel} props={{ item }} />
              </DetailRow>
            {/if}
            {#if itemType && typeChoices.length > 1}
              <DetailRow id="entry-type" label={t('app.item.entry_type')} value={entryTypeName(itemType)} isReadOnly={!isWritable}>
                <Stack gap="150">
                  <Select label={t('app.item.entry_type')} bind:value={typeDraft} options={typeChoices.map((each) => ({ value: each.key, label: entryTypeName(each) }))} />
                  <div>
                    <Button size="sm" onclick={() => void saveType()}>{t('app.workspace.save')}</Button>
                  </div>
                </Stack>
              </DetailRow>
            {/if}
            <DetailRow id="language" label={t('app.entries.language')} value={item.content_language ? languageName(item.content_language, messages.locale) : undefined} isReadOnly={!isWritable}>
              <Stack gap="150">
                <LanguagePicker
                  {languages}
                  bind:value={languageDraft}
                  label={t('app.entries.language')}
                  hint={t('app.entries.language_hint')}
                  otherLabel={t('app.entries.language_other')}
                  tagLabel={t('app.entries.language_tag')}
                  tagHint={t('app.entries.language_tag_hint')}
                />
                <div>
                  <Button size="sm" onclick={() => void saveLanguage()}>{t('app.workspace.save')}</Button>
                </div>
              </Stack>
            </DetailRow>
            {#if carries('COVER')}
              <DetailRow id="cover" label={t('app.media.cover')} value={item.cover ? t(`app.item.cover_${item.cover.kind}`) : undefined} isReadOnly={!isWritable}>
                <LazyView load={entryParts.CoverPanel} props={{ item }} />
              </DetailRow>
            {/if}
            {#if carries('ATTACHMENTS') && (isWritable || attachmentCount)}
              <DetailRow id="attachments" label={t('app.media.attachments')} value={attachmentCount ? t('app.item.attachments_count', { count: String(attachmentCount) }) : undefined}>
                <!-- Readable and downloadable by whoever reads the entry, archived or not; attaching and
                     detaching only where it may be written. -->
                <AttachmentPanel {item} isChangeable={isWritable} />
              </DetailRow>
            {/if}
            {#if carries('CUSTOM_FIELDS')}
              {#each fieldRows as row (row.definition.id)}
                {@const definition = row.definition}
                {@const held = (item.custom_fields as Record<string, unknown> | undefined)?.[definition.key]}
                {#if row.opens}<h3 class="form-section" data-form-section>{row.opens}</h3>{/if}
                <DetailRow
                  id={`field-${definition.key}`}
                  label={row.required ? t('app.entry_forms.required_label', { name: definition.key }) : definition.key}
                  value={valueText(definition, held, fieldReading)}
                  isReadOnly={!isWritable || row.readOnly}
                >
                  <CustomFieldPanel {item} path={peoplePath} only={definition.key} />
                </DetailRow>
              {/each}
            {/if}
  {/if}
{/snippet}

<!-- What the head's menu does, as on every surface: the dialogs once, the writes the same. A copy
     ends on the copy, which is a page; the trash leaves for the collection, with its *Undo*. -->
{#if item}
  {@const collectionId = item.collection_id}
  <EntryActions
    bind:this={actions}
    onopen={(itemId) => onnavigate?.(`/items/${itemId}`)}
    {onnavigate}
    ontrashed={() => onnavigate?.(`/collections/${collectionId}`)}
  />
{/if}

<style>
  .form-section {
    margin: 0;
    margin-block-start: var(--sp-150);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-secondary);
  }

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  .section-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-150); }

  .section-count { margin-inline-start: var(--sp-100); color: var(--text-subtle); font-size: var(--fs-100); font-weight: var(--fw-regular); font-variant-numeric: tabular-nums; }

  .head { display: flex; flex-direction: column; gap: var(--sp-150); position: relative; }


  .title-row { display: flex; align-items: center; gap: var(--sp-150); }

  /* The ticket key at the end of the title's line: quiet, in the code face, and a button because a
     click copies it - the reference a person pastes into a mail subject. */
  .ticket-key {
    flex: none;
    white-space: nowrap;
    padding: 0 var(--sp-050);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    cursor: pointer;
  }
  .ticket-key:hover { background: var(--bg-surface-hover); color: var(--text-secondary); }
  .ticket-key:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }

  /* The title as text until it has focus: the display face, no border, no surface; the field
     shows itself on focus with rule 5's ring and a surface, and only then. A textarea rather than
     an input so that a long title wraps as text does (rule 4); Enter is what saves it, so it
     never holds a line break. `field-sizing` grows it with its words where the engine has it and
     the row count is the floor where it has not. */
  .title-field {
    flex: 1;
    min-width: 0;
    margin: 0;
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    resize: none;
    overflow: hidden;
    field-sizing: content;
    overflow-wrap: anywhere;
  }

  .title-field:hover:not(:read-only) { background: var(--bg-surface-hover); }

  .title-field:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    background: var(--bg-surface);
    border-color: var(--border-subtle);
  }

  /* Rule 3: a completed entry is struck as well as ticked. */
  .title-field.done { color: var(--text-subtle); text-decoration: line-through; }

  /* A ✦ that has no field row of its own sits at the end of the line under its subject. */
  .field-ai { display: flex; justify-content: flex-end; }

  .notes-field {
    inline-size: 100%;
    box-sizing: border-box;
    margin: 0;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    line-height: var(--lh-normal);
    resize: vertical;
    field-sizing: content;
    min-block-size: var(--density-control-md-min);
  }

  .notes-tools { display: flex; justify-content: end; }

  .notes-field:placeholder-shown { color: var(--text-subtle); }
  .notes-field:hover:not(:read-only) { border-color: var(--border-default); }

  .notes-field:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    background: var(--bg-surface);
    border-color: var(--border-subtle);
    color: var(--text-primary);
  }

  /* The parts as one card: their heading, done of total, the rows and the add. */
  .parts {
    padding: var(--sp-150) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .marks { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .cover-image { inline-size: 100%; max-block-size: var(--sp-1600); object-fit: cover; border-radius: var(--r-lg); }

  /* The ten label colours, as a card draws them (`WorkItemCard`): the stripe reads its token. */
  .cover-stripe { block-size: var(--sp-100); border-radius: var(--r-full); background: var(--bg-surface-sunken); }
  .cover-stripe[data-token='slate'] { background: var(--label-slate-bg); }
  .cover-stripe[data-token='blue'] { background: var(--label-blue-bg); }
  .cover-stripe[data-token='teal'] { background: var(--label-teal-bg); }
  .cover-stripe[data-token='green'] { background: var(--label-green-bg); }
  .cover-stripe[data-token='lime'] { background: var(--label-lime-bg); }
  .cover-stripe[data-token='amber'] { background: var(--label-amber-bg); }
  .cover-stripe[data-token='orange'] { background: var(--label-orange-bg); }
  .cover-stripe[data-token='red'] { background: var(--label-red-bg); }
  .cover-stripe[data-token='magenta'] { background: var(--label-magenta-bg); }
  .cover-stripe[data-token='violet'] { background: var(--label-violet-bg); }

  /* Two columns from `expanded`: the text and the tree, and the details beside them at the
     pane's width. One column below, with the details folded under the head. */
  .columns { display: flex; flex-direction: column; gap: var(--sp-300); }

  .main-column { display: flex; flex-direction: column; gap: var(--sp-300); min-width: 0; flex: 1; }

  .details { min-width: 0; }

  /* A table inside the page or the panel, not a card on it: the border and no shadow of its own,
     so nothing is a layer over the layer it sits in. */
  .details-fold { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }

  .details-summary {
    padding: var(--sp-150) var(--sp-200);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-subtle);
    cursor: pointer;
  }

  .details-summary:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
    border-radius: var(--r-lg);
  }

  .rows { display: flex; flex-direction: column; padding: 0 var(--sp-100) var(--sp-100); }

  /* Folded: the fields with nothing set are out of the way until *More fields* is pressed. */
  .rows[data-folded] > :global(.detail[data-empty]) { display: none; }

  .more-fields {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    margin: 0 var(--sp-100) var(--sp-100);
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }
  .more-fields:hover { background: var(--bg-surface-hover); color: var(--text-primary); }
  .more-fields:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  /* In the panel the details are a grid of two at the top, as the concept draws it. */
  .columns[data-pane] .rows { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: var(--sp-200); }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .columns:not([data-pane]) { flex-direction: row-reverse; align-items: flex-start; }

    .columns:not([data-pane]) .details { flex: none; inline-size: var(--layout-pane-width); position: sticky; inset-block-start: calc(var(--layout-appbar-height) + var(--sp-200)); }

    /* From `expanded` the details never fold: the disclosure is the phone's, and the summary is
       a heading in all but name. Inside the pane the column is one and the fold stays a fold. */
    .details-summary { pointer-events: none; list-style: none; }

    .details-summary::-webkit-details-marker { display: none; }

    .columns[data-pane] .details-summary { pointer-events: auto; list-style: disclosure-closed inside; }

    .columns[data-pane] details[open] > .details-summary { list-style: disclosure-open inside; }
  }


  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

  /* What is missing and why, where the rows would be. Inside the fold's padding, because it
     stands in for the rows rather than beside them. */
  .pending { padding: var(--sp-150) 0; }

  .unread { margin: var(--sp-150) 0 0; color: var(--text-primary); font-size: var(--fs-075); }

  .unread-detail { margin: var(--sp-050) 0 var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }

  /* The column mark as a menu: a small badge's measure, with the turn that says it opens. */
  .column-mark {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    box-sizing: border-box;
    min-block-size: calc(var(--fs-100) + 2 * var(--sp-050) + 2 * var(--bw-hairline));
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--status-neutral-border);
    border-radius: var(--r-sm);
    background: var(--status-neutral-surface);
    color: var(--status-neutral-text);
    font: inherit;
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
    line-height: 1;
    cursor: pointer;
  }

  .column-mark:hover { border-color: var(--border-default); }

  .column-mark:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
