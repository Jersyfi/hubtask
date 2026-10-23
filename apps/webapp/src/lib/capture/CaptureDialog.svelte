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

  import { Button, Dialog, LabelChip, Textarea } from '@hubtask/design-system/components';
  import { DEFAULT_TIMEOUT_MS, type Container, type TransportError, type WorkItem } from '@hubtask/sync-engine';

  import { capture } from './capture.svelte.ts';
  import { offersMakeEntries } from './entries.ts';
  import MakeEntries from './MakeEntries.svelte';
  import { NOTHING, literalKey, readLine, readTargets, type Chip, type Context, type Named, type Reading } from './parse.ts';
  import { admits, jumbleWritable, liveCollections, liveHubs, openLevels, openRoles, pathOf, roleIn } from './places.svelte.ts';
  import { bodyOf, createAll, linesOf, type Writer } from './submit.ts';
  import { recognitionOf, resolveTarget, rulesOf, topLevelOf, type Target } from './target.ts';
  import { vocabularyOf } from './words.ts';
  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { itemAccess } from '../data/capability.ts';
  import { rootTypes } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { archivalOf } from '../data/containers.ts';
  import { engine } from '../data/engine.ts';
  import { itemPath } from '../data/item.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { jumble } from '../data/jumble.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { suggestions } from '../data/suggestions.svelte.ts';
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
  }

  const { currentCollectionId, currentItemId, currentHubId }: Props = $props();

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

  const current = $derived(currentCollectionId ?? itemCollectionId);
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
  const entryType = $derived(topLevelOf(settings, rootTypes()));

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
    const list = reading.chips.map((chip) => chipLabel(chip)).join(', ');
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
    submitJumble: (subject, key) => jumble.capture(subject, '', key),
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
    attempt = undefined;
    if (rest === '') capture.close();
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
        // The jumble decides nothing (UC-JUM-01): the line without its `#` and `@`.
        const subject = readLine(text, context, targets, NOTHING).title;
        if (subject === '') {
          error = t('jumble.entry_empty');
          return;
        }
        const entry = await writer.submitJumble(subject, keysFor(text, 1)[0]!);
        finish('');
        offerUndo({ kind: 'jumbled', entryId: entry.id }, undefined, { label: t('app.capture.open'), href: '/jumble' });
        return;
      }

      const place = target.collection;
      if (!rules || !entryType) return;
      const { lines, rest } = linesOf(text, rules.splitsLines);
      const readings = lines.map((line) => readLine(line, context, readTargets(line, context), recognition));
      if (readings.length === 0 || readings.some((each) => each.title === '')) {
        error = t('items.title_empty');
        return;
      }
      const bodies = readings.map((each) => bodyOf(each, { collectionId: place.id, type: entryType }, { now, zone: context.zone }));
      const outcome = await createAll(writer, bodies, keysFor(text, bodies.length), lines, rest);
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

  function keydown(event: KeyboardEvent) {
    // Enter captures; Shift+Enter starts a new line, which becomes an entry of its own where the
    // collection splits lines and is joined where it does not.
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
    event.preventDefault();
    void submit();
  }

  // A fresh field each time it opens.
  $effect(() => {
    if (capture.isOpen) return;
    untrack(() => {
      text = '';
      literal = new Set();
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
    <Textarea
      label={t('app.capture.field')}
      rows={1}
      bind:value={text}
      placeholder={t('app.capture.placeholder')}
      hint={isWaiting ? t('app.capture.resolving') : where}
      {error}
      autocomplete="off"
      onkeydown={keydown}
      oninput={() => {
        now = Date.now();
        error = undefined;
      }}
    />
    {#if reading.chips.length > 0}
      <ul class="chips" aria-label={t('app.capture.recognised')}>
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
    <div class="actions">
      <Button type="submit" tone="primary" icon="plus" {isBusy} busyLabel={t('app.capture.submitting')}>{t('app.capture.submit')}</Button>
    </div>
  </form>
  {#if collection && actor.account && (offersMakeEntries(text, { aiOn: manifest.value?.features?.ai_suggestions === true, intoCollection: true }) || suggestions.askingOf(actor.account.id)?.operation === 'entries')}
    <MakeEntries accountId={actor.account.id} collection={collection} {text} ondone={() => finish('')} />
  {/if}
</Dialog>

<style>
  .capture { display: flex; flex-direction: column; gap: var(--sp-150); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .actions { display: flex; justify-content: flex-end; }
</style>
