<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry, at the address the board and the search results already linked to: its head, the
  // whole subtree under it, its details beside its text, and its history (ADR-0061 decision 4).
  //
  // The head is the entry itself - the completion checkbox, the title and the notes edited in
  // place, the set values as chips. In place means an input that looks like text until it has
  // focus, the same `PATCH`, the same announcement and the same conflict path the form had; the
  // form stays behind "edit" in the menu for a reader who wants a form. The subtree is
  // `EntryList` with a root (decision 6). The details are rows that open the editors the product
  // already has; comments and activity are tabs. Two columns from `expanded`, one below.
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
    Checkbox,
    EmptyState,
    ErrorState,
    focusFirst,
    IconButton,
    Inline,
    Input,
    LabelChip,
    LoadMore,
    Menu,
    PageHeader,
    Skeleton,
    Stack,
    Tabs,
    Textarea,
    type ActivityStep,
    type MenuItem,
  } from '@hubtask/design-system/components';
  import type { ActivityEntry, ActivityPage, CommentPage, MediaPage, WorkItem } from '@hubtask/sync-engine';

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
  import { childTypes, supports } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { customFields } from '../lib/data/customfields.svelte.ts';
  import { definitionsFor } from '../lib/data/customfields.ts';
  import { commentsPath } from '../lib/data/comments.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { entryEditOf, type EntryDraft } from '../lib/data/edits.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { labels } from '../lib/data/labels.svelte.ts';
  import { attachmentsPath, media } from '../lib/data/media.svelte.ts';
  import { coverImageIdOf } from '../lib/data/media.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { reminders, series } from '../lib/data/reminders.svelte.ts';
  import { celebration } from '../lib/celebration.svelte.ts';
  import CelebrationSlot from '../lib/entries/CelebrationSlot.svelte';
  import CustomFieldPanel from '../lib/entries/CustomFieldPanel.svelte';
  import DetailRow from '../lib/entries/DetailRow.svelte';
  import DueMark from '../lib/entries/DueMark.svelte';
  import EntryList from '../lib/entries/EntryList.svelte';
  import LabelsPanel from '../lib/entries/LabelsPanel.svelte';
  import ReplicaMark from '../lib/frame/ReplicaMark.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { recents } from '../lib/recents.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import PeopleMarks from '../lib/people/PeopleMarks.svelte';
  import DuePanel from '../lib/entries/DuePanel.svelte';
  import RecurrencePanel from '../lib/entries/RecurrencePanel.svelte';
  import LanguagePicker from '../lib/entries/LanguagePicker.svelte';
  import ConflictStrip from '../lib/entries/ConflictStrip.svelte';
  import SuggestionStrip from '../lib/entries/SuggestionStrip.svelte';
  import TranslatePanel from '../lib/entries/TranslatePanel.svelte';
  import ReminderPanel from '../lib/entries/ReminderPanel.svelte';
  import AttachmentPanel from '../lib/media/AttachmentPanel.svelte';
  import CoverPanel from '../lib/media/CoverPanel.svelte';
  import AssigneePanel from '../lib/people/AssigneePanel.svelte';
  import CommentPanel from '../lib/people/CommentPanel.svelte';
  import MembersDialog from '../lib/people/MembersDialog.svelte';
  import { activityPath, itemPath } from '../lib/data/item.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { actor as signedIn } from '../lib/data/account.svelte.ts';
  import { formatDateTime, formatDue } from '../lib/i18n/datetime.ts';
  import { humanise } from '../lib/i18n/messages.ts';
  import { textLanguages } from '../lib/data/query.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';

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

  // Read once, and `untrack` says the once is deliberate: `App.svelte` keys this view on the id,
  // so a different entry is a different component rather than the same one asking again. A
  // resource that followed the prop would be a second answer to which entry this screen is.
  const entry = resource<WorkItem>(untrack(() => itemPath(id)));
  const history = resource<ActivityPage>(untrack(() => activityPath(id)));

  const item = $derived(entry.state.status === 'ready' ? entry.state.data : undefined);

  let isSharing = $state(false);

  // The bar carries the title on a phone (ADR-0061 decision 2); on every width the head's `h1`
  // is read rather than drawn, because the title the reader sees is the field they edit it in.
  $effect(() => (isInPane ? undefined : page.entitle(item?.title)));

  // What the overview's "what you had open" is built from. Noted when the entry has a title rather
  // than when the route resolved: a row naming an entry nothing could name yet would be a blank.
  // Not in the pane, where the reader is on the list beside it rather than on this.
  $effect(() => {
    if (isInPane || !item?.title) return;
    recents.note({ kind: 'item', id: item.id, title: item.title });
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
  const hub = $derived(collection?.parent_id ? containers.find(collection.parent_id) : undefined);
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
  // The series is asked for only when the row says there is one (issue 882); the effect follows
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
  const carriedLabels = $derived(
    item ? (item.label_ids ?? []).map((labelId) => labels.of(item.collection_id).find((each) => each.id === labelId)).filter((each) => each !== undefined) : [],
  );
  const definitions = $derived(item ? definitionsFor(customFields.of(item.collection_id), item.type as string) : []);

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
   * panel twice and the reader met the other date wherever they pressed (issue 916). One row,
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
   * `pending` — before the manifest has arrived — draws nothing either, because nothing is
   * knowable yet and a row that appeared late is better than one that was wrong.
   */
  const carries = $derived((capability: string) => (item ? supports(item.type, capability).status === 'permitted' : false));
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

  /** The subtree's head: the child type as the manifest names it, and how much of it is done. */
  const childType = $derived(item ? childTypes(item.type)[0] : undefined);
  /** A type as words: the manifest's identifier, read as `humanise` reads a code ("Work package"). */
  const typeName = (type: string) => humanise(type.toLowerCase());
  const subtreeHeading = $derived(childType ? typeName(childType) : t('app.item.children'));
  let subtree = $state<EntryList | undefined>(undefined);
  let isSubtreeOpen = $state(true);

  /** The entry's own menu: the form for the keyboard, and sharing. */
  const entryMenu = $derived<MenuItem[]>([
    { id: 'edit', label: t('app.entries.edit'), icon: 'pencil', disabledReason: item?.archived_at ? t('app.entries.archived') : undefined },
    { id: 'share', label: t('app.people.share'), icon: 'users' },
  ]);

  let activeTab = $state('comments');
  /** The tab that is actually shown: the history, where this type has no conversation to open. */
  const shownTab = $derived(activeTab === 'comments' && !carries('COMMENTS') ? 'activity' : activeTab);

  function chooseFromMenu(chosen: string) {
    if (chosen === 'edit') startEditing();
    else if (chosen === 'share') isSharing = true;
  }

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
  /** The entry's language as the editor holds it; empty is "none stated". */
  let draftLanguage = $state('');
  /** What the entry held when the form opened, so that the write names only what moved (`edits.ts`). */
  let opened: EntryDraft = { title: '', notes: '', language: '' };
  const languages = $derived(textLanguages(manifest.value));

  // The language the entry is written in, where it differs from the page's, so that a screen
  // reader switches voice on the title and the notes (design-system.md §10, 3.1.2). The same
  // language as the document says nothing - `lang` there would be noise.
  const entryLang = $derived(
    item?.content_language && item.content_language !== messages.locale ? item.content_language : undefined,
  );
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

  // The strip exists exactly when the manifest says AI is configured for this workspace, and not
  // otherwise - not disabled with a reason, not rendered empty. An installation with AI switched
  // off has a product that never mentioned it (milestone-F5.md decision 4), and the tokens the
  // strip is the only consumer of leave with it.
  const hasAi = $derived(manifest.value?.features?.ai_suggestions === true);

  // What sits under this entry, for the kinds that hold anything (F5-02): an accepted breakdown
  // creates children, and a screen that showed the proposal but not what it made would leave
  // the reader to find them in the collection. The subtree is the one read the section below
  // makes (issue 877), and this counts its first level; the acceptance's invalidation of the
  // lists brings them here without being asked.
  const takesChildren = $derived(item ? childTypes(item.type).length > 0 : false);
  $effect(() => {
    if (!takesChildren) return;
    return untrack(() => items.openSubtree(id));
  });
  const children = $derived(takesChildren ? items.childrenOf(id) : []);
  function startEditing() {
    if (!item) return;
    opened = { title: item.title, notes: item.notes ?? '', language: item.content_language ?? '' };
    draftTitle = opened.title;
    draftNotes = opened.notes;
    draftLanguage = opened.language;
    writeFailure = undefined;
    isTitleFailure = false;
    isEditing = true;
  }

  async function save() {
    if (!item || draftTitle.trim() === '' || isSaving) return;
    // Only what moved since the form opened (issue 779, offline-sync.md §4.2): a field repeated
    // unchanged would be queued with a fresh clock and win a merge it never entered.
    const body = entryEditOf(opened, { title: draftTitle, notes: draftNotes, language: draftLanguage });
    if (Object.keys(body).length === 0) {
      // Nothing moved: no write, and no clock stamped on a value nobody changed.
      isEditing = false;
      return;
    }
    isSaving = true;
    writeFailure = undefined;
    isTitleFailure = false;
    try {
      await items.update(item.id, body, item.version);
      // Both the entry and its history come back on their own: the write invalidates `/items`, and
      // the engine matches by prefix — so `/items/{id}` and `/items/{id}/activity` are re-read
      // without either being asked for here. A refresh would be a second read of what is arriving.
      announcer.say(t('app.entries.saved_announced'));
      isEditing = false;
    } catch (error) {
      const problem = error as { detailCode?: string };
      writeFailure = renderProblem(error as never, messages);
      isTitleFailure = writeFailure.fields.has('/title') || problem.detailCode === 'items.title_empty';
    } finally {
      isSaving = false;
    }
  }

  /**
   * The title and the notes edited in place: the same fields the form has, drawn as text until
   * they have focus, saved when the reader leaves them or presses Enter in the title, restored by
   * Escape. The write is `save()`'s - only what moved, the same version, the same announcement,
   * the same conflict path - so an in-place edit and a form edit cannot disagree.
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
    isSaving = true;
    writeFailure = undefined;
    isTitleFailure = false;
    try {
      await items.update(item.id, body, item.version);
      announcer.say(t('app.entries.saved_announced'));
    } catch (error) {
      const problem = error as { detailCode?: string };
      writeFailure = renderProblem(error as never, messages);
      isTitleFailure = writeFailure.fields.has('/title') || problem.detailCode === 'items.title_empty';
    } finally {
      isSaving = false;
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
    {#if isInPane}
      <!-- The pane's own head names the entry and leads to its page; the menu stands alone. -->
      <div class="pane-menu">
        <Menu label={t('app.workspace.actions', { name: item.title })} items={entryMenu} placement={{ side: 'block-end', align: 'end' }} onselect={chooseFromMenu}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={t('app.workspace.actions', { name: item.title })} tone="secondary" data-opener="entry-menu" {...props} />
          {/snippet}
        </Menu>
      </div>
    {:else}
      <PageHeader
        title={item.title}
        isTitleInBar={true}
        breadcrumb={{ trail, label: t('app.workspace.trail'), expandLabel: t('app.workspace.expand_trail'), onnavigate: goTo }}
        menu={{ label: t('app.workspace.actions', { name: item.title }), items: entryMenu, opener: 'entry-menu', onselect: chooseFromMenu }}
      />
    {/if}

    {#if isEditing}
      <!-- The form, for a reader who asked for one from the menu: it takes the focus (2.4.3). -->
      <Stack gap="150" {@attach focusFirst({ returnTo: '[data-opener="entry-menu"]' })}>
        <Input
          label={t('app.entries.new_title')}
          bind:value={draftTitle}
          error={isTitleFailure ? writeFailure?.message : undefined}
        />
        {#if carries('NOTES')}
          <Textarea label={t('app.entries.notes')} bind:value={draftNotes} rows={6} />
        {/if}
        <LanguagePicker
          {languages}
          bind:value={draftLanguage}
          label={t('app.entries.language')}
          hint={t('app.entries.language_hint')}
          otherLabel={t('app.entries.language_other')}
          tagLabel={t('app.entries.language_tag')}
          tagHint={t('app.entries.language_tag_hint')}
        />
        <!-- Everything that is not about the title is a sentence above the buttons: a version
             conflict is the ordinary case here, and nothing about the title is wrong when the
             entry moved underneath the reader. -->
        {#if writeFailure && !isTitleFailure}
          <p class="failure" role="alert">{writeFailure.message}</p>
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
      <!-- `data-tour`: where the tour points for "what an entry carries" (F6-14). -->
      <div class="head" data-tour="entry" data-celebrating={moment && moment.item.id === item.id ? '' : undefined}>
        {#if moment && moment.item.id === item.id}
          <CelebrationSlot current={moment} />
        {/if}
        <!-- The cover, where one is set: the stripe or the picture above the title, as on a card. -->
        {#if item.cover?.kind === 'IMAGE' && media.coverUrl(coverImageIdOf(item.cover), Date.now())}
          <img class="cover-image" src={media.coverUrl(coverImageIdOf(item.cover), Date.now())} alt="" />
        {:else if item.cover?.kind === 'COLOR' && item.cover.color_token}
          <div class="cover-stripe" data-token={item.cover.color_token} aria-hidden="true"></div>
        {/if}
        <div class="title-row">
          <!-- Completing the entry from its own page: the same control the row has, the same write. -->
          <Checkbox
            label={t(item.completion?.is_completed ? 'app.entries.reopen' : 'app.entries.complete', { title: item.title })}
            isLabelHidden
            checked={item.completion?.is_completed ?? false}
            disabledReason={frozenReason}
            onchange={() => void toggleCompleted()}
          />
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
            readonly={frozenReason !== undefined}
            title={frozenReason}
            oninput={() => (isInlineDirty = true)}
            onblur={() => void commitInline()}
            onkeydown={onTitleKey}
          ></textarea>
        </div>
        {#if writeFailure && isTitleFailure}
          <p class="failure" role="alert">{writeFailure.message}</p>
        {/if}
        <!-- What is set, as chips (decision 4: set before empty). Each is also a row below. -->
        <div class="marks">
          <Badge>{typeName(item.type)}</Badge>
          {#if item.archived_at}
            <Badge icon="archive">{t('app.entries.archived_label')}</Badge>
          {/if}
          <PeopleMarks assigneeId={item.assignee_id} memberIds={item.member_ids ?? []} />
          <DueMark {item} />
          {#each carriedLabels as label (label.id)}
            <LabelChip name={label.name} colorToken={label.color_token} description={label.description} />
          {/each}
          {#if repeatValue}<Badge icon="repeat">{repeatValue}</Badge>{/if}
          {#if reminderCount > 0}<Badge icon="bell">{t('app.item.reminders_count', { count: String(reminderCount) })}</Badge>{/if}
          {#if item.content_language}<Badge icon="globe">{languageName(item.content_language, messages.locale)}</Badge>{/if}
        </div>
        <!-- The notes, in place, for the same reasons; empty, the field says what it is for. An
             activity carries none (`domain-model.md` §2), and a field whose every save is refused
             is not a field. -->
        {#if carries('NOTES')}
        <textarea
          class="notes-field"
          lang={entryLang}
          aria-label={t('app.entries.notes')}
          placeholder={t('app.item.notes_placeholder')}
          rows="3"
          bind:value={inlineNotes}
          readonly={frozenReason !== undefined}
          oninput={() => (isInlineDirty = true)}
          onblur={() => void commitInline()}
          onkeydown={onNotesKey}
        ></textarea>
        {/if}
        {#if writeFailure && !isTitleFailure}
          <p class="failure" role="alert">{writeFailure.message}</p>
        {/if}
        {#if completionFailure}
          <p class="failure" role="alert">{completionFailure.message}</p>
        {/if}
      </div>
    {/if}

    <div class="columns" data-pane={isInPane ? '' : undefined}>
      <!-- The details before the text in the document: after the head they are what the entry
           is, so the reading order and the tab order meet them there on every width; from
           `expanded` they are drawn beside the text, at the end of the line. -->
      <aside class="details" aria-label={t('app.item.details')}>
        <details class="details-fold" open={!viewport.isCompact}>
          <summary class="details-summary">{t('app.item.details')}</summary>
          <div class="rows">
            {@render detailRows()}
          </div>
        </details>
      </aside>

      <div class="main-column">
        {#if hasAi}
          <Stack gap="150" data-ai>
            <h2 class="section">{t('app.suggestions.title')}</h2>
            <SuggestionStrip {item} />
            <TranslatePanel {item} {languages} />
          </Stack>
        {/if}

        {#if takesChildren}
          <!-- The whole subtree (decisions 6 and 7): the same tree the expanded list draws, one
               level down, headed by what it holds and how much of it is done. -->
          <Stack gap="150">
            <div class="section-head">
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
            </div>
            <EntryList bind:this={subtree} collectionId={item.collection_id} root={item} isReadOnly={frozenReason !== undefined} />
          </Stack>
        {/if}

        <Tabs
          label={t('app.item.tabs')}
          tabs={[
            // The conversation where the type has one. An activity carries no `COMMENTS`, and the
            // tab used to be there with a gate inside it saying so - a whole panel spent on a
            // refusal (issue 916).
            ...(carries('COMMENTS')
              ? [{ id: 'comments', label: commentCount === undefined ? t('app.comments.title') : t('app.item.tab_with_count', { title: t('app.comments.title'), count: String(commentCount) }) }]
              : []),
            { id: 'activity', label: t('app.activity.title') },
          ]}
          selected={shownTab}
          onselect={(chosen) => (activeTab = chosen)}
        >
          {#if shownTab === 'comments'}
            <CommentPanel {item} path={peoplePath} />
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
                {#if hasMore}
                  <LoadMore
                    label={t('app.activity.more')}
                    arrivedLabel={t('app.activity.arrived', { count: steps.length })}
                    onLoadMore={() => history.loadMore()}
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

{#snippet detailRows()}
  {#if item}
            {#if carries('ASSIGNMENT') || carries('MEMBERS')}
              <DetailRow id="assignee" label={t('app.people.assignee')} value={item.assignee_id ? (accounts.nameOf(item.assignee_id) ?? t('app.people.unnamed')) : undefined}>
                <AssigneePanel {item} path={peoplePath} />
              </DetailRow>
            {/if}
            {#if carries('DUE_DATE')}
              <!-- One row, because `DuePanel` is one editor: it writes the start and the due
                   together, and two rows opening it showed the reader the other date wherever
                   they pressed. -->
              <DetailRow id="due" label={t('app.due.title')} value={datesValue}>
                <DuePanel {item} disabledReason={frozenReason} />
              </DetailRow>
            {/if}
            {#if carries('LABELS')}
              <DetailRow id="labels" label={t('app.labels.choose')} value={carriedLabels.length > 0 ? carriedLabels.map((label) => label.name).join(', ') : undefined}>
                <LabelsPanel {item} disabledReason={frozenReason} />
              </DetailRow>
            {/if}
            {#if carries('REMINDER')}
              <DetailRow id="reminders" label={t('app.reminders.title')} value={reminderCount > 0 ? t('app.item.reminders_count', { count: String(reminderCount) }) : undefined}>
                <ReminderPanel {item} path={peoplePath} />
              </DetailRow>
            {/if}
            {#if carries('RECURRENCE')}
              <DetailRow id="recurrence" label={t('app.recurrence.title')} value={repeatValue}>
                <RecurrencePanel {item} />
              </DetailRow>
            {/if}
            <DetailRow id="language" label={t('app.entries.language')} value={item.content_language ? languageName(item.content_language, messages.locale) : undefined}>
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
                  <Button size="sm" onclick={() => void saveLanguage()} disabledReason={frozenReason}>{t('app.workspace.save')}</Button>
                </div>
              </Stack>
            </DetailRow>
            {#if carries('COVER')}
              <DetailRow id="cover" label={t('app.media.cover')} value={item.cover ? t(`app.item.cover_${item.cover.kind}`) : undefined}>
                <CoverPanel {item} />
              </DetailRow>
            {/if}
            {#if carries('ATTACHMENTS')}
              <DetailRow id="attachments" label={t('app.media.attachments')} value={attachmentCount ? t('app.item.attachments_count', { count: String(attachmentCount) }) : undefined}>
                <AttachmentPanel {item} />
              </DetailRow>
            {/if}
            {#if carries('CUSTOM_FIELDS')}
              {#each definitions as definition (definition.id)}
                {@const held = (item.custom_fields as Record<string, unknown> | undefined)?.[definition.key]}
                <DetailRow id={`field-${definition.key}`} label={definition.key} value={held === undefined || held === null || held === '' ? undefined : String(held)}>
                  <CustomFieldPanel {item} path={peoplePath} only={definition.key} />
                </DetailRow>
              {/each}
            {/if}
  {/if}
{/snippet}

{#if item}
  <MembersDialog
    bind:isOpen={isSharing}
    title={t('app.people.share_title')}
    scope={{ scopeType: 'ITEM', scopeId: item.id }}
    path={peoplePath}
  />
{/if}

<style>
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

  .notes-field {
    inline-size: 100%;
    max-inline-size: 64ch;
    box-sizing: border-box;
    margin: 0;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    line-height: var(--lh-normal);
    resize: vertical;
    field-sizing: content;
    min-block-size: calc(var(--density-control-md-min) * 2);
  }

  .notes-field:hover:not(:read-only) { background: var(--bg-surface-hover); }

  .notes-field:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    background: var(--bg-surface);
    border-color: var(--border-subtle);
    color: var(--text-primary);
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

  .details-fold { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }

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

  .pane-menu { display: flex; justify-content: flex-end; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
