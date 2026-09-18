// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// A server in memory, as much of one as the conformance runner (F6-08) touches: the fixtures it
// makes, the three sync verbs, the stream, and one read. It behaves the way offline-sync.md says
// the real one does on the paths the runner takes - once per op_id, a refusal for a cursor it
// never minted, an ACCESS_REVOKED record for the account whose membership went - so that the
// runner can be held to its own claims before it meets the Compose stack.

import { TransportError } from '../src/errors.ts';
import { mintUuidV7 } from '../src/device.ts';
import type {
  ByteTransfer,
  Clock,
  RequestOptions,
  Response,
  SnapshotLine,
  SnapshotOptions,
  StreamConnection,
  StreamOptions,
  Transport,
  TransportDocument,
} from '../src/ports.ts';
import type { ChangeRecord, SyncMutation, SyncMutationResult } from '../src/schema.ts';

type Document = Record<string, unknown>;

interface Change {
  readonly seq: number;
  /** The account the record is for, or every account. */
  readonly forAccount?: string;
  readonly record: ChangeRecord;
}

const OWNER = 'account-owner';
const OWNER_TOKEN = 'hbt_pat_owner';

function problem(status: number, code: string, detailCode?: string): TransportError {
  return new TransportError('problem', { status, code, detailCode });
}

export class FakeServer {
  readonly #clock: Clock;
  readonly #accounts = new Map<string, string>([[OWNER_TOKEN, OWNER]]);
  readonly #tokens = new Map<string, string>();
  readonly #containers = new Map<string, Document>();
  readonly #items = new Map<string, Document>();
  /** membership id → { account, hub } */
  readonly #memberships = new Map<string, { account: string; hub: string }>();
  readonly #applied = new Map<string, SyncMutationResult>();
  readonly #changes: Change[] = [];
  /** Every push, as it arrived. */
  readonly pushes: { token: string; body: unknown }[] = [];

  constructor(clock: Clock) {
    this.#clock = clock;
  }

  get ownerToken(): string {
    return OWNER_TOKEN;
  }

  get containers(): ReadonlyMap<string, Document> {
    return this.#containers;
  }

  get memberships(): ReadonlyMap<string, { account: string; hub: string }> {
    return this.#memberships;
  }

  get tokens(): ReadonlyMap<string, string> {
    return this.#tokens;
  }

  transportFor(token: string): Transport {
    return new FakeServerTransport(this, token);
  }

