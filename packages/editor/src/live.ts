// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A live document's connection: the editors' sync and awareness framing over the server's
 * WebSocket, and the server's own two messages (offline-sync.md §13) - `100`, a writer's snapshot,
 * sent when the server asks with `101`.
 *
 * **The socket is the host's.** It is opened by a function the application hands over, which is
 * where the bearer goes in as the subprotocol (the sync engine's `liveSocket`); this package never
 * sees a token or composes an address.
 *
 * **A reader sends nothing but its awareness.** The server closes the connection of a reader that
 * sends an update (`realtime.read_only`), so a read-only document's local changes - there should
 * be none, the editor is not editable - are dropped rather than sent.
 *
 * **It reconnects** with a doubling wait up to thirty seconds, and on every connection sends what
 * it holds: the server keeps no state vector (SY-F), so a reconnect re-sends the writer's state and
 * the log's duplicates are harmless to the CRDT.
 */

import * as decoding from 'lib0/decoding';
import * as encoding from 'lib0/encoding';
import * as awarenessProtocol from 'y-protocols/awareness';
import * as syncProtocol from 'y-protocols/sync';
import * as Y from 'yjs';

const MESSAGE_SYNC = 0;
const MESSAGE_AWARENESS = 1;
const MESSAGE_COMPACT = 100;
const MESSAGE_COMPACTION_DUE = 101;

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_MAX_MS = 30_000;

/** The close code the server ends a connection with when access ends while connected. */
export const CLOSE_ACCESS_ENDED = 4403;

/** The part of a WebSocket the provider uses, so a test can hand in a fake. */
export interface LiveSocket {
  binaryType: string;
  readonly readyState: number;
  onopen: ((event: unknown) => void) | null;
  onmessage: ((event: { data: unknown }) => void) | null;
  onclose: ((event: { code: number }) => void) | null;
  onerror: ((event: unknown) => void) | null;
  send(data: Uint8Array): void;
  close(code?: number): void;
}

export type LiveStatus = 'connecting' | 'connected' | 'offline' | 'refused';

export interface LiveDocumentOptions {
  /** Opens a new socket to the document. Asked again on every reconnect. */
  readonly open: () => LiveSocket | Promise<LiveSocket>;
  /** Whether the reader may write; a reader's local changes are not sent. */
  readonly writable: boolean;
  /** Told whenever the connection's state changes. */
  readonly onStatus?: (status: LiveStatus) => void;
  /** Told once the server's snapshot and log have been applied after a connection opened. */
  readonly onSynced?: () => void;
  /** The timer, injected so a test can run the reconnect at once (rule 4). */
  readonly schedule?: (run: () => void, ms: number) => () => void;
}

const OPEN = 1;

export class LiveDocument {
  readonly doc: Y.Doc;
  readonly awareness: awarenessProtocol.Awareness;
  readonly #options: LiveDocumentOptions;
  #socket: LiveSocket | undefined;
  #status: LiveStatus = 'connecting';
  #attempt = 0;
  #cancelRetry: (() => void) | undefined;
  #destroyed = false;
  #synced = false;

  constructor(doc: Y.Doc, options: LiveDocumentOptions) {
    this.doc = doc;
    this.awareness = new awarenessProtocol.Awareness(doc);
    this.#options = options;
    doc.on('update', this.#onUpdate);
    this.awareness.on('update', this.#onAwareness);
    void this.#connect();
  }

  get status(): LiveStatus {
    return this.#status;
  }

  /** Whether the server's state has arrived on the current connection. */
  get synced(): boolean {
    return this.#synced;
  }

  destroy(): void {
    this.#destroyed = true;
    this.#cancelRetry?.();
    this.doc.off('update', this.#onUpdate);
    this.awareness.off('update', this.#onAwareness);
    awarenessProtocol.removeAwarenessStates(this.awareness, [this.doc.clientID], 'destroy');
    this.#socket?.close(1000);
    this.#socket = undefined;
    this.awareness.destroy();
  }

  async #connect(): Promise<void> {
    if (this.#destroyed) return;
    this.#setStatus('connecting');
    let socket: LiveSocket;
    try {
      socket = await this.#options.open();
    } catch {
      this.#retry();
      return;
    }
    if (this.#destroyed) {
      socket.close(1000);
      return;
    }
    this.#socket = socket;
    this.#synced = false;
    socket.binaryType = 'arraybuffer';
    socket.onopen = () => {
      this.#attempt = 0;
      this.#setStatus('connected');
      if (this.#options.writable) {
        const encoder = encoding.createEncoder();
        encoding.writeVarUint(encoder, MESSAGE_SYNC);
        syncProtocol.writeSyncStep1(encoder, this.doc);
        socket.send(encoding.toUint8Array(encoder));
      }
      if (this.awareness.getLocalState() !== null) {
        socket.send(awarenessMessage(this.awareness, [this.doc.clientID]));
      }
    };
    socket.onmessage = (event) => this.#receive(new Uint8Array(event.data as ArrayBuffer));
    socket.onclose = (event) => {
      if (this.#socket !== socket) return;
      this.#socket = undefined;
      // Other people's cursors are gone with the connection.
      const others = [...this.awareness.getStates().keys()].filter((id) => id !== this.doc.clientID);
      awarenessProtocol.removeAwarenessStates(this.awareness, others, 'connection');
      if (event.code === CLOSE_ACCESS_ENDED) {
        this.#setStatus('refused');
        return;
      }
      this.#retry();
    };
    socket.onerror = () => {};
  }

  #retry(): void {
    if (this.#destroyed) return;
    this.#setStatus('offline');
    const wait = Math.min(RECONNECT_MAX_MS, RECONNECT_BASE_MS * 2 ** this.#attempt++);
    const schedule = this.#options.schedule ?? ((run, ms) => {
      const handle = setTimeout(run, ms);
      return () => clearTimeout(handle);
    });
    this.#cancelRetry = schedule(() => void this.#connect(), wait);
  }

  #receive(data: Uint8Array): void {
    const decoder = decoding.createDecoder(data);
    const type = decoding.readVarUint(decoder);
    switch (type) {
      case MESSAGE_SYNC: {
        const encoder = encoding.createEncoder();
        encoding.writeVarUint(encoder, MESSAGE_SYNC);
        const kind = syncProtocol.readSyncMessage(decoder, encoder, this.doc, this);
        // A reply to the server's state vector is everything this side holds - a writer's only.
        if (kind === syncProtocol.messageYjsSyncStep1 && this.#options.writable && encoding.length(encoder) > 1) {
          this.#send(encoding.toUint8Array(encoder));
        }
        if (!this.#synced && (kind === syncProtocol.messageYjsSyncStep1 || !this.#options.writable)) {
          this.#synced = true;
          this.#options.onSynced?.();
        }
        return;
      }
      case MESSAGE_AWARENESS:
        awarenessProtocol.applyAwarenessUpdate(this.awareness, decoding.readVarUint8Array(decoder), this);
        return;
      case MESSAGE_COMPACTION_DUE: {
        if (!this.#options.writable) return;
        const head = decoding.readVarUint(decoder);
        const encoder = encoding.createEncoder();
        encoding.writeVarUint(encoder, MESSAGE_COMPACT);
        encoding.writeVarUint(encoder, head);
        encoding.writeVarUint8Array(encoder, Y.encodeStateAsUpdate(this.doc));
        this.#send(encoding.toUint8Array(encoder));
        return;
      }
      default:
        // A type this client does not speak, as the protocol allows.
        return;
    }
  }

  #onUpdate = (update: Uint8Array, origin: unknown) => {
    if (origin === this || !this.#options.writable) return;
    const encoder = encoding.createEncoder();
    encoding.writeVarUint(encoder, MESSAGE_SYNC);
    syncProtocol.writeUpdate(encoder, update);
    this.#send(encoding.toUint8Array(encoder));
  };

  #onAwareness = ({ added, updated, removed }: { added: number[]; updated: number[]; removed: number[] }, origin: unknown) => {
    if (origin === this) return;
    this.#send(awarenessMessage(this.awareness, [...added, ...updated, ...removed]));
  };

  #send(message: Uint8Array): void {
    if (this.#socket?.readyState === OPEN) this.#socket.send(message);
  }

  #setStatus(status: LiveStatus): void {
    if (status === this.#status) return;
    this.#status = status;
    this.#options.onStatus?.(status);
  }
}

function awarenessMessage(awareness: awarenessProtocol.Awareness, clients: number[]): Uint8Array {
  const encoder = encoding.createEncoder();
  encoding.writeVarUint(encoder, MESSAGE_AWARENESS);
  encoding.writeVarUint8Array(encoder, awarenessProtocol.encodeAwarenessUpdate(awareness, clients));
  return encoding.toUint8Array(encoder);
}
