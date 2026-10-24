<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The capture field (UC-WRK-26): one line, typed anywhere, becomes an entry in the right place.
  // The frame mounts it once; `Q`, the bar's *Capture* and the bottom bar's ＋ open it.
  //
  // **It reads, it does not decide.** What a line says is `parse.ts`'s, where it goes and what is
  // read there `target.ts`'s, what Enter writes `submit.ts`'s - all pure and tested - and *Undo* is
  // the one way back every act offers (`entries/undo`). This binds them to the stores the screens
  // already read, and draws.
  //
  // **The target line is the field's hint**, so it is the field's description too: a reader hears
  // where the entry goes with the field, and again whenever typing pauses, through the frame's one
  // live region (design-system.md §10, 4.1.3).

  import { untrack } from 'svelte';

  import { Button, Checkbox, Dialog, LabelChip, Menu, Textarea } from '@hubtask/design-system/components';
  import { DEFAULT_TIMEOUT_MS, type Container, type TransportError, type WorkItem } from '@hubtask/sync-engine';

  import { capture, type Mode } from './capture.svelte.ts';
  import { SIGILS, offersFor, pick, tokenAt, type Offer, type Sigil, type Token } from './suggest.ts';
  import { commands } from '../palette/commands.svelte.ts';
  import { offersMakeEntries } from './entries.ts';
  import MakeEntries from './MakeEntries.svelte';
  import Lookalikes from '../entries/Lookalikes.svelte';
  import { NOTHING, literalKey, readLine, readTargets, type Chip, type Context, type Named, type Reading } from './parse.ts';
  import { admits, jumbleWritable, liveCollections, liveHubs, openLevels, openRoles, pathOf, roleIn } from './places.svelte.ts';
  import { bodyOf, createAll, readLinesOf, type Writer } from './submit.ts';
  import { recognitionOf, resolveTarget, rulesOf, topLevelOf, type Target } from './target.ts';
  import { vocabularyOf } from './words.ts';
  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { itemAccess } from '../data/capability.ts';
  import { holds, offerIn, rootTypes } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { archivalOf } from '../data/containers.ts';
  import { engine } from '../data/engine.ts';
  import { itemPath } from '../data/item.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { jumble } from '../data/jumble.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { suggestions } from '../data/suggestions.svelte.ts';
  import { captureRulesOf, unitsOf } from '../entries/capture.ts';
  import { narrow, partition } from '../entries/simplelist.ts';
  import { offerUndo } from '../entries/undo.svelte.ts';
  import { formatDue } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { instantOf } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The collection the reader stands in, from the route. */
    currentCollectionId?: string;
    /** The entry the reader stands on, whose collection is then the current one. */
    currentItemId?: string;
    /** The hub the reader stands in, which settles an `@` name two hubs share. */
    currentHubId?: string;
    /** Where a mode that is a screen of its own - a template, an import - takes the person. */
    onnavigate?: (path: string) => void;
  }

  const { currentCollectionId, currentItemId, currentHubId, onnavigate }: Props = $props();

  /** The place chosen in the dialog's own chip, over the door's and the route's: a collection, or the jumble. */
  let chosen = $state<string | undefined>(undefined);
  /** The level chosen where the door offers several. */
  let chosenType = $state<string | undefined>(undefined);
  /** Whether the entry still goes under the door's entry; its chip takes it out. */
  let isUnderParent = $state(true);
  let mode = $state<Mode>('one');
  /** Whether the field stays open after Enter, for a list typed line after line. */
  let keepsOpen = $state(false);
  /** Where the caret stands, for the token it is in. */
  let caret = $state(0);
  /** The suggestion the arrows stand on. */
  let active = $state(0);
  /** A token the person dismissed with Escape: its list stays shut until the token changes. */
  let dismissed = $state<string | undefined>(undefined);

  let text = $state('');
  /** The chips taken back in this text: their words stay in the title. */
  let literal = $state<ReadonlySet<string>>(new Set());
  /** The clock the line is read against, taken at each keystroke. */
  let now = $state(Date.now());
  let error = $state<string | undefined>(undefined);
  let isBusy = $state(false);
  /** Enter was pressed while the target was still being worked out (record D14). */
  let isWaiting = $state(false);
  /** One key per Enter, kept while the same text is sent again (`submit.ts`). */
  let attempt: { readonly text: string; readonly keys: readonly string[] } | undefined;
  let field = $state<HTMLElement | null>(null);

  // ------------------------------------------------------------------------------------------
  // Where the reader stands
  // ------------------------------------------------------------------------------------------

  /** The collection of the entry the reader is on, read through the entry page's own read. */
  let itemCollectionId = $state<string | undefined>(undefined);
  $effect(() => {
    const id = currentItemId;
    if (!capture.isOpen || !id) {
      itemCollectionId = undefined;
      return;
    }
    return untrack(() =>
      engine.subscribe<WorkItem>({ path: itemPath(id) }, (next) => {
        if (next.status === 'ready') itemCollectionId = next.data.collection_id;
      }),
    );
  });

  const door = $derived(capture.door);
  const current = $derived(chosen === 'jumble' ? undefined : (chosen ?? door?.collectionId ?? currentCollectionId ?? itemCollectionId));
  const hubHere = $derived(current ? (containers.find(current)?.parent_id ?? undefined) : currentHubId);

  // Every hub's collections while open, so `@` finds a collection wherever it is.
  const hubIds = $derived(capture.isOpen ? containers.hubs.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    if (hubIds === '') return;
    return openLevels();
  });

  // ------------------------------------------------------------------------------------------
  // Reading the line
  // ------------------------------------------------------------------------------------------

  const words = $derived.by(() => {
    void messages.locale;
    return vocabularyOf((code) => t(code));
  });

  const named = (container: Container): Named => ({ id: container.id, name: container.name });

  const context = $derived<Context>({
    now,
    zone: actor.zone,
    words,
    hubs: liveHubs().map(named),
    collections: liveCollections()
      .filter((collection) => admits(collection) !== false)
      .map((collection) => ({ ...named(collection), hubId: collection.parent_id ?? '' })),
    currentHubId: hubHere,
    literal,
  });

  const targets = $derived(readTargets(text, context));

  const levelOf = (hubId: string): readonly Container[] | undefined =>
    containers.isLevelLoading(hubId) ? undefined : containers.collectionsOf(hubId).filter((each) => archivalOf(each) === 'active');

  const target = $derived<Target>(
    resolveTarget({
      namedCollectionId: targets.collection?.id,
      namedHubId: targets.collection ? undefined : targets.hub?.id,
      currentCollectionId: current,
      find: (id) => containers.find(id),
      collectionsOf: levelOf,
      admits,
      jumbleWritable: jumbleWritable(),
    }),
  );

  // The roles that decide the target: the collection named, the named hub's collections, the
  // collection the reader stands in. Read once each, kept for the session.
  $effect(() => {
    if (!capture.isOpen) return;
    const candidates = [
      targets.collection?.id ? containers.find(targets.collection.id) : undefined,
      ...(targets.hub?.id && !targets.collection ? (levelOf(targets.hub.id) ?? []) : []),
      current ? containers.find(current) : undefined,
    ].filter((each): each is Container => each !== undefined);
    openRoles(candidates);
  });

  const collection = $derived(target.kind === 'collection' ? target.collection : undefined);

  // What a capture there may name: its labels, and the people on its path.
  $effect(() => {
    const id = collection?.id;
    if (!id) return;
    return untrack(() => labels.open(id));
  });

  const offered = (type: string): readonly string[] =>
    ((manifest.value?.item_types ?? []).find((entry) => entry.type === type)?.capabilities as readonly string[] | undefined) ?? [];

  const settings = $derived(collection?.structure?.settings);
  const rules = $derived(collection ? rulesOf(settings, rootTypes(), offered) : undefined);
  /** The levels the door offers, where it named them: under an entry, its child levels. */
  const doorTypes = $derived(door?.types && door.types.length > 0 && (!door.collectionId || door.collectionId === current) ? door.types : undefined);
  const entryType = $derived(chosenType ?? doorTypes?.[0] ?? topLevelOf(settings, rootTypes()));
  /** The entry it goes under, while the door's collection is still the target. */
  const parentId = $derived(isUnderParent && door?.parentId && door.collectionId === collection?.id ? door.parentId : undefined);
  const bucketId = $derived(door?.bucketId && door.collectionId === collection?.id ? door.bucketId : undefined);
  /** Lines and amounts, by the rule the list's own capture line reads too (`captureRulesOf`). */
  const lineRules = $derived(
    collection && entryType ? captureRulesOf(collection.structure, entryType, unitsOf(t('app.entries.capture_units'))) : undefined,
  );
  /** The person took the amount back into the title from its chip, for this text. */
  let keepsWhole = $state(false);
  // *Several lines* splits them wherever the collection would join them.
  const amountRules = $derived(
    lineRules ? { ...lineRules, readsAmount: lineRules.readsAmount && !keepsWhole, splitsLines: lineRules.splitsLines || mode === 'lines' } : undefined,
  );

  const peopleThere = $derived.by((): readonly Named[] => {
    if (!collection) return [];
    const self = actor.account?.id;
    // "Assigned only": a role that creates only what is its own names nobody else (domain-model.md §3.2).
    const onlySelf = itemAccess(manifest.value, roleIn(collection) ?? undefined, 'create') === 'ASSIGNED';
    const ids = onlySelf ? (self ? [self] : []) : people.candidates(pathOf(collection));
    return ids.flatMap((id) => {
      const name = accounts.nameOf(id);
      return name ? [{ id, name }] : [];
    });
  });

  const labelsThere = $derived(collection ? labels.of(collection.id).map((label) => ({ id: label.id, name: label.name })) : []);

  const recognition = $derived(recognitionOf(target, rules, { people: peopleThere, labels: labelsThere }));
  const reading = $derived<Reading>(readLine(text, context, targets, recognition));

  /** One line as Enter reads it, each with the targets it names itself. */
  const readOne = (line: string) => readLine(line, context, readTargets(line, context), recognition);

  /** The amount the first line will carry, shown before Enter so that a wrong split can be taken back. */
  const amount = $derived.by(() => {
    if (!amountRules?.readsAmount) return undefined;
    // Where lines are joined, the joined line is the one that is read.
    const first = amountRules.splitsLines ? (text.split(/\r?\n/u).find((line) => line.trim() !== '') ?? '') : text;
    return readLinesOf(first, amountRules, readOne).lines[0]?.amount;
  });

  /**
   * In a list typed line after line, what was ticked before and holds what is typed (UC-WRK-25
   * check 4): one press puts the same entry back on the list instead of adding it twice.
   */
  const tickedBefore = $derived.by((): readonly WorkItem[] => {
    if (!collection || !lineRules?.splitsLines || mode !== 'one' || text.includes('\n')) return [];
    const typed = readLinesOf(text, amountRules ?? lineRules, readOne).lines[0]?.reading.title ?? '';
    if (typed.trim() === '') return [];
    return narrow(partition(items.inCollection(collection.id), now).recently, typed).slice(0, 4);
  });

  async function putBack(entry: WorkItem) {
    try {
      await items.setCompleted(entry.id, false, crypto.randomUUID());
      announcer.say(t('app.entries.reopened_announced', { title: entry.title }));
      finish('');
    } catch (failure) {
      error = say(failure);
    }
  }

  // ------------------------------------------------------------------------------------------
  // What the field says
  // ------------------------------------------------------------------------------------------

  const hubName = (id: string | null | undefined) => (id ? containers.find(id)?.name : undefined);

  const where = $derived.by(() => {
    if (target.kind === 'collection') {
      const hub = target.hub?.name;
      return hub ? t('app.capture.into_in_hub', { place: target.collection.name, hub }) : t('app.capture.into', { place: target.collection.name });
    }
    if (target.kind === 'jumble') return t('app.capture.into_jumble');
    if (target.kind === 'pending') return t('app.capture.resolving');
    return target.hub ? t('app.capture.nowhere_in_hub', { hub: target.hub.name }) : t('app.capture.nowhere');
  });

  function chipLabel(chip: Chip): string {
    if (chip.kind === 'date' && chip.date) {
      const at = instantOf(chip.date, undefined, context.zone);
      return at ? formatDue(at, messages.locale, context.zone, { allDay: true }) : chip.text;
    }
    if (chip.kind === 'time' && chip.time) {
      return new Intl.DateTimeFormat(messages.locale, { timeStyle: 'short', timeZone: 'UTC' }).format(Date.parse(`1970-01-01T${chip.time}:00Z`));
    }
    if (chip.kind === 'person') return accounts.nameOf(chip.id) ?? chip.text;
    if (chip.kind === 'label') return labelsThere.find((label) => label.id === chip.id)?.name ?? chip.text;
    if (chip.kind === 'hub') return hubName(chip.id) ?? chip.text;
    return containers.find(chip.id ?? '')?.name ?? chip.text;
  }

  const colorOf = (chip: Chip) => (chip.kind === 'label' && collection ? labels.of(collection.id).find((label) => label.id === chip.id)?.color_token : undefined);

  function keepInTitle(chip: Chip) {
    literal = new Set([...literal, literalKey(chip.kind, chip.text)]);
    field?.querySelector('textarea')?.focus();
  }

  // What was recognised, said once typing pauses - never on every keystroke.
  $effect(() => {
    const list = [...(amount ? [amount] : []), ...reading.chips.map((chip) => chipLabel(chip))].join(', ');
    const said = list === '' ? where : t('app.capture.summary', { list, where });
    if (!capture.isOpen || text.trim() === '') return;
    const timer = setTimeout(() => untrack(() => announcer.say(said)), 700);
    return () => clearTimeout(timer);
  });

  // ------------------------------------------------------------------------------------------
  // Enter
  // ------------------------------------------------------------------------------------------

  const writer: Writer = {
    createItem: (body, key) => items.create(body, key),
    submitJumble: (subject, body, key) => jumble.capture(subject, body, key),
  };

  const keysFor = (sent: string, count: number): readonly string[] => {
    if (attempt?.text === sent && attempt.keys.length === count) return attempt.keys;
    attempt = { text: sent, keys: Array.from({ length: count }, () => crypto.randomUUID()) };
    return attempt.keys;
  };

  const say = (failure: unknown) => renderProblem(failure as TransportError, messages).message;

  function finish(rest: string) {
    text = rest;
    literal = new Set();
    keepsWhole = false;
    attempt = undefined;
    if (rest !== '') return;
    // *Keep capturing*: the field empties and keeps the focus for the next line.
    if (keepsOpen) void Promise.resolve().then(() => field?.querySelector('textarea')?.focus());
    else capture.close();
  }

  async function submit() {
    error = undefined;
    now = Date.now();
    if (isBusy || text.trim() === '') return;
    if (target.kind === 'pending') {
      isWaiting = true;
      return;
    }
    if (target.kind === 'nowhere') {
      error = where;
      return;
    }

    isBusy = true;
    try {
      if (target.kind === 'jumble') {
        // The jumble decides nothing (UC-JUM-01): the first line without its `#` and `@` is the
        // subject, and the lines after it the longer text, kept as typed (check 6). This field is
        // the app's one way into the jumble, so it carries what the jumble's own form carried.
        const [first = '', ...after] = text.split(/\r?\n/u);
        const subject = readLine(first, context, targets, NOTHING).title;
        const body = after.join('\n').trim() === '' ? '' : after.join('\n');
        if (subject === '' && body === '') {
          error = t('jumble.entry_empty');
          return;
        }
        const entry = await writer.submitJumble(subject, body, keysFor(text, 1)[0]!);
        finish('');
        offerUndo({ kind: 'jumbled', entryId: entry.id }, undefined, { label: t('app.capture.open'), href: '/jumble' });
        return;
      }

      const place = target.collection;
      if (!amountRules || !entryType) return;
      const { lines, rest } = readLinesOf(text, amountRules, readOne);
      if (lines.length === 0 || lines.some((each) => each.reading.title === '')) {
        error = t('items.title_empty');
        return;
      }
      const bodies = lines.map((each) =>
        bodyOf(each.reading, { collectionId: place.id, type: entryType, parentId, bucketId }, { now, zone: context.zone }, each.amount),
      );
      const outcome = await createAll(writer, bodies, keysFor(text, bodies.length), lines.map((each) => each.line), rest);
      if (outcome.kind !== 'items') return;

      if (outcome.failure !== undefined) {
        error = say(outcome.failure);
        text = outcome.rest;
        attempt = undefined;
      } else {
        finish(outcome.rest);
      }
      if (outcome.created.length === 0) return;
      const only = outcome.created.length === 1 ? outcome.created[0] : undefined;
      offerUndo(
        { kind: 'captured', itemIds: outcome.created.map((item) => item.id), place: place.name },
        undefined,
        { label: t('app.capture.open'), href: only ? `/items/${only.id}` : `/collections/${place.id}` },
      );
    } catch (failure) {
      error = say(failure);
    } finally {
      isBusy = false;
    }
  }

  // Enter waited for the target: it resolves, and the capture goes on - or the wait is bounded.
  $effect(() => {
    if (!isWaiting) return;
    if (target.kind !== 'pending') {
      isWaiting = false;
      void untrack(() => submit());
      return;
    }
    const timer = setTimeout(() => {
      isWaiting = false;
      error = t('app.capture.unresolved');
    }, DEFAULT_TIMEOUT_MS);
    return () => clearTimeout(timer);
  });

  // ------------------------------------------------------------------------------------------
  // Suggestions while a token is typed (design-system.md §11.8)
  // ------------------------------------------------------------------------------------------

  const token = $derived(capture.isOpen ? tokenAt(text, caret) : undefined);

  const candidatesOf = (sigil: Sigil): readonly Offer[] => {
    if (sigil === '@') return context.collections.map((each) => ({ id: each.id, name: each.name, hint: hubName(each.hubId) }));
    if (sigil === '#') return context.hubs.map((each) => ({ id: each.id, name: each.name }));
    if (sigil === '+') return peopleThere.map((each) => ({ id: each.id, name: each.name }));
    return labelsThere.map((each) => ({ id: each.id, name: each.name }));
  };

  /**
   * A token the field already read as a chip needs no list - the name is whole - and one the person
   * took back into the title asked for none.
   */
  const isSettled = (at: Token) =>
    reading.chips.some((chip) => chip.start === at.start) || literal.has(literalKey(SIGILS[at.sigil], `${at.sigil}${at.query}`));
  const offers = $derived(
    token && `${token.sigil}${token.query}@${token.start}` !== dismissed && !isSettled(token)
      ? offersFor(token.query, candidatesOf(token.sigil))
      : [],
  );
  $effect(() => {
    void offers;
    active = 0;
  });

  const listId = $props.id();

  /** A level's name as the collection calls it. */
  const offerLevelName = (type: string) => offerIn(collection?.structure?.settings).level(type).label;

  function readCaret(event: Event) {
    caret = (event.currentTarget as HTMLTextAreaElement | null)?.selectionStart ?? text.length;
  }

  function choose(offer: Offer) {
    if (!token) return;
    const next = pick(text, token, offer.name);
    text = next.text;
    caret = next.caret;
    const area = field?.querySelector('textarea');
    void Promise.resolve().then(() => {
      area?.focus();
      area?.setSelectionRange(next.caret, next.caret);
    });
  }

  /** The hint line's sigils: pressed, the sigil is typed at the caret and its list opens. */
  function insertSigil(sigil: Sigil) {
    const area = field?.querySelector('textarea');
    const at = area?.selectionStart ?? text.length;
    const before = text.slice(0, at);
    const lead = before === '' || /\s$/u.test(before) ? '' : ' ';
    text = `${before}${lead}${sigil}${text.slice(at)}`;
    caret = at + lead.length + 1;
    void Promise.resolve().then(() => {
      area?.focus();
      area?.setSelectionRange(caret, caret);
    });
  }

  function keydown(event: KeyboardEvent) {
    if (offers.length > 0 && token) {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault();
        active = (active + (event.key === 'ArrowDown' ? 1 : offers.length - 1)) % offers.length;
        return;
      }
      if ((event.key === 'Enter' || event.key === 'Tab') && !event.shiftKey && !event.isComposing) {
        event.preventDefault();
        const offer = offers[active];
        if (offer) choose(offer);
        return;
      }
      if (event.key === 'Escape') {
        // The list closes first; the dialog stays.
        event.preventDefault();
        event.stopPropagation();
        dismissed = `${token.sigil}${token.query}@${token.start}`;
        return;
      }
    }
    // Enter captures; Shift+Enter starts a new line, which becomes an entry of its own where the
    // collection splits lines and is joined where it does not.
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
    event.preventDefault();
    void submit();
  }

  // ------------------------------------------------------------------------------------------
  // Where it goes, and in what mode
  // ------------------------------------------------------------------------------------------

  /** The places the chip offers: the jumble where the reader may write there, and every collection they may add to. */
  const places = $derived<readonly { id: string; label: string }[]>([
    ...(jumbleWritable() ? [{ id: 'jumble', label: t('app.capture.place_jumble') }] : []),
    ...liveCollections()
      .filter((each) => admits(each) !== false)
      .map((each) => ({ id: each.id, label: hubName(each.parent_id) ? t('app.capture.place_in_hub', { place: each.name, hub: hubName(each.parent_id) ?? '' }) : each.name })),
  ]);

  const placeLabel = $derived(
    target.kind === 'collection'
      ? target.hub
        ? t('app.capture.place_in_hub', { place: target.collection.name, hub: target.hub.name })
        : target.collection.name
      : target.kind === 'jumble'
        ? t('app.capture.place_jumble')
        : // A place chosen and still being read is named as it was offered, not "nowhere".
          target.kind === 'pending' && current
          ? (places.find((each) => each.id === current)?.label ?? t('app.capture.resolving'))
          : t('app.capture.place_none'),
  );

  /** Import asks STRUCTURE on the collection: offered only to who may shape it (P-05). */
  const mayShapeHere = $derived(collection ? holds(roleIn(collection) ?? undefined, 'STRUCTURE').status === 'permitted' : false);

  /** A mode that is a screen of its own: the collection's page opens it, with its own handler. */
  function chooseMode(next: Mode) {
    if ((next === 'template' || next === 'import') && collection) {
      const id = collection.id;
      capture.close();
      commands.intend({ containerId: id, act: next });
      onnavigate?.(`/collections/${id}`);
      return;
    }
    mode = next;
    void Promise.resolve().then(() => field?.querySelector('textarea')?.focus());
  }

  // A door sets what it knows when the field opens; the person's own choices start fresh.
  $effect(() => {
    if (!capture.isOpen) return;
    const opened = door;
    untrack(() => {
      chosen = undefined;
      chosenType = undefined;
      isUnderParent = true;
      mode = opened?.mode === 'lines' ? 'lines' : 'one';
      // Opened from anywhere on a list typed line after line, the field stays for the next line.
      keepsOpen = opened?.keepsOpen ?? lineRules?.splitsLines ?? false;
    });
  });

  // A fresh field each time it opens.
  $effect(() => {
    if (capture.isOpen) return;
    untrack(() => {
      text = '';
      literal = new Set();
      keepsWhole = false;
      error = undefined;
      isWaiting = false;
      attempt = undefined;
    });
  });
