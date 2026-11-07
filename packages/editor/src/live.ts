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
 * **It reconnects** with a doubling wait up to thirty seconds, and answers the server's state
 * vector with only what the server lacks. The server reads no update, so its vector comes empty
 * (SY-F); the server's snapshot and log arrive before it, and the vector derived from them stands
 * in - a writer back from a tunnel sends its offline words, not the whole page again.
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
/**
 * How long a socket may take to open. A proxy that holds the upgrade without answering would
 * otherwise leave the document "connecting" for ever, never counted as a failed attempt (rule 7).
 */
export const OPEN_DEADLINE_MS = 10_000;

/** The close code the server ends a connection with when access ends while connected. */
export const CLOSE_ACCESS_ENDED = 4403;
/** The close code of a writer past its byte budget; the reason names the seconds to wait. */
export const CLOSE_TOO_FAST = 4429;

/** The part of a WebSocket the provider uses, so a test can hand in a fake. */
export interface LiveSocket {
  binaryType: string;
  readonly readyState: number;
  onopen: ((event: unknown) => void) | null;
  onmessage: ((event: { data: unknown }) => void) | null;
  onclose: ((event: { code: number; reason?: string }) => void) | null;
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
  /** The timer for a socket's opening deadline, injected like `schedule`. */
  readonly deadline?: (run: () => void, ms: number) => () => void;
  /**
   * Keeps a writer's words the server has not had yet on the device, so a tab closed or reloaded
   * offline loses none: what it loads is applied before the first connection and sent on it.
   */
  readonly keep?: UpdateKeeper;
  /** Told how many local changes wait for the server, whenever that changes. */
  readonly onUnsent?: (count: number) => void;
}

/** The device's store of a document's unsent updates; `save` replaces what it held. */
export interface UpdateKeeper {
  load(): Promise<readonly Uint8Array[]>;
  save(updates: readonly Uint8Array[]): Promise<void>;
}

const OPEN = 1;

function later(run: () => void, ms: number): () => void {
  const handle = setTimeout(run, ms);
  return () => clearTimeout(handle);
}

export class LiveDocument {
  readonly doc: Y.Doc;
  readonly awareness: awarenessProtocol.Awareness;
  readonly #options: LiveDocumentOptions;
  #socket: LiveSocket | undefined;
  #status: LiveStatus = 'connecting';
  #attempt = 0;
  #cancelRetry: (() => void) | undefined;
  #cancelDeadline: (() => void) | undefined;
  #destroyed = false;
  #synced = false;
  /** The server's snapshot and log on this connection, until its state vector arrives. */
  #held: Uint8Array[] = [];
  #answered = false;
  /** Local changes made while no answered connection carried them, merged into one update. */
  #unsent: Uint8Array[] = [];
  #unsentCount = 0;

