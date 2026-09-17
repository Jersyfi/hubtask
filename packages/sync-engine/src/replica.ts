// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The replica: the local copy of the workspace, written from what the server sends and from
// nothing else (offline-sync.md §2, §3).
//
// **Nothing here merges.** A record is the server's decision, already taken: an initial
// synchronisation carries whole objects, a delta carries the field that moved under the clock
// that decided it, a set change carries the element and whether it is in or out. Applying that to
// the stored copy is transcription. What would be a merge - two values for one field, and a rule
// choosing - happens on the server when a push meets it, and reaches this store as one more
// record like any other (ADR-0021, §4).
//
// Records are held by collection, keyed on the identifier, and a collection is the entity's name
// as the server sends it - `containers`, `items`, and for an entity this build has never heard of
// its own name, unchanged. That is rule 7 of §9 from the store's side: a record is kept as it came,
// fields this client does not read included, so that a later version of this client, or a push
// that sends the object back, finds them where the server put them.

import type { Clock, Storage } from './ports.ts';
import type { ChangeRecord, StoredRecord } from './schema.ts';
import type { DeviceIdentity } from './device.ts';
import type { HlcReading } from './hlc.ts';

/** The store's own collection: the cursor, the device, the clock. Never a server entity. */
export const META = 'meta';

/** Where the delta left off, and what the server said beside it. */
export interface SyncPosition {
  readonly cursor?: string;
  /** The window the server named on the last pull or snapshot: how long a cursor stays usable. */
  readonly tombstoneWindowDays?: number;
  /** The server's own time on that answer, for a client that wants to show how far behind it is. */
  readonly serverTime?: string;
  /** When this position was written, by the injected clock. */
  readonly at: number;
}

/**
 * The names the server uses for the entities the schema types name, both spellings of an entry
 * included (`live.ts` explains the two). Anything else is held under its own name.
 */
export function collectionOf(entity: string): string {
  switch (entity) {
    case 'item':
    case 'work_item':
      return 'items';
    case 'container':
      return 'containers';
    case 'bucket':
      return 'buckets';
    case 'label':
      return 'labels';
    case 'comment':
      return 'comments';
    default:
      return entity;
  }
}

type Document = Record<string, unknown>;

/** A set change as a record carries it: which set, which element, in or out. */
function setChangeOf(payload: Document): { set: string; element: string; op: 'add' | 'remove' } | undefined {
  const { set, element_id: element, op } = payload;
  if (typeof set !== 'string' || typeof element !== 'string') return undefined;
  if (op !== 'add' && op !== 'remove') return undefined;
  return { set, element, op };
}

export class Replica {
  readonly #storage: Storage;
  readonly #clock: Clock;

  constructor(storage: Storage, clock: Clock) {
    this.#storage = storage;
    this.#clock = clock;
  }

  /** The store beneath, for the engine's own reads. */
  get store(): Storage {
    return this.#storage;
  }

  // ------------------------------------------------------------------ what the store knows

  async position(): Promise<SyncPosition | undefined> {
    return this.#storage.get<SyncPosition>(META, 'position');
  }

  async hold(position: Omit<SyncPosition, 'at'>): Promise<void> {
    await this.#storage.put<SyncPosition>(META, 'position', { ...position, at: this.#clock.now() });
  }

  async device(): Promise<DeviceIdentity | undefined> {
    return this.#storage.get<DeviceIdentity>(META, 'device');
  }

  async holdDevice(device: DeviceIdentity): Promise<void> {
    await this.#storage.put(META, 'device', device);
  }

  async hlc(): Promise<HlcReading | undefined> {
    return this.#storage.get<HlcReading>(META, 'hlc');
  }

  async holdHlc(reading: HlcReading): Promise<void> {
    await this.#storage.put(META, 'hlc', reading);
  }

  // ------------------------------------------------------------------ what the server sent

  /** One record, transcribed. */
  async apply(record: ChangeRecord): Promise<void> {
    switch (record.op) {
      case 'UPSERT':
        return this.#upsert(record);
      case 'DELETE':
        return this.#delete(record);
      case 'ACCESS_REVOKED':
        // What a subtree deletion does, at the root the record names (§6). The root is the
        // container the grant covered; the record's own identity is that container too where
        // the server spelled it so.
        return this.#deleteSubtree(record.container_id ?? record.entity_id);
      default:
        // An operation this build does not know. Kept out of the store rather than guessed at:
        // the cursor still moves past it, and a later build reads the server again.
        return;
    }
  }