</script>

<Dialog title={t('app.capture.title')} dismissLabel={t('app.capture.close')} isOpen={capture.isOpen} onClose={() => capture.close()}>
  <form
    class="capture"
    bind:this={field}
    onsubmit={(event) => {
      event.preventDefault();
      void submit();
    }}
  >
    <!-- Where it goes, as chips at the top rather than a sentence under the field: the place, the
         entry it goes under, the level - each one press to change. -->
    <div class="where" role="group" aria-label={t('app.capture.where_group')}>
      <Menu
        label={t('app.capture.place_choose')}
        items={places.map((each) => ({ id: each.id, label: each.label }))}
        onselect={(id) => {
          chosen = id;
          field?.querySelector('textarea')?.focus();
        }}
      >
        {#snippet trigger(props)}
          <button type="button" class="chip" data-chip="place" {...props}>
            <span class="chip-key">{t('app.capture.place_key')}</span><span class="chip-value">{placeLabel}</span><span aria-hidden="true">▾</span>
          </button>
        {/snippet}
      </Menu>
      {#if parentId && door?.parentTitle}
        <span class="chip" data-chip="parent">
          <span class="chip-key">{t('app.capture.parent_key')}</span><span class="chip-value">{door.parentTitle}</span>
          <button type="button" class="chip-x" aria-label={t('app.capture.parent_remove', { title: door.parentTitle })} onclick={() => (isUnderParent = false)}>×</button>
        </span>
      {/if}
      {#if doorTypes && doorTypes.length > 1}
        <Menu
          label={t('app.capture.kind_choose')}
          items={doorTypes.map((type) => ({ id: type, label: offerLevelName(type) }))}
          onselect={(type) => (chosenType = type)}
        >
          {#snippet trigger(props)}
            <button type="button" class="chip" data-chip="kind" {...props}>
              <span class="chip-key">{t('app.capture.kind_key')}</span><span class="chip-value">{offerLevelName(entryType ?? '')}</span><span aria-hidden="true">▾</span>
            </button>
          {/snippet}
        </Menu>
      {/if}
    </div>
    <Textarea
      label={t('app.capture.field')}
      rows={mode === 'lines' ? 4 : 1}
      bind:value={text}
      placeholder={mode === 'lines' ? t('app.capture.placeholder_lines') : t('app.capture.placeholder')}
      hint={isWaiting ? t('app.capture.resolving') : target.kind === 'nowhere' || target.kind === 'jumble' ? where : undefined}
      {error}
      autocomplete="off"
      aria-autocomplete="list"
      aria-controls={offers.length > 0 ? `${listId}-offers` : undefined}
      aria-activedescendant={offers.length > 0 ? `${listId}-offer-${active}` : undefined}
      onkeydown={keydown}
      onkeyup={readCaret}
      onclick={readCaret}
      oninput={(event: Event) => {
        now = Date.now();
        error = undefined;
        readCaret(event);
      }}
    />
    {#if offers.length > 0 && token}
      <!-- What the token may name, as it is typed: arrows move, Enter or Tab take, Escape closes. -->
      <ul class="offers" id={`${listId}-offers`} role="listbox" aria-label={t(`app.capture.offers_${SIGILS[token.sigil]}`)}>
        {#each offers as offer, index (offer.id)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            id={`${listId}-offer-${index}`}
            role="option"
            aria-selected={index === active}
            class="offer"
            onmousedown={(event) => event.preventDefault()}
            onclick={() => choose(offer)}
          >
            <span class="offer-name">{offer.name}</span>
            {#if offer.hint}<span class="offer-hint">{offer.hint}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
    {#if collection && mode === 'one' && !text.includes('\n')}
      <!-- What already looks like it, while it is typed (UC-AI-04 check 7): nothing when nothing is. -->
      <Lookalikes title={reading.title} />
    {/if}
    {#if tickedBefore.length > 0}
      <!-- Already on the list and ticked: the same entry back, rather than a second of it. -->
      <section class="before" aria-label={t('app.capture.ticked_before')}>
        <h3>{t('app.capture.ticked_before')}</h3>
        <ul>
          {#each tickedBefore as entry (entry.id)}
            <li><Button size="sm" tone="subtle" icon="rotate-ccw" onclick={() => void putBack(entry)}>{t('app.capture.put_back', { title: entry.title })}</Button></li>
          {/each}
        </ul>
      </section>
    {/if}
    {#if amount || reading.chips.length > 0}
      <ul class="chips" aria-label={t('app.capture.recognised')}>
        {#if amount}
          <!-- The split Enter will make, before it makes it: removing the chip keeps the number in
               the title (UC-WRK-26 check 2, the list's own rule). -->
          <li>
            <LabelChip
              name={amount}
              description={t('app.capture.chip_kind', { kind: 'amount' })}
              removeLabel={t('app.capture.keep_in_title', { kind: 'amount', text: amount })}
              onRemove={() => {
                keepsWhole = true;
                field?.querySelector('textarea')?.focus();
              }}
            />
          </li>
        {/if}
        {#each reading.chips as chip (`${chip.kind}:${chip.start}`)}
          <li>
            <LabelChip
              name={chipLabel(chip)}
              colorToken={colorOf(chip)}
              description={t('app.capture.chip_kind', { kind: chip.kind })}
              removeLabel={t('app.capture.keep_in_title', { kind: chip.kind, text: chip.text })}
              onRemove={() => keepInTitle(chip)}
            />
          </li>
        {/each}
      </ul>
    {/if}
    <!-- The tokens the field reads, said rather than remembered; each one types its sigil. -->
    <div class="hint-line" role="group" aria-label={t('app.capture.tokens')}>
      {#each Object.entries(SIGILS) as [sigil, kind] (sigil)}
        <button type="button" class="token" onclick={() => insertSigil(sigil as Sigil)}><b>{sigil}</b> {t(`app.capture.token_${kind}`)}</button>
      {/each}
      <span class="token-plain">{t('app.capture.token_date')}</span>
    </div>
    <!-- The ways of adding, as one dialog's modes: one entry, several lines, from a template, a file. -->
    <div class="modes" role="group" aria-label={t('app.capture.modes')}>
      <button type="button" class="mode" aria-pressed={mode === 'one'} onclick={() => chooseMode('one')}>{t('app.capture.mode_one')}</button>
      <button type="button" class="mode" aria-pressed={mode === 'lines'} onclick={() => chooseMode('lines')}>{t('app.capture.mode_lines')}</button>
      {#if collection}
        <button type="button" class="mode" onclick={() => chooseMode('template')}>{t('app.capture.mode_template')}</button>
        {#if mayShapeHere}
          <button type="button" class="mode" onclick={() => chooseMode('import')}>{t('app.capture.mode_import')}</button>
        {/if}
      {/if}
    </div>
    <div class="actions">
      <Checkbox label={t('app.capture.keep_open')} bind:checked={keepsOpen} />
      <Button type="submit" tone="primary" icon="plus" {isBusy} busyLabel={t('app.capture.submitting')}>{t('app.capture.submit')}</Button>
    </div>
  </form>
  {#if collection && actor.account && (offersMakeEntries(text, { aiOn: manifest.value?.features?.ai_suggestions === true, intoCollection: true }) || suggestions.askingOf(actor.account.id)?.operation === 'entries')}
    <MakeEntries
      accountId={actor.account.id}
      {collection}
      {text}
      ondone={() => {
        // *Take all* closes the field (UC-AI-09 check 3), whether or not it was kept open.
        finish('');
        capture.close();
      }}
    />
  {/if}
</Dialog>

<style>
  .capture { display: flex; flex-direction: column; gap: var(--sp-150); }
  .before h3 { margin: 0 0 var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .before ul { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .actions { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-150); }

  /* The place, the parent and the level, as chips that open their choice. */
  .where { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    max-inline-size: 100%;
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
  }

  button.chip { cursor: pointer; }
  button.chip:hover { background: var(--bg-surface-hover); }

  .chip-key { color: var(--text-secondary); }
  .chip-value { font-weight: var(--fw-medium); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }

  .chip-x {
    padding: 0 var(--sp-050);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
  }

  .chip-x:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  /* What the token may name, under the field. */
  .offers {
    margin: 0;
    padding: var(--sp-050);
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-overlay);
  }

  .offer {
    display: flex;
    align-items: baseline;
    gap: var(--sp-150);
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-sm);
    cursor: pointer;
  }

  .offer[aria-selected='true'] { background: var(--accent-primary-subtle); }

  .offer-name { font-weight: var(--fw-medium); }
  .offer-hint { margin-inline-start: auto; color: var(--text-secondary); font-size: var(--fs-075); }

  /* The tokens the field reads, each a button that types its sigil. */
  .hint-line { display: flex; flex-wrap: wrap; gap: var(--sp-050); align-items: center; }

  .token,
  .token-plain {
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
  }

  .token {
    border: var(--bw-hairline) dashed var(--border-default);
    background: transparent;
    cursor: pointer;
  }

  .token:hover { color: var(--text-primary); border-style: solid; }
  .token b { color: var(--text-brand); font-family: var(--font-mono); }

  .modes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    padding-block-start: var(--sp-100);
  }

  .mode {
    padding: var(--sp-050) var(--sp-150);
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .mode:hover { background: var(--bg-surface-hover); color: var(--text-primary); }
  .mode[aria-pressed='true'] { background: var(--accent-primary-subtle); color: var(--accent-primary); font-weight: var(--fw-medium); }

  .chip:focus-visible,
  .chip-x:focus-visible,
  .token:focus-visible,
  .mode:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