  constructor(doc: Y.Doc, options: LiveDocumentOptions) {
    this.doc = doc;
    this.awareness = new awarenessProtocol.Awareness(doc);
    this.#options = options;
    doc.on('update', this.#onUpdate);
    this.awareness.on('update', this.#onAwareness);
    void this.#restore().then(() => this.#connect());
  }

  /** How many local changes wait for the server. */
  get unsent(): number {
    return this.#unsentCount;
  }

  async #restore(): Promise<void> {
    if (!this.#options.keep || !this.#options.writable) return;
    let kept: readonly Uint8Array[] = [];
    try {
      kept = await this.#options.keep.load();
    } catch {
      return;
    }
    if (kept.length === 0 || this.#destroyed) return;
    // As the server's: not sent on its own, but part of what the answer to its vector carries.
    for (const update of kept) Y.applyUpdate(this.doc, update, this);
    this.#unsent = [Y.mergeUpdates([...kept])];
    this.#setUnsent(kept.length);
  }

  #setUnsent(count: number): void {
    this.#unsentCount = count;
    this.#options.onUnsent?.(count);
  }

  #keep(): void {
    void this.#options.keep?.save(this.#unsent).catch(() => {});
  }

  get status(): LiveStatus {
    return this.#status;
  }

  /**
   * How many connections in a row failed before they opened. Several while the rest of the API
   * answers is the mark of a proxy that does not pass the upgrade: the reader is told so instead
   * of a retry that never ends.
   */
  get failedAttempts(): number {
    return this.#attempt;
  }

  /** Whether the server's state has arrived on the current connection. */
  get synced(): boolean {
    return this.#synced;
  }

  destroy(): void {
    this.#destroyed = true;
    this.#cancelRetry?.();
    this.#cancelDeadline?.();
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
    this.#held = [];
    this.#answered = false;
    socket.binaryType = 'arraybuffer';
    this.#cancelDeadline = (this.#options.deadline ?? later)(() => {
      if (this.#socket !== socket || socket.readyState === OPEN) return;
      this.#socket = undefined;
      socket.close(1000);
      this.#retry();
    }, OPEN_DEADLINE_MS);
    socket.onopen = () => {
      this.#cancelDeadline?.();
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
      this.#cancelDeadline?.();
      this.#socket = undefined;
      // Other people's cursors are gone with the connection.
      const others = [...this.awareness.getStates().keys()].filter((id) => id !== this.doc.clientID);
      awarenessProtocol.removeAwarenessStates(this.awareness, others, 'connection');
      if (event.code === CLOSE_ACCESS_ENDED) {
        this.#setStatus('refused');
        return;
      }
      if (event.code === CLOSE_TOO_FAST) {
        // Reconnecting sooner would be refused again; what was written meanwhile waits in the doc.
        const seconds = Number(event.reason?.split(':')[1]);
        this.#retry(Number.isFinite(seconds) && seconds > 0 ? seconds * 1000 : RECONNECT_MAX_MS);
        return;
      }
      this.#retry();
    };
    socket.onerror = () => {};
  }

  #retry(atLeast = 0): void {
    if (this.#destroyed) return;
    this.#setStatus('offline');
    const wait = Math.max(atLeast, Math.min(RECONNECT_MAX_MS, RECONNECT_BASE_MS * 2 ** this.#attempt++));
    this.#cancelRetry = (this.#options.schedule ?? later)(() => void this.#connect(), wait);
  }

  #receive(data: Uint8Array): void {
    const decoder = decoding.createDecoder(data);
    const type = decoding.readVarUint(decoder);
    switch (type) {
      case MESSAGE_SYNC: {
        if (this.#options.writable && !this.#answered && this.#answerStep1(data)) return;
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

  /**
   * Holds what the server hands out before its state vector, and answers that vector with the
   * difference to it. False leaves the message to the sync protocol.
   */
  #answerStep1(data: Uint8Array): boolean {
    const decoder = decoding.createDecoder(data);
    decoding.readVarUint(decoder);
    const kind = decoding.readVarUint(decoder);
    if (kind !== syncProtocol.messageYjsSyncStep1) {
      this.#held.push(decoding.readVarUint8Array(decoder));
      return false;
    }
    let vector = decoding.readVarUint8Array(decoder);
    if (Y.decodeStateVector(vector).size === 0 && this.#held.length > 0) {
      vector = Y.encodeStateVectorFromUpdate(Y.mergeUpdates(this.#held));
    }
    this.#held = [];
    this.#answered = true;
    const encoder = encoding.createEncoder();
    encoding.writeVarUint(encoder, MESSAGE_SYNC);
    syncProtocol.writeSyncStep2(encoder, this.doc, vector);
    this.#send(encoding.toUint8Array(encoder));
    if (this.#unsentCount > 0) {
      this.#unsent = [];
      this.#setUnsent(0);
      this.#keep();
    }
    if (!this.#synced) {
      this.#synced = true;
      this.#options.onSynced?.();
    }
    return true;
  }

  #onUpdate = (update: Uint8Array, origin: unknown) => {
    if (origin === this || !this.#options.writable) return;
    if (this.#socket?.readyState !== OPEN || !this.#answered) {
      this.#unsent = [Y.mergeUpdates([...this.#unsent, update])];
      this.#setUnsent(this.#unsentCount + 1);
      this.#keep();
    }
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
