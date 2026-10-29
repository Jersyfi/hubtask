// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A board's objects in its live document. Each object is a `Y.Map` under the document's
 * `elements` map, keyed by its identifier, so two people drawing at once each keep their objects
 * and a field changed by one merges with another changed by the other. Drawing order is a number
 * per object (`z`), ties broken by identifier, so every device draws the same order.
 *
 * The projection - `scene()` - is the contract's `WhiteboardElement[]` the board saves beside the
 * live document; `seed()` writes one into an empty document, the first writer's job.
 */

import * as Y from 'yjs';

export type ElementKind = 'STICKY' | 'SHAPE' | 'TEXT' | 'CONNECTOR' | 'FRAME' | 'CARD' | 'STROKE' | 'LINK' | 'IMAGE' | 'FILE';
export type Pen = 'PEN' | 'HIGHLIGHTER';
export type Shape = 'RECTANGLE' | 'ROUNDED' | 'ELLIPSE' | 'DIAMOND' | 'PARALLELOGRAM' | 'TRIANGLE';
export type Route = 'STRAIGHT' | 'ELBOW' | 'CURVED';
export type Head = 'NONE' | 'ARROW';

/** One object, as the contract carries it (schema WhiteboardElement). */
export interface Element {
  id: string;
  kind: ElementKind;
  x: number;
  y: number;
  width: number;
  height: number;
  text?: string;
  color_token?: string | null;
  shape?: Shape;
  from?: string | null;
  to?: string | null;
  route?: Route;
  start?: Head;
  end?: Head;
  target?: { kind: 'ENTRY'; id: string };
  locked?: boolean;
  /** A stroke's line: x and y after each other, relative to (x, y). */
  points?: number[];
  pen?: Pen;
  /** A web link's address; the board draws its card and never loads it. */
  url?: string;
  /** The group the object belongs to: chosen, moved and copied together. */
  group?: string | null;
  /** The upload a picture or a file draws, placed on the board first. */
  media_id?: string;
}

/** The first release's hard limit of one board (`whiteboards.too_many_elements`). */
export const MAX_ELEMENTS = 5000;

export const SHAPES: readonly Shape[] = ['RECTANGLE', 'ROUNDED', 'ELLIPSE', 'DIAMOND', 'PARALLELOGRAM', 'TRIANGLE'];
export const ROUTES: readonly Route[] = ['STRAIGHT', 'ELBOW', 'CURVED'];

/** The size an object takes when it is put on the board, in board units. */
export const DEFAULT_SIZE: Record<ElementKind, { width: number; height: number }> = {
  STICKY: { width: 160, height: 160 },
  SHAPE: { width: 160, height: 100 },
  TEXT: { width: 240, height: 48 },
  CONNECTOR: { width: 0, height: 0 },
  FRAME: { width: 800, height: 560 },
  CARD: { width: 260, height: 112 },
  STROKE: { width: 0, height: 0 },
  LINK: { width: 280, height: 72 },
  IMAGE: { width: 320, height: 200 },
  FILE: { width: 280, height: 72 },
};

const FIELDS = ['kind', 'x', 'y', 'width', 'height', 'text', 'color_token', 'shape', 'from', 'to', 'route', 'start', 'end', 'target', 'locked', 'points', 'pen', 'url', 'group', 'media_id'] as const;

type Stored = Y.Map<unknown>;

/** A board's live document, read and written as objects. */
export class Board {
  readonly doc: Y.Doc;
  readonly #map: Y.Map<Stored>;

  constructor(doc: Y.Doc) {
    this.doc = doc;
    this.#map = doc.getMap<Stored>('elements');
  }

  get size(): number {
    return this.#map.size;
  }

  /** Every object in drawing order, the last on top. */
  elements(): Element[] {
    const rows: { z: number; element: Element }[] = [];
    this.#map.forEach((stored, id) => rows.push({ z: Number(stored.get('z') ?? 0), element: read(id, stored) }));
    rows.sort((a, b) => a.z - b.z || (a.element.id < b.element.id ? -1 : a.element.id > b.element.id ? 1 : 0));
    return rows.map((row) => row.element);
  }

  get(id: string): Element | undefined {
    const stored = this.#map.get(id);
    return stored ? read(id, stored) : undefined;
  }

  /** The projection the board saves: the objects in drawing order, in the contract's shape. */
  scene(): Element[] {
    return this.elements();
  }