  async #upsert(record: ChangeRecord): Promise<void> {
    const collection = collectionOf(record.entity);
    const id = record.entity_id;
    const payload = (record.payload ?? undefined) as Document | undefined;
    const existing = await this.#storage.get<StoredRecord<Document>>(collection, id);
    const seenAt = this.#clock.now();

    if (payload === undefined) {
      // Nothing moved that the record spells out - a touch. The copy is kept and marked seen.
      if (existing) await this.#storage.put(collection, id, { ...existing, seenAt });
      return;
    }

    const setChange = setChangeOf(payload);
    if (setChange) {
      // A set travels element by element beside the entry, never inside its document (§4.2): the
      // document the server sends for an entry carries no labels, and a whole object arriving
      // later must not take the elements with it.
      const sets = { ...(existing?.sets ?? {}) };
      const held = new Set(sets[setChange.set] ?? []);
      if (setChange.op === 'add') held.add(setChange.element);
      else held.delete(setChange.element);
      sets[setChange.set] = [...held];
      await this.#storage.put<StoredRecord<Document>>(collection, id, {
        id,
        document: existing?.document ?? { id },
        version: existing?.version ?? 0,
        seenAt,
        sets,
        hlc: record.hlc ?? existing?.hlc,
      });
      return;
    }

    const whole = payload.id === id;
    const document: Document = whole ? payload : { ...(existing?.document ?? { id }), ...payload };
    const version = typeof document.version === 'number' ? document.version : (existing?.version ?? 0);
    await this.#storage.put<StoredRecord<Document>>(collection, id, {
      id,
      document,
      version,
      seenAt,
      ...(existing?.sets ? { sets: existing.sets } : {}),
      hlc: record.hlc ?? existing?.hlc,
    });
  }

  async #delete(record: ChangeRecord): Promise<void> {
    const collection = collectionOf(record.entity);
    if (collection === 'containers') {
      await this.#deleteSubtree(record.entity_id);
      return;
    }
    await this.#storage.delete(collection, record.entity_id);
    if (collection === 'items') await this.#deleteUnderItems(new Set([record.entity_id]));
  }

  /**
   * Everything under a container, by the tree the containers describe: the container, every
   * container below it, every entry in any of them, and every record that points at one of those
   * entries (§4.2's subtree deletion, §6's revocation).
   */
  async #deleteSubtree(rootId: string): Promise<void> {
    const containers = await this.#storage.all<StoredRecord<Document>>('containers');
    const gone = new Set<string>([rootId]);
    let grew = true;
    while (grew) {
      grew = false;
      for (const container of containers) {
        const parent = container.document.parent_id;
        if (!gone.has(container.id) && typeof parent === 'string' && gone.has(parent)) {
          gone.add(container.id);
          grew = true;
        }
      }
    }
    for (const id of gone) await this.#storage.delete('containers', id);

    const items = new Set<string>();
    for (const name of await this.#storage.collections()) {
      if (name === META || name === 'containers') continue;
      for (const held of await this.#storage.all<StoredRecord<Document>>(name)) {
        const { collection_id: collectionId, container_id: containerId } = held.document;
        if ((typeof collectionId === 'string' && gone.has(collectionId)) || (typeof containerId === 'string' && gone.has(containerId))) {
          await this.#storage.delete(name, held.id);
          if (name === 'items') items.add(held.id);
        }
      }
    }
    await this.#deleteUnderItems(items);
  }

  /** Every record that names one of these entries as its own: comments, reminders, rules. */
  async #deleteUnderItems(items: ReadonlySet<string>): Promise<void> {
    if (items.size === 0) return;
    for (const name of await this.#storage.collections()) {
      if (name === META || name === 'items' || name === 'containers') continue;
      for (const held of await this.#storage.all<StoredRecord<Document>>(name)) {
        const itemId = held.document.item_id;
        if (typeof itemId === 'string' && items.has(itemId)) await this.#storage.delete(name, held.id);
      }
    }
  }

  /**
   * Empties the replica and forgets the position, and keeps the device: what
   * `sync.cursor_too_old` asks for (§7, §9.4). The device is the same device - it is the copy
   * that is stale, not the identity - and a store that minted a new one on every resync would
   * leave a row per resync in the server's list.
   */
  async empty(): Promise<void> {
    for (const name of await this.#storage.collections()) {
      if (name === META) continue;
      for (const held of await this.#storage.all<{ id: string }>(name)) await this.#storage.delete(name, held.id);
    }
    await this.#storage.delete(META, 'position');
  }
}
