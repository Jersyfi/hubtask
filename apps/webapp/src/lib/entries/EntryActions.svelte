<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What an entry's menu does, once for a screen (UC-WRK-28 check 1).
  //
  // Every surface hands the chosen item here with the entry: the dialogs an item opens - dates,
  // person, labels, share, another collection, a copy - are drawn once, and the writes go through
  // the same calls whichever surface the menu was opened on. A surface that ranks an entry within
  // something of its own (the board's column) answers those items itself before calling `run`;
  // everything else, the moves within the entry's level included, is answered here from the level
  // the surface hands in or the one read when the menu opened (D6).

  import { untrack } from 'svelte';

  import { Button, Dialog, rankTarget, type RankCommand } from '@hubtask/design-system/components';
  import type { DroppedReference, WorkItem } from '@hubtask/sync-engine';

  import DuePanel from './DuePanel.svelte';
  import DuplicateDialog from './DuplicateDialog.svelte';
  import LabelsPanel from './LabelsPanel.svelte';
  import MoveDialog from './MoveDialog.svelte';
  import type { EntryLevel } from './entryMenu.ts';
  import { offerUndo } from './undo.svelte.ts';

  import { announcer } from '../announce.svelte.ts';
  import { celebration } from '../celebration.svelte.ts';
  import { offerIn } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { anchorFor } from '../data/rank.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import AssigneePanel from '../people/AssigneePanel.svelte';
  import MembersDialog from '../people/MembersDialog.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** Opening an entry is the surface's: beside it from `large` where it has a pane, else its page. */
    onopen: (itemId: string) => void;
    /** Where a copy or a link leads. */
    onnavigate?: (path: string) => void;
    /** *Select*, on a surface whose bulk bar acts on what is picked. */
    onselect?: (itemId: string) => void;
  }

  const { onopen, onnavigate, onselect }: Props = $props();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let dropped = $state<{ title: string; references: readonly DroppedReference[] } | undefined>(undefined);

  let dating = $state<WorkItem | undefined>(undefined);
  let assigning = $state<WorkItem | undefined>(undefined);
  let labelling = $state<WorkItem | undefined>(undefined);
  let sharing = $state<WorkItem | undefined>(undefined);
  let isSharing = $state(false);
  let moving = $state<WorkItem | undefined>(undefined);
  let isMoving = $state(false);
  let duplicating = $state<WorkItem | undefined>(undefined);

  /** Where an entry sits, for the people panels: its collection and that collection's hub. */
  const pathOf = (item: WorkItem) => ({
    hubId: containers.find(item.collection_id)?.parent_id ?? undefined,
    collectionId: item.collection_id,
    itemId: item.id,
  });

  /** The siblings an entry is ranked among, in the order the server keeps: its parent's, or its collection's top level. */
  function siblingsOf(item: WorkItem): readonly WorkItem[] | undefined {
    if (item.parent_id) return items.hasChildrenOf(item.parent_id) ? items.childrenOf(item.parent_id) : undefined;
    const level = items.stateOf(`container:${item.collection_id}`);
    return level?.status === 'ready' ? items.inCollection(item.collection_id) : undefined;
  }

  /**
   * Reads the entry's level, for a surface that does not hold it (search, the timeline): called
   * when its menu opens. The read stays open while the screen does; reading it again is free.
   */
  const opened = new Map<string, () => void>();
  export function readLevel(item: WorkItem) {
    const key = item.parent_id ?? `collection:${item.collection_id}`;
    if (opened.has(key)) return;
    untrack(() => {
      opened.set(key, item.parent_id ? items.openChildren(item.parent_id) : items.openCollection(item.collection_id));
      if (item.parent_id && !items.find(item.parent_id)) items.openChildren(item.parent_id);
    });
  }
  $effect(() => () => {
    for (const stop of opened.values()) stop();
    opened.clear();
  });

  /** The entry the item would move inside: the sibling above it. */
  const aboveOf = (item: WorkItem, siblings: readonly WorkItem[]) => siblings[siblings.findIndex((each) => each.id === item.id) - 1];

  /** The parent an entry moves out to: its parent's parent, or `null` for the collection's top level. */
  function outOf(item: WorkItem): { parentId: string | null } | undefined {
    if (!item.parent_id) return undefined;
    const parent = items.find(item.parent_id);
    return parent ? { parentId: parent.parent_id ?? null } : undefined;
  }

  /**
   * The level as the menu needs it, or nothing while it is unknown. `inside` and `out` ask the
   * manifest with the stored depths (UX-08 D5): the depth below the entry above, and below the
   * destination.
   */
  export function levelOf(item: WorkItem, given?: readonly WorkItem[]): EntryLevel | undefined {
    const siblings = given ?? siblingsOf(item);
    if (!siblings) return undefined;
    const index = siblings.findIndex((each) => each.id === item.id);
    if (index < 0) return undefined;
    const above = aboveOf(item, siblings);
    // The collection's answers, as the list's own menu gives them: its structure narrows what may
    // sit where, and names its levels (UC-WRK-23 checks 4 and 6).
    const offer = offerIn(containers.find(item.collection_id)?.structure?.settings);
    const inside = above ? offer.acceptsChild(above.type, item.type, Math.max(0, (above.depth ?? 1) - 1)) : undefined;
    let outReason: string | undefined;
    if (!item.parent_id) outReason = t('app.move.already_top');
    else {
      const out = outOf(item);
      const destination = out === undefined ? undefined : out.parentId === null ? null : items.find(out.parentId);
      const verdict = offer.placementOf(
        item.type,
        destination === null ? null : destination?.depth === undefined ? undefined : { type: destination.type, depth: destination.depth },
      );
      outReason = verdict.status === 'refused' ? offer.refusal(verdict) : undefined;
    }
    return {
      index,
      count: siblings.length,
      aboveTitle: above?.title,
      insideReason: !above
        ? t('app.move.nothing_above')
        : inside?.status === 'refused'
          ? offer.refusal(inside)
          : inside?.status === 'permitted'
            ? undefined
            : t('app.move.moving'),
      outReason,
    };
  }

  /** One chosen item, for one entry. `siblings` where the surface holds the level itself. */
  export async function run(item: WorkItem, choice: string, siblings?: readonly WorkItem[]) {
    failure = undefined;
    try {
      switch (choice) {
        case 'open':
          return onopen(item.id);
        case 'select':
          return onselect?.(item.id);
        case 'complete':
        case 'reopen': {
          const answered = await items.setCompleted(item.id, choice === 'complete', crypto.randomUUID());
          announcer.say(
            t(choice === 'complete' ? 'app.entries.completed_announced' : 'app.entries.reopened_announced', { title: item.title }),
          );
          if (answered.completion?.is_completed) void celebration.celebrate(answered);
          return;
        }
        case 'dates':
          dating = item;
          return;
        case 'person':
          assigning = item;
          return;
        case 'labels':
          labelling = item;
          return;
        case 'share':
          sharing = item;
          isSharing = true;
          return;
        case 'elsewhere':
          moving = item;
          return;
        case 'duplicate':
          duplicating = item;
          return;
        case 'copy-link':
          await navigator.clipboard.writeText(`${location.origin}/items/${item.id}`);
          announcer.say(t('app.entry_menu.link_copied'));
          return;
        case 'archive':
          // At once, and the message offers the way back (UC-WRK-28 check 3); it is also what
          // the reader hears.
          await items.setArchived(item.id, true, crypto.randomUUID());
          offerUndo({ kind: 'archive', itemId: item.id, title: item.title });
          return;
        case 'unarchive':
          await items.setArchived(item.id, false, crypto.randomUUID());
          announcer.say(t('app.entries.unarchived_announced', { title: item.title }));
          return;
        case 'trash':
          await items.trash(item.id, item.version);
          offerUndo({ kind: 'trash', itemId: item.id, title: item.title });
          return;
        case 'inside': {
          const level = siblings ?? siblingsOf(item);
          const above = level ? aboveOf(item, level) : undefined;
          if (above) await move(item, { parentId: above.id });
          return;
        }
        case 'out': {
          const out = outOf(item);
          if (out) await move(item, out.parentId === null ? { parentId: null, collectionId: item.collection_id } : out);
          return;
        }
        case 'up':
        case 'down':
        case 'top':
        case 'bottom': {
          const level = siblings ?? siblingsOf(item);
          if (!level) return;
          const index = level.findIndex((each) => each.id === item.id);
          const target = rankTarget(choice as RankCommand, index, level.length);
          if (target === null) return;
          await items.reorder(item.id, anchorFor(level, item.id, target), item.version, crypto.randomUUID());
          announcer.say(t('app.rank.announced', { title: item.title, position: target + 1, count: level.length }));
          return;
        }
      }
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  /** `:move`, and what it reports it left behind (I-W6) - shown, never dropped in silence. */
  async function move(item: WorkItem, destination: { parentId: string | null; collectionId?: string }) {
    const result = await items.move(item.id, destination, item.version, crypto.randomUUID());
    dropped = { title: item.title, references: result.dropped_references ?? [] };
    announcer.say(t('app.move.announced', { title: item.title }));
  }

  /** Every collection the workspace holds, read when the picker opens (the list's reason). */
  $effect(() => {
    if (!moving) return;
    const hubs = containers.hubs.map((hub) => hub.id);
    return untrack(() => {
      const stops = hubs.map((hubId) => containers.openLevel(hubId));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  const destinations = $derived(
    moving === undefined
      ? []
      : containers.hubs.flatMap((hub) =>
          containers
            .collectionsOf(hub.id)
            // Not where it is, and not an archived one (I-C3): a move the server refuses.
            .filter((collection) => collection.id !== moving?.collection_id && !collection.effective_archived)
            .map((collection) => ({
              value: collection.id,
              label: t('app.move.destination_option', { hub: hub.name, collection: collection.name }),
            })),
        ),
  );

  async function moveElsewhere(collectionId: string) {
    const item = moving;
    if (!item) return;
    isMoving = true;
    failure = undefined;
    try {
      await move(item, { parentId: null, collectionId });
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isMoving = false;
      moving = undefined;
    }
  }
</script>

{#if failure}
  <p class="failure" role="alert">{failure.message}</p>
{/if}
<!-- I-W6: what a move could not take along is reported rather than dropped in silence, and showing
     it is what makes the report worth having. -->
{#if dropped && dropped.references.length > 0}
  <div class="dropped">
    <p>{t('app.move.dropped', { title: dropped.title })}</p>
    <ul>
      {#each dropped.references as reference, index (`${reference.kind}-${reference.id}-${index}`)}
        <li><strong>{t(`app.move.dropped_kind.${reference.kind}`)}</strong> {t(reference.code)}</li>
      {/each}
    </ul>
    <div>
      <Button size="sm" tone="secondary" onclick={() => (dropped = undefined)}>{t('app.dismiss')}</Button>
    </div>
  </div>
{/if}

{#if dating}
  <Dialog isOpen={true} title={t('app.due.title')} dismissLabel={t('app.workspace.cancel')} onClose={() => (dating = undefined)}>
    <DuePanel item={items.find(dating.id) ?? dating} />
  </Dialog>
{/if}

{#if assigning}
  <Dialog isOpen={true} title={t('app.people.assignee')} dismissLabel={t('app.workspace.cancel')} onClose={() => (assigning = undefined)}>
    <AssigneePanel item={items.find(assigning.id) ?? assigning} path={pathOf(assigning)} />
  </Dialog>
{/if}

{#if labelling}
  <Dialog isOpen={true} title={t('app.entry_menu.labels')} dismissLabel={t('app.workspace.cancel')} onClose={() => (labelling = undefined)}>
    <LabelsPanel item={items.find(labelling.id) ?? labelling} />
  </Dialog>
{/if}

{#if sharing}
  <MembersDialog
    bind:isOpen={isSharing}
    title={t('app.people.share_title')}
    scope={{ scopeType: 'ITEM', scopeId: sharing.id }}
    path={pathOf(sharing)}
  />
{/if}

{#if moving}
  <MoveDialog
    isOpen={true}
    title={t('app.entry_menu.label', { title: moving.title })}
    label={t('app.move.destination')}
    placeholder={t('app.move.choose')}
    options={destinations}
    emptyLabel={t('app.move.no_destination')}
    warning={t('app.move.leaves_behind')}
    confirmLabel={t('app.move.confirm')}
    busyLabel={t('app.move.moving')}
    cancelLabel={t('app.entries.cancel')}
    chooseFirstLabel={t('app.move.choose_first')}
    isBusy={isMoving}
    onmove={(destination) => moveElsewhere(destination)}
  />
{/if}

<DuplicateDialog
  item={duplicating}
  hubId={duplicating ? (containers.find(duplicating.collection_id)?.parent_id ?? undefined) : undefined}
  onclose={() => (duplicating = undefined)}
  onopened={(itemId) => {
    duplicating = undefined;
    onnavigate?.(`/items/${itemId}`);
  }}
/>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .dropped {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    max-width: 64ch;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    font-size: var(--fs-075);
  }

  .dropped p { margin: 0; }

  .dropped ul {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    margin: 0;
    padding-inline-start: var(--sp-200);
  }
</style>