  /** Puts an object on top of the others. Refused past the board's limit. */
  add(element: Element): boolean {
    if (this.#map.size >= MAX_ELEMENTS || this.#map.has(element.id)) return false;
    this.doc.transact(() => {
      const stored = new Y.Map<unknown>();
      write(stored, element);
      stored.set('z', this.#top() + 1);
      this.#map.set(element.id, stored);
    });
    return true;
  }

  /** Changes some fields of an object; `null` clears one. */
  update(id: string, patch: Partial<Omit<Element, 'id'>>): void {
    const stored = this.#map.get(id);
    if (!stored) return;
    this.doc.transact(() => write(stored, patch));
  }

  /** Puts several objects on top of the others in one change, in their order. */
  addAll(elements: readonly Element[]): boolean {
    if (this.#map.size + elements.length > MAX_ELEMENTS || elements.some((e) => this.#map.has(e.id))) return false;
    this.doc.transact(() => {
      let z = this.#top();
      for (const element of elements) {
        const stored = new Y.Map<unknown>();
        write(stored, element);
        stored.set('z', (z += 1));
        this.#map.set(element.id, stored);
      }
    });
    return true;
  }

  /** Puts objects into one new group, or takes them out of theirs with `null`. */
  group(ids: readonly string[], group: string | null): void {
    this.doc.transact(() => {
      for (const id of ids) {
        const stored = this.#map.get(id);
        if (!stored || stored.get('kind') === 'CONNECTOR') continue;
        if (group) stored.set('group', group);
        else stored.delete('group');
      }
    });
  }

  /** Moves objects by a distance, together; a locked one stays. */
  move(ids: readonly string[], dx: number, dy: number): void {
    this.doc.transact(() => {
      for (const id of ids) {
        const stored = this.#map.get(id);
        if (!stored || stored.get('locked') === true) continue;
        stored.set('x', Number(stored.get('x') ?? 0) + dx);
        stored.set('y', Number(stored.get('y') ?? 0) + dy);
      }
    });
  }

  /** Takes objects off the board, and every connector attached to one of them. */
  remove(ids: readonly string[]): void {
    const gone = new Set(ids);
    this.doc.transact(() => {
      this.#map.forEach((stored, id) => {
        if (stored.get('kind') === 'CONNECTOR' && (gone.has(String(stored.get('from') ?? '')) || gone.has(String(stored.get('to') ?? '')))) {
          gone.add(id);
        }
      });
      for (const id of gone) this.#map.delete(id);
    });
  }

  front(id: string): void {
    const stored = this.#map.get(id);
    if (stored) stored.set('z', this.#top() + 1);
  }

  back(id: string): void {
    const stored = this.#map.get(id);
    if (!stored) return;
    let low = 0;
    this.#map.forEach((other) => (low = Math.min(low, Number(other.get('z') ?? 0))));
    stored.set('z', low - 1);
  }

  /**
   * A sticky note becomes the card of the entry it was turned into, in its place: the same
   * identifier, so a connector attached to the note stays attached to the card (UC-CNV-02).
   */
  toCard(id: string, entryId: string): boolean {
    const stored = this.#map.get(id);
    if (!stored || stored.get('kind') !== 'STICKY') return false;
    this.doc.transact(() => {
      stored.set('kind', 'CARD');
      stored.set('target', { kind: 'ENTRY', id: entryId });
      stored.delete('text');
      stored.delete('color_token');
      const size = DEFAULT_SIZE.CARD;
      stored.set('width', Math.max(Number(stored.get('width') ?? 0), size.width));
      stored.set('height', size.height);
    });
    return true;
  }

  /**
   * Puts a whole scene in place of what stands, in one change everyone sees at once: an older
   * version brought back (UC-CNV-06). Past the board's limit nothing changes.
   */
  replace(elements: readonly Element[]): boolean {
    if (elements.length > MAX_ELEMENTS) return false;
    this.doc.transact(() => {
      for (const id of [...this.#map.keys()]) this.#map.delete(id);
      elements.forEach((element, index) => {
        const stored = new Y.Map<unknown>();
        write(stored, element);
        stored.set('z', index);
        this.#map.set(element.id, stored);
      });
    });
    return true;
  }

  /** Writes a saved scene into an empty document - the first writer's job. */
  seed(elements: readonly Element[]): boolean {
    if (this.#map.size > 0 || elements.length === 0) return false;
    this.doc.transact(() => {
      elements.slice(0, MAX_ELEMENTS).forEach((element, index) => {
        const stored = new Y.Map<unknown>();
        write(stored, element);
        stored.set('z', index);
        this.#map.set(element.id, stored);
      });
    });
    return true;
  }

  /** Calls back after every change, local or another writer's; answers the unsubscription. */
  observe(changed: () => void): () => void {
    const handler = () => changed();
    this.#map.observeDeep(handler);
    return () => this.#map.unobserveDeep(handler);
  }

  #top(): number {
    let top = 0;
    this.#map.forEach((stored) => (top = Math.max(top, Number(stored.get('z') ?? 0))));
    return top;
  }
}

function write(stored: Stored, fields: Partial<Element>): void {
  for (const field of FIELDS) {
    if (!(field in fields)) continue;
    const value = fields[field];
    if (value === null || value === undefined) stored.delete(field);
    else stored.set(field, value);
  }
}

function read(id: string, stored: Stored): Element {
  const element: Element = {
    id,
    kind: String(stored.get('kind') ?? 'STICKY') as ElementKind,
    x: Number(stored.get('x') ?? 0),
    y: Number(stored.get('y') ?? 0),
    width: Number(stored.get('width') ?? 0),
    height: Number(stored.get('height') ?? 0),
  };
  for (const field of ['text', 'color_token', 'shape', 'from', 'to', 'route', 'start', 'end', 'pen', 'url', 'group', 'media_id'] as const) {
    const value = stored.get(field);
    if (typeof value === 'string') (element as unknown as Record<string, unknown>)[field] = value;
  }
  const target = stored.get('target') as { kind?: string; id?: string } | undefined;
  if (target && typeof target.id === 'string') element.target = { kind: 'ENTRY', id: target.id };
  if (stored.get('locked') === true) element.locked = true;
  const points = stored.get('points');
  if (Array.isArray(points)) element.points = points.map(Number);
  return element;
}

/** A new identifier for an object, shaped as a reference's anchor. */
export function elementId(random: () => string = () => crypto.randomUUID()): string {
  return random().replaceAll('-', '').slice(0, 20);
}