  #account(token: string | undefined): string {
    const account = token ? this.#accounts.get(token) : undefined;
    if (!account) throw problem(401, 'unauthorized');
    return account;
  }

  #hubOf(container: Document | undefined): string | undefined {
    let current = container;
    while (current) {
      if (current.type === 'HUB') return String(current.id);
      current = typeof current.parent_id === 'string' ? this.#containers.get(current.parent_id) : undefined;
    }
    return undefined;
  }

  #mayRead(account: string, hub: string | undefined): boolean {
    if (account === OWNER) return true;
    return [...this.#memberships.values()].some((m) => m.account === account && m.hub === hub);
  }

  #log(record: ChangeRecord, forAccount?: string): void {
    this.#changes.push({ seq: this.#changes.length + 1, forAccount, record });
  }

  #cursor(): string {
    return `c${this.#changes.length}`;
  }

  #itemRecord(item: Document): ChangeRecord {
    return { op: 'UPSERT', entity: 'item', entity_id: String(item.id), container_id: String(item.collection_id), hlc: '', payload: item } as ChangeRecord;
  }

  #containerRecord(container: Document): ChangeRecord {
    return { op: 'UPSERT', entity: 'container', entity_id: String(container.id), container_id: (container.parent_id as string | null) ?? null, hlc: '', payload: container } as ChangeRecord;
  }

  /** What the account may see, as records - the snapshot. */
  #visible(account: string): ChangeRecord[] {
    const records: ChangeRecord[] = [];
    for (const container of this.#containers.values()) {
      if (this.#mayRead(account, this.#hubOf(container))) records.push(this.#containerRecord(container));
    }
    for (const item of this.#items.values()) {
      const collection = this.#containers.get(String(item.collection_id));
      if (this.#mayRead(account, this.#hubOf(collection))) records.push(this.#itemRecord(item));
    }
    return records;
  }

  request(method: string, path: string, body: unknown, token: string | undefined): unknown {
    const account = this.#account(token);
    const b = (body ?? {}) as Document;
    let match: RegExpExecArray | null;

    if (method === 'POST' && path === '/containers') {
      const id = mintUuidV7(this.#clock);
      const container: Document = { id, type: b.type, name: b.name, parent_id: b.parent_id ?? null, version: 1 };
      this.#containers.set(id, container);
      this.#log(this.#containerRecord(container));
      return container;
    }
    if (method === 'DELETE' && (match = /^\/containers\/([^/]+)$/.exec(path))) {
      const id = match[1] ?? '';
      if (!this.#containers.has(id)) throw problem(404, 'not_found');
      // The subtree goes with it, as a trashed hub takes its collections.
      const gone = [...this.#containers].filter(([childId, child]) => childId === id || this.#hubOf(child) === id).map(([childId]) => childId);
      for (const childId of gone) this.#containers.delete(childId);
      this.#log({ op: 'DELETE', entity: 'container', entity_id: id, container_id: null, hlc: '' } as ChangeRecord);
      return undefined;
    }
    if (method === 'POST' && path === '/auth/service-accounts') {
      const id = mintUuidV7(this.#clock);
      return { id, display_name: b.display_name, kind: 'SERVICE' };
    }
    if (method === 'POST' && path === '/auth/tokens') {
      const id = mintUuidV7(this.#clock);
      const secret = `hbt_pat_${id}`;
      this.#accounts.set(secret, String(b.account_id ?? account));
      this.#tokens.set(id, secret);
      return { id, token: secret };
    }
    if (method === 'DELETE' && (match = /^\/auth\/tokens\/([^/]+)$/.exec(path))) {
      const secret = this.#tokens.get(match[1] ?? '');
      if (!secret) throw problem(404, 'not_found');
      this.#tokens.delete(match[1] ?? '');
      this.#accounts.delete(secret);
      return undefined;
    }
    if (method === 'POST' && path === '/memberships') {
      const id = mintUuidV7(this.#clock);
      this.#memberships.set(id, { account: String(b.account_id), hub: String(b.scope_id) });
      return { id, role: b.role };
    }
    if (method === 'DELETE' && (match = /^\/memberships\/([^/]+)$/.exec(path))) {
      const membership = this.#memberships.get(match[1] ?? '');
      if (!membership) throw problem(404, 'not_found');
      this.#memberships.delete(match[1] ?? '');
      this.#log({ op: 'ACCESS_REVOKED', entity: 'container', entity_id: membership.hub, container_id: membership.hub, hlc: '' } as ChangeRecord, membership.account);
      return undefined;
    }
    if (method === 'GET' && (match = /^\/items\/([^/]+)$/.exec(path))) {
      const item = this.#items.get(match[1] ?? '');
      if (!item) throw problem(404, 'not_found');
      return item;
    }
    if (method === 'POST' && path === '/sync:pull') {
      const cursor = b.cursor as string | null | undefined;
      let from = 0;
      if (cursor) {
        const parsed = /^c(\d+)$/.exec(cursor);
        if (!parsed) throw problem(400, 'validation_failed', 'sync.cursor_invalid');
        from = Number(parsed[1]);
      }
      const changes = cursor
        ? this.#changes.filter((c) => c.seq > from && (!c.forAccount || c.forAccount === account)).map((c) => c.record)
        : this.#visible(account);
      return { changes, cursor: this.#cursor(), has_more: false, tombstone_window_days: 90 };
    }
    if (method === 'POST' && path === '/sync:push') {
      this.pushes.push({ token: token ?? '', body });
      const results = ((b.mutations ?? []) as SyncMutation[]).map((mutation) => this.#apply(account, mutation));
      return { results, cursor: this.#cursor() };
    }
    throw problem(404, 'not_found');
  }

  #apply(account: string, mutation: SyncMutation): SyncMutationResult {
    const opId = String(mutation.op_id);
    const seen = this.#applied.get(opId);
    if (seen) return seen;
    const itemId = String(mutation.item_id);
    let result: SyncMutationResult;
    switch (mutation.kind) {
      case 'ITEM_CREATE': {
        const payload = (mutation.payload ?? {}) as Document;
        const collection = this.#containers.get(String(payload.collection_id));
        if (!this.#mayRead(account, this.#hubOf(collection))) {
          result = { op_id: opId, result: 'REJECTED', entity_id: itemId, error: { code: 'forbidden' } } as SyncMutationResult;
          break;
        }
        if (!payload.title) {
          result = { op_id: opId, result: 'REJECTED', entity_id: itemId, error: { code: 'validation_failed', message_code: 'items.title_required' } } as SyncMutationResult;
          break;
        }
        const item: Document = { id: itemId, ...payload, version: 1, state: 'OPEN', later_field: 'a field this engine has never seen' };
        this.#items.set(itemId, item);
        this.#log(this.#itemRecord(item));
        result = { op_id: opId, result: 'APPLIED', entity_id: itemId, server_state: item } as SyncMutationResult;
        break;
      }
      case 'ITEM_PATCH': {
        const item = this.#items.get(itemId);
        if (!item) {
          result = { op_id: opId, result: 'REJECTED', entity_id: itemId, error: { code: 'not_found' } } as SyncMutationResult;
          break;
        }
        const fields = (mutation.fields ?? {}) as Record<string, { value: unknown }>;
        for (const [key, field] of Object.entries(fields)) item[key] = field.value;
        item.version = Number(item.version) + 1;
        this.#log(this.#itemRecord(item));
        result = { op_id: opId, result: 'APPLIED', entity_id: itemId, server_state: item } as SyncMutationResult;
        break;
      }
      default:
        result = { op_id: opId, result: 'REJECTED', entity_id: itemId, error: { code: 'validation_failed', message_code: 'sync.kind_unknown' } } as SyncMutationResult;
    }
    this.#applied.set(opId, structuredClone(result));
    return structuredClone(result);
  }

  snapshotFor(token: string | undefined): { records: ChangeRecord[]; cursor: string } {
    const account = this.#account(token);
    return { records: this.#visible(account), cursor: this.#cursor() };
  }
}

class FakeServerTransport implements Transport {
  readonly #server: FakeServer;
  readonly #token: string;

  constructor(server: FakeServer, token: string) {
    this.#server = server;
    this.#token = token;
  }

  async get<T>(path: string, options: RequestOptions): Promise<Response<T>> {
    await Promise.resolve();
    return { status: 200, body: this.#server.request('GET', path, undefined, options.token ?? this.#token) as T };
  }

  async send<T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body: unknown, options: RequestOptions): Promise<Response<T>> {
    await Promise.resolve();
    return { status: 200, body: structuredClone(this.#server.request(method, path, body, options.token ?? this.#token)) as T };
  }

  async snapshot(_path: string, _body: unknown, options: SnapshotOptions): Promise<AsyncIterable<SnapshotLine>> {
    await Promise.resolve();
    const { records, cursor } = this.#server.snapshotFor(options.token ?? this.#token);
    return (async function* () {
      for (const record of records) yield { kind: 'record', record } as SnapshotLine;
      yield { kind: 'cursor', cursor } as SnapshotLine;
    })();
  }

  async stream(_path: string, options: StreamOptions): Promise<StreamConnection> {
    await Promise.resolve();
    const signal = options.signal;
    return {
      events: (async function* () {
        if (signal && !signal.aborted) {
          await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve(), { once: true }));
        }
      })(),
    };
  }

  async transfer(_transfer: ByteTransfer): Promise<void> {
    throw problem(404, 'not_found');
  }

  async document(_path: string, _body: unknown, _options: RequestOptions): Promise<TransportDocument> {
    throw problem(404, 'not_found');
  }
}
